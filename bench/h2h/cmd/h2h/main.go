// Command h2h runs the dsc-vs-Reasonix head-to-head cache benchmark.
//
// Required env for live runs (fairness §3.3): DSC_BENCH_API_KEY and
// REASONIX_BENCH_API_KEY must belong to TWO DIFFERENT DeepSeek
// accounts (provider cache is account-scoped and persists hours).
package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/amemiya02/deepseekcode/bench/h2h"
)

func main() {
	tasksPath := flag.String("tasks", "bench/h2h/tasks.json", "task spec file")
	dscBin := flag.String("dsc", "./bin/dsc", "dsc binary")
	rxBin := flag.String("reasonix", "", "pinned reasonix binary (required for live)")
	repeats := flag.Int("repeats", 2, "repeats per task per arm")
	outDir := flag.String("out", "docs/competitive/data", "output directory")
	validate := flag.Bool("validate", false, "validate tasks.json and exit")
	goldcheck := flag.Bool("goldcheck", false, "gold-validate tasks: clone, verify tests fail at buggy commit, pass after fix")
	flag.Parse()

	tasks, err := h2h.LoadTasks(*tasksPath)
	if err != nil {
		log.Fatal(err)
	}
	if *validate {
		fmt.Printf("OK: %d tasks\n", len(tasks))
		return
	}
	if *goldcheck {
		runGoldcheck(tasks)
		return
	}
	if *rxBin == "" || os.Getenv("DSC_BENCH_API_KEY") == "" || os.Getenv("REASONIX_BENCH_API_KEY") == "" {
		log.Fatal("live run needs -reasonix plus DSC_BENCH_API_KEY and REASONIX_BENCH_API_KEY (two different accounts)")
	}
	rxHash := fileSHA256(*rxBin)
	dscCommit := gitHead()

	rr := h2h.RunResult{Date: time.Now().UTC().Format("2006-01-02"), Model: "deepseek-v4-flash",
		ReasonixSHA256: rxHash, DscCommit: dscCommit}
	ctx := context.Background()
	for _, task := range tasks {
		for rep := 1; rep <= *repeats; rep++ {
			for _, arm := range []string{"dsc", "reasonix"} {
				ws, err := h2h.NewWorkspace(mustTemp(), task) // fresh checkout per arm per repeat
				if err != nil {
					log.Printf("[%s/%s#%d] workspace: %v (recorded as DNF)", arm, task.ID, rep, err)
					rr.Results = append(rr.Results, h2h.ArmResult{Arm: arm, TaskID: task.ID, Repeat: rep, DNF: true, Err: err.Error()})
					continue
				}
				var res h2h.ArmResult
				if arm == "dsc" {
					res, _ = h2h.RunDsc(ctx, *dscBin, task, ws)
				} else {
					res, _ = h2h.RunReasonix(ctx, *rxBin, task, ws)
				}
				res.Repeat = rep
				// Enforce turn cap: if the arm exceeded the allowed
				// number of turns, mark as DNF.
				if task.TurnCap > 0 && len(res.Turns) > task.TurnCap {
					res.DNF = true
					if res.Err == "" {
						res.Err = fmt.Sprintf("turn cap exceeded: %d > %d", len(res.Turns), task.TurnCap)
					}
				}
				res.Resolved = ws.Score(task)
				rr.Results = append(rr.Results, res)
				log.Printf("[%s/%s#%d] resolved=%v hit=%.1f%% billable=%d err=%q",
					arm, task.ID, rep, res.Resolved, 100*res.HitRate(), res.Billable(), res.Err)
			}
		}
	}
	os.MkdirAll(*outDir, 0o755)
	raw, _ := json.MarshalIndent(rr, "", "  ")
	os.WriteFile(fmt.Sprintf("%s/h2h-%s.json", *outDir, rr.Date), raw, 0o644)
	os.WriteFile(fmt.Sprintf("%s/h2h-%s.md", *outDir, rr.Date), []byte(h2h.RenderReport(rr)), 0o644)
	fmt.Println(h2h.RenderReport(rr))
}

func fileSHA256(p string) string {
	b, err := os.ReadFile(p)
	if err != nil {
		log.Fatal(err)
	}
	return fmt.Sprintf("%x", sha256.Sum256(b))
}

func gitHead() string {
	out, _ := exec.Command("git", "rev-parse", "--short", "HEAD").Output()
	return strings.TrimSpace(string(out))
}

