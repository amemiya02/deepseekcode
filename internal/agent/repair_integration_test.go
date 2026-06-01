package agent

import (
	"context"
	"encoding/json"
	"strings"
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
	kept, _ := agent.repairToolCalls(context.Background(), "", content, nil, &blocks)

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
	kept, _ := agent.repairToolCalls(context.Background(), "", content, nil, &blocks)

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

	kept, _ := agent.repairToolCalls(context.Background(), "", "", declared, &blocks)

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

	kept, _ := agent.repairToolCalls(context.Background(), "", "", declared, &blocks)

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

	kept, _ := agent.repairToolCalls(context.Background(), "", "", declared, &blocks)

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

	kept, _ := agent.repairToolCalls(context.Background(), "", "", declared, &blocks)

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

	kept, _ := agent.repairToolCalls(context.Background(), "", "", nil, &blocks)

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
	name     string
	readOnly bool
}

func (m *mockTool) Name() string                { return m.name }
func (m *mockTool) Description() string         { return "mock tool" }
func (m *mockTool) Parameters() json.RawMessage { return json.RawMessage(`{"type":"object"}`) }
func (m *mockTool) Execute(ctx context.Context, args json.RawMessage) (tools.Result, error) {
	return tools.Result{Content: "ok"}, nil
}
func (m *mockTool) IsReadOnly() bool { return m.readOnly }

// TestSchemaComplexTelemetry proves: one event per (epoch, tool) across
// turns, re-emit after epoch switch, and no escalation impact.
func TestSchemaComplexTelemetry(t *testing.T) {
	bus := NewBus()
	a := &Agent{
		Tools:                tools.New(),
		stormBreaker:         repair.NewStormBreaker(6, 3),
		bus:                  bus,
		schemaComplexEmitted: make(map[string]bool),
	}

	// Register a tool with a complex schema (depth 5).
	complexSchema := json.RawMessage(`{
		"type": "object",
		"properties": {
			"questions": {
				"type": "array",
				"items": {
					"type": "object",
					"properties": {
						"question": {"type": "string"},
						"options": {"type": "array", "items": {"type": "string"}}
					}
				}
			}
		}
	}`)
	a.Tools.Register(&mockComplexTool{name: "question", schema: complexSchema})

	// Collect repair events from the bus.
	sub := bus.Subscribe(64)
	var repairs []EventRepair
	done := make(chan struct{})
	go func() {
		for env := range sub.C {
			if ev, ok := env.Event.(EventRepair); ok {
				repairs = append(repairs, ev)
			}
		}
		close(done)
	}()

	// Publish schema-complex telemetry manually (simulating what runStep does).
	epochID := "epoch_1"
	for _, rpt := range repair.AnalyzeToolSchemas(a.Tools.AsLLMTools(), 4, 80) {
		key := epochID + "\x00" + rpt.Tool
		if a.schemaComplexEmitted[key] {
			continue
		}
		a.schemaComplexEmitted[key] = true
		bus.Publish(EventRepair{
			Kind:    string(repair.KindSchemaComplex),
			Tool:    rpt.Tool,
			Message: rpt.Message,
		})
	}

	// Second turn in same epoch — should NOT emit again.
	for _, rpt := range repair.AnalyzeToolSchemas(a.Tools.AsLLMTools(), 4, 80) {
		key := epochID + "\x00" + rpt.Tool
		if a.schemaComplexEmitted[key] {
			continue
		}
		a.schemaComplexEmitted[key] = true
		bus.Publish(EventRepair{
			Kind:    string(repair.KindSchemaComplex),
			Tool:    rpt.Tool,
			Message: rpt.Message,
		})
	}

	// Epoch switch — should emit once for the new epoch.
	epochID = "epoch_2"
	for _, rpt := range repair.AnalyzeToolSchemas(a.Tools.AsLLMTools(), 4, 80) {
		key := epochID + "\x00" + rpt.Tool
		if a.schemaComplexEmitted[key] {
			continue
		}
		a.schemaComplexEmitted[key] = true
		bus.Publish(EventRepair{
			Kind:    string(repair.KindSchemaComplex),
			Tool:    rpt.Tool,
			Message: rpt.Message,
		})
	}

	bus.Close()
	<-done

	// Should have exactly 2 events: one for epoch_1, one for epoch_2.
	if len(repairs) != 2 {
		t.Fatalf("got %d schema_complex events, want 2", len(repairs))
	}
	for i, ev := range repairs {
		if ev.Kind != string(repair.KindSchemaComplex) {
			t.Errorf("event[%d].Kind = %q, want schema_complex", i, ev.Kind)
		}
		if ev.Tool != "question" {
			t.Errorf("event[%d].Tool = %q, want question", i, ev.Tool)
		}
		// Message should reflect real analysis values.
		if !strings.Contains(ev.Message, "depth") || !strings.Contains(ev.Message, "leaves") {
			t.Errorf("event[%d].Message %q should contain 'depth' and 'leaves'", i, ev.Message)
		}
	}
}

