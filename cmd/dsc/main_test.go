package main

import (
	"errors"
	"net"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"
)

// TestBinary_ProxyEnvPassthrough verifies that the compiled binary actually
// routes outbound requests through DEEPSEEKCODE_PROXY. The test binds a TCP
// listener to an OS-assigned free port, captures its address, then immediately
// closes it so the port is in a connection-refused state. The binary is run
// with that address as DEEPSEEKCODE_PROXY. If the proxy transport is wired,
// the connection attempt reaches the closed port and the error output mentions
// that address. If the proxy is NOT wired, the binary would attempt a direct
// connection to api.deepseek.com instead, and the proxy address would never
// appear — which is a regression.
func TestBinary_ProxyEnvPassthrough(t *testing.T) {
	if testing.Short() {
		t.Skip("binary build skipped in short mode")
	}

	// Reserve a port via OS assignment, then immediately free it so it is in
	// connection-refused state when the binary tries to connect.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("could not reserve a free port: %v", err)
	}
	proxyAddr := ln.Addr().String() // e.g. "127.0.0.1:54321"
	ln.Close()                      // port now refuses connections

	// Build the binary into a temp dir.
	dir := t.TempDir()
	bin := dir + "/dsc"
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	if err := exec.Command("go", "build", "-o", bin, "github.com/amemiya02/deepseekcode/cmd/dsc").Run(); err != nil {
		t.Fatalf("build failed: %v", err)
	}

	// Run one-shot with the deliberately closed proxy. The binary must attempt
	// to connect through proxyAddr and fail with a connection-refused error
	// that mentions that address — proving the proxy transport is actually wired.
	// We pass a config-free HOME so no real key is loaded.
	cmd := exec.Command(bin, "-p", "hello")
	cmd.Env = append([]string{},
		"HOME="+dir,
		"USERPROFILE="+dir,
		"PATH="+os.Getenv("PATH"),
		"DEEPSEEKCODE_PROXY=http://"+proxyAddr,
		// DEEPSEEK_API_KEY is the env var the default deepseek provider reads via
		// config.ResolveSecret; it must be non-empty so onboarding is skipped.
		"DEEPSEEK_API_KEY=sk-fake-proxy-test",
	)
	// Windows: Go runtime needs SYSTEMROOT and TEMP to initialise.
	for _, k := range []string{"SYSTEMROOT", "TEMP", "COMSPEC"} {
		if v := os.Getenv(k); v != "" {
			cmd.Env = append(cmd.Env, k+"="+v)
		}
	}
	out, _ := cmd.CombinedOutput()
	outStr := string(out)

	// Sanity: the binary must have emitted a [stop: ...] line, proving it
	// actually ran a one-shot turn and reached the network path.
	// Note: runOneShot exits 0 even when the LLM call fails (the error is
	// printed inline as [stop: unknown err=...]). A vacuous success would be
	// an output that contains neither the stop line nor the proxy address.
	if !strings.Contains(outStr, "[stop:") && !strings.Contains(outStr, proxyAddr) {
		t.Errorf("binary produced no [stop:] line and no proxy address — did not reach the network path at all:\n%s", outStr)
	}
	// The binary must not panic.
	if strings.Contains(outStr, "panic") {
		t.Errorf("binary panicked with DEEPSEEKCODE_PROXY set:\n%s", outStr)
	}
	// The error must reference the proxy address, proving the proxy transport
	// was used. A direct connection to api.deepseek.com would never mention
	// the local proxy address.
	if !strings.Contains(outStr, proxyAddr) {
		t.Errorf("expected error output to mention proxy address %s (proxy transport not wired?):\n%s",
			proxyAddr, outStr)
	}
}

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
