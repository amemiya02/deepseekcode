package main

import "testing"

func TestGreet(t *testing.T) {
	got := greet("Test")
	want := "Hello, Test!"
	if got != want {
		t.Errorf("greet(%q) = %q, want %q", "Test", got, want)
	}
}

func TestAdd(t *testing.T) {
	if add(2, 3) != 5 {
		t.Errorf("add(2,3) = %d, want 5", add(2, 3))
	}
}
