package mcp

import (
	"encoding/json"
	"testing"
)

// TestMCPSchemaKeyReorder_NoPhantomDrift is the regression for the phantom-drift
// defect fixed in M3 (docs/refactor-prefix-fingerprint.md, docs/adr/0001). The
// raw, key-order-sensitive SchemaHash was removed; MCP schema identity is now
// judged only by the canonical paths (CompareToolLists / PendingSchemaChanges).
// A reconnect that re-emits the same tool with reordered JSON-Schema keys must
// therefore produce zero drift and zero pending change.
//
// (Before M3 this file characterized the opposite: SchemaHash flipping on a key
// reorder while the canonical paths saw no change — the two-path disagreement.)
func TestMCPSchemaKeyReorder_NoPhantomDrift(t *testing.T) {
	schemaAB := json.RawMessage(`{"type":"object","properties":{"alpha":{"type":"string"},"beta":{"type":"number"}}}`)
	schemaBA := json.RawMessage(`{"properties":{"beta":{"type":"number"},"alpha":{"type":"string"}},"type":"object"}`)

	mk := func(schema json.RawMessage) *Registry {
		reg := NewRegistry()
		reg.mu.Lock()
		reg.servers["s1"] = &ServerProxy{
			Name:  "s1",
			State: StateConnected,
			Tools: []McpToolMeta{{Name: "search", Description: "search the web", InputSchema: schema}},
		}
		reg.mu.Unlock()
		return reg
	}
	before := mk(schemaAB)
	after := mk(schemaBA)

	if rep := CompareToolLists(before.Tools(), after.Tools()); rep.Kind != DriftNone {
		t.Errorf("CompareToolLists must see no drift on key reorder, got %q (%s)", rep.Kind, rep.Message)
	}
	if changes := after.PendingSchemaChanges(before.Tools()); len(changes) != 0 {
		t.Errorf("PendingSchemaChanges must see no change on key reorder, got %d: %+v", len(changes), changes)
	}
}
