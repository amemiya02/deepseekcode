package main

import "testing"

func TestProcess(t *testing.T) {
	cfg := LoadConfig()
	svc := NewService(cfg)
	result, err := svc.Process("hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "HELLO" {
		t.Fatalf("expected HELLO, got %s", result)
	}
}

func TestProcessEmpty(t *testing.T) {
	cfg := LoadConfig()
	svc := NewService(cfg)
	_, err := svc.Process("")
	if err == nil {
		t.Fatal("expected error for empty input")
	}
}
