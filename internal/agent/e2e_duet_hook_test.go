package agent

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/amemiya02/deepseekcode/internal/hooks"
	"github.com/amemiya02/deepseekcode/internal/llm"
	"github.com/amemiya02/deepseekcode/internal/permissions"
	"github.com/amemiya02/deepseekcode/internal/tools"
)

// dummyBashTool is a bare-minimum bash tool for Duet hook tests.
// Only implements Name/Description/Parameters/Execute; the Duet hook
// checks IsDestructiveBash based on the command in args, not the tool
// implementation.
type dummyBashTool struct{}

func (dummyBashTool) Name() string        { return "bash" }
func (dummyBashTool) Description() string { return "run a shell command" }
func (dummyBashTool) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"command":{"type":"string"}}}`)
}
func (dummyBashTool) Execute(_ context.Context, args json.RawMessage) (tools.Result, error) {
	return tools.Result{Content: "ok"}, nil
}

// dummyWriteFileTool is a bare-minimum write_file tool for Duet hook tests.
type dummyWriteFileTool struct{}

func (dummyWriteFileTool) Name() string        { return "write_file" }
func (dummyWriteFileTool) Description() string { return "write a file" }
func (dummyWriteFileTool) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"},"content":{"type":"string"}}}`)
}
func (dummyWriteFileTool) Execute(_ context.Context, args json.RawMessage) (tools.Result, error) {
	return tools.Result{Content: "ok"}, nil
}

// fakeDuetClient implements hooks.DuetClient with controllable responses.
type fakeDuetClient struct {
	approve   bool
	reasoning string
	err       error
	calls     int
}

func (f *fakeDuetClient) ValidatePro(_ context.Context, _ string) (bool, string, error) {
	f.calls++
	if f.err != nil {
		return false, "", f.err
	}
	return f.approve, f.reasoning, nil
}

// TestDuetHookDestructiveDeny verifies that a destructive bash call
// (rm -rf) is blocked when the Duet hook returns deny.
func TestDuetHookDestructiveDeny(t *testing.T) {
	reg := tools.New()
	reg.Register(echoTool{})
	reg.Register(dummyBashTool{})

	pol := permissions.New(permissions.ModeYolo, t.TempDir(), nil, nil, nil)

	fake := &fakeDuetClient{approve: false, reasoning: "too dangerous"}

	hr := hooks.NewRunner()
	hr.Register("duet", hooks.NewDuetHook(
		fake,
		nil,                    // extraDestructive
		pol.Cwd,                // cwd
		pol.SecretPathPatterns, // secretPatterns (nil)
		func() string { return "deepseek-v4-flash" },
		func() []byte { return nil },
	))
	hr.Configure([]hooks.HookConfig{
		{Event: hooks.EventPreToolUse, Type: hooks.TypeBuiltin, Name: "duet"},
	})

	a := New(llm.NewClient("k", "http://localhost:1"), reg, pol, "deepseek-v4-flash")
	a.HookRunner = hr

	args := json.RawMessage(`{"command":"rm -rf /tmp/test"}`)
	call := llm.ToolCall{ID: "c1", Type: "function", Function: llm.ToolCallFunc{Name: "bash", Arguments: string(args)}}

	res, _ := a.executeOne(context.Background(), call)

	if !res.IsError {
		t.Error("expected Result.IsError = true when Duet hook denies destructive bash")
	}
	if fake.calls != 1 {
		t.Errorf("expected 1 ValidatePro call, got %d", fake.calls)
	}
}

// TestDuetHookDestructiveApprove verifies that a destructive call is
// allowed when the Duet hook returns approve.
func TestDuetHookDestructiveApprove(t *testing.T) {
	reg := tools.New()
	ct := &countTool{inner: echoTool{}}
	reg.Register(ct)
	reg.Register(dummyBashTool{})

	pol := permissions.New(permissions.ModeYolo, t.TempDir(), nil, nil, nil)

	fake := &fakeDuetClient{approve: true, reasoning: "looks safe"}

	hr := hooks.NewRunner()
	hr.Register("duet", hooks.NewDuetHook(
		fake,
		nil,
		pol.Cwd,
		nil,
		func() string { return "deepseek-v4-flash" },
		func() []byte { return nil },
	))
	hr.Configure([]hooks.HookConfig{
		{Event: hooks.EventPreToolUse, Type: hooks.TypeBuiltin, Name: "duet"},
	})

	a := New(llm.NewClient("k", "http://localhost:1"), reg, pol, "deepseek-v4-flash")
	a.HookRunner = hr

	args := json.RawMessage(`{"command":"rm -rf /tmp/test"}`)
	call := llm.ToolCall{ID: "c1", Type: "function", Function: llm.ToolCallFunc{Name: "bash", Arguments: string(args)}}

	res, _ := a.executeOne(context.Background(), call)

	if res.IsError {
		t.Errorf("expected tool to execute when Duet approves, got error: %s", res.Content)
	}
	if fake.calls != 1 {
		t.Errorf("expected 1 ValidatePro call, got %d", fake.calls)
	}
}

