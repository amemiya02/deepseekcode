// Package taubench — the Reasonix arm.
//
// RunReasonixArmLive shells out to Reasonix's own tau-bench runner:
//
//	npx tsx benchmarks/tau-bench/runner.ts --task <id> --mode reasonix \
//	    --model <model> --repeats 1 --out <tmpfile>
//
// in the Reasonix repository directory. Reasonix writes its BenchReport to the
// FILE named by --out (printing only "wrote <path>" to stdout), so we read that
// file and parse it — NOT stdout. The model is pinned per arm so the harness
// comparison holds the underlying model constant (reasonix-flash ->
// deepseek-v4-flash, matching dsc's flash tier; reasonix-pro -> deepseek-v4-pro)
// instead of using Reasonix's default (deepseek-chat), which would confound a
// harness comparison with a model difference.
//
// The JSON parser (parseReasonixReport) and the command builder
// (buildReasonixCmd) are the only logic exercised by the unit suite; live exec
// is reached solely from the -live CLI flow.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
)

// reasonixBenchMeta mirrors the BenchMeta shape in Reasonix types.ts.
// Only the fields the Go side needs are decoded; extras are silently ignored.
type reasonixBenchMeta struct {
	Date            string `json:"date"`
	Model           string `json:"model"`
	UserSimModel    string `json:"userSimModel"`
	TaskCount       int    `json:"taskCount"`
	RepeatsPerTask  int    `json:"repeatsPerTask"`
	ReasonixVersion string `json:"reasonixVersion"`
}

// reasonixBenchReport mirrors the BenchReport shape in Reasonix types.ts.
// RunResult is reused directly: its JSON tags match the Reasonix camelCase field
// names (taskId, mode, pass, turns, toolCalls, cacheHitRatio, costUsd,
// claudeEquivalentUsd, promptTokens, completionTokens, truncated,
// finalAgentMessage, errorMessage).
type reasonixBenchReport struct {
	Meta    reasonixBenchMeta `json:"meta"`
	Results []RunResult       `json:"results"`
}

// parseReasonixReport unmarshals a BenchReport JSON blob (as written by
// Reasonix's runner.ts writeReport) into a slice of RunResult values.
// It returns an error for malformed JSON but tolerates an empty results array.
func parseReasonixReport(data []byte) ([]RunResult, error) {
	var report reasonixBenchReport
	if err := json.Unmarshal(data, &report); err != nil {
		return nil, fmt.Errorf("parse reasonix report: %w", err)
	}
	return report.Results, nil
}

// buildReasonixCmd constructs the exec.Cmd that shells to Reasonix's runner:
//
//	npx tsx benchmarks/tau-bench/runner.ts --task <taskID> --mode reasonix \
//	    --model <model> --repeats 1 --out <outPath>
//
// run with Dir set to repoDir (the Reasonix repository root). ctx is wired
// directly so a cancelled/timed-out benchmark loop can abort the subprocess
// without leaking it. The caller runs the Cmd and reads outPath.
func buildReasonixCmd(ctx context.Context, repoDir, taskID, model, outPath string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "npx", "tsx", "benchmarks/tau-bench/runner.ts",
		"--task", taskID,
		"--mode", "reasonix",
		"--model", model,
		"--repeats", "1",
		"--out", outPath,
	)
	cmd.Dir = repoDir
	return cmd
}

// RunReasonixArmLive runs a single tau-bench task through Reasonix's own runner
// on the given model and returns the parsed results. It directs the report to a
// temp file via --out and reads THAT file: Reasonix prints only "wrote <path>"
// to stdout, so parsing stdout yields no report. It is the only function that
// executes the subprocess; unit tests and `go build` never call it.
//
// repoDir must point to the root of the Reasonix repository (the directory
// containing benchmarks/tau-bench/runner.ts and a valid package.json).
func RunReasonixArmLive(ctx context.Context, repoDir, taskID, model string) ([]RunResult, error) {
	f, err := os.CreateTemp("", "reasonix-taubench-*.json")
	if err != nil {
		return nil, fmt.Errorf("reasonix runner [task=%s]: temp file: %w", taskID, err)
	}
	outPath := f.Name()
	_ = f.Close()
	defer func() { _ = os.Remove(outPath) }()

	cmd := buildReasonixCmd(ctx, repoDir, taskID, model, outPath)
	// CombinedOutput captures stderr too, so a runner crash surfaces with its
	// diagnostics rather than an opaque exit-status error.
	if out, runErr := cmd.CombinedOutput(); runErr != nil {
		return nil, fmt.Errorf("reasonix runner [task=%s]: %w\n%s", taskID, runErr, truncBytes(out, 600))
	}

	data, err := os.ReadFile(outPath)
	if err != nil {
		return nil, fmt.Errorf("reasonix runner [task=%s]: read report %s: %w", taskID, outPath, err)
	}
	results, err := parseReasonixReport(data)
	if err != nil {
		return nil, fmt.Errorf("reasonix runner [task=%s] parse: %w", taskID, err)
	}
	return results, nil
}

// truncBytes renders up to n bytes of b for error context.
func truncBytes(b []byte, n int) string {
	if len(b) > n {
		return string(b[:n]) + "…"
	}
	return string(b)
}
