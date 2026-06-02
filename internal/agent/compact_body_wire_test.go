// compact_body_wire_test.go locks in the Task-3 wiring: maybeCompact fires the
// cost-driven body compaction tier when the carried body exceeds BodyTokenBudget
// and the anti-thrash throttle (MinTurnsBetweenBodyCompactions) has elapsed. It
// asserts both halves of the contract:
//
//  1. Behavior — the body collapses to [summary, ...last PreserveRecentMessages],
//     i.e. exactly the same deterministic shape the overflow path produces.
//  2. Cache safety — the frozen Static Prefix fingerprint is byte-identical
//     before and after, mirroring TestCompactionPreservesFrozenPrefix. Body
//     compaction touches only a.Messages, never the frozen epoch; this test
//     fails loudly if a future change ever lets the live prompt drift.
package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/amemiya02/deepseekcode/internal/llm"
	"github.com/amemiya02/deepseekcode/internal/permissions"
	"github.com/amemiya02/deepseekcode/internal/tools"
)

func TestBodyCompactWiringCollapsesAndPreservesFrozenPrefix(t *testing.T) {
	a := New(nil, tools.New(), permissions.New(permissions.ModeYolo, "", nil, nil, nil), "deepseek-v4-flash")
	a.System = "static system prompt"
	a.DisableSemanticCompaction = true
	// Cost-driven body tier with a tiny budget so a modest body trips it, and a
	// throttle of 3 (the measured eviction cadence). Overflow stays at its high
	// default so ONLY the body tier can fire here.
	a.CompactionCfg = CompactionConfig{
		PreserveRecentMessages:         4,
		AutoCompactInputTokens:         800_000,
		BodyTokenBudget:                1_000,
		MinTurnsBetweenBodyCompactions: 3,
	}

	// Freeze the epoch so there is an authoritative frozen prefix to guard.
	epoch := a.epochMgr.InitEpoch("session_start", a.buildEpochComponents())
	a.epochMgr.FreezeEpoch()
	if !a.epochMgr.IsFrozen() {
		t.Fatal("precondition: epoch must be frozen")
	}
	frozenHashBefore := epoch.StaticPrefixHash
	if frozenHashBefore == "" {
		t.Fatal("precondition: frozen epoch must carry a static prefix hash")
	}

	// Body well over the 1,000-token budget at charsPerToken=4: 20 messages ×
	// 2,000 chars ≈ 10,000 tokens.
	for i := 0; i < 20; i++ {
		role := "assistant"
		if i%2 == 1 {
			role = "user"
		}
		a.Messages = append(a.Messages, llm.Message{
			Role:   role,
			Blocks: []llm.ContentBlock{llm.TextBlock{Text: strings.Repeat("x", 2000)}},
		})
	}
	msgCountBefore := len(a.Messages)

	// Throttle satisfied: curTurn(5) - lastBodyCompactTurn(0) = 5 >= 3.
	const curTurn = 5
	a.maybeCompact(context.Background(), curTurn)

	// 1) Behavior: collapsed to [summary, ...last PreserveRecentMessages].
	want := 1 + a.CompactionCfg.PreserveRecentMessages
	if len(a.Messages) != want {
		t.Fatalf("body compaction shape: got %d messages, want %d ([summary, ...last %d]); before=%d",
			len(a.Messages), want, a.CompactionCfg.PreserveRecentMessages, msgCountBefore)
	}

	// The throttle clock must have been advanced to the firing turn so the next
	// few turns can't re-fire and thrash the cache.
	if a.lastBodyCompactTurn != curTurn {
		t.Errorf("lastBodyCompactTurn=%d, want %d", a.lastBodyCompactTurn, curTurn)
	}

	// 2) Cache safety: frozen Static Prefix fingerprint byte-identical after.
	frozenHashAfter := a.epochMgr.CurrentEpoch().StaticPrefixHash
	if frozenHashAfter != frozenHashBefore {
		t.Errorf("body compaction moved the frozen prefix hash: before=%s after=%s",
			frozenHashBefore, frozenHashAfter)
	}
}

// TestBodyCompactWiringRespectsThrottle proves the anti-thrash guard is wired:
// an over-budget body must NOT collapse when too few turns have elapsed since
// the last body compaction, so a rewrite can never out-pace the eviction it
// replaces.
func TestBodyCompactWiringRespectsThrottle(t *testing.T) {
	a := New(nil, tools.New(), permissions.New(permissions.ModeYolo, "", nil, nil, nil), "deepseek-v4-flash")
	a.System = "static system prompt"
	a.DisableSemanticCompaction = true
	a.CompactionCfg = CompactionConfig{
		PreserveRecentMessages:         4,
		AutoCompactInputTokens:         800_000,
		BodyTokenBudget:                1_000,
		MinTurnsBetweenBodyCompactions: 3,
	}

	for i := 0; i < 20; i++ {
		a.Messages = append(a.Messages, llm.Message{
			Role:   "assistant",
			Blocks: []llm.ContentBlock{llm.TextBlock{Text: strings.Repeat("x", 2000)}},
		})
	}
	before := len(a.Messages)

	// Gap of 1 < MinTurnsBetweenBodyCompactions(3): must NOT fire.
	a.maybeCompact(context.Background(), 1)

	if len(a.Messages) != before {
		t.Fatalf("throttle violated: body compacted at gap=1 (before=%d after=%d)", before, len(a.Messages))
	}
	if a.lastBodyCompactTurn != 0 {
		t.Errorf("lastBodyCompactTurn moved despite throttle: got %d, want 0", a.lastBodyCompactTurn)
	}
}
