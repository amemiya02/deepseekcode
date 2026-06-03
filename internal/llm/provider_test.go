package llm

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestCapabilitiesZero(t *testing.T) {
	var caps Capabilities
	if caps.Thinking || caps.PrefixCache || caps.JSONMode || caps.MaxContextTokens != 0 || caps.SupportsModels != nil {
		t.Fatalf("zero Capabilities = %#v, want all zero values", caps)
	}
	data, err := json.Marshal(caps)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var round Capabilities
	if err := json.Unmarshal(data, &round); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if !reflect.DeepEqual(round, caps) {
		t.Fatalf("round trip = %#v, want %#v", round, caps)
	}
}

func TestNewProviderUnknown(t *testing.T) {
	_, err := NewProvider("bogus", ProviderConfig{})
	if err == nil || !strings.Contains(err.Error(), "unknown provider") {
		t.Fatalf("NewProvider unknown err = %v, want unknown provider", err)
	}
}

func TestCapabilitiesField(t *testing.T) {
	caps := Capabilities{Thinking: true}
	if !caps.Thinking {
		t.Fatal("Capabilities{Thinking:true}.Thinking = false")
	}
}

func TestNewProviderSwitchArms(t *testing.T) {
	cases := []struct {
		name    string
		wantErr bool
	}{
		{"deepseek", false},
		{"", false},          // default alias for deepseek
		{"openai-compat", false},
		{"anthropic", false}, // NEW
		{"openai", false},    // NEW
		{"unknown-xyz", true},
	}
	for _, tc := range cases {
		p, err := NewProvider(tc.name, ProviderConfig{APIKey: "k"})
		if tc.wantErr {
			if err == nil {
				t.Errorf("NewProvider(%q): expected error, got nil", tc.name)
			}
			continue
		}
		if err != nil {
			t.Errorf("NewProvider(%q): unexpected error: %v", tc.name, err)
			continue
		}
		if p == nil {
			t.Errorf("NewProvider(%q): got nil provider", tc.name)
		}
	}
}
