// compact.go owns session compaction: when the live message list
// grows past CompactionConfig.AutoCompactInputTokens the agent
// collapses the older portion into a synthetic summary message,
// preserving a configurable tail of recent turns. Compaction must
// not split a tool_use from its matching tool_result — see
// adjustBoundary in T-204.
package agent

import (
	"context"
	"os"
	"strconv"

	"github.com/amemiya02/deepseekcode/internal/cacheunit"
	"github.com/amemiya02/deepseekcode/internal/llm"
	"github.com/amemiya02/deepseekcode/internal/tokenizer"
)

// CompactionConfig controls when and how the agent compacts its
// running message list. Values flow in via Agent.CompactionCfg;
// the agent reads them under no lock — set them once at construction.
type CompactionConfig struct {
	// PreserveRecentMessages is how many trailing messages stay
	// outside the compaction window (default 4).
	PreserveRecentMessages int

	// MaxEstimatedTokens caps the compacted summary's own token
	// budget — used by the summarizer to truncate (default 10_000).
	MaxEstimatedTokens int

	// AutoCompactInputTokens is the trigger threshold: once the
	// estimated token count of the full message list exceeds this
	// value, compaction fires (default 100_000; override via env
	// DEEPSEEKCODE_AUTO_COMPACT_INPUT_TOKENS).
	AutoCompactInputTokens int

	// CacheUnit, when > 0, is the measured DeepSeek cache-unit boundary
	// (tokens) used to align the rebuilt post-compaction tail (§3.6). After a
	// compaction the live transcript is [summary || kept-tail]; padding the
	// summary message so the whole rebuilt body lands on a cache-unit multiple
	// maximizes the reusable, fully-persisted portion on the next turn. 0
	// (default) = disabled, in which case CompactSession is byte-identical to
	// its pre-§3.6 behavior. Comes from a cacheprobe measurement; it never
	// touches the frozen prefix (the summary is an assistant body message).
	CacheUnit int
}

// CompactionResult is what CompactSession produces. Summary == ""
// means "no compaction performed" — the caller should leave the
// message list untouched.
type CompactionResult struct {
	Summary        string
	FromIdx, ToIdx int
	RemovedCount   int
	SummaryMessage llm.Message
	KeptMessages   []llm.Message
}

// perMessageOverhead is a small fixed surcharge per message to
// approximate role tags, JSON envelope, etc. that the wire format
// adds on top of raw block content.
const perMessageOverhead = 5

// defaultCharsPerToken is the cold-start chars-per-token prior — the divisor
// today's UTF-8 byte ÷ 4 heuristic uses. Go len(string) is byte length, so
// CJK characters (3 bytes each) naturally count ~3/4 token per char. This is
// intentional: no tokenizer dependency, and the byte count approximates the
// wire token count well enough for compaction triggering and budget projection.
// EstimateTokens == EstimateTokensCalibrated at this ratio, so behaviour is
// byte-identical until the first provider usage frame calibrates a per-session
// ratio (T4.1, Agent.calibrateCharsPerToken).
const defaultCharsPerToken = 4.0

// estimateChars sums the raw model-visible character count of the message
// blocks (text, thinking, tool name+input, tool result) and returns it with the
// message count. It isolates the deterministic char measurement from the
// divisor so calibration can re-divide the same chars; perMessageOverhead is
// added by the caller per message, never folded into the char count.
func estimateChars(messages []llm.Message) (chars, msgCount int) {
	for _, m := range messages {
		msgCount++
		for _, b := range m.Blocks {
			switch v := b.(type) {
			case llm.TextBlock:
				chars += len(v.Text)
			case llm.ThinkingBlock:
				chars += len(v.Text)
			case llm.ToolUseBlock:
				chars += len(v.Name) + len(v.Input)
			case llm.ToolResultBlock:
				chars += len(v.Content)
			}
		}
	}
	return chars, msgCount
}

// EstimateTokensCalibrated estimates the token count of a message list using
// the given chars-per-token ratio — a per-session value learned from provider
// usage frames (see Agent.calibrateCharsPerToken). A non-positive ratio falls
// back to the cold-start prior. Used only for compaction triggering and the
// pre-stream budget projection — never for cost computation. Cheap,
// deterministic, no tokenizer dependency.
//
// Rounding-locus note: this divides the SUMMED char count once, whereas the
// historical EstimateTokens floored each block's len/4 independently. For the
// existing strict test inputs every block length is a multiple of 4, so the
// results are identical; for other inputs the two differ by at most
// (blocks-1) tokens — negligible on a 1M window.
func EstimateTokensCalibrated(messages []llm.Message, charsPerToken float64) int {
	if charsPerToken <= 0 {
		charsPerToken = defaultCharsPerToken
	}
	chars, n := estimateChars(messages)
	return n*perMessageOverhead + int(float64(chars)/charsPerToken)
}

