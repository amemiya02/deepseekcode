package skills

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStore_Load(t *testing.T) {
	dir := t.TempDir()

	// Create a skill with frontmatter
	skillDir := filepath.Join(dir, "test-skill")
	os.MkdirAll(skillDir, 0o755)
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(`---
name: review
description: Review code
runAs: subagent
---
# Review
This is the skill body.
`), 0o644)

	store, err := Load([]string{dir})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	skills := store.List()
	if len(skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(skills))
	}

	sk := skills[0]
	if sk.Name != "review" {
		t.Errorf("Name = %q, want %q", sk.Name, "review")
	}
	if sk.Description != "Review code" {
		t.Errorf("Description = %q, want %q", sk.Description, "Review code")
	}
	if sk.RunAs != "subagent" {
		t.Errorf("RunAs = %q, want %q", sk.RunAs, "subagent")
	}
	if sk.Scope != "test-skill" {
		t.Errorf("Scope = %q, want %q", sk.Scope, "test-skill")
	}
}

func TestStore_DirectoryNameAsDefault(t *testing.T) {
	dir := t.TempDir()

	// Create a skill without name in frontmatter
	skillDir := filepath.Join(dir, "my-skill")
	os.MkdirAll(skillDir, 0o755)
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(`---
description: My skill description
---
# Body
`), 0o644)

	store, err := Load([]string{dir})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	skills := store.List()
	if len(skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(skills))
	}

	if skills[0].Name != "my-skill" {
		t.Errorf("Name = %q, want %q (directory name)", skills[0].Name, "my-skill")
	}
}

func TestStore_EmptyDescriptionIgnored(t *testing.T) {
	dir := t.TempDir()

	// Create a skill with empty description
	skillDir := filepath.Join(dir, "no-desc")
	os.MkdirAll(skillDir, 0o755)
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(`---
name: no-desc
---
# Body
`), 0o644)

	store, err := Load([]string{dir})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if len(store.List()) != 0 {
		t.Errorf("expected 0 skills with empty description, got %d", len(store.List()))
	}
}

func TestStore_MissingRootIgnored(t *testing.T) {
	store, err := Load([]string{"/nonexistent/path"})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if len(store.List()) != 0 {
		t.Errorf("expected 0 skills for missing root, got %d", len(store.List()))
	}
}

func TestStore_DuplicateNamesKeepFirst(t *testing.T) {
	dir1 := t.TempDir()
	dir2 := t.TempDir()

	// Create same skill name in both roots
	for _, dir := range []string{dir1, dir2} {
		skillDir := filepath.Join(dir, "dup-skill")
		os.MkdirAll(skillDir, 0o755)
		os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(`---
name: dup
description: From `+filepath.Base(dir)+`
---
# Body
`), 0o644)
	}

	store, err := Load([]string{dir1, dir2})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	skills := store.List()
	if len(skills) != 1 {
		t.Fatalf("expected 1 skill (deduped), got %d", len(skills))
	}

	if skills[0].Description != "From "+filepath.Base(dir1) {
		t.Errorf("expected first root's description, got %q", skills[0].Description)
	}
}

func TestStore_IndexTextDeterministic(t *testing.T) {
	dir := t.TempDir()

	// Create multiple skills
	for _, name := range []string{"beta", "alpha", "gamma"} {
		skillDir := filepath.Join(dir, name)
		os.MkdirAll(skillDir, 0o755)
		os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(`---
name: `+name+`
description: Skill `+name+`
---
# Body
`), 0o644)
	}

	store, err := Load([]string{dir})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	// Load multiple times and check determinism
	text1 := store.IndexText()
	store2, _ := Load([]string{dir})
	text2 := store2.IndexText()

	if text1 != text2 {
		t.Errorf("IndexText not deterministic:\n%s\nvs\n%s", text1, text2)
	}

	// Should be sorted by name
	expected := "alpha | Skill alpha | alpha | direct\nbeta | Skill beta | beta | direct\ngamma | Skill gamma | gamma | direct\n"
	if text1 != expected {
		t.Errorf("IndexText = %q, want %q", text1, expected)
	}
}

func TestStore_IndexTextExcludesBodies(t *testing.T) {
	dir := t.TempDir()

	skillDir := filepath.Join(dir, "test")
	os.MkdirAll(skillDir, 0o755)
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(`---
name: test
description: Test skill
---
# Body
This is a long body that should not appear in index text.
`), 0o644)

	store, err := Load([]string{dir})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	text := store.IndexText()
	if len(text) > 200 {
		t.Errorf("IndexText too long (%d chars), body may be included", len(text))
	}
}

func TestStore_Body(t *testing.T) {
	dir := t.TempDir()

	skillDir := filepath.Join(dir, "test")
	os.MkdirAll(skillDir, 0o755)
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(`---
name: test
description: Test skill
---
# Body
This is the skill body.
`), 0o644)

	store, err := Load([]string{dir})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	body, ok := store.Body("test")
	if !ok {
		t.Fatal("expected Body to return true")
	}
	if body == "" {
		t.Error("expected non-empty body")
	}
}

func TestStore_BodyMissing(t *testing.T) {
	dir := t.TempDir()

	store, err := Load([]string{dir})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	body, ok := store.Body("nonexistent")
	if ok {
		t.Error("expected Body to return false for missing skill")
	}
	if body != "" {
		t.Errorf("expected empty body, got %q", body)
	}
}

