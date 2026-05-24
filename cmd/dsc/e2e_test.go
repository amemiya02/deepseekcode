package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/amemiya02/deepseekcode/internal/agent"
	"github.com/amemiya02/deepseekcode/internal/llm"
	"github.com/amemiya02/deepseekcode/internal/permissions"
	"github.com/amemiya02/deepseekcode/internal/tools"
)

// TestE2ESmoke runs a full ReAct cycle with a fake LLM server, verifying
// key scenarios: prompt loading, tool calls, permission denial, and normal
// completion.
func TestE2ESmoke(t *testing.T) {
	dir := t.TempDir()

	// Create project structure.
	mustWriteFile(t, filepath.Join(dir, "go.mod"), "module e2e.test\ngo 1.22\n")
	mustWriteFile(t, filepath.Join(dir, "main.go"), "package main\nfunc main() {}\n")
	mustWriteFile(t, filepath.Join(dir, "DEEPSEEK.md"), "# Test\nBuild: go build ./...\n")
	mustWriteFile(t, filepath.Join(dir, "binary.bin"), string([]byte{0x00, 0x01, 0x02, 0xFF}))
	mustWriteDir(t, filepath.Join(dir, ".deepseek"))

	// Initialize git for context.
	gitInit(t, dir)

	// Fake LLM server that responds with tool calls in sequence.
	var mu sync.Mutex
	stepCount := 0
	responses := []fakeResponse{
		// Step 1: Call read_file on a binary — should be rejected by file safety.
		{toolCalls: []fakeToolCall{{ID: "call_1", Name: "read_file", Args: `{"path":"binary.bin"}`}}},
		// Step 2: Call bash with rm -rf — should be blocked by permissions.
		{toolCalls: []fakeToolCall{{ID: "call_2", Name: "bash", Args: `{"command":"rm -rf /"}`}}},
		// Step 3: Normal text response to finish.
		{text: "Done checking the project."},
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		idx := stepCount
		if idx >= len(responses) {
			idx = len(responses) - 1
		}
		resp := responses[idx]
		stepCount++
		mu.Unlock()

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		flusher := w.(http.Flusher)

		// Send reasoning delta.
		writeSSEChunk(w, flusher, sseChunkOut{
			Choices: []sseChoiceOut{{Delta: sseDeltaOut{ReasoningContent: "thinking..."}}},
		})

		if len(resp.toolCalls) > 0 {
			for i, tc := range resp.toolCalls {
				writeSSEChunk(w, flusher, sseChunkOut{
					Choices: []sseChoiceOut{{Delta: sseDeltaOut{
						ToolCalls: []sseToolCallOut{{
							Index: i, ID: tc.ID, Type: "function",
							Function: sseFunctionOut{Name: tc.Name, Arguments: tc.Args},
						}},
					}}},
				})
			}
			writeSSEChunk(w, flusher, sseChunkOut{
				Choices: []sseChoiceOut{{FinishReason: "tool_calls", Delta: sseDeltaOut{}}},
			})
		} else {
			writeSSEChunk(w, flusher, sseChunkOut{
				Choices: []sseChoiceOut{{Delta: sseDeltaOut{Content: resp.text}}},
			})
			writeSSEChunk(w, flusher, sseChunkOut{
				Choices: []sseChoiceOut{{FinishReason: "stop", Delta: sseDeltaOut{}}},
			})
		}

		writeSSEChunk(w, flusher, sseChunkOut{
			Usage: &usageOut{
				PromptTokens: 100, CompletionTokens: 50, TotalTokens: 150,
				PromptCacheHitTokens: 80, PromptCacheMissTokens: 20,
			},
		})

		fmt.Fprintf(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
	defer ts.Close()

	client := llm.NewClient("test-key", ts.URL)
	client.FirstTokenTimeout = 5 * time.Second
	client.ChunkStallTimeout = 5 * time.Second
	client.MaxRetries = 0

	reg := tools.New()
	tools.RegisterBuiltins(reg, 5*1024*1024, 5*1024*1024, dir)

	pol := permissions.New(permissions.ModeDefault, dir, nil, nil, nil)

	a := agent.New(client, reg, pol, "deepseek-v4-flash")
	a.Thinking = true
	a.StepTimeout = 10 * time.Second

	var (
		sawReasoning      bool
		sawText           bool
		sawToolResult     bool
		sawPermissionDeny bool
		sawStepFinish     bool
		sawDone           bool
	)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Consume events in the background.
	doneCh := make(chan struct{})
	go func() {
		defer close(doneCh)
		for ev := range a.Events() {
			t.Logf("event: %T", ev)
			switch e := ev.(type) {
			case agent.EventReasoningDelta:
				sawReasoning = true
			case agent.EventTextDelta:
				sawText = true
			case agent.EventToolCallResult:
				sawToolResult = true
				if e.Result.IsError {
					sawPermissionDeny = true
				}
			case agent.EventStepFinish:
				sawStepFinish = true
			case agent.EventPermissionAsk:
				t.Logf("permission ask: %s", e.Check.Tool.Name())
				e.Reply <- agent.PermissionResponse{Allow: false}
			case agent.EventDone:
				sawDone = true
				return // EventDone is the last meaningful event
			}
		}
	}()

	// Run agent in a goroutine so the event consumer can drain.
	go func() {
		a.Run(ctx, "Check the project files")
	}()

	// Wait for event consumer to finish (it returns on EventDone).
	select {
	case <-doneCh:
	case <-ctx.Done():
		t.Fatal("test timed out")
	}

	if !sawReasoning {
		t.Error("no reasoning delta — prompt/thinking not loaded")
	}
	if !sawText {
		t.Error("no text delta — LLM response not processed")
	}
	if !sawStepFinish {
		t.Error("no step finish seen")
	}
	if !sawPermissionDeny {
		t.Error("no denied tool result — binary read or destructive bash should fail")
	}
	if !sawToolResult {
		t.Error("no tool results received")
	}
	if !sawDone {
		t.Error("agent did not emit EventDone")
	}
}

// --- SSE wire types for the fake server ---

type fakeResponse struct {
	toolCalls []fakeToolCall
	text      string
}

type fakeToolCall struct {
	ID   string
	Name string
	Args string
}

type sseChunkOut struct {
	ID      string         `json:"id"`
	Object  string         `json:"object"`
	Created int64          `json:"created"`
	Model   string         `json:"model"`
	Choices []sseChoiceOut `json:"choices"`
	Usage   *usageOut      `json:"usage,omitempty"`
}

type sseChoiceOut struct {
	Index        int         `json:"index"`
	Delta        sseDeltaOut `json:"delta"`
	FinishReason string      `json:"finish_reason,omitempty"`
}

type sseDeltaOut struct {
	Role             string           `json:"role,omitempty"`
	Content          string           `json:"content,omitempty"`
	ReasoningContent string           `json:"reasoning_content,omitempty"`
	ToolCalls        []sseToolCallOut `json:"tool_calls,omitempty"`
}

type sseToolCallOut struct {
	Index    int            `json:"index"`
	ID       string         `json:"id,omitempty"`
	Type     string         `json:"type,omitempty"`
	Function sseFunctionOut `json:"function"`
}

type sseFunctionOut struct {
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}

type usageOut struct {
	PromptTokens          int `json:"prompt_tokens"`
	CompletionTokens      int `json:"completion_tokens"`
	TotalTokens           int `json:"total_tokens"`
	PromptCacheHitTokens  int `json:"prompt_cache_hit_tokens"`
	PromptCacheMissTokens int `json:"prompt_cache_miss_tokens"`
}

func writeSSEChunk(w http.ResponseWriter, flusher http.Flusher, v any) {
	b, _ := json.Marshal(v)
	fmt.Fprintf(w, "data: %s\n\n", b)
	flusher.Flush()
}

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func mustWriteDir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
}

func gitInit(t *testing.T, dir string) {
	t.Helper()
	cmd := exec.Command("git", "init")
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		t.Skip("git not available, skipping e2e")
	}
	cmd = exec.Command("git", "add", ".")
	cmd.Dir = dir
	_ = cmd.Run()
	cmd = exec.Command("git", "commit", "-m", "init")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@test.com",
		"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@test.com")
	_ = cmd.Run()
}
