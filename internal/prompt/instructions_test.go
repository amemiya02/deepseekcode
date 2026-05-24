package prompt

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestLoadInstructionFilesWhitelist(t *testing.T) {
	cwd := t.TempDir()
	write(t, filepath.Join(cwd, "DEEPSEEK.md"), "alpha")
	write(t, filepath.Join(cwd, "AGENTS.md"), "beta")
	write(t, filepath.Join(cwd, ".deepseek", "instructions.md"), "gamma")

	got, err := LoadInstructionFiles(cwd, "")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("want 3 files, got %d (%v)", len(got), got)
	}
	contents := []string{got[0].Content, got[1].Content, got[2].Content}
	for _, want := range []string{"alpha", "beta", "gamma"} {
		found := false
		for _, c := range contents {
			if strings.Contains(c, want) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("missing content %q in %v", want, contents)
		}
	}
}

func TestLoadInstructionFilesRejectsClaudeMD(t *testing.T) {
	cwd := t.TempDir()
	write(t, filepath.Join(cwd, "DEEPSEEK.md"), "ok")
	write(t, filepath.Join(cwd, "CLAUDE.md"), "should-not-load")
	write(t, filepath.Join(cwd, ".claude", "CLAUDE.md"), "should-not-load")

	got, _ := LoadInstructionFiles(cwd, "")
	for _, f := range got {
		base := filepath.Base(f.Path)
		if strings.Contains(strings.ToLower(base), "claude") {
			t.Errorf("loaded forbidden file: %s", f.Path)
		}
		if strings.Contains(f.Content, "should-not-load") {
			t.Errorf("loaded forbidden content: %s", f.Content)
		}
	}
}

func TestLoadInstructionFilesTruncatesAt4000(t *testing.T) {
	cwd := t.TempDir()
	big := strings.Repeat("a", maxInstructionFileChars+500)
	write(t, filepath.Join(cwd, "DEEPSEEK.md"), big)

	got, _ := LoadInstructionFiles(cwd, "")
	if len(got) != 1 {
		t.Fatalf("want 1 file, got %d", len(got))
	}
	if !strings.Contains(got[0].Content, "[truncated]") {
		t.Errorf("expected truncation marker; content len=%d", len(got[0].Content))
	}
}

func TestLoadInstructionFilesTotalCap(t *testing.T) {
	cwd := t.TempDir()
	// Fill the per-file cap, then add another full-cap file. Total
	// after the second file would exceed 12000 so it should be
	// trimmed to fit (or dropped entirely if no headroom).
	full := strings.Repeat("b", maxInstructionFileChars)
	write(t, filepath.Join(cwd, "DEEPSEEK.md"), full)
	write(t, filepath.Join(cwd, "AGENTS.md"), full)
	write(t, filepath.Join(cwd, ".deepseek", "instructions.md"), full)

	got, _ := LoadInstructionFiles(cwd, "")
	total := 0
	for _, f := range got {
		total += len(f.Content)
	}
	if total > maxTotalInstructionChars+len("\n... [truncated]") {
		t.Errorf("total chars %d exceeds cap %d", total, maxTotalInstructionChars)
	}
}

func TestLoadInstructionFilesCwdEqualsHome(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "DEEPSEEK.md"), "once")
	got, _ := LoadInstructionFiles(dir, dir)
	if len(got) != 1 {
		t.Errorf("expected single load when cwd==home, got %d", len(got))
	}
}

func TestLoadInstructionFilesCwdWinsOverHome(t *testing.T) {
	cwd := t.TempDir()
	home := t.TempDir()
	write(t, filepath.Join(cwd, "DEEPSEEK.md"), "cwd-version")
	write(t, filepath.Join(home, "DEEPSEEK.md"), "home-version")
	got, _ := LoadInstructionFiles(cwd, home)
	if len(got) != 1 {
		t.Fatalf("want 1 file, got %d", len(got))
	}
	if !strings.Contains(got[0].Content, "cwd-version") {
		t.Errorf("expected cwd to win; got %q", got[0].Content)
	}
}

func TestLoadInstructionFilesSkipsEmpty(t *testing.T) {
	cwd := t.TempDir()
	write(t, filepath.Join(cwd, "DEEPSEEK.md"), "")
	got, _ := LoadInstructionFiles(cwd, "")
	if len(got) != 0 {
		t.Errorf("empty file should be skipped; got %v", got)
	}
}
