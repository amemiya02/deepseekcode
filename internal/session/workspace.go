// Package session implements the SQLite-backed conversation store.
package session

import (
	"hash/fnv"
	"path/filepath"
)

// Fingerprint returns a stable FNV-1a hash of the canonical project path.
// Symlinks in the path are resolved so two symlinked paths produce the
// same fingerprint.
func Fingerprint(projectPath string) (string, error) {
	abs, err := filepath.Abs(projectPath)
	if err != nil {
		return "", err
	}
	real, err := filepath.EvalSymlinks(abs)
	if err != nil {
		real = abs // fallback: use unresolved path
	}
	h := fnv.New64a()
	h.Write([]byte(real))
	return hexEncode(h.Sum(nil)), nil
}

func hexEncode(b []byte) string {
	const hex = "0123456789abcdef"
	out := make([]byte, len(b)*2)
	for i, v := range b {
		out[i*2] = hex[v>>4]
		out[i*2+1] = hex[v&0x0f]
	}
	return string(out)
}
