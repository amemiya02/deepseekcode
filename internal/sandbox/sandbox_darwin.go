//go:build darwin

package sandbox

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

const sandboxExecPath = "/usr/bin/sandbox-exec"

type seatbelt struct{}

func defaultSandbox() Sandbox { return seatbelt{} }

func (seatbelt) Name() string { return "seatbelt" }

func (seatbelt) Available() bool {
	_, err := exec.LookPath("sandbox-exec")
	return err == nil
}

func (s seatbelt) Wrap(ctx context.Context, p Profile, cmd *exec.Cmd) error {
	_ = ctx
	if cmd == nil {
		return fmt.Errorf("seatbelt: nil command")
	}
	if cmd.Path == sandboxExecPath {
		return nil
	}

	profileText := renderProfile(p)
	origPath := cmd.Path
	origArgs := append([]string(nil), cmd.Args[1:]...)

	cmd.Path = sandboxExecPath
	cmd.Args = append([]string{"sandbox-exec", "-p", profileText, origPath}, origArgs...)
	return nil
}

func (seatbelt) WasDenied(stderr string) bool {
	if stderr == "" {
		return false
	}
	for _, needle := range []string{
		"Operation not permitted",
		"deny file-read",
		"deny file-write",
		"deny network-outbound",
		"deny process-exec",
	} {
		if strings.Contains(stderr, needle) {
			return true
		}
	}
	return false
}

func renderProfile(p Profile) string {
	var b strings.Builder
	b.WriteString("(version 1)\n")
	b.WriteString("(deny default)\n")
	b.WriteString("(allow process-exec)\n")
	b.WriteString("(allow process-fork)\n")
	b.WriteString("(allow file-read*)\n")
	if !profileAllows(p.AllowReadPaths, "/etc") && !profileAllows(p.AllowReadPaths, "/private/etc") {
		b.WriteString("(deny file-read* (subpath \"/etc\"))\n")
		b.WriteString("(deny file-read* (subpath \"/private/etc\"))\n")
	}
	for _, path := range append([]string{}, p.AllowReadPaths...) {
		if abs := absPath(path); abs != "" {
			fmt.Fprintf(&b, "(allow file-read* (subpath %q))\n", abs)
		}
	}
	for _, path := range p.AllowWritePaths {
		if abs := absPath(path); abs != "" {
			fmt.Fprintf(&b, "(allow file-write* (subpath %q))\n", abs)
		}
	}
	if p.AllowNetwork {
		b.WriteString("(allow network*)\n")
	}
	b.WriteString("(allow mach-lookup)\n")
	return b.String()
}

func profileAllows(paths []string, target string) bool {
	target = absPath(target)
	for _, path := range paths {
		if absPath(path) == target {
			return true
		}
	}
	return false
}

func absPath(path string) string {
	if path == "" {
		return ""
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	return abs
}

func RunSandboxedChild(args []string) error {
	return errSandboxRunUnsupported
}
