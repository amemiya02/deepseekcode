package tui

import (
	"fmt"
	"strings"

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
	reasoningEffort llm.ReasoningEffort
	hint            string
	cacheNote       string
	mode            appMode
	activeAgent     string
	runningJobs     int
	// queued is the number of prompts waiting behind the active run (G11). It
	// renders as a subtle "queued N" segment only when non-zero, so an idle
	// session (the common case, and every existing golden) is unaffected.
	queued int

	// balanceNote holds the formatted account balance (e.g. "CNY=110.00
	// USD=2.50") fetched at TUI startup. Empty means balance unavailable
	// or not yet fetched.
	balanceNote string

	// Capability counters for the status-line capability segment.
	mcpTools int  // total tools across connected MCP servers; 0 hides the sub-part
	lspReady bool // at least one LSP server attached
	skills   int  // loaded skill count; 0 hides the sub-part

	// missAccum accumulates per-cause miss tokens from CacheReceipt events
	// for the four-cause HUD chip.
	missAccum MissCauses
}

// render returns the one-line status string. When in Normal (scroll)
// mode, a "NORMAL" badge is shown on the left.
//
// Brightness hierarchy (spec): the model name is the brightest element
// (fgBase, bold); the "·" separators recede to fgFaint; metric labels sit
// in between at fgSubtle. Health-bearing metrics layer a semantic color on
// top of that base (cache% ok/warn, cost warn-when-high). Only genuinely
// transient state — the prefix-cache invalidation note, "compacted N", and
// error states — is promoted to a filled Theme.Badge chip; the always-on
// metrics stay plain colored text so the line doesn't strobe with chips.
func (s statusState) render(t Theme) string {
	hit := llm.CacheHitRate(s.usage) * 100

	// Separators recede (fgFaint); metric labels sit at fgSubtle; the model
	// name is the single bright anchor (fgBase, bold).
	sep := lipgloss.NewStyle().Foreground(t.FgFaint)
	label := lipgloss.NewStyle().Foreground(t.FgSubtle)
	dot := sep.Render(" · ")

	var modeBadge string
	if s.mode == modeNormal {
		modeBadge = t.Badge(BadgeInfo).Render("NORMAL") + " "
	}

	// ¥? is reserved for models with no pricing entry. A known model
	// reads ¥0.0000 before the first turn — not ¥?, which would conflate
	// "unknown pricing" with "no cost yet".
	costStr := "¥?"
	costStyle := label
	if s.costKnown || llm.CostKnown(s.model) {
		costStr = fmt.Sprintf("¥%.4f", s.costYuan)
		// Cost turns amber once it climbs past a noticeable per-session spend
		// so an expensive run reads at a glance; cheap runs stay subtle.
		if s.costYuan >= costWarnYuan {
			costStyle = lipgloss.NewStyle().Foreground(t.WarnColor)
		}
	}

	// Single cache chip for the live line: "cache N% · saved ¥X". The cache
	// percentage is health-colored — ok (green) when the prefix cache is
	// landing, warn (amber) when it has dropped low — because the 50× cache
	// discount is the cost story and a sagging hit-rate is what a user wants
	// to notice. The saved-¥ suffix stays at label brightness.
	cacheStyle := label
	if hit >= cacheOkPct {
		cacheStyle = lipgloss.NewStyle().Foreground(t.OkColor)
	} else if hit < cacheWarnPct {
		cacheStyle = lipgloss.NewStyle().Foreground(t.WarnColor)
	}
	cacheChip := cacheStyle.Render(fmt.Sprintf("cache %.0f%%", hit))
	// Four-cause breakdown when we have miss data.
	if mc := s.missAccum; mc.ColdTokens+mc.MutTokens+mc.ResidualTokens+mc.ResetTokens > 0 {
		fourCause := fmt.Sprintf("[cold %d·mut %d·res %d·rst %d]",
			mc.ColdTokens, mc.MutTokens, mc.ResidualTokens, mc.ResetTokens)
		cacheChip += " " + label.Render(fourCause)
	}

	// Context fill bar with accent-colored filled cells. Built locally (not
	// via RenderHUD) so the filled cells can carry the brand accent while the
	// track + counts stay at label brightness. RenderHUD stays plain for its
	// standalone / golden-test callers.
	ctxTokens := s.usage.PromptCacheHitTokens + s.usage.PromptCacheMissTokens + s.usage.CompletionTokens
	var ctxSeg string
	if ctxTokens > 0 && s.contextLimit > 0 {
		ctxSeg = renderContextBar(t, ctxTokens, s.contextLimit)
	}

	// Build the full status line preserving existing model/step/cost semantics.
	line1 := modeBadge +
		t.StatusModel.Render(shortModel(s.model)) +
		dot + label.Render(fmt.Sprintf("%d steps", s.steps)) +
		dot + cacheChip +
		dot + costStyle.Render(costStr)
	if s.balanceNote != "" {
		line1 += dot + label.Render("bal "+s.balanceNote)
	}
	if s.thinking && s.reasoningEffort.Valid() {
		line1 += dot + label.Render("effort "+string(s.reasoningEffort))
	}
	if ctxSeg != "" {
		line1 += dot + ctxSeg
	}
	if s.runningJobs > 0 {
		line1 += dot + label.Render(fmt.Sprintf("jobs %d", s.runningJobs))
	}
	// Queued-prompt count (G11): subtle, label-brightness, present only while
	// prompts wait behind the active run so it never strobes an idle line.
	if s.queued > 0 {
		line1 += dot + label.Render(fmt.Sprintf("queued %d", s.queued))
	}
	// Capability segment: mcp/lsp/skills — only non-zero/true parts shown.
	if s.mcpTools > 0 {
		line1 += dot + label.Render(fmt.Sprintf("mcp %d", s.mcpTools))
	}
	if s.lspReady {
		line1 += dot + label.Render("lsp ✓")
	}

	// Transient-state chips. "compacted N" is an episodic event, so it reads
	// as a brand chip rather than a plain metric.
	if s.compactionCount > 0 {
		line1 += "  " + t.Badge(BadgeBrand).Render(fmt.Sprintf("compacted %d", s.compactionCount))
	}
	// cacheNote (prefix-cache invalidation) is a warning-worthy transient: a
	// cache miss just cost real money. Surface it as a "⚠ CACHE" amber chip
	// with the original note trailing at label brightness, kept in its own
	// slot so it never clobbers the generic hint. The note CONTENT is
	// preserved verbatim — only its presentation is upgraded.
	if s.cacheNote != "" {
		line1 += "  " + t.Badge(BadgeWarn).Render("⚠ CACHE") +
			" " + label.Render(s.cacheNote)
	}
	if s.hint != "" {
		line1 += dot + t.Hint.Render(s.hint)
	}
	return t.Status.Render(line1)
}

