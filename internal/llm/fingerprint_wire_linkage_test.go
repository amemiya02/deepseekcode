package llm

import (
	"encoding/json"
	"testing"
)

// TestFingerprintTracksWireStaticHead locks the P1 contract from
// docs/refactor-prefix-fingerprint.md: the Prefix Fingerprint must move IFF the
// MarshalCacheStable static head moves. Today hashToolsCanonical only promises
// to match MarshalCacheStable via a hand-maintained comment; this test pins the
// linkage *behaviorally* on identical inputs, so the M1 consolidation (one
// shared canonicalization) is provably byte-preserving — any future change that
// moves the wire bytes without moving the fingerprint (or vice versa) fails here.
//
// Note: this asserts they move *together*, not that the fingerprint bytes equal
// the wire bytes (the fingerprint hashes the raw system string; the wire wraps
// it in a message). M1 makes the linkage structural; M0 pins it behaviorally.
func TestFingerprintTracksWireStaticHead(t *testing.T) {
	const sys = "You are a coding assistant."
	mkReq := func(tools []Tool) Request {
		return Request{
			Model:         "deepseek-v4-flash",
			Stream:        true,
			StreamOptions: &StreamOptions{IncludeUsage: true},
			Tools:         tools,
			Messages: []Message{
				{Role: "system", Blocks: []ContentBlock{TextBlock{Text: sys}}},
				{Role: "user", Blocks: []ContentBlock{TextBlock{Text: "hi"}}},
			},
		}
	}
	wire := func(t *testing.T, tools []Tool) string {
		t.Helper()
		b, err := mkReq(tools).MarshalCacheStable()
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	fp := func(tools []Tool) PrefixFingerprint {
		return ComputeFingerprint(PrefixInput{SystemPrompt: sys, Tools: tools})
	}

	toolsA := []Tool{
		{Type: "function", Function: ToolFunction{Name: "read_file", Description: "Read",
			Parameters: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"},"limit":{"type":"integer"}}}`)}},
		{Type: "function", Function: ToolFunction{Name: "bash", Description: "Run",
			Parameters: json.RawMessage(`{"type":"object","properties":{"command":{"type":"string"}}}`)}},
	}
	// Same prefix, cache-irrelevant scrambles: tool order swapped AND nested
	// schema keys permuted. Neither the wire head nor the fingerprint may move.
	toolsB := []Tool{
		{Type: "function", Function: ToolFunction{Name: "bash", Description: "Run",
			Parameters: json.RawMessage(`{"properties":{"command":{"type":"string"}},"type":"object"}`)}},
		{Type: "function", Function: ToolFunction{Name: "read_file", Description: "Read",
			Parameters: json.RawMessage(`{"properties":{"limit":{"type":"integer"},"path":{"type":"string"}},"type":"object"}`)}},
	}

	if wire(t, toolsA) != wire(t, toolsB) {
		t.Error("wire bytes moved on a cache-irrelevant reorder/permute")
	}
	if fp(toolsA) != fp(toolsB) {
		t.Error("fingerprint moved on a cache-irrelevant reorder/permute")
	}

	// A real change (tool description) must move BOTH, together.
	toolsC := []Tool{
		{Type: "function", Function: ToolFunction{Name: "read_file", Description: "Read CHANGED",
			Parameters: toolsA[0].Function.Parameters}},
		toolsA[1],
	}
	if wire(t, toolsC) == wire(t, toolsA) {
		t.Error("wire bytes did not move on a real tool-description change")
	}
	if fp(toolsC) == fp(toolsA) {
		t.Error("fingerprint did not move on a real tool-description change")
	}
}

// TestPrefixDiff covers the typed verdict directly: cache-irrelevant scrambles
// are not a diff, a schema edit is a schema-change, a name swap is add/remove,
// and a system edit is system-only.
func TestPrefixDiff(t *testing.T) {
	a := StaticPrefix{System: "S", Tools: []Tool{
		{Type: "function", Function: ToolFunction{Name: "read", Description: "d",
			Parameters: json.RawMessage(`{"type":"object","properties":{"a":{"type":"string"},"b":{"type":"number"}}}`)}},
		{Type: "function", Function: ToolFunction{Name: "bash", Description: "d",
			Parameters: json.RawMessage(`{"type":"object"}`)}},
	}}

	// Reorder tools + permute nested schema keys → no diff.
	b := StaticPrefix{System: "S", Tools: []Tool{
		{Type: "function", Function: ToolFunction{Name: "bash", Description: "d",
			Parameters: json.RawMessage(`{"type":"object"}`)}},
		{Type: "function", Function: ToolFunction{Name: "read", Description: "d",
			Parameters: json.RawMessage(`{"properties":{"b":{"type":"number"},"a":{"type":"string"}},"type":"object"}`)}},
	}}
	if d := Diff(a, b); d.Changed() {
		t.Errorf("reorder/key-permute must not diff, got %+v", d)
	}

	// Schema edit on "read" → schema-changed, system unchanged.
	c := StaticPrefix{System: "S", Tools: append([]Tool(nil), a.Tools...)}
	c.Tools[0].Function.Parameters = json.RawMessage(`{"type":"object","properties":{"a":{"type":"integer"}}}`)
	if d := Diff(a, c); d.SystemChanged || len(d.Tools.SchemaChanged) != 1 || d.Tools.SchemaChanged[0] != "read" {
		t.Errorf("expected only read schema-changed, got %+v", d)
	}

	// System-only edit.
	if d := Diff(a, StaticPrefix{System: "S2", Tools: a.Tools}); !d.SystemChanged || d.Tools.Changed() {
		t.Errorf("expected system-only change, got %+v", d)
	}
}
