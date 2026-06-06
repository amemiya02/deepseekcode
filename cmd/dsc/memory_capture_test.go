package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/amemiya02/deepseekcode/internal/hooks"
)

// TestOpenMemoryCaptureBuiltin covers the two code paths in
// openMemoryCaptureBuiltin: a write-protected directory that prevents store
// creation (→ nil), and a writable temp dir where the store can be opened
// (→ non-nil closure that returns "continue").
func TestOpenMemoryCaptureBuiltin(t *testing.T) {
	t.Run("returns nil when store cannot be created", func(t *testing.T) {
		// Make a directory whose .deepseek child cannot be written.
		cwd := t.TempDir()
		blocked := filepath.Join(cwd, ".deepseek")
		if err := os.MkdirAll(blocked, 0o755); err != nil {
			t.Fatal(err)
		}
		// Remove write permission so MkdirAll succeeds but NewJSONLStore fails.
		if err := os.Chmod(blocked, 0o444); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(blocked, 0o755) })

		// On some CI environments the process runs as root and can still write
		// to a 0444 directory; skip rather than fail in that case.
		probe := filepath.Join(blocked, ".probe")
		if f, err := os.Create(probe); err == nil {
			f.Close()
			_ = os.Remove(probe)
			t.Skip("process can write to mode-0444 dir (root?); skipping permission test")
		}

		got := openMemoryCaptureBuiltin(cwd)
		if got != nil {
			t.Errorf("expected nil when store unavailable, got non-nil hook")
		}
	})

	t.Run("returns non-nil hook when store opens successfully", func(t *testing.T) {
		cwd := t.TempDir()

		got := openMemoryCaptureBuiltin(cwd)
		if got == nil {
			t.Fatal("expected non-nil hook, got nil")
		}

		// Verify the returned hook is callable and returns "continue".
		out, err := got(context.Background(), hooks.HookInput{Event: hooks.EventPostToolUse})
		if err != nil {
			t.Errorf("hook returned unexpected error: %v", err)
		}
		if out.Decision != "continue" {
			t.Errorf("hook decision = %q, want %q", out.Decision, "continue")
		}

		// Confirm the .deepseek directory was created (store file is lazy).
		storeDir := filepath.Join(cwd, ".deepseek")
		if _, err := os.Stat(storeDir); os.IsNotExist(err) {
			t.Errorf("expected store directory %s to exist", storeDir)
		}
	})
}
