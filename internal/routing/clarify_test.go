// internal/routing/clarify_test.go
package routing

import "testing"

func TestVaguePromptNeedsClarification(t *testing.T) {
	for _, in := range []string{"fix it", "make it better", "improve this", "do the thing"} {
		need, qs := NeedsClarification(in)
		if !need || len(qs) == 0 {
			t.Fatalf("%q should need clarification", in)
		}
	}
}

func TestConcretePromptNeedsNoClarification(t *testing.T) {
	for _, in := range []string{
		"read internal/llm/client.go and explain Stream",
		"add a -turns flag to bench/cmd/cachedemo",
	} {
		if need, _ := NeedsClarification(in); need {
			t.Fatalf("%q should NOT need clarification", in)
		}
	}
}
