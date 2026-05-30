package repair

import (
	"encoding/json"
	"testing"

	"github.com/amemiya02/deepseekcode/internal/llm"
)

func TestAnalyzeSchema_ShallowObject(t *testing.T) {
	schema := json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}}}`)

	analysis, err := AnalyzeSchema(schema)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if analysis.MaxDepth != 1 {
		t.Errorf("expected MaxDepth=1, got %d", analysis.MaxDepth)
	}
	if analysis.LeafCount != 1 {
		t.Errorf("expected LeafCount=1, got %d", analysis.LeafCount)
	}
	if analysis.HasNestedArrayObject {
		t.Error("expected HasNestedArrayObject=false")
	}
	if analysis.ShouldAdapt {
		t.Error("expected ShouldAdapt=false for shallow schema")
	}
}

func TestAnalyzeSchema_Depth3(t *testing.T) {
	schema := json.RawMessage(`{
		"type": "object",
		"properties": {
			"outer": {
				"type": "object",
				"properties": {
					"middle": {
						"type": "object",
						"properties": {
							"inner": {"type": "string"}
						}
					}
				}
			}
		}
	}`)

	analysis, err := AnalyzeSchema(schema)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if analysis.MaxDepth != 3 {
		t.Errorf("expected MaxDepth=3, got %d", analysis.MaxDepth)
	}
	if !analysis.ShouldAdapt {
		t.Error("expected ShouldAdapt=true for depth > 2")
	}
}

func TestAnalyzeSchema_ManyLeaves(t *testing.T) {
	// Schema with 11 leaf properties
	schema := json.RawMessage(`{
		"type": "object",
		"properties": {
			"p1": {"type": "string"},
			"p2": {"type": "string"},
			"p3": {"type": "string"},
			"p4": {"type": "string"},
			"p5": {"type": "string"},
			"p6": {"type": "string"},
			"p7": {"type": "string"},
			"p8": {"type": "string"},
			"p9": {"type": "string"},
			"p10": {"type": "string"},
			"p11": {"type": "string"}
		}
	}`)

	analysis, err := AnalyzeSchema(schema)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if analysis.LeafCount != 11 {
		t.Errorf("expected LeafCount=11, got %d", analysis.LeafCount)
	}
	if !analysis.ShouldAdapt {
		t.Error("expected ShouldAdapt=true for leaf count > 10")
	}
}

func TestAnalyzeSchema_InvalidJSON(t *testing.T) {
	schema := json.RawMessage(`{invalid}`)

	_, err := AnalyzeSchema(schema)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestAnalyzeSchema_EmptyObject(t *testing.T) {
	schema := json.RawMessage(`{}`)

	analysis, err := AnalyzeSchema(schema)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if analysis.MaxDepth != 0 {
		t.Errorf("expected MaxDepth=0 for empty schema, got %d", analysis.MaxDepth)
	}
	if analysis.LeafCount != 0 {
		t.Errorf("expected LeafCount=0, got %d", analysis.LeafCount)
	}
	if analysis.ShouldAdapt {
		t.Error("expected ShouldAdapt=false for empty schema")
	}
}

func TestAnalyzeSchema_NullSchema(t *testing.T) {
	schema := json.RawMessage(`null`)

	analysis, err := AnalyzeSchema(schema)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if analysis.ShouldAdapt {
		t.Error("expected ShouldAdapt=false for null schema")
	}
}

func TestAnalyzeSchema_ArrayOfScalars(t *testing.T) {
	schema := json.RawMessage(`{
		"type": "object",
		"properties": {
			"tags": {
				"type": "array",
				"items": {"type": "string"}
			}
		}
	}`)

	analysis, err := AnalyzeSchema(schema)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if analysis.HasNestedArrayObject {
		t.Error("expected HasNestedArrayObject=false for array of scalars")
	}
	if analysis.ShouldAdapt {
		t.Error("expected ShouldAdapt=false for array of scalars")
	}
}

func TestAnalyzeSchema_NestedArrayObject(t *testing.T) {
	// question-style array of object with nested options
	schema := json.RawMessage(`{
		"type": "object",
		"properties": {
			"questions": {
				"type": "array",
				"items": {
					"type": "object",
					"properties": {
						"question": {"type": "string"},
						"options": {
							"type": "array",
							"items": {"type": "string"}
						}
					}
				}
			}
		}
	}`)

	analysis, err := AnalyzeSchema(schema)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !analysis.HasNestedArrayObject {
		t.Error("expected HasNestedArrayObject=true for nested array in object")
	}
	if !analysis.ShouldAdapt {
		t.Error("expected ShouldAdapt=true for nested array-object")
	}
}

func TestAnalyzeSchema_OneOf(t *testing.T) {
	schema := json.RawMessage(`{
		"type": "object",
		"properties": {
			"value": {
				"oneOf": [
					{"type": "string"},
					{"type": "number"}
				]
			}
		}
	}`)

	analysis, err := AnalyzeSchema(schema)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// oneOf with scalar types should not trigger adaptation
	if analysis.ShouldAdapt {
		t.Error("expected ShouldAdapt=false for oneOf with scalars")
	}
}

func TestAnalyzeSchema_AnyOf(t *testing.T) {
	schema := json.RawMessage(`{
		"type": "object",
		"properties": {
			"value": {
				"anyOf": [
					{"type": "string"},
					{"type": "null"}
				]
			}
		}
	}`)

	analysis, err := AnalyzeSchema(schema)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if analysis.ShouldAdapt {
		t.Error("expected ShouldAdapt=false for anyOf with scalars")
	}
}

func TestAnalyzeSchema_AllOf(t *testing.T) {
	schema := json.RawMessage(`{
		"type": "object",
		"properties": {
			"config": {
				"allOf": [
					{
						"type": "object",
						"properties": {
							"nested": {"type": "string"}
						}
					}
				]
			}
		}
	}`)

	analysis, err := AnalyzeSchema(schema)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// allOf with nested object should track depth
	if analysis.MaxDepth < 2 {
		t.Errorf("expected MaxDepth>=2, got %d", analysis.MaxDepth)
	}
}

func TestAnalyzeSchema_MixedTypes(t *testing.T) {
	schema := json.RawMessage(`{
		"type": "object",
		"properties": {
			"name": {"type": "string"},
			"count": {"type": "integer"},
			"enabled": {"type": "boolean"},
			"ratio": {"type": "number"}
		}
	}`)

	analysis, err := AnalyzeSchema(schema)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if analysis.LeafCount != 4 {
		t.Errorf("expected LeafCount=4, got %d", analysis.LeafCount)
	}
}

func TestAnalyzeSchema_NestedObjectInArrayItem(t *testing.T) {
	// Array item with nested object property
	schema := json.RawMessage(`{
		"type": "object",
		"properties": {
			"items": {
				"type": "array",
				"items": {
					"type": "object",
					"properties": {
						"metadata": {
							"type": "object",
							"properties": {
								"key": {"type": "string"}
							}
						}
					}
				}
			}
		}
	}`)

	analysis, err := AnalyzeSchema(schema)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !analysis.HasNestedArrayObject {
		t.Error("expected HasNestedArrayObject=true for object with nested object in array item")
	}
}

func TestAnalyzeSchema_ExactlyTenLeaves(t *testing.T) {
	// Schema with exactly 10 leaves - should NOT adapt
	schema := json.RawMessage(`{
		"type": "object",
		"properties": {
			"p1": {"type": "string"},
			"p2": {"type": "string"},
			"p3": {"type": "string"},
			"p4": {"type": "string"},
			"p5": {"type": "string"},
			"p6": {"type": "string"},
			"p7": {"type": "string"},
			"p8": {"type": "string"},
			"p9": {"type": "string"},
			"p10": {"type": "string"}
		}
	}`)

	analysis, err := AnalyzeSchema(schema)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if analysis.LeafCount != 10 {
		t.Errorf("expected LeafCount=10, got %d", analysis.LeafCount)
	}
	if analysis.ShouldAdapt {
		t.Error("expected ShouldAdapt=false for exactly 10 leaves")
	}
}

func TestAnalyzeSchema_Depth2(t *testing.T) {
	// Depth 2 should NOT adapt (threshold is > 2)
	schema := json.RawMessage(`{
		"type": "object",
		"properties": {
			"outer": {
				"type": "object",
				"properties": {
					"inner": {"type": "string"}
				}
			}
		}
	}`)

	analysis, err := AnalyzeSchema(schema)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if analysis.MaxDepth != 2 {
		t.Errorf("expected MaxDepth=2, got %d", analysis.MaxDepth)
	}
	if analysis.ShouldAdapt {
		t.Error("expected ShouldAdapt=false for depth=2 (threshold is >2)")
	}
}

func TestAnalyzeSchema_ReadFileStyle(t *testing.T) {
	// Typical read_file-style schema
	schema := json.RawMessage(`{
		"type": "object",
		"properties": {
			"file_path": {"type": "string"},
			"offset": {"type": "integer"},
			"limit": {"type": "integer"}
		},
		"required": ["file_path"]
	}`)

	analysis, err := AnalyzeSchema(schema)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if analysis.ShouldAdapt {
		t.Error("expected ShouldAdapt=false for read_file-style shallow schema")
	}
}

func TestAnalyzeToolSchemas(t *testing.T) {
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
	simpleSchema := json.RawMessage(`{"type": "object", "properties": {"path": {"type": "string"}}}`)

	tools := []llm.Tool{
		{Function: llm.ToolFunction{Name: "simple", Parameters: simpleSchema}},
		{Function: llm.ToolFunction{Name: "complex", Parameters: complexSchema}},
	}

	reports := AnalyzeToolSchemas(tools, 4, 80)
	if len(reports) != 1 {
		t.Fatalf("got %d reports, want 1", len(reports))
	}
	if reports[0].Tool != "complex" {
		t.Errorf("report tool = %q, want complex", reports[0].Tool)
	}
	if reports[0].Kind != KindSchemaComplex {
		t.Errorf("report kind = %q, want %q", reports[0].Kind, KindSchemaComplex)
	}
}

func TestAnalyzeToolSchemasEmpty(t *testing.T) {
	reports := AnalyzeToolSchemas(nil, 4, 80)
	if len(reports) != 0 {
		t.Errorf("got %d reports, want 0", len(reports))
	}
}

func TestAnalyzeToolSchemasSorted(t *testing.T) {
	schema := json.RawMessage(`{
		"type": "object",
		"properties": {
			"questions": {
				"type": "array",
				"items": {
					"type": "object",
					"properties": {"q": {"type": "string"}, "opts": {"type": "array", "items": {"type": "string"}}}
				}
			}
		}
	}`)

	tools := []llm.Tool{
		{Function: llm.ToolFunction{Name: "z_tool", Parameters: schema}},
		{Function: llm.ToolFunction{Name: "a_tool", Parameters: schema}},
	}

	reports := AnalyzeToolSchemas(tools, 4, 80)
	if len(reports) != 2 {
		t.Fatalf("got %d reports, want 2", len(reports))
	}
	if reports[0].Tool != "a_tool" || reports[1].Tool != "z_tool" {
		t.Errorf("reports not sorted by tool name: %s, %s", reports[0].Tool, reports[1].Tool)
	}
}
