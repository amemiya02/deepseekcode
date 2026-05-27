package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/amemiya02/deepseekcode/internal/llm"
	"github.com/amemiya02/deepseekcode/internal/permissions"
	"github.com/amemiya02/deepseekcode/internal/tools"
)

// TestDeterministicCompactionEmitsMeasuredRecord proves the deterministic
// fallback path publishes an EventSemanticCompaction carrying measured
// before/after static-prefix hashes (UsedSemantic=false), so a compaction-
// required gate has real evidence even when semantic compaction is disabled.
// Deterministic compaction never rebuilds the prefix, so before == after.
func TestDeterministicCompactionEmitsMeasuredRecord(t *testing.T) {
	reg := tools.New()
	a := New(nil, reg, permissions.New(permissions.ModeYolo, "", nil, nil, nil), "m")
	a.System = "sys"
	a.DisableSemanticCompaction = true
	a.CompactionCfg = CompactionConfig{PreserveRecentMessages: 2}

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

	sub := a.bus.Subscribe(256)
	defer a.bus.Unsubscribe(sub)

	a.ForceCompact(context.Background())

	var sc *EventSemanticCompaction
	for drained := false; !drained; {
		select {
		case env := <-sub.C:
			if e, ok := env.Event.(EventSemanticCompaction); ok {
				ev := e
				sc = &ev
			}
		default:
			drained = true
		}
	}

	if sc == nil {
		t.Fatal("deterministic compaction did not emit EventSemanticCompaction")
	}
	if sc.UsedSemantic {
		t.Error("deterministic path must report UsedSemantic=false")
	}
	if sc.StaticPrefixHashBefore == "" || sc.StaticPrefixHashAfter == "" {
		t.Fatalf("compaction record must carry before/after prefix hashes: %+v", sc)
	}
	if sc.StaticPrefixHashBefore != sc.StaticPrefixHashAfter {
		t.Errorf("deterministic compaction must not move the prefix: before=%s after=%s",
			sc.StaticPrefixHashBefore, sc.StaticPrefixHashAfter)
	}
}
