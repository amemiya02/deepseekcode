//go:build !windows

package tools

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestPTYRunnerEcho(t *testing.T) {
	ctx := context.Background()
	output, exitCode, err := runPTY(ctx, "echo hello", 5*time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}
	if !containsString(output, "hello") {
		t.Fatalf("expected output to contain 'hello', got %q", output)
	}
}

func TestPTYRunnerExitCode(t *testing.T) {
	ctx := context.Background()
	output, exitCode, err := runPTY(ctx, "exit 7", 5*time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exitCode != 7 {
		t.Fatalf("expected exit code 7, got %d", exitCode)
	}
	_ = output
}

func TestPTYRunnerTimeout(t *testing.T) {
	ctx := context.Background()
	_, _, err := runPTY(ctx, "sleep 10", 200*time.Millisecond)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected DeadlineExceeded error, got %v", err)
	}
}

func TestPTYRunnerFalse(t *testing.T) {
	ctx := context.Background()
	output, exitCode, err := runPTY(ctx, "false", 5*time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exitCode != 1 {
		t.Fatalf("expected exit code 1, got %d", exitCode)
	}
	_ = output
}

func TestPTYRunnerMultiline(t *testing.T) {
	ctx := context.Background()
	output, exitCode, err := runPTY(ctx, "printf 'a\\nb\\nc'", 5*time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}
	// PTY outputs have \r\n line endings, normalize for comparison
	normalized := strings.ReplaceAll(output, "\r\n", "\n")
	expected := "a\nb\nc"
	if !strings.Contains(normalized, expected) {
		t.Fatalf("expected output to contain %q, got %q (normalized: %q)", expected, output, normalized)
	}
}

func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
