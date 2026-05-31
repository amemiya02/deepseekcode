package tokenizer

import (
	"testing"
)

func TestEncode_KnownVectors(t *testing.T) {
	if !Available() {
		t.Skip("tokenizer not available")
	}
	l, _ := load()
	tests := []struct {
		name string
		text string
	}{
		{"ascii_word", "hello"},
		{"sentence", "hello world"},
		{"cjk", "中文"},
		{"emoji", "😀"},
		{"code_snippet", "func main() {\n\tfmt.Println(\"hi\")\n}"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ids := l.Encode(tt.text)
			if len(ids) == 0 {
				t.Fatalf("Encode(%q) returned empty ids", tt.text)
			}
		})
	}
}

func TestEncode_Empty(t *testing.T) {
	if !Available() {
		t.Skip("tokenizer not available")
	}
	if Count("") != 0 {
		t.Fatal("Count(\"\") != 0")
	}
}

func TestCountBounded_ExactBelowCap(t *testing.T) {
	if !Available() {
		t.Skip("tokenizer not available")
	}
	s := "hello world"
	exact := Count(s)
	bounded := CountBounded(s, 4096)
	if bounded != exact {
		t.Fatalf("CountBounded(%q, 4096) = %d, want %d", s, bounded, exact)
	}
}

func TestCountBounded_SamplesAboveCap(t *testing.T) {
	if !Available() {
		t.Skip("tokenizer not available")
	}
	// Build a ~10k char string.
	s := ""
	for len(s) < 10000 {
		s += "The quick brown fox jumps over the lazy dog. "
	}
	exact := Count(s)
	bounded := CountBounded(s, 2048)
	// Must be within 20% of exact.
	diff := exact - bounded
	if diff < 0 {
		diff = -diff
	}
	if float64(diff)/float64(exact) > 0.20 {
		t.Fatalf("CountBounded(%d chars, 2048) = %d, exact = %d, diff=%.1f%%",
			len(s), bounded, exact, 100*float64(diff)/float64(exact))
	}
}
