// Package taubench — report aggregation and JSON writer.
//
// AggregateReport groups []RunResult by Mode (arm), computes per-arm:
//   - total runs, passed runs, pass-rate
//   - total cost in USD
//   - cost-per-solved-task = totalCostUsd / passed  (0 when passed==0)
//
// The n=8 caveat ("pass-rate is statistically thin at n=8") is printed by
// RenderTable whenever a table is rendered — the spec requires it.
//
// RunDry produces synthetic []RunResult offline (no network) so the CLI's
// -dry flag can exercise the full aggregation + JSON-write path without an
// API key.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
)

// ArmSummary is the per-arm aggregated result written to the JSON report and
// rendered in the cost-per-solved-task table.
type ArmSummary struct {
	Arm              string  `json:"arm"`
	Total            int     `json:"total"`
	Passed           int     `json:"passed"`
	PassRate         float64 `json:"passRate"`
	TotalCostUsd     float64 `json:"totalCostUsd"`
	CostPerSolvedUsd float64 `json:"costPerSolvedUsd"`
}

// aggregateArm computes one ArmSummary from the results for a single arm.
// Division by zero is guarded: CostPerSolvedUsd is 0 when no tasks passed.
func aggregateArm(arm string, results []RunResult) ArmSummary {
	var passed int
	var totalCost float64
	for _, r := range results {
		totalCost += r.CostUsd
		if r.Pass {
			passed++
		}
	}
	var costPerSolved float64
	if passed > 0 {
		costPerSolved = totalCost / float64(passed)
	}
	var passRate float64
	if len(results) > 0 {
		passRate = float64(passed) / float64(len(results))
	}
	return ArmSummary{
		Arm:              arm,
		Total:            len(results),
		Passed:           passed,
		PassRate:         passRate,
		TotalCostUsd:     totalCost,
		CostPerSolvedUsd: costPerSolved,
	}
}

// AggregateReport groups results by Mode (arm name) and returns one ArmSummary
// per arm, sorted alphabetically by arm name for deterministic output.
func AggregateReport(results []RunResult) []ArmSummary {
	byArm := map[string][]RunResult{}
	for _, r := range results {
		byArm[r.Mode] = append(byArm[r.Mode], r)
	}
	arms := make([]string, 0, len(byArm))
	for a := range byArm {
		arms = append(arms, a)
	}
	sort.Strings(arms)
	summaries := make([]ArmSummary, 0, len(arms))
	for _, a := range arms {
		summaries = append(summaries, aggregateArm(a, byArm[a]))
	}
	return summaries
}

// WriteReportJSON marshals summaries as indented JSON and writes them to path.
func WriteReportJSON(path string, summaries []ArmSummary) error {
	data, err := json.MarshalIndent(summaries, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

// RenderTable renders the cost-per-solved-task table as a plain-text string.
// The n=8 caveat is always included because 8 tasks is statistically thin and
// the spec mandates it be stated alongside the pass-rate.
func RenderTable(summaries []ArmSummary) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%-20s %6s %6s %9s %14s %18s\n",
		"arm", "total", "pass", "pass%", "total_cost_$", "cost_per_solved_$")
	fmt.Fprintln(&b, strings.Repeat("-", 78))
	for _, s := range summaries {
		fmt.Fprintf(&b, "%-20s %6d %6d %8.1f%% %14.6f %18.6f\n",
			s.Arm, s.Total, s.Passed, s.PassRate*100, s.TotalCostUsd, s.CostPerSolvedUsd)
	}
	fmt.Fprintln(&b, "")
	fmt.Fprintln(&b, "Note: pass-rate is statistically thin at n=8 tasks — treat as directional only.")
	return b.String()
}

// RunDry produces synthetic []RunResult for every arm×task combination
// without making any network calls. Each dry result marks Pass=true with a
// nominal cost so the aggregation math and JSON writer can be exercised fully
// offline. This is the backend for the CLI -dry flag.
func RunDry() ([]RunResult, error) {
	arms := []string{"dsc", "reasonix-flash"}
	var results []RunResult
	for _, arm := range arms {
		for _, task := range Tasks {
			results = append(results, RunResult{
				TaskID:              task.ID,
				Mode:                arm,
				Pass:                true,
				Turns:               1,
				ToolCalls:           1,
				CacheHitRatio:       0.0,
				CostUsd:             0.0001,
				ClaudeEquivalentUsd: 0.0005,
				PromptTokens:        100,
				CompletionTokens:    20,
				Truncated:           false,
				FinalAgentMessage:   "[dry-run]",
			})
		}
	}
	return results, nil
}
