package prompt

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestParseSkill(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    Skill
		wantErr bool
	}{
		{
			name:  "full frontmatter",
			input: "---\nname: pdf-tools\ndescription: edit PDFs\n---\n# PDF Tools\n...body...",
			want:  Skill{Name: "pdf-tools", Description: "edit PDFs", Body: "# PDF Tools\n...body..."},
		},
		{
			name:  "name only frontmatter",
			input: "---\nname: my-skill\n---\nsome body",
			want:  Skill{Name: "my-skill", Description: "", Body: "some body"},
		},
		{
			name:  "no frontmatter with heading",
			input: "# Quick Helper\nsome text",
			want:  Skill{Name: "Quick Helper", Description: "", Body: "# Quick Helper\nsome text"},
		},
		{
			name:    "no frontmatter no heading",
			input:   "just some text without a heading",
			wantErr: true,
		},
		{
			name:    "frontmatter missing name",
			input:   "---\ndescription: no name field\n---\nbody",
			wantErr: true,
		},
		{
			name:    "empty file",
			input:   "",
			wantErr: true,
		},
		{
			name:  "description with colon",
			input: "---\nname: x\ndescription: foo: bar baz\n---\nbody",
			want:  Skill{Name: "x", Description: "foo: bar baz", Body: "body"},
		},
		{
			name:  "quoted values",
			input: "---\nname: \"quoted-name\"\ndescription: 'single quoted'\n---\nbody",
			want:  Skill{Name: "quoted-name", Description: "single quoted", Body: "body"},
		},
		{
			name:  "frontmatter with comments and blank lines",
			input: "---\n# comment\n\nname: test-skill\n---\nbody text",
			want:  Skill{Name: "test-skill", Description: "", Body: "body text"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseSkill(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Name != tt.want.Name {
				t.Errorf("Name = %q, want %q", got.Name, tt.want.Name)
			}
			if got.Description != tt.want.Description {
				t.Errorf("Description = %q, want %q", got.Description, tt.want.Description)
			}
			if got.Body != tt.want.Body {
				t.Errorf("Body = %q, want %q", got.Body, tt.want.Body)
			}
		})
	}
}

func TestLoadSkills(t *testing.T) {
	cwd := t.TempDir()
	home := t.TempDir()

	// .deepseek/skills/a/SKILL.md — higher priority
	writeSkill(t, filepath.Join(cwd, ".deepseek", "skills", "a"), "pdf-tools", "edit PDFs")
	// .claude/skills/a/SKILL.md — same name, should be deduped (deepseek wins)
	writeSkill(t, filepath.Join(cwd, ".claude", "skills", "a"), "pdf-tools", "claude version")
	// .claude/skills/b/SKILL.md — unique, proves .claude is scanned
	writeSkill(t, filepath.Join(cwd, ".claude", "skills", "b"), "git-helper", "git ops")
	// home-level skill
	writeSkill(t, filepath.Join(home, ".deepseek", "skills", "c"), "home-skill", "from home")

	skills, err := LoadSkills(cwd, home)
	if err != nil {
		t.Fatal(err)
	}

	// 3 unique skills: pdf-tools (from .deepseek, not .claude), git-helper, home-skill
	if len(skills) != 3 {
		t.Fatalf("got %d skills, want 3", len(skills))
	}

	// pdf-tools from .deepseek, not .claude
	if skills[0].Name != "pdf-tools" {
		t.Errorf("skills[0].Name = %q, want %q", skills[0].Name, "pdf-tools")
	}
	if skills[0].Description != "edit PDFs" {
		t.Errorf("skills[0].Description = %q, want %q", skills[0].Description, "edit PDFs")
	}

	// git-helper proves .claude was scanned
	if skills[1].Name != "git-helper" {
		t.Errorf("skills[1].Name = %q, want %q", skills[1].Name, "git-helper")
	}

	// home-skill from home dir
	if skills[2].Name != "home-skill" {
		t.Errorf("skills[2].Name = %q, want %q", skills[2].Name, "home-skill")
	}

	// All paths should be absolute and non-empty
	for i, s := range skills {
		if s.Path == "" || !filepath.IsAbs(s.Path) {
			t.Errorf("skills[%d].Path = %q, want absolute", i, s.Path)
		}
	}
}

