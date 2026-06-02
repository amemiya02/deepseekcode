// Package taubench — the Reasonix arm.
//
// RunReasonixArm shells out to Reasonix's own tau-bench runner:
//
//	npx tsx benchmarks/tau-bench/runner.ts --task <id> --mode reasonix --repeats 1
//
// in the Reasonix repository directory, captures stdout, and parses the
// BenchReport JSON into a []RunResult. The design is faithful to plan 4 §7:
// "shell to their runner, parse JSON → RunResult."
//
// The JSON parser (parseReasonixReport / buildReasonixCmd) is the only logic
// tested in the unit suite; live exec is gated behind RunReasonixArmLive, which
// is reached solely from the -live CLI flow.
package main

import (
	"context"
	"encoding/json"
	"fmt"
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

// buildReasonixCmd constructs the exec.Cmd that shells to Reasonix's runner.
// The command is:
//
//	npx tsx benchmarks/tau-bench/runner.ts --task <taskID> --mode reasonix --repeats 1
//
// run with Dir set to repoDir (the Reasonix repository root). ctx is wired
// directly so a cancelled/timed-out benchmark loop can abort the subprocess
// without leaking it. The caller is responsible for Cmd.Output() / Cmd.Run().
func buildReasonixCmd(ctx context.Context, repoDir, taskID string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "npx", "tsx", "benchmarks/tau-bench/runner.ts",
		"--task", taskID,
		"--mode", "reasonix",
		"--repeats", "1",
	)
	cmd.Dir = repoDir
	return cmd
}

// RunReasonixArmLive runs a single tau-bench task through Reasonix's own
// runner and returns the parsed results. It is the only function that actually
// executes the subprocess; unit tests and `go build` never call it.
//
// repoDir must point to the root of the Reasonix repository (the directory
// that contains benchmarks/tau-bench/runner.ts and a valid package.json).
func RunReasonixArmLive(ctx context.Context, repoDir, taskID string) ([]RunResult, error) {
	cmd := buildReasonixCmd(ctx, repoDir, taskID)

	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("reasonix runner [task=%s]: %w", taskID, err)
	}

	results, err := parseReasonixReport(out)
	if err != nil {
		return nil, fmt.Errorf("reasonix runner [task=%s] parse: %w", taskID, err)
	}
	return results, nil
}
