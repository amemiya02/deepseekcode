package traceinspect

import (
	"os"
	"strings"
	"testing"
)

func TestDiffBytes_CleanPrefix(t *testing.T) {
	a := []byte(`{"messages":[1,2]}`)
	b := []byte(`{"messages":[1,2]}tail`)
	d := DiffBytes(a, b)
	if !d.AIsPrefixOfB {
		t.Fatalf("expected A to be a clean prefix of B, got %+v", d)
	}
	if d.Diverged {
		t.Fatalf("clean prefix must not be diverged, got %+v", d)
	}
	if d.DivergeAt != len(a) {
		t.Fatalf("DivergeAt = %d, want %d", d.DivergeAt, len(a))
	}
}

func TestDiffBytes_HistoricalDrift(t *testing.T) {
	// Index 6 is the 'X' vs 'Y' byte: {"c":"X...
	a := []byte(`{"c":"X","tail":1}`)
	b := []byte(`{"c":"Y","tail":1}`)
	d := DiffBytes(a, b)
	if !d.Diverged {
		t.Fatalf("expected divergence, got %+v", d)
	}
	if d.AIsPrefixOfB {
		t.Fatalf("must not be a clean prefix when a byte changed, got %+v", d)
	}
	if d.DivergeAt != 6 {
		t.Fatalf("DivergeAt = %d, want 6", d.DivergeAt)
	}
}

func TestDiffBodyFiles_CleanPrefix(t *testing.T) {
	dir := t.TempDir()
	a := dir + "/turn_0001.json"
	b := dir + "/turn_0002.json"
	if err := os.WriteFile(a, []byte("HEAD"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b, []byte("HEADtail"), 0o644); err != nil {
		t.Fatal(err)
	}
	d, err := DiffBodyFiles(a, b)
	if err != nil {
		t.Fatal(err)
	}
	if !d.AIsPrefixOfB {
		t.Fatalf("expected clean prefix, got %+v", d)
	}
}

func TestRenderBodyDiff_Verdicts(t *testing.T) {
	stable := RenderBodyDiff(DiffBytes([]byte("HEAD"), []byte("HEADtail")))
	if !strings.Contains(stable, "cache-stable") {
		t.Fatalf("want cache-stable verdict, got: %s", stable)
	}
	evict := RenderBodyDiff(DiffBytes([]byte("aXc"), []byte("aYc")))
	if !strings.Contains(evict, "EVICTION CAUSE") {
		t.Fatalf("want EVICTION CAUSE verdict, got: %s", evict)
	}
}