// EstimateInputTokens returns the exact tokenizer count of messages when the
// embedded V4 tokenizer is available, else the calibrated heuristic. This is
// the preferred token estimator for compaction triggering and budget projection.
func EstimateInputTokens(messages []llm.Message, charsPerToken float64) int {
	if n, ok := tokenizer.CountMessages(messages); ok {
		return n
	}
	return EstimateTokensCalibrated(messages, charsPerToken)
}

// EstimateTokens returns the cold-start (UTF-8 byte ÷ 4) token estimate. It
// is the uncalibrated wrapper over EstimateTokensCalibrated; callers holding a
// learned per-session ratio should use the calibrated form directly.
func EstimateTokens(messages []llm.Message) int {
	return EstimateTokensCalibrated(messages, defaultCharsPerToken)
}

// CompactSession runs the full pipeline: ShouldCompact →
// adjustBoundary → summarize. Returns a CompactionResult with
// Summary == "" when no compaction was performed (caller must
// check Summary before mutating its message list).
//
// CompactSession does NOT persist — the caller wires the result
// into Persister.ReplaceWithCompaction (T-209) and replaces its
// in-memory a.Messages slice.
func CompactSession(messages []llm.Message, cfg CompactionConfig, charsPerToken float64) CompactionResult {
	ok, fromIdx, toIdx := ShouldCompact(messages, cfg, charsPerToken)
	if !ok {
		return CompactionResult{}
	}
	toIdx = adjustBoundary(messages, toIdx)
	if toIdx <= fromIdx {
		return CompactionResult{}
	}
	// summarizeWithMerge folds a prior compaction summary (if the window opens
	// with one) into the fresh summary so early facts survive multi-round
	// compaction (T4.3).
	summary := summarizeWithMerge(messages[fromIdx:toIdx])
	if summary == "" {
		return CompactionResult{}
	}
	// The summary is an assistant-role body message, not a second system
	// message wedged after the real system prompt (T4.3). staticSystem() only
	// fingerprints a.System, so a body message — whatever its role — never
	// enters the cache prefix; assistant role keeps the wire shape
	// provider-conventional (one system message, at the head).
	summaryMsg := llm.Message{
		Role:   "assistant",
		Blocks: []llm.ContentBlock{llm.TextBlock{Text: summary}},
	}
	kept := make([]llm.Message, len(messages)-toIdx)
	copy(kept, messages[toIdx:])
	// §3.6: align the rebuilt tail ([summary || kept]) to a cache-unit boundary
	// so the post-compaction body ends on a fully-persisted cache block. Pads
	// the summary (an assistant BODY message), never the frozen prefix, so the
	// Prefix Fingerprint is untouched. cfg.CacheUnit==0 (default) = no-op:
	// alignTailToCacheUnit returns summaryMsg unchanged → byte-identical output.
	summaryMsg = alignTailToCacheUnit(summaryMsg, kept, cfg.CacheUnit)
	return CompactionResult{
		Summary:        summary,
		FromIdx:        fromIdx,
		ToIdx:          toIdx,
		RemovedCount:   toIdx - fromIdx,
		SummaryMessage: summaryMsg,
		KeptMessages:   kept,
	}
}

