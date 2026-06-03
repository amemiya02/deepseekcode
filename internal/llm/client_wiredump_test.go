package llm

import "testing"

func TestNewClient_WireDumpFromEnv(t *testing.T) {
	t.Setenv("DEEPSEEKCODE_WIRE_DUMP", "/tmp/dsc-wire-test")
	c := NewClient("k", "http://localhost")
	if c.WireDumpDir != "/tmp/dsc-wire-test" {
		t.Fatalf("WireDumpDir = %q, want /tmp/dsc-wire-test", c.WireDumpDir)
	}
}

func TestNewClient_WireDumpDefaultEmpty(t *testing.T) {
	t.Setenv("DEEPSEEKCODE_WIRE_DUMP", "")
	c := NewClient("k", "http://localhost")
	if c.WireDumpDir != "" {
		t.Fatalf("WireDumpDir = %q, want empty by default", c.WireDumpDir)
	}
}
