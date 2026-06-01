package repair

import (
	"encoding/json"
	"fmt"

	"github.com/amemiya02/deepseekcode/internal/llm"
)

// SchemaAdapter maps flattened tool parameters back to their original nested structure.
type SchemaAdapter struct {
	PublicName   string        // model-visible flattened tool name (same as original unless collision)
	OriginalName string        // original tool name
	FieldMap     []FieldMapping // maps flat names back to nested paths
}

// FieldMapping represents a single field's mapping from flat to nested structure.
type FieldMapping struct {
	FlatName   string   // e.g. "target__path"
	NestedPath []string // e.g. ["target", "path"]
	Required   bool
}

// BuildSchemaAdapters processes tool schemas and returns adapted versions for complex schemas.
// It returns the (possibly modified) tool list and a map from original name→SchemaAdapter.
// Schemas that exceed depth>2 or have nested-array-objects are flattened using "__" separator.
// Schemas that can't be safely flattened (arrays of objects, oneOf/anyOf) pass through unchanged.
func BuildSchemaAdapters(tools []llm.Tool) ([]llm.Tool, map[string]SchemaAdapter, error) {
	result := make([]llm.Tool, len(tools))
	adapters := make(map[string]SchemaAdapter)

	for i, tool := range tools {
		schema := tool.Function.Parameters
		if len(schema) == 0 {
			result[i] = tool
			continue
		}

		// Analyze schema complexity
		analysis, err := AnalyzeSchema(schema)
		if err != nil {
			result[i] = tool
			continue
		}

		// Only flatten if schema exceeds complexity thresholds
		if !analysis.ShouldAdapt {
			result[i] = tool
			continue
		}

		// Try to flatten the schema
		flattenedSchema, fieldMap, err := flattenSchema(schema)
		if err != nil || len(fieldMap) == 0 {
			// Can't flatten safely, pass through unchanged
			result[i] = tool
			continue
		}

		// Check for field name collisions
		if hasCollisions(fieldMap) {
			result[i] = tool
			continue
		}

		// Successfully flattened - create adapted tool
		result[i] = llm.Tool{
			Type: tool.Type,
			Function: llm.ToolFunction{
				Name:        tool.Function.Name,
				Description: tool.Function.Description,
				Parameters:  flattenedSchema,
			},
		}

		adapters[tool.Function.Name] = SchemaAdapter{
			PublicName:   tool.Function.Name,
			OriginalName: tool.Function.Name,
			FieldMap:     fieldMap,
		}
	}

	return result, adapters, nil
}

// flattenSchema attempts to flatten a nested JSON schema.
// Returns the flattened schema and field mappings, or an error if flattening is not possible.
func flattenSchema(schema json.RawMessage) (json.RawMessage, []FieldMapping, error) {
	var parsed map[string]any
	if err := json.Unmarshal(schema, &parsed); err != nil {
		return nil, nil, fmt.Errorf("invalid JSON: %w", err)
	}

	// Check for unsupported constructs
	if hasUnsupportedConstructs(parsed) {
		return nil, nil, fmt.Errorf("schema contains unsupported constructs")
	}

	// Extract required fields
	required := extractRequired(parsed)

	// Flatten properties
	props, ok := parsed["properties"].(map[string]any)
	if !ok {
		return nil, nil, fmt.Errorf("no properties found")
	}

	var fieldMap []FieldMapping
	flattenedProps := make(map[string]any)
	flattenedRequired := make([]string, 0)

	// Process each property
	for propName, propSchema := range props {
		propMap, ok := propSchema.(map[string]any)
		if !ok {
			// Non-object property, keep as-is
			flattenedProps[propName] = propSchema
			if isRequired(required, propName) {
				flattenedRequired = append(flattenedRequired, propName)
			}
			continue
		}

		// Check if this is a nested object that can be flattened
		if isFlattenableObject(propMap) {
			nestedProps, fieldMappings := flattenObjectProperty([]string{propName}, propName, propMap, isRequired(required, propName))
			for flatName, flatSchema := range nestedProps {
				flattenedProps[flatName] = flatSchema
			}
			fieldMap = append(fieldMap, fieldMappings...)
		} else {
			// Not flattenable, keep as-is
			flattenedProps[propName] = propSchema
			if isRequired(required, propName) {
				flattenedRequired = append(flattenedRequired, propName)
			}
		}
	}

	// Build flattened schema
	result := make(map[string]any)
	result["type"] = "object"
	result["properties"] = flattenedProps
	if len(flattenedRequired) > 0 {
		result["required"] = flattenedRequired
	}

	output, err := json.Marshal(result)
	if err != nil {
		return nil, nil, err
	}

	return output, fieldMap, nil
}

