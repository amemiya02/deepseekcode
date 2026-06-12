package modelreg

import (
	"fmt"

	"github.com/amemiya02/deepseekcode/internal/config"
	"github.com/amemiya02/deepseekcode/internal/llm"
)

func providerOrDefault(name string) string {
	if name == "" {
		return "deepseek"
	}
	return name
}

func providerType(name string, p config.ProviderConfigTOML) string {
	if name == "deepseek" || name == "" {
		return "deepseek"
	}
	return p.Type
}

func staticCatalog(name string, p config.ProviderConfigTOML) []ModelInfo {
	caps, _ := llm.ProviderCapabilities(providerType(name, p))
	// A provider may declare its real context window in config; honor it over
	// the built-in capability default (e.g. an openai-compat provider that is
	// actually 1M, not the 128k assumed default). Live /models discovery, when
	// it reports a window, overrides this in hybridCatalog.
	if p.MaxContextTokens > 0 {
		caps.MaxContextTokens = p.MaxContextTokens
	}
	mk := func(id string, src Source) ModelInfo {
		return ModelInfo{Provider: name, ID: id, Label: id, Caps: caps, Source: src, Available: true}
	}
	if len(p.Models) > 0 {
		out := make([]ModelInfo, 0, len(p.Models))
		for _, id := range p.Models {
			out = append(out, mk(id, SourceDeclared))
		}
		return out
	}
	if len(caps.SupportsModels) > 0 {
		out := make([]ModelInfo, 0, len(caps.SupportsModels))
		for _, id := range caps.SupportsModels {
			if config.IsLegacyDeepSeekAlias(id) {
				continue
			}
			out = append(out, mk(id, SourceBuiltin))
		}
		return out
	}
	if p.DefaultModel != "" {
		return []ModelInfo{mk(p.DefaultModel, SourceDefault)}
	}
	return nil
}

func resolveInitial(cfg config.Config) (Selection, string) {
	name := providerOrDefault(cfg.Active.Provider)
	p := cfg.Providers[name]
	cat := staticCatalog(name, p)
	model := p.DefaultModel
	notice := ""
	if cfg.DefaultsModelExplicit && cfg.Defaults.Model != "" {
		if inCatalog(cat, cfg.Defaults.Model) {
			model = cfg.Defaults.Model
		} else {
			notice = fmt.Sprintf(
				"[defaults].model = %q is not a model of active provider %q; ignored",
				cfg.Defaults.Model, name)
		}
	}
	if model == "" && len(cat) > 0 {
		model = cat[0].ID
	}
	return Selection{Provider: name, Model: model, Effort: cfg.Defaults.ReasoningEffort}, notice
}

func inCatalog(cat []ModelInfo, id string) bool {
	for _, m := range cat {
		if m.ID == id {
			return true
		}
	}
	return false
}
