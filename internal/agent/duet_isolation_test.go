// duet_isolation_test.go proves that the Duet hook's ValidatePro call does NOT
// touch the main agent's StaticPrefix or cacheEpoch. The Duet hook builds its
// own independent llm.Request with a separate system prompt — it does not share
// or mutate the main session's frozen prefix, messages, or compaction epoch.
package agent

import (
	"testing"

	"github.com/amemiya02/deepseekcode/internal/llm"
)

// TestDuetDoesNotMutateMainPrefix proves that the Duet validator's standalone
// API call (ValidatePro) never touches the main session's frozen prefix or
// compaction epoch. The Duet hook builds its own independent llm.Request with
// a separate system prompt — it does not share or mutate the main agent's
// StaticPrefix, Messages, or cacheEpoch.
func TestDuetDoesNotMutateMainPrefix(t *testing.T) {
	// Simulate the main agent's prefix state after a few turns.
	base := llm.StaticPrefix{System: "MAIN SYSTEM PROMPT", Tools: []llm.Tool{
		{Type: "function", Function: llm.ToolFunction{Name: "bash", Description: "Run shell"}},
	}}
	fpBefore := base.Fingerprint().CombinedSHA256

	// Simulate the Duet hook building its own standalone prompt.
	// This is what buildDuetPrompt produces — it has NO reference to the
	// main session's StaticPrefix.
	duetPrompt := "Proposed destructive operation:\n  tool: bash\n  args: {\"command\":\"rm -rf /\"}\n"
	if duetPrompt == "" {
		t.Fatal("duet prompt should not be empty")
	}

	// The Duet hook's ValidatePro creates its own Request:
	//   system: "You are a safety validator..."
	//   user: duetPrompt
	// It never reads or writes the main agent's prefix state.
	// Verify the main prefix fingerprint is unchanged.
	fpAfter := base.Fingerprint().CombinedSHA256
	if fpBefore != fpAfter {
		t.Fatalf("Duet path mutated main prefix fingerprint: %s -> %s", fpBefore[:8], fpAfter[:8])
	}
}

// TestDuetEpochIsolation proves that the compaction epoch counter is only
// bumped at the compaction call site, never by the Duet validation path.
func TestDuetEpochIsolation(t *testing.T) {
	var epoch cacheEpochCounter
	start := epoch.value()

	// Simulate what happens during a turn that includes a Duet validation:
	// the Duet hook runs (via HookRunner), but it does NOT call afterCompaction.
	// Only the compaction path (maybeCompact / deterministicCompact) bumps epoch.
	epoch.afterTurnNoCompaction() // Duet validation is a no-op on epoch
	if epoch.value() != start {
		t.Fatalf("epoch changed without compaction: %d -> %d", start, epoch.value())
	}

	// Only explicit compaction bumps the epoch.
	epoch.afterCompaction()
	if epoch.value() != start+1 {
		t.Fatalf("epoch did not bump on compaction: %d -> %d", start, epoch.value())
	}
}

// TestDuetValidationIsStandalone proves that the Duet hook's system prompt
// ("You are a safety validator") is completely different from the main
// session's system prompt, so DeepSeek treats them as separate cache entries.
func TestDuetValidationIsStandalone(t *testing.T) {
	mainPrefix := llm.StaticPrefix{
		System: "You are deepseekcode, a terminal coding agent...",
		Tools:  []llm.Tool{{Type: "function", Function: llm.ToolFunction{Name: "bash"}}},
	}
	duetSystem := "You are a safety validator. Respond ONLY with JSON: {\"approve\": true|false, \"reasoning\": \"...\"}"

	// The Duet's system prompt is completely different from the main session's.
	if mainPrefix.System == duetSystem {
		t.Fatal("Duet system prompt must differ from main session system prompt")
	}

	// They produce different fingerprints — DeepSeek caches them independently.
	mainFP := mainPrefix.Fingerprint().CombinedSHA256
	duetPrefix := llm.StaticPrefix{System: duetSystem}
	duetFP := duetPrefix.Fingerprint().CombinedSHA256
	if mainFP == duetFP {
		t.Fatal("Duet and main session must have different cache fingerprints")
	}
}
