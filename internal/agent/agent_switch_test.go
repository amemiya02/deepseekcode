package agent

import (
	"testing"

	"github.com/amemiya02/deepseekcode/internal/llm"
)

func TestSwitchProviderUpdatesClientModelAndCaps(t *testing.T) {
	a := &Agent{Model: "deepseek-v4-flash", Thinking: true, MaxContextTokens: 1_000_000,
		ReasoningEffort: llm.ReasoningEffortMax}
	newClient := &llm.Client{}
	caps := llm.Capabilities{Thinking: false, MaxContextTokens: 128_000}
	a.SwitchProvider(newClient, "mimo-pro", caps)

	if a.Client != newClient || a.Model != "mimo-pro" {
		t.Fatalf("client/model not swapped: %v %q", a.Client, a.Model)
	}
	if a.Thinking {
		t.Errorf("thinking should be off for a non-thinking provider")
	}
	if a.MaxContextTokens != 128_000 {
		t.Errorf("MaxContextTokens = %d, want 128000", a.MaxContextTokens)
	}
}
