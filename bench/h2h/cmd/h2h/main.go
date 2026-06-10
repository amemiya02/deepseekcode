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
	flag.Parse()

	tasks, err := h2h.LoadTasks(*tasksPath)
	if err != nil {
		log.Fatal(err)
	}
	if *validate {
		fmt.Printf("OK: %d tasks\n", len(tasks))
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
