package agent

import (
	"encoding/json"
	"testing"

	"github.com/amemiya02/deepseekcode/internal/llm"
	"github.com/amemiya02/deepseekcode/internal/mcp"
	"github.com/amemiya02/deepseekcode/internal/tokenizer"
	"github.com/amemiya02/deepseekcode/internal/tools"
)

func testTools() []llm.Tool {
	return []llm.Tool{
		{Type: "function", Function: llm.ToolFunction{Name: "read_file", Description: "Read a file", Parameters: json.RawMessage(`{"type":"object"}`)}},
		{Type: "function", Function: llm.ToolFunction{Name: "bash", Description: "Run a command", Parameters: json.RawMessage(`{"type":"object"}`)}},
	}
}

func testComponents() EpochComponents {
	return EpochComponents{
		StaticSystem:   "You are a coding agent.",
		ToolSpecs:      testTools(),
		Model:          "deepseek-v4-flash",
		AgentProfileID: "default",
		Capability:     CapabilitySet{ProfileID: "default"},
	}
}

func TestEpochDeterministicHash(t *testing.T) {
	comps := testComponents()
	m1 := NewEpochManager()
	e1 := m1.InitEpoch("test", comps)

	m2 := NewEpochManager()
	e2 := m2.InitEpoch("test", comps)

	if e1.StaticPrefixHash != e2.StaticPrefixHash {
		t.Errorf("same inputs should produce same hash\n  e1: %s\n  e2: %s", e1.StaticPrefixHash, e2.StaticPrefixHash)
	}
	if e1.ComponentHashes["system"] != e2.ComponentHashes["system"] {
		t.Error("system component hash should be deterministic")
	}
	if e1.ComponentHashes["tools"] != e2.ComponentHashes["tools"] {
		t.Error("tools component hash should be deterministic")
	}
}

func TestEpochToolSchemaFieldOrderStable(t *testing.T) {
	comps1 := testComponents()
	comps1.ToolSpecs = []llm.Tool{
		{Type: "function", Function: llm.ToolFunction{
			Name: "tool", Description: "desc",
			Parameters: json.RawMessage(`{"type":"object","properties":{"b":{"type":"string"},"a":{"type":"integer"}}}`),
		}},
	}
	comps2 := testComponents()
	comps2.ToolSpecs = []llm.Tool{
		{Type: "function", Function: llm.ToolFunction{
			Name: "tool", Description: "desc",
			Parameters: json.RawMessage(`{"type":"object","properties":{"a":{"type":"integer"},"b":{"type":"string"}}}`),
		}},
	}

	m1 := NewEpochManager()
	e1 := m1.InitEpoch("test", comps1)
	m2 := NewEpochManager()
	e2 := m2.InitEpoch("test", comps2)

	if e1.ComponentHashes["tools"] != e2.ComponentHashes["tools"] {
		t.Error("tool schema field order should not change hash")
	}
	if e1.StaticPrefixHash != e2.StaticPrefixHash {
		t.Error("tool schema field order should not change epoch hash")
	}
}

func TestEpochToolDescriptionChangeChangesHash(t *testing.T) {
	comps1 := testComponents()
	comps1.ToolSpecs = []llm.Tool{
		{Type: "function", Function: llm.ToolFunction{Name: "read_file", Description: "Read a file"}},
	}
	comps2 := testComponents()
	comps2.ToolSpecs = []llm.Tool{
		{Type: "function", Function: llm.ToolFunction{Name: "read_file", Description: "Read a file from disk"}},
	}

	m1 := NewEpochManager()
	e1 := m1.InitEpoch("test", comps1)
	m2 := NewEpochManager()
	e2 := m2.InitEpoch("test", comps2)

	if e1.ComponentHashes["tools"] == e2.ComponentHashes["tools"] {
		t.Error("different tool descriptions should change tools hash")
	}
	if e1.StaticPrefixHash == e2.StaticPrefixHash {
		t.Error("different tool descriptions should change epoch hash")
	}
}

