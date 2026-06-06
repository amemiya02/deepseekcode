package i18n_test

import (
	"os"
	"testing"

	"github.com/amemiya02/deepseekcode/internal/i18n"
)

// TestMain registers test-only catalog entries before any test runs and
// removes them when the suite finishes.  This keeps the production messages
// map free of test data while still allowing TestT_Fallback_MissingKey to
// exercise the English-fallback path.
func TestMain(m *testing.M) {
	cleanup := i18n.RegisterTestMessages("en", map[string]string{
		"_test.en_only_sentinel": "english-only-sentinel",
	})
	code := m.Run()
	cleanup()
	os.Exit(code)
}

func TestT_English_default(t *testing.T) {
	// No locale env set → English fallback.
	os.Unsetenv("DEEPSEEKCODE_LANG")
	os.Unsetenv("LANG")
	os.Unsetenv("LANGUAGE")
	i18n.ReloadLocale()

	got := i18n.T("welcome.tagline")
	want := "Terminal coding agent for DeepSeek"
	if got != want {
		t.Errorf("T(welcome.tagline) = %q; want %q", got, want)
	}
}

func TestT_ZhCN(t *testing.T) {
	os.Setenv("DEEPSEEKCODE_LANG", "zh-CN")
	defer os.Unsetenv("DEEPSEEKCODE_LANG")
	i18n.ReloadLocale()

	got := i18n.T("welcome.tagline")
	want := "DeepSeek 终端编程代理"
	if got != want {
		t.Errorf("T(welcome.tagline) zh-CN = %q; want %q", got, want)
	}
}

func TestT_Fallback_MissingKey(t *testing.T) {
	os.Setenv("DEEPSEEKCODE_LANG", "zh-CN")
	defer os.Unsetenv("DEEPSEEKCODE_LANG")
	i18n.ReloadLocale()

	// "_test.en_only_sentinel" exists only in the English catalog, so with
	// zh-CN active the lookup must fall back to English and return the exact
	// English string — not the zh-CN translation and not an empty string.
	got := i18n.T("_test.en_only_sentinel")
	want := "english-only-sentinel"
	if got != want {
		t.Errorf("T(_test.en_only_sentinel) with zh-CN locale = %q; want English fallback %q", got, want)
	}
}

func TestT_Fallback_UnknownLocale(t *testing.T) {
	os.Setenv("DEEPSEEKCODE_LANG", "fr-FR")
	defer os.Unsetenv("DEEPSEEKCODE_LANG")
	i18n.ReloadLocale()

	got := i18n.T("welcome.tagline")
	want := "Terminal coding agent for DeepSeek"
	if got != want {
		t.Errorf("unknown locale fallback = %q; want English %q", got, want)
	}
}

func TestT_FormatArgs(t *testing.T) {
	os.Unsetenv("DEEPSEEKCODE_LANG")
	os.Unsetenv("LANG")
	i18n.ReloadLocale()

	got := i18n.T("permission.prompt", "read_file", "/etc/passwd")
	if got == "" {
		t.Error("T with format args returned empty string")
	}
}

func TestT_LANGEnvDetection(t *testing.T) {
	os.Unsetenv("DEEPSEEKCODE_LANG")
	os.Setenv("LANG", "zh_CN.UTF-8")
	defer os.Unsetenv("LANG")
	i18n.ReloadLocale()

	got := i18n.T("welcome.tagline")
	want := "DeepSeek 终端编程代理"
	if got != want {
		t.Errorf("LANG=zh_CN.UTF-8 detection = %q; want %q", got, want)
	}
}

// TestPermissionSessionKey guards against regression where the "S" button in
// the permission modal used the format-template key "permission.prompt" instead
// of the label key "permission.session". The session key must not contain "%s".
func TestPermissionSessionKey(t *testing.T) {
	os.Unsetenv("DEEPSEEKCODE_LANG")
	os.Unsetenv("LANG")
	os.Unsetenv("LANGUAGE")
	i18n.ReloadLocale()

	got := i18n.T("permission.session")
	if got == "permission.session" {
		t.Error("permission.session key is missing from the catalog")
	}
	// Must be a plain label — not a format string with %s placeholders.
	if len(got) > 0 && got[0] == '%' {
		t.Errorf("permission.session looks like a format template: %q", got)
	}
	// Must differ from permission.prompt (the old bug used the wrong key).
	prompt := i18n.T("permission.prompt", "tool", "path")
	if got == prompt {
		t.Errorf("permission.session == permission.prompt(formatted); session key is wrong: %q", got)
	}
}

// TestQuestionNavHint guards against regression where the has-options hint
// dropped "↑↓ move" and "esc cancel". The nav hint must contain both.
func TestQuestionNavHint(t *testing.T) {
	os.Unsetenv("DEEPSEEKCODE_LANG")
	os.Unsetenv("LANG")
	os.Unsetenv("LANGUAGE")
	i18n.ReloadLocale()

	// Single-select: no multiHint.
	got := i18n.T("question.nav.hint", "")
	for _, want := range []string{"↑↓", "⏎", "esc"} {
		if !contains(got, want) {
			t.Errorf("question.nav.hint (single) missing %q; got %q", want, got)
		}
	}

	// Multi-select: multiHint appended via %s.
	gotMulti := i18n.T("question.nav.hint", " · space toggle")
	if !contains(gotMulti, "space toggle") {
		t.Errorf("question.nav.hint (multi) missing 'space toggle'; got %q", gotMulti)
	}
}

// TestAppInputHint guards against regression where the default input hint
// was split between hardcoded English and a partial i18n key, producing
// mixed-language output in zh-CN. The full hint must be a single catalog key.
func TestAppInputHint(t *testing.T) {
	os.Unsetenv("DEEPSEEKCODE_LANG")
	os.Unsetenv("LANG")
	os.Unsetenv("LANGUAGE")
	i18n.ReloadLocale()

	got := i18n.T("app.input.hint")
	if got == "app.input.hint" {
		t.Error("app.input.hint key is missing from the catalog")
	}
	// Must include the key navigation tokens present in the English value.
	for _, want := range []string{"⏎", "^C", "^R"} {
		if !contains(got, want) {
			t.Errorf("app.input.hint missing %q; got %q", want, got)
		}
	}

	// zh-CN variant must not fall back to the English value (it has its own translation).
	os.Setenv("DEEPSEEKCODE_LANG", "zh-CN")
	defer os.Unsetenv("DEEPSEEKCODE_LANG")
	i18n.ReloadLocale()
	gotZh := i18n.T("app.input.hint")
	if gotZh == got {
		t.Errorf("app.input.hint zh-CN is identical to English — zh-CN translation missing? got %q", gotZh)
	}
}

// contains is a simple substring helper to keep test code readable.
func contains(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && func() bool {
		for i := 0; i <= len(s)-len(sub); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	}())
}
