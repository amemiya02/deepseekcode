// compact_frozen_prefix_test.go locks in the architectural guarantee that
// compaction NEVER alters the frozen-prefix bytes. The frozen prefix is the
// cache-critical Static Prefix the model has been receiving since the epoch
// froze; compaction only collapses the body tail into a summary message, so
// the prefix fingerprint must be byte-identical before and after. This is true
// today by construction (compaction touches only a.Messages, never the epoch);
// the test exists to fail loudly if a future change ever lets the live prompt
// drift away from the frozen baseline.
package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/amemiya02/deepseekcode/internal/llm"
	"github.com/amemiya02/deepseekcode/internal/permissions"
	"github.com/amemiya02/deepseekcode/internal/tools"
)

func TestCompactionPreservesFrozenPrefix(t *testing.T) {
	a := New(nil, tools.New(), permissions.New(permissions.ModeYolo, "", nil, nil, nil), "deepseek-v4-flash")
	a.System = "static system prompt"
	a.DisableSemanticCompaction = true
	a.CompactionCfg = CompactionConfig{PreserveRecentMessages: 2}

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

	// Enough body messages that ForceCompact (threshold lowered to 1) fires.
	a.Messages = append(a.Messages, llm.Message{
		Role:   "user",
		Blocks: []llm.ContentBlock{llm.TextBlock{Text: "start"}},
	})
	for i := 0; i < 10; i++ {
		a.Messages = append(a.Messages,
			llm.Message{Role: "assistant", Blocks: []llm.ContentBlock{llm.TextBlock{Text: strings.Repeat("x", 200)}}},
			llm.Message{Role: "user", Blocks: []llm.ContentBlock{llm.TextBlock{Text: strings.Repeat("y", 200)}}},
		)
	}
	msgCountBefore := len(a.Messages)

	a.ForceCompact(context.Background())

	// Sanity: compaction actually happened (otherwise the guard is vacuous).
	if len(a.Messages) >= msgCountBefore {
		t.Fatalf("compaction did not fire: messages before=%d after=%d", msgCountBefore, len(a.Messages))
	}

	// The frozen epoch's prefix hash must be byte-identical after compaction.
	frozenHashAfter := a.epochMgr.CurrentEpoch().StaticPrefixHash
	if frozenHashAfter != frozenHashBefore {
		t.Errorf("compaction moved the frozen prefix hash: before=%s after=%s",
			frozenHashBefore, frozenHashAfter)
	}
}

// TestAlignTailToCacheUnit covers the §3.6 boundary-alignment helper:
//   - unit<=0 is a strict no-op (byte-identical summary message), so
//     CompactSession behaves exactly as before when CacheUnit is unset.
//   - unit>0 lands the rebuilt tail on a cache-unit multiple by appending a
//     padding block, WITHOUT mutating the summary's first block (so
//     priorSummaryText detection survives multi-round compaction).
func TestAlignTailToCacheUnit(t *testing.T) {
	summary := llm.Message{
		Role:   "assistant",
		Blocks: []llm.ContentBlock{llm.TextBlock{Text: "<summary>\n- objective: x\n</summary>"}},
	}
	kept := []llm.Message{
		{Role: "user", Blocks: []llm.ContentBlock{llm.TextBlock{Text: strings.Repeat("k", 137)}}},
	}

	// unit<=0 → unchanged (same block count, same first-block text).
	if got := alignTailToCacheUnit(summary, kept, 0); len(got.Blocks) != 1 {
		t.Fatalf("unit=0 must not add blocks: got %d blocks", len(got.Blocks))
	}

	const unit = 8
	got := alignTailToCacheUnit(summary, kept, unit)

	// First block is untouched (summary detection relies on it).
	if textOf(got) != textOf(summary) {
		t.Fatalf("first summary block was mutated: %q != %q", textOf(got), textOf(summary))
	}

	// Rebuilt tail must land on a unit boundary under the same estimator.
	tail := append([]llm.Message{got}, kept...)
	total := EstimateTokensCalibrated(tail, defaultCharsPerToken)
	if total%unit != 0 {
		t.Fatalf("aligned tail token count %d is not a multiple of %d", total, unit)
	}
}
