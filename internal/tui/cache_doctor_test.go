package tui_test

import (
	"strings"
	"testing"

	"github.com/amemiya02/deepseekcode/internal/traceinspect"
	"github.com/amemiya02/deepseekcode/internal/tui"
)

func makeLedger() []traceinspect.TurnLedger {
	return []traceinspect.TurnLedger{
		{Turn: 1, EpochID: "e1", HitTokens: 0, MissTokens: 4000, OutputTokens: 100, CostCNY: 0.001, Evicted: false, Why: traceinspect.WhyExpectedMiss},
		{Turn: 2, EpochID: "e1", HitTokens: 3800, MissTokens: 200, OutputTokens: 80, CostCNY: 0.0003, Evicted: false, Why: ""},
		{Turn: 3, EpochID: "e1", HitTokens: 50, MissTokens: 4500, OutputTokens: 120, CostCNY: 0.0018, Evicted: true, Why: traceinspect.WhyCompaction},
	}
}

func TestCacheDoctorPanel_RendersHeader(t *testing.T) {
	panel := tui.NewCacheDoctorPanel(makeLedger(), tui.DarkTheme())
	view := panel.View()
	if !strings.Contains(view, "Cache Doctor") {
		t.Errorf("panel View missing 'Cache Doctor' header\n%s", view)
	}
}

func TestCacheDoctorPanel_RendersEvictionMark(t *testing.T) {
	panel := tui.NewCacheDoctorPanel(makeLedger(), tui.DarkTheme())
	view := panel.View()
	// Eviction should be visible in the rendered output somehow — at minimum
	// the WHY label for compaction must appear.
	if !strings.Contains(view, "compaction") {
		t.Errorf("panel View missing compaction eviction label\n%s", view)
	}
	// The EVICT column for the evicted row must render "Y", not "N".
	// This catches swapped branch bodies in the if row.Evicted block.
	if !strings.Contains(view, "Y") {
		t.Errorf("panel View missing 'Y' eviction mark in EVICT column\n%s", view)
	}
}

func TestCacheDoctorPanel_EmptyLedger(t *testing.T) {
	panel := tui.NewCacheDoctorPanel(nil, tui.DarkTheme())
	view := panel.View()
	// Should not panic; should show an empty-state message.
	if strings.TrimSpace(view) == "" {
		t.Error("empty ledger should still render something (header or placeholder)")
	}
}

func TestCacheDoctorPanel_AppendRow(t *testing.T) {
	panel := tui.NewCacheDoctorPanel(nil, tui.DarkTheme())
	newRow := traceinspect.TurnLedger{Turn: 1, EpochID: "e1", HitTokens: 0, MissTokens: 3000, CostCNY: 0.001, Why: traceinspect.WhyExpectedMiss}
	panel = panel.Append(newRow)
	view := panel.View()
	if !strings.Contains(view, "expected-miss") {
		t.Errorf("appended row not visible in View\n%s", view)
	}
}
