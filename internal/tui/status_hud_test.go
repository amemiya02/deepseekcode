package tui

import (
	"strings"
	"testing"
)

func TestRenderHUD_Empty(t *testing.T) {
	result := RenderHUD(HUDData{}, 80)
	if result != "" {
		t.Errorf("expected empty string for empty data, got %q", result)
	}
}

func TestRenderHUD_ModelOnly(t *testing.T) {
	data := HUDData{Model: "flash"}
	result := RenderHUD(data, 80)
	if result != "flash" {
		t.Errorf("expected %q, got %q", "flash", result)
	}
}

func TestRenderHUD_ModelAndEffort(t *testing.T) {
	data := HUDData{Model: "flash", Effort: "high"}
	result := RenderHUD(data, 80)
	if result != "flash high" {
		t.Errorf("expected %q, got %q", "flash high", result)
	}
}

func TestRenderHUD_CacheRatio(t *testing.T) {
	data := HUDData{
		InputHitTokens:  994,
		InputMissTokens: 6,
	}
	result := RenderHUD(data, 80)
	if !strings.Contains(result, "cache 99.4%") {
		t.Errorf("expected cache ratio in output, got %q", result)
	}
}

func TestRenderHUD_CacheRatioNoData(t *testing.T) {
	data := HUDData{Model: "flash"}
	result := RenderHUD(data, 80)
	if strings.Contains(result, "cache") {
		t.Errorf("should not include cache when no token data, got %q", result)
	}
}

func TestRenderHUD_OutputTokens(t *testing.T) {
	data := HUDData{OutputTokens: 1000}
	result := RenderHUD(data, 80)
	if !strings.Contains(result, "out 1000") {
		t.Errorf("expected output tokens in output, got %q", result)
	}
}

func TestRenderHUD_ReasoningTokens(t *testing.T) {
	data := HUDData{ReasoningTokens: 500}
	result := RenderHUD(data, 80)
	if !strings.Contains(result, "reason 500") {
		t.Errorf("expected reasoning tokens in output, got %q", result)
	}
}

func TestRenderHUD_ContextUsage(t *testing.T) {
	data := HUDData{ContextTokens: 50000, ContextLimit: 100000}
	result := RenderHUD(data, 80)
	if !strings.Contains(result, "ctx 50000/100000") {
		t.Errorf("expected context usage in output, got %q", result)
	}
}

func TestRenderHUD_StepCost(t *testing.T) {
	data := HUDData{StepCNY: 0.013}
	result := RenderHUD(data, 80)
	if !strings.Contains(result, "¥0.013") {
		t.Errorf("expected step cost in output, got %q", result)
	}
}

func TestRenderHUD_SessionCost(t *testing.T) {
	data := HUDData{SessionCNY: 0.500}
	result := RenderHUD(data, 80)
	if !strings.Contains(result, "Σ¥0.500") {
		t.Errorf("expected session cost in output, got %q", result)
	}
}

func TestRenderHUD_ZeroCost(t *testing.T) {
	data := HUDData{Model: "flash", StepCNY: 0, SessionCNY: 0}
	result := RenderHUD(data, 80)
	if strings.Contains(result, "¥") {
		t.Errorf("should not include cost when zero, got %q", result)
	}
}

func TestRenderHUD_Populated(t *testing.T) {
	data := HUDData{
		Model:           "flash",
		Effort:          "high",
		InputHitTokens:  994,
		InputMissTokens: 6,
		OutputTokens:    1000,
		StepCNY:         0.013,
	}
	result := RenderHUD(data, 80)
	if !strings.Contains(result, "flash high") {
		t.Errorf("expected model and effort, got %q", result)
	}
	if !strings.Contains(result, "cache 99.4%") {
		t.Errorf("expected cache ratio, got %q", result)
	}
	if !strings.Contains(result, "out 1000") {
		t.Errorf("expected output tokens, got %q", result)
	}
	if !strings.Contains(result, "¥0.013") {
		t.Errorf("expected cost, got %q", result)
	}
}

func TestRenderHUD_TruncatesLong(t *testing.T) {
	data := HUDData{
		Model:           "very-long-model-name-that-exceeds-width",
		Effort:          "very-long-effort",
		InputHitTokens:  994,
		InputMissTokens: 6,
		OutputTokens:    1000,
		ReasoningTokens: 500,
		StepCNY:         0.013,
		SessionCNY:      0.500,
	}
	result := RenderHUD(data, 30)
	// The result should be truncated (UTF-8 "…" is 3 bytes, so byte length may exceed width)
	if len(result) > 33 { // 30 + 3 bytes for "…"
		t.Errorf("result too long (%d bytes), expected <= 33: %q", len(result), result)
	}
}

func TestRenderHUD_SmallWidth(t *testing.T) {
	data := HUDData{Model: "flash", StepCNY: 0.013}
	result := RenderHUD(data, 10)
	// Width < 20 should be adjusted to 20
	if len(result) > 20 {
		t.Errorf("result too long for small width: %q", result)
	}
}

func TestRenderHUD_TruncatesNonASCII(t *testing.T) {
	// Test with non-ASCII characters (Chinese/Japanese/Korean)
	data := HUDData{
		Model:  "模型名称",
		Effort: "高",
	}
	result := RenderHUD(data, 10)
	// Should not panic and should be truncated
	runes := []rune(result)
	if len(runes) > 10 {
		t.Errorf("result too long (%d runes), expected <= 10: %q", len(runes), result)
	}
}

