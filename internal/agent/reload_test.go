package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/amemiya02/deepseekcode/internal/prompt"
	"github.com/amemiya02/deepseekcode/internal/skills"
	"github.com/amemiya02/deepseekcode/internal/tools"
)

// writeReloadSkill writes <cwd>/.deepseek/skills/review/SKILL.md with a fixed
// description (so only the body — and thus the body-derived version_hash — can
// move the model-visible directory) and the given body.
func writeReloadSkill(t *testing.T, cwd, body string) {
	t.Helper()
	dir := filepath.Join(cwd, ".deepseek", "skills", "review")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "---\nname: review\ndescription: Review code for defects\n---\n# Review\n" + body + "\n"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// newReloadAgent builds an agent whose System is owned by a PromptBuilder fed
// from the canonical skill store, with a frozen epoch #1 — the state a live
// session is in when the user types /reload-skills.
func newReloadAgent(t *testing.T, cwd string) *Agent {
	t.Helper()
	store, err := skills.LoadScan(cwd, "")
	if err != nil {
		t.Fatalf("LoadScan: %v", err)
	}
	a := New(nil, tools.New(), nil, "deepseek-v4-flash")
	a.Skills = store
	a.PromptBuilder = &prompt.SystemPromptBuilder{
		StaticBase:     "BASE",
		SkillDirectory: store.PromptIndex(),
		Project:        &prompt.ProjectContext{CWD: cwd},
	}
	a.System = a.PromptBuilder.Build()
	a.epochMgr.InitEpoch("session_start", a.buildEpochComponents())
	a.epochMgr.FreezeEpoch()
	return a
}

func TestReloadSkillsMovesFingerprintAndMintsEpoch(t *testing.T) {
	cwd := t.TempDir()
	writeReloadSkill(t, cwd, "original body")
	a := newReloadAgent(t, cwd)

	firstEpoch := a.epochMgr.CurrentEpoch()
	oldStatic := a.staticSystem()

	// Edit the skill body on disk — same description, new body.
	writeReloadSkill(t, cwd, "REWRITTEN body with new guidance")

	res, err := a.ReloadSkills(cwd, "")
	if err != nil {
		t.Fatalf("ReloadSkills: %v", err)
	}
	if !res.FingerprintMoved {
		t.Fatal("a body edit must move the model-visible prefix")
	}
	if a.staticSystem() == oldStatic {
		t.Error("system prompt did not change after reload")
	}
	if len(res.Changes) != 1 || res.Changes[0].Kind != "body_changed" || res.Changes[0].Name != "review" {
		t.Errorf("changes = %+v, want one body_changed for review", res.Changes)
	}
	// A new epoch was minted (one deliberate cache miss), distinct from the old.
	if res.NewEpochID == "" || res.NewEpochID == firstEpoch.EpochID {
		t.Errorf("expected a new epoch id, got old=%s new=%s", firstEpoch.EpochID, res.NewEpochID)
	}
	if res.OldEpochID != firstEpoch.EpochID {
		t.Errorf("OldEpochID = %s, want %s", res.OldEpochID, firstEpoch.EpochID)
	}
	if cur := a.epochMgr.CurrentEpoch(); cur == nil || cur.EpochID != res.NewEpochID {
		t.Error("current epoch is not the newly minted one")
	}
	if !a.epochMgr.ExpectedCacheMiss() {
		t.Error("first turn after reload must expect a cache miss")
	}
	// skill_read (which shares the same store pointer) now resolves the new body.
	body, ok := a.Skills.Body("review")
	if !ok || !strings.Contains(body, "REWRITTEN body") {
		t.Errorf("Body after reload = %q (ok=%v), want the rewritten text", body, ok)
	}
}

func TestReloadSkillsNoChangeIsNoOp(t *testing.T) {
	cwd := t.TempDir()
	writeReloadSkill(t, cwd, "stable body")
	a := newReloadAgent(t, cwd)
	firstEpoch := a.epochMgr.CurrentEpoch()
	oldStatic := a.staticSystem()

	res, err := a.ReloadSkills(cwd, "") // no disk edit between load and reload
	if err != nil {
		t.Fatalf("ReloadSkills: %v", err)
	}
	if res.FingerprintMoved {
		t.Error("no skill change must not move the fingerprint")
	}
	if len(res.Changes) != 0 {
		t.Errorf("changes = %+v, want none", res.Changes)
	}
	if res.NewEpochID != "" {
		t.Error("a no-op reload must not mint a new epoch (no wasted cache miss)")
	}
	if a.epochMgr.CurrentEpoch().EpochID != firstEpoch.EpochID {
		t.Error("epoch must be unchanged on a no-op reload")
	}
	if a.staticSystem() != oldStatic {
		t.Error("system prompt must be unchanged on a no-op reload")
	}
}

func TestReloadSkillsRequiresStore(t *testing.T) {
	a := New(nil, tools.New(), nil, "m")
	if _, err := a.ReloadSkills(t.TempDir(), ""); err == nil {
		t.Error("ReloadSkills with no skill store must error")
	}
}

// TestReloadSkillsRequiresPromptBuilder confirms the reload refuses (rather than
// swapping the shared store invisibly) when skills are not model-visible.
func TestReloadSkillsRequiresPromptBuilder(t *testing.T) {
	cwd := t.TempDir()
	writeReloadSkill(t, cwd, "body")
	store, err := skills.LoadScan(cwd, "")
	if err != nil {
		t.Fatalf("LoadScan: %v", err)
	}
	a := New(nil, tools.New(), nil, "m")
	a.Skills = store // store set, but no PromptBuilder
	if _, err := a.ReloadSkills(cwd, ""); err == nil {
		t.Error("ReloadSkills with no PromptBuilder must error, not mutate the store invisibly")
	}
}

// TestHasActiveBackgroundWork pins the guard /reload-skills relies on: a running
// background job (e.g. an async subagent that outlives the main loop) must report
// as active so the reload refuses while it could still read the shared store.
func TestHasActiveBackgroundWork(t *testing.T) {
	a := New(nil, tools.New(), nil, "m")
	if a.HasActiveBackgroundWork() {
		t.Fatal("a fresh agent has no background work")
	}
	job, _ := a.Jobs.Start(context.Background(), JobSubagent, "child")
	if !a.HasActiveBackgroundWork() {
		t.Error("a running job must report active background work")
	}
	a.Jobs.Finish(job.ID, JobSucceeded, "done")
	if a.HasActiveBackgroundWork() {
		t.Error("a finished job must not report active background work")
	}
}
