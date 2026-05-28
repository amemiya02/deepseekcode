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

	"github.com/amemiya02/deepseekcode/internal/llm"
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

// EstimateTokens returns a rough char/4 token count for the message
// list. Used only for compaction triggering — not for cost
// computation. Cheap and deterministic; no tokenizer dependency.
func EstimateTokens(messages []llm.Message) int {
	total := 0
	for _, m := range messages {
		total += perMessageOverhead
		for _, b := range m.Blocks {
			switch v := b.(type) {
			case llm.TextBlock:
				total += len(v.Text) / 4
			case llm.ThinkingBlock:
				total += len(v.Text) / 4
			case llm.ToolUseBlock:
				total += (len(v.Name) + len(v.Input)) / 4
			case llm.ToolResultBlock:
				total += len(v.Content) / 4
			}
		}
	}
	return total
}

// CompactSession runs the full pipeline: ShouldCompact →
// adjustBoundary → summarize. Returns a CompactionResult with
// Summary == "" when no compaction was performed (caller must
// check Summary before mutating its message list).
//
// CompactSession does NOT persist — the caller wires the result
// into Persister.ReplaceWithCompaction (T-209) and replaces its
// in-memory a.Messages slice.
func CompactSession(messages []llm.Message, cfg CompactionConfig) CompactionResult {
	ok, fromIdx, toIdx := ShouldCompact(messages, cfg)
	if !ok {
		return CompactionResult{}
	}
	toIdx = adjustBoundary(messages, toIdx)
	if toIdx <= fromIdx {
		return CompactionResult{}
	}
	summary := summarizeMessages(messages[fromIdx:toIdx])
	if summary == "" {
		return CompactionResult{}
	}
	summaryMsg := llm.Message{
		Role:   "system",
		Blocks: []llm.ContentBlock{llm.TextBlock{Text: summary}},
	}
	kept := make([]llm.Message, len(messages)-toIdx)
	copy(kept, messages[toIdx:])
	return CompactionResult{
		Summary:        summary,
		FromIdx:        fromIdx,
		ToIdx:          toIdx,
		RemovedCount:   toIdx - fromIdx,
		SummaryMessage: summaryMsg,
		KeptMessages:   kept,
	}
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
func ShouldCompact(messages []llm.Message, cfg CompactionConfig) (ok bool, fromIdx, toIdx int) {
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
	if EstimateTokens(messages) < threshold {
		return false, 0, 0
	}
	return true, 0, len(messages) - preserve
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

	pressure := ContextPressure(messages, maxContextTokens)
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
