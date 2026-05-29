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
