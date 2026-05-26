package tui

import (
	"fmt"

	"charm.land/lipgloss/v2"

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
	dot := lipgloss.NewStyle().Foreground(t.BrandDeep).Render(" · ")
	var modeBadge string
	if s.mode == modeNormal {
		modeBadge = t.Hint.Render("NORMAL") + dot
	}
	// ¥? is reserved for models with no pricing entry. A known model
	// reads ¥0.0000 before the first turn — not ¥?, which would conflate
	// "unknown pricing" with "no cost yet".
	costStr := "¥?"
	if s.costKnown || llm.CostKnown(s.model) {
		costStr = fmt.Sprintf("¥%.4f", s.costYuan)
	}

	// Build HUD data for additional metrics (without model to avoid duplication)
	hudData := HUDData{
		ContextTokens:   s.usage.PromptCacheHitTokens + s.usage.PromptCacheMissTokens + s.usage.CompletionTokens,
		InputHitTokens:  s.usage.PromptCacheHitTokens,
		InputMissTokens: s.usage.PromptCacheMissTokens,
		OutputTokens:    s.usage.CompletionTokens,
	}

	hudLine := RenderHUD(hudData, 200) // Use wide width for status line

	// Build the full status line preserving existing model/step/cost semantics
	line1 := modeBadge + t.StatusModel.Render(shortModel(s.model)) +
		dot + fmt.Sprintf("%d steps", s.steps) +
		dot + fmt.Sprintf("cache %.0f%%", hit) +
		dot + costStr
	if hudLine != "" {
		line1 += dot + hudLine
	}
	if s.compactionCount > 0 {
		line1 += dot + fmt.Sprintf("compacted %d", s.compactionCount)
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
