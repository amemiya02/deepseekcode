package gateway

import (
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"github.com/amemiya02/deepseekcode/internal/config"
)

// rawIO is the read/write seam over ~/.deepseek/config.toml as a raw map, so the
// MCP writer edits only the targeted [mcp_servers.<name>] table and never
// re-marshals the env-expanded in-memory Config (which would bake $HOME-style
// placeholders in other servers). Tests swap this for an in-memory pair.
type rawIO struct {
	read  func() (map[string]any, error)
	write func(map[string]any) error
}

var mcpRawIO = rawIO{read: readUserConfigRaw, write: writeUserConfigRaw}

func userConfigPath() (string, error) {
	dir := config.UserConfigDir()
	if dir == "" {
		return "", errNoHome{}
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.toml"), nil
}

func readUserConfigRaw() (map[string]any, error) {
	path, err := userConfigPath()
	if err != nil {
		return nil, err
	}
	out := map[string]any{}
	if data, err := os.ReadFile(path); err == nil {
		if err := toml.Unmarshal(data, &out); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func writeUserConfigRaw(m map[string]any) error {
	path, err := userConfigPath()
	if err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return toml.NewEncoder(f).Encode(m)
}

// mutateMCP reads the raw config map, applies fn to the (created-if-absent)
// mcp_servers sub-table, and writes the whole map back. Other tables are
// preserved verbatim.
func mutateMCP(name string, fn func(servers map[string]any)) error {
	existing, err := mcpRawIO.read()
	if err != nil {
		return err
	}
	servers, _ := existing["mcp_servers"].(map[string]any)
	if servers == nil {
		servers = map[string]any{}
	}
	fn(servers)
	existing["mcp_servers"] = servers
	return mcpRawIO.write(existing)
}

// buildOneMCP renders one server config as a TOML-encodable map, omitting empty
// fields so the written table stays minimal.
func buildOneMCP(srv config.MCPServerConfig) map[string]any {
	m := map[string]any{}
	if srv.Transport != "" {
		m["transport"] = srv.Transport
	}
	if srv.Command != "" {
		m["command"] = srv.Command
	}
	if srv.URL != "" {
		m["url"] = srv.URL
	}
	if len(srv.Args) > 0 {
		m["args"] = srv.Args
	}
	if len(srv.Env) > 0 {
		m["env"] = srv.Env
	}
	if srv.TimeoutSeconds > 0 {
		m["timeout_seconds"] = srv.TimeoutSeconds
	}
	if len(srv.EnabledTools) > 0 {
		m["enabled_tools"] = srv.EnabledTools
	}
	if len(srv.DisabledTools) > 0 {
		m["disabled_tools"] = srv.DisabledTools
	}
	if srv.Disabled {
		m["disabled"] = true
	}
	return m
}