func mustTemp() string {
	d, err := os.MkdirTemp("", "h2h-*")
	if err != nil {
		log.Fatal(d, err)
	}
	return d
}

// runGoldcheck validates each task by:
//  1. Cloning the repo and checking out the buggy commit.
//  2. Verifying that the fail-to-pass tests FAIL at the buggy commit
//     (negative control -- if they pass, the task is bogus).
//  3. Applying the fix commit's test files and verifying they PASS
//     (positive control -- the gold reference for "resolved").
func runGoldcheck(tasks []h2h.TaskSpec) {
	var failed []string
	for _, task := range tasks {
		fmt.Printf("--- %s ---\n", task.ID)

		// 1. Clone and checkout buggy commit.
		ws, err := h2h.NewWorkspace(mustTemp(), task)
		if err != nil {
			log.Printf("FAIL %s: workspace: %v", task.ID, err)
			failed = append(failed, task.ID)
			continue
		}

		// 2. Negative control: tests must FAIL at the buggy commit.
		//    First checkout the test files from fix commit so they exist.
		testDir := task.TestDir
		if testDir == "" {
			testDir = "."
		}
		gitPath := strings.TrimSuffix(testDir, "/...")
		if gitPath == "" {
			gitPath = "."
		}
		cmd := exec.Command("git", "checkout", task.FixCommit, "--", gitPath)
		cmd.Dir = ws.Dir
		if out, err := cmd.CombinedOutput(); err != nil {
			log.Printf("FAIL %s: checkout fix tests: %v\n%s", task.ID, err, out)
			failed = append(failed, task.ID)
			continue
		}

		// Run tests at buggy commit (should fail).
		// Anchor patterns to avoid false matches.
		var anchored []string
		for _, p := range task.FailToPass {
			if !strings.HasPrefix(p, "^") {
				p = "^" + p
			}
			if !strings.HasSuffix(p, "$") {
				p = p + "$"
			}
			anchored = append(anchored, p)
		}
		run := strings.Join(anchored, "|")
		testCmd := exec.Command("go", "test", "-count=1", "-timeout", "10m", "-run", run, "-v", task.TestDir)
		testCmd.Dir = ws.Dir
		testCmd.Env = append(os.Environ(), "GOFLAGS=-mod=mod")
		out, err := testCmd.CombinedOutput()
		output := string(out)

		if strings.Contains(output, "no tests to run") || strings.Contains(output, "[no test files]") {
			log.Printf("FAIL %s: no tests matched pattern %q -- check fail_to_pass in tasks.json", task.ID, task.FailToPass)
			failed = append(failed, task.ID)
			continue
		}
		if err == nil {
			log.Printf("FAIL %s: tests PASS at buggy commit (expected FAIL) -- task is bogus or commit is wrong", task.ID)
			failed = append(failed, task.ID)
			continue
		}
		fmt.Printf("  NEGATIVE OK: tests fail at buggy commit\n")

		// 3. Positive control: now apply the fix (cherry-pick the fix commit).
		cpCmd := exec.Command("git", "cherry-pick", "--no-commit", task.FixCommit)
		cpCmd.Dir = ws.Dir
		if _, err := cpCmd.CombinedOutput(); err != nil {
			// Cherry-pick may conflict; try a simpler approach:
			// just checkout ALL files from fix commit except .git.
			log.Printf("  cherry-pick failed (%v), trying full checkout from fix commit", err)
			exec.Command("git", "checkout", task.FixCommit, "--", ".").Run()
		}

		testCmd2 := exec.Command("go", "test", "-count=1", "-timeout", "10m", "-run", run, "-v", task.TestDir)
		testCmd2.Dir = ws.Dir
		testCmd2.Env = append(os.Environ(), "GOFLAGS=-mod=mod")
		out2, err2 := testCmd2.CombinedOutput()
		output2 := string(out2)

		if err2 != nil || !strings.Contains(output2, "PASS") {
			log.Printf("FAIL %s: tests do not PASS after fix: %v\n%s", task.ID, err2, output2)
			failed = append(failed, task.ID)
			continue
		}
		fmt.Printf("  POSITIVE OK: tests pass after fix\n")
	}

	fmt.Printf("\n=== RESULTS ===\n")
	if len(failed) > 0 {
		log.Fatalf("GOLDCHECK FAILED: %v", failed)
	}
	fmt.Println("ALL TASKS GOLD-VALIDATED")
}
