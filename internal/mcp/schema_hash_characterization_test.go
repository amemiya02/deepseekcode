package mcp

import (
	"encoding/json"
	"testing"
)

// TestSchemaHash_KeyOrderSensitivity_Characterization pins the phantom-drift
// defect that motivates the prefix-fingerprint refactor (see
// docs/refactor-prefix-fingerprint.md and docs/adr/0001-*). The MCP registry
// has TWO ways to answer "did a tool's schema change":
//
//   - SchemaHash() writes InputSchema RAW (registry.go), so reordering
//     JSON-Schema keys flips the hash even though the schema is semantically
//     identical — and that hash feeds the epoch's static-prefix hash, causing a
//     phantom cache miss on an idempotent MCP reconnect.
//   - CompareToolLists() / PendingSchemaChanges() canonicalize (key-sort) via
//     canonicalEqual, so they correctly see NO change.
//
// The two paths therefore DISAGREE on the same input. This test documents that
// disagreement so the M3 consolidation (which removes SchemaHash) is provably a
// behavior fix, not a behavior change.
//
// WHEN M3 REMOVES SchemaHash: delete the SchemaHash assertion below. The
// CompareToolLists / PendingSchemaChanges assertions stay — they are the
// behavior the unified module must preserve.
func TestSchemaHash_KeyOrderSensitivity_Characterization(t *testing.T) {
	// Same schema, object keys in different order at the properties level.
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

	// Canonical path: correctly sees no change (key order is irrelevant).
	if rep := CompareToolLists(before.Tools(), after.Tools()); rep.Kind != DriftNone {
		t.Errorf("CompareToolLists must see no drift on key reorder, got %q (%s)", rep.Kind, rep.Message)
	}
	if changes := after.PendingSchemaChanges(before.Tools()); len(changes) != 0 {
		t.Errorf("PendingSchemaChanges must see no change on key reorder, got %d: %+v", len(changes), changes)
	}

	// Buggy path (DELETE this block in M3 when SchemaHash is removed):
	// SchemaHash is key-order-sensitive, so it disagrees with the canonical path.
	if before.SchemaHash() == after.SchemaHash() {
		t.Error("CHARACTERIZATION CHANGED: SchemaHash is no longer key-order-sensitive — " +
			"the phantom-drift bug appears fixed. If M3 did this, remove this assertion and " +
			"mark ADR-0001 as implemented.")
	}
}
