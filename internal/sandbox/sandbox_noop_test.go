//go:build !darwin && !linux

package sandbox

import (
	"context"
	"os/exec"
	"reflect"
	"testing"
)

func TestWrapNoopOnUnsupported(t *testing.T) {
	sb := Detect()
	if sb.Name() != "noop" {
		t.Fatalf("Detect().Name() = %q, want noop", sb.Name())
	}
	if !sb.Available() {
		t.Fatal("noop sandbox should be available")
	}

	cmd := exec.Command("echo", "x")
	path := cmd.Path
	args := append([]string(nil), cmd.Args...)
	env := append([]string(nil), cmd.Env...)

	if err := sb.Wrap(context.Background(), Profile{}, cmd); err != nil {
		t.Fatalf("Wrap() error = %v", err)
	}
	if cmd.Path != path || !reflect.DeepEqual(cmd.Args, args) || !reflect.DeepEqual(cmd.Env, env) {
		t.Fatalf("Wrap mutated command: path %q -> %q args %v -> %v env %v -> %v",
			path, cmd.Path, args, cmd.Args, env, cmd.Env)
	}
}
