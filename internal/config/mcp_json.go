package config

import (
	"encoding/json"
	"fmt"
	"os"
)

// LoadDotMCPJSON reads a Claude/Cursor .mcp.json file and returns the
// servers as MCPServerConfig entries. An absent file returns (nil, nil).
func LoadDotMCPJSON(path string) (map[string]MCPServerConfig, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading .mcp.json: %w", err)
	}
	var doc struct {
		MCPServers map[string]struct {
			Command   string            `json:"command"`
			Args      []string          `json:"args"`
			Env       map[string]string `json:"env"`
			URL       string            `json:"url"`
			Transport string            `json:"transport"`
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		return nil, fmt.Errorf("parsing .mcp.json: %w", err)
	}
	out := make(map[string]MCPServerConfig, len(doc.MCPServers))
	for name, raw := range doc.MCPServers {
		out[name] = MCPServerConfig{
			Transport: raw.Transport,
			URL:       raw.URL,
			Command:   raw.Command,
			Args:      raw.Args,
			Env:       raw.Env,
		}
	}
	return out, nil
}
