package llm

import (
	"encoding/json"
	"testing"
)

func TestComputeFingerprintOrderIndependence(t *testing.T) {
	tools := []Tool{
		{Type: "function", Function: ToolFunction{Name: "b", Description: "tool b", Parameters: json.RawMessage(`{"type":"object"}`)}},
		{Type: "function", Function: ToolFunction{Name: "a", Description: "tool a", Parameters: json.RawMessage(`{"type":"object"}`)}},
	}
	toolsReordered := []Tool{
		{Type: "function", Function: ToolFunction{Name: "a", Description: "tool a", Parameters: json.RawMessage(`{"type":"object"}`)}},
		{Type: "function", Function: ToolFunction{Name: "b", Description: "tool b", Parameters: json.RawMessage(`{"type":"object"}`)}},
	}
	fp1 := ComputeFingerprint(PrefixInput{SystemPrompt: "SYS", Tools: tools})
	fp2 := ComputeFingerprint(PrefixInput{SystemPrompt: "SYS", Tools: toolsReordered})
	if fp1 != fp2 {
		t.Errorf("same tools different order should yield same fingerprint\nfp1: %+v\nfp2: %+v", fp1, fp2)
	}
}

func TestComputeFingerprintDifferentSystem(t *testing.T) {
	tools := []Tool{{Type: "function", Function: ToolFunction{Name: "x", Description: "tool x"}}}
	fp1 := ComputeFingerprint(PrefixInput{SystemPrompt: "SYS-A", Tools: tools})
	fp2 := ComputeFingerprint(PrefixInput{SystemPrompt: "SYS-B", Tools: tools})
	if fp1.SystemSHA256 == fp2.SystemSHA256 {
		t.Error("different systems should yield different SystemSHA256")
	}
	if fp1.CombinedSHA256 == fp2.CombinedSHA256 {
		t.Error("different systems should yield different CombinedSHA256")
	}
	// ToolsSHA256 should be the same (same tool set)
	if fp1.ToolsSHA256 != fp2.ToolsSHA256 {
		t.Error("same tools should yield same ToolsSHA256 regardless of system")
	}
}

func TestComputeFingerprintEmptyTools(t *testing.T) {
	fp1 := ComputeFingerprint(PrefixInput{SystemPrompt: "SYS", Tools: nil})
	fp2 := ComputeFingerprint(PrefixInput{SystemPrompt: "SYS", Tools: []Tool{}})
	// Both should produce the same stable hash for empty tool set.
	if fp1.ToolsSHA256 != fp2.ToolsSHA256 {
		t.Error("nil and empty tools should yield same ToolsSHA256")
	}
	if fp1.CombinedSHA256 != fp2.CombinedSHA256 {
		t.Error("nil and empty tools should yield same CombinedSHA256")
	}
	// Deterministic: same input always same output.
	fp3 := ComputeFingerprint(PrefixInput{SystemPrompt: "SYS", Tools: nil})
	if fp1 != fp3 {
		t.Error("fingerprint should be deterministic")
	}
}

func TestComputeFingerprintEmptySystem(t *testing.T) {
	fp := ComputeFingerprint(PrefixInput{SystemPrompt: "", Tools: nil})
	// Should not panic; sha256 of empty string is a known constant.
	if fp.SystemSHA256 == "" {
		t.Error("SystemSHA256 should not be empty even for empty input")
	}
	if fp.CombinedSHA256 == "" {
		t.Error("CombinedSHA256 should not be empty")
	}
}

func TestComputeFingerprintDifferentTools(t *testing.T) {
	fp1 := ComputeFingerprint(PrefixInput{SystemPrompt: "SYS", Tools: []Tool{{Type: "function", Function: ToolFunction{Name: "a", Description: "tool a"}}}})
	fp2 := ComputeFingerprint(PrefixInput{SystemPrompt: "SYS", Tools: []Tool{{Type: "function", Function: ToolFunction{Name: "b", Description: "tool b"}}}})
	if fp1.ToolsSHA256 == fp2.ToolsSHA256 {
		t.Error("different tool names should yield different ToolsSHA256")
	}
}

func TestComputeFingerprint_DescriptionChange(t *testing.T) {
	// Changing a tool description should change the fingerprint
	fp1 := ComputeFingerprint(PrefixInput{SystemPrompt: "SYS", Tools: []Tool{
		{Type: "function", Function: ToolFunction{Name: "read_file", Description: "Read a file"}},
	}})
	fp2 := ComputeFingerprint(PrefixInput{SystemPrompt: "SYS", Tools: []Tool{
		{Type: "function", Function: ToolFunction{Name: "read_file", Description: "Read a file from disk"}},
	}})
	if fp1.ToolsSHA256 == fp2.ToolsSHA256 {
		t.Error("different tool descriptions should yield different ToolsSHA256")
	}
}

