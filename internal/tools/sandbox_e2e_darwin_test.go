//go:build darwin

package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/amemiya02/deepseekcode/internal/sandbox"
)

func TestSandboxE2ESeatbeltDenied(t *testing.T) {
	sb := sandbox.Detect()
	if sb.Name() != "seatbelt" || !sb.Available() {
		t.Skip("seatbelt sandbox not available")
	}
	result, err := (Bash{
		Sandbox:        sb,
		SandboxProfile: sandbox.Profile{AllowReadPaths: []string{"/tmp"}},
	}).Execute(context.Background(), json.RawMessage(`{"command":"cat /etc/master.passwd"}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !result.IsError {
		t.Fatalf("IsError = false, want true; content=%q", result.Content)
	}
	if !strings.HasPrefix(result.Content, "sandboxed: blocked by seatbelt") {
		t.Fatalf("Content = %q, want sandboxed prefix", result.Content)
	}
}
