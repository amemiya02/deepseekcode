//go:build linux

package sandbox

import (
	"fmt"
	"os"
	"testing"
)

// TestMain intercepts the "__sandbox_run" re-exec path. When the landlock
// integration test wraps a command, cmd.Path is set to /proc/self/exe and
// the args begin with "__sandbox_run". Under `go test`, /proc/self/exe is
// the test binary itself, so without this dispatcher the child would re-run
// the entire test suite recursively — fork-bombing the CI runner.
func TestMain(m *testing.M) {
	if len(os.Args) > 1 && os.Args[1] == "__sandbox_run" {
		if err := RunSandboxedChild(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "sandbox test child:", err)
			os.Exit(1)
		}
		return
	}
	os.Exit(m.Run())
}
