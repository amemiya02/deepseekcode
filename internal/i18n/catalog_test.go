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
