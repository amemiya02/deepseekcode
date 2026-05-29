package agent

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/amemiya02/deepseekcode/internal/agents"
	"github.com/amemiya02/deepseekcode/internal/skills"
	"github.com/amemiya02/deepseekcode/internal/tools"
)

// TestSwitchProfileCreatesEpochAndExpectsCacheMiss covers finding #4: a
// profile switch is a first-class runtime event that creates a new epoch
// (carrying the profile name in its hash) and records one expected cache
// miss, rather than mutating the live frozen epoch.
func TestSwitchProfileCreatesEpochAndExpectsCacheMiss(t *testing.T) {
	a := New(nil, tools.New(), nil, "deepseek-v4-flash")
	a.System = "static system prompt"

	first := a.epochMgr.InitEpoch("session_start", a.buildEpochComponents())
	a.epochMgr.FreezeEpoch()
	if a.currentProfileID() != "default" {
		t.Fatalf("default profile id = %q, want default", a.currentProfileID())
	}

	second := a.SwitchProfile(agents.AgentProfile{
		Name:      "explore",
		ToolTiers: []string{"core", "profile"},
	})
	if second == nil {
		t.Fatal("SwitchProfile returned nil epoch")
	}
	if second.EpochID == first.EpochID {
		t.Error("profile switch must create a new epoch id")
	}
	if second.AgentProfileID != "explore" {
		t.Errorf("new epoch AgentProfileID = %q, want explore", second.AgentProfileID)
	}
	// Under P1 the profile name is latent (not in the fingerprint). The test
	// registry has no profile-tier tools, so the model-visible bytes are
	// unchanged and the fingerprint stays the same — the new epoch is minted by
	// policy and the expected cache miss is driven by SwitchEpoch, not by a hash
	// change (docs/adr/0001).
	if second.StaticPrefixHash != first.StaticPrefixHash {
		t.Error("profile switch with no visible-tool change must not move the fingerprint")
	}
	if !a.epochMgr.ExpectedCacheMiss() {
		t.Error("first turn after a profile switch must expect a cache miss")
	}
	if a.epochMgr.ExpectedCacheMiss() {
		t.Error("expected-cache-miss must clear after the first read")
	}
	wantTiers := []tools.ToolTier{tools.TierCore, tools.TierProfile}
	if len(a.ActiveTiers) != len(wantTiers) {
		t.Fatalf("ActiveTiers = %v, want %v", a.ActiveTiers, wantTiers)
	}
	for i := range wantTiers {
		if a.ActiveTiers[i] != wantTiers[i] {
			t.Errorf("ActiveTiers[%d] = %q, want %q", i, a.ActiveTiers[i], wantTiers[i])
		}
	}
}

// TestSwitchProfileBeforeEpochInit is a no-op: the first runStep folds the
// profile into epoch #1, so there is nothing to switch yet.
func TestSwitchProfileBeforeEpochInit(t *testing.T) {
	a := New(nil, tools.New(), nil, "deepseek-v4-flash")
	if got := a.SwitchProfile(agents.AgentProfile{Name: "review"}); got != nil {
		t.Errorf("SwitchProfile before epoch init = %v, want nil", got)
	}
	if a.currentProfileID() != "review" {
		t.Errorf("profile should still be recorded; got %q", a.currentProfileID())
	}
}

// TestSkillBodyChangeIsPendingNotLiveDrift covers finding #3's epoch
// integration: editing a skill body changes the skill version hash and
// therefore surfaces a pending change against a frozen epoch, but the live
// epoch's static prefix hash stays byte-stable.
func TestSkillBodyChangeIsPendingNotLiveDrift(t *testing.T) {
	writeSkill := func(dir, body string) string {
		skillDir := filepath.Join(dir, "review")
		if err := os.MkdirAll(skillDir, 0o755); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(skillDir, "SKILL.md")
		if err := os.WriteFile(path, []byte(`---
name: review
description: Review code
---
# Review
`+body+`
`), 0o644); err != nil {
			t.Fatal(err)
		}
		return dir
	}

	dir := t.TempDir()
	writeSkill(dir, "Original guidance.")
	store1, err := skills.Load([]string{dir})
	if err != nil {
		t.Fatal(err)
	}

	a := New(nil, tools.New(), nil, "deepseek-v4-flash")
	a.System = "static system prompt"
	a.Skills = store1

	epoch := a.epochMgr.InitEpoch("session_start", a.buildEpochComponents())
	a.epochMgr.FreezeEpoch()
	liveHash := epoch.StaticPrefixHash

	// Body-only edit: same description, different body.
	writeSkill(dir, "Updated guidance with more detail.")
	store2, err := skills.Load([]string{dir})
	if err != nil {
		t.Fatal(err)
	}
	if store1.VersionHash() == store2.VersionHash() {
		t.Fatal("precondition: body edit must change the skill store version hash")
	}
	a.Skills = store2

	changes := a.epochMgr.DetectDrift(a.buildEpochComponents())
	found := false
	for _, c := range changes {
		if c.Kind == PendingSkillBodyChanged {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a pending skill_body_changed, got %v", changes)
	}
	if a.epochMgr.CurrentEpoch().StaticPrefixHash != liveHash {
		t.Error("live epoch static prefix hash must not change on a skill body edit")
	}
}
