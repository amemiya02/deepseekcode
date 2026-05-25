package commands

import (
	"reflect"
	"testing"
)

func TestTokenize_Empty(t *testing.T) {
	if got := Tokenize(""); got != nil {
		t.Errorf("empty → %v, want nil", got)
	}
	if got := Tokenize("   "); got != nil {
		t.Errorf("spaces → %v, want nil", got)
	}
}

func TestTokenize_SimpleWords(t *testing.T) {
	got := Tokenize("a b c")
	want := []string{"a", "b", "c"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestTokenize_DoubleQuoted(t *testing.T) {
	got := Tokenize(`fix the "login bug"`)
	want := []string{"fix", "the", "login bug"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestTokenize_SingleQuoted(t *testing.T) {
	got := Tokenize("a 'b c' d")
	want := []string{"a", "b c", "d"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestTokenize_MixedQuotes(t *testing.T) {
	got := Tokenize(`"hello world" 'foo bar' baz`)
	want := []string{"hello world", "foo bar", "baz"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestTokenize_UnclosedQuote(t *testing.T) {
	// Unclosed quote: leading `"` not matched by any alternative → dropped.
	// `"a b` → [`a`, `b`] (known approximate behavior).
	got := Tokenize(`"a b`)
	want := []string{"a", "b"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}