// Health thresholds for status-line metric coloring. Tuned for the DeepSeek
// prompt-cache economics: a hit-rate at or above cacheOkPct is "the cache is
// working" (green); below cacheWarnPct it is worth flagging (amber); the band
// between stays neutral. costWarnYuan is the per-session spend past which the
// cost figure turns amber.
const (
	cacheOkPct   = 80.0
	cacheWarnPct = 50.0
	costWarnYuan = 1.0
)

// renderContextBar renders the ctx fill bar with the brand accent on the
// filled cells. The label "ctx", the track "[…]" brackets, the empty cells,
// and the "N/M" counts stay at label (fgSubtle) brightness; only the filled
// "█" run is promoted to BrandLight so the consumed slice pops. Layout matches
// the pure contextBar() so the golden tests on that function are unaffected.
func renderContextBar(t Theme, tokens, limit int) string {
	const cells = 10
	filled := contextFill(tokens, limit, cells) // shared geometry; cannot drift from contextBar
	label := lipgloss.NewStyle().Foreground(t.FgSubtle)
	accent := lipgloss.NewStyle().Foreground(t.BrandLight)
	bar := label.Render("[") +
		accent.Render(strings.Repeat("█", filled)) +
		label.Render(strings.Repeat("░", cells-filled)+"]")
	return label.Render("ctx ") + bar + label.Render(" "+humanTokens(tokens)+"/"+humanTokens(limit))
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
