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
