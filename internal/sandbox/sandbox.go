package sandbox

import (
	"context"
	"os/exec"
)

// Profile describes the filesystem and network access a sandboxed command
// should receive. AllowExecPaths is used by Landlock; other implementations
// may ignore it.
type Profile struct {
	AllowReadPaths  []string
	AllowWritePaths []string
	AllowNetwork    bool
	AllowExecPaths  []string
}

// Sandbox wraps commands with an OS-native sandbox implementation.
type Sandbox interface {
	Name() string
	Available() bool
	Wrap(ctx context.Context, profile Profile, cmd *exec.Cmd) error
	WasDenied(stderr string) bool
}

// Detect returns the default sandbox for the current OS.
func Detect() Sandbox {
	sb := defaultSandbox()
	if sb == nil {
		return noopSandbox{}
	}
	return sb
}

type noopSandbox struct{}

func (noopSandbox) Name() string { return "noop" }

func (noopSandbox) Available() bool { return true }

func (noopSandbox) Wrap(context.Context, Profile, *exec.Cmd) error { return nil }

func (noopSandbox) WasDenied(string) bool { return false }
