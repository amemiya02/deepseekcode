package repair

import (
	"encoding/json"
	"testing"

	"github.com/amemiya02/deepseekcode/internal/llm"
)

func TestBuildSchemaAdapters_ShallowNoOp(t *testing.T) {
	// Shallow schemas should pass through unchanged
	tools := []llm.Tool{
		{
			Type: "function",
			Function: llm.ToolFunction{
				Name:        "read_file",
				Description: "Read a file",
				Parameters:  json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}},"required":["path"]}`),
			},
		},
	}

	result, adapters, err := BuildSchemaAdapters(tools)
	if err != nil {
		t.Fatalf("BuildSchemaAdapters failed: %v", err)
	}

	if len(result) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(result))
	}

	// Tool should be unchanged
	if string(result[0].Function.Parameters) != string(tools[0].Function.Parameters) {
		t.Errorf("shallow schema should not be modified")
	}

	// No adapters should be created
	if len(adapters) != 0 {
		t.Errorf("expected no adapters for shallow schema, got %d", len(adapters))
	}
}

func TestBuildSchemaAdapters_DeepObjectFlattened(t *testing.T) {
	// Nested object should be flattened
	schema := `{
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
		},
		"required": ["target"]
	}`

	tools := []llm.Tool{
		{
			Type: "function",
			Function: llm.ToolFunction{
				Name:        "edit_file",
				Description: "Edit a file",
				Parameters:  json.RawMessage(schema),
			},
		},
	}

	result, adapters, err := BuildSchemaAdapters(tools)
	if err != nil {
		t.Fatalf("BuildSchemaAdapters failed: %v", err)
	}

	if len(result) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(result))
	}

	// Should have created an adapter
	adapter, exists := adapters["edit_file"]
	if !exists {
		t.Fatal("expected adapter for edit_file")
	}

	// Verify field mappings
	if len(adapter.FieldMap) == 0 {
		t.Fatal("expected field mappings")
	}

	// Check for flattened field names
	expectedFields := map[string]bool{
		"target__path":        false,
		"target__range__start": false,
		"target__range__end":   false,
	}

	for _, fm := range adapter.FieldMap {
		if _, exists := expectedFields[fm.FlatName]; exists {
			expectedFields[fm.FlatName] = true
		}
	}

	for field, found := range expectedFields {
		if !found {
			t.Errorf("expected field %s not found in mappings", field)
		}
	}
}

func TestRehydrate_FlattenRoundTrip(t *testing.T) {
	// Test that flattening and rehydrating returns original structure
	// Schema needs depth > 2 to trigger flattening
	schema := `{
		"type": "object",
		"properties": {
			"config": {
				"type": "object",
				"properties": {
					"target": {
						"type": "object",
						"properties": {
							"path": {"type": "string"},
							"line": {"type": "integer"}
						}
					}
				}
			}
		},
		"required": ["config"]
	}`

	tools := []llm.Tool{
		{
			Type: "function",
			Function: llm.ToolFunction{
				Name:        "edit",
				Description: "Edit",
				Parameters:  json.RawMessage(schema),
			},
		},
	}

	_, adapters, err := BuildSchemaAdapters(tools)
	if err != nil {
		t.Fatalf("BuildSchemaAdapters failed: %v", err)
	}

	adapter, exists := adapters["edit"]
	if !exists {
		t.Fatal("expected adapter")
	}

	// Flat arguments from model
	flatArgs := json.RawMessage(`{"config__target__path":"a.go","config__target__line":42}`)

	// Rehydrate
	rehydrated, err := adapter.Rehydrate(flatArgs)
	if err != nil {
		t.Fatalf("Rehydrate failed: %v", err)
	}

	// Should match original nested structure
	var result map[string]any
	if err := json.Unmarshal(rehydrated, &result); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	// Check nested structure
	config, ok := result["config"].(map[string]any)
	if !ok {
		t.Fatal("expected nested config object")
	}

	target, ok := config["target"].(map[string]any)
	if !ok {
		t.Fatal("expected nested target object")
	}

	if target["path"] != "a.go" {
		t.Errorf("expected path='a.go', got %v", target["path"])
	}

	if target["line"] != float64(42) {
		t.Errorf("expected line=42, got %v", target["line"])
	}
}

func TestRehydrate_NoAdapterPassthrough(t *testing.T) {
	// Empty field map should pass through unchanged
	adapter := SchemaAdapter{
		PublicName:   "test",
		OriginalName: "test",
		FieldMap:     nil,
	}

	args := json.RawMessage(`{"foo":"bar"}`)
	result, err := adapter.Rehydrate(args)
	if err != nil {
		t.Fatalf("Rehydrate failed: %v", err)
	}

	if string(result) != string(args) {
		t.Errorf("expected passthrough, got %s", string(result))
	}
}

func TestBuildSchemaAdapters_ArrayOfObjectsNotFlattened(t *testing.T) {
	// Arrays of objects should not be flattened
	schema := `{
		"type": "object",
		"properties": {
			"items": {
				"type": "array",
				"items": {
					"type": "object",
					"properties": {
						"name": {"type": "string"},
						"value": {"type": "integer"}
					}
				}
			}
		}
	}`

	tools := []llm.Tool{
		{
			Type: "function",
			Function: llm.ToolFunction{
				Name:        "batch_edit",
				Description: "Batch edit",
				Parameters:  json.RawMessage(schema),
			},
		},
	}

	result, adapters, err := BuildSchemaAdapters(tools)
	if err != nil {
		t.Fatalf("BuildSchemaAdapters failed: %v", err)
	}

	// Schema should pass through unchanged
	if string(result[0].Function.Parameters) != string(tools[0].Function.Parameters) {
		t.Errorf("array-of-objects schema should not be modified")
	}

	// No adapter should be created
	if len(adapters) != 0 {
		t.Errorf("expected no adapter for array-of-objects, got %d", len(adapters))
	}
}

func TestBuildSchemaAdapters_CollisionSkipsFlatten(t *testing.T) {
	// Name collisions should prevent flattening
	// This would create a collision: both a__b and a__b from different paths
	schema := `{
		"type": "object",
		"properties": {
			"a__b": {"type": "string"},
			"a": {
				"type": "object",
				"properties": {
					"b": {"type": "integer"}
				}
			}
		}
	}`

	tools := []llm.Tool{
		{
			Type: "function",
			Function: llm.ToolFunction{
				Name:        "collision_test",
				Description: "Test",
				Parameters:  json.RawMessage(schema),
			},
		},
	}

	result, adapters, err := BuildSchemaAdapters(tools)
	if err != nil {
		t.Fatalf("BuildSchemaAdapters failed: %v", err)
	}

	// Should pass through unchanged due to collision
	if len(adapters) != 0 {
		t.Errorf("expected no adapter when collision detected, got %d", len(adapters))
	}

	if string(result[0].Function.Parameters) != string(tools[0].Function.Parameters) {
		t.Error("schema with collision should not be modified")
	}
}

func TestBuildSchemaAdapters_ByteStable(t *testing.T) {
	// Running twice should produce identical output
	schema := `{
		"type": "object",
		"properties": {
			"config": {
				"type": "object",
				"properties": {
					"enabled": {"type": "boolean"},
					"level": {"type": "integer"}
				}
			}
		}
	}`

	tools := []llm.Tool{
		{
			Type: "function",
			Function: llm.ToolFunction{
				Name:        "configure",
				Description: "Configure",
				Parameters:  json.RawMessage(schema),
			},
		},
	}

	// First run
	result1, adapters1, err := BuildSchemaAdapters(tools)
	if err != nil {
		t.Fatalf("first BuildSchemaAdapters failed: %v", err)
	}

	// Second run with same input
	result2, adapters2, err := BuildSchemaAdapters(tools)
	if err != nil {
		t.Fatalf("second BuildSchemaAdapters failed: %v", err)
	}

	// Compare schemas
	if string(result1[0].Function.Parameters) != string(result2[0].Function.Parameters) {
		t.Errorf("byte instability detected: schemas differ between runs")
	}

	// Compare adapters (field map order should be deterministic)
	if len(adapters1) != len(adapters2) {
		t.Errorf("adapter count differs: %d vs %d", len(adapters1), len(adapters2))
	}

	adapter1 := adapters1["configure"]
	adapter2 := adapters2["configure"]

	if len(adapter1.FieldMap) != len(adapter2.FieldMap) {
		t.Errorf("field map count differs: %d vs %d", len(adapter1.FieldMap), len(adapter2.FieldMap))
	}
}

func TestRehydrate_DeeplyNested(t *testing.T) {
	// Test 3+ levels of nesting
	schema := `{
		"type": "object",
		"properties": {
			"level1": {
				"type": "object",
				"properties": {
					"level2": {
						"type": "object",
						"properties": {
							"level3": {
								"type": "object",
								"properties": {
									"value": {"type": "string"}
								}
							}
						}
					}
				}
			}
		}
	}`

	tools := []llm.Tool{
		{
			Type: "function",
			Function: llm.ToolFunction{
				Name:        "deep_nest",
				Description: "Deep",
				Parameters:  json.RawMessage(schema),
			},
		},
	}

	_, adapters, err := BuildSchemaAdapters(tools)
	if err != nil {
		t.Fatalf("BuildSchemaAdapters failed: %v", err)
	}

	adapter, exists := adapters["deep_nest"]
	if !exists {
		t.Fatal("expected adapter for deeply nested schema")
	}

	// Verify the flattened field name
	found := false
	for _, fm := range adapter.FieldMap {
		if fm.FlatName == "level1__level2__level3__value" {
			found = true
			// Verify nested path
			expected := []string{"level1", "level2", "level3", "value"}
			if len(fm.NestedPath) != len(expected) {
				t.Errorf("path length mismatch: %d vs %d", len(fm.NestedPath), len(expected))
			} else {
				for i, p := range expected {
					if fm.NestedPath[i] != p {
						t.Errorf("path element %d: expected %s, got %s", i, p, fm.NestedPath[i])
					}
				}
			}
			break
		}
	}

	if !found {
		t.Error("expected field mapping for level1__level2__level3__value")
	}

	// Test rehydration
	flatArgs := json.RawMessage(`{"level1__level2__level3__value":"test"}`)
	rehydrated, err := adapter.Rehydrate(flatArgs)
	if err != nil {
		t.Fatalf("Rehydrate failed: %v", err)
	}

	var result map[string]any
	json.Unmarshal(rehydrated, &result)

	// Navigate to deeply nested value
	l1 := result["level1"].(map[string]any)
	l2 := l1["level2"].(map[string]any)
	l3 := l2["level3"].(map[string]any)
	val := l3["value"].(string)

	if val != "test" {
		t.Errorf("expected value='test', got %v", val)
	}
}

func TestBuildSchemaAdapters_OneOfAnyOfNotFlattened(t *testing.T) {
	// oneOf/anyOf schemas should not be flattened
	tests := []struct {
		name   string
		schema string
	}{
		{
			name: "oneOf at top level",
			schema: `{
				"type": "object",
				"oneOf": [
					{"properties": {"a": {"type": "string"}}},
					{"properties": {"b": {"type": "integer"}}}
				]
			}`,
		},
		{
			name: "anyOf at top level",
			schema: `{
				"type": "object",
				"anyOf": [
					{"properties": {"a": {"type": "string"}}},
					{"properties": {"b": {"type": "integer"}}}
				]
			}`,
		},
		{
			name: "oneOf in nested object",
			schema: `{
				"type": "object",
				"properties": {
					"field": {
						"type": "object",
						"oneOf": [
							{"properties": {"x": {"type": "string"}}}
						]
					}
				}
			}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tools := []llm.Tool{
				{
					Type: "function",
					Function: llm.ToolFunction{
						Name:        "test_tool",
						Description: "Test",
						Parameters:  json.RawMessage(tt.schema),
					},
				},
			}

			result, adapters, err := BuildSchemaAdapters(tools)
			if err != nil {
				t.Fatalf("BuildSchemaAdapters failed: %v", err)
			}

			// Schema should pass through unchanged
			if string(result[0].Function.Parameters) != string(tools[0].Function.Parameters) {
				t.Errorf("oneOf/anyOf schema should not be modified")
			}

			// No adapter should be created
			if len(adapters) != 0 {
				t.Errorf("expected no adapter for oneOf/anyOf schema")
			}
		})
	}
}

