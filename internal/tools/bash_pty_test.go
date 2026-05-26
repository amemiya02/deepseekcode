//go:build !windows

package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestBashPTYName(t *testing.T) {
	var tool BashPTY
	if tool.Name() != "bash_pty" {
		t.Fatalf("expected name 'bash_pty', got %q", tool.Name())
	}
}

func TestBashPTYAffectedPaths(t *testing.T) {
	var tool BashPTY
	paths := tool.AffectedPaths(nil)
	if paths != nil {
		t.Fatalf("expected nil AffectedPaths, got %v", paths)
	}
}

func TestBashPTYEcho(t *testing.T) {
	var tool BashPTY
	ctx := context.Background()
	result, err := tool.Execute(ctx, mustMarshal(map[string]any{
		"command": "echo hello",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected IsError: %v", result)
	}
	// Normalize \r\n to \n for comparison
	normalized := strings.ReplaceAll(result.Content, "\r\n", "\n")
	if !strings.Contains(normalized, "hello") {
		t.Fatalf("expected output to contain 'hello', got %q", result.Content)
	}
}

func TestBashPTYExitCode(t *testing.T) {
	var tool BashPTY
	ctx := context.Background()
	result, err := tool.Execute(ctx, mustMarshal(map[string]any{
		"command": "exit 3",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatalf("expected IsError=true, got false")
	}
	if !strings.Contains(result.Content, "exit status 3") {
		t.Fatalf("expected content to contain 'exit status 3', got %q", result.Content)
	}
}

func TestBashPTYTimeout(t *testing.T) {
	var tool BashPTY
	ctx := context.Background()
	result, err := tool.Execute(ctx, mustMarshal(map[string]any{
		"command":    "sleep 10",
		"timeout_ms": 200,
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatalf("expected IsError=true for timeout, got false")
	}
	if !strings.Contains(result.Content, "timed out") {
		t.Fatalf("expected content to contain 'timed out', got %q", result.Content)
	}
}

func TestBashPTYEmptyCommand(t *testing.T) {
	var tool BashPTY
	ctx := context.Background()
	result, err := tool.Execute(ctx, mustMarshal(map[string]any{
		"command": "",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatalf("expected IsError=true for empty command, got false")
	}
	if !strings.Contains(result.Content, "command is required") {
		t.Fatalf("expected content to contain 'command is required', got %q", result.Content)
	}
}

func mustMarshal(v any) []byte {
	data, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return data
}
