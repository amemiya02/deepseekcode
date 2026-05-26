package agent

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/amemiya02/deepseekcode/internal/llm"
	"github.com/amemiya02/deepseekcode/internal/repair"
	"github.com/amemiya02/deepseekcode/internal/tools"
)

func TestRepairIntegration_ScavengeRecovered(t *testing.T) {
	// Test that repairToolCalls recovers hidden tool calls from content
	agent := &Agent{
		Tools:        tools.New(),
		stormBreaker: repair.NewStormBreaker(6, 3),
		bus:          NewBus(),
	}

	agent.Tools.Register(&mockTool{name: "read_file", readOnly: true})

	// Simulate content containing a hidden tool call (model text output)
	content := `{"name":"read_file","arguments":{"path":"README.md"}}`
	blocks := []llm.ContentBlock{llm.TextBlock{Text: content}}

	// Call repairToolCalls with empty declared calls, content containing hidden call
	kept := agent.repairToolCalls(context.Background(), "", content, nil, &blocks)

	// Should have recovered one call
	if len(kept) != 1 {
		t.Fatalf("expected 1 recovered call, got %d", len(kept))
	}
	if kept[0].Function.Name != "read_file" {
		t.Errorf("expected recovered call named read_file, got %s", kept[0].Function.Name)
	}

	// Blocks should now contain ToolUseBlock for the recovered call
	hasToolUse := false
	for _, b := range blocks {
		if _, ok := b.(llm.ToolUseBlock); ok {
			hasToolUse = true
			break
		}
	}
	if !hasToolUse {
		t.Error("expected blocks to contain ToolUseBlock after recovery")
	}
}

func TestFinishReasonOverride(t *testing.T) {
	// Verify that recovered calls enable tool execution even with empty declared calls
	agent := &Agent{
		Tools:        tools.New(),
		stormBreaker: repair.NewStormBreaker(6, 3),
		bus:          NewBus(),
	}

	agent.Tools.Register(&mockTool{name: "read_file", readOnly: true})

	// Content containing a hidden tool call
	content := `{"name":"read_file","arguments":{"path":"README.md"}}`
	blocks := []llm.ContentBlock{llm.TextBlock{Text: content}}

	// Call repairToolCalls with empty declared (simulating finish_reason=stop but recoverable content)
	kept := agent.repairToolCalls(context.Background(), "", content, nil, &blocks)

	// Merged calls should be non-empty (triggering tool execution)
	if len(kept) == 0 {
		t.Error("expected recovered calls to enable tool execution, got empty")
	}
}

func TestRepairIntegration_StormBreakerSuppresses(t *testing.T) {
	// Storm breaker should suppress repeated identical read-only calls
	agent := &Agent{
		Tools:        tools.New(),
		stormBreaker: repair.NewStormBreaker(6, 3),
		bus:          NewBus(),
	}

	agent.Tools.Register(&mockTool{name: "read_file", readOnly: true})

	// Three identical calls
	declared := []llm.ToolCall{
		{ID: "1", Function: llm.ToolCallFunc{Name: "read_file", Arguments: `{"path":"a"}`}},
		{ID: "2", Function: llm.ToolCallFunc{Name: "read_file", Arguments: `{"path":"a"}`}},
		{ID: "3", Function: llm.ToolCallFunc{Name: "read_file", Arguments: `{"path":"a"}`}},
	}

	blocks := []llm.ContentBlock{
		llm.ToolUseBlock{ID: "1", Name: "read_file"},
		llm.ToolUseBlock{ID: "2", Name: "read_file"},
		llm.ToolUseBlock{ID: "3", Name: "read_file"},
	}

	kept := agent.repairToolCalls(context.Background(), "", "", declared, &blocks)

	// Third call should be suppressed
	if len(kept) != 2 {
		t.Errorf("expected 2 kept calls (third suppressed), got %d", len(kept))
	}

	// Blocks should only contain 2 ToolUseBlocks
	toolUseCount := 0
	for _, b := range blocks {
		if _, ok := b.(llm.ToolUseBlock); ok {
			toolUseCount++
		}
	}
	if toolUseCount != 2 {
		t.Errorf("expected 2 ToolUseBlocks in blocks, got %d", toolUseCount)
	}
}

func TestRepairIntegration_MutatingNeverSuppressed(t *testing.T) {
	// Mutating calls should never be suppressed even when repeated
	agent := &Agent{
		Tools:        tools.New(),
		stormBreaker: repair.NewStormBreaker(6, 3),
		bus:          NewBus(),
	}

	agent.Tools.Register(&mockTool{name: "write_file", readOnly: false})

	// Four identical mutating calls
	declared := []llm.ToolCall{
		{ID: "1", Function: llm.ToolCallFunc{Name: "write_file", Arguments: `{"path":"a"}`}},
		{ID: "2", Function: llm.ToolCallFunc{Name: "write_file", Arguments: `{"path":"a"}`}},
		{ID: "3", Function: llm.ToolCallFunc{Name: "write_file", Arguments: `{"path":"a"}`}},
		{ID: "4", Function: llm.ToolCallFunc{Name: "write_file", Arguments: `{"path":"a"}`}},
	}

	blocks := []llm.ContentBlock{
		llm.ToolUseBlock{ID: "1", Name: "write_file"},
		llm.ToolUseBlock{ID: "2", Name: "write_file"},
		llm.ToolUseBlock{ID: "3", Name: "write_file"},
		llm.ToolUseBlock{ID: "4", Name: "write_file"},
	}

	kept := agent.repairToolCalls(context.Background(), "", "", declared, &blocks)

	// All calls should be kept
	if len(kept) != 4 {
		t.Errorf("expected all 4 mutating calls kept, got %d", len(kept))
	}

	// All 4 ToolUseBlocks should remain
	toolUseCount := 0
	for _, b := range blocks {
		if _, ok := b.(llm.ToolUseBlock); ok {
			toolUseCount++
		}
	}
	if toolUseCount != 4 {
		t.Errorf("expected 4 ToolUseBlocks in blocks, got %d", toolUseCount)
	}
}

