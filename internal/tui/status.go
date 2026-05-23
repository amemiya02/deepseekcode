package tui

import (
	"fmt"

	"github.com/amemiya02/deepseekcode/internal/llm"
)

// statusState is the live status-line state. Updated on every
// stepFinishMsg and on /models switches.
type statusState struct {
	model      string
	steps      int
	usage      llm.Usage
	costYuan   float64
	thinking   bool
	duetActive bool
	proUsage   llm.Usage
	proCalls   int
	proCost    float64
	hint       string
	mode       appMode
}

// render returns the one- or two-line status string. Two lines when
// the Duet validator has actually fired this session.
// When in Normal (scroll) mode, a "NORMAL" badge is shown on the left.
func (s statusState) render(t Theme) string {
	hit := llm.CacheHitRate(s.usage) * 100
	var modeBadge string
	if s.mode == modeNormal {
		modeBadge = t.Hint.Render("NORMAL ") + "· "
	}
	line1 := fmt.Sprintf("%s%s · %d steps · cache %.0f%% · ¥%.4f",
		modeBadge, t.StatusModel.Render(shortModel(s.model)), s.steps, hit, s.costYuan)
	if s.hint != "" {
		line1 += t.Hint.Render("  · " + s.hint)
	}
	if !s.duetActive {
		return t.Status.Render(line1)
	}
	proHit := llm.CacheHitRate(s.proUsage) * 100
	line2 := fmt.Sprintf("%s · %d calls · cache %.0f%% · ¥%.4f",
		t.DuetApprove.Render("pro  "), s.proCalls, proHit, s.proCost)
	return t.Status.Render(line1) + "\n" + t.Status.Render(line2)
}

// shortModel returns a one-word friendly name for the status slot.
// "deepseek-v4-flash" → "flash", "deepseek-v4-pro" → "pro", everything
// else returned verbatim.
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
