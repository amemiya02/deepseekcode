//go:build !windows

package agent

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/amemiya02/deepseekcode/internal/agents"
	"github.com/amemiya02/deepseekcode/internal/llm"
	"github.com/amemiya02/deepseekcode/internal/permissions"
	"github.com/amemiya02/deepseekcode/internal/tools"
	"github.com/amemiya02/deepseekcode/internal/worktree"
)

func TestE2EWorktreeSubagent(t *testing.T) {
	// Create a temporary git repo to serve as the project root.
	dir, err := os.MkdirTemp("", "worktree-e2e-*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	defer os.RemoveAll(dir)

	// Init git repo.
	runGitIn(t, dir, "init", "-b", "main")
	runGitIn(t, dir, "config", "user.email", "test@example.com")
	runGitIn(t, dir, "config", "user.name", "Test")

	// Create initial commit so HEAD exists.
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# test"), 0o644); err != nil {
		t.Fatalf("write README: %v", err)
	}
	runGitIn(t, dir, "add", "README.md")
	runGitIn(t, dir, "commit", "-m", "initial")

	// Create agent def with worktree:true and write_file access.
	agentDir := filepath.Join(dir, ".deepseek", "agent")
	if err := os.MkdirAll(agentDir, 0o755); err != nil {
		t.Fatalf("mkdir agent dir: %v", err)
	}
	agentDef := `---
description: isolated writer
worktree: true
tools: write_file
---
You write files into your isolated worktree.`
	if err := os.WriteFile(filepath.Join(agentDir, "iso.md"), []byte(agentDef), 0o644); err != nil {
		t.Fatalf("write agent def: %v", err)
	}

	// Load agent defs.
	defs, err := agents.Load(dir, "")
	if err != nil {
		t.Fatalf("Load agents: %v", err)
	}

	// Verify the agent def was loaded correctly.
	iso, ok := defs["iso"]
	if !ok {
		t.Fatalf("agent 'iso' not found in loaded defs: %v", defs)
	}
	if !iso.Worktree {
		t.Fatal("agent 'iso' Worktree should be true")
	}

	var reqCount int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&reqCount, 1)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		switch n {
		case 1:
			// Parent turn: model calls task{agent:"iso",...}
			emitSSE(w,
				`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"c1","type":"function","function":{"name":"task","arguments":"{\"agent\":\"iso\",\"description\":\"write x.txt with hi\"}"}}]}}]}`,
				`{"choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":10,"completion_tokens":2,"total_tokens":12}}`,
			)
		case 2:
			// Child turn 1: calls write_file.
			emitSSE(w,
				`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"c2","type":"function","function":{"name":"write_file","arguments":"{\"path\":\"x.txt\",\"content\":\"hi from worktree\"}"}}]}}]}`,
				`{"choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":5,"completion_tokens":2,"total_tokens":7}}`,
			)
		case 3:
			// Child turn 2: final text (after write_file succeeded).
			emitSSE(w,
				`{"choices":[{"index":0,"delta":{"content":"wrote x.txt successfully"}}]}`,
				`{"choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":5,"completion_tokens":1,"total_tokens":6}}`,
			)
		default:
			// Parent's final turn after subagent result.
			emitSSE(w,
				`{"choices":[{"index":0,"delta":{"content":"subagent completed"}}]}`,
				`{"choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":5,"completion_tokens":1,"total_tokens":6}}`,
			)
		}
	}))
	defer srv.Close()

	client := llm.NewClient("k", srv.URL)
	reg := tools.New()
	// Register builtins so write_file is available for the child.
	tools.RegisterBuiltins(reg, 1<<20, 1<<20, dir)
	pol := permissions.New(permissions.ModeYolo, dir, nil, nil, nil)
	parent := New(client, reg, pol, "test-model")
	parent.System = "sys"
	parent.StopWhen = []StopCondition{MaxSteps(5)}
	parent.MaxToolCalls = 50

	// Set up spawner with worktree support.
	wtMgr := worktree.NewManager(dir)
	wtLocks := worktree.NewBranchLock()
	spawner := &LoopSpawner{
		Client:   client,
		Parent:   parent,
		Defs:     defs,
		MaxDepth: 2,
		WT:       wtMgr,
		Locks:    wtLocks,
	}
	parent.Spawner = spawner
	reg.Register(tools.NewSubagentTool(spawner))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		_, _ = parent.Run(ctx, "use iso agent to write x.txt")
	}()

	var sawSubagentFinish bool
	var subagentSummary string

	timeout := time.After(10 * time.Second)
	for {
		select {
		case ev := <-parent.Events():
			switch e := ev.(type) {
			case EventSubagentFinish:
				sawSubagentFinish = true
				subagentSummary = e.Result.Summary
			case EventDone:
				// Continue waiting.
			}
			if sawSubagentFinish {
				goto done
			}
		case <-timeout:
			t.Fatal("timeout waiting for subagent finish")
		}
	}
