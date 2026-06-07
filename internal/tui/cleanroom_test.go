package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// No file under internal/tui may import any charmbracelet/crush package.
func TestNoCrushImports(t *testing.T) {
	files, _ := filepath.Glob("*.go")
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		// Only an import line is a violation; allow the word in comments.
		for _, line := range strings.Split(string(b), "\n") {
			tl := strings.TrimSpace(line)
			if strings.HasPrefix(tl, `"`) && strings.Contains(tl, "charmbracelet/crush") {
				t.Fatalf("%s imports crush — clean-room violation: %s", f, tl)
			}
		}
	}
}