func TestLoadSkillsEmptyCwd(t *testing.T) {
	_, err := LoadSkills("", "/home")
	if err == nil {
		t.Error("expected error for empty cwd")
	}
}

func TestLoadSkillsMissingDirs(t *testing.T) {
	cwd := t.TempDir()
	skills, err := LoadSkills(cwd, "/nonexistent/home")
	if err != nil {
		t.Fatal(err)
	}
	if len(skills) != 0 {
		t.Errorf("got %d skills, want 0", len(skills))
	}
}

func TestLoadSkillsHomeEqualsCwd(t *testing.T) {
	dir := t.TempDir()
	writeSkill(t, filepath.Join(dir, ".deepseek", "skills", "x"), "only-skill", "desc")

	skills, err := LoadSkills(dir, dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(skills) != 1 {
		t.Fatalf("got %d skills, want 1", len(skills))
	}
	if skills[0].Name != "only-skill" {
		t.Errorf("Name = %q, want %q", skills[0].Name, "only-skill")
	}
}

func TestLoadSkillsNestedSkill(t *testing.T) {
	cwd := t.TempDir()
	writeSkill(t, filepath.Join(cwd, ".deepseek", "skills", "group", "sub"), "nested-skill", "nested desc")

	skills, err := LoadSkills(cwd, "/nonexistent")
	if err != nil {
		t.Fatal(err)
	}
	if len(skills) != 1 {
		t.Fatalf("got %d skills, want 1", len(skills))
	}
	if skills[0].Name != "nested-skill" {
		t.Errorf("Name = %q, want %q", skills[0].Name, "nested-skill")
	}
}

func TestLoadSkillsDepthLimit(t *testing.T) {
	cwd := t.TempDir()
	// Build deep path under a scan root (.deepseek/skills).
	deep := filepath.Join(cwd, ".deepseek", "skills")
	for i := 0; i < 10; i++ {
		deep = filepath.Join(deep, fmt.Sprintf("d%d", i))
	}
	writeSkill(t, deep, "deep-skill", "too deep")
	// Shallow skill at scan root should still be found.
	writeSkill(t, filepath.Join(cwd, ".deepseek", "skills", "shallow"), "shallow", "ok")

	skills, err := LoadSkills(cwd, "")
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, s := range skills {
		names[s.Name] = true
	}
	if !names["shallow"] {
		t.Error("shallow skill should be found")
	}
	if names["deep-skill"] {
		t.Error("depth>8 skill should be truncated")
	}
}

func TestLoadSkillsStopsAtSkillMD(t *testing.T) {
	cwd := t.TempDir()
	// Parent dir has SKILL.md → should NOT descend into its subdirs.
	writeSkill(t, filepath.Join(cwd, ".deepseek", "skills", "pdf"), "pdf", "PDF tools")
	// Child skill should be ignored because parent already has SKILL.md.
	writeSkill(t, filepath.Join(cwd, ".deepseek", "skills", "pdf", "examples"), "pdf-example", "examples")

	skills, err := LoadSkills(cwd, "")
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, s := range skills {
		names[s.Name] = true
	}
	if !names["pdf"] {
		t.Error("parent pdf skill should be found")
	}
	if names["pdf-example"] {
		t.Error("child skill should be skipped (SKILL.md stops subtree descent)")
	}
}

func TestLoadSkillsSkipsHiddenDirs(t *testing.T) {
	cwd := t.TempDir()
	// Hidden dir under scan root should be skipped.
	writeSkill(t, filepath.Join(cwd, ".deepseek", "skills", ".hidden", "x"), "hidden-skill", "should be skipped")
	// Normal dir should still work.
	writeSkill(t, filepath.Join(cwd, ".deepseek", "skills", "visible"), "visible-skill", "ok")

	skills, err := LoadSkills(cwd, "")
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, s := range skills {
		names[s.Name] = true
	}
	if !names["visible-skill"] {
		t.Error("visible skill should be found")
	}
	if names["hidden-skill"] {
		t.Error("skill in hidden dir should be skipped")
	}
}

func writeSkill(t *testing.T, dir, name, description string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "---\nname: " + name + "\ndescription: " + description + "\n---\n# " + name + "\nbody"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
