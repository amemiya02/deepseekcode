package memory_test

import (
	"strings"
	"testing"

	"github.com/amemiya02/deepseekcode/internal/prompt"
)

// TestRecalledMemoryNeverInFrozenPrefix calls InjectRecalled and asserts
// the recalled fact lands in the dynamic section, never in the frozen prefix.
func TestRecalledMemoryNeverInFrozenPrefix(t *testing.T) {
	boundary := prompt.DynamicContextBoundary

	frozenPrefix := "You are a coding assistant.\nBase instructions here."
	recalledFact := "The agent prefers terse replies in Go."

	// Use InjectRecalled starting from a prompt that already has the boundary.
	base := frozenPrefix + boundary + "Existing dynamic content."
	result := prompt.InjectRecalled(base, recalledFact)

	idx := strings.Index(result, boundary)
	if idx < 0 {
		t.Fatal("boundary not found after InjectRecalled")
	}
	frozenPart := result[:idx]
	dynamicPart := result[idx:]

	if strings.Contains(frozenPart, recalledFact) {
		t.Errorf("recalled fact leaked into frozen prefix:\nfrozen: %q", frozenPart)
	}
	if !strings.Contains(dynamicPart, recalledFact) {
		t.Errorf("recalled fact not found in dynamic section:\ndynamic: %q", dynamicPart)
	}
}

// TestBuggyInjectionBeforeBoundaryIsDetected verifies that InjectRecalled
// always places recalled text AFTER the boundary, even when content already
// follows the boundary in the prompt.
func TestBuggyInjectionBeforeBoundaryIsDetected(t *testing.T) {
	boundary := prompt.DynamicContextBoundary
	recalledFact := "Secret preference injected too early."

	// Adversarial input: boundary is present and is followed by existing
	// dynamic content. InjectRecalled must NOT place recalledFact before
	// the boundary.
	adversarial := "Static base." + boundary + "Dynamic part."
	result := prompt.InjectRecalled(adversarial, recalledFact)

	idx := strings.Index(result, boundary)
	if idx < 0 {
		t.Fatal("boundary not found after InjectRecalled")
	}
	frozenPart := result[:idx]

	if strings.Contains(frozenPart, recalledFact) {
		t.Errorf("InjectRecalled placed recalled fact in frozen prefix:\nfrozen: %q\nresult: %q",
			frozenPart, result)
	}
}
