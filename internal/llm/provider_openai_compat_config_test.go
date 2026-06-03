package llm

import (
	"testing"
)

func TestOpenAICompatBaseURLThreadThrough(t *testing.T) {
	want := "https://custom.example.com/v1"
	p, err := NewProvider("openai-compat", ProviderConfig{
		APIKey:  "test",
		BaseURL: want,
	})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}

	bc := p.BaseClient()
	if bc == nil {
		t.Fatal("BaseClient() returned nil")
	}
	if bc.BaseURL != want {
		t.Errorf("BaseClient().BaseURL = %q, want %q", bc.BaseURL, want)
	}
}

func TestOpenAICompatDefaultModel(t *testing.T) {
	wantModel := "mistral-large-latest"
	p, err := NewProvider("openai-compat", ProviderConfig{
		APIKey:       "test",
		BaseURL:      "https://api.mistral.ai/v1",
		DefaultModel: wantModel,
	})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	// Provider must not panic on construction; model is validated at Stream time.
	if p == nil {
		t.Error("expected non-nil provider")
	}
}
