package skills

import (
	"os"
	"path/filepath"
	"testing"
)

// parseTmp writes content to a SKILL.md under a temp dir named dirName and
// parses it the way LoadScan/discoverSkills would (dirName is the fallback
// skill name).
func parseTmp(t *testing.T, dirName, content string) Skill {
	t.Helper()
	dir := filepath.Join(t.TempDir(), dirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "SKILL.md")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	sk, err := parseSkillFile(path, dirName)
	if err != nil {
		t.Fatalf("parseSkillFile: %v", err)
	}
	return sk
}

// TestParseSkillFile_Semantics ports the prompt loader's ParseSkill cases to
// the canonical skills parser: frontmatter name/description, the heading-only
// fallback, quoted-value unquoting, and a colon inside a description. These
// guard against the model-visible directory shrinking when the dual loader was
// removed.
func TestParseSkillFile_Semantics(t *testing.T) {
	tests := []struct {
		name        string
		dirName     string
		input       string
		wantName    string
		wantDescrip string
	}{
		{
			name:        "full frontmatter",
			dirName:     "pdf",
			input:       "---\nname: pdf-tools\ndescription: edit PDFs\n---\n# PDF Tools\n...body...",
			wantName:    "pdf-tools",
			wantDescrip: "edit PDFs",
		},
		{
			name:     "name only frontmatter",
			dirName:  "my",
			input:    "---\nname: my-skill\n---\nsome body",
			wantName: "my-skill",
		},
		{
			name:     "no frontmatter with heading",
			dirName:  "quick",
			input:    "# Quick Helper\nsome text",
			wantName: "Quick Helper",
		},
		{
			// No frontmatter and no heading: the prompt loader rejected this,
			// but the store must keep the skill (dir name is the fallback) so
			// the directory and skill_read do not silently lose it.
			name:     "no frontmatter no heading falls back to dir name",
			dirName:  "loose",
			input:    "just some text without a heading",
			wantName: "loose",
		},
		{
			// Missing name in frontmatter falls back to the dir name rather
			// than erroring — again, do not drop the skill.
			name:        "frontmatter missing name",
			dirName:     "nameless",
			input:       "---\ndescription: no name field\n---\nbody",
			wantName:    "nameless",
			wantDescrip: "no name field",
		},
		{
			name:        "description with colon",
			dirName:     "x",
			input:       "---\nname: x\ndescription: foo: bar baz\n---\nbody",
			wantName:    "x",
			wantDescrip: "foo: bar baz",
		},
		{
			name:        "quoted values",
			dirName:     "q",
			input:       "---\nname: \"quoted-name\"\ndescription: 'single quoted'\n---\nbody",
			wantName:    "quoted-name",
			wantDescrip: "single quoted",
		},
		{
			name:     "frontmatter with comments and blank lines",
			dirName:  "c",
			input:    "---\n# comment\n\nname: test-skill\n---\nbody text",
			wantName: "test-skill",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sk := parseTmp(t, tt.dirName, tt.input)
			if sk.Name != tt.wantName {
				t.Errorf("Name = %q, want %q", sk.Name, tt.wantName)
			}
			if sk.Description != tt.wantDescrip {
				t.Errorf("Description = %q, want %q", sk.Description, tt.wantDescrip)
			}
			if sk.BodyHash == "" {
				t.Error("BodyHash should never be empty")
			}
		})
	}
}

// TestHeadingOnlySkillReadable proves a heading-only SKILL.md (no frontmatter)
// is both indexed and readable via Body — the regression the review flagged
// when the frontmatter-required loader replaced the prompt loader.
func TestHeadingOnlySkillReadable(t *testing.T) {
	root := t.TempDir()
	skillDir := filepath.Join(root, ".deepseek", "skills", "quick")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "# Quick Helper\n\nDo the quick thing."
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	store, err := LoadScan(root, "")
	if err != nil {
		t.Fatalf("LoadScan: %v", err)
	}

	names := store.Names()
	if len(names) != 1 || names[0] != "Quick Helper" {
		t.Fatalf("Names() = %v, want [Quick Helper]", names)
	}

	got, ok := store.Body("Quick Helper")
	if !ok {
		t.Fatal("Body(Quick Helper) not found — heading-only skill dropped")
	}
	if got != body {
		t.Errorf("Body = %q, want %q", got, body)
	}
}

// TestBodyEditFlipsVersionHashHeadingOnly confirms the body-hash machinery
// still works for the no-frontmatter layout: editing the body changes the
// version hash even though there is no frontmatter description to change.
func TestBodyEditFlipsVersionHashHeadingOnly(t *testing.T) {
	first := parseTmp(t, "quick", "# Quick Helper\noriginal body")
	second := parseTmp(t, "quick", "# Quick Helper\nedited body")
	if first.BodyHash == second.BodyHash {
		t.Error("editing a heading-only skill body should change BodyHash")
	}
}
