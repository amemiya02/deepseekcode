package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/amemiya02/deepseekcode/internal/config"
	"github.com/amemiya02/deepseekcode/internal/permissions"
)

func TestEnforceRequirements(t *testing.T) {
	cases := []struct {
		name      string
		req       *config.Requirements
		mode      permissions.Mode
		sandboxOn bool
		wantErr   bool
	}{
		{"nil floor always ok", nil, permissions.ModeYolo, false, false},
		{"yolo refused under default ceiling", &config.Requirements{MaxMode: "default"}, permissions.ModeYolo, false, true},
		{"default allowed under default ceiling", &config.Requirements{MaxMode: "default"}, permissions.ModeDefault, false, false},
		{"read-only allowed under default ceiling", &config.Requirements{MaxMode: "default"}, permissions.ModeReadOnly, false, false},
		{"default refused under read-only ceiling", &config.Requirements{MaxMode: "read-only"}, permissions.ModeDefault, false, true},
		// ask-all ranks below default (every call gates through a human), so an
		// ask-all ceiling forbids the normal default policy — pin this so the
		// non-obvious ordering is intentional, not incidental.
		{"default refused under ask-all ceiling", &config.Requirements{MaxMode: "ask-all"}, permissions.ModeDefault, false, true},
		{"ask-all allowed under ask-all ceiling", &config.Requirements{MaxMode: "ask-all"}, permissions.ModeAskAll, false, false},
		{"unrecognized max_mode errors", &config.Requirements{MaxMode: "bogus"}, permissions.ModeDefault, false, true},
		{"sandbox required but off", &config.Requirements{RequireSandbox: true}, permissions.ModeDefault, false, true},
		{"sandbox required and on", &config.Requirements{RequireSandbox: true}, permissions.ModeDefault, true, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := enforceRequirements(c.req, c.mode, c.sandboxOn)
			if (err != nil) != c.wantErr {
				t.Errorf("enforceRequirements(%+v, %v, sandbox=%v) err = %v, wantErr %v", c.req, c.mode, c.sandboxOn, err, c.wantErr)
			}
		})
	}
}

// TestResolveLaunchModeHonorsFloor proves the floor is enforced end-to-end: a
// requirements.toml in cwd forbidding yolo causes --yolo to be refused, while a
// within-ceiling launch is allowed.
func TestResolveLaunchModeHonorsFloor(t *testing.T) {
	cwd := t.TempDir()
	dir := filepath.Join(cwd, ".deepseek")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "requirements.toml"), []byte("max_mode = \"default\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := resolveLaunchMode(modeFlags{yolo: true}, cwd, false); err == nil {
		t.Error("resolveLaunchMode must refuse --yolo when requirements.toml forbids it")
	}
	mode, err := resolveLaunchMode(modeFlags{}, cwd, false)
	if err != nil {
		t.Errorf("default mode should be allowed under a default ceiling: %v", err)
	}
	if mode != permissions.ModeDefault {
		t.Errorf("mode = %v, want default", mode)
	}
}

// TestResolveLaunchModeNoFloor confirms the mechanism is opt-in: with no
// requirements.toml, --yolo resolves to ModeYolo unobstructed.
func TestResolveLaunchModeNoFloor(t *testing.T) {
	mode, err := resolveLaunchMode(modeFlags{yolo: true}, t.TempDir(), false)
	if err != nil {
		t.Fatalf("no floor must not error: %v", err)
	}
	if mode != permissions.ModeYolo {
		t.Errorf("mode = %v, want yolo", mode)
	}
}
