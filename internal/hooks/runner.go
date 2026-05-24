package hooks

import (
	"context"
	"fmt"
	"log/slog"
)

// Runner dispatches hooks at lifecycle events. It owns the list of
// configured hooks and the registry of builtin implementations.
type Runner struct {
	hooks    map[HookEvent][]HookConfig
	builtins map[string]BuiltinHook
}

// NewRunner returns an empty Runner with no hooks configured.
func NewRunner() *Runner {
	return &Runner{
		hooks:    make(map[HookEvent][]HookConfig),
		builtins: make(map[string]BuiltinHook),
	}
}

// Register adds a builtin hook by name. Hook configs whose Type is
// "builtin" and Name matches will dispatch here.
func (r *Runner) Register(name string, fn BuiltinHook) {
	r.builtins[name] = fn
}

// Configure replaces the current hook config list. Typical usage:
// load HookConfig list from TOML, then call Configure once.
func (r *Runner) Configure(configs []HookConfig) {
	r.hooks = make(map[HookEvent][]HookConfig)
	for _, cfg := range configs {
		r.hooks[cfg.Event] = append(r.hooks[cfg.Event], cfg)
	}
}

// Run fires all configured hooks for the given event. Returns the first
// non-"continue" decision. Order: deny short-circuits, then ask, then
// allow. If no hooks are configured for this event, returns allow.
//
// Fail-open: any error or timeout inside a hook results in a "continue"
// decision, logged with the reason, and execution proceeds.
func (r *Runner) Run(ctx context.Context, event HookEvent, in HookInput) (HookOutput, error) {
	configs := r.hooks[event]
	if len(configs) == 0 {
		return HookOutput{Decision: "allow"}, nil
	}

	// Aggregate: deny short-circuits, ask wins over allow, continue is neutral.
	var best HookOutput
	hasAsk := false

	for _, cfg := range configs {
		out, err := r.dispatch(ctx, cfg, in)
		if err != nil {
			slog.Warn("hook error, treating as continue", "event", event, "name", hookLabel(cfg), "err", err)
			continue
		}

		switch out.Decision {
		case "deny":
			return out, nil // immediate short-circuit
		case "ask":
			hasAsk = true
			best = out
		case "allow":
			if best.Decision == "" {
				best = out
			}
		case "continue":
			// neutral
		default:
			slog.Warn("hook returned unknown decision, treating as continue", "event", event, "name", hookLabel(cfg), "decision", out.Decision)
		}
	}

	if hasAsk {
		return best, nil
	}
	if best.Decision == "" {
		return HookOutput{Decision: "allow"}, nil
	}
	return best, nil
}

func (r *Runner) dispatch(ctx context.Context, cfg HookConfig, in HookInput) (HookOutput, error) {
	switch cfg.Type {
	case TypeBuiltin:
		fn, ok := r.builtins[cfg.Name]
		if !ok {
			return HookOutput{}, fmt.Errorf("builtin hook %q not registered", cfg.Name)
		}
		return fn(ctx, in)
	case TypeSubprocess:
		return runSubprocessHook(ctx, cfg, in)
	default:
		return HookOutput{}, fmt.Errorf("unknown hook type %q", cfg.Type)
	}
}

func hookLabel(cfg HookConfig) string {
	switch cfg.Type {
	case TypeBuiltin:
		return cfg.Name
	case TypeSubprocess:
		return cfg.Command
	default:
		return string(cfg.Type)
	}
}
