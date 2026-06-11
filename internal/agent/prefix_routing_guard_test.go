// prefix_routing_guard_test.go pins the invariant that routing escalation
// (flash -> pro re-issue on NEEDS_PRO marker) and Duet validator calls never
// perturb the static prefix fingerprint. The frozen prefix is the cache-critical
// baseline the model has been receiving since the epoch froze; escalation only
// changes the request's Model field, and Duet runs via a bypass that doesn't
// mutate the prefix bytes. This test exists to fail loudly if a future change
// ever lets either path mutate the frozen prefix bytes.
package agent

import (
	"context"
	"testing"

	"github.com/amemiya02/deepseekcode/internal/hooks"
	"github.com/amemiya02/deepseekcode/internal/llmtest"
	"github.com/amemiya02/deepseekcode/internal/permissions"
	"github.com/amemiya02/deepseekcode/internal/tools"
)

// TestPrefixRoutingEscalationPreservesHash creates an agent with escalation
// enabled, freezes the epoch, records the static prefix fingerprint, drives a
// turn that triggers a NEEDS_PRO escalation (flash marker -> pro re-issue), and
// asserts the fingerprint is byte-identical after the escalation completes.
func TestPrefixRoutingEscalationPreservesHash(t *testing.T) {
	// Step 1: Wire the agent to the mock server.
	// Turn 1 (flash): emits the NEEDS_PRO marker.
	// Turn 2 (pro re-issue): returns a real answer.
	srv := llmtest.NewServer(
		llmtest.Turn{Text: "<<<NEEDS_PRO: this needs deeper reasoning>>>"},
		llmtest.Turn{Text: "pro's considered answer"},
	)
	defer srv.Close()

	a := newMockLoopAgent(t, srv)

	// Step 2: Enable escalation (flash -> pro).
	a.EscalationModel = proModel

	// Step 3: Pre-initialize and freeze the epoch so the fingerprint is
	// established before the Run drives the turn. This mirrors the manual
	// InitEpoch+FreezeEpoch pattern in compact_frozen_prefix_test.go.
	epoch := a.epochMgr.InitEpoch("session_start", a.buildEpochComponents())
	a.epochMgr.FreezeEpoch()
	if !a.epochMgr.IsFrozen() {
		t.Fatal("precondition: epoch must be frozen")
	}
	frozenHashBefore := epoch.StaticPrefixHash
	if frozenHashBefore == "" {
		t.Fatal("precondition: frozen epoch must carry a static prefix hash")
	}

	// Step 4: Drive a turn that triggers escalation.
	// Flash emits the NEEDS_PRO marker; the loop re-issues on pro.
	reason, err := a.Run(context.Background(), "hard question")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if reason != StopModelDone {
		t.Fatalf("reason = %v, want StopModelDone", reason)
	}
	if srv.Count() != 2 {
		t.Fatalf("served %d requests, want 2 (flash marker + pro re-issue)", srv.Count())
	}

	// Step 5: Assert the frozen prefix hash is unchanged after escalation.
	// Escalation only changes req.Model; the static prefix (system prompt +
	// tool specs) that feeds the fingerprint must be byte-identical.
	frozenHashAfter := a.epochMgr.CurrentEpoch().StaticPrefixHash
	if frozenHashAfter != frozenHashBefore {
		t.Errorf("escalation moved the frozen prefix hash: before=%s after=%s",
			frozenHashBefore, frozenHashAfter)
	}
}

// TestDuetValidatorPreservesPrefixHash pins the other half of the W0.6
// invariant: a Duet validator call (triggered by a destructive bash tool)
// must not perturb the static prefix fingerprint. Duet runs via a pre-tool
// hook that calls ValidatePro on a separate client; it must not mutate
// a.Messages' prefix or the frozen system/tools that feed the fingerprint.
func TestDuetValidatorPreservesPrefixHash(t *testing.T) {
	// Mock server: flash emits a destructive bash call, then finishes.
	srv := llmtest.NewServer(
		llmtest.Turn{ToolCalls: []llmtest.ToolCall{
			{ID: "c1", Name: "bash", Args: `{"command":"rm -rf /tmp/test"}`},
		}},
		llmtest.Turn{Text: "done"},
	)
	defer srv.Close()

	reg := tools.New()
	pol := permissions.New(permissions.ModeYolo, t.TempDir(), nil, nil, nil)

	// fakeDuetClient approves the destructive call.
	fake := &fakeDuetClient{approve: true, reasoning: "looks safe"}

	hr := hooks.NewRunner()
	hr.Register("duet", hooks.NewDuetHook(
		fake,
		nil,                    // extraDestructive
		pol.Cwd,                // cwd
		pol.SecretPathPatterns, // secretPatterns
		func() string { return "deepseek-v4-flash" },
		func() []byte { return nil },
	))
	hr.Configure([]hooks.HookConfig{
		{Event: hooks.EventPreToolUse, Type: hooks.TypeBuiltin, Name: "duet"},
	})

	a := New(srv.Client(), reg, pol, "deepseek-v4-flash")
	a.System = "test system"
	a.HookRunner = hr

	// Register a dummy bash tool so the destructive call has something to match.
	a.Tools.Register(dummyBashTool{})

	// Pre-initialize and freeze the epoch.
	epoch := a.epochMgr.InitEpoch("session_start", a.buildEpochComponents())
	a.epochMgr.FreezeEpoch()
	if !a.epochMgr.IsFrozen() {
		t.Fatal("precondition: epoch must be frozen")
	}
	frozenHashBefore := epoch.StaticPrefixHash
	if frozenHashBefore == "" {
		t.Fatal("precondition: frozen epoch must carry a static prefix hash")
	}

	// Drive a turn that triggers Duet validation.
	reason, err := a.Run(context.Background(), "delete temp files")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if reason != StopModelDone {
		t.Fatalf("reason = %v, want StopModelDone", reason)
	}

	// Duet must have been called.
	if fake.calls != 1 {
		t.Errorf("expected 1 Duet ValidatePro call, got %d", fake.calls)
	}

	// The frozen prefix hash must be unchanged after Duet validation.
	frozenHashAfter := a.epochMgr.CurrentEpoch().StaticPrefixHash
	if frozenHashAfter != frozenHashBefore {
		t.Errorf("Duet validation moved the frozen prefix hash: before=%s after=%s",
			frozenHashBefore, frozenHashAfter)
	}
}

// Note: fakeDuetClient and dummyBashTool are declared in e2e_duet_hook_test.go
// and reused here — both are in the same test package.
