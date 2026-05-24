// compact.go owns session compaction: when the live message list
// grows past CompactionConfig.AutoCompactInputTokens the agent
// collapses the older portion into a synthetic summary message,
// preserving a configurable tail of recent turns. Compaction must
// not split a tool_use from its matching tool_result — see
// adjustBoundary in T-204.
package agent

import (
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
		AutoCompactInputTokens: 100_000,
	}
	if v := os.Getenv("DEEPSEEKCODE_AUTO_COMPACT_INPUT_TOKENS"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			cfg.AutoCompactInputTokens = parsed
		}
	}
	return cfg
}
