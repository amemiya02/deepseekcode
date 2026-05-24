package config

import (
	"testing"
)

func TestValidateStrictValidDefault(t *testing.T) {
	cfg := Default()
	errs := ValidateStrict(&cfg)
	if len(errs) > 0 {
		t.Fatalf("default config should be valid, got: %v", errs)
	}
}

func TestValidateStrictAPITimeouts(t *testing.T) {
	cfg := Default()
	cfg.API.FirstTokenTimeoutMs = 0
	cfg.API.ChunkStallTimeoutMs = -1
	errs := ValidateStrict(&cfg)
	if len(errs) != 2 {
		t.Fatalf("expected 2 errors, got %d: %v", len(errs), errs)
	}
	var foundFirst, foundChunk bool
	for _, e := range errs {
		if e.Path == "api.first_token_timeout_ms" {
			foundFirst = true
		}
		if e.Path == "api.chunk_stall_timeout_ms" {
			foundChunk = true
		}
	}
	if !foundFirst {
		t.Error("missing first_token_timeout_ms error")
	}
	if !foundChunk {
		t.Error("missing chunk_stall_timeout_ms error")
	}
}

func TestValidateStrictMCPServerEmptyCommand(t *testing.T) {
	cfg := Default()
	cfg.MCPServers = map[string]MCPServerConfig{
		"myserver": {Command: ""},
	}
	errs := ValidateStrict(&cfg)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
	}
	if errs[0].Path != "mcp_servers[myserver].command" {
		t.Errorf("path = %q, want mcp_servers[myserver].command", errs[0].Path)
	}
}

func TestValidateStrictMCPServerEmptyName(t *testing.T) {
	cfg := Default()
	cfg.MCPServers = map[string]MCPServerConfig{
		"": {Command: "echo"},
	}
	errs := ValidateStrict(&cfg)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
	}
	if errs[0].Path != "mcp_servers" {
		t.Errorf("path = %q, want mcp_servers", errs[0].Path)
	}
}

func TestValidateStrictHookBadEvent(t *testing.T) {
	cfg := Default()
	cfg.Hooks = []HookItemConfig{
		{Event: "BadEvent", Type: "subprocess", Command: "echo hi"},
	}
	errs := ValidateStrict(&cfg)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
	}
	if errs[0].Path != "hooks[0].event" {
		t.Errorf("path = %q, want hooks[0].event", errs[0].Path)
	}
}

func TestValidateStrictHookBadType(t *testing.T) {
	cfg := Default()
	cfg.Hooks = []HookItemConfig{
		{Event: "PreToolUse", Type: "badtype", Command: "echo hi"},
	}
	errs := ValidateStrict(&cfg)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
	}
	if errs[0].Path != "hooks[0].type" {
		t.Errorf("path = %q, want hooks[0].type", errs[0].Path)
	}
}

func TestValidateStrictBuiltinNoName(t *testing.T) {
	cfg := Default()
	cfg.Hooks = []HookItemConfig{
		{Event: "PreToolUse", Type: "builtin", Name: ""},
	}
	errs := ValidateStrict(&cfg)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
	}
	if errs[0].Path != "hooks[0].name" {
		t.Errorf("path = %q, want hooks[0].name", errs[0].Path)
	}
}

func TestValidateStrictSubprocessNoCommand(t *testing.T) {
	cfg := Default()
	cfg.Hooks = []HookItemConfig{
		{Event: "PreToolUse", Type: "subprocess", Command: ""},
	}
	errs := ValidateStrict(&cfg)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
	}
	if errs[0].Path != "hooks[0].command" {
		t.Errorf("path = %q, want hooks[0].command", errs[0].Path)
	}
}

func TestValidateStrictRuleEmptyTool(t *testing.T) {
	cfg := Default()
	cfg.Permissions.Rules.Allow = []RuleItemConfig{{Tool: "", Args: ".*"}}
	cfg.Permissions.Rules.Deny = []RuleItemConfig{{Tool: "bash", Args: "rm"}}
	cfg.Permissions.Rules.Ask = []RuleItemConfig{{Tool: "", Args: ".*"}}
	errs := ValidateStrict(&cfg)
	if len(errs) != 2 {
		t.Fatalf("expected 2 errors, got %d: %v", len(errs), errs)
	}
	var foundAllow, foundAsk bool
	for _, e := range errs {
		if e.Path == "permissions.rules.allow[0].tool" {
			foundAllow = true
		}
		if e.Path == "permissions.rules.ask[0].tool" {
			foundAsk = true
		}
	}
	if !foundAllow {
		t.Error("missing allow[0].tool error")
	}
	if !foundAsk {
		t.Error("missing ask[0].tool error")
	}
}

func TestValidateStrictMultipleErrors(t *testing.T) {
	cfg := Default()
	cfg.API.FirstTokenTimeoutMs = 0
	cfg.MCPServers = map[string]MCPServerConfig{
		"bad": {Command: ""},
	}
	cfg.Hooks = []HookItemConfig{
		{Event: "BadEvent", Type: "subprocess", Command: "echo"},
	}
	errs := ValidateStrict(&cfg)
	if len(errs) < 3 {
		t.Fatalf("expected >=3 errors, got %d: %v", len(errs), errs)
	}
}
