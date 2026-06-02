// Command taubench runs the tau-bench-lite harness across one or more arms
// and reports cost-per-solved-task.
//
// Usage:
//
//	go run ./bench/cmd/taubench [flags]
//
// Flags:
//
//	-tasks         comma-separated task IDs to run (default: all 8)
//	-repeats       repeats per task per arm (default: 2)
//	-arms          comma-separated arm names (default: "dsc,reasonix-flash")
//	-out           path to write the JSON report (default: stdout only)
//	-dry           wire everything and run offline without API calls; exits 0
//	-reasonix-dir  Reasonix repo root, required for reasonix-* arms
//	-base-url      DeepSeek API base URL (dsc arm)
//	-run-timeout   per-run timeout in seconds
//
// The -dry flag mirrors Reasonix's --dry: it calls RunDry(), aggregates the
// synthetic results, prints the table, and optionally writes JSON to -out.
//
// The live path dispatches each (arm, task, repeat) through runOne: the "dsc"
// arm runs dsc's real in-process agent loop (RunDSCArmLive); any "reasonix*"
// arm shells to Reasonix's own runner (RunReasonixArmLive). Each run is bounded
// by -run-timeout (released via defer in runOne). The arm label is stamped onto
// every RunResult.Mode so the report aggregates per arm.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/amemiya02/deepseekcode/internal/llm"
)

func main() {
	tasksFlag := flag.String("tasks", "", "comma-separated task IDs (default: all)")
	repeatsFlag := flag.Int("repeats", 2, "repeats per task per arm")
	armsFlag := flag.String("arms", "dsc,reasonix-flash", "comma-separated arm names")
	outFlag := flag.String("out", "", "path to write JSON report (empty = stdout only)")
	dryFlag := flag.Bool("dry", false, "offline dry-run: wire everything, no API calls")
	reasonixDirFlag := flag.String("reasonix-dir", "", "Reasonix repo root (required for reasonix-* arms)")
	baseURLFlag := flag.String("base-url", "https://api.deepseek.com", "DeepSeek API base URL (dsc arm)")
	runTimeoutFlag := flag.Int("run-timeout", 240, "per-run timeout in seconds")
	flag.Parse()

	if *repeatsFlag < 1 {
		log.Fatalf("-repeats must be >= 1, got %d", *repeatsFlag)
	}

	arms := splitTrimmed(*armsFlag)
	if len(arms) == 0 {
		log.Fatal("-arms must name at least one arm")
	}

	var taskFilter []string
	if *tasksFlag != "" {
		taskFilter = splitTrimmed(*tasksFlag)
	}
	tasks := resolveTasks(taskFilter)
	if len(tasks) == 0 {
		log.Fatal("no matching tasks found")
	}

	ctx := context.Background()
	var results []RunResult

	if *dryFlag {
		dry, err := RunDry()
		if err != nil {
			log.Fatalf("dry run: %v", err)
		}
		results = dry
	} else {
		apiKey := os.Getenv("DEEPSEEK_API_KEY")
		if apiKey == "" {
			log.Fatal("DEEPSEEK_API_KEY is not set; use -dry for an offline smoke test")
		}
		// Validate every arm up front so a typo fails fast (before any spend)
		// and reasonix arms have a repo dir.
		for _, arm := range arms {
			switch {
			case arm == "dsc":
			case strings.HasPrefix(arm, "reasonix"):
				if *reasonixDirFlag == "" {
					log.Fatalf("-reasonix-dir must be set for arm %q", arm)
				}
			default:
				log.Fatalf("unknown arm %q (want dsc or reasonix-*)", arm)
			}
		}
		client := llm.NewClient(apiKey, *baseURLFlag)
		cwd, err := os.Getwd()
		if err != nil {
			log.Fatalf("getwd: %v", err)
		}
		perRun := time.Duration(*runTimeoutFlag) * time.Second

		for _, arm := range arms {
			for _, task := range tasks {
				for r := 0; r < *repeatsFlag; r++ {
					results = append(results, runOne(ctx, arm, task, client, cwd, *reasonixDirFlag, perRun)...)
					fmt.Fprintf(os.Stderr, "[%s] %s repeat %d/%d done\n", arm, task.ID, r+1, *repeatsFlag)
				}
			}
		}
	}

	report := AggregateReport(results)
	fmt.Print(RenderTable(report))

	if *outFlag != "" {
		if err := WriteReportJSON(*outFlag, report); err != nil {
			log.Fatalf("write report: %v", err)
		}
		fmt.Fprintf(os.Stderr, "report written to %s\n", *outFlag)
	}
}

// runOne executes a single (arm, task) run under a per-run timeout and returns
// the resulting RunResult(s) with Mode stamped to the arm label. The timeout
// context is released via defer at the end of THIS function — extracting the
// per-run body out of the loop is what makes `defer cancel()` correct (a
// defer in the loop body would otherwise pile up until main returns).
//
// Arms are validated by the caller before the loop, so the default branch here
// is defensive only.
func runOne(parent context.Context, arm string, task TaskDef, client *llm.Client, cwd, reasonixDir string, perRun time.Duration) []RunResult {
	rctx, cancel := context.WithTimeout(parent, perRun)
	defer cancel()

	switch {
	case arm == "dsc":
		rr, runErr := RunDSCArmLive(rctx, client, task, cwd)
		rr.TaskID = task.ID
		rr.Mode = arm
		if runErr != nil && rr.ErrorMessage == "" {
			rr.ErrorMessage = runErr.Error()
		}
		return []RunResult{rr}
	case strings.HasPrefix(arm, "reasonix"):
		// Pin the model so the harness comparison holds the tier constant:
		// reasonix-pro -> pro, everything else (reasonix-flash) -> flash.
		model := "deepseek-v4-flash"
		if strings.Contains(arm, "pro") {
			model = "deepseek-v4-pro"
		}
		rrs, runErr := RunReasonixArmLive(rctx, reasonixDir, task.ID, model)
		if runErr != nil {
			return []RunResult{{TaskID: task.ID, Mode: arm, ErrorMessage: runErr.Error()}}
		}
		for i := range rrs {
			rrs[i].Mode = arm
		}
		return rrs
	default:
		return []RunResult{{TaskID: task.ID, Mode: arm, ErrorMessage: "unknown arm " + arm}}
	}
}

// splitTrimmed splits s on commas and trims whitespace, dropping empty tokens.
func splitTrimmed(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// resolveTasks returns the TaskDef slice matching the given IDs.
// An empty filter returns all tasks.
func resolveTasks(filter []string) []TaskDef {
	if len(filter) == 0 {
		return Tasks
	}
	keep := map[string]bool{}
	for _, id := range filter {
		keep[id] = true
	}
	var out []TaskDef
	for _, t := range Tasks {
		if keep[t.ID] {
			out = append(out, t)
		}
	}
	return out
}
