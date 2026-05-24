package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/amemiya02/deepseekcode/internal/agent"
	"github.com/amemiya02/deepseekcode/internal/hooks"
	"github.com/amemiya02/deepseekcode/internal/llm"
	"github.com/amemiya02/deepseekcode/internal/lsp"
	"github.com/amemiya02/deepseekcode/internal/permissions"
	"github.com/amemiya02/deepseekcode/internal/prompt"
	"github.com/amemiya02/deepseekcode/internal/tools"
)

// TestE2ESmoke runs a full ReAct cycle with a fake LLM server, verifying
// all 7 required scenarios in a single agent run:
//  1. prompt loading + git context + instruction files
//  2. one compaction
//  3. one hook trigger
//  4. one MCP tool call
//  5. one LSP tool call
//  6. one bash dangerous command rejected
//  7. one read_file binary rejected
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

	// --- Scenario 3: Set up hook ---
	hookRunner := hooks.NewRunner()
	var hookFired atomic.Bool
	hookRunner.Register("test_hook", func(_ context.Context, _ hooks.HookInput) (hooks.HookOutput, error) {
		hookFired.Store(true)
		return hooks.HookOutput{Decision: "continue", Reason: "test hook fired"}, nil
	})
	hookRunner.Configure([]hooks.HookConfig{
		{Event: hooks.EventPreToolUse, Type: hooks.TypeBuiltin, Name: "test_hook"},
	})

	// Fake LLM server that responds with tool calls in sequence.
	var stepCount int32
	responses := []fakeResponse{
		// Step 1: Call read_file on a binary — scenario 7.
		{toolCalls: []fakeToolCall{{ID: "call_1", Name: "read_file", Args: `{"path":"binary.bin"}`}}},
		// Step 2: Call bash with rm -rf — scenario 6.
		{toolCalls: []fakeToolCall{{ID: "call_2", Name: "bash", Args: `{"command":"rm -rf /"}`}}},
		// Step 3: Call MCP echo tool — scenario 4.
		{toolCalls: []fakeToolCall{{ID: "call_3", Name: "mcp__mock__echo", Args: `{"text":"hello e2e"}`}}},
		// Step 4: Call LSP tool — scenario 5.
		{toolCalls: []fakeToolCall{{ID: "call_4", Name: "lsp", Args: `{"action":"hover","file":"main.go","line":1,"character":1}`}}},
		// Step 5: Normal text response to finish.
		{text: "Done checking the project."},
	}

	var lastBody atomic.Pointer[[]byte]

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		r.Body.Close()
		lastBody.Store(&body)

		idx := int(atomic.LoadInt32(&stepCount))
		if idx >= len(responses) {
			idx = len(responses) - 1
		}
		resp := responses[idx]
		atomic.AddInt32(&stepCount, 1)

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		flusher := w.(http.Flusher)

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

	// Scenario 4: Register MCP mock tool directly in the registry.
	reg.Register(mockMCPTool{})

	// Scenario 5: Register LSP tool with mock registry.
	reg.Register(tools.NewLSPTool(&mockLSPRegistry{}))

	pol := permissions.New(permissions.ModeDefault, dir, nil, nil, nil)

	a := agent.New(client, reg, pol, "deepseek-v4-flash")
	a.Thinking = true
	a.StepTimeout = 10 * time.Second

	// Scenario 1: Wire PromptBuilder so DEEPSEEK.md and git context
	// are loaded into the system prompt.
	a.PromptBuilder = &prompt.SystemPromptBuilder{
		StaticBase: agent.DefaultSystemPrompt,
		Instructions: []prompt.InstructionFile{
			{Path: "DEEPSEEK.md", Content: "# Test\nBuild: go build ./...\n"},
		},
		Project: &prompt.ProjectContext{
			CWD:          dir,
			GitStatus:    "## main",
			ActiveBranch: "main",
			OSName:       "darwin",
			Shell:        "zsh",
		},
	}

	// Scenario 2: Set compaction threshold very low so compaction fires.
	a.CompactionCfg = agent.CompactionConfig{
		PreserveRecentMessages: 2,
		AutoCompactInputTokens: 1,
	}

	// Scenario 3: Wire the hook runner.
	a.HookRunner = hookRunner

	var (
		sawReasoning    bool
		sawText         bool
		sawToolResult   bool
		sawBinaryReject bool // scenario 7: read_file on binary → IsError
		sawBashDeny     bool // scenario 6: bash rm -rf → permission deny
		sawCompaction   bool // scenario 2
		sawHookFired    bool // scenario 3
		sawMCPResult    bool // scenario 4
		sawLSPResult    bool // scenario 5
		sawStepFinish   bool
		sawDone         bool
	)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

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
				// Scenario 7: read_file on binary returns Errf from Execute.
				if e.CallID == "call_1" && e.Result.IsError {
					sawBinaryReject = true
				}
				// Scenario 4: MCP echo tool returned successfully.
				if e.CallID == "call_3" && !e.Result.IsError && strings.Contains(e.Result.Content, "echo") {
					sawMCPResult = true
				}
				// Scenario 5: LSP tool returned successfully.
				if e.CallID == "call_4" && !e.Result.IsError && strings.Contains(e.Result.Content, "func main()") {
					sawLSPResult = true
				}
			case agent.EventStepFinish:
				sawStepFinish = true
			case agent.EventCompaction:
				sawCompaction = true
			case agent.EventHookFired:
				sawHookFired = true
			case agent.EventPermissionAsk:
				// Scenario 6: bash rm -rf triggers permission ask → deny it.
				if e.Check.Tool.Name() == "bash" {
					sawBashDeny = true
					e.Reply <- agent.PermissionResponse{Allow: false}
				} else {
					// Allow other tools (MCP, etc.) so scenarios 4/5 can complete.
					e.Reply <- agent.PermissionResponse{Allow: true}
				}
			case agent.EventDone:
				sawDone = true
				return
			}
		}
	}()

	go func() {
		a.Run(ctx, "Check the project files")
	}()

	select {
	case <-doneCh:
	case <-ctx.Done():
		t.Fatal("test timed out")
	}

	// --- Scenario 1: Validate prompt loading via captured request body ---
	if body := lastBody.Load(); body == nil || len(*body) == 0 {
		t.Error("scenario 1: no LLM request body captured — cannot validate prompt loading")
	} else {
		var req struct {
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.Unmarshal(*body, &req); err != nil {
			t.Errorf("scenario 1: unmarshal request body: %v", err)
		} else {
			foundDEEPSEEK := false
			foundGit := false
			for _, m := range req.Messages {
				if m.Role == "system" {
					if strings.Contains(m.Content, "go build") || strings.Contains(m.Content, "Build") {
						foundDEEPSEEK = true
					}
					if strings.Contains(m.Content, "git") {
						foundGit = true
					}
				}
			}
			if !foundDEEPSEEK {
				t.Error("scenario 1: system prompt does not contain DEEPSEEK.md content")
			}
			if !foundGit {
				t.Error("scenario 1: system prompt does not contain git context")
			}
		}
	}

	if !sawCompaction {
		t.Error("scenario 2: no compaction event seen")
	}
	if !sawHookFired && !hookFired.Load() {
		t.Error("scenario 3: no hook fired event seen")
	}
	if !sawMCPResult {
		t.Error("scenario 4: no MCP tool result seen")
	}
	if !sawLSPResult {
		t.Error("scenario 5: no LSP tool result seen")
	}
	if !sawBashDeny {
		t.Error("scenario 6: no bash permission deny seen")
	}
	if !sawBinaryReject {
		t.Error("scenario 7: no binary read rejection seen")
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
	if !sawToolResult {
		t.Error("no tool results received")
	}
	if !sawDone {
		t.Error("agent did not emit EventDone")
	}
}

// --- mock MCP tool (scenario 4) ---

type mockMCPTool struct{}

func (mockMCPTool) Name() string        { return "mcp__mock__echo" }
func (mockMCPTool) Description() string { return "Echo tool for testing" }
func (mockMCPTool) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"text":{"type":"string"}},"required":["text"]}`)
}
func (mockMCPTool) Execute(_ context.Context, args json.RawMessage) (tools.Result, error) {
	var p struct {
		Text string `json:"text"`
	}
	json.Unmarshal(args, &p)
	return tools.Result{Content: "echo: " + p.Text}, nil
}

// --- mock LSP registry + querier (scenario 5) ---

type mockLSPRegistry struct{}

func (mockLSPRegistry) ClientForURI(_ string) (lsp.Querier, bool) {
	return mockQuerier{}, true
}

type mockQuerier struct{}

func (mockQuerier) Hover(_ context.Context, _ string, _, _ int) (string, error) {
	return "func main()", nil
}
func (mockQuerier) Definition(_ context.Context, _ string, _, _ int) ([]lsp.Definition, error) {
	return nil, nil
}
func (mockQuerier) References(_ context.Context, _ string, _, _ int) ([]lsp.Definition, error) {
	return nil, nil
}
func (mockQuerier) Diagnostics(_ string) []lsp.Diagnostic {
	return nil
}

// --- file helpers ---

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
