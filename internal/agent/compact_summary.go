// compact_summary.go owns the deterministic local summarizer that
// CompactSession folds removed messages through. No LLM call — the
// summary is rules-based so compaction is fast, free, and
// reproducible across restarts of the same input.
//
// T-205 ships only the minimal entry-point that CompactSession needs
// to compile; T-206 lands the real implementation (counts, tools
// used, recent requests, pending work, key files, current work,
// timeline).
package agent

import (
	"fmt"

	"github.com/amemiya02/deepseekcode/internal/llm"
)

// summarizeMessages produces a deterministic <summary> XML-ish block
// from the messages being compacted. Empty input returns "" so the
// caller can treat "no work to summarize" as a no-op compaction.
//
// T-205 placeholder: returns a one-line count. T-206 replaces with
// the full structured summary.
func summarizeMessages(removed []llm.Message) string {
	if len(removed) == 0 {
		return ""
	}
	return fmt.Sprintf("<summary>\n- messages: %d\n</summary>", len(removed))
}
