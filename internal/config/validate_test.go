package config

import (
	"strings"
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

func TestValidateStrictMCPServerSSENoURL(t *testing.T) {
	cfg := Default()
	cfg.MCPServers = map[string]MCPServerConfig{
		"remote": {Transport: "sse", URL: ""},
	}
	errs := ValidateStrict(&cfg)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
	}
	if errs[0].Path != "mcp_servers[remote].url" {
		t.Errorf("path = %q, want mcp_servers[remote].url", errs[0].Path)
	}
}

func TestValidateStrictMCPServerSSEWithURL(t *testing.T) {
	cfg := Default()
	cfg.MCPServers = map[string]MCPServerConfig{
		"remote": {Transport: "sse", URL: "https://example.com/mcp/sse"},
	}
	errs := ValidateStrict(&cfg)
	for _, e := range errs {
		if strings.Contains(e.Path, "mcp_servers[remote]") {
			t.Errorf("unexpected MCP error: %v", e)
		}
	}
}

func TestValidateStrictMCPServerUnknownTransport(t *testing.T) {
	cfg := Default()
	cfg.MCPServers = map[string]MCPServerConfig{
		"remote": {Transport: "websocket"},
	}
	errs := ValidateStrict(&cfg)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
	}
	if errs[0].Path != "mcp_servers[remote].transport" {
		t.Errorf("path = %q, want mcp_servers[remote].transport", errs[0].Path)
	}
}

func TestValidateStrictMCPServerSSENoCommandRequired(t *testing.T) {
	cfg := Default()
	cfg.MCPServers = map[string]MCPServerConfig{
		"remote": {Transport: "sse", URL: "https://example.com/mcp/sse", Command: ""},
	}
	// SSE transport does not require Command — should not produce a command error.
	errs := ValidateStrict(&cfg)
	for _, e := range errs {
		if strings.Contains(e.Path, "mcp_servers[remote].command") {
			t.Errorf("SSE transport should not require command, got error: %v", e)
		}
	}
}

func TestValidateStrictMCPServerBothEnabledAndDisabled(t *testing.T) {
	cfg := Default()
	cfg.MCPServers = map[string]MCPServerConfig{
		"fs": {
			Command:       "mcp-fs",
			EnabledTools:  []string{"read_file"},
			DisabledTools: []string{"delete_file"},
		},
	}
	errs := ValidateStrict(&cfg)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
	}
	if errs[0].Path != "mcp_servers[fs].enabled_tools" {
		t.Errorf("path = %q, want mcp_servers[fs].enabled_tools", errs[0].Path)
	}
}

func TestValidateStrictMCPServerDisabledToolsOnly(t *testing.T) {
	cfg := Default()
	cfg.MCPServers = map[string]MCPServerConfig{
		"fs": {
			Command:       "mcp-fs",
			DisabledTools: []string{"delete_file"},
		},
	}
	errs := ValidateStrict(&cfg)
	for _, e := range errs {
		if strings.Contains(e.Path, "mcp_servers[fs]") {
			t.Errorf("disabled_tools only should be valid, got error: %v", e)
		}
	}
}

func TestValidateStrictMCPServerEnabledToolsOnly(t *testing.T) {
	cfg := Default()
	cfg.MCPServers = map[string]MCPServerConfig{
		"github": {
			Command:      "mcp-github",
			EnabledTools: []string{"search_issues"},
		},
	}
	errs := ValidateStrict(&cfg)
	for _, e := range errs {
		if strings.Contains(e.Path, "mcp_servers[github]") {
			t.Errorf("enabled_tools only should be valid, got error: %v", e)
		}
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

func TestValidateStrictBuiltinUnknownName(t *testing.T) {
	// A typo'd builtin name must be rejected at validation time (it would
	// otherwise fail-open silently at runtime).
	cfg := Default()
	cfg.Hooks = []HookItemConfig{
		{Event: "PreToolUse", Type: "builtin", Name: "duett"},
	}
	errs := ValidateStrict(&cfg)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
	}
	if errs[0].Path != "hooks[0].name" {
		t.Errorf("path = %q, want hooks[0].name", errs[0].Path)
	}
	if !strings.Contains(errs[0].Message, "duet") {
		t.Errorf("message %q should list the valid builtin name", errs[0].Message)
	}
}

func TestValidateStrictBuiltinKnownName(t *testing.T) {
	// Regression: the one registered builtin ("duet") must still pass.
	cfg := Default()
	cfg.Hooks = []HookItemConfig{
		{Event: "PreToolUse", Type: "builtin", Name: "duet"},
	}
	for _, e := range ValidateStrict(&cfg) {
		if strings.HasPrefix(e.Path, "hooks[0]") {
			t.Errorf("known builtin should not error, got %v", e)
		}
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

func TestValidateStrictProviderType(t *testing.T) {
	cfg := Default()
	cfg.Providers["bad"] = ProviderConfigTOML{Type: "bogus", BaseURL: "https://example.com"}
	errs := ValidateStrict(&cfg)
	var found bool
	for _, err := range errs {
		if err.Path == "providers[bad].type" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected providers[bad].type error, got %v", errs)
	}
}

func TestValidateStrictActiveProvider(t *testing.T) {
	cfg := Default()
	cfg.Active.Provider = "missing"
	errs := ValidateStrict(&cfg)
	if len(errs) == 0 || errs[0].Path != "active.provider" {
		t.Fatalf("expected active.provider error, got %v", errs)
	}
}
