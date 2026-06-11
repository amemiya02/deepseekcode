package config

import "testing"

func TestSetProviderModelPreservesOtherTables(t *testing.T) {
	store := map[string]any{
		"active": map[string]any{"provider": "deepseek"},
		"providers": map[string]any{
			"deepseek": map[string]any{"default_model": "deepseek-v4-flash"},
			"mimo": map[string]any{
				"type": "openai-compat", "base_url": "https://api.mimo/v1",
				"env_var": "MIMO_API_KEY", "default_model": "mimo-pro",
			},
		},
	}
	restore := SetRawIO(ConfigRawIO{
		Read:  func() (map[string]any, error) { return store, nil },
		Write: func(m map[string]any) error { store = m; return nil },
	})
	defer restore()

	if err := SetProviderModel("mimo", "mimo-flash"); err != nil {
		t.Fatalf("SetProviderModel: %v", err)
	}
	if err := SetActiveProvider("mimo"); err != nil {
		t.Fatalf("SetActiveProvider: %v", err)
	}

	providers := store["providers"].(map[string]any)
	mimo := providers["mimo"].(map[string]any)
	if mimo["default_model"] != "mimo-flash" {
		t.Errorf("mimo.default_model = %v, want mimo-flash", mimo["default_model"])
	}
	if mimo["env_var"] != "MIMO_API_KEY" || mimo["base_url"] != "https://api.mimo/v1" {
		t.Errorf("mimo lost fields: %v", mimo)
	}
	ds := providers["deepseek"].(map[string]any)
	if ds["default_model"] != "deepseek-v4-flash" {
		t.Errorf("deepseek mutated: %v", ds)
	}
	if store["active"].(map[string]any)["provider"] != "mimo" {
		t.Errorf("active.provider not set to mimo")
	}
}