func TestRenderToolSummary_TruncatesNonASCII(t *testing.T) {
	// Test with non-ASCII characters
	result := RenderToolSummary("bash", `{"command":"echo 你好世界"}`, "output", false, 15)
	// Should not panic and should be truncated
	runes := []rune(result)
	if len(runes) > 15 {
		t.Errorf("result too long (%d runes), expected <= 15: %q", len(runes), result)
	}
}

func TestRenderToolSummary_ReadFile(t *testing.T) {
	result := RenderToolSummary("read_file", `{"path":"test.go"}`, "line1\nline2\nline3", false, 80)
	if !strings.Contains(result, "read test.go") {
		t.Errorf("expected read_file summary, got %q", result)
	}
	// strings.Count counts newlines, so "line1\nline2\nline3" has 2 newlines
	if !strings.Contains(result, "2 lines") {
		t.Errorf("expected line count, got %q", result)
	}
}

func TestRenderToolSummary_ReadFileError(t *testing.T) {
	result := RenderToolSummary("read_file", `{"path":"test.go"}`, "file not found", true, 80)
	if !strings.Contains(result, "✗") {
		t.Errorf("expected error marker, got %q", result)
	}
}

func TestRenderToolSummary_Bash(t *testing.T) {
	result := RenderToolSummary("bash", `{"command":"ls -la"}`, "file1\nfile2", false, 80)
	if !strings.Contains(result, "bash: ls -la") {
		t.Errorf("expected bash summary, got %q", result)
	}
	// strings.Count counts newlines, so "file1\nfile2" has 1 newline
	if !strings.Contains(result, "1 lines") {
		t.Errorf("expected line count, got %q", result)
	}
}

func TestRenderToolSummary_BashLongCommand(t *testing.T) {
	longCmd := "very long command that exceeds fifty characters in length"
	result := RenderToolSummary("bash", `{"command":"`+longCmd+`"}`, "output", false, 80)
	if len(longCmd) > 50 && !strings.Contains(result, "...") {
		t.Errorf("expected truncation for long command, got %q", result)
	}
}

func TestRenderToolSummary_BashError(t *testing.T) {
	result := RenderToolSummary("bash", `{"command":"rm -rf /"}`, "permission denied", true, 80)
	if !strings.Contains(result, "✗") {
		t.Errorf("expected error marker, got %q", result)
	}
}

func TestRenderToolSummary_Grep(t *testing.T) {
	result := RenderToolSummary("grep", `{"pattern":"func main"}`, "match1\nmatch2", false, 80)
	if !strings.Contains(result, "grep: func main") {
		t.Errorf("expected grep summary, got %q", result)
	}
	// strings.Count counts newlines, so "match1\nmatch2" has 1 newline
	if !strings.Contains(result, "1 matches") {
		t.Errorf("expected match count, got %q", result)
	}
}

func TestRenderToolSummary_GrepError(t *testing.T) {
	result := RenderToolSummary("grep", `{"pattern":"[invalid"}`, "bad pattern", true, 80)
	if !strings.Contains(result, "✗") {
		t.Errorf("expected error marker, got %q", result)
	}
}

func TestRenderToolSummary_Default(t *testing.T) {
	result := RenderToolSummary("custom_tool", `{}`, "result", false, 80)
	if result != "custom_tool" {
		t.Errorf("expected tool name, got %q", result)
	}
}

func TestRenderToolSummary_DefaultWithLines(t *testing.T) {
	result := RenderToolSummary("custom_tool", `{}`, "line1\nline2", false, 80)
	// strings.Count counts newlines, so "line1\nline2" has 1 newline
	if !strings.Contains(result, "1 lines") {
		t.Errorf("expected line count, got %q", result)
	}
}

func TestRenderToolSummary_TruncatesLong(t *testing.T) {
	longResult := strings.Repeat("a", 100)
	result := RenderToolSummary("bash", `{"command":"test"}`, longResult, false, 30)
	if len(result) > 30 {
		t.Errorf("result too long (%d chars), expected <= 30: %q", len(result), result)
	}
}

func TestRenderToolSummary_SmallWidth(t *testing.T) {
	result := RenderToolSummary("bash", `{"command":"test"}`, "output", false, 10)
	// Width < 20 should be adjusted to 20
	if len(result) > 20 {
		t.Errorf("result too long for small width: %q", result)
	}
}

func TestRenderHUDShowsEpochAndPendingDrift(t *testing.T) {
	got := RenderHUD(HUDData{
		Model:           "deepseek-v4-flash",
		Effort:          "medium",
		ContextTokens:   64000,
		ContextLimit:    128000,
		InputHitTokens:  950,
		InputMissTokens: 50,
		OutputTokens:    20,
		SessionCNY:      0.0123,
		EpochID:         "epoch_abcdef123456",
		PrefixHash:      "1234567890abcdef",
		PendingChanges:  2,
		DriftReason:     "tools",
		ActiveAgent:     "coding-default",
		RunningJobs:     1,
	}, 240)

	for _, want := range []string{
		"cache 95.0%",
		"ctx 64000/128000",
		"epoch abcdef12",
		"pfx 12345678",
		"pending 2",
		"drift tools",
		"agent coding-default",
		"jobs 1",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("RenderHUD missing %q:\n%s", want, got)
		}
	}
}
