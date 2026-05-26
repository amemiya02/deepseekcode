//go:build linux

package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/amemiya02/deepseekcode/internal/sandbox"
)

func TestSandboxE2ELandlockDenied(t *testing.T) {
	sb := sandbox.Detect()
	if sb.Name() != "landlock" || !sb.Available() {
		t.Skip("landlock sandbox not available")
	}
	result, err := (Bash{
		Sandbox:        sb,
		SandboxProfile: sandbox.Profile{AllowReadPaths: []string{"/tmp"}},
	}).Execute(context.Background(), json.RawMessage(`{"command":"cat /etc/shadow"}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !result.IsError {
		t.Fatalf("IsError = false, want true; content=%q", result.Content)
	}
	if !strings.HasPrefix(result.Content, "sandboxed: blocked by landlock") {
		t.Fatalf("Content = %q, want sandboxed prefix", result.Content)
	}
}