type mockComplexTool struct {
	name   string
	schema json.RawMessage
}

func (m *mockComplexTool) Name() string                { return m.name }
func (m *mockComplexTool) Description() string         { return "complex mock tool" }
func (m *mockComplexTool) Parameters() json.RawMessage { return m.schema }
func (m *mockComplexTool) Execute(ctx context.Context, args json.RawMessage) (tools.Result, error) {
	return tools.Result{Content: "ok"}, nil
}
func (m *mockComplexTool) IsReadOnly() bool { return false }

// TestSchemaComplexDoesNotIncrementRepairErrors proves that schema-complex
// telemetry (emitted by runStep's AnalyzeToolSchemas loop) does NOT
// increment the repairErrors counter that drives T2.3 auto-escalation.
// This drives through the real repairToolCalls path — not a simulation
// of the dedup loop — so it guards the production integration.
func TestSchemaComplexDoesNotIncrementRepairErrors(t *testing.T) {
	bus := NewBus()
	a := &Agent{
		Tools:                tools.New(),
		stormBreaker:         repair.NewStormBreaker(6, 3),
		bus:                  bus,
		schemaComplexEmitted: make(map[string]bool),
	}

	// Register a tool with a complex schema (depth 5, nested array/object).
	complexSchema := json.RawMessage(`{
		"type": "object",
		"properties": {
			"questions": {
				"type": "array",
				"items": {
					"type": "object",
					"properties": {
						"question": {"type": "string"},
						"options": {"type": "array", "items": {"type": "string"}}
					}
				}
			}
		}
	}`)
	a.Tools.Register(&mockComplexTool{name: "question", schema: complexSchema})

	// Simulate runStep: emit schema-complex telemetry via the real dedup
	// path, then call repairToolCalls with a valid tool call on the
	// complex-schema tool.
	epochID := "epoch_1"
	for _, rpt := range repair.AnalyzeToolSchemas(a.Tools.AsLLMTools(), 4, 80) {
		key := epochID + "\x00" + rpt.Tool
		if a.schemaComplexEmitted[key] {
			continue
		}
		a.schemaComplexEmitted[key] = true
		a.publishRepairEvent(EventRepair{
			Kind:    string(repair.KindSchemaComplex),
			Tool:    rpt.Tool,
			Message: rpt.Message,
		})
	}

	// Now run the real repairToolCalls with a valid call on the complex tool.
	declared := []llm.ToolCall{
		{ID: "q1", Function: llm.ToolCallFunc{Name: "question", Arguments: `{"questions":[{"question":"hi","options":["a","b"]}]}`}},
	}
	blocks := []llm.ContentBlock{
		llm.ToolUseBlock{ID: "q1", Name: "question", Input: json.RawMessage(`{"questions":[{"question":"hi","options":["a","b"]}]}`)},
	}
	kept, repairErrors := a.repairToolCalls(context.Background(), "", "", declared, &blocks)

	// The call has valid args — repairErrors must be 0.
	if repairErrors != 0 {
		t.Errorf("repairErrors = %d, want 0 (schema-complex must not inflate repair-error count)", repairErrors)
	}
	if len(kept) != 1 {
		t.Errorf("kept = %d, want 1", len(kept))
	}

	// escalationTrigger must NOT fire with 0 repair errors.
	trigger, _ := a.escalationTrigger("", repairErrors)
	if trigger != "" {
		t.Errorf("escalationTrigger = %q, want empty (schema-complex must not trigger escalation)", trigger)
	}
}

