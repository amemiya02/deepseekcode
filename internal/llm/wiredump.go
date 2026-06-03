package llm

import (
	"fmt"
	"os"
	"path/filepath"
)

// dumpWireBody writes one turn's canonical wire body to
// dir/turn_<seq>.json (zero-padded to 4 digits). It is the on-disk capture
// behind the DEEPSEEKCODE_WIRE_DUMP knob, consumed by `dsc trace diff-body`.
// The body is written verbatim — the exact bytes sent to the provider — so a
// later byte-diff reflects precisely what the prefix cache saw. Returns the
// path written. Diagnostic only: the bytes may contain source code, so this
// runs only when WireDumpDir is set, never in production.
func dumpWireBody(dir string, seq int64, body []byte) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	name := filepath.Join(dir, fmt.Sprintf("turn_%04d.json", seq))
	if err := os.WriteFile(name, body, 0o644); err != nil {
		return "", err
	}
	return name, nil
}
