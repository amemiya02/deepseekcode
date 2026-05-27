//go:build !windows

package tools

import (
	"context"
	"encoding/json"
	"os/exec"
	"strings"
	"testing"

	"github.com/amemiya02/deepseekcode/internal/sandbox"
)

type fakeSandbox struct {
	name      string
	available bool
	denied    string
	wrapped   bool
}

func (f *fakeSandbox) Name() string {
	if f.name == "" {
		return "fake"
	}
	return f.name
}

func (f *fakeSandbox) Available() bool { return f.available }

func (f *fakeSandbox) Wrap(ctx context.Context, p sandbox.Profile, cmd *exec.Cmd) error {
	f.wrapped = true
	return nil
}

func (f *fakeSandbox) WasDenied(stderr string) bool {
	return f.denied != "" && strings.Contains(stderr, f.denied)
}

func TestBashSandboxNilPassThrough(t *testing.T) {
	result, err := (Bash{}).Execute(context.Background(), json.RawMessage(`{"command":"echo hi"}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.IsError || !strings.Contains(result.Content, "hi") {
		t.Fatalf("Execute() = %#v, want successful hi", result)
	}
}

func TestBashSandboxUnavailableFailOpen(t *testing.T) {
	sb := &fakeSandbox{available: false}
	result, err := (Bash{Sandbox: sb}).Execute(context.Background(), json.RawMessage(`{"command":"echo hi"}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.IsError {
		t.Fatalf("Execute() IsError = true, want false")
	}
	if !strings.Contains(result.Content, "(sandbox unavailable; running unsandboxed)") {
		t.Fatalf("missing sandbox unavailable notice: %q", result.Content)
	}
	if !strings.Contains(result.Content, "hi") {
		t.Fatalf("missing command output: %q", result.Content)
	}
}

func TestBashSandboxWrapAndClassifyDenied(t *testing.T) {
	sb := &fakeSandbox{available: true, name: "seatbelt", denied: "Operation not permitted"}
	result, err := (Bash{Sandbox: sb}).Execute(context.Background(), json.RawMessage(`{"command":"printf 'Operation not permitted'; exit 1"}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !sb.wrapped {
		t.Fatal("sandbox Wrap was not called")
	}
	if !result.IsError {
		t.Fatalf("Execute() IsError = false, want true")
	}
	want := "sandboxed: blocked by seatbelt\nOperation not permitted"
	if result.Content != want {
		t.Fatalf("Content = %q, want %q", result.Content, want)
	}
}

func TestClassifyBlocked(t *testing.T) {
	blocked, body := classifyBlocked(nil, "anything")
	if blocked || body != "anything" {
		t.Fatalf("classifyBlocked(nil) = (%v, %q), want (false, anything)", blocked, body)
	}

	sb := &fakeSandbox{available: true, name: "seatbelt", denied: "Operation not permitted"}
	blocked, body = classifyBlocked(sb, "Operation not permitted")
	if !blocked || body != "sandboxed: blocked by seatbelt\nOperation not permitted" {
		t.Fatalf("classifyBlocked(denied) = (%v, %q)", blocked, body)
	}
}
