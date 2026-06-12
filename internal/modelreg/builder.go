package modelreg

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/amemiya02/deepseekcode/internal/config"
	"github.com/amemiya02/deepseekcode/internal/llm"
)

type httpFetcher struct{ client *http.Client }

func (f *httpFetcher) Fetch(ctx context.Context, p config.ProviderConfigTOML) ([]FetchedModel, error) {
	if p.BaseURL == "" {
		return nil, fmt.Errorf("no base_url")
	}
	cl := f.client
	if cl == nil {
		cl = &http.Client{Timeout: 5 * time.Second}
	}
	url := llm.ModelsEndpoint(p.BaseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	if key, err := config.ResolveSecret(p); err == nil && key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	resp, err := cl.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("models endpoint: %s", resp.Status)
	}
	// Context-window keys vary across OpenAI-compatible servers: context_length
	// (OpenRouter), max_model_len (vLLM), context_window (others). Parse all and
	// take the first non-zero so the picker can show a real window dynamically.
	var body struct {
		Data []struct {
			ID            string `json:"id"`
			ContextLength int    `json:"context_length"`
			MaxModelLen   int    `json:"max_model_len"`
			ContextWindow int    `json:"context_window"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, err
	}
	out := make([]FetchedModel, 0, len(body.Data))
	for _, d := range body.Data {
		if d.ID == "" {
			continue
		}
		cw := d.ContextLength
		if cw == 0 {
			cw = d.MaxModelLen
		}
		if cw == 0 {
			cw = d.ContextWindow
		}
		out = append(out, FetchedModel{ID: d.ID, ContextTokens: cw})
	}
	return out, nil
}

func DefaultBuilder(cfg config.Config, providerName string) (BuildResult, error) {
	name := providerOrDefault(providerName)
	pcfg, ok := cfg.Providers[name]
	if !ok && name != "deepseek" {
		return BuildResult{}, fmt.Errorf("provider %q is not configured", name)
	}
	var apiKey string
	if ok {
		var err error
		apiKey, err = config.ResolveSecret(pcfg)
		if err != nil {
			return BuildResult{}, err
		}
	}
	prov, err := llm.NewProvider(providerType(name, pcfg), llm.ProviderConfig{
		Name:                name,
		BaseURL:             pcfg.BaseURL,
		APIKey:              apiKey,
		FirstTokenTimeoutMs: pcfg.FirstTokenTimeoutMs,
		ChunkStallTimeoutMs: pcfg.ChunkStallTimeoutMs,
		DefaultModel:        pcfg.DefaultModel,
		ValidationModel:     pcfg.DefaultModel,
	})
	if err != nil {
		return BuildResult{}, err
	}
	client := prov.BaseClient().WithProxyTransport()
	if apiKey != "" {
		client.APIKey = apiKey
	}
	if pcfg.BaseURL != "" {
		client.BaseURL = pcfg.BaseURL
	}
	return BuildResult{Client: client, Caps: prov.Capabilities()}, nil
}

type DefaultWriter struct{}

func (DefaultWriter) SetActiveProvider(name string) error { return config.SetActiveProvider(name) }
func (DefaultWriter) SetProviderModel(p, model string) error {
	return config.SetProviderModel(p, model)
}

var _ Fetcher = (*httpFetcher)(nil)
var _ ConfigWriter = DefaultWriter{}
