package agent_test

import (
	"context"
	"strings"
	"testing"

	"github.com/amemiya02/deepseekcode/internal/agent"
	"github.com/amemiya02/deepseekcode/internal/llm"
	"github.com/amemiya02/deepseekcode/internal/permissions"
	"github.com/amemiya02/deepseekcode/internal/tools"
)

// newMinimalAgent builds the smallest Agent that can be constructed without
// a real LLM connection. The client is nil (no API calls will be made because
// ForceCompact / maybeCompact only calls the LLM for semantic compaction, which
// is disabled here).
func newMinimalAgent(t *testing.T) *agent.Agent {
	t.Helper()
	reg := tools.New()
	pol := permissions.New(permissions.ModeYolo, t.TempDir(), nil, nil, nil)
	a := agent.New(nil, reg, pol, "deepseek-v4-flash")
	a.DisableSemanticCompaction = true
	return a
}

// TestCompact_FlagTriggersCompact verifies that RunCompact on an agent with a
// long-enough message list actually compacts and prints the success message.
func TestCompact_FlagTriggersCompact(t *testing.T) {
	a := newMinimalAgent(t)

	// Force compaction threshold to 1 token so any non-empty message list fires.
	a.CompactionCfg.AutoCompactInputTokens = 1
	// Keep only the last 2 messages outside the window; we need at least
	// preserve*2+1 messages for ShouldCompact to fire.
	a.CompactionCfg.PreserveRecentMessages = 2
	// Disable the token-based tail floor — this test's fixture (10 short
	// messages) produces far fewer than 16k tokens, so the floor would
	// consume the entire window and suppress compaction.
	a.CompactionCfg.TailTokensFloor = 0

	// Populate the message list with enough messages to exceed the threshold.
	for i := 0; i < 10; i++ {
		a.Messages = append(a.Messages, llm.Message{
			Role:   "user",
			Blocks: []llm.ContentBlock{llm.TextBlock{Text: "hello world this is message content"}},
		})
	}

	var out strings.Builder
	err := agent.RunCompact(context.Background(), a, &out)
	if err != nil {
		t.Fatalf("RunCompact returned unexpected error: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "compacted successfully") {
		t.Errorf("expected 'compacted successfully' in output, got: %q", got)
	}
}

// TestCompact_NothingToCompact verifies that RunCompact on a short transcript
// prints the "nothing to compact" message and returns no error.
func TestCompact_NothingToCompact(t *testing.T) {
	a := newMinimalAgent(t)

	// With only 2 messages the preserve*2 guard in ShouldCompact fires (preserve=4,
	// len<=8), so no compaction should happen.
	a.Messages = append(a.Messages, llm.Message{
		Role:   "user",
		Blocks: []llm.ContentBlock{llm.TextBlock{Text: "hi"}},
	})

	var out strings.Builder
	err := agent.RunCompact(context.Background(), a, &out)
	if err != nil {
		t.Fatalf("RunCompact returned unexpected error: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "nothing to compact") {
		t.Errorf("expected 'nothing to compact' in output, got: %q", got)
	}
}

// TestAgent_Compact_ReturnsError verifies that Compact propagates errors.
// This uses a recover-wrapped path: we can't easily inject a panic from
// outside the package, so instead we verify the happy path returns nil error.
func TestAgent_Compact_NoError(t *testing.T) {
	a := newMinimalAgent(t)
	compacted, err := a.Compact(context.Background())
	if err != nil {
		t.Fatalf("Compact returned unexpected error: %v", err)
	}
	// With an empty message list, compaction should not have fired.
	if compacted {
		t.Errorf("Compact reported compacted=true on empty message list")
	}
}
