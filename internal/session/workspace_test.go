package session

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFingerprint(t *testing.T) {
	// Same input → same output.
	fp1, err := Fingerprint("/tmp/test-project")
	if err != nil {
		t.Fatal(err)
	}
	fp2, err := Fingerprint("/tmp/test-project")
	if err != nil {
		t.Fatal(err)
	}
	if fp1 != fp2 {
		t.Errorf("same path should produce same fp: %q vs %q", fp1, fp2)
	}
	if len(fp1) != 16 { // fnv.New64a produces 8 bytes → 16 hex chars
		t.Errorf("expected 16 hex chars, got %d", len(fp1))
	}
}

func TestFingerprintDifferent(t *testing.T) {
	fp1, _ := Fingerprint("/tmp/a")
	fp2, _ := Fingerprint("/tmp/b")
	if fp1 == fp2 {
		t.Error("different paths should produce different fps")
	}
}

func TestFingerprintSymlinkEquivalent(t *testing.T) {
	dir := t.TempDir()
	realDir := filepath.Join(dir, "real")
	if err := os.MkdirAll(realDir, 0o755); err != nil {
		t.Fatal(err)
	}
	linkDir := filepath.Join(dir, "link")
	if err := os.Symlink(realDir, linkDir); err != nil {
		t.Fatal(err)
	}

	fpReal, err := Fingerprint(realDir)
	if err != nil {
		t.Fatal(err)
	}
	fpLink, err := Fingerprint(linkDir)
	if err != nil {
		t.Fatal(err)
	}
	if fpReal != fpLink {
		t.Errorf("symlinked paths should produce same fp: %q vs %q", fpReal, fpLink)
	}
}