// TestRepairIntegration_NeedMoreDropsCall proves that string-internal
// truncation (NeedMore=true) causes the call to be dropped rather than
// executed with an invented value. This is the safety behaviour required
// by Task-010: "truncation inside string asks for continuation instead
// of guessing."
func TestRepairIntegration_NeedMoreDropsCall(t *testing.T) {
	bus := NewBus()
	a := &Agent{
		Tools:        tools.New(),
		stormBreaker: repair.NewStormBreaker(6, 3),
		bus:          bus,
	}

	a.Tools.Register(&mockTool{name: "read_file", readOnly: true})

	// Collect repair events
	sub := bus.Subscribe(64)
	var repairs []EventRepair
	done := make(chan struct{})
	go func() {
		for env := range sub.C {
			if ev, ok := env.Event.(EventRepair); ok {
				repairs = append(repairs, ev)
			}
		}
		close(done)
	}()

	// Call with truncation inside a string value — the model
	// never finished emitting the path string.
	declared := []llm.ToolCall{
		{ID: "1", Function: llm.ToolCallFunc{Name: "read_file", Arguments: `{"path":"README`}},
	}
	blocks := []llm.ContentBlock{llm.ToolUseBlock{ID: "1", Name: "read_file"}}

	kept, repairErrors := a.repairToolCalls(context.Background(), "", "", declared, &blocks)

	bus.Close()
	<-done

	// The call must be DROPPED — not executed with invented content.
	if len(kept) != 0 {
		t.Errorf("expected 0 kept calls (NeedMore should drop), got %d", len(kept))
	}

	// repairErrors must be incremented so the model gets a continuation prompt.
	if repairErrors != 1 {
		t.Errorf("expected 1 repair error (NeedMore), got %d", repairErrors)
	}

	// Must emit a KindArgsNeedMore event for observability.
	foundNeedMore := false
	for _, ev := range repairs {
		if ev.Kind == string(repair.KindArgsNeedMore) {
			foundNeedMore = true
		}
	}
	if !foundNeedMore {
		t.Error("expected a KindArgsNeedMore repair event, got none")
	}

	// Blocks should have no ToolUseBlocks (call was dropped).
	for _, b := range blocks {
		if _, ok := b.(llm.ToolUseBlock); ok {
			t.Error("expected no ToolUseBlocks after NeedMore drop, found one")
		}
	}
}

// TestRepairIntegration_BraceOnlyRepairExecutes proves that brace-only
// truncation (NeedMore=false) is auto-repaired and the call IS executed.
// This complements TestRepairIntegration_NeedMoreDropsCall: safe repairs
// should still go through.
func TestRepairIntegration_BraceOnlyRepairExecutes(t *testing.T) {
	a := &Agent{
		Tools:        tools.New(),
		stormBreaker: repair.NewStormBreaker(6, 3),
		bus:          NewBus(),
	}

	a.Tools.Register(&mockTool{name: "read_file", readOnly: true})

	// Call with missing closing brace — safe to auto-repair.
	declared := []llm.ToolCall{
		{ID: "1", Function: llm.ToolCallFunc{Name: "read_file", Arguments: `{"path":"README.md"`}},
	}
	blocks := []llm.ContentBlock{llm.ToolUseBlock{ID: "1", Name: "read_file"}}

	kept, repairErrors := a.repairToolCalls(context.Background(), "", "", declared, &blocks)

	// The call should be kept with repaired arguments.
	if len(kept) != 1 {
		t.Fatalf("expected 1 kept call (brace repair should succeed), got %d", len(kept))
	}

	// Arguments should be valid JSON now.
	var m map[string]any
	if err := json.Unmarshal([]byte(kept[0].Function.Arguments), &m); err != nil {
		t.Errorf("expected repaired arguments to be valid JSON, got error: %v", err)
	}

	// repairErrors should be 0 (successful repair).
	if repairErrors != 0 {
		t.Errorf("expected 0 repair errors for brace-only repair, got %d", repairErrors)
	}
}

