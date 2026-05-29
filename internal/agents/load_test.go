package agents

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
)

func TestParseAgent(t *testing.T) {
	t.Run("full frontmatter", func(t *testing.T) {
		d, err := ParseAgent("---\nmode: plan\ntools: read_file, grep\ndescription: x\n---\nhello")
		if err != nil {
			t.Fatal(err)
		}
		if d.Mode != "plan" {
			t.Errorf("Mode = %q, want %q", d.Mode, "plan")
		}
		want := []string{"read_file", "grep"}
		if len(d.Tools) != len(want) {
			t.Fatalf("Tools = %v, want %v", d.Tools, want)
		}
		for i, w := range want {
			if d.Tools[i] != w {
				t.Errorf("Tools[%d] = %q, want %q", i, d.Tools[i], w)
			}
		}
		if d.Description != "x" {
			t.Errorf("Description = %q, want %q", d.Description, "x")
		}
		if d.Prompt != "hello" {
			t.Errorf("Prompt = %q, want %q", d.Prompt, "hello")
		}
	})

	t.Run("just body defaults subagent", func(t *testing.T) {
		d, err := ParseAgent("just body")
		if err != nil {
			t.Fatal(err)
		}
		if d.Mode != "subagent" {
			t.Errorf("Mode = %q, want %q", d.Mode, "subagent")
		}
		if d.Prompt != "just body" {
			t.Errorf("Prompt = %q, want %q", d.Prompt, "just body")
		}
		if d.Description != "" {
			t.Errorf("Description = %q, want empty", d.Description)
		}
		if d.Tools != nil {
			t.Errorf("Tools = %v, want nil", d.Tools)
		}
	})

	t.Run("empty content", func(t *testing.T) {
		d, err := ParseAgent("")
		if err != nil {
			t.Fatal(err)
		}
		if d.Mode != "" {
			t.Errorf("Mode = %q, want empty", d.Mode)
		}
	})

	t.Run("only opening ---", func(t *testing.T) {
		d, err := ParseAgent("---\nsome text without closing")
		if err != nil {
			t.Fatal(err)
		}
		if d.Prompt != "---\nsome text without closing" {
			t.Errorf("Prompt = %q, want full content", d.Prompt)
		}
	})

	t.Run("empty tools value", func(t *testing.T) {
		d, err := ParseAgent("---\ntools:\n---\nbody")
		if err != nil {
			t.Fatal(err)
		}
		if d.Tools != nil {
			t.Errorf("Tools = %v, want nil for empty value", d.Tools)
		}
	})

	t.Run("tools all commas", func(t *testing.T) {
		d, err := ParseAgent("---\ntools: ,,\n---\nbody")
		if err != nil {
			t.Fatal(err)
		}
		if d.Tools != nil {
			t.Errorf("Tools = %v, want nil for all-commas", d.Tools)
		}
	})

	t.Run("unknown mode preserved", func(t *testing.T) {
		d, err := ParseAgent("---\nmode: custom\n---\nbody")
		if err != nil {
			t.Fatal(err)
		}
		if d.Mode != "custom" {
			t.Errorf("Mode = %q, want %q (preserved)", d.Mode, "custom")
		}
	})

	t.Run("crlf normalization", func(t *testing.T) {
		d, err := ParseAgent("---\r\nmode: plan\r\n---\r\nhello\r\nworld")
		if err != nil {
			t.Fatal(err)
		}
		if d.Mode != "plan" {
			t.Errorf("Mode = %q, want %q", d.Mode, "plan")
		}
	})

	t.Run("quoted values", func(t *testing.T) {
		d, err := ParseAgent("---\ndescription: \"quoted val\"\n---\nbody")
		if err != nil {
			t.Fatal(err)
		}
		if d.Description != "quoted val" {
			t.Errorf("Description = %q, want %q", d.Description, "quoted val")
		}
	})
}