// alignTailToCacheUnit pads the post-compaction summary message so the rebuilt
// live tail ([summaryMsg || kept]) ends on a DeepSeek cache-unit boundary (§3.6).
// V4 reuses a stored prefix only up to its last COMPLETE compression block, so
// landing the rebuilt body on a unit multiple maximizes the reusable portion of
// the next turn's prompt. unit comes from a cacheprobe measurement.
//
// Gated on unit>0: when unit<=0 (the default everywhere today) it returns
// summaryMsg UNCHANGED, so CompactSession's output is byte-identical to its
// pre-§3.6 behavior. The padding is added as a SEPARATE trailing TextBlock so
// the summary's first block stays exactly "<summary>...</summary>" — keeping
// priorSummaryText's detection intact across multi-round compaction — and it
// only ever touches an assistant BODY message, never the frozen Static Prefix,
// so the Prefix Fingerprint is unaffected (guarded by
// TestCompactionPreservesFrozenPrefix).
func alignTailToCacheUnit(summaryMsg llm.Message, kept []llm.Message, unit int) llm.Message {
	if unit <= 0 {
		return summaryMsg
	}
	tail := append([]llm.Message{summaryMsg}, kept...)
	// Measure the tail and the pad with the SAME deterministic char estimator so
	// the boundary arithmetic is self-consistent (the exact-tokenizer path is
	// intentionally not used here: the pad is filler whose only job is to land
	// the body on a unit multiple under one consistent measure).
	have := EstimateTokensCalibrated(tail, defaultCharsPerToken)
	// The pad is appended as a TextBlock to an existing message, so its token
	// contribution is just the calibrated char estimate of its own text, net of
	// the per-message overhead the estimator adds to the synthetic wrapper.
	count := func(s string) int {
		return EstimateTokensCalibrated(
			[]llm.Message{{Blocks: []llm.ContentBlock{llm.TextBlock{Text: s}}}},
			defaultCharsPerToken,
		) - perMessageOverhead
	}
	pad := cacheunit.PadText(have, unit, count)
	if pad == "" {
		return summaryMsg
	}
	blocks := make([]llm.ContentBlock, len(summaryMsg.Blocks), len(summaryMsg.Blocks)+1)
	copy(blocks, summaryMsg.Blocks)
	blocks = append(blocks, llm.TextBlock{Text: pad})
	summaryMsg.Blocks = blocks
	return summaryMsg
}

// adjustBoundary tweaks toIdx so the compaction window doesn't
// split a tool_use from its matching tool_result. Two-pass:
//
//  1. Push toIdx forward past any tool_result whose tool_use is
//     inside [0, toIdx). The result must travel with its use, so
//     keeping the use compacted while the result lives in the tail
//     would leave the model facing a tool_result with no
//     preceding tool_use.
//  2. If after step 1 there are still tool_use IDs in [0, toIdx)
//     with no matching tool_result anywhere in the list, that
//     tool_use is "orphaned" — the corresponding result never
//     arrived (interrupted turn). Pull toIdx back so any message
//     containing an unmatched tool_use sits OUTSIDE the compaction
//     window, preserving an orphan use in the live tail rather
//     than silently dropping it.
//
// Returns toIdx clamped to [0, len(messages)].
func adjustBoundary(messages []llm.Message, toIdx int) int {
	if toIdx <= 0 {
		return 0
	}
	if toIdx > len(messages) {
		toIdx = len(messages)
	}

	inWindow := collectToolUseIDs(messages[:toIdx])
	if len(inWindow) == 0 {
		return toIdx
	}

	// Drop uses that already have a matching result inside the
	// window — both halves of those pairs will be summarized
	// together; the model never sees the split.
	for i := 0; i < toIdx; i++ {
		for _, b := range messages[i].Blocks {
			if tr, ok := b.(llm.ToolResultBlock); ok {
				delete(inWindow, tr.ToolUseID)
			}
		}
	}
	if len(inWindow) == 0 {
		return toIdx
	}

	// Pass 1: advance toIdx past every result whose use is in the window.
	for i := toIdx; i < len(messages); i++ {
		matched := false
		for _, b := range messages[i].Blocks {
			tr, ok := b.(llm.ToolResultBlock)
			if !ok {
				continue
			}
			if inWindow[tr.ToolUseID] {
				matched = true
				delete(inWindow, tr.ToolUseID)
			}
		}
		if matched && i >= toIdx {
			toIdx = i + 1
		}
		if len(inWindow) == 0 {
			break
		}
	}

	// Pass 2: any remaining IDs in inWindow are orphan uses. Pull
	// toIdx back to exclude the earliest message containing one.
	if len(inWindow) > 0 {
		earliest := toIdx
		for i := 0; i < toIdx; i++ {
			for _, b := range messages[i].Blocks {
				tu, ok := b.(llm.ToolUseBlock)
				if !ok {
					continue
				}
				if inWindow[tu.ID] {
					if i < earliest {
						earliest = i
					}
				}
			}
		}
		toIdx = earliest
	}

	if toIdx < 0 {
		return 0
	}
	if toIdx > len(messages) {
		return len(messages)
	}
	return toIdx
}

func collectToolUseIDs(messages []llm.Message) map[string]bool {
	out := map[string]bool{}
	for _, m := range messages {
		for _, b := range m.Blocks {
			if tu, ok := b.(llm.ToolUseBlock); ok && tu.ID != "" {
				out[tu.ID] = true
			}
		}
	}
	return out
}

