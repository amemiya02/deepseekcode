package main

import (
	"strings"
	"testing"

	"github.com/amemiya02/deepseekcode/internal/cache"
)

func TestHeadToHeadReport_FourCauseBreakdown(t *testing.T) {
	receipts := []cache.CacheReceipt{
		{Turn: 0, Dominant: cache.CauseColdFirst, MissTokens: 1000, CostCNY: 0.001},
		{Turn: 1, Dominant: cache.CauseSteady, HitTokens: 1000, MissTokens: 40, ResidualEst: 40, CostCNY: 0.0001},
		{Turn: 2, Dominant: cache.CauseCompactReset, MissTokens: 5000, CostCNY: 0.005},
	}
	rep := headToHeadReport("dsc-Agroup", receipts)
	for _, want := range []string{"cold_first", "steady", "compact_reset", "TOTAL"} {
		if !strings.Contains(rep, want) {
			t.Fatalf("report missing %q:\n%s", want, rep)
		}
	}
	// Verify the header label.
	if !strings.Contains(rep, "dsc-Agroup") {
		t.Fatalf("report missing label:\n%s", rep)
	}
}

func TestHeadToHeadReport_Empty(t *testing.T) {
	rep := headToHeadReport("empty", nil)
	if !strings.Contains(rep, "TOTAL") {
		t.Fatalf("empty report should still have TOTAL line:\n%s", rep)
	}
	if !strings.Contains(rep, "empty") {
		t.Fatalf("empty report should have label:\n%s", rep)
	}
}

func TestHeadToHeadReport_AllCauses(t *testing.T) {
	receipts := []cache.CacheReceipt{
		{Turn: 0, Dominant: cache.CauseColdFirst, MissTokens: 500, CostCNY: 0.001},
		{Turn: 1, Dominant: cache.CausePrefixMut, MissTokens: 300, CostCNY: 0.0005},
		{Turn: 2, Dominant: cache.CauseResidualTail, MissTokens: 44, CostCNY: 0.0001},
		{Turn: 3, Dominant: cache.CauseCompactReset, MissTokens: 2000, CostCNY: 0.003},
		{Turn: 4, Dominant: cache.CauseSteady, HitTokens: 8000, MissTokens: 100, CostCNY: 0.0002, SavedCNY: 0.05},
	}
	rep := headToHeadReport("all-five", receipts)
	for _, want := range []string{"cold_first", "prefix_mut", "residual_tail", "compact_reset", "steady", "TOTAL"} {
		if !strings.Contains(rep, want) {
			t.Fatalf("report missing %q:\n%s", want, rep)
		}
	}
}
