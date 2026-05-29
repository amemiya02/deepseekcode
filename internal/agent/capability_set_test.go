package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/amemiya02/deepseekcode/internal/llm"
	"github.com/amemiya02/deepseekcode/internal/mcp"
	"github.com/amemiya02/deepseekcode/internal/skills"
)

func TestCapabilityDiff_ProfileChange(t *testing.T) {
	changes := CapabilityDiff(CapabilitySet{ProfileID: "default"}, CapabilitySet{ProfileID: "explore"})
	if len(changes) != 1 || changes[0].Kind != PendingAgentProfileChanged {
		t.Fatalf("expected one agent_profile_changed, got %+v", changes)
	}
}

func TestCapabilityDiff_Stable(t *testing.T) {
	cs := CapabilitySet{ProfileID: "default", MCPTools: []mcp.McpToolMeta{
		{Name: "mcp__s__a", Description: "a", InputSchema: json.RawMessage(`{"type":"object"}`)},
	}}
	if changes := CapabilityDiff(cs, cs); len(changes) != 0 {
		t.Fatalf("identical capability sets must produce no changes, got %+v", changes)
	}
}

func TestCapabilityDiff_MCPAddRemove(t *testing.T) {
	before := CapabilitySet{ProfileID: "default", MCPTools: []mcp.McpToolMeta{
		{Name: "mcp__s__a", Description: "a", InputSchema: json.RawMessage(`{}`)},
	}}
	added := CapabilitySet{ProfileID: "default", MCPTools: []mcp.McpToolMeta{
		{Name: "mcp__s__a", Description: "a", InputSchema: json.RawMessage(`{}`)},
		{Name: "mcp__s__b", Description: "b", InputSchema: json.RawMessage(`{}`)},
	}}
	if ch := CapabilityDiff(before, added); len(ch) != 1 || ch[0].Kind != PendingMCPToolAdded {
		t.Fatalf("expected mcp_tool_added, got %+v", ch)
	}
	if ch := CapabilityDiff(added, before); len(ch) != 1 || ch[0].Kind != PendingMCPToolRemoved {
		t.Fatalf("expected mcp_tool_removed, got %+v", ch)
	}
}

// TestCapabilityDiff_MCPKeyReorderNoDrift is the capability-level regression for
// the phantom-drift fix (docs/adr/0001): a reconnect re-emitting the same MCP
// tool with reordered JSON-Schema keys must produce zero pending changes.
func TestCapabilityDiff_MCPKeyReorderNoDrift(t *testing.T) {
	before := CapabilitySet{ProfileID: "default", MCPTools: []mcp.McpToolMeta{
		{Name: "mcp__s__search", Description: "s", InputSchema: json.RawMessage(`{"type":"object","properties":{"a":{"type":"string"},"b":{"type":"number"}}}`)},
	}}
	after := CapabilitySet{ProfileID: "default", MCPTools: []mcp.McpToolMeta{
		{Name: "mcp__s__search", Description: "s", InputSchema: json.RawMessage(`{"properties":{"b":{"type":"number"},"a":{"type":"string"}},"type":"object"}`)},
	}}
	if changes := CapabilityDiff(before, after); len(changes) != 0 {
		t.Fatalf("reordered MCP schema keys must not produce pending changes, got %+v", changes)
	}
}

// TestSkillEditReportedOnceNotAlsoSystem is the regression for the double-report
// defect (docs/adr/0001): a skill-body edit moves the Prefix Fingerprint via the
// system prompt (the skill directory is rendered into it), yet it must surface
// as exactly one skill_body_changed pending change — never also a raw
// system_changed. Because pending changes now come solely from CapabilityDiff,
// the raw system kind cannot be emitted at all.
func TestSkillEditReportedOnceNotAlsoSystem(t *testing.T) {
	dir := t.TempDir()
	write := func(body string) {
		sd := filepath.Join(dir, "review")
		if err := os.MkdirAll(sd, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(sd, "SKILL.md"),
			[]byte("---\nname: review\ndescription: Review code\n---\n# Review\n"+body+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("Original guidance.")
	s1, err := skills.Load([]string{dir})
	if err != nil {
		t.Fatal(err)
	}
	write("Updated guidance with more detail.")
	s2, err := skills.Load([]string{dir})
	if err != nil {
		t.Fatal(err)
	}
	if s1.VersionHash() == s2.VersionHash() {
		t.Fatal("precondition: body edit must change the skill store version hash")
	}

	// The skill directory is part of the model-visible system prompt, so the
	// fingerprint moves.
	sysWith := func(s *skills.Store) string { return "You are an agent.\n\n## Skills\n" + s.IndexText() }
	fp1 := llm.StaticPrefix{System: sysWith(s1)}.Fingerprint()
	fp2 := llm.StaticPrefix{System: sysWith(s2)}.Fingerprint()
	if fp1.CombinedSHA256 == fp2.CombinedSHA256 {
		t.Fatal("skill body edit should move the fingerprint via the system prompt")
	}

	// ...but it is reported as exactly one skill_body_changed.
	changes := CapabilityDiff(CapabilitySet{Skills: s1}, CapabilitySet{Skills: s2})
	if len(changes) != 1 || changes[0].Kind != PendingSkillBodyChanged {
		t.Fatalf("expected exactly one skill_body_changed (never a raw system change), got %+v", changes)
	}
}