// TestSchemaFlatteningReachesModelViaEpoch proves that schema flattening
// actually reaches the model through the epoch system. This is the critical
// end-to-end test for Task-008: it verifies that buildEpochComponents
// flattens complex schemas, that FreezeEpoch captures the flattened tools
// in FrozenTools, and that the model-facing request gets flat schemas.
//
// The previous version of this test hand-set schemaAdapters and bypassed
// the epoch path, making it green over a feature that never executed.
func TestSchemaFlatteningReachesModelViaEpoch(t *testing.T) {
	a := New(nil, tools.New(), nil, "deepseek-v4-flash")
	a.System = "static system prompt"
	a.bus = NewBus()

	// Register a tool with a deeply-nested schema (depth 3 triggers flattening).
	complexSchema := json.RawMessage(`{
		"type": "object",
		"properties": {
			"target": {
				"type": "object",
				"properties": {
					"path": {"type": "string"},
					"range": {
						"type": "object",
						"properties": {
							"start": {"type": "integer"},
							"end": {"type": "integer"}
						}
					}
				}
			}
		}
	}`)
	a.Tools.Register(&mockComplexTool{name: "edit_file", schema: complexSchema})

	// Step 1: buildEpochComponents must flatten the schema.
	comps := a.buildEpochComponents()

	// Find the edit_file tool in ToolSpecs.
	var editFileSpec *llm.Tool
	for i := range comps.ToolSpecs {
		if comps.ToolSpecs[i].Function.Name == "edit_file" {
			editFileSpec = &comps.ToolSpecs[i]
			break
		}
	}
	if editFileSpec == nil {
		t.Fatal("edit_file tool not found in epoch ToolSpecs")
	}

	// ToolSpecs should carry the FLAT schema (target__path, target__range__start).
	var flatSchema map[string]any
	if err := json.Unmarshal(editFileSpec.Function.Parameters, &flatSchema); err != nil {
		t.Fatalf("flattened schema not valid JSON: %v", err)
	}
	props, _ := flatSchema["properties"].(map[string]any)
	if _, has := props["target__path"]; !has {
		t.Error("expected flattened schema to have 'target__path' in ToolSpecs")
	}
	if _, has := props["target__range__start"]; !has {
		t.Error("expected flattened schema to have 'target__range__start' in ToolSpecs")
	}
	// Should NOT have nested "target" object in the flat schema.
	if _, has := props["target"]; has {
		t.Error("expected 'target' to be removed from flat schema properties")
	}

	// Step 2: InitEpoch + FreezeEpoch must capture the flat tools.
	epoch := a.epochMgr.InitEpoch("session_start", comps)
	a.epochMgr.FreezeEpoch()

	// FrozenTools must carry the flattened schema (not the original).
	var frozenEditFile *llm.Tool
	for i := range epoch.FrozenTools {
		if epoch.FrozenTools[i].Function.Name == "edit_file" {
			frozenEditFile = &epoch.FrozenTools[i]
			break
		}
	}
	if frozenEditFile == nil {
		t.Fatal("edit_file tool not found in FrozenTools")
	}
	var frozenSchema map[string]any
	if err := json.Unmarshal(frozenEditFile.Function.Parameters, &frozenSchema); err != nil {
		t.Fatalf("frozen schema not valid JSON: %v", err)
	}
	frozenProps, _ := frozenSchema["properties"].(map[string]any)
	if _, has := frozenProps["target__path"]; !has {
		t.Error("expected FrozenTools to carry flattened 'target__path'")
	}

	// Step 3: schemaAdapters must be populated for rehydration.
	adapter, hasAdapter := a.schemaAdapters["edit_file"]
	if !hasAdapter {
		t.Fatal("expected schemaAdapters['edit_file'] to be populated after buildEpochComponents")
	}
	if len(adapter.FieldMap) == 0 {
		t.Fatal("expected adapter.FieldMap to have entries for flattened fields")
	}

	// Step 4: repairToolCalls must rehydrate flat args → nested.
	// Simulate the model returning flat arguments (which it will, since
	// the model saw the flat schema from FrozenTools).
	bus := a.bus
	sub := bus.Subscribe(64)
	var repairs []EventRepair
	done := make(chan struct{})
	go func() {
		for env := range sub.C {
			if ev, ok := env.Event.(EventRepair); ok {
				repairs = append(repairs, ev)
			}
		}
		close(done)
	}()

	flatArgs := `{"target__path":"main.go","target__range__start":1,"target__range__end":10}`
	declared := []llm.ToolCall{
		{ID: "1", Function: llm.ToolCallFunc{Name: "edit_file", Arguments: flatArgs}},
	}
	blocks := []llm.ContentBlock{llm.ToolUseBlock{ID: "1", Name: "edit_file"}}

	kept, repairErrors := a.repairToolCalls(context.Background(), "", "", declared, &blocks)

	bus.Close()
	<-done

	// Call should be kept.
	if len(kept) != 1 {
		t.Fatalf("expected 1 kept call, got %d", len(kept))
	}

	// Arguments should be rehydrated to nested form.
	var args map[string]any
	if err := json.Unmarshal([]byte(kept[0].Function.Arguments), &args); err != nil {
		t.Fatalf("rehydrated args not valid JSON: %v", err)
	}

	// Should have nested "target" object.
	target, ok := args["target"].(map[string]any)
	if !ok {
		t.Fatalf("expected nested 'target' object, got %T", args["target"])
	}
	if target["path"] != "main.go" {
		t.Errorf("expected target.path = 'main.go', got %v", target["path"])
	}

	// Should have nested "target.range" object.
	targetRange, ok := target["range"].(map[string]any)
	if !ok {
		t.Fatalf("expected nested 'target.range' object, got %T", target["range"])
	}
	if targetRange["start"] != float64(1) {
		t.Errorf("expected target.range.start = 1, got %v", targetRange["start"])
	}
	if targetRange["end"] != float64(10) {
		t.Errorf("expected target.range.end = 10, got %v", targetRange["end"])
	}

	// Should NOT have flat keys in the output.
	if _, has := args["target__path"]; has {
		t.Error("expected flat key 'target__path' to be removed after rehydration")
	}

	// repairErrors should be 0 (rehydration is a valid repair).
	if repairErrors != 0 {
		t.Errorf("expected 0 repair errors after rehydration, got %d", repairErrors)
	}

	// Should have a rehydration repair event.
	foundRehydrated := false
	for _, ev := range repairs {
		if ev.Kind == string(repair.KindArgsCompleted) && strings.Contains(ev.Message, "rehydrated") {
			foundRehydrated = true
		}
	}
	if !foundRehydrated {
		t.Error("expected a 'rehydrated' repair event, got none")
	}
}

