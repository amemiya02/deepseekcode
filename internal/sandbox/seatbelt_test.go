//go:build darwin

package sandbox

import (
	"bytes"
	"context"
	"os/exec"
	"strings"
	"testing"
)

func TestSeatbeltWrap(t *testing.T) {
	sb := seatbelt{}
	cmd := exec.Command("sh", "-c", "echo hi")
	if err := sb.Wrap(context.Background(), Profile{AllowReadPaths: []string{"/tmp"}}, cmd); err != nil {
		t.Fatalf("Wrap() error = %v", err)
	}
	if cmd.Path != sandboxExecPath {
		t.Fatalf("cmd.Path = %q, want %q", cmd.Path, sandboxExecPath)
	}
	if len(cmd.Args) < 5 || cmd.Args[0] != "sandbox-exec" || cmd.Args[1] != "-p" {
		t.Fatalf("cmd.Args = %#v, want sandbox-exec -p profile ...", cmd.Args)
	}
	if !strings.Contains(cmd.Args[2], "(deny default)") {
		t.Fatalf("profile missing deny default: %q", cmd.Args[2])
	}
	if got := strings.Join(cmd.Args[4:], " "); got != "-c echo hi" || !strings.HasSuffix(cmd.Args[3], "/sh") {
		t.Fatalf("wrapped command tail = %#v", cmd.Args[3:])
	}
}

func TestSeatbeltWrapIdempotent(t *testing.T) {
	sb := seatbelt{}
	cmd := exec.Command("sh", "-c", "echo hi")
	if err := sb.Wrap(context.Background(), Profile{}, cmd); err != nil {
		t.Fatalf("first Wrap() error = %v", err)
	}
	args := append([]string(nil), cmd.Args...)
	if err := sb.Wrap(context.Background(), Profile{}, cmd); err != nil {
		t.Fatalf("second Wrap() error = %v", err)
	}
	if strings.Join(cmd.Args, "\x00") != strings.Join(args, "\x00") {
		t.Fatalf("Wrap not idempotent: %#v -> %#v", args, cmd.Args)
	}
}

func TestSeatbeltRunAndDeny(t *testing.T) {
	sb := seatbelt{}
	if !sb.Available() {
		t.Skip("sandbox-exec not available")
	}

	cmd := exec.Command("sh", "-c", "echo hi")
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := sb.Wrap(context.Background(), Profile{AllowReadPaths: []string{"/tmp"}}, cmd); err != nil {
		t.Fatalf("Wrap() error = %v", err)
	}
	if err := cmd.Run(); err != nil {
		t.Fatalf("wrapped echo failed: %v; out=%q", err, out.String())
	}
	if !strings.Contains(out.String(), "hi") {
		t.Fatalf("wrapped echo output = %q, want hi", out.String())
	}

	deny := exec.Command("sh", "-c", "cat /etc/passwd")
	out.Reset()
	deny.Stdout = &out
	deny.Stderr = &out
	if err := sb.Wrap(context.Background(), Profile{}, deny); err != nil {
		t.Fatalf("Wrap() error = %v", err)
	}
	_ = deny.Run()
	if !sb.WasDenied(out.String()) {
		t.Fatalf("WasDenied(%q) = false, want true", out.String())
	}
}