// TestEpochProfileChangeDoesNotChangeFingerprint covers the P1 fix: a profile
// change with no change to the model-visible bytes (same system, same tools)
// leaves the Prefix Fingerprint untouched — it is latent capability state, not
// part of the cache key (docs/adr/0001).
func TestEpochProfileChangeDoesNotChangeFingerprint(t *testing.T) {
	comps1 := testComponents()
	comps1.AgentProfileID = "default"
	comps1.Capability.ProfileID = "default"
	comps2 := testComponents()
	comps2.AgentProfileID = "explore"
	comps2.Capability.ProfileID = "explore"

	m1 := NewEpochManager()
	e1 := m1.InitEpoch("test", comps1)
	m2 := NewEpochManager()
	e2 := m2.InitEpoch("test", comps2)

	if e1.StaticPrefixHash != e2.StaticPrefixHash {
		t.Error("profile name must not affect the model-visible Prefix Fingerprint")
	}
}

func TestEpochMutationsAfterFreezeBecomePending(t *testing.T) {
	comps := testComponents()
	m := NewEpochManager()
	m.InitEpoch("test", comps)
	m.FreezeEpoch()

	changedComps := testComponents()
	changedComps.Capability.ProfileID = "explore"
	changes := m.DetectDrift(changedComps)

	if len(changes) != 1 {
		t.Fatalf("expected 1 pending change, got %d", len(changes))
	}
	if changes[0].Kind != PendingAgentProfileChanged {
		t.Errorf("expected PendingAgentProfileChanged, got %s", changes[0].Kind)
	}

	epoch := m.CurrentEpoch()
	if epoch.Capability.ProfileID != "default" {
		t.Error("epoch capability should not have been mutated")
	}
	if len(m.PendingChanges()) != 1 {
		t.Errorf("expected 1 pending change recorded, got %d", len(m.PendingChanges()))
	}
}

func TestEpochMultiplePendingChanges(t *testing.T) {
	comps := testComponents()
	comps.Capability.MCPTools = []mcp.McpToolMeta{
		{Name: "mcp__s__a", Description: "a", InputSchema: json.RawMessage(`{}`)},
	}
	m := NewEpochManager()
	m.InitEpoch("test", comps)
	m.FreezeEpoch()

	changedComps := testComponents()
	changedComps.Capability.ProfileID = "explore"
	changedComps.Capability.MCPTools = []mcp.McpToolMeta{
		{Name: "mcp__s__a", Description: "a", InputSchema: json.RawMessage(`{}`)},
		{Name: "mcp__s__b", Description: "b", InputSchema: json.RawMessage(`{}`)},
	}

	changes := m.DetectDrift(changedComps)
	if len(changes) != 2 {
		t.Fatalf("expected 2 pending changes, got %d: %+v", len(changes), changes)
	}
	kinds := map[PendingChangeKind]bool{}
	for _, c := range changes {
		kinds[c.Kind] = true
	}
	if !kinds[PendingAgentProfileChanged] {
		t.Error("expected PendingAgentProfileChanged")
	}
	if !kinds[PendingMCPToolAdded] {
		t.Error("expected PendingMCPToolAdded")
	}
}

func TestEpochStaticPrefixStableDuringCompaction(t *testing.T) {
	comps := testComponents()
	m := NewEpochManager()
	epoch := m.InitEpoch("test", comps)
	m.FreezeEpoch()

	hashBefore := epoch.StaticPrefixHash

	epoch.FewShots = append(epoch.FewShots, llm.Message{
		Role:   "user",
		Blocks: []llm.ContentBlock{llm.TextBlock{Text: "compact this conversation into a summary"}},
	})

	if epoch.StaticPrefixHash != hashBefore {
		t.Error("modifying conversation messages should not change static prefix hash")
	}

	// Verify frozen snapshot was captured at freeze time.
	if len(epoch.FrozenTools) != len(comps.ToolSpecs) {
		t.Errorf("FrozenTools should match ToolSpecs: got %d, want %d", len(epoch.FrozenTools), len(comps.ToolSpecs))
	}
	if epoch.FrozenSystem != comps.StaticSystem {
		t.Errorf("FrozenSystem should match StaticSystem: got %q, want %q", epoch.FrozenSystem, comps.StaticSystem)
	}

	// Mutating the original ToolSpecs slice should not affect frozen copy.
	epoch.ToolSpecs = append(epoch.ToolSpecs, llm.Tool{Type: "function", Function: llm.ToolFunction{Name: "extra"}})
	if len(epoch.FrozenTools) != len(comps.ToolSpecs) {
		t.Error("FrozenTools must be independent of later ToolSpecs mutations")
	}
}

