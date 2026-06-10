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
				tmp := mustTemp()
				ws, err := h2h.NewWorkspace(tmp, task) // fresh checkout per arm per repeat
				if err != nil {
					os.RemoveAll(tmp)
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
				os.RemoveAll(tmp)
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

// runGoldcheck machine-validates each task:
//
//	negative control — with the canonical tests restored (test files
//	only), the F2P tests must RUN and FAIL at the buggy commit; a
//	build error or "no tests to run" does not count as failing.
//	positive control — after applying the fix commit's non-test
//	changes (the gold solution), ws.Score() itself must report
//	resolved, so goldcheck doubles as a regression test for the scorer.
func runGoldcheck(tasks []h2h.TaskSpec) {
	var failed []string
	for _, task := range tasks {
		fmt.Printf("--- %s ---\n", task.ID)
		if err := goldcheckTask(task); err != nil {
			log.Printf("FAIL %s: %v", task.ID, err)
			failed = append(failed, task.ID)
		}
	}
	fmt.Printf("\n=== RESULTS ===\n")
	if len(failed) > 0 {
		log.Fatalf("GOLDCHECK FAILED: %v", failed)
	}
	fmt.Println("ALL TASKS GOLD-VALIDATED")
}

func goldcheckTask(task h2h.TaskSpec) error {
	tmp := mustTemp()
	defer os.RemoveAll(tmp)
	ws, err := h2h.NewWorkspace(tmp, task)
	if err != nil {
		return fmt.Errorf("workspace: %w", err)
	}

	// Negative control.
	if err := ws.RestoreCanonicalTests(task); err != nil {
		return fmt.Errorf("restore canonical tests: %w", err)
	}
	output, err := ws.RunFailToPass(task)
	if strings.Contains(output, "no tests to run") || strings.Contains(output, "[no test files]") {
		return fmt.Errorf("no tests matched pattern %q -- check fail_to_pass in tasks.json", task.FailToPass)
	}
	if err == nil {
		return fmt.Errorf("tests PASS at buggy commit (expected FAIL) -- task is bogus or commit is wrong")
	}
	// Require evidence a test actually ran and failed; a compile error
	// of the fix-commit tests against buggy sources also exits non-zero
	// but proves nothing about the task.
	if !strings.Contains(output, "--- FAIL: ") {
		return fmt.Errorf("tests did not run-and-fail at buggy commit (build error?):\n%s", tail(output, 2000))
	}
	fmt.Printf("  NEGATIVE OK: tests run and fail at buggy commit\n")

	// Positive control.
	if err := ws.ApplyGoldFix(task); err != nil {
		return fmt.Errorf("apply gold fix: %w", err)
	}
	if !ws.Score(task) {
		return fmt.Errorf("Score() not resolved after gold fix -- scorer or task data broken")
	}
	fmt.Printf("  POSITIVE OK: Score()=resolved after gold fix\n")
	return nil
}

func tail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return "..." + s[len(s)-n:]
}
