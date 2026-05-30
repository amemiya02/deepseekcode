package repair

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/amemiya02/deepseekcode/internal/llm"
)

// SchemaAnalysis holds the results of schema complexity analysis.
type SchemaAnalysis struct {
	MaxDepth             int
	LeafCount            int
	HasNestedArrayObject bool
	ShouldAdapt          bool
}

// AnalyzeToolSchemas inspects a slice of llm.Tool schemas and returns
// one Report per tool whose schema exceeds maxDepth, maxLeaves, or has
// nested array/object shape. Reports are sorted by tool name for
// deterministic order.
func AnalyzeToolSchemas(tools []llm.Tool, maxDepth int, maxLeaves int) []Report {
	var reports []Report
	for _, tool := range tools {
		schema := tool.Function.Parameters
		if len(schema) == 0 {
			continue
		}
		analysis, err := AnalyzeSchema(schema)
		if err != nil {
			continue
		}
		if analysis.MaxDepth > maxDepth || analysis.LeafCount > maxLeaves || analysis.HasNestedArrayObject {
			msg := fmt.Sprintf("schema depth %d leaves %d nested_array_object=%v (limits: depth %d leaves %d)",
				analysis.MaxDepth, analysis.LeafCount, analysis.HasNestedArrayObject, maxDepth, maxLeaves)
			reports = append(reports, Report{
				Kind:    KindSchemaComplex,
				Tool:    tool.Function.Name,
				Message: msg,
			})
		}
	}
	// Sort by tool name for deterministic order.
	sort.Slice(reports, func(i, j int) bool {
		return reports[i].Tool < reports[j].Tool
	})
	return reports
}

// AnalyzeSchema analyzes a JSON schema for complexity indicators.
// It returns the analysis results or an error for invalid JSON.
func AnalyzeSchema(schema json.RawMessage) (SchemaAnalysis, error) {
	var analysis SchemaAnalysis

	// Parse schema into generic structure
	var parsed any
	if err := json.Unmarshal(schema, &parsed); err != nil {
		return analysis, fmt.Errorf("invalid JSON: %w", err)
	}

	// Handle empty or null schema
	if parsed == nil {
		return analysis, nil
	}

	// Walk the schema
	walkSchema(parsed, 1, &analysis)

	// Determine if adaptation is needed
	analysis.ShouldAdapt = analysis.MaxDepth > 2 ||
		analysis.LeafCount > 10 ||
		analysis.HasNestedArrayObject

	return analysis, nil
}

// walkSchema recursively walks a JSON schema, tracking depth and counting leaves.
func walkSchema(node any, depth int, analysis *SchemaAnalysis) {
	if node == nil {
		return
	}

	switch v := node.(type) {
	case map[string]any:
		schemaType, hasType := v["type"]
		properties, hasProperties := v["properties"]
		items, hasItems := v["items"]

		// Handle object with properties - only count depth for actual object schemas
		if hasType && schemaType == "object" && hasProperties {
			// Update max depth for this object level
			if depth > analysis.MaxDepth {
				analysis.MaxDepth = depth
			}

			props, ok := properties.(map[string]any)
			if ok {
				for _, prop := range props {
					// Check if this property is a leaf (scalar type)
					if isLeafType(prop) {
						analysis.LeafCount++
					} else {
						// Recurse into nested object
						walkSchema(prop, depth+1, analysis)
					}
				}
			}
		}

		// Handle array items
		if hasType && schemaType == "array" && hasItems {
			itemsSchema, ok := items.(map[string]any)
			if ok {
				// Check if array item is an object with nested fields
				if isArrayItemNestedObject(itemsSchema) {
					analysis.HasNestedArrayObject = true
				}
				// Recurse into array items
				walkSchema(items, depth+1, analysis)
			}
		}

		// Handle oneOf, anyOf, allOf
		for _, key := range []string{"oneOf", "anyOf", "allOf"} {
			if variants, ok := v[key].([]any); ok {
				for _, variant := range variants {
					walkSchema(variant, depth, analysis)
				}
			}
		}

	case []any:
		// Array at top level (rare in schemas, but handle it)
		for _, item := range v {
			walkSchema(item, depth, analysis)
		}
	}
}

// isLeafType checks if a schema node represents a scalar/leaf type.
func isLeafType(node any) bool {
	m, ok := node.(map[string]any)
	if !ok {
		return false
	}

	t, hasType := m["type"]
	if !hasType {
		return false
	}

	// Scalar types
	switch t {
	case "string", "number", "integer", "boolean", "null":
		return true
	case "array":
		// Array of scalars is considered a leaf
		if items, ok := m["items"].(map[string]any); ok {
			itemType, hasItemType := items["type"]
			if hasItemType {
				switch itemType {
				case "string", "number", "integer", "boolean", "null":
					return true
				}
			}
		}
	}

	return false
}

// isArrayItemNestedObject checks if an array item schema contains nested objects or arrays.
func isArrayItemNestedObject(itemsSchema map[string]any) bool {
	t, hasType := itemsSchema["type"]
	if !hasType || t != "object" {
		return false
	}

	properties, hasProperties := itemsSchema["properties"]
	if !hasProperties {
		return false
	}

	props, ok := properties.(map[string]any)
	if !ok {
		return false
	}

	// Check each property for nested object or array
	for _, prop := range props {
		propSchema, ok := prop.(map[string]any)
		if !ok {
			continue
		}

		propType, hasPropType := propSchema["type"]
		if !hasPropType {
			continue
		}

		// Nested object
		if propType == "object" {
			return true
		}

		// Nested array
		if propType == "array" {
			return true
		}
	}

	return false
}
