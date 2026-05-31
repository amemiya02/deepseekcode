package tokenizer

import (
	"testing"
	"unicode/utf8"
)

func TestBuildByteToChar(t *testing.T) {
	b2c := buildByteToChar()
	// All 256 entries must be distinct.
	seen := make(map[rune]bool, 256)
	for i, r := range b2c {
		if seen[r] {
			t.Fatalf("duplicate rune %U at byte %d", r, i)
		}
		seen[r] = true
	}
	// Printable bytes map to themselves.
	if b2c[33] != '!' {
		t.Fatalf("b2c[33] = %U, want '!'", b2c[33])
	}
	if b2c[65] != 'A' {
		t.Fatalf("b2c[65] = %U, want 'A'", b2c[65])
	}
	// Byte 0 maps to a rune >= 256.
	if b2c[0] < 256 {
		t.Fatalf("b2c[0] = %d, want >= 256", b2c[0])
	}
}

func TestByteLevelEncode_ASCII(t *testing.T) {
	b2c := buildByteToChar()
	enc := byteLevelEncode("ab", b2c)
	if len(enc) != 2 {
		t.Fatalf("len(enc) = %d, want 2", len(enc))
	}
	// 'a' and 'b' are printable, should map to themselves.
	if enc != "ab" {
		t.Fatalf("enc = %q, want %q", enc, "ab")
	}
}

func TestByteLevelEncode_UTF8(t *testing.T) {
	b2c := buildByteToChar()
	// CJK character 中 is 3 UTF-8 bytes: 0xE4 0xB8 0xAD
	enc := byteLevelEncode("中", b2c)
	// Each UTF-8 byte maps to exactly one rune in the byte-level encoding.
	if utf8.RuneCountInString(enc) != 3 {
		t.Fatalf("rune count = %d, want 3 (one per UTF-8 byte)", utf8.RuneCountInString(enc))
	}
}