func TestLoad(t *testing.T) {
	t.Run("nonexistent dir returns empty map", func(t *testing.T) {
		m, err := Load("/nonexistent/path/that/does/not/exist", "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(m) != 0 {
			t.Errorf("expected empty map, got %d entries", len(m))
		}
	})

	t.Run("loads agents from temp dir", func(t *testing.T) {
		dir := t.TempDir()
		agentDir := filepath.Join(dir, ".deepseek", "agent")
		if err := os.MkdirAll(agentDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(agentDir, "explore.md"), []byte("---\ndescription: explorer\ntools: read_file, grep\n---\nExplore."), 0o644); err != nil {
			t.Fatal(err)
		}

		m, err := Load(dir, "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(m) != 1 {
			t.Fatalf("expected 1 agent, got %d", len(m))
		}
		def, ok := m["explore"]
		if !ok {
			t.Fatal("missing 'explore' agent")
		}
		if def.Description != "explorer" {
			t.Errorf("Description = %q, want %q", def.Description, "explorer")
		}
		if def.Mode != "subagent" {
			t.Errorf("Mode = %q, want %q", def.Mode, "subagent")
		}
		wantTools := []string{"read_file", "grep"}
		if len(def.Tools) != len(wantTools) {
			t.Fatalf("Tools = %v, want %v", def.Tools, wantTools)
		}
		if def.Name != "explore" {
			t.Errorf("Name = %q, want %q", def.Name, "explore")
		}
	})

	t.Run("cwd wins over home on name conflict", func(t *testing.T) {
		cwd := t.TempDir()
		home := t.TempDir()
		for _, d := range []string{cwd, home} {
			agentDir := filepath.Join(d, ".deepseek", "agent")
			os.MkdirAll(agentDir, 0o755)
		}
		os.WriteFile(filepath.Join(cwd, ".deepseek", "agent", "x.md"), []byte("---\ndescription: cwd\n---\ncwd"), 0o644)
		os.WriteFile(filepath.Join(home, ".deepseek", "agent", "x.md"), []byte("---\ndescription: home\n---\nhome"), 0o644)

		m, err := Load(cwd, home)
		if err != nil {
			t.Fatal(err)
		}
		def, ok := m["x"]
		if !ok {
			t.Fatal("missing 'x' agent")
		}
		if def.Description != "cwd" {
			t.Errorf("Description = %q, want %q (cwd should win)", def.Description, "cwd")
		}
	})

	t.Run("skips hidden dirs and non-md files", func(t *testing.T) {
		dir := t.TempDir()
		agentDir := filepath.Join(dir, ".deepseek", "agent")
		os.MkdirAll(filepath.Join(agentDir, ".hidden"), 0o755)
		os.WriteFile(filepath.Join(agentDir, ".hidden", "secret.md"), []byte("---\ndescription: hidden\n---\nbody"), 0o644)
		os.WriteFile(filepath.Join(agentDir, "notanagent.txt"), []byte("ignored"), 0o644)
		os.WriteFile(filepath.Join(agentDir, "real.md"), []byte("---\ndescription: real\n---\nbody"), 0o644)

		m, err := Load(dir, "")
		if err != nil {
			t.Fatal(err)
		}
		if len(m) != 1 {
			t.Fatalf("expected 1 agent, got %d: %v", len(m), m)
		}
		if _, ok := m["real"]; !ok {
			t.Error("missing 'real' agent")
		}
	})

	t.Run("subdir agents get slash names", func(t *testing.T) {
		dir := t.TempDir()
		agentDir := filepath.Join(dir, ".deepseek", "agent")
		os.MkdirAll(filepath.Join(agentDir, "team"), 0o755)
		os.WriteFile(filepath.Join(agentDir, "team", "reviewer.md"), []byte("---\ndescription: reviewer\n---\nreview"), 0o644)

		m, err := Load(dir, "")
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := m["team/reviewer"]; !ok {
			t.Errorf("expected 'team/reviewer', keys: %v", keys(m))
		}
	})

	t.Run("empty cwd returns empty map", func(t *testing.T) {
		m, err := Load("", "")
		if err != nil {
			t.Fatal(err)
		}
		if len(m) != 0 {
			t.Errorf("expected empty, got %d", len(m))
		}
	})
}

func keys(m map[string]AgentDef) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

func TestParseAgentExtendedFrontmatter(t *testing.T) {
	got, err := ParseAgent("---\ndescription: Scout the repo\nmode: all\nhidden: true\nmax_steps: 12\npermission_ruleset: read-only\ntemperature: 0.2\ntop_p: 0.9\ndefault_agent: scout\nomit_project_context: true\ntools: read_file, grep\n---\nScout instructions.")
	if err != nil {
		t.Fatalf("ParseAgent returned error: %v", err)
	}
	if got.Mode != "all" || !got.Hidden || got.MaxSteps != 12 {
		t.Fatalf("mode/hidden/max_steps not parsed: %+v", got)
	}
	if !got.OmitProjectContext {
		t.Fatalf("OmitProjectContext not parsed: %+v", got)
	}
	if got.PermissionRuleset != "read-only" {
		t.Fatalf("PermissionRuleset = %q", got.PermissionRuleset)
	}
	if got.Temperature == nil || *got.Temperature != 0.2 {
		t.Fatalf("Temperature = %v, want 0.2", got.Temperature)
	}
	if got.TopP == nil || *got.TopP != 0.9 {
		t.Fatalf("TopP = %v, want 0.9", got.TopP)
	}
	if got.DefaultAgent != "scout" {
		t.Fatalf("DefaultAgent = %q, want scout", got.DefaultAgent)
	}
}
