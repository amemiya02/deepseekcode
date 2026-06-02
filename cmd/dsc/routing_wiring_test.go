package main

import (
	"testing"

	"github.com/amemiya02/deepseekcode/internal/agent"
	"github.com/amemiya02/deepseekcode/internal/config"
)

func TestApplyRoutingConfigOptInDefaults(t *testing.T) {
	a := &agent.Agent{}
	applyRoutingConfig(a, config.Config{}) // empty config = everything off
	if a.AutoRoute || a.AutoClarify || a.EscalationModel != "" {
		t.Fatalf("empty config must leave routing off: route=%v clarify=%v esc=%q",
			a.AutoRoute, a.AutoClarify, a.EscalationModel)
	}
}

func TestApplyRoutingConfigDefaultsProModel(t *testing.T) {
	a := &agent.Agent{}
	var cfg config.Config
	cfg.Routing.AutoRoute = true // no escalation model set
	applyRoutingConfig(a, cfg)
	if a.EscalationModel != "deepseek-v4-pro" {
		t.Fatalf("auto_route with no escalation_model should default the pro tier, got %q", a.EscalationModel)
	}
}

func TestApplyRoutingConfigPreservesExplicitEscalation(t *testing.T) {
	a := &agent.Agent{}
	var cfg config.Config
	cfg.Routing.AutoRoute = true
	cfg.Routing.EscalationModel = "deepseek-v4-custom"
	applyRoutingConfig(a, cfg)
	if a.EscalationModel != "deepseek-v4-custom" {
		t.Fatalf("explicit escalation_model should be preserved, got %q", a.EscalationModel)
	}
}

func TestApplyRoutingConfigClarifyPassthrough(t *testing.T) {
	a := &agent.Agent{}
	var cfg config.Config
	cfg.Clarify.AutoClarify = true
	applyRoutingConfig(a, cfg)
	if !a.AutoClarify {
		t.Fatal("clarify.auto_clarify=true should set a.AutoClarify")
	}
}
