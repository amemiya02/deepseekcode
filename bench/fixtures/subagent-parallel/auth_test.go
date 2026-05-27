package main

import "testing"

func TestAuthenticate(t *testing.T) {
	if err := Authenticate("user", "pass"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := Authenticate("", "pass"); err == nil {
		t.Fatal("expected error for empty user")
	}
}