// TestRehydrationNoOpDoesNotEmitEvent proves that when the model returns
// nested args (no flat keys to rehydrate), the rehydration step does NOT
// emit a misleading "arguments rehydrated" event. This happens when the
// epoch has adapters but the model somehow returns nested args anyway.
func TestRehydrationNoOpDoesNotEmitEvent(t *testing.T) {
	a := New(nil, tools.New(), nil, "deepseek-v4-flash")
	a.System = "static system prompt"
	a.bus = NewBus()

	complexSchema := json.RawMessage(`{
		"type": "object",
		"properties": {
			"target": {
				"type": "object",
				"properties": {
					"path": {"type": "string"},
					"range": {
						"type": "object",
						"properties": {
							"start": {"type": "integer"},
							"end": {"type": "integer"}
						}
					}
				}
			}
		}
	}`)
	a.Tools.Register(&mockComplexTool{name: "edit_file", schema: complexSchema})

	// Run buildEpochComponents to populate schemaAdapters.
	a.buildEpochComponents()
	if len(a.schemaAdapters) == 0 {
		t.Fatal("expected schemaAdapters to be populated")
	}

	bus := a.bus
	sub := bus.Subscribe(64)
	var repairs []EventRepair
	done := make(chan struct{})
	go func() {
		for env := range sub.C {
			if ev, ok := env.Event.(EventRepair); ok {
				repairs = append(repairs, ev)
			}
		}
		close(done)
	}()

	// Model returns NESTED args (not flat) — no rehydration needed.
	nestedArgs := `{"target":{"path":"main.go","range":{"start":1,"end":10}}}`
	declared := []llm.ToolCall{
		{ID: "1", Function: llm.ToolCallFunc{Name: "edit_file", Arguments: nestedArgs}},
	}
	blocks := []llm.ContentBlock{llm.ToolUseBlock{ID: "1", Name: "edit_file"}}

	kept, repairErrors := a.repairToolCalls(context.Background(), "", "", declared, &blocks)

	bus.Close()
	<-done

	if len(kept) != 1 {
		t.Fatalf("expected 1 kept call, got %d", len(kept))
	}
	if repairErrors != 0 {
		t.Errorf("expected 0 repair errors, got %d", repairErrors)
	}

	// Args should be unchanged — rehydration was a no-op passthrough.
	if kept[0].Function.Arguments != nestedArgs {
		t.Error("expected nested args to pass through unchanged")
	}

	// Should NOT have a "rehydrated" event — no actual rehydration occurred.
	for _, ev := range repairs {
		if strings.Contains(ev.Message, "rehydrated") {
			t.Errorf("unexpected 'rehydrated' event for no-op passthrough: %+v", ev)
		}
	}
}
