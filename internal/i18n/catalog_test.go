package i18n_test

import (
	"os"
	"testing"

	"github.com/amemiya02/deepseekcode/internal/i18n"
)

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

	// Key exists only in English catalog → must return English string, not "".
	got := i18n.T("welcome.hint.send")
	if got == "" {
		t.Error("T(welcome.hint.send) returned empty string; want English fallback")
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
