package llm

import (
	"context"
	"strings"
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
	if p == nil {
		t.Fatal("expected non-nil provider")
	}

	// Verify that cfg.DefaultModel was threaded through into valModel: when
	// DefaultModel is set, ValidatePro must NOT return "validate model not
	// configured".  Any other error (auth, network) is acceptable in tests.
	_, _, valErr := p.ValidatePro(context.Background(), "probe")
	if valErr != nil && strings.Contains(valErr.Error(), "validate model not configured") {
		t.Errorf("DefaultModel not threaded through: got %q", valErr.Error())
	}
}
