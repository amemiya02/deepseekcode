// loop.go holds helpers that sit between the CLI and the agent loop,
// handling one-shot compound operations (e.g. --compact) that need to
// drive the agent without entering interactive or one-shot prompt mode.
package agent

import (
	"context"
	"fmt"
	"io"
)

// RunCompact performs a forced compaction on a (already-constructed) Agent and
// reports the outcome to w. It is the implementation of the `dsc --compact`
// flag path.
//
// Error handling:
//   - If a.Compact returns a non-nil error the error is returned to the caller
//     (CLI will print "dsc: …" and exit 1).
//   - The compacted bool distinguishes "nothing to compact" from a real
//     compaction so the output message is accurate.
func RunCompact(ctx context.Context, a *Agent, w io.Writer) error {
	compacted, err := a.Compact(ctx)
	if err != nil {
		return fmt.Errorf("compact: %w", err)
	}
	if compacted {
		fmt.Fprintln(w, "dsc compact: conversation compacted successfully.")
	} else {
		fmt.Fprintln(w, "dsc compact: nothing to compact (transcript too short or already below threshold).")
	}
	return nil
}
