package h2h

import (
	"strings"
	"testing"
)

func TestRenderReportAggregates(t *testing.T) {
	rr := RunResult{Date: "2026-06-10", Model: "deepseek-v4-flash", Results: []ArmResult{
		{Arm: "dsc", TaskID: "a", Resolved: true, Turns: []TurnUsage{{HitTokens: 900, MissTokens: 100, OutTokens: 50}}},
		{Arm: "reasonix", TaskID: "a", Resolved: true, Turns: []TurnUsage{{HitTokens: 800, MissTokens: 200, OutTokens: 50}}},
	}}
	md := RenderReport(rr)
	for _, want := range []string{"| dsc |", "| reasonix |", "90.0%", "80.0%", "WIN GATE: true"} {
		if !strings.Contains(md, want) {
			t.Fatalf("report missing %q:\n%s", want, md)
		}
	}
}

// many builds n identical TurnUsage values.
func many(n int, t TurnUsage) []TurnUsage {
	out := make([]TurnUsage, n)
	for i := range out {
		out[i] = t
	}
	return out
}

func TestWinGateAllowsResolvingMore(t *testing.T) {
	rr := RunResult{Date: "2026-06-10", Model: "test", Results: []ArmResult{
		// dsc resolves 2 tasks, higher hit rate, lower billable (resolved-only)
		{Arm: "dsc", TaskID: "a", Repeat: 1, Resolved: true, Turns: many(2, TurnUsage{HitTokens: 95, MissTokens: 5, OutTokens: 1})},
		{Arm: "dsc", TaskID: "b", Repeat: 1, Resolved: true, Turns: many(2, TurnUsage{HitTokens: 95, MissTokens: 5, OutTokens: 1})},
		// reasonix resolves 1 task, lower hit rate, higher billable
		{Arm: "reasonix", TaskID: "a", Repeat: 1, Resolved: true, Turns: many(2, TurnUsage{HitTokens: 80, MissTokens: 20, OutTokens: 5})},
		{Arm: "reasonix", TaskID: "b", Repeat: 1, Resolved: false, Turns: many(2, TurnUsage{HitTokens: 50, MissTokens: 50, OutTokens: 10})},
	}}
	md := RenderReport(rr)
	if !strings.Contains(md, "WIN GATE: true") {
		t.Fatalf("expected win gate true when dsc resolves more with better resolved-only metrics:\n%s", md)
	}
}

func TestReportShowsResolvedOnlyAndTurnsAndMedian(t *testing.T) {
	rr := RunResult{Date: "2026-06-10", Model: "test", Results: []ArmResult{
		// Short resolved run: 2 turns, high hit rate
		{Arm: "dsc", TaskID: "a", Repeat: 1, Resolved: true, DNF: false,
			Turns: many(2, TurnUsage{HitTokens: 800, MissTokens: 200, OutTokens: 10})},
		// Long DNF run: 10 turns, low hit rate
		{Arm: "dsc", TaskID: "b", Repeat: 1, Resolved: false, DNF: true,
			Turns: many(10, TurnUsage{HitTokens: 100, MissTokens: 900, OutTokens: 50})},
		// reasonix: one resolved, one DNF
		{Arm: "reasonix", TaskID: "a", Repeat: 1, Resolved: true, DNF: false,
			Turns: many(3, TurnUsage{HitTokens: 700, MissTokens: 300, OutTokens: 10})},
		{Arm: "reasonix", TaskID: "b", Repeat: 1, Resolved: false, DNF: true,
			Turns: many(8, TurnUsage{HitTokens: 200, MissTokens: 800, OutTokens: 40})},
	}}
	md := RenderReport(rr)

	// Check turns column exists in per-run detail.
	if !strings.Contains(md, "| turns |") {
		t.Fatalf("per-run detail missing turns column:\n%s", md)
	}
	// Verify actual turn counts appear.
	if !strings.Contains(md, "| 2 |") {
		t.Fatalf("per-run detail missing turn count 2:\n%s", md)
	}
	if !strings.Contains(md, "| 10 |") {
		t.Fatalf("per-run detail missing turn count 10:\n%s", md)
	}

	// Check resolved-only section exists.
	if !strings.Contains(md, "## Resolved-only aggregate") {
		t.Fatalf("report missing resolved-only section:\n%s", md)
	}

	// The resolved-only section should only count resolved runs.
	// dsc has 1 resolved run with hit=800, miss=200 => 80.0% hit rate.
	// reasonix has 1 resolved run with hit=700, miss=300 => 70.0% hit rate.
	// Verify the resolved-only table has these values.
	resolvedSection := md[strings.Index(md, "## Resolved-only aggregate"):]
	if !strings.Contains(resolvedSection, "80.0%") {
		t.Fatalf("resolved-only section missing dsc 80.0%% hit rate:\n%s", resolvedSection)
	}
	if !strings.Contains(resolvedSection, "70.0%") {
		t.Fatalf("resolved-only section missing reasonix 70.0%% hit rate:\n%s", resolvedSection)
	}

	// Check median hit rate column exists in all-runs table.
	if !strings.Contains(md, "| median hit rate |") {
		t.Fatalf("all-runs table missing median hit rate column:\n%s", md)
	}
	// dsc per-run rates: 80.0%, 10.0% => median = (10+80)/2 = 45.0%
	if !strings.Contains(md, "45.0%") {
		t.Fatalf("report missing median hit rate 45.0%%:\n%s", md)
	}
}

