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
