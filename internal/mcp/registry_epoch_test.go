package mcp

import (
	"encoding/json"
	"testing"
)

// The SchemaHash unit tests were removed in M3 — SchemaHash is gone
// (docs/adr/0001). MCP schema-change detection is covered by the
// PendingSchemaChanges tests below and the key-reorder regression in
// schema_hash_characterization_test.go.

func TestPendingSchemaChangesNoDrift(t *testing.T) {
	reg := NewRegistry()
	reg.mu.Lock()
	reg.servers["s1"] = &ServerProxy{
		Name:  "s1",
		State: StateConnected,
		Tools: []McpToolMeta{
			{Name: "echo", Description: "echoes", InputSchema: json.RawMessage(`{}`)},
		},
	}
	reg.mu.Unlock()

	oldTools := reg.Tools()
	changes := reg.PendingSchemaChanges(oldTools)
	if len(changes) != 0 {
		t.Errorf("expected 0 changes for same tools, got %d", len(changes))
	}
}

func TestPendingSchemaChangesToolAdded(t *testing.T) {
	reg := NewRegistry()
	reg.mu.Lock()
	reg.servers["s1"] = &ServerProxy{
		Name:  "s1",
		State: StateConnected,
		Tools: []McpToolMeta{
			{Name: "echo", Description: "echoes", InputSchema: json.RawMessage(`{}`)},
		},
	}
	reg.mu.Unlock()

	oldTools := reg.Tools()

	reg.mu.Lock()
	reg.servers["s1"] = &ServerProxy{
		Name:  "s1",
		State: StateConnected,
		Tools: []McpToolMeta{
			{Name: "echo", Description: "echoes", InputSchema: json.RawMessage(`{}`)},
			{Name: "fetch", Description: "fetches", InputSchema: json.RawMessage(`{}`)},
		},
	}
	reg.mu.Unlock()

	changes := reg.PendingSchemaChanges(oldTools)
	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(changes))
	}
	if changes[0].Kind != "tool_added" {
		t.Errorf("expected 'tool_added', got %q", changes[0].Kind)
	}
	if changes[0].ToolName != "mcp__s1__fetch" {
		t.Errorf("expected 'mcp__s1__fetch', got %q", changes[0].ToolName)
	}
}

func TestPendingSchemaChangesToolRemoved(t *testing.T) {
	reg := NewRegistry()
	reg.mu.Lock()
	reg.servers["s1"] = &ServerProxy{
		Name:  "s1",
		State: StateConnected,
		Tools: []McpToolMeta{
			{Name: "echo", Description: "echoes", InputSchema: json.RawMessage(`{}`)},
			{Name: "add", Description: "adds", InputSchema: json.RawMessage(`{}`)},
		},
	}
	reg.mu.Unlock()

	oldTools := reg.Tools()

	reg.mu.Lock()
	reg.servers["s1"] = &ServerProxy{
		Name:  "s1",
		State: StateConnected,
		Tools: []McpToolMeta{
			{Name: "echo", Description: "echoes", InputSchema: json.RawMessage(`{}`)},
		},
	}
	reg.mu.Unlock()

	changes := reg.PendingSchemaChanges(oldTools)
	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(changes))
	}
	if changes[0].Kind != "tool_removed" {
		t.Errorf("expected 'tool_removed', got %q", changes[0].Kind)
	}
}

func TestPendingSchemaChangesSchemaChanged(t *testing.T) {
	reg := NewRegistry()
	reg.mu.Lock()
	reg.servers["s1"] = &ServerProxy{
		Name:  "s1",
		State: StateConnected,
		Tools: []McpToolMeta{
			{Name: "echo", Description: "echoes", InputSchema: json.RawMessage(`{"type":"object"}`)},
		},
	}
	reg.mu.Unlock()

	oldTools := reg.Tools()

	reg.mu.Lock()
	reg.servers["s1"] = &ServerProxy{
		Name:  "s1",
		State: StateConnected,
		Tools: []McpToolMeta{
			{Name: "echo", Description: "echoes v2", InputSchema: json.RawMessage(`{"type":"object","properties":{"text":{"type":"string"}}}`)},
		},
	}
	reg.mu.Unlock()

	changes := reg.PendingSchemaChanges(oldTools)
	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(changes))
	}
	if changes[0].Kind != "tool_schema_changed" {
		t.Errorf("expected 'tool_schema_changed', got %q", changes[0].Kind)
	}
}

func TestPendingSchemaChangesReconnectStable(t *testing.T) {
	makeReg := func() *Registry {
		reg := NewRegistry()
		reg.mu.Lock()
		reg.servers["s1"] = &ServerProxy{
			Name:  "s1",
			State: StateConnected,
			Tools: []McpToolMeta{
				{Name: "echo", Description: "echoes", InputSchema: json.RawMessage(`{}`)},
			},
		}
		reg.mu.Unlock()
		return reg
	}

	r1 := makeReg()
	oldTools := r1.Tools()
	r2 := makeReg()

	changes := r2.PendingSchemaChanges(oldTools)
	if len(changes) != 0 {
		t.Errorf("reconnect with same schema should produce 0 changes, got %d", len(changes))
	}
}
