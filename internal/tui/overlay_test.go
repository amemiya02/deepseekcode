package tui

import (
	"strings"
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

// TestAvailableModelsOfficialFirst verifies that official V4 models appear
// before legacy aliases in the picker.
func TestAvailableModelsOfficialFirst(t *testing.T) {
	models := availableModels()
	if len(models) < 2 {
		t.Fatal("need at least 2 models")
	}
	if models[0].ID != "deepseek-v4-flash" {
		t.Errorf("first model = %q, want deepseek-v4-flash", models[0].ID)
	}
	if models[1].ID != "deepseek-v4-pro" {
		t.Errorf("second model = %q, want deepseek-v4-pro", models[1].ID)
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

func TestOpenThemesSelectsActive(t *testing.T) {
	o := NewOverlay()
	o.OpenThemes("nebula")
	if o.Mode() != modeThemes {
		t.Fatalf("expected modeThemes, got %d", o.Mode())
	}
	if o.SelectedThemeID() != "nebula" {
		t.Fatalf("expected selected 'nebula', got %q", o.SelectedThemeID())
	}
}

func TestThemesFilterNarrows(t *testing.T) {
	o := NewOverlay()
	o.OpenThemes("dark")
	// Type "aurora" to narrow
	for _, r := range "aurora" {
		o.FilterType(r)
	}
	if o.SelectedThemeID() != "aurora" {
		t.Fatalf("after filter 'aurora': expected selected 'aurora', got %q", o.SelectedThemeID())
	}
	// Backspace to clear
	for i := 0; i < 6; i++ {
		o.FilterBackspace()
	}
	if len(o.VisibleRows()) != 5 {
		t.Fatalf("after clearing filter: expected 5 visible rows, got %d", len(o.VisibleRows()))
	}
}

func TestRenderMCPShowsBackoffAndOmitsZeroBackoff(t *testing.T) {
	th := DarkTheme()

	// Degraded server with backoff: "backoff 3:04PM" must appear.
	out := renderMCP(th, []McpServerRow{{
		Name: "fs", State: "degraded", ToolCount: 2, BackoffUntil: "3:04PM",
	}}, []int{0}, 0, "", 100, 20)
	stripped := stripANSI(out)
	if !strings.Contains(stripped, "backoff 3:04PM") {
		t.Fatalf("renderMCP missing backoff text:\n%s", stripped)
	}

	// Connected server with no backoff: "backoff" must NOT appear.
	out = renderMCP(th, []McpServerRow{{
		Name: "fs", State: "connected", ToolCount: 2,
	}}, []int{0}, 0, "", 100, 20)
	stripped = stripANSI(out)
	if strings.Contains(stripped, "backoff") {
		t.Fatalf("renderMCP showed spurious backoff for connected server:\n%s", stripped)
	}
}

func TestRenderMCPShowsErrorAndEmptyState(t *testing.T) {
	th := DarkTheme()

	// Server with last error.
	out := renderMCP(th, []McpServerRow{{
		Name: "fs", State: "failed", ToolCount: 0, LastError: "spawn: exec not found",
	}}, []int{0}, 0, "", 100, 20)
	stripped := stripANSI(out)
	if !strings.Contains(stripped, "spawn: exec not found") {
		t.Fatalf("renderMCP missing error text:\n%s", stripped)
	}

	// Empty server list.
	out = renderMCP(th, nil, nil, 0, "", 100, 20)
	stripped = stripANSI(out)
	if !strings.Contains(stripped, "no MCP servers configured") {
		t.Fatalf("renderMCP missing empty state:\n%s", stripped)
	}
}

func TestRenderLSPShowsCommandAndZeroDiagnostics(t *testing.T) {
	th := DarkTheme()

	// Connected server with command and zero diagnostics. Name and Command
	// are distinct so the test proves Command is actually rendered.
	out := renderLSP(th, []LspServerRow{{
		Name: "go", Command: "gopls-custom", Connected: true, DiagnosticCount: 0,
	}}, []int{0}, 0, "", 100, 20)
	stripped := stripANSI(out)
	if !strings.Contains(stripped, "go") {
		t.Fatalf("renderLSP missing server name:\n%s", stripped)
	}
	if !strings.Contains(stripped, "gopls-custom") {
		t.Fatalf("renderLSP missing Command text:\n%s", stripped)
	}
	if !strings.Contains(stripped, "0 diagnostics") {
		t.Fatalf("renderLSP missing zero diagnostics:\n%s", stripped)
	}
}

func TestRenderLSPShowsErrorAndEmptyState(t *testing.T) {
	th := DarkTheme()

	// Failed server with error.
	out := renderLSP(th, []LspServerRow{{
		Name: "rust-analyzer", Command: "rust-analyzer", Connected: false,
		LastError: "exec: not found",
	}}, []int{0}, 0, "", 100, 20)
	stripped := stripANSI(out)
	if !strings.Contains(stripped, "rust-analyzer") {
		t.Fatalf("renderLSP missing server name:\n%s", stripped)
	}
	if !strings.Contains(stripped, "exec: not found") {
		t.Fatalf("renderLSP missing error text:\n%s", stripped)
	}

	// Empty server list.
	out = renderLSP(th, nil, nil, 0, "", 100, 20)
	stripped = stripANSI(out)
	if !strings.Contains(stripped, "no LSP servers detected") {
		t.Fatalf("renderLSP missing empty state:\n%s", stripped)
	}
}

func TestRenderThemesPickerListsAll(t *testing.T) {
	th := DarkTheme()
	rows := availableThemes()
	visible := make([]int, len(rows))
	for i := range visible {
		visible[i] = i
	}
	out := renderThemesPicker(th, rows, visible, 0, "", "dark", 100, 40)
	stripped := stripANSI(out)
	for _, want := range []string{"DeepSeek Ocean", "Ocean Light", "Midnight", "Nebula", "Aurora"} {
		if !strings.Contains(stripped, want) {
			t.Errorf("renderThemesPicker output missing label %q", want)
		}
	}
	// Active marker should be present for "dark"
	if !strings.Contains(stripped, "*") {
		t.Error("renderThemesPicker output missing active marker '*'")
	}
}
