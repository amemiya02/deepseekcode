// compact_prefix_test.go guards that CompactSession never mutates the
// system-prompt head of the message list — the cache-critical prefix.
package agent

import (
	"testing"

	"github.com/amemiya02/deepseekcode/internal/llm"
)

// textOf is defined in compact.go (production) — reused here.

func TestCompactionNeverTouchesSystemPrefix(t *testing.T) {
	msgs := []llm.Message{
		{Role: "system", Blocks: []llm.ContentBlock{llm.TextBlock{Text: "SYSTEM-PREFIX"}}},
	}
	for i := 0; i < 20; i++ {
		msgs = append(msgs, llm.Message{Role: "user", Blocks: []llm.ContentBlock{llm.TextBlock{Text: "u"}}})
		msgs = append(msgs, llm.Message{Role: "assistant", Blocks: []llm.ContentBlock{llm.TextBlock{Text: "a"}}})
	}
	cfg := CompactionConfig{PreserveRecentMessages: 4, MaxEstimatedTokens: 1000, AutoCompactInputTokens: 1}
	res := CompactSession(msgs, cfg, 4.0)
	if res.Summary == "" {
		t.Skip("no compaction performed for this input")
	}
	// The system head is never in the compaction window; its bytes must survive.
	if msgs[0].Role != "system" || textOf(msgs[0]) != "SYSTEM-PREFIX" {
		t.Fatalf("system prefix mutated by compaction: %+v", msgs[0])
	}
}
