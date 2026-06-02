package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestAggregate_CostPerSolvedTask verifies the core math:
// costPerSolvedTask = sum(CostUsd) / count(Pass==true).
// If no tasks pass, costPerSolvedTask must be 0 (not NaN/Inf).
func TestAggregate_CostPerSolvedTask(t *testing.T) {
	results := []RunResult{
		{TaskID: "t01", Mode: "dsc", Pass: true, CostUsd: 0.10},
		{TaskID: "t02", Mode: "dsc", Pass: false, CostUsd: 0.05},
		{TaskID: "t03", Mode: "dsc", Pass: true, CostUsd: 0.20},
	}
	got := aggregateArm("dsc", results)
	if got.Arm != "dsc" {
		t.Errorf("Arm = %q, want %q", got.Arm, "dsc")
	}
	if got.Total != 3 {
		t.Errorf("Total = %d, want 3", got.Total)
	}
	if got.Passed != 2 {
		t.Errorf("Passed = %d, want 2", got.Passed)
	}
	wantPass := 2.0 / 3.0
	if abs(got.PassRate-wantPass) > 1e-9 {
		t.Errorf("PassRate = %f, want %f", got.PassRate, wantPass)
	}
	wantTotal := 0.10 + 0.05 + 0.20
	if abs(got.TotalCostUsd-wantTotal) > 1e-9 {
		t.Errorf("TotalCostUsd = %f, want %f", got.TotalCostUsd, wantTotal)
	}
	// costPerSolvedTask = (0.10+0.05+0.20) / 2 = 0.175
	wantCPST := (0.10 + 0.05 + 0.20) / 2.0
	if abs(got.CostPerSolvedUsd-wantCPST) > 1e-9 {
		t.Errorf("CostPerSolvedUsd = %f, want %f", got.CostPerSolvedUsd, wantCPST)
	}
}

// TestAggregate_NoPasses verifies zero-safe division (no NaN/Inf when 0 tasks pass).
func TestAggregate_NoPasses(t *testing.T) {
	results := []RunResult{
		{TaskID: "t01", Mode: "dsc", Pass: false, CostUsd: 0.10},
	}
	got := aggregateArm("dsc", results)
	if got.Passed != 0 {
		t.Errorf("Passed = %d, want 0", got.Passed)
	}
	if got.CostPerSolvedUsd != 0 {
		t.Errorf("CostPerSolvedUsd = %f, want 0 when no tasks pass", got.CostPerSolvedUsd)
	}
}

// TestAggregate_MultiArm verifies that AggregateReport groups results by Mode
// and returns one ArmSummary per arm.
func TestAggregate_MultiArm(t *testing.T) {
	results := []RunResult{
		{TaskID: "t01", Mode: "dsc", Pass: true, CostUsd: 0.10},
		{TaskID: "t01", Mode: "reasonix-flash", Pass: true, CostUsd: 0.20},
		{TaskID: "t02", Mode: "dsc", Pass: false, CostUsd: 0.05},
		{TaskID: "t02", Mode: "reasonix-flash", Pass: true, CostUsd: 0.15},
	}
	report := AggregateReport(results)
	if len(report) != 2 {
		t.Fatalf("len(report) = %d, want 2", len(report))
	}
	byArm := map[string]ArmSummary{}
	for _, s := range report {
		byArm[s.Arm] = s
	}
	dsc := byArm["dsc"]
	if dsc.Total != 2 || dsc.Passed != 1 {
		t.Errorf("dsc: Total=%d Passed=%d, want 2/1", dsc.Total, dsc.Passed)
	}
	rx := byArm["reasonix-flash"]
	if rx.Total != 2 || rx.Passed != 2 {
		t.Errorf("reasonix-flash: Total=%d Passed=%d, want 2/2", rx.Total, rx.Passed)
	}
}

// TestWriteReportJSON verifies that WriteReportJSON writes valid JSON that
// round-trips back to []ArmSummary.
func TestWriteReportJSON(t *testing.T) {
	summaries := []ArmSummary{
		{Arm: "dsc", Total: 8, Passed: 6, PassRate: 0.75, TotalCostUsd: 0.5, CostPerSolvedUsd: 0.0833},
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "out.json")
	if err := WriteReportJSON(path, summaries); err != nil {
		t.Fatalf("WriteReportJSON: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var got []ArmSummary
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if len(got) != 1 || got[0].Arm != "dsc" {
		t.Errorf("round-trip mismatch: %+v", got)
	}
}

// TestRenderTable verifies the table output contains the n=8 caveat.
func TestRenderTable(t *testing.T) {
	summaries := []ArmSummary{
		{Arm: "dsc", Total: 8, Passed: 5, PassRate: 0.625, TotalCostUsd: 0.5, CostPerSolvedUsd: 0.1},
	}
	out := RenderTable(summaries)
	if !strings.Contains(out, "n=8") {
		t.Errorf("RenderTable output missing n=8 caveat:\n%s", out)
	}
	if !strings.Contains(out, "dsc") {
		t.Errorf("RenderTable output missing arm name:\n%s", out)
	}
}

// TestDryRunExitsZero verifies that the dry-run path wires everything without
// network calls and returns no error. This is the offline smoke test the spec
// requires: `go run ./bench/cmd/taubench -dry` must succeed.
func TestDryRunExitsZero(t *testing.T) {
	results, err := RunDry()
	if err != nil {
		t.Fatalf("RunDry() error: %v", err)
	}
	// dry run produces one result per arm×task combination
	if len(results) == 0 {
		t.Errorf("RunDry() returned 0 results, want >0")
	}
	report := AggregateReport(results)
	if len(report) == 0 {
		t.Errorf("AggregateReport returned 0 summaries")
	}
	// all dry results must have non-empty TaskID and Mode
	for i, r := range results {
		if r.TaskID == "" {
			t.Errorf("results[%d].TaskID is empty", i)
		}
		if r.Mode == "" {
			t.Errorf("results[%d].Mode is empty", i)
		}
	}
}

// abs returns the absolute value of f.
func abs(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}
