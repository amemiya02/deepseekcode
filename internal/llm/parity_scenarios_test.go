package llm

import (
	"path/filepath"
	"testing"
)

// TestParityScenariosUnique pins the named scenario table: at least one
// entry, every name unique, every Build non-nil. Adding a scenario whose
// name collides with another silently masks drift downstream — that's
// the failure mode this test exists to catch.
func TestParityScenariosUnique(t *testing.T) {
	scenarios := ParityScenarios()
	if len(scenarios) < 1 {
		t.Fatalf("ParityScenarios() returned %d entries; want >=1", len(scenarios))
	}
	seen := make(map[string]int, len(scenarios))
	for i, s := range scenarios {
		if s.Name == "" {
			t.Errorf("scenarios[%d]: empty Name", i)
		}
		if s.Build == nil {
			t.Errorf("scenarios[%d] (%q): Build is nil", i, s.Name)
		}
		if prev, ok := seen[s.Name]; ok {
			t.Errorf("duplicate scenario name %q at indices %d and %d", s.Name, prev, i)
		}
		seen[s.Name] = i
	}
}

// TestParityScenariosManifestLoads verifies the on-disk manifest parses
// and pins Version==1 — bumping the version is a deliberate migration
// signal, not an accidental bump.
func TestParityScenariosManifestLoads(t *testing.T) {
	path := filepath.Join("testdata", "parity", "manifest.json")
	m, err := loadParityManifest(path)
	if err != nil {
		t.Fatalf("loadParityManifest(%q): %v", path, err)
	}
	if m.Version != 1 {
		t.Errorf("manifest.Version = %d; want 1", m.Version)
	}
	if len(m.Scenarios) < 1 {
		t.Errorf("manifest.Scenarios is empty; want >=1 entry")
	}
}

// TestParityScenariosManifestMissing verifies a missing manifest returns
// a non-nil error rather than panicking — operators see a clear
// regenerate command instead of a crash.
func TestParityScenariosManifestMissing(t *testing.T) {
	_, err := loadParityManifest(filepath.Join("testdata", "parity", "does-not-exist.json"))
	if err == nil {
		t.Fatal("loadParityManifest on missing file returned nil error; want error")
	}
}
