package tokenizer

import "testing"

func TestLoad_Succeeds(t *testing.T) {
	l, err := load()
	if err != nil {
		t.Fatalf("load() error: %v", err)
	}
	if !Available() {
		t.Fatal("Available() returned false")
	}
	if len(l.vocab) <= 100000 {
		t.Fatalf("vocab too small: %d entries", len(l.vocab))
	}
	if len(l.mergeRank) == 0 {
		t.Fatal("mergeRank is empty")
	}
}

func TestLoad_Idempotent(t *testing.T) {
	l1, err1 := load()
	if err1 != nil {
		t.Fatalf("first load() error: %v", err1)
	}
	l2, err2 := load()
	if err2 != nil {
		t.Fatalf("second load() error: %v", err2)
	}
	if l1 != l2 {
		t.Fatal("load() returned different pointers on second call")
	}
}

func TestLoad_VocabLookup(t *testing.T) {
	l, err := load()
	if err != nil {
		t.Fatalf("load() error: %v", err)
	}
	// Single-byte token "!" should be in vocab (byte 0x21)
	id, ok := l.vocab["!"]
	if !ok {
		t.Fatal("single-byte token '!' not found in vocab")
	}
	if id < 0 {
		t.Fatalf("unexpected negative id for '!': %d", id)
	}
}
