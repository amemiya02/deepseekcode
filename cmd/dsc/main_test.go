package main

import (
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestDscDoctor_Runs(t *testing.T) {
	cmd := exec.Command("go", "run", ".", "doctor")
	cmd.Dir = "."
	out, err := cmd.CombinedOutput()
	// A non-nil err here is either a build/compile failure or a runtime exit
	// (doctor exits 1 when checks fail). Distinguish them with ExitError: a
	// build failure produces an exec error that is NOT an *exec.ExitError (the
	// process never started), while a runtime exit IS one. Only the former is
	// a test-infrastructure problem worth fatalf-ing on immediately.
	if err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			t.Fatalf("build/exec error running 'dsc doctor': %v\noutput:\n%s", err, out)
		}
		// exitErr means the binary ran but exited non-zero (some checks failed).
		// That is valid runtime behaviour — fall through to check the header.
	}
	if !strings.Contains(string(out), "dsc doctor") {
		t.Fatalf("expected 'dsc doctor' header in output, got:\n%s", out)
	}
}

func TestDscInit_NonInteractive_MissingKey(t *testing.T) {
	// We need HOME to point somewhere empty so neither the secrets file nor the
	// config file from the developer's real environment can supply a key. But we
	// must NOT redirect GOMODCACHE/GOCACHE into t.TempDir() — the go toolchain
	// creates read-only files in the module cache and t.TempDir() cleanup would
	// then fail with "permission denied". Instead we resolve the real Go cache
	// dirs first and keep them in the subprocess env.
	realGoEnv := func(key string) string {
		out, err := exec.Command("go", "env", key).Output()
		if err != nil {
			return os.Getenv(key)
		}
		return strings.TrimSpace(string(out))
	}
	goModCache := realGoEnv("GOMODCACHE")
	goCache := realGoEnv("GOCACHE")
	goRoot := realGoEnv("GOROOT")

	tmp := t.TempDir()
	cmd := exec.Command("go", "run", ".", "init", "--non-interactive")
	cmd.Dir = "."
	env := []string{
		"HOME=" + tmp,
		"USERPROFILE=" + tmp, // Windows compat
		"DEEPSEEK_API_KEY=",  // explicitly empty — no env-var key
		"PATH=" + os.Getenv("PATH"),
		"GOMODCACHE=" + goModCache,
		"GOCACHE=" + goCache,
		"GOROOT=" + goRoot,
	}
	// Propagate any remaining Go tool env vars that might be needed.
	for _, k := range []string{"GOPATH", "GOENV", "GOFLAGS"} {
		if v := os.Getenv(k); v != "" {
			env = append(env, k+"="+v)
		}
	}
	cmd.Env = env
	out, _ := cmd.CombinedOutput()
	// Must mention DEEPSEEK_API_KEY in the error when the key is absent.
	if !strings.Contains(string(out), "DEEPSEEK_API_KEY") {
		t.Errorf("expected output to mention DEEPSEEK_API_KEY when key is unset, got:\n%s", out)
	}
}
