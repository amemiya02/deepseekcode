package agent

import (
	"strings"
	"testing"

	"github.com/amemiya02/deepseekcode/internal/llm"
)

// textBlock constructs an llm.ContentBlock text block, matching the
// llm.TextBlock construction style used by the existing compaction tests
// (see compact_test.go). It exists so bodyMsgs can stay terse.
func textBlock(s string) llm.ContentBlock {
	return llm.TextBlock{Text: s}
}

// bodyMsgs builds n messages each ~perMsgChars long, alternating roles.
func bodyMsgs(n, perMsgChars int) []llm.Message {
	out := make([]llm.Message, n)
	for i := range out {
		role := "tool"
		if i%2 == 0 {
			role = "assistant"
		}
		out[i] = llm.Message{Role: role, Blocks: []llm.ContentBlock{textBlock(strings.Repeat("x", perMsgChars))}}
	}
	return out
}

func TestShouldCompactBody_FiresAboveBudget(t *testing.T) {
	cfg := CompactionConfig{PreserveRecentMessages: 4, BodyTokenBudget: 1000, MinTurnsBetweenBodyCompactions: 3}
	msgs := bodyMsgs(20, 2000) // well over budget at charsPerToken=4
	ok, _, toIdx := ShouldCompactBody(msgs, cfg, 4.0, 5, false)
	if !ok {
		t.Fatal("expected body compaction to fire above budget")
	}
	if toIdx != len(msgs)-4 {
		t.Errorf("toIdx=%d want %d (preserve last 4)", toIdx, len(msgs)-4)
	}
}

func TestShouldCompactBody_RespectsThrottle(t *testing.T) {
	cfg := CompactionConfig{PreserveRecentMessages: 4, BodyTokenBudget: 1000, MinTurnsBetweenBodyCompactions: 3}
	msgs := bodyMsgs(20, 2000)
	if ok, _, _ := ShouldCompactBody(msgs, cfg, 4.0, 1, false); ok {
		t.Error("must not fire when turnsSinceLast < MinTurnsBetweenBodyCompactions")
	}
}

func TestShouldCompactBody_BelowBudgetNoop(t *testing.T) {
	cfg := CompactionConfig{PreserveRecentMessages: 4, BodyTokenBudget: 1_000_000, MinTurnsBetweenBodyCompactions: 3}
	msgs := bodyMsgs(20, 100)
	if ok, _, _ := ShouldCompactBody(msgs, cfg, 4.0, 5, false); ok {
		t.Error("must not fire below budget")
	}
}
