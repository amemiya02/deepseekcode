//go:build linux

package sandbox

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	stdsyscall "syscall"

	ll "github.com/landlock-lsm/go-landlock/landlock"
	llsys "github.com/landlock-lsm/go-landlock/landlock/syscall"
)

const sandboxProfileEnv = "DSC_SANDBOX_PROFILE_JSON"

type landlock struct{}

func defaultSandbox() Sandbox { return landlock{} }

func (landlock) Name() string { return "landlock" }

func (landlock) Available() bool {
	version, err := llsys.LandlockGetABIVersion()
	return err == nil && version >= 1
}

func (landlock) Wrap(ctx context.Context, p Profile, cmd *exec.Cmd) error {
	_ = ctx
	if cmd == nil {
		return fmt.Errorf("landlock: nil command")
	}
	if len(cmd.Args) > 1 && cmd.Args[1] == "__sandbox_run" {
		return nil
	}

	data, err := json.Marshal(p)
	if err != nil {
		return fmt.Errorf("landlock: marshal profile: %w", err)
	}
	if len(data) > 128*1024 {
		return fmt.Errorf("landlock: profile too large for env")
	}

	origPath := cmd.Path
	origArgs := append([]string(nil), cmd.Args[1:]...)
	cmd.Path = "/proc/self/exe"
	cmd.Args = append([]string{"dsc", "__sandbox_run", "--", origPath}, origArgs...)
	cmd.Env = upsertEnv(cmd.Env, sandboxProfileEnv, string(data))
	return nil
}

func (landlock) WasDenied(stderr string) bool {
	if stderr == "" {
		return false
	}
	lower := strings.ToLower(stderr)
	return strings.Contains(lower, "permission denied") ||
		strings.Contains(stderr, "EACCES") ||
		strings.Contains(lower, "operation not permitted")
}

// RunSandboxedChild applies the profile from DSC_SANDBOX_PROFILE_JSON and
// replaces this process with the command after "--".
func RunSandboxedChild(args []string) error {
	if len(args) < 2 || args[0] != "--" {
		return fmt.Errorf("__sandbox_run usage: __sandbox_run -- <command> [args...]")
	}

	var p Profile
	raw := os.Getenv(sandboxProfileEnv)
	if raw == "" {
		return fmt.Errorf("__sandbox_run: missing %s", sandboxProfileEnv)
	}
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		return fmt.Errorf("__sandbox_run: invalid profile: %w", err)
	}

	realPath := args[1]
	if !strings.ContainsRune(realPath, os.PathSeparator) {
		resolved, err := exec.LookPath(realPath)
		if err != nil {
			return fmt.Errorf("__sandbox_run: resolve %q: %w", realPath, err)
		}
		realPath = resolved
	}

	if err := restrictLandlock(p, realPath); err != nil {
		return fmt.Errorf("__sandbox_run: restrict: %w", err)
	}

	argv := append([]string{args[1]}, args[2:]...)
	env := removeEnv(os.Environ(), sandboxProfileEnv)
	return stdsyscall.Exec(realPath, argv, env)
}

func restrictLandlock(p Profile, realPath string) error {
	var rules []ll.Rule

	readPaths := cleanExistingPaths(append([]string{}, p.AllowReadPaths...))
	if len(readPaths) > 0 {
		rules = append(rules, ll.RODirs(readPaths...))
	}

	writePaths := cleanExistingPaths(append([]string{}, p.AllowWritePaths...))
	if len(writePaths) > 0 {
		rules = append(rules, ll.RWDirs(writePaths...))
	}

	execPaths := append([]string{}, p.AllowExecPaths...)
	if realPath != "" {
		execPaths = append(execPaths, filepath.Dir(realPath))
	}
	execPaths = append(execPaths, "/lib", "/lib64", "/usr/lib", "/usr/lib64")
	execPaths = cleanExistingPaths(execPaths)
	if len(execPaths) > 0 {
		rules = append(rules, ll.RODirs(execPaths...))
	}

	return ll.V1.RestrictPaths(rules...)
}

func cleanExistingPaths(paths []string) []string {
	seen := make(map[string]bool, len(paths))
	var out []string
	for _, path := range paths {
		if path == "" {
			continue
		}
		abs, err := filepath.Abs(path)
		if err != nil {
			continue
		}
		if _, err := os.Stat(abs); err != nil {
			continue
		}
		if !seen[abs] {
			seen[abs] = true
			out = append(out, abs)
		}
	}
	return out
}

func upsertEnv(env []string, key, value string) []string {
	if env == nil {
		env = os.Environ()
	}
	prefix := key + "="
	out := append([]string(nil), env...)
	for i, item := range out {
		if strings.HasPrefix(item, prefix) {
			out[i] = prefix + value
			return out
		}
	}
	return append(out, prefix+value)
}

func removeEnv(env []string, key string) []string {
	prefix := key + "="
	out := env[:0]
	for _, item := range env {
		if !strings.HasPrefix(item, prefix) {
			out = append(out, item)
		}
	}
	return out
}
