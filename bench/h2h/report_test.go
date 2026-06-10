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
