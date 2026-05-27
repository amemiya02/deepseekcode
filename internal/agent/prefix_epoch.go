package agent

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/amemiya02/deepseekcode/internal/llm"
)

// PrefixEpoch is a frozen model-visible prefix snapshot.
// Once frozen (after first model request), it cannot change.
// Changes to tools, skills, MCP, system prompt, etc. become
// pending changes that are visible in receipts but not model-visible
// until an explicit epoch switch.
type PrefixEpoch struct {
	EpochID          string
	AgentProfileID   string
	Model            string
	ReasoningEffort  string
	StaticSystem     string
	FewShots         []llm.Message
	ToolSpecs        []llm.Tool
	StableSkillDir   string
	MCPSchemaSnapshot string
	CreatedAt        time.Time
	CreatedReason    string
	ComponentHashes  map[string]string
	StaticPrefixHash string

	// FrozenTools and FrozenSystem capture the tool list and system
	// prompt at the moment FreezeEpoch is called. When the epoch is
	// frozen, runStep and maybeCompact MUST use these instead of the
	// live values to guarantee cache-stable prefixes.
	FrozenTools  []llm.Tool
	FrozenSystem string
}

// EpochComponents is the input for creating a PrefixEpoch.
type EpochComponents struct {
	AgentProfileID   string
	Model            string
	ReasoningEffort  string
	StaticSystem     string
	FewShots         []llm.Message
	ToolSpecs        []llm.Tool
	StableSkillDir   string
	MCPSchemaSnapshot string
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
	ch := computeComponentHashes(components)
	epochHash := llm.ComputeEpochHash(ch)
	now := time.Now()
	componentHashes := map[string]string{
		"system":        ch.SystemSHA256,
		"tools":         ch.ToolsSHA256,
		"skill_dir":     ch.SkillDirSHA256,
		"mcp_schema":    ch.MCPSchemaSHA256,
		"agent_profile": ch.AgentProfileSHA256,
		"few_shots":     ch.FewShotsSHA256,
	}
	return &PrefixEpoch{
		EpochID:           fmt.Sprintf("epoch_%d", now.UnixNano()),
		AgentProfileID:    components.AgentProfileID,
		Model:             components.Model,
		ReasoningEffort:   components.ReasoningEffort,
		StaticSystem:      components.StaticSystem,
		FewShots:          components.FewShots,
		ToolSpecs:         components.ToolSpecs,
		StableSkillDir:    components.StableSkillDir,
		MCPSchemaSnapshot: components.MCPSchemaSnapshot,
		CreatedAt:         now,
		CreatedReason:     reason,
		ComponentHashes:   componentHashes,
		StaticPrefixHash:  epochHash,
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

// DetectDrift compares the current component state against the frozen
// epoch and records any differences as pending changes. Returns the
// list of newly detected changes.
func (m *EpochManager) DetectDrift(components EpochComponents) []PendingChange {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.current == nil || !m.frozen {
		return nil
	}

	ch := computeComponentHashes(components)
	var changes []PendingChange

	if ch.SystemSHA256 != m.current.ComponentHashes["system"] {
		changes = append(changes, PendingChange{
			Kind:        PendingSystemChanged,
			Description: "system prompt changed",
			DetectedAt:  time.Now(),
		})
	}
	if ch.ToolsSHA256 != m.current.ComponentHashes["tools"] {
		changes = append(changes, PendingChange{
			Kind:        PendingToolSchemaChanged,
			Description: "tool set changed",
			DetectedAt:  time.Now(),
		})
	}
	if ch.SkillDirSHA256 != m.current.ComponentHashes["skill_dir"] {
		changes = append(changes, PendingChange{
			Kind:        PendingSkillBodyChanged,
			Description: "skill directory changed",
			DetectedAt:  time.Now(),
		})
	}
	if ch.MCPSchemaSHA256 != m.current.ComponentHashes["mcp_schema"] {
		changes = append(changes, PendingChange{
			Kind:        PendingMCPToolSchemaChanged,
			Description: "MCP schema changed",
			DetectedAt:  time.Now(),
		})
	}
	if ch.AgentProfileSHA256 != m.current.ComponentHashes["agent_profile"] {
		changes = append(changes, PendingChange{
			Kind:        PendingAgentProfileChanged,
			Description: "agent profile changed",
			DetectedAt:  time.Now(),
		})
	}
	if ch.FewShotsSHA256 != m.current.ComponentHashes["few_shots"] {
		changes = append(changes, PendingChange{
			Kind:        PendingFewShotsChanged,
			Description: "few-shots changed",
			DetectedAt:  time.Now(),
		})
	}

	m.pending = append(m.pending, changes...)
	return changes
}

func computeComponentHashes(c EpochComponents) llm.EpochComponentHashes {
	fp := llm.ComputeFingerprint(llm.PrefixInput{
		SystemPrompt: c.StaticSystem,
		Tools:        c.ToolSpecs,
	})
	return llm.EpochComponentHashes{
		SystemSHA256:       fp.SystemSHA256,
		ToolsSHA256:        fp.ToolsSHA256,
		SkillDirSHA256:     sha256hex(c.StableSkillDir),
		MCPSchemaSHA256:    sha256hex(c.MCPSchemaSnapshot),
		AgentProfileSHA256: sha256hex(c.AgentProfileID),
		FewShotsSHA256:     hashFewShots(c.FewShots),
	}
}

func hashFewShots(msgs []llm.Message) string {
	if len(msgs) == 0 {
		return sha256hex("")
	}
	var sb strings.Builder
	for _, m := range msgs {
		sb.WriteString(m.Role)
		sb.WriteByte(':')
		for _, b := range m.Blocks {
			switch tb := b.(type) {
			case llm.TextBlock:
				sb.WriteString("text:")
				sb.WriteString(tb.Text)
			case llm.ThinkingBlock:
				sb.WriteString("thinking:")
				sb.WriteString(tb.Text)
			case llm.ToolUseBlock:
				sb.WriteString("tool_use:")
				sb.WriteString(tb.Name)
				sb.WriteByte(':')
				sb.WriteString(string(tb.Input))
			case llm.ToolResultBlock:
				sb.WriteString("tool_result:")
				sb.WriteString(tb.ToolUseID)
				sb.WriteByte(':')
				sb.WriteString(tb.Content)
			default:
				sb.WriteString("unknown:")
				sb.WriteString(fmt.Sprintf("%T", b))
			}
		}
	}
	return sha256hex(sb.String())
}

func sha256hex(s string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(s)))
}