// TestDuetHookSafePass verifies that a non-destructive call bypasses
// the Duet validator entirely.
func TestDuetHookSafePass(t *testing.T) {
	reg := tools.New()
	ct := &countTool{inner: echoTool{}}
	reg.Register(ct)

	pol := permissions.New(permissions.ModeYolo, t.TempDir(), nil, nil, nil)

	fake := &fakeDuetClient{approve: true, reasoning: "ok"}

	hr := hooks.NewRunner()
	hr.Register("duet", hooks.NewDuetHook(
		fake,
		nil,
		pol.Cwd,
		nil,
		func() string { return "deepseek-v4-flash" },
		func() []byte { return nil },
	))
	hr.Configure([]hooks.HookConfig{
		{Event: hooks.EventPreToolUse, Type: hooks.TypeBuiltin, Name: "duet"},
	})

	a := New(llm.NewClient("k", "http://localhost:1"), reg, pol, "deepseek-v4-flash")
	a.HookRunner = hr

	args := json.RawMessage(`{}`)
	call := llm.ToolCall{ID: "c1", Type: "function", Function: llm.ToolCallFunc{Name: "echo", Arguments: string(args)}}

	res, _ := a.executeOne(context.Background(), call)

	if res.IsError {
		t.Errorf("safe tool should execute, got error: %s", res.Content)
	}
	if fake.calls != 0 {
		t.Errorf("expected 0 ValidatePro calls for safe tool, got %d", fake.calls)
	}
}

// TestDuetHookSelfValidates verifies that when the main model is pro,
// the Duet hook skips validation (continue).
func TestDuetHookSelfValidates(t *testing.T) {
	reg := tools.New()
	ct := &countTool{inner: echoTool{}}
	reg.Register(ct)
	reg.Register(dummyBashTool{})

	pol := permissions.New(permissions.ModeYolo, t.TempDir(), nil, nil, nil)

	fake := &fakeDuetClient{approve: true, reasoning: "ok"}

	hr := hooks.NewRunner()
	hr.Register("duet", hooks.NewDuetHook(
		fake,
		nil,
		pol.Cwd,
		nil,
		func() string { return "deepseek-v4-pro" }, // self-validating
		func() []byte { return nil },
	))
	hr.Configure([]hooks.HookConfig{
		{Event: hooks.EventPreToolUse, Type: hooks.TypeBuiltin, Name: "duet"},
	})

	a := New(llm.NewClient("k", "http://localhost:1"), reg, pol, "deepseek-v4-pro")
	a.HookRunner = hr

	args := json.RawMessage(`{"command":"rm -rf /tmp/test"}`)
	call := llm.ToolCall{ID: "c1", Type: "function", Function: llm.ToolCallFunc{Name: "bash", Arguments: string(args)}}

	res, _ := a.executeOne(context.Background(), call)

	if res.IsError {
		t.Errorf("pro self-validating should allow tool, got error: %s", res.Content)
	}
	if fake.calls != 0 {
		t.Errorf("expected 0 ValidatePro calls when self-validating, got %d", fake.calls)
	}
}

// TestDuetHookFailOpen verifies that a ValidatePro error does not block
// tool execution (fail-open).
func TestDuetHookFailOpen(t *testing.T) {
	reg := tools.New()
	ct := &countTool{inner: echoTool{}}
	reg.Register(ct)
	reg.Register(dummyBashTool{})

	pol := permissions.New(permissions.ModeYolo, t.TempDir(), nil, nil, nil)

	fake := &fakeDuetClient{err: context.DeadlineExceeded}

	hr := hooks.NewRunner()
	hr.Register("duet", hooks.NewDuetHook(
		fake,
		nil,
		pol.Cwd,
		nil,
		func() string { return "deepseek-v4-flash" },
		func() []byte { return nil },
	))
	hr.Configure([]hooks.HookConfig{
		{Event: hooks.EventPreToolUse, Type: hooks.TypeBuiltin, Name: "duet"},
	})

	a := New(llm.NewClient("k", "http://localhost:1"), reg, pol, "deepseek-v4-flash")
	a.HookRunner = hr

	args := json.RawMessage(`{"command":"rm -rf /tmp/test"}`)
	call := llm.ToolCall{ID: "c1", Type: "function", Function: llm.ToolCallFunc{Name: "bash", Arguments: string(args)}}

	res, _ := a.executeOne(context.Background(), call)

	if res.IsError {
		t.Errorf("fail-open should allow tool, got error: %s", res.Content)
	}
	if fake.calls != 1 {
		t.Errorf("expected 1 ValidatePro call, got %d", fake.calls)
	}
}

// TestDuetHookSecretPathDeny verifies that write_file to a secret path
// (e.g. .env) triggers the Duet hook and is blocked on deny.
func TestDuetHookSecretPathDeny(t *testing.T) {
	reg := tools.New()
	reg.Register(dummyWriteFileTool{})

	pol := permissions.New(permissions.ModeYolo, t.TempDir(), []string{".env"}, nil, nil)

	fake := &fakeDuetClient{approve: false, reasoning: "secret file"}

	hr := hooks.NewRunner()
	hr.Register("duet", hooks.NewDuetHook(
		fake,
		nil,
		pol.Cwd,
		pol.SecretPathPatterns,
		func() string { return "deepseek-v4-flash" },
		func() []byte { return nil },
	))
	hr.Configure([]hooks.HookConfig{
		{Event: hooks.EventPreToolUse, Type: hooks.TypeBuiltin, Name: "duet"},
	})

	a := New(llm.NewClient("k", "http://localhost:1"), reg, pol, "deepseek-v4-flash")
	a.HookRunner = hr

	args := json.RawMessage(`{"path":".env","content":"SECRET=xyz"}`)
	call := llm.ToolCall{ID: "c1", Type: "function", Function: llm.ToolCallFunc{Name: "write_file", Arguments: string(args)}}

	res, _ := a.executeOne(context.Background(), call)

	if !res.IsError {
		t.Error("expected Result.IsError = true when Duet hook denies write to secret path")
	}
	if fake.calls != 1 {
		t.Errorf("expected 1 ValidatePro call, got %d", fake.calls)
	}
}