func TestComputeFingerprint_SchemaKeyOrder(t *testing.T) {
	// Different key order in schema should produce same fingerprint
	schema1 := json.RawMessage(`{"type":"object","properties":{"b":{"type":"string"},"a":{"type":"integer"}}}`)
	schema2 := json.RawMessage(`{"type":"object","properties":{"a":{"type":"integer"},"b":{"type":"string"}}}`)

	fp1 := ComputeFingerprint(PrefixInput{SystemPrompt: "SYS", Tools: []Tool{
		{Type: "function", Function: ToolFunction{Name: "tool", Description: "desc", Parameters: schema1}},
	}})
	fp2 := ComputeFingerprint(PrefixInput{SystemPrompt: "SYS", Tools: []Tool{
		{Type: "function", Function: ToolFunction{Name: "tool", Description: "desc", Parameters: schema2}},
	}})

	if fp1.ToolsSHA256 != fp2.ToolsSHA256 {
		t.Errorf("schema key order differences should not change ToolsSHA256\nfp1: %s\nfp2: %s", fp1.ToolsSHA256, fp2.ToolsSHA256)
	}
	if fp1.CombinedSHA256 != fp2.CombinedSHA256 {
		t.Error("schema key order differences should not change CombinedSHA256")
	}
}

func TestComputeFingerprint_ToolsReorderIdentical(t *testing.T) {
	// Same tools in different order should produce identical fingerprint
	tools1 := []Tool{
		{Type: "function", Function: ToolFunction{Name: "c", Description: "tool c", Parameters: json.RawMessage(`{"type":"object"}`)}},
		{Type: "function", Function: ToolFunction{Name: "a", Description: "tool a", Parameters: json.RawMessage(`{"type":"object","properties":{"x":{"type":"string"}}}`)}},
		{Type: "function", Function: ToolFunction{Name: "b", Description: "tool b", Parameters: json.RawMessage(`{"type":"object"}`)}},
	}
	tools2 := []Tool{
		{Type: "function", Function: ToolFunction{Name: "b", Description: "tool b", Parameters: json.RawMessage(`{"type":"object"}`)}},
		{Type: "function", Function: ToolFunction{Name: "c", Description: "tool c", Parameters: json.RawMessage(`{"type":"object"}`)}},
		{Type: "function", Function: ToolFunction{Name: "a", Description: "tool a", Parameters: json.RawMessage(`{"type":"object","properties":{"x":{"type":"string"}}}`)}},
	}

	fp1 := ComputeFingerprint(PrefixInput{SystemPrompt: "SYS", Tools: tools1})
	fp2 := ComputeFingerprint(PrefixInput{SystemPrompt: "SYS", Tools: tools2})

	if fp1.ToolsSHA256 != fp2.ToolsSHA256 {
		t.Error("reordering tools should produce identical ToolsSHA256")
	}
	if fp1.CombinedSHA256 != fp2.CombinedSHA256 {
		t.Error("reordering tools should produce identical CombinedSHA256")
	}
}

func TestPrefixMonitorFirstPin(t *testing.T) {
	m := NewPrefixMonitor()
	tools := []Tool{{Type: "function", Function: ToolFunction{Name: "a", Description: "tool a"}}}
	changed, which := m.Check("SYS", tools)
	if changed {
		t.Error("first Check should only pin, not report change")
	}
	if which != "" {
		t.Errorf("first Check which = %q, want empty", which)
	}
}

func TestPrefixMonitorStable(t *testing.T) {
	m := NewPrefixMonitor()
	tools := []Tool{{Type: "function", Function: ToolFunction{Name: "a", Description: "tool a"}}}
	m.Check("SYS", tools)
	changed, _ := m.Check("SYS", tools)
	if changed {
		t.Error("same input should not report change")
	}
}

func TestPrefixMonitorSystemDrift(t *testing.T) {
	m := NewPrefixMonitor()
	tools := []Tool{{Type: "function", Function: ToolFunction{Name: "a", Description: "tool a"}}}
	m.Check("SYS", tools)
	changed, which := m.Check("SYS2", tools)
	if !changed {
		t.Error("system change should report change")
	}
	if which != "sys" {
		t.Errorf("which = %q, want %q", which, "sys")
	}
}

func TestPrefixMonitorToolsDrift(t *testing.T) {
	m := NewPrefixMonitor()
	m.Check("SYS", []Tool{{Type: "function", Function: ToolFunction{Name: "a", Description: "tool a"}}})
	changed, which := m.Check("SYS", []Tool{{Type: "function", Function: ToolFunction{Name: "b", Description: "tool b"}}})
	if !changed {
		t.Error("tools change should report change")
	}
	if which != "tools" {
		t.Errorf("which = %q, want %q", which, "tools")
	}
}

func TestPrefixMonitorBothDrift(t *testing.T) {
	m := NewPrefixMonitor()
	m.Check("SYS", []Tool{{Type: "function", Function: ToolFunction{Name: "a", Description: "tool a"}}})
	changed, which := m.Check("SYS2", []Tool{{Type: "function", Function: ToolFunction{Name: "b", Description: "tool b"}}})
	if !changed {
		t.Error("both change should report change")
	}
	if which != "sys+tools" {
		t.Errorf("which = %q, want %q", which, "sys+tools")
	}
}

func TestPrefixMonitorStabilityRatio(t *testing.T) {
	m := NewPrefixMonitor()
	if m.StabilityRatio() != 1 {
		t.Error("empty monitor should have ratio 1")
	}
	tools := []Tool{{Type: "function", Function: ToolFunction{Name: "a", Description: "tool a"}}}
	m.Check("SYS", tools)  // pin
	m.Check("SYS", tools)  // stable
	m.Check("SYS2", tools) // drift
	r := m.StabilityRatio()
	// 2 stable out of 3 checks
	if r != float64(2)/float64(3) {
		t.Errorf("StabilityRatio = %v, want %v", r, float64(2)/float64(3))
	}
}
