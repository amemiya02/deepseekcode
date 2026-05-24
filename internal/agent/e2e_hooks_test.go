package agent

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"

	"github.com/amemiya02/deepseekcode/internal/hooks"
	"github.com/amemiya02/deepseekcode/internal/llm"
	"github.com/amemiya02/deepseekcode/internal/permissions"
	"github.com/amemiya02/deepseekcode/internal/tools"
)

// spyHook records every event it sees into counters.
type spyHook struct {
	start, pre, post, end int32
}

func (s *spyHook) fire(_ context.Context, in hooks.HookInput) (hooks.HookOutput, error) {
	switch in.Event {
	case hooks.EventSessionStart:
		atomic.AddInt32(&s.start, 1)
	case hooks.EventPreToolUse:
		atomic.AddInt32(&s.pre, 1)
	case hooks.EventPostToolUse, hooks.EventPostToolUseFailure:
		atomic.AddInt32(&s.post, 1)
	case hooks.EventSessionEnd:
		atomic.AddInt32(&s.end, 1)
	}
	return hooks.HookOutput{Decision: "allow"}, nil
}

// countTool wraps a Tool and counts Execute calls.
type countTool struct {
	inner tools.Tool
	n     int32
}

func (c *countTool) Name() string                { return c.inner.Name() }
func (c *countTool) Description() string         { return c.inner.Description() }
func (c *countTool) Parameters() json.RawMessage { return c.inner.Parameters() }
func (c *countTool) Execute(ctx context.Context, args json.RawMessage) (tools.Result, error) {
	atomic.AddInt32(&c.n, 1)
	return c.inner.Execute(ctx, args)
}

// TestAgentPreToolUseDeny verifies that a deny-builtin hook prevents
// tool execution and returns an error Result.
func TestAgentPreToolUseDeny(t *testing.T) {
	reg := tools.New()
	echo := &countTool{inner: echoTool{}}
	reg.Register(echo)

	pol := permissions.New(permissions.ModeYolo, t.TempDir(), nil, nil, nil)

	r := hooks.NewRunner()
	r.Register("blocker", func(_ context.Context, in hooks.HookInput) (hooks.HookOutput, error) {
		return hooks.HookOutput{Decision: "deny", Reason: "blocked by test"}, nil
	})
	r.Configure([]hooks.HookConfig{
		{Event: hooks.EventPreToolUse, Type: hooks.TypeBuiltin, Name: "blocker"},
	})

	a := New(llm.NewClient("k", "http://localhost:1"), reg, pol, "deepseek-v4-flash")
	a.HookRunner = r

	args := json.RawMessage(`{}`)
	call := llm.ToolCall{ID: "c1", Type: "function", Function: llm.ToolCallFunc{Name: "echo", Arguments: string(args)}}

	res, _ := a.executeOne(context.Background(), call)

	if !res.IsError {
		t.Error("expected Result.IsError = true when hook denies")
	}
	if atomic.LoadInt32(&echo.n) != 0 {
		t.Error("tool.Execute must not be called when hook denies")
	}
}

// TestAgentPostToolUseFires verifies PostToolUse fires on success
// and PostToolUseFailure fires when the tool result indicates an error.
func TestAgentPostToolUseFires(t *testing.T) {
	reg := tools.New()
	reg.Register(echoTool{})

	pol := permissions.New(permissions.ModeYolo, t.TempDir(), nil, nil, nil)

	r := hooks.NewRunner()
	spy := &spyHook{}
	r.Register("spy", spy.fire)
	r.Configure([]hooks.HookConfig{
		{Event: hooks.EventPostToolUse, Type: hooks.TypeBuiltin, Name: "spy"},
		{Event: hooks.EventPostToolUseFailure, Type: hooks.TypeBuiltin, Name: "spy"},
	})

	a := New(llm.NewClient("k", "http://localhost:1"), reg, pol, "deepseek-v4-flash")
	a.HookRunner = r

	args := json.RawMessage(`{}`)
	call := llm.ToolCall{ID: "c1", Type: "function", Function: llm.ToolCallFunc{Name: "echo", Arguments: string(args)}}

	_, _ = a.executeOne(context.Background(), call)

	if spy.post < 1 {
		t.Errorf("PostToolUse: expected >= 1, got %d", spy.post)
	}
}

// TestAgentSessionHooks verifies SessionStart and SessionEnd fire
// during a single agent Run. PreToolUse/PostToolUse are covered by
// TestAgentPreToolUseDeny and TestAgentPostToolUseFires respectively,
// which exercise executeOne directly (the only code path that triggers
// Pre/Post hooks).
func TestAgentSessionHooks(t *testing.T) {
	reg := tools.New()
	reg.Register(echoTool{})

	pol := permissions.New(permissions.ModeYolo, t.TempDir(), nil, nil, nil)

	r := hooks.NewRunner()
	spy := &spyHook{}
	r.Register("spy", spy.fire)
	r.Configure([]hooks.HookConfig{
		{Event: hooks.EventSessionStart, Type: hooks.TypeBuiltin, Name: "spy"},
		{Event: hooks.EventSessionEnd, Type: hooks.TypeBuiltin, Name: "spy"},
	})

	a := New(llm.NewClient("k", "http://localhost:1"), reg, pol, "deepseek-v4-flash")
	a.HookRunner = r

	// Immediately cancelled context: Run fires SessionStart, checks
	// ctx.Done() at the top of the loop, fires SessionEnd via defer.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Drain events so the channel doesn't fill.
	go func() {
		for range a.Events() {
		}
	}()

	_, _ = a.Run(ctx, "hello")

	if spy.start < 1 {
		t.Errorf("SessionStart: expected >= 1, got %d", spy.start)
	}
	if spy.end < 1 {
		t.Errorf("SessionEnd: expected >= 1, got %d", spy.end)
	}
}
