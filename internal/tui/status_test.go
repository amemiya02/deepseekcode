package tui

import (
	"strings"
	"testing"

	"github.com/amemiya02/deepseekcode/internal/llm"
)

// A known-priced model must read ¥0.0000 before the first turn — never
// ¥?, which is reserved for models with no pricing entry.
func TestStatusCostKnownModelShowsZero(t *testing.T) {
	th := DarkTheme()
	s := statusState{model: "deepseek-v4-flash", steps: 0, costYuan: 0, costKnown: false}
	plain := stripANSI(s.render(th))
	if strings.Contains(plain, "¥?") {
		t.Errorf("known model should not show ¥?; got %q", plain)
	}
	if !strings.Contains(plain, "¥0.0000") {
		t.Errorf("known model at 0 steps should show ¥0.0000; got %q", plain)
	}
}

// An unknown model (no pricing entry) keeps the ¥? sentinel.
func TestStatusUnknownModelShowsQuestion(t *testing.T) {
	th := DarkTheme()
	s := statusState{model: "mystery-model", costKnown: false}
	plain := stripANSI(s.render(th))
	if !strings.Contains(plain, "¥?") {
		t.Errorf("unknown model should show ¥?; got %q", plain)
	}
}

// T5.3: the live status line carries a single cache chip with the saved-¥
// suffix, a populated context fill bar, and separate slots for the
// prefix-cache note and the generic transient hint (they coexist).
func TestStatusCacheSavingsAndNoteCoexist(t *testing.T) {
	th := DarkTheme()
	s := statusState{
		model:        "deepseek-v4-flash",
		savedYuan:    0.123,
		contextLimit: 1_000_000,
		usage: llm.Usage{
			PromptCacheHitTokens:  950,
			PromptCacheMissTokens: 50,
			CompletionTokens:      20,
		},
		cacheNote: "⚠ cache:tools changed",
		hint:      "generic hint",
	}
	plain := stripANSI(s.render(th))
	// Exactly one "cache " chip — the duplicate HUD cache chip was removed.
	if n := strings.Count(plain, "cache "); n != 1 {
		t.Errorf("expected exactly one 'cache ' chip, got %d in %q", n, plain)
	}
	if !strings.Contains(plain, "saved ¥0.123") {
		t.Errorf("expected saved-¥ suffix, got %q", plain)
	}
	if !strings.Contains(plain, "cache:tools changed") || !strings.Contains(plain, "generic hint") {
		t.Errorf("cacheNote and hint must coexist, got %q", plain)
	}
	if !strings.Contains(plain, "ctx [") {
		t.Errorf("expected ctx fill bar from populated context limit, got %q", plain)
	}
}

// Rework Task-004 crit 4: the live status line (statusState.render) shows
// "effort" when thinking is enabled and a valid effort is set.
func TestStatusEffortShownWhenThinkingOn(t *testing.T) {
	th := DarkTheme()
	s := statusState{
		model:           "deepseek-v4-flash",
		thinking:        true,
		reasoningEffort: llm.ReasoningEffortMax,
		steps:           1,
	}
	plain := stripANSI(s.render(th))
	if !strings.Contains(plain, "effort max") {
		t.Errorf("thinking=true + valid effort should show 'effort max'; got %q", plain)
	}
}

// Rework Task-004 crit 4: the live status line hides "effort" when thinking
// is disabled, even if a valid effort is configured.
func TestStatusEffortHiddenWhenThinkingOff(t *testing.T) {
	th := DarkTheme()
	s := statusState{
		model:           "deepseek-v4-flash",
		thinking:        false,
		reasoningEffort: llm.ReasoningEffortMax,
		steps:           1,
	}
	plain := stripANSI(s.render(th))
	if strings.Contains(plain, "effort") {
		t.Errorf("thinking=false should hide effort; got %q", plain)
	}
}

// TestStatusCapabilitySegment verifies the mcp/lsp/skills capability
// segment: non-zero/true parts are shown, zero/false parts are omitted.
func TestStatusCapabilitySegment(t *testing.T) {
	th := DarkTheme()

	// All zero/false → no capability segment, output identical to before.
	s0 := statusState{model: "deepseek-v4-flash", steps: 1}
	out0 := stripANSI(s0.render(th))
	if strings.Contains(out0, "mcp") || strings.Contains(out0, "lsp") || strings.Contains(out0, "skills") {
		t.Errorf("all-zero capabilities should not appear: %q", out0)
	}

	// Mixed: mcp=3, lsp=true, skills=5.
	s1 := statusState{model: "deepseek-v4-flash", steps: 1, mcpTools: 3, lspReady: true, skills: 5}
	out1 := stripANSI(s1.render(th))
	if !strings.Contains(out1, "mcp 3") {
		t.Errorf("expected 'mcp 3', got %q", out1)
	}
	if !strings.Contains(out1, "lsp ✓") {
		t.Errorf("expected 'lsp ✓', got %q", out1)
	}
	if !strings.Contains(out1, "skills 5") {
		t.Errorf("expected 'skills 5', got %q", out1)
	}

	// skills=0 → skills part omitted, others shown.
	s2 := statusState{model: "deepseek-v4-flash", steps: 1, mcpTools: 2, lspReady: true}
	out2 := stripANSI(s2.render(th))
	if strings.Contains(out2, "skills") {
		t.Errorf("skills=0 should be omitted: %q", out2)
	}
	if !strings.Contains(out2, "mcp 2") {
		t.Errorf("expected 'mcp 2', got %q", out2)
	}
}
