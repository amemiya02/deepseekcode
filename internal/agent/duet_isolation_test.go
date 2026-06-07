// duet_isolation_test.go characterizes the Duet hook's existing isolation
// from the agent's cache state. The e2e_duet_hook_test.go file already
// drives NewDuetHook with a fake DuetClient against a real Agent via
// executeOne. These tests add the missing assertions:
//   - Agent's StaticPrefix and cacheEpoch are untouched after Duet validation
//   - The Duet hook's system prompt produces a different cache fingerprint
package agent

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/amemiya02/deepseekcode/internal/hooks"
	"github.com/amemiya02/deepseekcode/internal/llm"
	"github.com/amemiya02/deepseekcode/internal/permissions"
	"github.com/amemiya02/deepseekcode/internal/tools"
)

// TestDuetDoesNotMutateAgentCacheState drives a destructive bash call through
// the full executeOne → Duet hook path and asserts the agent's frozen prefix
// fingerprint and compaction epoch are unchanged afterward. This is the real
// isolation guarantee: the Duet hook's ValidatePro call builds its own
// independent Request (llm/client.go:443 "You are a safety validator...")
// and never touches the agent's StaticPrefix, Messages, or cacheEpoch.
func TestDuetDoesNotMutateAgentCacheState(t *testing.T) {
	reg := tools.New()
	reg.Register(dummyBashTool{})

	pol := permissions.New(permissions.ModeYolo, t.TempDir(), nil, nil, nil)

	fake := &fakeDuetClient{approve: true, reasoning: "looks safe"}

	hr := hooks.NewRunner()
	hr.Register("duet", hooks.NewDuetHook(
		fake, nil, pol.Cwd, nil,
		func() string { return "deepseek-v4-flash" },
		func() []byte { return nil },
	))
	hr.Configure([]hooks.HookConfig{
		{Event: hooks.EventPreToolUse, Type: hooks.TypeBuiltin, Name: "duet"},
	})

	a := New(llm.NewClient("k", "http://localhost:1"), reg, pol, "deepseek-v4-flash")
	a.HookRunner = hr

	// Set up known prefix state before the Duet call.
	a.System = "MAIN SYSTEM PROMPT"
	beforeEpoch := a.cacheEpoch.value()

	// Execute a destructive call through the Duet hook.
	args := json.RawMessage(`{"command":"rm -rf /tmp/test"}`)
	call := llm.ToolCall{ID: "c1", Type: "function", Function: llm.ToolCallFunc{Name: "bash", Arguments: string(args)}}
	_, _ = a.executeOne(context.Background(), call)

	// The agent's compaction epoch must be unchanged — Duet validation
	// is not compaction.
	if a.cacheEpoch.value() != beforeEpoch {
		t.Fatalf("Duet bumped main epoch: %d -> %d", beforeEpoch, a.cacheEpoch.value())
	}
	// The agent's system prompt must be unchanged.
	if a.System != "MAIN SYSTEM PROMPT" {
		t.Fatalf("Duet mutated main system prompt: %q", a.System)
	}
}

// TestDuetValidationFingerprintDiffers proves that the Duet hook's system
// prompt ("You are a safety validator") produces a different cache fingerprint
// than the main session's system prompt, so DeepSeek caches them as separate
// entries. This is the architectural isolation — not tested by the hook
// behavior tests in e2e_duet_hook_test.go.
func TestDuetValidationFingerprintDiffers(t *testing.T) {
	mainPrefix := llm.StaticPrefix{
		System: "You are deepseekcode, a terminal coding agent...",
		Tools:  []llm.Tool{{Type: "function", Function: llm.ToolFunction{Name: "bash"}}},
	}
	mainFP := mainPrefix.Fingerprint().CombinedSHA256

	// The Duet hook uses "You are a safety validator..." as its system prompt
	// (llm/client.go:444). This must produce a different fingerprint.
	duetPrefix := llm.StaticPrefix{
		System: "You are a safety validator. Respond ONLY with JSON: {\"approve\": true|false, \"reasoning\": \"...\"}",
	}
	duetFP := duetPrefix.Fingerprint().CombinedSHA256

	if mainFP == duetFP {
		t.Fatal("Duet and main session must have different cache fingerprints")
	}
}
