// Package hooks provides an extensible hook system for deepseekcode.
// Hooks fire at named lifecycle events (PreToolUse, PostToolUse, etc.)
// and can be either in-process builtin functions or subprocess commands.
//
// Fail-open semantics: a hook that errors or times out returns a
// "continue" decision, never blocking the agent loop.
package hooks

import (
	"context"
	"encoding/json"
	"time"
)

// HookEvent names a point in the agent lifecycle where hooks are fired.
type HookEvent string

const (
	EventPreToolUse         HookEvent = "PreToolUse"
	EventPostToolUse        HookEvent = "PostToolUse"
	EventPostToolUseFailure HookEvent = "PostToolUseFailure"
	EventSessionStart       HookEvent = "SessionStart"
	EventSessionEnd         HookEvent = "SessionEnd"
)

// HookType distinguishes in-process builtins from spawned subprocesses.
type HookType string

const (
	TypeSubprocess HookType = "subprocess"
	TypeBuiltin    HookType = "builtin"
)

// HookConfig describes one registered hook. Event + (Command xor Name)
// identifies what fires and how.
type HookConfig struct {
	Event   HookEvent
	Type    HookType
	Command string        // when Type == subprocess
	Name    string        // when Type == builtin
	Timeout time.Duration // default 30s
}

// HookInput is the JSON payload delivered to every hook invocation.
type HookInput struct {
	Event     HookEvent       `json:"event"`
	ToolName  string          `json:"tool_name,omitempty"`
	ToolInput json.RawMessage `json:"tool_input,omitempty"`
	CWD       string          `json:"cwd"`
	SessionID string          `json:"session_id"`
	// Extra holds event-specific metadata (e.g. tool output for PostToolUse,
	// summary for SessionEnd). Keys vary by event; callers must type-assert.
	Extra map[string]any `json:"extra,omitempty"`
}

// HookOutput is the structured response a hook must produce on stdout
// (subprocess) or return (builtin).
type HookOutput struct {
	Decision string `json:"decision"` // "allow" | "deny" | "ask" | "continue"
	Reason   string `json:"reason,omitempty"`
}

// BuiltinHook is a hook implemented as an in-process Go function.
type BuiltinHook func(ctx context.Context, in HookInput) (HookOutput, error)
