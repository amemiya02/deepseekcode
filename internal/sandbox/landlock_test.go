//go:build linux

package sandbox

import (
	"bytes"
	"context"
	"os/exec"
	"strings"
	"testing"
)

func TestLandlockWrap(t *testing.T) {
	sb := landlock{}
	cmd := exec.Command("sh", "-c", "echo hi")
	if err := sb.Wrap(context.Background(), Profile{AllowReadPaths: []string{"/tmp"}}, cmd); err != nil {
		t.Fatalf("Wrap() error = %v", err)
	}
	if cmd.Path != "/proc/self/exe" {
		t.Fatalf("cmd.Path = %q, want /proc/self/exe", cmd.Path)
	}
	wantPrefix := []string{"dsc", "__sandbox_run", "--", "sh"}
	if len(cmd.Args) < len(wantPrefix) {
		t.Fatalf("cmd.Args too short: %#v", cmd.Args)
	}
	for i, want := range wantPrefix {
		if cmd.Args[i] != want {
			t.Fatalf("cmd.Args[%d] = %q, want %q; args=%#v", i, cmd.Args[i], want, cmd.Args)
		}
	}
	if !envContains(cmd.Env, sandboxProfileEnv+"=") {
		t.Fatalf("cmd.Env missing %s", sandboxProfileEnv)
	}
}

func TestLandlockWrapIdempotent(t *testing.T) {
	sb := landlock{}
	cmd := exec.Command("sh", "-c", "echo hi")
	if err := sb.Wrap(context.Background(), Profile{}, cmd); err != nil {
		t.Fatalf("first Wrap() error = %v", err)
	}
	args := append([]string(nil), cmd.Args...)
	if err := sb.Wrap(context.Background(), Profile{}, cmd); err != nil {
		t.Fatalf("second Wrap() error = %v", err)
	}
	if strings.Join(args, "\x00") != strings.Join(cmd.Args, "\x00") {
		t.Fatalf("Wrap not idempotent: %#v -> %#v", args, cmd.Args)
	}
}

func TestLandlockRunAndDeny(t *testing.T) {
	sb := landlock{}
	if !sb.Available() {
		t.Skip("landlock not available")
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

	deny := exec.Command("sh", "-c", "cat /etc/shadow")
	out.Reset()
	deny.Stdout = &out
	deny.Stderr = &out
	if err := sb.Wrap(context.Background(), Profile{AllowReadPaths: []string{"/tmp"}}, deny); err != nil {
		t.Fatalf("Wrap() error = %v", err)
	}
	_ = deny.Run()
	if !sb.WasDenied(out.String()) {
		t.Fatalf("WasDenied(%q) = false, want true", out.String())
	}
}

func envContains(env []string, prefix string) bool {
	for _, item := range env {
		if strings.HasPrefix(item, prefix) {
			return true
		}
	}
	return false
}
