// Command taubench runs the tau-bench-lite harness across one or more arms
// and reports cost-per-solved-task.
//
// Usage:
//
//	go run ./bench/cmd/taubench [flags]
//
// Flags:
//
//	-tasks    comma-separated task IDs to run (default: all 8)
//	-repeats  repeats per task per arm (default: 2)
//	-arms     comma-separated arm names (default: "dsc,reasonix-flash")
//	-out      path to write the JSON report (default: stdout only)
//	-dry      wire everything and run offline without API calls; exits 0
//
// The -dry flag mirrors Reasonix's --dry: it calls RunDry(), aggregates the
// synthetic results, prints the table, and optionally writes JSON to -out.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
)

func main() {
	tasksFlag := flag.String("tasks", "", "comma-separated task IDs (default: all)")
	repeatsFlag := flag.Int("repeats", 2, "repeats per task per arm")
	armsFlag := flag.String("arms", "dsc,reasonix-flash", "comma-separated arm names")
	outFlag := flag.String("out", "", "path to write JSON report (empty = stdout only)")
	dryFlag := flag.Bool("dry", false, "offline dry-run: wire everything, no API calls")
	flag.Parse()

	// Validate repeats.
	if *repeatsFlag < 1 {
		log.Fatalf("-repeats must be >= 1, got %d", *repeatsFlag)
	}

	// Parse arms.
	arms := splitTrimmed(*armsFlag)
	if len(arms) == 0 {
		log.Fatal("-arms must name at least one arm")
	}

	// Parse tasks (empty = all).
	var taskFilter []string
	if *tasksFlag != "" {
		taskFilter = splitTrimmed(*tasksFlag)
	}

	// Resolve the TaskDef list.
	tasks := resolveTasks(taskFilter)
	if len(tasks) == 0 {
		log.Fatal("no matching tasks found")
	}

	ctx := context.Background()
	_ = ctx // used by live paths below

	var results []RunResult
	var err error

	if *dryFlag {
		results, err = RunDry()
		if err != nil {
			log.Fatalf("dry run: %v", err)
		}
	} else {
		// Live path: gate on DEEPSEEK_API_KEY presence, then dispatch arms.
		apiKey := os.Getenv("DEEPSEEK_API_KEY")
		if apiKey == "" {
			log.Fatal("DEEPSEEK_API_KEY is not set; use -dry for an offline smoke test")
		}
		_ = tasks
		_ = arms
		_ = repeatsFlag
		log.Fatal("live run not yet implemented; use -dry for now")
	}

	// Aggregate and render.
	report := AggregateReport(results)
	fmt.Print(RenderTable(report))

	// Write JSON if -out was given.
	if *outFlag != "" {
		if err := WriteReportJSON(*outFlag, report); err != nil {
			log.Fatalf("write report: %v", err)
		}
		fmt.Fprintf(os.Stderr, "report written to %s\n", *outFlag)
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
