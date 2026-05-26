package agent

import (
	"testing"

	"github.com/amemiya02/deepseekcode/internal/llm"
)

// constantEstimator returns a fixed token count per message.
func constantEstimator(tokens int) func(llm.Message) int {
	return func(llm.Message) int {
		return tokens
	}
}

func TestPlanContextFold_EmptyMessages(t *testing.T) {
	plan, ok := PlanContextFold(nil, 100, 30, constantEstimator(20))
	if ok {
		t.Errorf("expected no fold for nil messages, got %+v", plan)
	}

	plan, ok = PlanContextFold([]llm.Message{}, 100, 30, constantEstimator(20))
	if ok {
		t.Errorf("expected no fold for empty messages, got %+v", plan)
	}
}

func TestPlanContextFold_UnderBudget(t *testing.T) {
	// 5 messages * 10 tokens = 50, max = 100 → no fold
	msgs := makeMessages(5, "user", false)
	plan, ok := PlanContextFold(msgs, 100, 30, constantEstimator(10))
	if ok {
		t.Errorf("expected no fold for under-budget, got %+v", plan)
	}
}

func TestPlanContextFold_OverBudget(t *testing.T) {
	// 10 messages * 20 tokens = 200, max = 100 → fold needed
	msgs := makeMessages(10, "user", false)
	plan, ok := PlanContextFold(msgs, 100, 30, constantEstimator(20))
	if !ok {
		t.Fatal("expected fold for over-budget, got false")
	}

	// Should fold some older messages
	if plan.FromIdx != 0 {
		t.Errorf("FromIdx = %d, want 0", plan.FromIdx)
	}
	if plan.ToIdx <= 0 || plan.ToIdx >= len(msgs) {
		t.Errorf("ToIdx = %d, want between 1 and %d", plan.ToIdx, len(msgs)-1)
	}
}

func TestPlanContextFold_PreservesTail(t *testing.T) {
	// 10 messages * 20 tokens = 200, max = 100, tail = 60
	// tail = 60 / 20 = 3 messages preserved
	msgs := makeMessages(10, "user", false)
	plan, ok := PlanContextFold(msgs, 100, 60, constantEstimator(20))
	if !ok {
		t.Fatal("expected fold, got false")
	}

	// At least 3 messages should be preserved (tail)
	preserved := len(msgs) - plan.ToIdx
	if preserved < 3 {
		t.Errorf("preserved %d messages, want at least 3", preserved)
	}
}

func TestPlanContextFold_PreservesAssistantToolCall(t *testing.T) {
	// Create messages where the last one is an assistant with tool calls
	msgs := makeMessages(9, "user", false)
	msgs = append(msgs, llm.Message{
		Role: "assistant",
		Blocks: []llm.ContentBlock{
			llm.TextBlock{Text: "calling tool"},
			llm.ToolUseBlock{ID: "c1", Name: "bash", Input: []byte(`{}`)},
		},
	})

	// 10 messages * 20 tokens = 200, max = 100
	plan, ok := PlanContextFold(msgs, 100, 30, constantEstimator(20))
	if !ok {
		t.Fatal("expected fold, got false")
	}

	// The final message should NOT be in the fold range
	if plan.ToIdx >= len(msgs) {
		t.Errorf("ToIdx = %d includes final assistant tool-call message", plan.ToIdx)
	}

	// Verify the final message is preserved
	last := msgs[len(msgs)-1]
	if last.Role != "assistant" {
		t.Error("final message should be assistant")
	}
	if !hasToolCalls(last) {
		t.Error("final message should have tool calls")
	}
}

func TestPlanContextFold_TailTokensZero(t *testing.T) {
	// tailTokens <= 0 should preserve at least the final message
	msgs := makeMessages(10, "user", false)
	plan, ok := PlanContextFold(msgs, 100, 0, constantEstimator(20))
	if !ok {
		t.Fatal("expected fold, got false")
	}

	// At least 1 message should be preserved
	if plan.ToIdx >= len(msgs) {
		t.Errorf("ToIdx = %d should preserve at least 1 message", plan.ToIdx)
	}
}

func TestPlanContextFold_NoSafeRange(t *testing.T) {
	// All messages are assistant tool-calls → no safe fold range
	msgs := []llm.Message{
		{Role: "assistant", Blocks: []llm.ContentBlock{
			llm.ToolUseBlock{ID: "c1", Name: "bash"},
		}},
		{Role: "assistant", Blocks: []llm.ContentBlock{
			llm.ToolUseBlock{ID: "c2", Name: "bash"},
		}},
	}

	plan, ok := PlanContextFold(msgs, 10, 0, constantEstimator(100))
	if ok {
		t.Errorf("expected no fold when all messages are tool-calls, got %+v", plan)
	}
}

func TestPlanContextFold_TailTokensExceedsTotal(t *testing.T) {
	// 3 messages * 10 tokens = 30 total, max = 20, tail = 100
	// tailTokens (100) exceeds total (30) → no safe fold range
	msgs := makeMessages(3, "user", false)
	plan, ok := PlanContextFold(msgs, 20, 100, constantEstimator(10))
	if ok {
		t.Fatalf("expected no safe fold when tail budget exceeds total, got %+v", plan)
	}
}

func TestPlanContextFold_ExampleFromCard(t *testing.T) {
	// Example from task card: 10 messages, max=100, tail=30, each=20 tokens
	// Total = 200, need to fold some older messages, preserve tail ~30 tokens
	msgs := makeMessages(10, "user", false)
	plan, ok := PlanContextFold(msgs, 100, 30, constantEstimator(20))
	if !ok {
		t.Fatal("expected fold, got false")
	}

	if plan.FromIdx != 0 {
		t.Errorf("FromIdx = %d, want 0", plan.FromIdx)
	}

	// Should preserve at least 2 messages (30 tokens / 20 = 1.5, round up to 2)
	preserved := len(msgs) - plan.ToIdx
	if preserved < 2 {
		t.Errorf("preserved %d messages, want at least 2", preserved)
	}

	// Fold range should be valid
	if plan.ToIdx <= plan.FromIdx {
		t.Errorf("invalid range [%d, %d)", plan.FromIdx, plan.ToIdx)
	}
}

// makeMessages creates n messages with the given role. If toolCall is true,
// the last message will have a tool_use block.
func makeMessages(n int, role string, toolCall bool) []llm.Message {
	msgs := make([]llm.Message, n)
	for i := range msgs {
		msgs[i] = llm.Message{
			Role:   role,
			Blocks: []llm.ContentBlock{llm.TextBlock{Text: "hello"}},
		}
	}
	if toolCall && n > 0 {
		msgs[n-1] = llm.Message{
			Role: "assistant",
			Blocks: []llm.ContentBlock{
				llm.TextBlock{Text: "calling tool"},
				llm.ToolUseBlock{ID: "c1", Name: "bash", Input: []byte(`{}`)},
			},
		}
	}
	return msgs
}
