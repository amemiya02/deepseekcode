package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestParseReasonixResults_SampleFixture parses the committed real
// results-*.json from Reasonix (copied verbatim as testdata/reasonix-results-sample.json)
// and asserts the JSON parser maps every field into RunResult correctly.
// No network calls — this is a pure unit test against a static fixture.
func TestParseReasonixResults_SampleFixture(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "reasonix-results-sample.json"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	results, err := parseReasonixReport(data)
	if err != nil {
		t.Fatalf("parseReasonixReport: %v", err)
	}

	// The fixture has 48 results (8 tasks × 2 modes × 3 repeats).
	if len(results) != 48 {
		t.Errorf("len(results) = %d, want 48", len(results))
	}

	// Spot-check the first result (t01_address_happy / baseline).
	r0 := results[0]
	if r0.TaskID != "t01_address_happy" {
		t.Errorf("results[0].TaskID = %q, want t01_address_happy", r0.TaskID)
	}
	if r0.Mode != "baseline" {
		t.Errorf("results[0].Mode = %q, want baseline", r0.Mode)
	}
	if !r0.Pass {
		t.Errorf("results[0].Pass = false, want true")
	}
	if r0.Turns != 3 {
		t.Errorf("results[0].Turns = %d, want 3", r0.Turns)
	}
	if r0.ToolCalls != 3 {
		t.Errorf("results[0].ToolCalls = %d, want 3", r0.ToolCalls)
	}
	// cacheHitRatio from fixture: 0.4792298092351578
	if got := r0.CacheHitRatio; got < 0.479 || got > 0.480 {
		t.Errorf("results[0].CacheHitRatio = %v, want ~0.4792", got)
	}
	if r0.CostUsd <= 0 {
		t.Errorf("results[0].CostUsd = %v, want > 0", r0.CostUsd)
	}
	if r0.PromptTokens != 5609 {
		t.Errorf("results[0].PromptTokens = %d, want 5609", r0.PromptTokens)
	}
	if r0.CompletionTokens != 352 {
		t.Errorf("results[0].CompletionTokens = %d, want 352", r0.CompletionTokens)
	}
	if r0.Truncated {
		t.Errorf("results[0].Truncated = true, want false")
	}
	if r0.FinalAgentMessage == "" {
		t.Errorf("results[0].FinalAgentMessage is empty")
	}

	// Spot-check the second result (t01_address_happy / reasonix).
	r1 := results[1]
	if r1.Mode != "reasonix" {
		t.Errorf("results[1].Mode = %q, want reasonix", r1.Mode)
	}
	if r1.Turns != 2 {
		t.Errorf("results[1].Turns = %d, want 2", r1.Turns)
	}
	// cacheHitRatio from fixture: 0.8861538461538462
	if got := r1.CacheHitRatio; got < 0.886 || got > 0.887 {
		t.Errorf("results[1].CacheHitRatio = %v, want ~0.8861", got)
	}

	// claudeEquivalentUsd must be present (it's in the Reasonix RunResult schema).
	if r0.ClaudeEquivalentUsd <= 0 {
		t.Errorf("results[0].ClaudeEquivalentUsd = %v, want > 0", r0.ClaudeEquivalentUsd)
	}
}

// TestParseReasonixReport_MalformedJSON ensures the parser surfaces a clear
// error on bad input rather than silently returning an empty slice.
func TestParseReasonixReport_MalformedJSON(t *testing.T) {
	_, err := parseReasonixReport([]byte(`not json`))
	if err == nil {
		t.Fatal("expected error on malformed JSON, got nil")
	}
}

// TestParseReasonixReport_EmptyResults handles a valid BenchReport with zero
// results (e.g. --dry with no tasks).
func TestParseReasonixReport_EmptyResults(t *testing.T) {
	report := reasonixBenchReport{
		Meta:    reasonixBenchMeta{TaskCount: 0},
		Results: []RunResult{},
	}
	data, _ := json.Marshal(report)
	results, err := parseReasonixReport(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("len(results) = %d, want 0", len(results))
	}
}

// TestBuildReasonixCmd verifies that buildReasonixCmd assembles the npx
// invocation with the correct flags from the spec:
//
//	npx tsx benchmarks/tau-bench/runner.ts --task <id> --mode reasonix --repeats 1
func TestBuildReasonixCmd_Flags(t *testing.T) {
	repoDir := "/some/reasonix/repo"
	taskID := "t03_cancel_processing"
	model := "deepseek-v4-flash"
	outPath := "/tmp/reasonix-out.json"
	cmd := buildReasonixCmd(context.Background(), repoDir, taskID, model, outPath)

	if cmd.Dir != repoDir {
		t.Errorf("cmd.Dir = %q, want %q", cmd.Dir, repoDir)
	}
	args := cmd.Args
	// args[0] is the executable path; the rest are the arguments.
	wantArgs := []string{"npx", "tsx", "benchmarks/tau-bench/runner.ts",
		"--task", taskID, "--mode", "reasonix", "--model", model, "--repeats", "1", "--out", outPath}
	if len(args) != len(wantArgs) {
		t.Fatalf("len(args) = %d, want %d; got %v", len(args), len(wantArgs), args)
	}
	for i, w := range wantArgs {
		if args[i] != w {
			t.Errorf("args[%d] = %q, want %q", i, args[i], w)
		}
	}
}
