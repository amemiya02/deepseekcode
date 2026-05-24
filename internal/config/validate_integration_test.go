package config

import (
	"testing"
)

func TestValidateStrictIntegrationMisconfigurations(t *testing.T) {
	tests := []struct {
		name      string
		mutate    func(*Config)
		wantPaths []string
	}{
		{
			name: "zero api timeout",
			mutate: func(c *Config) {
				c.API.FirstTokenTimeoutMs = 0
				c.API.ChunkStallTimeoutMs = 0
			},
			wantPaths: []string{"api.first_token_timeout_ms", "api.chunk_stall_timeout_ms"},
		},
		{
			name: "negative api timeout",
			mutate: func(c *Config) {
				c.API.FirstTokenTimeoutMs = -1
			},
			wantPaths: []string{"api.first_token_timeout_ms"},
		},
		{
			name: "mcp server empty command",
			mutate: func(c *Config) {
				c.MCPServers = map[string]MCPServerConfig{
					"broken": {Command: ""},
				}
			},
			wantPaths: []string{"mcp_servers[broken].command"},
		},
		{
			name: "mcp server empty name",
			mutate: func(c *Config) {
				c.MCPServers = map[string]MCPServerConfig{
					"": {Command: "echo"},
				}
			},
			wantPaths: []string{"mcp_servers"},
		},
		{
			name: "hook typo event",
			mutate: func(c *Config) {
				c.Hooks = []HookItemConfig{
					{Event: "PreTooluse", Type: "subprocess", Command: "echo"},
				}
			},
			wantPaths: []string{"hooks[0].event"},
		},
		{
			name: "hook unknown type",
			mutate: func(c *Config) {
				c.Hooks = []HookItemConfig{
					{Event: "PreToolUse", Type: "lambda", Command: "echo"},
				}
			},
			wantPaths: []string{"hooks[0].type"},
		},
		{
			name: "builtin hook missing name",
			mutate: func(c *Config) {
				c.Hooks = []HookItemConfig{
					{Event: "PreToolUse", Type: "builtin", Name: ""},
				}
			},
			wantPaths: []string{"hooks[0].name"},
		},
		{
			name: "subprocess hook missing command",
			mutate: func(c *Config) {
				c.Hooks = []HookItemConfig{
					{Event: "PreToolUse", Type: "subprocess", Command: ""},
				}
			},
			wantPaths: []string{"hooks[0].command"},
		},
		{
			name: "rule empty tool",
			mutate: func(c *Config) {
				c.Permissions.Rules.Deny = []RuleItemConfig{{Tool: "", Args: "rm"}}
			},
			wantPaths: []string{"permissions.rules.deny[0].tool"},
		},
		{
			name: "combined multiple errors",
			mutate: func(c *Config) {
				c.API.FirstTokenTimeoutMs = 0
				c.MCPServers = map[string]MCPServerConfig{
					"x": {Command: ""},
				}
				c.Hooks = []HookItemConfig{
					{Event: "BadEvent", Type: "subprocess", Command: "echo"},
				}
				c.Permissions.Rules.Allow = []RuleItemConfig{{Tool: ""}}
			},
			wantPaths: []string{
				"api.first_token_timeout_ms",
				"mcp_servers[x].command",
				"hooks[0].event",
				"permissions.rules.allow[0].tool",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Default()
			tt.mutate(&cfg)
			errs := ValidateStrict(&cfg)

			found := make(map[string]bool)
			for _, e := range errs {
				found[e.Path] = true
			}
			for _, p := range tt.wantPaths {
				if !found[p] {
					t.Errorf("missing error for path %q; got errors: %v", p, errs)
				}
			}
		})
	}
}

func TestValidateStrictErrorsHaveMessages(t *testing.T) {
	cfg := Default()
	cfg.API.FirstTokenTimeoutMs = 0
	cfg.MCPServers = map[string]MCPServerConfig{
		"bad": {Command: ""},
	}
	errs := ValidateStrict(&cfg)
	for _, e := range errs {
		if e.Message == "" {
			t.Errorf("ValidationError for %q has empty message", e.Path)
		}
		if e.Error() == "" {
			t.Errorf("ValidationError.Error() returns empty for %q", e.Path)
		}
	}
}