func TestRehydrate_MixedFields(t *testing.T) {
	// Test rehydration with both flattened and non-flattened fields
	// Schema needs depth > 2 to trigger flattening
	schema := `{
		"type": "object",
		"properties": {
			"simple": {"type": "string"},
			"config": {
				"type": "object",
				"properties": {
					"nested": {
						"type": "object",
						"properties": {
							"value": {"type": "integer"}
						}
					}
				}
			}
		}
	}`

	tools := []llm.Tool{
		{
			Type: "function",
			Function: llm.ToolFunction{
				Name:        "mixed",
				Description: "Mixed",
				Parameters:  json.RawMessage(schema),
			},
		},
	}

	_, adapters, err := BuildSchemaAdapters(tools)
	if err != nil {
		t.Fatalf("BuildSchemaAdapters failed: %v", err)
	}

	adapter, exists := adapters["mixed"]
	if !exists {
		t.Fatal("expected adapter")
	}

	// Flat args with both simple and flattened fields
	flatArgs := json.RawMessage(`{"simple":"test","config__nested__value":42}`)

	rehydrated, err := adapter.Rehydrate(flatArgs)
	if err != nil {
		t.Fatalf("Rehydrate failed: %v", err)
	}

	var result map[string]any
	json.Unmarshal(rehydrated, &result)

	// Check simple field
	if result["simple"] != "test" {
		t.Errorf("expected simple='test', got %v", result["simple"])
	}

	// Check nested field
	config := result["config"].(map[string]any)
	nested := config["nested"].(map[string]any)
	if nested["value"] != float64(42) {
		t.Errorf("expected config.nested.value=42, got %v", nested["value"])
	}
}