func TestReportShowsTokenAttribution(t *testing.T) {
	rr := RunResult{Date: "2026-06-10", Model: "test", Results: []ArmResult{
		{Arm: "dsc", TaskID: "a", Repeat: 1, Resolved: true,
			Turns: []TurnUsage{
				{HitTokens: 800, MissTokens: 200, OutTokens: 10,
					Attribution: &TokenAttribution{ToolResult: 500, AssistantText: 200, Reasoning: 100, System: 50}},
				{HitTokens: 850, MissTokens: 150, OutTokens: 10,
					Attribution: &TokenAttribution{ToolResult: 300, AssistantText: 150, Reasoning: 80, System: 40}},
			}},
		{Arm: "reasonix", TaskID: "a", Repeat: 1, Resolved: true,
			Turns: []TurnUsage{
				{HitTokens: 700, MissTokens: 300, OutTokens: 10,
					Attribution: &TokenAttribution{ToolResult: 400, AssistantText: 100, Reasoning: 50, System: 30}},
			}},
	}}
	md := RenderReport(rr)

	// Check the attribution section header exists.
	if !strings.Contains(md, "## Token attribution") {
		t.Fatalf("report missing Token attribution section:\n%s", md)
	}
	// Check column headers.
	for _, col := range []string{"tool_result", "assistant_text", "reasoning", "system"} {
		if !strings.Contains(md, "| "+col+" |") {
			t.Fatalf("report missing %q column header:\n%s", col, md)
		}
	}
	// dsc aggregated: tool_result=500+300=800, assistant_text=200+150=350,
	// reasoning=100+80=180, system=50+40=90.
	for _, want := range []string{"| 800 |", "| 350 |", "| 180 |", "| 90 |"} {
		if !strings.Contains(md, want) {
			t.Fatalf("report missing aggregated dsc attribution value %q:\n%s", want, md)
		}
	}
	// reasonix: tool_result=400, assistant_text=100, reasoning=50, system=30.
	if !strings.Contains(md, "| 400 |") {
		t.Fatalf("report missing reasonix tool_result 400:\n%s", md)
	}
}

func TestReportOmitsAttributionSectionWhenEmpty(t *testing.T) {
	rr := RunResult{Date: "2026-06-10", Model: "test", Results: []ArmResult{
		{Arm: "dsc", TaskID: "a", Repeat: 1, Resolved: true,
			Turns: []TurnUsage{{HitTokens: 800, MissTokens: 200, OutTokens: 10}}},
	}}
	md := RenderReport(rr)
	if strings.Contains(md, "## Token attribution") {
		t.Fatalf("report should not have Token attribution section when no attribution data:\n%s", md)
	}
}