// flattenObjectProperty recursively flattens a nested object property.
// pathPrefix tracks the actual nested path (e.g., ["target", "range"])
// flatPrefix is the flattened name prefix (e.g., "target__range")
func flattenObjectProperty(pathPrefix []string, flatPrefix string, propMap map[string]any, isParentRequired bool) (map[string]any, []FieldMapping) {
	props, ok := propMap["properties"].(map[string]any)
	if !ok {
		return nil, nil
	}

	// Get required fields for this object
	required := extractRequired(propMap)

	result := make(map[string]any)
	var mappings []FieldMapping

	for childName, childSchema := range props {
		flatName := flatPrefix + "__" + childName
		childMap, ok := childSchema.(map[string]any)

		if ok && isFlattenableObject(childMap) {
			// Recurse into deeper nesting
			newPathPrefix := append(pathPrefix, childName)
			nested, nestedMappings := flattenObjectProperty(newPathPrefix, flatName, childMap, isParentRequired && isRequired(required, childName))
			for k, v := range nested {
				result[k] = v
			}
			mappings = append(mappings, nestedMappings...)
		} else {
			// Leaf or non-flattenable, add directly
			result[flatName] = childSchema
			nestedPath := append([]string{}, pathPrefix...)
			nestedPath = append(nestedPath, childName)
			mappings = append(mappings, FieldMapping{
				FlatName:   flatName,
				NestedPath: nestedPath,
				Required:   isParentRequired && isRequired(required, childName),
			})
		}
	}

	return result, mappings
}

// isFlattenableObject checks if a schema node is an object that can be flattened.
func isFlattenableObject(propMap map[string]any) bool {
	t, hasType := propMap["type"]
	if !hasType || t != "object" {
		return false
	}

	// Check for oneOf/anyOf - can't flatten these
	if _, hasOneOf := propMap["oneOf"]; hasOneOf {
		return false
	}
	if _, hasAnyOf := propMap["anyOf"]; hasAnyOf {
		return false
	}

	// Must have properties to flatten
	_, hasProps := propMap["properties"]
	return hasProps
}

// hasUnsupportedConstructs checks for constructs that prevent flattening.
func hasUnsupportedConstructs(schema map[string]any) bool {
	// Check for oneOf/anyOf at top level
	if _, has := schema["oneOf"]; has {
		return true
	}
	if _, has := schema["anyOf"]; has {
		return true
	}

	// Check properties for arrays of objects
	props, ok := schema["properties"].(map[string]any)
	if !ok {
		return false
	}

	for _, prop := range props {
		propMap, ok := prop.(map[string]any)
		if !ok {
			continue
		}

		// Check for array of objects
		if t, has := propMap["type"]; has && t == "array" {
			if items, hasItems := propMap["items"].(map[string]any); hasItems {
				if isArrayItemNestedObject(items) {
					return true
				}
			}
		}

		// Check nested properties recursively
		if hasUnsupportedConstructs(propMap) {
			return true
		}
	}

	return false
}

// hasCollisions checks if any flattened field names would collide.
func hasCollisions(fieldMap []FieldMapping) bool {
	seen := make(map[string]bool)
	for _, fm := range fieldMap {
		if seen[fm.FlatName] {
			return true
		}
		seen[fm.FlatName] = true
	}
	return false
}

// extractRequired extracts the required field list from a schema.
func extractRequired(schema map[string]any) []string {
	req, ok := schema["required"].([]any)
	if !ok {
		return nil
	}
	result := make([]string, 0, len(req))
	for _, r := range req {
		if s, ok := r.(string); ok {
			result = append(result, s)
		}
	}
	return result
}

// isRequired checks if a field is in the required list.
func isRequired(required []string, field string) bool {
	for _, r := range required {
		if r == field {
			return true
		}
	}
	return false
}

// Rehydrate transforms flat arguments back to their original nested structure.
// If no flat keys are found in the input, it returns the input unchanged
// (no re-serialization, no side effects). This prevents misleading repair
// events when the model returns nested args (e.g. because the flattening
// was bypassed or the model didn't follow the flat schema).
func (a SchemaAdapter) Rehydrate(args json.RawMessage) (json.RawMessage, error) {
	if len(a.FieldMap) == 0 {
		return args, nil
	}

	var flatArgs map[string]any
	if err := json.Unmarshal(args, &flatArgs); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}

	// Check if any flat keys are actually present in the input.
	// If none, the model returned nested args — return unchanged.
	hasFlatKey := false
	for key := range flatArgs {
		for i := range a.FieldMap {
			if a.FieldMap[i].FlatName == key {
				hasFlatKey = true
				break
			}
		}
		if hasFlatKey {
			break
		}
	}
	if !hasFlatKey {
		return args, nil
	}

	// Build nested structure
	nested := make(map[string]any)
	for key, value := range flatArgs {
		// Find the field mapping for this key
		var mapping *FieldMapping
		for i := range a.FieldMap {
			if a.FieldMap[i].FlatName == key {
				mapping = &a.FieldMap[i]
				break
			}
		}

		if mapping == nil {
			// No mapping found, keep as-is (might be a non-flattened field)
			nested[key] = value
			continue
		}

		// Insert value at nested path
		insertAtPath(nested, mapping.NestedPath, value)
	}

	output, err := json.Marshal(nested)
	if err != nil {
		return nil, err
	}

	return output, nil
}

// insertAtPath inserts a value into a nested map at the specified path.
func insertAtPath(m map[string]any, path []string, value any) {
	if len(path) == 0 {
		return
	}

	current := m
	for i := 0; i < len(path)-1; i++ {
		key := path[i]
		if _, exists := current[key]; !exists {
			current[key] = make(map[string]any)
		}
		if next, ok := current[key].(map[string]any); ok {
			current = next
		} else {
			// Path collision with non-object, create new map
			newMap := make(map[string]any)
			current[key] = newMap
			current = newMap
		}
	}

	current[path[len(path)-1]] = value
}
