package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/amemiya02/deepseekcode/internal/llm"
	"github.com/amemiya02/deepseekcode/internal/permissions"
	"github.com/amemiya02/deepseekcode/internal/tools"
)

// runFileSafetyAgent creates a fake SSE server that emits a single tool_call
// with the given tool name and args, then runs Agent.Run and returns the
// ToolResultBlock from the resulting Messages.
func runFileSafetyAgent(t *testing.T, dir string, toolName string, args map[string]any, maxReadBytes, maxWriteBytes int64) llm.ToolResultBlock {
	t.Helper()

	argsJSON, err := json.Marshal(args)
	if err != nil {
		t.Fatal(err)
	}
	argsStr, err := json.Marshal(string(argsJSON))
	if err != nil {
		t.Fatal(err)
	}

	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		switch n {
		case 1:
			emitSSE(w,
				`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"c1","type":"function","function":{"name":"`+toolName+`","arguments":`+string(argsStr)+`}}]}}]}`,
				`{"choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":10,"completion_tokens":2,"total_tokens":12}}`,
			)
		case 2:
			emitSSE(w,
				`{"choices":[{"index":0,"delta":{"content":"done"}}]}`,
				`{"choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":15,"completion_tokens":1,"total_tokens":16}}`,
			)
		default:
			http.Error(w, "unexpected extra call", http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	reg := tools.New()
	tools.RegisterBuiltins(reg, maxReadBytes, maxWriteBytes, dir)

	a := New(llm.NewClient("k", srv.URL), reg, permissions.New(permissions.ModeYolo, dir, nil, nil, nil), "test-model")
	a.StopWhen = []StopCondition{MaxSteps(2)}

	_, err = a.Run(context.Background(), "test")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	for _, msg := range a.Messages {
		if msg.Role == "tool" {
			for _, b := range msg.Blocks {
				if tb, ok := b.(llm.ToolResultBlock); ok {
					return tb
				}
			}
		}
	}
	t.Fatal("no ToolResultBlock found in agent Messages")
	panic("unreachable")
}

// TestE2EFileSafety validates all three file-safety categories through
// Agent.Run: symlink escape (read_file), binary read (read_file), and
// oversize write (write_file). Each must produce a ToolResultBlock with
// IsError=true and the expected error sub-string.
func TestE2EFileSafety(t *testing.T) {
	dir := t.TempDir()

	// Shared setup: outside target for symlink escape.
	outsidePath := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(outsidePath, []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	symlinkPath := filepath.Join(dir, "escape")
	if err := os.Symlink(outsidePath, symlinkPath); err != nil {
		t.Fatal(err)
	}

	// Binary file for binary detection.
	binPath := filepath.Join(dir, "bin.bin")
	if err := os.WriteFile(binPath, []byte("\x00\x01\x02"), 0o644); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name          string
		tool          string
		args          map[string]any
		maxReadBytes  int64
		maxWriteBytes int64
		want          string
	}{
		{
			name:          "symlink_escape_read",
			tool:          "read_file",
			args:          map[string]any{"path": symlinkPath},
			maxReadBytes:  0,
			maxWriteBytes: 0,
			want:          "escapes",
		},
		{
			name:          "binary_read",
			tool:          "read_file",
			args:          map[string]any{"path": binPath},
			maxReadBytes:  0,
			maxWriteBytes: 0,
			want:          "NUL byte",
		},
		{
			name:          "oversize_write",
			tool:          "write_file",
			args:          map[string]any{"path": filepath.Join(dir, "big.txt"), "content": strings.Repeat("x", 100)},
			maxReadBytes:  0,
			maxWriteBytes: 50,
			want:          "content too large",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			block := runFileSafetyAgent(t, dir, tc.tool, tc.args, tc.maxReadBytes, tc.maxWriteBytes)
			if !block.IsError {
				t.Fatalf("expected IsError=true, got false (content=%q)", block.Content)
			}
			if !strings.Contains(block.Content, tc.want) {
				t.Fatalf("expected content containing %q, got %q", tc.want, block.Content)
			}
		})
	}
}
