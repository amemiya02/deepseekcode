package tui

import (
	"strings"
	"testing"

	"github.com/amemiya02/deepseekcode/internal/llm"
	"github.com/amemiya02/deepseekcode/internal/session"
)

func TestRenderCostReport(t *testing.T) {
	in := CostReportInput{
		Model:        "deepseek-v4-flash",
		Steps:        5,
		Usage:        llm.Usage{PromptCacheHitTokens: 900, PromptCacheMissTokens: 100, CompletionTokens: 50},
		CostYuan:     0.0002,
		SavedYuan:    0.000882,
		ContextLimit: 1_000_000,
		CostKnown:    true,
	}
	out := RenderCostReport(in)
	for _, want := range []string{
		"model deepseek-v4-flash",
		"5 steps",
		"cache 90.0%",
		"hit 900",
		"miss 100",
		"out 50",
		"saved",
		"cost",
		"context",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("RenderCostReport missing %q:\n%s", want, out)
		}
	}
}

func TestRenderCostReportNoUsage(t *testing.T) {
	in := CostReportInput{
		Model:     "deepseek-v4-flash",
		Steps:     0,
		CostKnown: true,
	}
	out := RenderCostReport(in)
	if !strings.Contains(out, "cache 0.0%") {
		t.Errorf("expected 0%% cache for no usage:\n%s", out)
	}
	if strings.Contains(out, "context") {
		t.Errorf("should not show context when ContextLimit=0:\n%s", out)
	}
}

func TestRenderCostReportUnknownModel(t *testing.T) {
	in := CostReportInput{
		Model: "unknown-model",
		Steps: 1,
		Usage: llm.Usage{PromptCacheHitTokens: 100, PromptCacheMissTokens: 50, CompletionTokens: 10},
	}
	out := RenderCostReport(in)
	if !strings.Contains(out, "unknown model") {
		t.Errorf("expected 'unknown model' for unknown model:\n%s", out)
	}
}

func TestSlashCost(t *testing.T) {
	sb := NewScrollback()
	a := &App{
		scrollback: sb,
		status: statusState{
			model:     "deepseek-v4-flash",
			steps:     3,
			usage:     llm.Usage{PromptCacheHitTokens: 800, PromptCacheMissTokens: 200, CompletionTokens: 40},
			costYuan:  0.0005,
			savedYuan: 0.001,
			costKnown: true,
		},
	}
	a.handleSlash("/cost")
	items := sb.Items()
	if len(items) == 0 {
		t.Fatal("expected info row, got 0 items")
	}
	last := items[len(items)-1]
	if last.kind != itemInfo {
		t.Errorf("last item kind = %d, want itemInfo", last.kind)
	}
	if !strings.Contains(last.text, "cache 80.0%") {
		t.Errorf("cost report missing cache hit rate:\n%s", last.text)
	}
}

// TestResumeSeedsStatus proves that InitialUsageSummary values appear in
// statusState before any EventStepFinish (criterion 3 from Task-005).
func TestResumeSeedsStatus(t *testing.T) {
	summary := session.UsageSummary{
		Turns:           7,
		CacheHitTokens:  8000,
		CacheMissTokens: 2000,
		OutputTokens:    500,
		CostYuan:        0.005,
		SavedYuan:       0.01,
	}
	app := New(Config{
		Model:               "deepseek-v4-flash",
		Thinking:            true,
		InitialUsageSummary: summary,
	})

	st := app.status
	if st.steps != 7 {
		t.Errorf("steps = %d, want 7", st.steps)
	}
	if st.usage.PromptCacheHitTokens != 8000 {
		t.Errorf("usage.PromptCacheHitTokens = %d, want 8000", st.usage.PromptCacheHitTokens)
	}
	if st.usage.PromptCacheMissTokens != 2000 {
		t.Errorf("usage.PromptCacheMissTokens = %d, want 2000", st.usage.PromptCacheMissTokens)
	}
	if st.usage.CompletionTokens != 500 {
		t.Errorf("usage.CompletionTokens = %d, want 500", st.usage.CompletionTokens)
	}
	if st.costYuan != 0.005 {
		t.Errorf("costYuan = %f, want 0.005", st.costYuan)
	}
	if st.savedYuan != 0.01 {
		t.Errorf("savedYuan = %f, want 0.01", st.savedYuan)
	}
	if !st.costKnown {
		t.Error("costKnown should be true for deepseek-v4-flash")
	}
}
