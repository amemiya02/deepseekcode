package config

import (
	"testing"

	"github.com/BurntSushi/toml"
)

func TestRoutingClarifyOverlayApplied(t *testing.T) {
	base := Default()
	var ov Config
	meta, err := toml.Decode(`
[routing]
auto_route = true
escalation_model = "deepseek-v4-pro"

[clarify]
auto_clarify = true
`, &ov)
	if err != nil {
		t.Fatal(err)
	}
	applyOverlay(&base, ov, meta)

	if !base.Routing.AutoRoute {
		t.Error("routing.auto_route should be applied from overlay")
	}
	if base.Routing.EscalationModel != "deepseek-v4-pro" {
		t.Errorf("escalation_model = %q, want deepseek-v4-pro", base.Routing.EscalationModel)
	}
	if !base.Clarify.AutoClarify {
		t.Error("clarify.auto_clarify should be applied from overlay")
	}
}

func TestRoutingDefaultsOff(t *testing.T) {
	base := Default()
	if base.Routing.AutoRoute || base.Clarify.AutoClarify || base.Routing.EscalationModel != "" {
		t.Fatalf("defaults must be off: route=%v clarify=%v esc=%q",
			base.Routing.AutoRoute, base.Clarify.AutoClarify, base.Routing.EscalationModel)
	}
}
