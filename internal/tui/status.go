package tui

import (
	"fmt"

	"github.com/amemiya02/deepseekcode/internal/llm"
)

// statusState is the live status-line state. Updated on every
// stepFinishMsg and on /models switches.
type statusState struct {
	model           string
	steps           int
	compactionCount int
	usage           llm.Usage
	costYuan        float64
	costKnown       bool
	thinking        bool
	hint            string
	mode            appMode
}

// render returns the one-line status string. When in Normal (scroll)
// mode, a "NORMAL" badge is shown on the left.
func (s statusState) render(t Theme) string {
	hit := llm.CacheHitRate(s.usage) * 100
	var modeBadge string
	if s.mode == modeNormal {
		modeBadge = t.Hint.Render("NORMAL ") + "· "
	}
	costStr := "¥?"
	if s.costKnown {
		costStr = fmt.Sprintf("¥%.4f", s.costYuan)
	}
	line1 := fmt.Sprintf("%s%s · %d steps · cache %.0f%% · %s",
		modeBadge, t.StatusModel.Render(shortModel(s.model)), s.steps, hit, costStr)
	if s.compactionCount > 0 {
		line1 += fmt.Sprintf(" · compacted %d", s.compactionCount)
	}
	if s.hint != "" {
		line1 += t.Hint.Render("  · " + s.hint)
	}
	return t.Status.Render(line1)
}

// shortModel returns a one-word friendly name for the status slot.
func shortModel(m string) string {
	switch m {
	case "deepseek-v4-flash":
		return "flash"
	case "deepseek-v4-pro":
		return "pro"
	case "deepseek-chat":
		return "chat"
	case "deepseek-reasoner":
		return "reasoner"
	}
	return m
}
