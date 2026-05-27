package prompt_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/amemiya02/deepseekcode/internal/prompt"
	"github.com/amemiya02/deepseekcode/internal/skills"
)

// writeReviewSkill writes a review/SKILL.md under <cwd>/.deepseek/skills with
// a fixed frontmatter description and the given body, then loads the canonical
// store. The description never changes — only the body — so any change to the
// rendered prefix must come from the body-derived version_hash.
func writeReviewSkill(t *testing.T, cwd, body string) *skills.Store {
	t.Helper()
	dir := filepath.Join(cwd, ".deepseek", "skills", "review")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "---\nname: review\ndescription: Review code for defects\n---\n# Review\n" + body + "\n"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	store, err := skills.LoadScan(cwd, "")
	if err != nil {
		t.Fatalf("LoadScan: %v", err)
	}
	return store
}

func staticPrefix(t *testing.T, store *skills.Store) string {
	t.Helper()
	b := prompt.SystemPromptBuilder{
		StaticBase:     "BASE",
		SkillDirectory: store.PromptIndex(),
		Project:        &prompt.ProjectContext{CWD: "/tmp"},
	}
	out := b.Build()
	i := strings.Index(out, prompt.DynamicContextBoundary)
	if i < 0 {
		t.Fatal("boundary missing")
	}
	return out[:i]
}

// TestBuildPrefixReflectsSkillBodyEdit is the P0 cache-chain proof: the
// version_hash lives in the DeepSeek-visible static prefix, so editing a skill
// body — without touching its description — moves the prefix bytes. Before the
// canonical store was rendered into the prompt, the body edit changed only the
// internal epoch hash while the request bytes stayed identical, defeating
// cache invalidation.
func TestBuildPrefixReflectsSkillBodyEdit(t *testing.T) {
	bodyOriginal := "Look for nil dereferences."
	bodyEdited := "Look for nil dereferences and data races and unchecked errors."

	s1 := writeReviewSkill(t, t.TempDir(), bodyOriginal)
	s2 := writeReviewSkill(t, t.TempDir(), bodyEdited)

	p1 := staticPrefix(t, s1)
	p2 := staticPrefix(t, s2)

	if p1 == p2 {
		t.Fatal("body edit must change the static prefix bytes (cache must invalidate)")
	}

	// The version_hash the store computed must actually appear in the prefix.
	// IndexText line: name | desc | run_mode | version_hash | tools
	fields := strings.Split(strings.TrimRight(s1.IndexText(), "\n"), " | ")
	if len(fields) != 5 {
		t.Fatalf("unexpected index line: %q", s1.IndexText())
	}
	hash := fields[3]
	if len(hash) != 12 {
		t.Fatalf("version_hash %q is not 12 hex chars", hash)
	}
	if !strings.Contains(p1, hash) {
		t.Errorf("version_hash %q missing from static prefix", hash)
	}

	// The prefix must NOT leak the skill body or a local absolute path.
	if strings.Contains(p1, bodyOriginal) || strings.Contains(p2, bodyEdited) {
		t.Error("static prefix leaked the full skill body")
	}
	if strings.Contains(p1, "SKILL.md") || strings.Contains(p1, "/.deepseek/skills/") {
		t.Errorf("static prefix leaked a local path; got: %q", p1)
	}
}

// TestBuildPrefixStableAcrossReload proves the prefix is deterministic: the
// same skills on disk produce byte-identical prefixes across reloads, so the
// prompt cache is not invalidated by the loader itself.
func TestBuildPrefixStableAcrossReload(t *testing.T) {
	cwd := t.TempDir()
	s1 := writeReviewSkill(t, cwd, "Stable body.")
	store2, err := skills.LoadScan(cwd, "")
	if err != nil {
		t.Fatalf("LoadScan: %v", err)
	}
	if got, want := staticPrefix(t, store2), staticPrefix(t, s1); got != want {
		t.Errorf("prefix not stable across reload:\n%q\nvs\n%q", got, want)
	}
}
