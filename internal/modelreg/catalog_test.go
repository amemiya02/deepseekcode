package modelreg

import (
	"testing"

	"github.com/amemiya02/deepseekcode/internal/config"
)

func cfgWith() config.Config {
	return config.Config{
		Active:   config.ActiveConfig{Provider: "mimo"},
		Defaults: config.DefaultsConfig{Model: "deepseek-v4-flash"},
		Providers: map[string]config.ProviderConfigTOML{
			"deepseek": {DefaultModel: "deepseek-v4-flash"},
			"mimo": {
				Type: "openai-compat", BaseURL: "https://api.mimo/v1",
				DefaultModel: "mimo-pro", Models: []string{"mimo-pro", "mimo-flash"},
			},
		},
	}
}

func TestStaticCatalogPrecedence(t *testing.T) {
	c := cfgWith()
	mimo := staticCatalog("mimo", c.Providers["mimo"])
	if len(mimo) != 2 || mimo[0].ID != "mimo-pro" || mimo[0].Source != SourceDeclared {
		t.Fatalf("mimo catalog = %+v", mimo)
	}
	ds := staticCatalog("deepseek", c.Providers["deepseek"])
	ids := idsOf(ds)
	if !contains(ids, "deepseek-v4-flash") || !contains(ids, "deepseek-v4-pro") {
		t.Fatalf("deepseek builtin missing: %v", ids)
	}
	if contains(ids, "deepseek-chat") || contains(ids, "deepseek-reasoner") {
		t.Fatalf("legacy alias not filtered: %v", ids)
	}
}

func TestResolveInitialSelectionPerProviderAuthoritative(t *testing.T) {
	c := cfgWith()
	sel, notice := resolveInitial(c)
	if sel.Provider != "mimo" || sel.Model != "mimo-pro" {
		t.Fatalf("sel = %+v, want mimo/mimo-pro", sel)
	}
	if notice == "" {
		t.Fatalf("expected a notice that [defaults].model was ignored")
	}
}

func TestResolveHonorsDefaultsModelWhenValidForProvider(t *testing.T) {
	c := cfgWith()
	c.Active.Provider = "deepseek"
	c.Defaults.Model = "deepseek-v4-pro"
	sel, notice := resolveInitial(c)
	if sel.Model != "deepseek-v4-pro" {
		t.Fatalf("sel.Model = %q, want deepseek-v4-pro", sel.Model)
	}
	if notice != "" {
		t.Fatalf("unexpected notice: %q", notice)
	}
}

func idsOf(ms []ModelInfo) []string {
	out := make([]string, len(ms))
	for i, m := range ms {
		out[i] = m.ID
	}
	return out
}

func contains(ss []string, s string) bool {
	for _, x := range ss {
		if x == s {
			return true
		}
	}
	return false
}
