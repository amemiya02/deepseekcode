package tui

import (
	"strings"
	"testing"
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
