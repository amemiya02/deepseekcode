package agent

import (
	"time"

	"github.com/amemiya02/deepseekcode/internal/mcp"
	"github.com/amemiya02/deepseekcode/internal/skills"
)

// CapabilitySet is the latent capability identity behind a PrefixEpoch: the
// inputs that determine *which* StaticPrefix gets built (the active agent
// profile, the skill catalog, the connected MCP tools) but which are not
// themselves the model-visible bytes. EpochManager watches it to record pending
// changes; it is deliberately NOT part of the Prefix Fingerprint — see
// /CONTEXT.md and docs/adr/0001-prefix-fingerprint-is-model-visible-bytes-only.
//
// Skill and active-MCP changes also move the fingerprint (the skill directory
// is rendered into the system prompt; active MCP tools are in the tool set), so
// they are reported here as one fine-grained pending change rather than also as
// a raw "system"/"tools" change — that is the double-report the cache-epoch
// review flagged.
type CapabilitySet struct {
	ProfileID string
	Skills    *skills.Store     // nil when no skill store is configured
	MCPTools  []mcp.McpToolMeta // nil when no MCP servers are connected
}

// CapabilityDiff returns the pending changes between two capability sets using
// canonical comparisons (skills.Store.Diff and mcp.CompareToolLists), so
// cache-irrelevant noise — a reordered MCP tool list, or reordered JSON-Schema
// keys within a tool — never registers as a change.
func CapabilityDiff(oldCS, newCS CapabilitySet) []PendingChange {
	now := time.Now()
	var changes []PendingChange

	if oldCS.ProfileID != newCS.ProfileID {
		changes = append(changes, PendingChange{
			Kind:        PendingAgentProfileChanged,
			Description: "agent profile changed: " + oldCS.ProfileID + " -> " + newCS.ProfileID,
			DetectedAt:  now,
		})
	}

	if newCS.Skills != nil {
		for _, sc := range newCS.Skills.Diff(oldCS.Skills) {
			kind := PendingSkillBodyChanged
			switch sc.Kind {
			case "added":
				kind = PendingSkillAdded
			case "removed":
				kind = PendingSkillRemoved
			}
			changes = append(changes, PendingChange{
				Kind:        kind,
				Description: "skill " + sc.Kind + ": " + sc.Name,
				DetectedAt:  now,
			})
		}
	}

	if rep := mcp.CompareToolLists(oldCS.MCPTools, newCS.MCPTools); rep.Kind != mcp.DriftNone {
		kind := PendingMCPToolSchemaChanged
		switch rep.Kind {
		case mcp.DriftAdded:
			kind = PendingMCPToolAdded
		case mcp.DriftRemoved:
			kind = PendingMCPToolRemoved
		}
		changes = append(changes, PendingChange{
			Kind:        kind,
			Description: "mcp: " + rep.Message,
			DetectedAt:  now,
		})
	}

	return changes
}

// buildCapabilitySet snapshots the agent's current latent capability identity.
func (a *Agent) buildCapabilitySet() CapabilitySet {
	cs := CapabilitySet{ProfileID: a.currentProfileID(), Skills: a.Skills}
	if a.MCPRegistry != nil {
		cs.MCPTools = a.MCPRegistry.Tools()
	}
	return cs
}
