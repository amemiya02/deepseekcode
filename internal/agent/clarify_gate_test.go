// internal/agent/clarify_gate_test.go
package agent

import "testing"

func TestShouldClarifyOnlyWhenEnabled(t *testing.T) {
	a := &Agent{} // AutoClarify false
	if a.shouldClarify("fix it") {
		t.Fatal("clarify must be off by default")
	}
	a.AutoClarify = true
	if !a.shouldClarify("fix it") {
		t.Fatal("vague prompt with AutoClarify should clarify")
	}
	if a.shouldClarify("read internal/llm/client.go") {
		t.Fatal("concrete prompt should not clarify")
	}
}
