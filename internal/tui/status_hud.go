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
	OutputTokens    int
	ReasoningTokens int
	StepCNY         float64
	SessionCNY      float64
	EpochID         string
	PrefixHash      string
	PendingChanges  int
	DriftReason     string
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

	// Cache ratio (only when hit+miss token data exists)
	totalInput := data.InputHitTokens + data.InputMissTokens
	if totalInput > 0 {
		ratio := float64(data.InputHitTokens) / float64(totalInput) * 100
		parts = append(parts, fmt.Sprintf("cache %.1f%%", ratio))
	}

	// Token counts
	if data.OutputTokens > 0 {
		parts = append(parts, fmt.Sprintf("out %d", data.OutputTokens))
	}
	if data.ReasoningTokens > 0 {
		parts = append(parts, fmt.Sprintf("reason %d", data.ReasoningTokens))
	}

	// Context usage
	if data.ContextTokens > 0 && data.ContextLimit > 0 {
		parts = append(parts, fmt.Sprintf("ctx %d/%d", data.ContextTokens, data.ContextLimit))
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
	if data.EpochID != "" {
		parts = append(parts, "epoch "+shortHUD(data.EpochID))
	}
	if data.PrefixHash != "" {
		parts = append(parts, "pfx "+shortHUD(data.PrefixHash))
	}
	if data.PendingChanges > 0 {
		parts = append(parts, fmt.Sprintf("pending %d", data.PendingChanges))
	}
	if data.DriftReason != "" {
		parts = append(parts, "drift "+data.DriftReason)
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

func shortHUD(s string) string {
	if strings.HasPrefix(s, "epoch_") {
		s = strings.TrimPrefix(s, "epoch_")
	}
	r := []rune(s)
	if len(r) <= 8 {
		return s
	}
	return string(r[:8])
}
