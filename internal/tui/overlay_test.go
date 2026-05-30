package tui

import (
	"testing"

	"github.com/amemiya02/deepseekcode/internal/llm"
)

// TestAvailableModelsIncludesChatAndReasoner pins the specific fix: the
// /models picker (which is ALSO the applyModelSwitch allowlist) must offer
// deepseek-chat and deepseek-reasoner, not just the v4 flash/pro pair.
// Before the fix, `/models deepseek-chat` was rejected as "unknown model".
func TestAvailableModelsIncludesChatAndReasoner(t *testing.T) {
	want := []string{"deepseek-v4-flash", "deepseek-v4-pro", "deepseek-chat", "deepseek-reasoner"}
	have := map[string]bool{}
	for _, m := range availableModels() {
		have[m.ID] = true
	}
	for _, id := range want {
		if !have[id] {
			t.Errorf("availableModels() is missing %q — it must be selectable in /models", id)
		}
	}
}

// TestAvailableModelsArePricedAndNamed guards the bug class that produced the
// missing-models defect: the picker drifting out of sync with the pricing and
// display tables. Every model offered for selection must (1) have a pricing
// entry so the cost HUD reads ¥0.0000 rather than ¥?, and (2) have an explicit
// shortModel mapping so the status slot shows a friendly name rather than the
// raw id. A new picker row with no Prices/shortModel entry fails here.
// TestHelpTabCyclesWithWraparound pins the tab-cycling logic: Next wraps
// 2→0, Prev wraps 0→2, and out-of-range SetHelpTab clamps.
func TestHelpTabCyclesWithWraparound(t *testing.T) {
	o := NewOverlay()
	o.OpenHelp()

	if got := o.HelpTab(); got != 0 {
		t.Fatalf("fresh OpenHelp: HelpTab() = %d, want 0", got)
	}
	// Next wraps: 0 → 1 → 2 → 0
	o.NextHelpTab()
	if got := o.HelpTab(); got != 1 {
		t.Fatalf("after 1 Next: HelpTab() = %d, want 1", got)
	}
	o.NextHelpTab()
	if got := o.HelpTab(); got != 2 {
		t.Fatalf("after 2 Next: HelpTab() = %d, want 2", got)
	}
	o.NextHelpTab()
	if got := o.HelpTab(); got != 0 {
		t.Fatalf("after 3 Next (wrap): HelpTab() = %d, want 0", got)
	}
	// Prev wraps: 0 → 2
	o.PrevHelpTab()
	if got := o.HelpTab(); got != 2 {
		t.Fatalf("Prev from 0 (wrap): HelpTab() = %d, want 2", got)
	}
	// SetHelpTab clamps over-range to helpTabCount-1.
	o.SetHelpTab(99)
	if got := o.HelpTab(); got != helpTabCount-1 {
		t.Fatalf("SetHelpTab(99): HelpTab() = %d, want %d", got, helpTabCount-1)
	}
	// SetHelpTab clamps negative to 0.
	o.SetHelpTab(-1)
	if got := o.HelpTab(); got != 0 {
		t.Fatalf("SetHelpTab(-1): HelpTab() = %d, want 0", got)
	}
}

// TestHelpTabSwitchResetsScroll verifies that switching tabs resets the scroll
// offset (cursor) to 0.
func TestHelpTabSwitchResetsScroll(t *testing.T) {
	o := NewOverlay()
	o.OpenHelp()
	o.MoveDown()
	o.MoveDown()
	if got := o.Cursor(); got != 2 {
		t.Fatalf("after 2 MoveDown: Cursor() = %d, want 2", got)
	}
	o.NextHelpTab()
	if got := o.Cursor(); got != 0 {
		t.Fatalf("NextHelpTab should reset cursor to 0, got %d", got)
	}
}

// TestOpenHelpResetsToFirstTab verifies that closing and reopening help always
// lands on tab 0, even if the user left on a different tab.
func TestOpenHelpResetsToFirstTab(t *testing.T) {
	o := NewOverlay()
	o.OpenHelp()
	o.NextHelpTab()
	o.NextHelpTab()
	if got := o.HelpTab(); got != 2 {
		t.Fatalf("precondition: HelpTab() = %d, want 2", got)
	}
	o.Close()
	o.OpenHelp()
	if got := o.HelpTab(); got != 0 {
		t.Fatalf("after Close+OpenHelp: HelpTab() = %d, want 0", got)
	}
	if got := o.Cursor(); got != 0 {
		t.Fatalf("after Close+OpenHelp: Cursor() = %d, want 0", got)
	}
}

func TestAvailableModelsArePricedAndNamed(t *testing.T) {
	seen := map[string]bool{}
	for _, m := range availableModels() {
		if m.ID == "" {
			t.Error("picker row has empty ID")
			continue
		}
		if seen[m.ID] {
			t.Errorf("duplicate model ID in picker: %q", m.ID)
		}
		seen[m.ID] = true

		if !llm.CostKnown(m.ID) {
			t.Errorf("model %q has no pricing entry — the cost HUD would show ¥? for a selectable model", m.ID)
		}
		if short := shortModel(m.ID); short == m.ID {
			t.Errorf("model %q has no shortModel mapping — the status slot would show the raw id", m.ID)
		}
		if m.Short == "" {
			t.Errorf("model %q has an empty Short label", m.ID)
		}
	}
}