func TestStore_VersionHashDeterministic(t *testing.T) {
	dir := t.TempDir()

	for _, name := range []string{"alpha", "beta"} {
		skillDir := filepath.Join(dir, name)
		os.MkdirAll(skillDir, 0o755)
		os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(`---
name: `+name+`
description: Skill `+name+`
---
# Body
`), 0o644)
	}

	s1, _ := Load([]string{dir})
	s2, _ := Load([]string{dir})

	h1 := s1.VersionHash()
	h2 := s2.VersionHash()

	if h1 != h2 {
		t.Errorf("VersionHash not deterministic: %s vs %s", h1, h2)
	}
	if h1 == "" {
		t.Error("VersionHash should not be empty")
	}
}

func TestStore_VersionHashChangesOnNewSkill(t *testing.T) {
	dir := t.TempDir()

	skillDir := filepath.Join(dir, "alpha")
	os.MkdirAll(skillDir, 0o755)
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(`---
name: alpha
description: Skill alpha
---
# Body
`), 0o644)

	s1, _ := Load([]string{dir})
	h1 := s1.VersionHash()

	skillDir2 := filepath.Join(dir, "beta")
	os.MkdirAll(skillDir2, 0o755)
	os.WriteFile(filepath.Join(skillDir2, "SKILL.md"), []byte(`---
name: beta
description: Skill beta
---
# Body
`), 0o644)

	s2, _ := Load([]string{dir})
	h2 := s2.VersionHash()

	if h1 == h2 {
		t.Error("adding a skill should change VersionHash")
	}
}

func TestStore_DiffNilOld(t *testing.T) {
	dir := t.TempDir()

	skillDir := filepath.Join(dir, "test")
	os.MkdirAll(skillDir, 0o755)
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(`---
name: test
description: Test skill
---
# Body
`), 0o644)

	store, _ := Load([]string{dir})
	changes := store.Diff(nil)

	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(changes))
	}
	if changes[0].Kind != "added" {
		t.Errorf("expected 'added', got %q", changes[0].Kind)
	}
	if changes[0].Name != "test" {
		t.Errorf("expected 'test', got %q", changes[0].Name)
	}
}

func TestStore_DiffNoChange(t *testing.T) {
	dir := t.TempDir()

	skillDir := filepath.Join(dir, "test")
	os.MkdirAll(skillDir, 0o755)
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(`---
name: test
description: Test skill
---
# Body
`), 0o644)

	s1, _ := Load([]string{dir})
	s2, _ := Load([]string{dir})
	changes := s2.Diff(s1)

	if len(changes) != 0 {
		t.Errorf("expected 0 changes for identical stores, got %d", len(changes))
	}
}

func TestStore_DiffAdded(t *testing.T) {
	dir1 := t.TempDir()
	dir2 := t.TempDir()

	for _, dir := range []string{dir1, dir2} {
		skillDir := filepath.Join(dir, "alpha")
		os.MkdirAll(skillDir, 0o755)
		os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(`---
name: alpha
description: Alpha skill
---
# Body
`), 0o644)
	}

	skillDir2 := filepath.Join(dir2, "beta")
	os.MkdirAll(skillDir2, 0o755)
	os.WriteFile(filepath.Join(skillDir2, "SKILL.md"), []byte(`---
name: beta
description: Beta skill
---
# Body
`), 0o644)

	old, _ := Load([]string{dir1})
	newStore, _ := Load([]string{dir2})
	changes := newStore.Diff(old)

	found := false
	for _, c := range changes {
		if c.Kind == "added" && c.Name == "beta" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'added' change for beta, got %v", changes)
	}
}

func TestStore_DiffRemoved(t *testing.T) {
	dir1 := t.TempDir()
	dir2 := t.TempDir()

	for _, dir := range []string{dir1, dir2} {
		skillDir := filepath.Join(dir, "alpha")
		os.MkdirAll(skillDir, 0o755)
		os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(`---
name: alpha
description: Alpha skill
---
# Body
`), 0o644)
	}

	skillDirBeta := filepath.Join(dir1, "beta")
	os.MkdirAll(skillDirBeta, 0o755)
	os.WriteFile(filepath.Join(skillDirBeta, "SKILL.md"), []byte(`---
name: beta
description: Beta skill
---
# Body
`), 0o644)

	old, _ := Load([]string{dir1})
	newStore, _ := Load([]string{dir2})
	changes := newStore.Diff(old)

	found := false
	for _, c := range changes {
		if c.Kind == "removed" && c.Name == "beta" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'removed' change for beta, got %v", changes)
	}
}

func TestStore_DiffBodyChanged(t *testing.T) {
	dir1 := t.TempDir()
	dir2 := t.TempDir()

	for _, dir := range []string{dir1, dir2} {
		skillDir := filepath.Join(dir, "test")
		os.MkdirAll(skillDir, 0o755)
	}

	os.WriteFile(filepath.Join(dir1, "test", "SKILL.md"), []byte(`---
name: test
description: Old description
---
# Body
`), 0o644)

	os.WriteFile(filepath.Join(dir2, "test", "SKILL.md"), []byte(`---
name: test
description: New description
---
# Body
`), 0o644)

	old, _ := Load([]string{dir1})
	newStore, _ := Load([]string{dir2})
	changes := newStore.Diff(old)

	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(changes))
	}
	if changes[0].Kind != "body_changed" {
		t.Errorf("expected 'body_changed', got %q", changes[0].Kind)
	}
	if changes[0].OldHash == changes[0].NewHash {
		t.Error("old and new hashes should differ")
	}
}
