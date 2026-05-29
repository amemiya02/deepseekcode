package tui

import (
	"fmt"
	"strings"
)

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
	filled := 0
	if limit > 0 {
		// integer round-half-up of tokens/limit*cells, no math import
		filled = (tokens*cells + limit/2) / limit
		if filled < 0 {
			filled = 0
		}
		if filled > cells {
			filled = cells
		}
	}
	bar := "[" + strings.Repeat("█", filled) + strings.Repeat("░", cells-filled) + "]"
	return fmt.Sprintf("ctx %s %s/%s", bar, humanTokens(tokens), humanTokens(limit))
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
