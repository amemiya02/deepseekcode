package config

import "testing"

func TestActiveMCPServersExcludesDisabled(t *testing.T) {
	c := Config{MCPServers: map[string]MCPServerConfig{
		"on":  {Command: "a"},
		"off": {Command: "b", Disabled: true},
	}}
	active := c.ActiveMCPServers()
	if len(active) != 1 {
		t.Fatalf("len = %d, want 1", len(active))
	}
	if _, ok := active["on"]; !ok {
		t.Error("enabled server missing")
	}
	if _, ok := active["off"]; ok {
		t.Error("disabled server should be excluded")
	}
}
