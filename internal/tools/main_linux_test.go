//go:build linux

package tools

import (
	"fmt"
	"os"
	"testing"

	"github.com/amemiya02/deepseekcode/internal/sandbox"
)

// TestMain intercepts the "__sandbox_run" re-exec path. Bash.Execute on
// Linux wraps the child command through landlock by setting cmd.Path to
// /proc/self/exe and prefixing args with "__sandbox_run". Under `go test`
// /proc/self/exe is this test binary, so without a dispatcher the child
// would re-run the entire test suite recursively (fork bomb).
func TestMain(m *testing.M) {
	if len(os.Args) > 1 && os.Args[1] == "__sandbox_run" {
		if err := sandbox.RunSandboxedChild(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "tools test child:", err)
			os.Exit(1)
		}
		return
	}
	os.Exit(m.Run())
}
