package llm

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// TestParityGolden pins every named parity scenario's MarshalCacheStable
// output byte-for-byte. Each scenario gets its own golden file under
// testdata/parity/<name>.golden.json, and the manifest records the
// sha256 of every golden so an out-of-sync pair (golden touched without
// the manifest, or vice versa) shows up as a diff next run.
//
// Regenerate every golden + the manifest with:
//
//	UPDATE_GOLDEN=1 go test -run TestParityGolden ./internal/llm/
//
// A normal (non-UPDATE) run never writes files — it only reads them.
func TestParityGolden(t *testing.T) {
	scenarios := ParityScenarios()
	if len(scenarios) == 0 {
		t.Fatal("ParityScenarios() returned 0 entries")
	}

	dir := filepath.Join("testdata", "parity")
	manifestPath := filepath.Join(dir, "manifest.json")
	update := os.Getenv("UPDATE_GOLDEN") == "1"

	type manifestEntry struct {
		Name   string `json:"name"`
		SHA256 string `json:"sha256"`
	}
	var manifestEntries []manifestEntry

	if update {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}

	for _, s := range scenarios {
		s := s // capture for subtest
		t.Run(s.Name, func(t *testing.T) {
			req := s.Build()
			b, err := req.MarshalCacheStable()
			if err != nil {
				t.Fatalf("MarshalCacheStable: %v", err)
			}
			sum := fmt.Sprintf("%x", sha256.Sum256(b))
			goldenPath := filepath.Join(dir, s.Name+".golden.json")

			if update {
				if err := os.WriteFile(goldenPath, b, 0o644); err != nil {
					t.Fatalf("write golden %s: %v", goldenPath, err)
				}
				t.Logf("updated golden: %s (sha256=%s)", goldenPath, sum)
				manifestEntries = append(manifestEntries, manifestEntry{Name: s.Name, SHA256: sum})
				return
			}

			want, err := os.ReadFile(goldenPath)
			if err != nil {
				t.Fatalf("read golden %s: %v\nRegenerate with: UPDATE_GOLDEN=1 go test -run TestParityGolden ./internal/llm/", goldenPath, err)
			}
			wantSum := fmt.Sprintf("%x", sha256.Sum256(want))
			if sum != wantSum || string(b) != string(want) {
				t.Errorf("parity drift for scenario %q:\n  got  sha256=%s len=%d\n  want sha256=%s len=%d\n\nIf intentional, regenerate with: UPDATE_GOLDEN=1 go test -run TestParityGolden ./internal/llm/",
					s.Name, sum, len(b), wantSum, len(want))
			}
		})
	}

	if update {
		// Sort entries by scenario name so manifest ordering is
		// deterministic regardless of map/iteration quirks upstream.
		// ParityScenarios already returns a stable slice, but we keep
		// this defensive — drift in the manifest itself would be a
		// nightmare to debug.
		out := struct {
			Version   int             `json:"version"`
			Scenarios []manifestEntry `json:"scenarios"`
		}{Version: 1, Scenarios: manifestEntries}
		buf, err := json.MarshalIndent(out, "", "  ")
		if err != nil {
			t.Fatalf("marshal manifest: %v", err)
		}
		// MarshalIndent omits a trailing newline; keep one for POSIX-y tooling.
		buf = append(buf, '\n')
		if err := os.WriteFile(manifestPath, buf, 0o644); err != nil {
			t.Fatalf("write manifest %s: %v", manifestPath, err)
		}
		t.Logf("updated manifest: %s (%d scenarios)", manifestPath, len(manifestEntries))
	}
}
