package h2h

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMain(m *testing.M) {
	if os.Getenv("H2H_FAKE_DSC") == "1" {
		// Fake dsc binary: ignore CLI args except -trace-jsonl <path> and working directory.
		// (a) Rewrite add.go in cwd to the fixed version
		cwd, _ := os.Getwd()
		os.WriteFile(filepath.Join(cwd, "add.go"), []byte("package fixture\n\nfunc Add(a, b int) int { return a + b }\n"), 0o644)
		// (b) Write a 2-frame usage trace to the -trace-jsonl path
		var tracePath string
		args := os.Args[1:]
		for i := 0; i < len(args); i++ {
			if args[i] == "-trace-jsonl" && i+1 < len(args) {
				tracePath = args[i+1]
				break
			}
		}
		if tracePath != "" {
			frames := []traceFrame{
				{Type: "usage", CacheHitTokens: 900, CacheMissTokens: 100, OutputTokens: 50},
				{Type: "usage", CacheHitTokens: 800, CacheMissTokens: 200, OutputTokens: 50},
			}
			var lines []string
			for _, f := range frames {
				b, _ := json.Marshal(f)
				lines = append(lines, string(b))
			}
			os.WriteFile(tracePath, []byte(strings.Join(lines, "\n")+"\n"), 0o644)
		}
		// (c) exit 0
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestE2EFakeDsc(t *testing.T) {
	// Build a fixture repo
	src, commit := makeFixtureRepo(t)
	task := TaskSpec{ID: "fix-add", Repo: src, Commit: commit, Prompt: "fix Add",
		FailToPass: []string{"TestAdd"}, TestDir: "./...", TurnCap: 5, WallclockCapMin: 5}

	// Create workspace
	ws, err := NewWorkspace(t.TempDir(), task)
	if err != nil {
		t.Fatalf("NewWorkspace: %v", err)
	}

	// Get the test binary path (which will act as fake dsc)
	binary, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}

	// Set environment variable to make the binary act as fake dsc
	origEnv := os.Getenv("H2H_FAKE_DSC")
	os.Setenv("H2H_FAKE_DSC", "1")
	defer os.Setenv("H2H_FAKE_DSC", origEnv)

	// Run with fake dsc
	res, err := RunDsc(context.Background(), binary, task, ws)
	if err != nil {
		t.Fatalf("RunDsc: %v", err)
	}

	// Verify turns
	if len(res.Turns) != 2 {
		t.Fatalf("expected 2 turns, got %d", len(res.Turns))
	}

	// Verify workspace is resolved
	if !ws.Score(task) {
		t.Fatal("expected workspace to be resolved")
	}

	// Build RunResult and verify RenderReport contains WIN GATE
	rr := RunResult{
		Date:  "2026-06-10",
		Model: "deepseek-v4-flash",
		Results: []ArmResult{
			{Arm: "dsc", TaskID: task.ID, Resolved: true, Turns: res.Turns},
			{Arm: "reasonix", TaskID: task.ID, Resolved: true, Turns: []TurnUsage{
				{HitTokens: 800, MissTokens: 200, OutTokens: 50},
				{HitTokens: 700, MissTokens: 300, OutTokens: 50},
			}},
		},
	}
	report := RenderReport(rr)
	if !strings.Contains(report, "WIN GATE") {
		t.Fatalf("report missing WIN GATE:\n%s", report)
	}

	fmt.Println("E2E test passed - fake dsc worked correctly")
}
