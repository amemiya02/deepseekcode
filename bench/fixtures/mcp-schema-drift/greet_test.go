package main

import "testing"

func TestGreet(t *testing.T) {
	if Greet("world") != "Hello, world!" {
		t.Fatal("unexpected greeting")
	}
}
