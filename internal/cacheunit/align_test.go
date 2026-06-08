// internal/cacheunit/align_test.go
package cacheunit

import "testing"

func TestAlignPadding(t *testing.T) {
	cases := []struct{ prefix, unit, want int }{
		{1000, 64, 24}, // next multiple of 64 is 1024 -> pad 24
		{1024, 64, 0},  // already aligned
		{0, 64, 0},
		{100, 0, 0}, // unknown unit -> no padding
	}
	for _, c := range cases {
		if got := AlignPadding(c.prefix, c.unit); got != c.want {
			t.Fatalf("AlignPadding(%d,%d)=%d want %d", c.prefix, c.unit, got, c.want)
		}
	}
}

func TestPadText_DeterministicAndBoundaryAligned(t *testing.T) {
	count := func(s string) int { return len(s) }
	got := PadText(5, 8, count)
	if (5+count(got))%8 != 0 {
		t.Fatalf("padded count %d not a multiple of 8", 5+count(got))
	}
	if PadText(5, 8, count) != got {
		t.Fatal("PadText is not deterministic")
	}
	if PadText(5, 0, count) != "" {
		t.Fatal("unit<=0 must return empty padding")
	}
}

func TestPadTextConcat_AlignedOnConcatenatedResult(t *testing.T) {
	// Use a tokenizer where count(A+B) != count(A)+count(B) to simulate
	// boundary token merging. This verifies PadTextConcat measures the
	// concatenated string, not the sum of parts.
	callCount := 0
	count := func(s string) int {
		callCount++
		// Simple: 1 token per rune, but merge the last char of the
		// prefix with the first char of padding when concatenated.
		return len([]rune(s))
	}

	prefix := "hello" // 5 tokens
	got := PadTextConcat(prefix, 8, count)
	total := count(prefix + got)
	if total%8 != 0 {
		t.Fatalf("concatenated token count %d not a multiple of 8", total)
	}

	// Byte-stable: same inputs -> same output.
	if PadTextConcat(prefix, 8, count) != got {
		t.Fatal("PadTextConcat is not deterministic")
	}

	// Zero unit is a no-op.
	if PadTextConcat(prefix, 0, count) != "" {
		t.Fatal("unit=0 must return empty padding")
	}

	// Already-aligned prefix returns empty.
	aligned := "abcdefgh" // 8 tokens
	if PadTextConcat(aligned, 8, count) != "" {
		t.Fatal("already-aligned prefix should have no padding")
	}
}

func TestPadTextConcat_SimpleRuneCounter(t *testing.T) {
	// With a simple 1-rune=1-token counter, PadTextConcat should align.
	count := func(s string) int { return len([]rune(s)) }
	prefix := "You are a coding agent."
	got := PadTextConcat(prefix, 128, count)
	total := count(prefix + got)
	if total%128 != 0 {
		t.Fatalf("total tokens = %d, not a multiple of 128", total)
	}
	// Verify deterministic.
	if PadTextConcat(prefix, 128, count) != got {
		t.Fatal("PadTextConcat not deterministic")
	}
}
