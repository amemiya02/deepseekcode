package agent

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/amemiya02/deepseekcode/internal/llm"
)

// epochSeq is a process-wide monotonic counter for unique epoch IDs.
var epochSeq atomic.Int64

// PrefixEpoch is a frozen model-visible prefix snapshot.
// Once frozen (after first model request), it cannot change.
// Changes to tools, skills, MCP, system prompt, etc. become
// pending changes that are visible in receipts but not model-visible
// until an explicit epoch switch.
type PrefixEpoch struct {
	EpochID         string
	AgentProfileID  string
	Model           string
	ReasoningEffort string
	StaticSystem    string
	FewShots        []llm.Message
	ToolSpecs       []llm.Tool
	// Capability is the latent identity (profile/skills/MCP) frozen with this
	// epoch. It drives pending-change detection but is NOT in StaticPrefixHash.
	Capability      CapabilitySet
	CreatedAt       time.Time
	CreatedReason   string
	ComponentHashes map[string]string
	// StaticPrefixHash is the Prefix Fingerprint: the canonical hash of the
	// model-visible bytes (system + tools) — the DeepSeek cache key. Latent
	// capability state is intentionally excluded (see docs/adr/0001).
	StaticPrefixHash string

	// FrozenTools and FrozenSystem capture the tool list and system
	// prompt at the moment FreezeEpoch is called. When the epoch is
	// frozen, runStep and maybeCompact MUST use these instead of the
	// live values to guarantee cache-stable prefixes.
	FrozenTools  []llm.Tool
	FrozenSystem string
}

// EpochComponents is the input for creating a PrefixEpoch. StaticSystem,
// ToolSpecs (and, when folded in, FewShots) are the model-visible bytes that
// determine the Prefix Fingerprint; Capability is the latent identity used only
// for pending-change detection.
type EpochComponents struct {
	AgentProfileID  string
	Model           string
	ReasoningEffort string
	StaticSystem    string
	FewShots        []llm.Message
	ToolSpecs       []llm.Tool
	Capability      CapabilitySet
}

// PendingChangeKind identifies the type of change detected after an
// epoch was frozen.
type PendingChangeKind string

const (
	PendingSystemChanged        PendingChangeKind = "system_changed"
	PendingToolAdded            PendingChangeKind = "tool_added"
	PendingToolRemoved          PendingChangeKind = "tool_removed"
	PendingToolSchemaChanged    PendingChangeKind = "tool_schema_changed"
	PendingSkillAdded           PendingChangeKind = "skill_added"
	PendingSkillRemoved         PendingChangeKind = "skill_removed"
	PendingSkillBodyChanged     PendingChangeKind = "skill_body_changed"
	PendingMCPToolAdded         PendingChangeKind = "mcp_tool_added"
	PendingMCPToolRemoved       PendingChangeKind = "mcp_tool_removed"
	PendingMCPToolSchemaChanged PendingChangeKind = "mcp_tool_schema_changed"
	PendingAgentProfileChanged  PendingChangeKind = "agent_profile_changed"
	PendingFewShotsChanged      PendingChangeKind = "few_shots_changed"
)

// PendingChange is a detected mutation that is blocked from
// model-visibility until an explicit epoch switch.
type PendingChange struct {
	Kind        PendingChangeKind
	Description string
	DetectedAt  time.Time
}

// EpochManager manages PrefixEpoch lifecycle.
type EpochManager struct {
	mu              sync.Mutex
	current         *PrefixEpoch
	frozen          bool
	pending         []PendingChange
	expectCacheMiss bool
	bus             *Bus
}

func NewEpochManager() *EpochManager {
	return &EpochManager{}
}

// SetBus attaches an event bus for epoch lifecycle events.
func (m *EpochManager) SetBus(bus *Bus) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.bus = bus
}

// CreateEpoch builds a new PrefixEpoch from components but does not
// make it current. Use SwitchEpoch or the initial CreateEpoch path.
func (m *EpochManager) CreateEpoch(reason string, components EpochComponents) *PrefixEpoch {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.createEpochLocked(reason, components)
}

// InitEpoch creates and sets the initial epoch. Called once at session
// start. Panics if called when an epoch already exists.
func (m *EpochManager) InitEpoch(reason string, components EpochComponents) *PrefixEpoch {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.current != nil {
		panic("epoch already initialized")
	}
	epoch := m.createEpochLocked(reason, components)
	m.current = epoch
	return epoch
}