func TestEpochChildDoesNotMutateParent(t *testing.T) {
	parentComps := testComponents()
	parentMgr := NewEpochManager()
	parentEpoch := parentMgr.InitEpoch("parent", parentComps)
	parentMgr.FreezeEpoch()

	parentHash := parentEpoch.StaticPrefixHash
	parentSystem := parentEpoch.StaticSystem

	childComps := testComponents()
	childComps.StaticSystem = "You are a sub-agent."
	childMgr := NewEpochManager()
	childEpoch := childMgr.InitEpoch("child", childComps)

	if parentEpoch.StaticPrefixHash != parentHash {
		t.Error("child epoch creation should not mutate parent hash")
	}
	if parentEpoch.StaticSystem != parentSystem {
		t.Error("child epoch creation should not mutate parent system")
	}
	if childEpoch.StaticPrefixHash == parentHash {
		t.Error("child and parent should have different hashes")
	}
	if childEpoch.EpochID == parentEpoch.EpochID {
		t.Error("child and parent should have different epoch IDs")
	}
}

func TestEpochExpectedCacheMissFirstTurn(t *testing.T) {
	comps := testComponents()
	m := NewEpochManager()
	m.InitEpoch("test", comps)

	epoch := m.SwitchEpoch("first switch", comps)
	_ = epoch

	if !m.ExpectedCacheMiss() {
		t.Error("first turn of new epoch should have ExpectedCacheMiss = true")
	}
}

func TestEpochExpectedCacheMissSubsequentTurns(t *testing.T) {
	comps := testComponents()
	m := NewEpochManager()
	m.InitEpoch("test", comps)

	m.SwitchEpoch("switch", comps)
	m.ExpectedCacheMiss() // consume the first-turn flag

	if m.ExpectedCacheMiss() {
		t.Error("subsequent turns should have ExpectedCacheMiss = false")
	}
	if m.ExpectedCacheMiss() {
		t.Error("third turn should also have ExpectedCacheMiss = false")
	}
}

func TestEpochSwitchCreatesNewEpoch(t *testing.T) {
	comps := testComponents()
	m := NewEpochManager()
	first := m.InitEpoch("test", comps)
	m.FreezeEpoch()

	changedComps := testComponents()
	changedComps.StaticSystem = "New system prompt."
	second := m.SwitchEpoch("explicit switch", changedComps)

	if second.EpochID == first.EpochID {
		t.Error("switched epoch should have different ID")
	}
	if second.StaticPrefixHash == first.StaticPrefixHash {
		t.Error("switched epoch should have different hash")
	}
	if m.IsFrozen() {
		t.Error("new epoch should not be frozen yet")
	}
	if len(m.PendingChanges()) != 0 {
		t.Error("new epoch should have no pending changes")
	}
}

func TestEpochFreezeIdempotent(t *testing.T) {
	comps := testComponents()
	m := NewEpochManager()
	m.InitEpoch("test", comps)

	m.FreezeEpoch()
	if !m.IsFrozen() {
		t.Error("should be frozen after FreezeEpoch")
	}

	m.FreezeEpoch()
	if !m.IsFrozen() {
		t.Error("double freeze should be idempotent")
	}
}

func TestEpochDetectDriftNoEpoch(t *testing.T) {
	m := NewEpochManager()
	changes := m.DetectDrift(testComponents())
	if changes != nil {
		t.Error("no epoch should return nil changes")
	}
}

