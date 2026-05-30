package main

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/amemiya02/deepseekcode/internal/agent"
	"github.com/amemiya02/deepseekcode/internal/tools"
)

func TestParseToolTiersEnv(t *testing.T) {
	const key = "DEEPSEEKCODE_TOOL_TIERS_TEST"

	cases := []struct {
		name      string
		envValue  string
		wantTiers []tools.ToolTier
		wantOK    bool
	}{
		{"unset", "", nil, false},
		{"all", "all", nil, true},
		{"all_uppercase", "ALL", nil, true},
		{"legacy_true", "true", []tools.ToolTier{tools.TierCore}, true},
		{"legacy_1", "1", []tools.ToolTier{tools.TierCore}, true},
		{"core_only", "core", []tools.ToolTier{tools.TierCore}, true},
		{"core_profile", "core,profile", []tools.ToolTier{tools.TierCore, tools.TierProfile}, true},
		{"all_three", "core,profile,lazy", []tools.ToolTier{tools.TierCore, tools.TierProfile, tools.TierLazy}, true},
		{"unknown_only", "frobozz", nil, false},
		{"unknown_mixed", "core,frobozz", []tools.ToolTier{tools.TierCore}, true},
		{"trailing_comma", "core,", []tools.ToolTier{tools.TierCore}, true},
		{"whitespace", "  core , profile ", []tools.ToolTier{tools.TierCore, tools.TierProfile}, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(key, tc.envValue)
			gotTiers, gotOK := parseToolTiersEnv(key)
			if gotOK != tc.wantOK {
				t.Fatalf("ok: got %v, want %v", gotOK, tc.wantOK)
			}
			if !reflect.DeepEqual(gotTiers, tc.wantTiers) {
				t.Fatalf("tiers: got %v, want %v", gotTiers, tc.wantTiers)
			}
		})
	}
}

// TestToolTiersAllVsCore proves that "all" and "core" produce a different
// model-visible tool count on the same registry — the previous
// implementation made both behave identically (a no-op A/B).
func TestToolTiersAllVsCore(t *testing.T) {
	reg := tools.New()
	reg.RegisterWithTier(&tierTestTool{name: "tier_core_a"}, tools.TierCore)
	reg.RegisterWithTier(&tierTestTool{name: "tier_core_b"}, tools.TierCore)
	reg.RegisterWithTier(&tierTestTool{name: "tier_profile_a"}, tools.TierProfile)
	reg.RegisterWithTier(&tierTestTool{name: "tier_lazy_a"}, tools.TierLazy)

	const key = "DEEPSEEKCODE_TOOL_TIERS_TEST"

	t.Setenv(key, "all")
	allTiers, ok := parseToolTiersEnv(key)
	if !ok || allTiers != nil {
		t.Fatalf(`"all" must yield nil tiers (no filter) ok=true; got tiers=%v ok=%v`, allTiers, ok)
	}
	allTools := reg.AsLLMToolsFiltered(allTiers...)

	t.Setenv(key, "core")
	coreTiers, ok := parseToolTiersEnv(key)
	if !ok || len(coreTiers) != 1 || coreTiers[0] != tools.TierCore {
		t.Fatalf(`"core" must yield [TierCore] ok=true; got tiers=%v ok=%v`, coreTiers, ok)
	}
	coreTools := reg.AsLLMToolsFiltered(coreTiers...)

	if len(allTools) <= len(coreTools) {
		t.Fatalf("expected all-tier count (%d) > core-only count (%d) — A/B is a no-op",
			len(allTools), len(coreTools))
	}
	if len(coreTools) != 2 {
		t.Fatalf("core-only count: got %d, want 2", len(coreTools))
	}
	if len(allTools) != 4 {
		t.Fatalf("all-tier count: got %d, want 4", len(allTools))
	}
}

// TestApplyToolTiersFromEnv exercises the wiring path the CLI actually
// takes, instead of only the parser. Both runTUI and the one-shot
// `-p` path delegate to this helper, so asserting it on a real
// *agent.Agent locks in that DEEPSEEKCODE_TOOL_TIERS reaches the
// runtime tier filter (previously a no-op A/B even after the parser
// landed, because the agent's default already matched).
func TestApplyToolTiersFromEnv(t *testing.T) {
	cases := []struct {
		name      string
		envValue  string
		wantTiers []tools.ToolTier
	}{
		{"all_means_no_filter", "all", nil},
		{"core_only", "core", []tools.ToolTier{tools.TierCore}},
		{"core_profile", "core,profile", []tools.ToolTier{tools.TierCore, tools.TierProfile}},
		{"unset_keeps_default", "", []tools.ToolTier{tools.TierCore}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("DEEPSEEKCODE_TOOL_TIERS", tc.envValue)
			a := agent.New(nil, tools.New(), nil, "test-model")
			applyToolTiersFromEnv(a)
			if !reflect.DeepEqual(a.ActiveTiers, tc.wantTiers) {
				t.Fatalf("ActiveTiers: got %v, want %v", a.ActiveTiers, tc.wantTiers)
			}
		})
	}
}

// tierTestTool is a minimal tools.Tool used only for tier-count assertions.
type tierTestTool struct{ name string }

func (t *tierTestTool) Name() string        { return t.name }
func (t *tierTestTool) Description() string { return "test stub" }
func (t *tierTestTool) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object"}`)
}
func (t *tierTestTool) Execute(_ context.Context, _ json.RawMessage) (tools.Result, error) {
	return tools.Result{}, nil
}
