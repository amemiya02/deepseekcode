package llm

import "testing"

func TestComputeFingerprintOrderIndependence(t *testing.T) {
	tools := []Tool{
		{Type: "function", Function: ToolFunction{Name: "b"}},
		{Type: "function", Function: ToolFunction{Name: "a"}},
	}
	toolsReordered := []Tool{
		{Type: "function", Function: ToolFunction{Name: "a"}},
		{Type: "function", Function: ToolFunction{Name: "b"}},
	}
	fp1 := ComputeFingerprint("SYS", tools)
	fp2 := ComputeFingerprint("SYS", toolsReordered)
	if fp1 != fp2 {
		t.Errorf("same tools different order should yield same fingerprint\nfp1: %+v\nfp2: %+v", fp1, fp2)
	}
}

func TestComputeFingerprintDifferentSystem(t *testing.T) {
	tools := []Tool{{Type: "function", Function: ToolFunction{Name: "x"}}}
	fp1 := ComputeFingerprint("SYS-A", tools)
	fp2 := ComputeFingerprint("SYS-B", tools)
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
	fp1 := ComputeFingerprint("SYS", nil)
	fp2 := ComputeFingerprint("SYS", []Tool{})
	// Both should produce the same stable hash for empty tool set.
	if fp1.ToolsSHA256 != fp2.ToolsSHA256 {
		t.Error("nil and empty tools should yield same ToolsSHA256")
	}
	if fp1.CombinedSHA256 != fp2.CombinedSHA256 {
		t.Error("nil and empty tools should yield same CombinedSHA256")
	}
	// Deterministic: same input always same output.
	fp3 := ComputeFingerprint("SYS", nil)
	if fp1 != fp3 {
		t.Error("fingerprint should be deterministic")
	}
}

func TestComputeFingerprintEmptySystem(t *testing.T) {
	fp := ComputeFingerprint("", nil)
	// Should not panic; sha256 of empty string is a known constant.
	if fp.SystemSHA256 == "" {
		t.Error("SystemSHA256 should not be empty even for empty input")
	}
	if fp.CombinedSHA256 == "" {
		t.Error("CombinedSHA256 should not be empty")
	}
}

func TestComputeFingerprintDifferentTools(t *testing.T) {
	fp1 := ComputeFingerprint("SYS", []Tool{{Type: "function", Function: ToolFunction{Name: "a"}}})
	fp2 := ComputeFingerprint("SYS", []Tool{{Type: "function", Function: ToolFunction{Name: "b"}}})
	if fp1.ToolsSHA256 == fp2.ToolsSHA256 {
		t.Error("different tool names should yield different ToolsSHA256")
	}
}

func TestPrefixMonitorFirstPin(t *testing.T) {
	m := NewPrefixMonitor()
	tools := []Tool{{Type: "function", Function: ToolFunction{Name: "a"}}}
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
	tools := []Tool{{Type: "function", Function: ToolFunction{Name: "a"}}}
	m.Check("SYS", tools)
	changed, _ := m.Check("SYS", tools)
	if changed {
		t.Error("same input should not report change")
	}
}

func TestPrefixMonitorSystemDrift(t *testing.T) {
	m := NewPrefixMonitor()
	tools := []Tool{{Type: "function", Function: ToolFunction{Name: "a"}}}
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
	m.Check("SYS", []Tool{{Type: "function", Function: ToolFunction{Name: "a"}}})
	changed, which := m.Check("SYS", []Tool{{Type: "function", Function: ToolFunction{Name: "b"}}})
	if !changed {
		t.Error("tools change should report change")
	}
	if which != "tools" {
		t.Errorf("which = %q, want %q", which, "tools")
	}
}

func TestPrefixMonitorBothDrift(t *testing.T) {
	m := NewPrefixMonitor()
	m.Check("SYS", []Tool{{Type: "function", Function: ToolFunction{Name: "a"}}})
	changed, which := m.Check("SYS2", []Tool{{Type: "function", Function: ToolFunction{Name: "b"}}})
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
	tools := []Tool{{Type: "function", Function: ToolFunction{Name: "a"}}}
	m.Check("SYS", tools)  // pin
	m.Check("SYS", tools)  // stable
	m.Check("SYS2", tools) // drift
	r := m.StabilityRatio()
	// 2 stable out of 3 checks
	if r != float64(2)/float64(3) {
		t.Errorf("StabilityRatio = %v, want %v", r, float64(2)/float64(3))
	}
}
