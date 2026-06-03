package tui

import (
	"fmt"
	"strings"

	"github.com/amemiya02/deepseekcode/internal/i18n"
)

// StatusLabel returns the i18n-translated label for a named status phase.
// Valid keys: "thinking", "streaming", "idle", "error", "compacting".
// Unknown keys fall through to the raw key via i18n.T.
func StatusLabel(phase string) string {
	switch phase {
	case "thinking":
		return i18n.T("status.thinking")
	case "streaming":
		return i18n.T("status.streaming")
	case "idle":
		return i18n.T("status.idle")
	case "error":
		return i18n.T("status.error")
	case "compacting":
		return i18n.T("status.compacting")
	default:
		return phase
	}
}

// HUDData holds the data for rendering the status HUD.
type HUDData struct {
	Model           string
	Effort          string
	ContextTokens   int
	ContextLimit    int
	CacheHitRatio   float64
	InputHitTokens  int
	InputMissTokens int
	SavedCNY        float64
	OutputTokens    int
	ReasoningTokens int
	StepCNY         float64
	SessionCNY      float64
	ActiveAgent     string
	RunningJobs     int
}

// RenderHUD renders a single-line status HUD with width-aware truncation.
// Returns an empty string when data is empty.
func RenderHUD(data HUDData, width int) string {
	if width < 20 {
		width = 20
	}

	var parts []string

	// Model and effort
	if data.Model != "" {
		model := data.Model
		if data.Effort != "" {
			model += " " + data.Effort
		}
		parts = append(parts, model)
	}

	// Cache ratio (only when hit+miss token data exists). The persistent
	// "cache % · saved ¥X" chip makes the cache-stability investment visible;
	// the saved-¥ suffix is shown only once a positive saving has accrued.
	totalInput := data.InputHitTokens + data.InputMissTokens
	if totalInput > 0 {
		ratio := float64(data.InputHitTokens) / float64(totalInput) * 100
		chip := fmt.Sprintf("cache %.1f%%", ratio)
		if data.SavedCNY > 0 {
			chip += fmt.Sprintf(" · saved ¥%.3f", data.SavedCNY)
		}
		parts = append(parts, chip)
	}

	// Token counts
	if data.OutputTokens > 0 {
		parts = append(parts, fmt.Sprintf("out %d", data.OutputTokens))
	}
	if data.ReasoningTokens > 0 {
		parts = append(parts, fmt.Sprintf("reason %d", data.ReasoningTokens))
	}

	// Context usage — a fixed-width fill bar plus human-readable token
	// counts (e.g. "ctx [█████░░░░░] 64k/1M") so the 1M-window headroom is
	// legible at a glance.
	if data.ContextTokens > 0 && data.ContextLimit > 0 {
		parts = append(parts, contextBar(data.ContextTokens, data.ContextLimit))
	}

	// CNY cost (only when > 0)
	if data.StepCNY > 0 {
		parts = append(parts, fmt.Sprintf("¥%.3f", data.StepCNY))
	}
	if data.SessionCNY > 0 {
		parts = append(parts, fmt.Sprintf("Σ¥%.3f", data.SessionCNY))
	}

	if data.ActiveAgent != "" {
		parts = append(parts, "agent "+data.ActiveAgent)
	}
	if data.RunningJobs > 0 {
		parts = append(parts, fmt.Sprintf("jobs %d", data.RunningJobs))
	}

	if len(parts) == 0 {
		return ""
	}

	result := strings.Join(parts, " | ")
	// UTF-8 safe truncation using rune-based slicing
	runes := []rune(result)
	if len(runes) > width {
		result = string(runes[:width-1]) + "…"
	}
	return result
}

// contextBar renders a fixed-width fill bar plus human-readable token
// counts for the context window, e.g. "ctx [█████░░░░░] 64k/1M". The
// fill is integer-rounded to the nearest of contextBarCells cells; it is
// deterministic and pure (no time/locale), so it is golden-test safe.
// Callers guarantee limit > 0.
func contextBar(tokens, limit int) string {
	const cells = 10
	filled := contextFill(tokens, limit, cells)
	bar := "[" + strings.Repeat("█", filled) + strings.Repeat("░", cells-filled) + "]"
	return fmt.Sprintf("ctx %s %s/%s", bar, humanTokens(tokens), humanTokens(limit))
}

// contextFill returns the number of filled cells (0..cells) for a context bar:
// an integer round-half-up of tokens/limit, clamped. It is the SINGLE source of
// the bar geometry shared by the plain contextBar (HUD / golden tests) and the
// accent-colored renderContextBar (status line) so the two can never drift —
// no math import, deterministic and golden-test safe.
func contextFill(tokens, limit, cells int) int {
	if limit <= 0 {
		return 0
	}
	filled := (tokens*cells + limit/2) / limit
	if filled < 0 {
		return 0
	}
	if filled > cells {
		return cells
	}
	return filled
}

// humanTokens formats a token count compactly: 1_000_000 → "1M",
// 1_500_000 → "1.5M", 128_000 → "128k", 950 → "950".
func humanTokens(n int) string {
	switch {
	case n >= 1_000_000:
		v := float64(n) / 1_000_000
		if v == float64(int(v)) {
			return fmt.Sprintf("%dM", int(v))
		}
		return fmt.Sprintf("%.1fM", v)
	case n >= 1000:
		return fmt.Sprintf("%dk", n/1000)
	default:
		return fmt.Sprintf("%d", n)
	}
}
