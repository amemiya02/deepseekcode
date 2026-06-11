package modelreg

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/amemiya02/deepseekcode/internal/config"
	"github.com/amemiya02/deepseekcode/internal/llm"
)

type httpFetcher struct{ client *http.Client }

func (f *httpFetcher) Fetch(ctx context.Context, p config.ProviderConfigTOML) ([]string, error) {
	if p.BaseURL == "" {
		return nil, fmt.Errorf("no base_url")
	}
	cl := f.client
	if cl == nil {
		cl = &http.Client{Timeout: 5 * time.Second}
	}
	url := strings.TrimRight(p.BaseURL, "/") + "/models"
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
	var body struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(body.Data))
	for _, d := range body.Data {
		if d.ID != "" {
			ids = append(ids, d.ID)
		}
	}
	return ids, nil
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

func (DefaultWriter) SetActiveProvider(name string) error    { return config.SetActiveProvider(name) }
func (DefaultWriter) SetProviderModel(p, model string) error { return config.SetProviderModel(p, model) }

var _ Fetcher = (*httpFetcher)(nil)
var _ ConfigWriter = DefaultWriter{}
