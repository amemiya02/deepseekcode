package prompt

import (
	"runtime"
	"testing"
	"time"
)

func TestDiscoverProjectContext(t *testing.T) {
	ctx := DiscoverProjectContext("/tmp/proj")
	if ctx.CWD != "/tmp/proj" {
		t.Errorf("CWD: got %q", ctx.CWD)
	}
	if _, err := time.Parse("2006-01-02", ctx.CurrentDate); err != nil {
		t.Errorf("CurrentDate %q not YYYY-MM-DD: %v", ctx.CurrentDate, err)
	}
	if ctx.OSName != runtime.GOOS {
		t.Errorf("OSName: got %q want %q", ctx.OSName, runtime.GOOS)
	}
	if ctx.OSVersion == "" {
		t.Errorf("OSVersion empty")
	}
	if ctx.Shell == "" {
		t.Errorf("Shell empty (expected at least \"unknown\")")
	}
	// Git fields must be untouched here.
	if ctx.GitStatus != "" || ctx.GitDiff != "" || ctx.ActiveBranch != "" {
		t.Errorf("git fields should be empty until refreshed by agent: %+v", ctx)
	}
}

func TestReadShellFallsBackToUnknown(t *testing.T) {
	t.Setenv("SHELL", "")
	if got := readShell(); got != "unknown" {
		t.Errorf("readShell empty: got %q want %q", got, "unknown")
	}
}