func TestRepairIntegration_BlocksUpdatedAfterRepair(t *testing.T) {
	// Verify that blocks are updated to reflect kept calls only
	agent := &Agent{
		Tools:        tools.New(),
		stormBreaker: repair.NewStormBreaker(6, 3),
		bus:          NewBus(),
	}

	agent.Tools.Register(&mockTool{name: "read_file", readOnly: true})

	// Original blocks have text + 3 tool-use blocks
	blocks := []llm.ContentBlock{
		llm.TextBlock{Text: "Let me read the file"},
		llm.ToolUseBlock{ID: "1", Name: "read_file"},
		llm.ToolUseBlock{ID: "2", Name: "read_file"},
		llm.ToolUseBlock{ID: "3", Name: "read_file"},
	}

	declared := []llm.ToolCall{
		{ID: "1", Function: llm.ToolCallFunc{Name: "read_file", Arguments: `{"path":"a"}`}},
		{ID: "2", Function: llm.ToolCallFunc{Name: "read_file", Arguments: `{"path":"a"}`}},
		{ID: "3", Function: llm.ToolCallFunc{Name: "read_file", Arguments: `{"path":"a"}`}},
	}

	kept := agent.repairToolCalls(context.Background(), "", "", declared, &blocks)

	// Should have 2 kept calls (third suppressed)
	if len(kept) != 2 {
		t.Errorf("expected 2 kept calls, got %d", len(kept))
	}

	// Blocks should have: 1 text block + 2 tool-use blocks
	if len(blocks) != 3 {
		t.Errorf("expected 3 blocks (1 text + 2 tool-use), got %d", len(blocks))
	}

	// First block should still be text
	if _, ok := blocks[0].(llm.TextBlock); !ok {
		t.Error("expected first block to be TextBlock")
	}

	// Remaining blocks should be ToolUseBlocks
	for i, b := range blocks[1:] {
		if _, ok := b.(llm.ToolUseBlock); !ok {
			t.Errorf("expected block %d to be ToolUseBlock", i+1)
		}
	}
}

func TestRepairIntegration_ArgsRepaired(t *testing.T) {
	// Verify that malformed arguments are repaired
	agent := &Agent{
		Tools:        tools.New(),
		stormBreaker: repair.NewStormBreaker(6, 3),
		bus:          NewBus(),
	}

	agent.Tools.Register(&mockTool{name: "read_file", readOnly: true})

	// Call with missing closing brace
	declared := []llm.ToolCall{
		{ID: "1", Function: llm.ToolCallFunc{Name: "read_file", Arguments: `{"path":"a"`}},
	}

	blocks := []llm.ContentBlock{llm.ToolUseBlock{ID: "1", Name: "read_file"}}

	kept := agent.repairToolCalls(context.Background(), "", "", declared, &blocks)

	// Should have repaired the arguments
	if len(kept) != 1 {
		t.Fatalf("expected 1 kept call, got %d", len(kept))
	}

	// Arguments should be valid JSON now
	var m map[string]any
	if err := json.Unmarshal([]byte(kept[0].Function.Arguments), &m); err != nil {
		t.Errorf("expected repaired arguments to be valid JSON, got error: %v", err)
	}
}

func TestRepairIntegration_NoOpWhenNoCalls(t *testing.T) {
	// Verify behavior when there are no declared or recoverable calls
	agent := &Agent{
		Tools:        tools.New(),
		stormBreaker: repair.NewStormBreaker(6, 3),
		bus:          NewBus(),
	}

	agent.Tools.Register(&mockTool{name: "read_file", readOnly: true})

	blocks := []llm.ContentBlock{llm.TextBlock{Text: "Hello"}}

	kept := agent.repairToolCalls(context.Background(), "", "", nil, &blocks)

	// Should return empty
	if len(kept) != 0 {
		t.Errorf("expected 0 kept calls, got %d", len(kept))
	}

	// Blocks should remain unchanged (just text)
	if len(blocks) != 1 {
		t.Errorf("expected 1 block, got %d", len(blocks))
	}
}

// mockTool is a minimal Tool implementation for testing
type mockTool struct {
	name    string
	readOnly bool
}

func (m *mockTool) Name() string                          { return m.name }
func (m *mockTool) Description() string                   { return "mock tool" }
func (m *mockTool) Parameters() json.RawMessage           { return json.RawMessage(`{"type":"object"}`) }
func (m *mockTool) Execute(ctx context.Context, args json.RawMessage) (tools.Result, error) {
	return tools.Result{Content: "ok"}, nil
}
func (m *mockTool) IsReadOnly() bool                      { return m.readOnly }
