package agent

import (
	"encoding/json"
	"testing"

	"github.com/amemiya02/deepseekcode/internal/llm"
)

func testTools() []llm.Tool {
	return []llm.Tool{
		{Type: "function", Function: llm.ToolFunction{Name: "read_file", Description: "Read a file", Parameters: json.RawMessage(`{"type":"object"}`)}},
		{Type: "function", Function: llm.ToolFunction{Name: "bash", Description: "Run a command", Parameters: json.RawMessage(`{"type":"object"}`)}},
	}
}

func testComponents() EpochComponents {
	return EpochComponents{
		StaticSystem:    "You are a coding agent.",
		ToolSpecs:       testTools(),
		Model:           "deepseek-v4-flash",
		AgentProfileID:  "default",
		StableSkillDir:  "/skills/v1",
		MCPSchemaSnapshot: "mcp-schema-v1",
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

func TestEpochMutationsAfterFreezeBecomePending(t *testing.T) {
	comps := testComponents()
	m := NewEpochManager()
	m.InitEpoch("test", comps)
	m.FreezeEpoch()

	changedComps := testComponents()
	changedComps.StaticSystem = "You are a DIFFERENT agent."
	changes := m.DetectDrift(changedComps)

	if len(changes) != 1 {
		t.Fatalf("expected 1 pending change, got %d", len(changes))
	}
	if changes[0].Kind != PendingSystemChanged {
		t.Errorf("expected PendingSystemChanged, got %s", changes[0].Kind)
	}

	epoch := m.CurrentEpoch()
	if epoch.StaticSystem != "You are a coding agent." {
		t.Error("epoch should not have been mutated")
	}
	if len(m.PendingChanges()) != 1 {
		t.Errorf("expected 1 pending change recorded, got %d", len(m.PendingChanges()))
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

func TestEpochSkillBodyChangeNotLiveEpoch(t *testing.T) {
	comps := testComponents()
	comps.StableSkillDir = "/skills/v1"
	m := NewEpochManager()
	m.InitEpoch("test", comps)
	m.FreezeEpoch()

	changedComps := testComponents()
	changedComps.StableSkillDir = "/skills/v2"
	changes := m.DetectDrift(changedComps)

	if len(changes) != 1 {
		t.Fatalf("expected 1 pending change, got %d", len(changes))
	}
	if changes[0].Kind != PendingSkillBodyChanged {
		t.Errorf("expected PendingSkillBodyChanged, got %s", changes[0].Kind)
	}

	epoch := m.CurrentEpoch()
	if epoch.StableSkillDir != "/skills/v1" {
		t.Error("epoch should retain original skill dir")
	}
	if epoch.ComponentHashes["skill_dir"] == computeComponentHashes(changedComps).SkillDirSHA256 {
		t.Error("epoch hash should not have changed")
	}
}

func TestEpochMCPSchemaReconnectStable(t *testing.T) {
	comps1 := testComponents()
	comps1.MCPSchemaSnapshot = `{"tools":[{"name":"search"}]}`
	comps2 := testComponents()
	comps2.MCPSchemaSnapshot = `{"tools":[{"name":"search"}]}`

	m1 := NewEpochManager()
	e1 := m1.InitEpoch("test", comps1)
	m2 := NewEpochManager()
	e2 := m2.InitEpoch("test", comps2)

	if e1.ComponentHashes["mcp_schema"] != e2.ComponentHashes["mcp_schema"] {
		t.Error("same MCP schema should produce same hash")
	}
	if e1.StaticPrefixHash != e2.StaticPrefixHash {
		t.Error("same MCP schema should produce same epoch hash")
	}
}

func TestEpochMCPSchemaDriftPending(t *testing.T) {
	comps := testComponents()
	comps.MCPSchemaSnapshot = `{"tools":[{"name":"search"}]}`
	m := NewEpochManager()
	m.InitEpoch("test", comps)
	m.FreezeEpoch()

	changedComps := testComponents()
	changedComps.MCPSchemaSnapshot = `{"tools":[{"name":"search"},{"name":"fetch"}]}`
	changes := m.DetectDrift(changedComps)

	if len(changes) != 1 {
		t.Fatalf("expected 1 pending change, got %d", len(changes))
	}
	if changes[0].Kind != PendingMCPToolSchemaChanged {
		t.Errorf("expected PendingMCPToolSchemaChanged, got %s", changes[0].Kind)
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
	changedComps.StaticSystem = "Different."
	changes := m.DetectDrift(changedComps)
	if changes != nil {
		t.Error("unfrozen epoch should not detect drift")
	}
}

func TestEpochMultiplePendingChanges(t *testing.T) {
	comps := testComponents()
	m := NewEpochManager()
	m.InitEpoch("test", comps)
	m.FreezeEpoch()

	changedComps := testComponents()
	changedComps.StaticSystem = "New system."
	changedComps.ToolSpecs = []llm.Tool{
		{Type: "function", Function: llm.ToolFunction{Name: "new_tool", Description: "new"}},
	}
	changedComps.StableSkillDir = "/skills/v2"

	changes := m.DetectDrift(changedComps)
	if len(changes) != 3 {
		t.Fatalf("expected 3 pending changes, got %d", len(changes))
	}

	kinds := map[PendingChangeKind]bool{}
	for _, c := range changes {
		kinds[c.Kind] = true
	}
	if !kinds[PendingSystemChanged] {
		t.Error("expected PendingSystemChanged")
	}
	if !kinds[PendingToolSchemaChanged] {
		t.Error("expected PendingToolSchemaChanged")
	}
	if !kinds[PendingSkillBodyChanged] {
		t.Error("expected PendingSkillBodyChanged")
	}
}

func TestComputeEpochHashConsistency(t *testing.T) {
	ch := llm.EpochComponentHashes{
		SystemSHA256:       "aaa",
		ToolsSHA256:        "bbb",
		SkillDirSHA256:     "ccc",
		MCPSchemaSHA256:    "ddd",
		AgentProfileSHA256: "eee",
		FewShotsSHA256:     "fff",
	}
	h1 := llm.ComputeEpochHash(ch)
	h2 := llm.ComputeEpochHash(ch)
	if h1 != h2 {
		t.Error("ComputeEpochHash should be deterministic")
	}
	if h1 == "" {
		t.Error("hash should not be empty")
	}
}

func TestComputeEpochHashDifferentInputs(t *testing.T) {
	ch1 := llm.EpochComponentHashes{SystemSHA256: "aaa"}
	ch2 := llm.EpochComponentHashes{SystemSHA256: "bbb"}
	h1 := llm.ComputeEpochHash(ch1)
	h2 := llm.ComputeEpochHash(ch2)
	if h1 == h2 {
		t.Error("different inputs should produce different hashes")
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

func TestHashFewShotsDistinguishesBlockTypes(t *testing.T) {
	textOnly := []llm.Message{{
		Role:   "user",
		Blocks: []llm.ContentBlock{llm.TextBlock{Text: "hello"}},
	}}
	thinkingOnly := []llm.Message{{
		Role:   "user",
		Blocks: []llm.ContentBlock{llm.ThinkingBlock{Text: "hello"}},
	}}
	h1 := hashFewShots(textOnly)
	h2 := hashFewShots(thinkingOnly)
	if h1 == h2 {
		t.Error("TextBlock and ThinkingBlock with same text should produce different hashes")
	}
}

func TestEpochSkillHashStableAcrossReloads(t *testing.T) {
	comps := testComponents()
	comps.StableSkillDir = "sha256-of-index-text"

	m1 := NewEpochManager()
	e1 := m1.InitEpoch("test", comps)

	comps2 := testComponents()
	comps2.StableSkillDir = "sha256-of-index-text"

	m2 := NewEpochManager()
	e2 := m2.InitEpoch("test", comps2)

	if e1.ComponentHashes["skill_dir"] != e2.ComponentHashes["skill_dir"] {
		t.Error("same skill VersionHash should produce same component hash")
	}
}

func TestEpochSkillHashDriftEntersPending(t *testing.T) {
	comps := testComponents()
	comps.StableSkillDir = "hash-v1"
	m := NewEpochManager()
	m.InitEpoch("test", comps)
	m.FreezeEpoch()

	changedComps := testComponents()
	changedComps.StableSkillDir = "hash-v2"
	changes := m.DetectDrift(changedComps)

	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(changes))
	}
	if changes[0].Kind != PendingSkillBodyChanged {
		t.Errorf("expected PendingSkillBodyChanged, got %s", changes[0].Kind)
	}
}

func TestEpochMCPSchemaHashStable(t *testing.T) {
	comps := testComponents()
	comps.MCPSchemaSnapshot = "sha256-of-mcp-schemas"

	m1 := NewEpochManager()
	e1 := m1.InitEpoch("test", comps)

	comps2 := testComponents()
	comps2.MCPSchemaSnapshot = "sha256-of-mcp-schemas"

	m2 := NewEpochManager()
	e2 := m2.InitEpoch("test", comps2)

	if e1.ComponentHashes["mcp_schema"] != e2.ComponentHashes["mcp_schema"] {
		t.Error("same MCP SchemaHash should produce same component hash")
	}
	if e1.StaticPrefixHash != e2.StaticPrefixHash {
		t.Error("same MCP SchemaHash should produce same epoch hash")
	}
}

func TestEpochMCPSchemaDriftEntersPending(t *testing.T) {
	comps := testComponents()
	comps.MCPSchemaSnapshot = "schema-hash-v1"
	m := NewEpochManager()
	m.InitEpoch("test", comps)
	m.FreezeEpoch()

	changedComps := testComponents()
	changedComps.MCPSchemaSnapshot = "schema-hash-v2"
	changes := m.DetectDrift(changedComps)

	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(changes))
	}
	if changes[0].Kind != PendingMCPToolSchemaChanged {
		t.Errorf("expected PendingMCPToolSchemaChanged, got %s", changes[0].Kind)
	}
}

func TestEpochSkillBodyChangeDoesNotAlterLiveEpoch(t *testing.T) {
	comps := testComponents()
	comps.StableSkillDir = "skills-hash-v1"
	m := NewEpochManager()
	m.InitEpoch("test", comps)
	m.FreezeEpoch()

	epoch := m.CurrentEpoch()
	originalHash := epoch.StaticPrefixHash

	changedComps := testComponents()
	changedComps.StableSkillDir = "skills-hash-v2"
	m.DetectDrift(changedComps)

	if epoch.StaticPrefixHash != originalHash {
		t.Error("live epoch hash should not change after drift detection")
	}
	if epoch.StableSkillDir != "skills-hash-v1" {
		t.Error("live epoch skill hash should not change after drift detection")
	}
}

func TestEpochMCPReconnectSameSchemaNoDrift(t *testing.T) {
	comps := testComponents()
	comps.MCPSchemaSnapshot = "stable-schema-hash"
	m := NewEpochManager()
	m.InitEpoch("test", comps)
	m.FreezeEpoch()

	sameComps := testComponents()
	sameComps.MCPSchemaSnapshot = "stable-schema-hash"
	changes := m.DetectDrift(sameComps)

	if len(changes) != 0 {
		t.Errorf("reconnect with same schema should produce 0 changes, got %d", len(changes))
	}
}
