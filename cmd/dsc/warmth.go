package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// warmthRecord is the on-disk sidecar that persists the static-prefix
// fingerprint and last-used timestamp across sessions. It lives at
// <dir>/.deepseek/prefix_warmth.json and is written with 0644 perms.
type warmthRecord struct {
	Fingerprint  string `json:"fingerprint"`
	LastUsedUnix int64  `json:"last_used_unix"`
}

// warmthPath returns the canonical sidecar path for a project root.
func warmthPath(dir string) string {
	return filepath.Join(dir, ".deepseek", "prefix_warmth.json")
}

// loadWarmth reads the warmth sidecar from <dir>/.deepseek/prefix_warmth.json.
// A missing or unreadable file returns a zero warmthRecord (no error), so a
// failed load never breaks startup.
func loadWarmth(dir string) warmthRecord {
	data, err := os.ReadFile(warmthPath(dir))
	if err != nil {
		return warmthRecord{}
	}
	var rec warmthRecord
	if err := json.Unmarshal(data, &rec); err != nil {
		return warmthRecord{}
	}
	return rec
}

// saveWarmth writes fp and nowUnix to <dir>/.deepseek/prefix_warmth.json,
// creating the directory with mode 0755 if needed. Errors are returned to the
// caller; the caller must treat them as best-effort (never break startup).
func saveWarmth(dir, fp string, nowUnix int64) error {
	rec := warmthRecord{Fingerprint: fp, LastUsedUnix: nowUnix}
	data, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	dotDir := filepath.Join(dir, ".deepseek")
	if err := os.MkdirAll(dotDir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(warmthPath(dir), data, 0o644)
}