func (m *EpochManager) createEpochLocked(reason string, components EpochComponents) *PrefixEpoch {
	// The Prefix Fingerprint is the canonical hash of the model-visible bytes
	// only (system + tools). Latent capability state (profile/skills/MCP) is
	// snapshotted on the epoch for pending-change detection but never hashed in.
	fp := llm.StaticPrefix{System: components.StaticSystem, Tools: components.ToolSpecs}.Fingerprint()
	now := time.Now()
	seq := epochSeq.Add(1)
	return &PrefixEpoch{
		EpochID:          fmt.Sprintf("epoch_%d_%d", now.UnixMilli(), seq),
		AgentProfileID:   components.AgentProfileID,
		Model:            components.Model,
		ReasoningEffort:  components.ReasoningEffort,
		StaticSystem:     components.StaticSystem,
		FewShots:         components.FewShots,
		ToolSpecs:        components.ToolSpecs,
		Capability:       components.Capability,
		CreatedAt:        now,
		CreatedReason:    reason,
		ComponentHashes:  map[string]string{"system": fp.SystemSHA256, "tools": fp.ToolsSHA256},
		StaticPrefixHash: fp.CombinedSHA256,
	}
}

// FreezeEpoch marks the epoch as immutable after first model request
// and captures FrozenTools/FrozenSystem from the current epoch.
func (m *EpochManager) FreezeEpoch() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.current != nil {
		m.current.FrozenTools = m.current.ToolSpecs
		m.current.FrozenSystem = m.current.StaticSystem
	}
	m.frozen = true
}

// IsFrozen reports whether the epoch is frozen.
func (m *EpochManager) IsFrozen() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.frozen
}

// RecordPendingChange records a mutation that occurred after the epoch
// was frozen. The change is not applied to the current epoch.
func (m *EpochManager) RecordPendingChange(change PendingChange) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pending = append(m.pending, change)
}

// PendingChanges returns a copy of the pending changes list.
func (m *EpochManager) PendingChanges() []PendingChange {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]PendingChange, len(m.pending))
	copy(out, m.pending)
	return out
}

// SwitchEpoch creates a new epoch, makes it current, and resets the
// frozen/pending state. The first turn of the new epoch will report
// ExpectedCacheMiss() = true.
func (m *EpochManager) SwitchEpoch(reason string, components EpochComponents) *PrefixEpoch {
	m.mu.Lock()
	oldEpoch := m.current
	epoch := m.createEpochLocked(reason, components)
	m.current = epoch
	m.frozen = false
	m.pending = nil
	m.expectCacheMiss = true
	bus := m.bus
	m.mu.Unlock()

	if bus != nil {
		var oldID string
		if oldEpoch != nil {
			oldID = oldEpoch.EpochID
		}
		bus.Publish(EventEpochSwitched{
			OldEpochID:       oldID,
			NewEpochID:       epoch.EpochID,
			StaticPrefixHash: epoch.StaticPrefixHash,
			ToolsHash:        epoch.ComponentHashes["tools"],
			Reason:           reason,
		})
	}
	return epoch
}

// CurrentEpoch returns the current epoch. Returns nil if no epoch has
// been initialized.
func (m *EpochManager) CurrentEpoch() *PrefixEpoch {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.current
}

// ExpectedCacheMiss returns true on the first turn after an epoch
// switch. Returns false on subsequent turns. Call once per turn — it
// clears the flag on read.
func (m *EpochManager) ExpectedCacheMiss() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.expectCacheMiss {
		m.expectCacheMiss = false
		return true
	}
	return false
}

// DetectDrift records the latent capability deltas (profile / skills / MCP)
// between the frozen epoch and the live components as pending changes, using
// canonical comparisons. Model-visible byte drift is NOT detected here — it is
// caught per turn by llm.PrefixMonitor and treated as a bug, not a pending
// change. Returns the newly detected changes. See docs/adr/0001.
func (m *EpochManager) DetectDrift(components EpochComponents) []PendingChange {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.current == nil || !m.frozen {
		return nil
	}

	changes := CapabilityDiff(m.current.Capability, components.Capability)
	m.pending = append(m.pending, changes...)
	return changes
}