func TestEpochDetectDriftNotFrozen(t *testing.T) {
	comps := testComponents()
	m := NewEpochManager()
	m.InitEpoch("test", comps)

	changedComps := testComponents()
	changedComps.Capability.ProfileID = "explore"
	changes := m.DetectDrift(changedComps)
	if changes != nil {
		t.Error("unfrozen epoch should not detect drift")
	}
}

func TestEpochSwitchPublishesEvent(t *testing.T) {
	comps := testComponents()
	bus := NewBus()
	sub := bus.Subscribe(16)

	m := NewEpochManager()
	m.SetBus(bus)
	first := m.InitEpoch("test", comps)
	m.FreezeEpoch()

	changedComps := testComponents()
	changedComps.StaticSystem = "New system."
	second := m.SwitchEpoch("explicit switch", changedComps)

	bus.Unsubscribe(sub)
	var found bool
	for env := range sub.C {
		if ev, ok := env.Event.(EventEpochSwitched); ok {
			found = true
			if ev.OldEpochID != first.EpochID {
				t.Errorf("OldEpochID = %q, want %q", ev.OldEpochID, first.EpochID)
			}
			if ev.NewEpochID != second.EpochID {
				t.Errorf("NewEpochID = %q, want %q", ev.NewEpochID, second.EpochID)
			}
			if ev.StaticPrefixHash != second.StaticPrefixHash {
				t.Error("StaticPrefixHash should match new epoch")
			}
			if ev.Reason != "explicit switch" {
				t.Errorf("Reason = %q, want %q", ev.Reason, "explicit switch")
			}
		}
	}
	if !found {
		t.Error("EventEpochSwitched was not published")
	}
}

func TestEpochSwitchNoBusNoPanic(t *testing.T) {
	comps := testComponents()
	m := NewEpochManager()
	m.InitEpoch("test", comps)

	changedComps := testComponents()
	changedComps.StaticSystem = "New."
	epoch := m.SwitchEpoch("no bus", changedComps)
	if epoch == nil {
		t.Error("SwitchEpoch should return epoch even without bus")
	}
}

func TestSetCacheUnit_IntegratedAlignment(t *testing.T) {
	// Verify that SetCacheUnit flows through buildEpochComponents so the
	// frozen system prompt is padded to a unit boundary.
	a := New(nil, tools.New(), nil, "deepseek-v4-flash")
	a.System = "You are a coding agent."
	const unit = 128
	a.SetCacheUnit(unit)

	comps := a.buildEpochComponents()

	// When the tokenizer is available, the padded system should end on a
	// unit boundary. When unavailable, the code falls through without
	// padding (unit > 0 but Count returns 0 → PadText returns "").
	if tokenizer.Available() {
		n := tokenizer.Count(comps.StaticSystem)
		if n%unit != 0 {
			t.Fatalf("padded system prompt token count = %d, not a multiple of %d", n, unit)
		}
		// The padded system should differ from the unpadded one.
		unpadded := a.staticSystem()
		if comps.StaticSystem == unpadded {
			t.Fatal("buildEpochComponents should pad the system when unit > 0")
		}
	}

	// Verify that identical inputs produce identical padded output (byte-stable).
	comps2 := a.buildEpochComponents()
	if comps.StaticSystem != comps2.StaticSystem {
		t.Fatal("buildEpochComponents should be deterministic for identical inputs")
	}
}

func TestSetCacheUnit_ZeroIsNoop(t *testing.T) {
	// Verify that cacheUnit=0 does NOT change the system prompt.
	a := New(nil, tools.New(), nil, "deepseek-v4-flash")
	a.System = "You are a coding agent."
	a.SetCacheUnit(0)

	comps := a.buildEpochComponents()
	if comps.StaticSystem != a.staticSystem() {
		t.Fatalf("cacheUnit=0 should not change the system prompt\n  got:  %q\n  want: %q", comps.StaticSystem, a.staticSystem())
	}
}
