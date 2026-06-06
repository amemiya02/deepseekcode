package llm

import (
	"os"
	"strings"
	"testing"
)

func TestCacheABRunbook_Exists(t *testing.T) {
	path := "../../docs/competitive/2026-06-03-cache-ab-runbook.md"
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("runbook not found at %s: %v", path, err)
	}
	s := string(data)
	for _, marker := range []string{"arm-current", "arm-drop-all", "arm-retain-last"} {
		if !strings.Contains(s, marker) {
			t.Fatalf("runbook missing marker %q", marker)
		}
	}
}
