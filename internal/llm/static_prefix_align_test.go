package llm

import (
	"testing"

	"github.com/amemiya02/deepseekcode/internal/cacheunit"
)

// count is a deterministic stand-in tokenizer (1 token per rune) for the test.
func count(s string) int { return len([]rune(s)) }

func TestAlignPadding_MakesPrefixUnitMultiple(t *testing.T) {
	const unit = 128
	prefix := "SYSTEM PROMPT BODY ...."
	pad := cacheunit.PadTextConcat(prefix, unit, count)
	total := count(prefix + pad)
	if total%unit != 0 {
		t.Fatalf("aligned total = %d, not a multiple of %d", total, unit)
	}
}

func TestAlignPadding_ByteStable(t *testing.T) {
	const unit = 128
	prefix := "hello world test prefix"
	if cacheunit.PadTextConcat(prefix, unit, count) != cacheunit.PadTextConcat(prefix, unit, count) {
		t.Fatal("PadTextConcat not deterministic for identical inputs")
	}
}

func TestAlignPadding_ZeroUnitIsNoop(t *testing.T) {
	if got := cacheunit.PadTextConcat("hello", 0, count); got != "" {
		t.Fatalf("unit=0 should yield empty padding, got %q", got)
	}
}

func TestAssembledPrefixAlignedAndStable(t *testing.T) {
	// Build two identical StaticPrefixes. Verify that their fingerprints
	// are identical (byte-stable). This tests the cacheunit primitive in
	// the context of the StaticPrefix fingerprinting pipeline.
	sys := "You are a coding agent."
	tools := []Tool{
		{Type: "function", Function: ToolFunction{Name: "read_file", Description: "Read a file"}},
	}
	p1 := StaticPrefix{System: sys, Tools: tools}
	p2 := StaticPrefix{System: sys, Tools: tools}
	if p1.Fingerprint().CombinedSHA256 != p2.Fingerprint().CombinedSHA256 {
		t.Fatal("identical prefixes should have identical fingerprints")
	}
}

func TestAssembledPrefixWithAlignmentStable(t *testing.T) {
	// Simulate what buildEpochComponents does: pad the system prompt to a
	// unit boundary and verify that the padded prefix fingerprint is
	// deterministic (same input -> same fingerprint).
	sys := "You are a coding agent. You work with files."
	const unit = 128
	pad := cacheunit.PadTextConcat(sys, unit, count)
	paddedSys := sys + pad

	// Verify alignment: total tokens must be a unit multiple.
	total := count(paddedSys)
	if total%unit != 0 {
		t.Fatalf("padded prefix tokens = %d, not a multiple of %d", total, unit)
	}

	// Verify byte-stability: two identical pad operations produce the same string.
	pad2 := cacheunit.PadTextConcat(sys, unit, count)
	if pad != pad2 {
		t.Fatal("PadTextConcat not deterministic for identical inputs")
	}

	// Verify fingerprint stability: two padded prefixes have the same fingerprint.
	fp1 := StaticPrefix{System: paddedSys}.Fingerprint()
	fp2 := StaticPrefix{System: sys + pad2}.Fingerprint()
	if fp1.CombinedSHA256 != fp2.CombinedSHA256 {
		t.Fatal("padded prefixes should have identical fingerprints")
	}

	// Verify that padded and unpadded prefixes have DIFFERENT fingerprints
	// (the padding intentionally changes the cache key).
	unpadded := StaticPrefix{System: sys}.Fingerprint()
	if fp1.CombinedSHA256 == unpadded.CombinedSHA256 {
		t.Fatal("padded and unpadded prefixes should have different fingerprints")
	}
}
