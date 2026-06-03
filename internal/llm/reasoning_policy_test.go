// internal/llm/reasoning_policy_test.go
package llm

import (
	"testing"
)

func TestReadPolicy_Unset(t *testing.T) {
	t.Setenv("DEEPSEEKCODE_REASONING_RETAIN", "")
	t.Setenv("DEEPSEEKCODE_REASONING_DROP", "")
	p, err := ReadPolicy()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Mode != PolicyPassThrough {
		t.Fatalf("expected PassThrough, got %v", p.Mode)
	}
}

func TestReadPolicy_Drop(t *testing.T) {
	t.Setenv("DEEPSEEKCODE_REASONING_DROP", "1")
	t.Setenv("DEEPSEEKCODE_REASONING_RETAIN", "")
	p, err := ReadPolicy()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Mode != PolicyDropAll {
		t.Fatalf("expected DropAll, got %v", p.Mode)
	}
}

func TestReadPolicy_Retain(t *testing.T) {
	t.Setenv("DEEPSEEKCODE_REASONING_DROP", "")
	t.Setenv("DEEPSEEKCODE_REASONING_RETAIN", "3")
	p, err := ReadPolicy()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Mode != PolicyRetainLast {
		t.Fatalf("expected RetainLast, got %v", p.Mode)
	}
	if p.N != 3 {
		t.Fatalf("expected N=3, got %d", p.N)
	}
}

func TestReadPolicy_RetainInvalid(t *testing.T) {
	t.Setenv("DEEPSEEKCODE_REASONING_DROP", "")
	t.Setenv("DEEPSEEKCODE_REASONING_RETAIN", "0")
	_, err := ReadPolicy()
	if err == nil {
		t.Fatal("expected error for N=0")
	}
}

func TestReadPolicy_BothSet(t *testing.T) {
	t.Setenv("DEEPSEEKCODE_REASONING_DROP", "1")
	t.Setenv("DEEPSEEKCODE_REASONING_RETAIN", "2")
	_, err := ReadPolicy()
	if err == nil {
		t.Fatal("expected error when both env vars set")
	}
}