// ShouldCompact decides whether the message list has grown enough
// to merit compaction. Returns ok=true with the proposed
// [fromIdx, toIdx) window of messages to summarize. The window is
// not yet boundary-safe — callers must run adjustBoundary (T-204)
// before deleting anything.
//
// Returns ok=false (and zero indices) when:
//   - estimated tokens are below AutoCompactInputTokens, or
//   - len(messages) <= preserve*2 (nothing meaningful to compact)
func ShouldCompact(messages []llm.Message, cfg CompactionConfig, charsPerToken float64) (ok bool, fromIdx, toIdx int) {
	preserve := cfg.PreserveRecentMessages
	if preserve <= 0 {
		preserve = 4
	}
	threshold := cfg.AutoCompactInputTokens
	if threshold <= 0 {
		return false, 0, 0
	}
	if len(messages) <= preserve*2 {
		return false, 0, 0
	}
	if EstimateInputTokens(messages, charsPerToken) < threshold {
		return false, 0, 0
	}
	return true, 0, len(messages) - preserve
}

// reconcileCompactThreshold returns the effective absolute token threshold for
// the deterministic compaction fallback: the STRICTER (smaller) of the
// configured absolute trigger and the ratio-derived semantic trigger
// (compactRatio × maxContextTokens). Folding the two into one min() keeps the
// absolute and ratio-of-max triggers from disagreeing on the firing point
// (T4.4) — the deterministic fallback never waits longer than the semantic
// path's compact tier, nor vice versa. With the defaults (800_000 absolute,
// 0.80 ratio, 1_000_000 window) both equal 800_000, so this is a no-op until
// one is overridden.
//
// A non-positive absolute is left as-is (deterministic intentionally disabled —
// never enabled from the ratio); a non-positive ratio mirrors
// ShouldSemanticCompact's 0.80 default; a non-positive maxContextTokens leaves
// the absolute unchanged (no ratio to derive).
func reconcileCompactThreshold(absolute int, compactRatio float64, maxContextTokens int) int {
	if absolute <= 0 {
		return absolute
	}
	if compactRatio <= 0 {
		compactRatio = 0.80
	}
	if maxContextTokens <= 0 {
		return absolute
	}
	if ratioThreshold := int(compactRatio * float64(maxContextTokens)); ratioThreshold > 0 && ratioThreshold < absolute {
		return ratioThreshold
	}
	return absolute
}

// DefaultCompactionConfig returns the default config. The
// AutoCompactInputTokens value can be overridden at process start
// via DEEPSEEKCODE_AUTO_COMPACT_INPUT_TOKENS — malformed values
// fall back to the default rather than crash.
func DefaultCompactionConfig() CompactionConfig {
	cfg := CompactionConfig{
		PreserveRecentMessages: 4,
		MaxEstimatedTokens:     10_000,
		AutoCompactInputTokens: 800_000,
	}
	if v := os.Getenv("DEEPSEEKCODE_AUTO_COMPACT_INPUT_TOKENS"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			cfg.AutoCompactInputTokens = parsed
		}
	}
	return cfg
}

// MaxContextTokens is the default maximum context window size for
// DeepSeek V4 models (1M context). Used by ContextPressure to compute
// the usage ratio. Override via Agent.MaxContextTokens.
const MaxContextTokens = 1_000_000

// CompactWithSemantic checks context pressure and decides between
// no compaction, a warning, semantic compaction (LLM), or
// deterministic fallback. It returns a SemanticCompactionResult with
// Summary == "" when no compaction was performed.
//
// The action decision:
//   - "none": below all thresholds → no compaction
//   - "warn": above warn threshold → warning only
//   - "compact": above compact threshold → semantic compaction
//   - "protect": above protection threshold → semantic compaction (same as compact for now)
//
// When semantic compaction fails, it falls back to the deterministic
// CompactSession. The caller should check UsedSemantic and
// FallbackReason to report the outcome.
func CompactWithSemantic(
	ctx context.Context,
	messages []llm.Message,
	client *llm.Client,
	systemPrompt string,
	tools []llm.Tool,
	compCfg CompactionConfig,
	semanticCfg SemanticCompactionConfig,
	maxContextTokens int,
) SemanticCompactionResult {
	if semanticCfg.WarnThreshold <= 0 {
		semanticCfg = defaultSemanticCompactionConfig()
	}

	// CompactWithSemantic is not on the production path (maybeCompact calls
	// SemanticCompact directly); use the cold-start prior here.
	pressure := ContextPressure(messages, maxContextTokens, defaultCharsPerToken)
	action := ShouldSemanticCompact(pressure, semanticCfg)

	switch action {
	case "none", "warn":
		return SemanticCompactionResult{}
	case "compact", "protect":
		return SemanticCompact(ctx, messages, client, systemPrompt, tools, semanticCfg)
	default:
		return SemanticCompactionResult{}
	}
}
