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
	// T5.3: context usage now renders a fill bar + human-readable counts.
	if !strings.Contains(result, "ctx [") || !strings.Contains(result, "50k/100k") {
		t.Errorf("expected context fill bar with 50k/100k in output, got %q", result)
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
	// "line1\nline2\nline3" is 3 lines (the last has no trailing newline);
	// lineCount counts it correctly where strings.Count undercounted to 2.
	if !strings.Contains(result, "3 lines") {
		t.Errorf("expected 3-line count, got %q", result)
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
	// "file1\nfile2" is 2 lines (lineCount; strings.Count undercounted to 1).
	if !strings.Contains(result, "2 lines") {
		t.Errorf("expected 2-line count, got %q", result)
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
	// "match1\nmatch2" is 2 match lines (lineCount; strings.Count undercounted to 1).
	if !strings.Contains(result, "2 matches") {
		t.Errorf("expected 2-match count, got %q", result)
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
	// "line1\nline2" is 2 lines (lineCount; strings.Count undercounted to 1).
	if !strings.Contains(result, "2 lines") {
		t.Errorf("expected 2-line count, got %q", result)
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

// TestRenderToolSummaryLineCountSemantics pins the T5.2 line-count fix: the last
// line counts even without a trailing newline; a trailing newline doesn't
// inflate the count; and single-line output shows no "(N lines)" suffix.
func TestRenderToolSummaryLineCountSemantics(t *testing.T) {
	cases := []struct {
		name, result, wantContains string
		wantNoCount                bool
	}{
		{name: "three lines no trailing newline", result: "a\nb\nc", wantContains: "(3 lines)"},
		{name: "trailing newline counts effective lines", result: "a\nb\n", wantContains: "(2 lines)"},
		{name: "single line no suffix", result: "just one line", wantNoCount: true},
		{name: "single line trailing newline no suffix", result: "one\n", wantNoCount: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := RenderToolSummary("read_file", `{"path":"f.go"}`, c.result, false, 80)
			if c.wantNoCount {
				if strings.Contains(got, "lines)") {
					t.Errorf("single-line result should have no line-count suffix, got %q", got)
				}
				return
			}
			if !strings.Contains(got, c.wantContains) {
				t.Errorf("got %q, want it to contain %q", got, c.wantContains)
			}
		})
	}
}

// TestExtractJSONStringHandlesEscapes proves the json.Unmarshal-based field
// extraction (T5.2) handles escaped quotes and ignores non-string values —
// cases the old substring scanner got wrong.
func TestExtractJSONStringHandlesEscapes(t *testing.T) {
	if got := extractJSONString(`{"command":"echo \"hi\" there"}`, "command"); got != `echo "hi" there` {
		t.Errorf("escaped-quote value = %q, want %q", got, `echo "hi" there`)
	}
	if got := extractJSONString(`{"path":"a.go","n":3}`, "path"); got != "a.go" {
		t.Errorf("path = %q, want a.go", got)
	}
	if got := extractJSONString(`{"n":3}`, "n"); got != "" {
		t.Errorf("non-string value should yield \"\", got %q", got)
	}
	if got := extractJSONString(`not json`, "path"); got != "" {
		t.Errorf("malformed JSON should yield \"\", got %q", got)
	}
}

// TestRenderHUD_CacheSavingsChip pins the T5.3 "cache % · saved ¥X" chip:
// the saved suffix appears only when SavedCNY > 0, and the cache % is
// always present once input tokens exist.
func TestRenderHUD_CacheSavingsChip(t *testing.T) {
	withSavings := RenderHUD(HUDData{InputHitTokens: 950, InputMissTokens: 50, SavedCNY: 0.123}, 200)
	if !strings.Contains(withSavings, "cache 95.0%") {
		t.Errorf("expected cache ratio, got %q", withSavings)
	}
	if !strings.Contains(withSavings, "saved ¥0.123") {
		t.Errorf("expected saved suffix, got %q", withSavings)
	}
	noSavings := RenderHUD(HUDData{InputHitTokens: 950, InputMissTokens: 50}, 200)
	if strings.Contains(noSavings, "saved") {
		t.Errorf("saved suffix must not appear when SavedCNY == 0, got %q", noSavings)
	}
}

// TestContextBar checks the fill bar is proportional, clamped, and pure.
func TestContextBar(t *testing.T) {
	cases := []struct {
		tokens, limit        int
		wantFill, wantCounts string
	}{
		{0, 1_000_000, "[░░░░░░░░░░]", "0/1M"},
		{500_000, 1_000_000, "[█████░░░░░]", "500k/1M"},
		{1_000_000, 1_000_000, "[██████████]", "1M/1M"},
		{2_000_000, 1_000_000, "[██████████]", "2M/1M"}, // clamp at full
		{64_000, 128_000, "[█████░░░░░]", "64k/128k"},
	}
	for _, c := range cases {
		got := contextBar(c.tokens, c.limit)
		if !strings.Contains(got, c.wantFill) {
			t.Errorf("contextBar(%d,%d) = %q, want fill %q", c.tokens, c.limit, got, c.wantFill)
		}
		if !strings.Contains(got, c.wantCounts) {
			t.Errorf("contextBar(%d,%d) = %q, want counts %q", c.tokens, c.limit, got, c.wantCounts)
		}
	}
	// Deterministic: same inputs → same output (no time/locale dependence).
	if contextBar(123_456, 1_000_000) != contextBar(123_456, 1_000_000) {
		t.Error("contextBar must be deterministic")
	}
}

func TestHumanTokens(t *testing.T) {
	cases := []struct {
		n    int
		want string
	}{
		{0, "0"}, {950, "950"}, {64_000, "64k"}, {128_000, "128k"},
		{1_000_000, "1M"}, {1_500_000, "1.5M"},
	}
	for _, c := range cases {
		if got := humanTokens(c.n); got != c.want {
			t.Errorf("humanTokens(%d) = %q, want %q", c.n, got, c.want)
		}
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
		"ctx [",
		"64k/128k",
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
