package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadWarmthMissingFile(t *testing.T) {
	dir := t.TempDir()
	w := loadWarmth(dir)
	if w.Fingerprint != "" {
		t.Fatalf("missing file: expected empty Fingerprint, got %q", w.Fingerprint)
	}
	if w.LastUsedUnix != 0 {
		t.Fatalf("missing file: expected LastUsedUnix==0, got %d", w.LastUsedUnix)
	}
}

func TestSaveAndLoadWarmthRoundTrip(t *testing.T) {
	dir := t.TempDir()
	const fp = "abc123fingerprint"
	const ts = int64(1717000000)

	if err := saveWarmth(dir, fp, ts); err != nil {
		t.Fatalf("saveWarmth: %v", err)
	}

	// File must live at <dir>/.deepseek/prefix_warmth.json
	wantPath := filepath.Join(dir, ".deepseek", "prefix_warmth.json")
	if _, err := os.Stat(wantPath); err != nil {
		t.Fatalf("expected file at %s, got: %v", wantPath, err)
	}

	got := loadWarmth(dir)
	if got.Fingerprint != fp {
		t.Fatalf("Fingerprint: want %q got %q", fp, got.Fingerprint)
	}
	if got.LastUsedUnix != ts {
		t.Fatalf("LastUsedUnix: want %d got %d", ts, got.LastUsedUnix)
	}
}