done:

	if !sawSubagentFinish {
		t.Fatal("expected EventSubagentFinish")
	}
	t.Logf("subagent summary: %q", subagentSummary)

	// T-2904 acceptance: parent cwd must NOT have x.txt.
	if _, err := os.Stat(filepath.Join(dir, "x.txt")); err == nil {
		t.Fatal("parent cwd should NOT have x.txt (child must write into worktree)")
	}

	// Find the worktree and verify x.txt exists there.
	worktreesDir := filepath.Join(dir, ".deepseek", "worktrees")
	entries, err := os.ReadDir(worktreesDir)
	if err != nil {
		t.Fatalf("read worktrees dir: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("no worktree directories found")
	}

	foundWorktree := false
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		// Look inside for the iso-* subdirectory.
		subPath := filepath.Join(worktreesDir, entry.Name())
		subEntries, err := os.ReadDir(subPath)
		if err != nil {
			continue
		}
		for _, subEntry := range subEntries {
			if !subEntry.IsDir() || !strings.HasPrefix(subEntry.Name(), "iso-") {
				continue
			}
			foundWorktree = true
			wtPath := filepath.Join(subPath, subEntry.Name())
			t.Logf("found worktree: %s", wtPath)

			// Verify x.txt exists in the worktree with correct content.
			data, err := os.ReadFile(filepath.Join(wtPath, "x.txt"))
			if err != nil {
				t.Fatalf("worktree x.txt missing: %v", err)
			}
			if !strings.Contains(string(data), "hi from worktree") {
				t.Fatalf("worktree x.txt = %q, want 'hi from worktree'", string(data))
			}
			t.Logf("worktree x.txt content: %q", string(data))
			break
		}
		if foundWorktree {
			break
		}
	}
	if !foundWorktree {
		t.Fatal("worktree directory not found")
	}
}

func TestE2EWorktreeSpawnerNilWT(t *testing.T) {
	dir, err := os.MkdirTemp("", "worktree-nil-wt-*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	defer os.RemoveAll(dir)

	// Init git repo so worktree manager works (though we won't use it).
	runGitIn(t, dir, "init", "-b", "main")
	runGitIn(t, dir, "config", "user.email", "test@example.com")
	runGitIn(t, dir, "config", "user.name", "Test")
	os.WriteFile(filepath.Join(dir, "README.md"), []byte("# test"), 0o644)
	runGitIn(t, dir, "add", "README.md")
	runGitIn(t, dir, "commit", "-m", "initial")

	// Create agent def with worktree:true.
	agentDir := filepath.Join(dir, ".deepseek", "agent")
	os.MkdirAll(agentDir, 0o755)
	os.WriteFile(filepath.Join(agentDir, "iso.md"), []byte(`---
description: worker
worktree: true
tools: read_file
---
worker`), 0o644)

	defs, _ := agents.Load(dir, "")

	var reqCount int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&reqCount, 1)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		if n == 1 {
			emitSSE(w,
				`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"c1","type":"function","function":{"name":"task","arguments":"{\"agent\":\"iso\",\"description\":\"test\"}"}}]}}]}`,
				`{"choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":10,"completion_tokens":2,"total_tokens":12}}`,
			)
		} else {
			emitSSE(w,
				`{"choices":[{"index":0,"delta":{"content":"done"}}]}`,
				`{"choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":5,"completion_tokens":1,"total_tokens":6}}`,
			)
		}
	}))
	defer srv.Close()

	client := llm.NewClient("k", srv.URL)
	reg := tools.New()
	reg.Register(stubTool{name: "read_file"})
	pol := permissions.New(permissions.ModeYolo, dir, nil, nil, nil)
	parent := New(client, reg, pol, "test-model")
	parent.System = "sys"
	parent.StopWhen = []StopCondition{MaxSteps(3)}
	parent.MaxToolCalls = 50

	// WT is nil — worktree should degrade gracefully.
	spawner := &LoopSpawner{
		Client: client,
		Parent: parent,
		Defs:   defs,
		WT:     nil,
		Locks:  nil,
	}
	parent.Spawner = spawner
	reg.Register(tools.NewSubagentTool(spawner))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		_, _ = parent.Run(ctx, "use iso")
	}()

	var sawSubagentFinish bool
	timeout := time.After(10 * time.Second)
	for {
		select {
		case ev := <-parent.Events():
			switch ev.(type) {
			case EventSubagentFinish:
				sawSubagentFinish = true
			case EventDone:
			}
			if sawSubagentFinish {
				goto nilDone
			}
		case <-timeout:
			t.Fatal("timeout waiting for subagent finish")
		}
	}
nilDone:
	// Should complete without panic — worktree is gracefully disabled.
	t.Log("nil WT: worktree subagent degraded gracefully")
}

// runGitIn runs a git command in the given directory.
func runGitIn(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %s: %v", args[0], string(out), err)
	}
	return strings.TrimSpace(string(out))
}
