package tui_test

import (
	"os"
	"strings"
	"testing"

	"github.com/amemiya02/deepseekcode/internal/i18n"
	"github.com/amemiya02/deepseekcode/internal/tui"
)

func TestWelcomeBanner_ZhCN(t *testing.T) {
	os.Setenv("DEEPSEEKCODE_LANG", "zh-CN")
	defer os.Unsetenv("DEEPSEEKCODE_LANG")
	i18n.ReloadLocale()

	banner := tui.ExportedRenderWelcome(tui.DefaultTheme(), 100)
	if !strings.Contains(banner, "DeepSeek 终端编程代理") {
		t.Errorf("zh-CN banner missing Chinese tagline; got:\n%s", banner)
	}
}

func TestWelcomeBanner_English_default(t *testing.T) {
	os.Unsetenv("DEEPSEEKCODE_LANG")
	os.Unsetenv("LANG")
	i18n.ReloadLocale()

	banner := tui.ExportedRenderWelcome(tui.DefaultTheme(), 100)
	if !strings.Contains(banner, "Terminal coding agent for DeepSeek") {
		t.Errorf("English banner missing tagline; got:\n%s", banner)
	}
}

// TestPermissionRender_SessionButton guards that the "S" button in the
// permission modal renders the session-allow label, NOT the format-template
// "permission.prompt" string. Regression: d66948c wired the wrong key.
func TestPermissionRender_SessionButton(t *testing.T) {
	os.Unsetenv("DEEPSEEKCODE_LANG")
	os.Unsetenv("LANG")
	i18n.ReloadLocale()

	rendered := tui.ExportedRenderPermission(tui.DefaultTheme(), 80)
	// The session label must appear somewhere in the rendered card.
	sessionLabel := i18n.T("permission.session")
	if !strings.Contains(rendered, sessionLabel) {
		t.Errorf("permission card missing session-button label %q; got:\n%s", sessionLabel, rendered)
	}
	// The raw format template must NOT appear as literal button text.
	rawTemplate := "Allow tool %s on %s?"
	if strings.Contains(rendered, rawTemplate) {
		t.Errorf("permission card rendered raw format template as button label (wrong key used); got:\n%s", rendered)
	}
}

// TestQuestionRender_NavHint guards that the has-options hint includes the full
// navigation text (↑↓ move, ⏎ confirm, esc cancel). Regression: d66948c
// replaced the full hint with "question.input.hint" which drops navigation text.
func TestQuestionRender_NavHint(t *testing.T) {
	os.Unsetenv("DEEPSEEKCODE_LANG")
	os.Unsetenv("LANG")
	i18n.ReloadLocale()

	rendered := tui.ExportedRenderQuestion(tui.DefaultTheme(), 80)
	for _, want := range []string{"↑↓", "⏎", "esc"} {
		if !strings.Contains(rendered, want) {
			t.Errorf("question card nav hint missing %q; got:\n%s", want, rendered)
		}
	}
}
