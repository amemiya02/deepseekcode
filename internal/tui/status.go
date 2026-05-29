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
	savedYuan       float64
	contextLimit    int
	costKnown       bool
	thinking        bool
	hint            string
	cacheNote       string
	mode            appMode
	activeAgent     string
	runningJobs     int
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

	// Single cache chip for the live line: "cache N% · saved ¥X". The cache
	// hit/miss tokens are intentionally NOT fed into hudData below — doing so
	// would render a second, redundant cache chip on the same line. RenderHUD
	// retains its own cache+saved chip for callers that do pass those tokens
	// (the HUD snapshot test and any future standalone HUD surface).
	cacheChip := fmt.Sprintf("cache %.0f%%", hit)
	if s.savedYuan > 0 {
		cacheChip += fmt.Sprintf(" · saved ¥%.3f", s.savedYuan)
	}

	// Build HUD data for additional metrics (without model to avoid duplication)
	hudData := HUDData{
		ContextTokens: s.usage.PromptCacheHitTokens + s.usage.PromptCacheMissTokens + s.usage.CompletionTokens,
		ContextLimit:  s.contextLimit,
		OutputTokens:  s.usage.CompletionTokens,
		ActiveAgent:   s.activeAgent,
		RunningJobs:   s.runningJobs,
	}

	hudLine := RenderHUD(hudData, 200) // Use wide width for status line

	// Build the full status line preserving existing model/step/cost semantics
	line1 := modeBadge + t.StatusModel.Render(shortModel(s.model)) +
		dot + fmt.Sprintf("%d steps", s.steps) +
		dot + cacheChip +
		dot + costStr
	if hudLine != "" {
		line1 += dot + hudLine
	}
	if s.compactionCount > 0 {
		line1 += dot + fmt.Sprintf("compacted %d", s.compactionCount)
	}
	// cacheNote (prefix-cache invalidation warning) and the generic transient
	// hint occupy separate slots so neither clobbers the other.
	if s.cacheNote != "" {
		line1 += t.Hint.Render("  · " + s.cacheNote)
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
