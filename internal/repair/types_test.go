package repair

import (
	"testing"
)

func TestHashArgs_Empty(t *testing.T) {
	h1 := HashArgs("")
	h2 := HashArgs("")
	if h1 != h2 {
		t.Errorf("empty string hash not consistent: %s vs %s", h1, h2)
	}
	// SHA-256 produces 64 hex characters
	if len(h1) != 64 {
		t.Errorf("expected 64-char hex hash, got %d", len(h1))
	}
}

func TestHashArgs_Consistent(t *testing.T) {
	input := `{"path":"README.md"}`
	h1 := HashArgs(input)
	h2 := HashArgs(input)
	if h1 != h2 {
		t.Errorf("same input produced different hashes: %s vs %s", h1, h2)
	}
}

func TestHashArgs_Different(t *testing.T) {
	h1 := HashArgs(`{"a":1}`)
	h2 := HashArgs(`{"a":2}`)
	if h1 == h2 {
		t.Errorf("different inputs produced same hash")
	}
}

func TestCanonicalArgs_Valid(t *testing.T) {
	input := `{"b":2,"a":1}`
	out, ok := CanonicalArgs(input)
	if !ok {
		t.Fatalf("expected valid JSON, got false")
	}
	expected := `{"a":1,"b":2}`
	if out != expected {
		t.Errorf("expected %q, got %q", expected, out)
	}
}

func TestCanonicalArgs_Nested(t *testing.T) {
	input := `{"b":1,"a":{"d":4,"c":3}}`
	out, ok := CanonicalArgs(input)
	if !ok {
		t.Fatalf("expected valid JSON, got false")
	}
	expected := `{"a":{"c":3,"d":4},"b":1}`
	if out != expected {
		t.Errorf("expected %q, got %q", expected, out)
	}
}

func TestCanonicalArgs_Invalid(t *testing.T) {
	input := `{`
	out, ok := CanonicalArgs(input)
	if ok {
		t.Errorf("expected false for invalid JSON, got true")
	}
	if out != input {
		t.Errorf("expected original input returned, got %q", out)
	}
}

func TestCanonicalArgs_ArrayPreservesOrder(t *testing.T) {
	input := `[3,2,1]`
	out, ok := CanonicalArgs(input)
	if !ok {
		t.Fatalf("expected valid JSON, got false")
	}
	// Arrays preserve order
	expected := `[3,2,1]`
	if out != expected {
		t.Errorf("expected array order preserved %q, got %q", expected, out)
	}
}

func TestCanonicalArgs_ArrayWithObjects(t *testing.T) {
	input := `[{"b":2},{"a":1}]`
	out, ok := CanonicalArgs(input)
	if !ok {
		t.Fatalf("expected valid JSON, got false")
	}
	expected := `[{"b":2},{"a":1}]`
	if out != expected {
		t.Errorf("expected %q, got %q", expected, out)
	}
}

func TestCanonicalArgs_Empty(t *testing.T) {
	out, ok := CanonicalArgs("")
	if ok {
		t.Errorf("empty string is not valid JSON, expected false")
	}
	if out != "" {
		t.Errorf("expected original input returned, got %q", out)
	}
}

func TestCanonicalArgs_DeepNesting(t *testing.T) {
	input := `{"z":{"y":{"x":1}}}`
	out, ok := CanonicalArgs(input)
	if !ok {
		t.Fatalf("expected valid JSON, got false")
	}
	// Already sorted, should be unchanged
	expected := `{"z":{"y":{"x":1}}}`
	if out != expected {
		t.Errorf("expected %q, got %q", expected, out)
	}
}

func TestCanonicalArgs_MixedTypes(t *testing.T) {
	input := `{"c":"str","b":123,"a":true,"d":null}`
	out, ok := CanonicalArgs(input)
	if !ok {
		t.Fatalf("expected valid JSON, got false")
	}
	// Keys should be sorted alphabetically
	expected := `{"a":true,"b":123,"c":"str","d":null}`
	if out != expected {
		t.Errorf("expected %q, got %q", expected, out)
	}
}
