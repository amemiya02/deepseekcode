package mcp

import (
	"encoding/json"
	"testing"
)

func TestSchemaHashEmpty(t *testing.T) {
	reg := NewRegistry()
	h := reg.SchemaHash()
	if h == "" {
		t.Error("SchemaHash should not be empty even for empty registry")
	}
}

func TestSchemaHashDeterministic(t *testing.T) {
	reg := NewRegistry()
	reg.mu.Lock()
	reg.servers["s1"] = &ServerProxy{
		Name:  "s1",
		State: StateConnected,
		Tools: []McpToolMeta{
			{Name: "echo", Description: "echoes", InputSchema: json.RawMessage(`{"type":"object"}`)},
			{Name: "add", Description: "adds", InputSchema: json.RawMessage(`{"type":"object","properties":{"a":{"type":"number"}}}`)},
		},
	}
	reg.mu.Unlock()

	h1 := reg.SchemaHash()
	h2 := reg.SchemaHash()
	if h1 != h2 {
		t.Errorf("SchemaHash not deterministic: %s vs %s", h1, h2)
	}
}

func TestSchemaHashIgnoresConnectionOrder(t *testing.T) {
	makeReg := func() *Registry {
		reg := NewRegistry()
		reg.mu.Lock()
		reg.servers["s1"] = &ServerProxy{
			Name:  "s1",
			State: StateConnected,
			Tools: []McpToolMeta{
				{Name: "echo", Description: "echoes", InputSchema: json.RawMessage(`{"type":"object"}`)},
			},
		}
		reg.servers["s2"] = &ServerProxy{
			Name:  "s2",
			State: StateConnected,
			Tools: []McpToolMeta{
				{Name: "add", Description: "adds", InputSchema: json.RawMessage(`{"type":"object"}`)},
			},
		}
		reg.mu.Unlock()
		return reg
	}

	r1 := makeReg()
	r2 := makeReg()
	if r1.SchemaHash() != r2.SchemaHash() {
		t.Error("same tools in different registries should produce same hash")
	}
}

func TestSchemaHashChangesOnNewTool(t *testing.T) {
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

	h1 := reg.SchemaHash()

	reg.mu.Lock()
	reg.servers["s1"] = &ServerProxy{
		Name:  "s1",
		State: StateConnected,
		Tools: []McpToolMeta{
			{Name: "echo", Description: "echoes", InputSchema: json.RawMessage(`{}`)},
			{Name: "new", Description: "new tool", InputSchema: json.RawMessage(`{}`)},
		},
	}
	reg.mu.Unlock()

	h2 := reg.SchemaHash()
	if h1 == h2 {
		t.Error("adding a tool should change SchemaHash")
	}
}

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
