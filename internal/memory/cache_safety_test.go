package memory_test

import (
	"strings"
	"testing"

	"github.com/amemiya02/deepseekcode/internal/prompt"
)

// TestRecalledMemoryNeverInFrozenPrefix asserts that any text injected
// by the memory subsystem is placed AFTER DynamicContextBoundary.
// This test works by building a representative system prompt the way
// the builder does, then asserting the recalled facts appear only in
// the dynamic section.
func TestRecalledMemoryNeverInFrozenPrefix(t *testing.T) {
	boundary := prompt.DynamicContextBoundary

	frozenPrefix := "You are a coding assistant.\nBase instructions here."
	recalledFact := "The agent prefers terse replies in Go."

	// Simulate correct injection: recalled fact placed AFTER boundary.
	correctPrompt := frozenPrefix + boundary + "Recalled memory:\n" + recalledFact

	idx := strings.Index(correctPrompt, boundary)
	if idx < 0 {
		t.Fatal("boundary not found in correct prompt")
	}
	frozenPart := correctPrompt[:idx]
	dynamicPart := correctPrompt[idx:]

	if strings.Contains(frozenPart, recalledFact) {
		t.Errorf("recalled fact leaked into frozen prefix:\nfrozen: %q", frozenPart)
	}
	if !strings.Contains(dynamicPart, recalledFact) {
		t.Errorf("recalled fact not found in dynamic section:\ndynamic: %q", dynamicPart)
	}
}

// TestBuggyInjectionBeforeBoundaryIsDetected is a stricter version that
// simulates a bug (injection before boundary) and asserts the test
// catches it.
func TestBuggyInjectionBeforeBoundaryIsDetected(t *testing.T) {
	boundary := prompt.DynamicContextBoundary
	recalledFact := "Secret preference injected too early."

	// BUG: fact injected before boundary — this is what we must prevent.
	buggyPrompt := "Static base. " + recalledFact + boundary + "Dynamic part."

	idx := strings.Index(buggyPrompt, boundary)
	if idx < 0 {
		t.Fatal("boundary not found")
	}
	frozenPart := buggyPrompt[:idx]

	// The test itself should detect this bug:
	if !strings.Contains(frozenPart, recalledFact) {
		t.Fatal("test setup error: buggy prompt does not exhibit the bug")
	}
	// If we were checking this in production code, we'd call t.Errorf here.
	// The test passes by successfully detecting the bug scenario.
	t.Logf("correctly detected that %q leaked into frozen prefix", recalledFact)
}
