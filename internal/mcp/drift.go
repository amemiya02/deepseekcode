package mcp

import (
	"encoding/json"
	"sort"
	"strings"
)

// DriftKind identifies the type of tool list drift.
type DriftKind string

const (
	DriftNone       DriftKind = "none"
	DriftAdded      DriftKind = "added"
	DriftRemoved    DriftKind = "removed"
	DriftReordered  DriftKind = "reordered"
	DriftStructured DriftKind = "structured"
)

// DriftReport describes the drift between two tool lists.
type DriftReport struct {
	Kind    DriftKind
	Message string
}

// CompareToolLists compares before and after tool lists and returns a
// DriftReport describing the difference. Nil and empty lists are
// equivalent.
func CompareToolLists(before, after []McpToolMeta) DriftReport {
	// Nil and empty are equivalent.
	if len(before) == 0 && len(after) == 0 {
		return DriftReport{Kind: DriftNone}
	}

	// Extract names in order.
	beforeNames := extractNames(before)
	afterNames := extractNames(after)

	// Check for duplicates in either list (treated as structured drift).
	if hasDuplicates(beforeNames) || hasDuplicates(afterNames) {
		return DriftReport{
			Kind:    DriftStructured,
			Message: "duplicate tool names detected",
		}
	}

	// Check for added tools.
	afterSet := make(map[string]bool)
	for _, n := range afterNames {
		afterSet[n] = true
	}
	var added []string
	for _, n := range afterNames {
		if !contains(beforeNames, n) {
			added = append(added, n)
		}
	}
	if len(added) > 0 {
		return DriftReport{
			Kind:    DriftAdded,
			Message: "added tools: " + strings.Join(added, ", "),
		}
	}

	// Check for removed tools.
	beforeSet := make(map[string]bool)
	for _, n := range beforeNames {
		beforeSet[n] = true
	}
	var removed []string
	for _, n := range beforeNames {
		if !contains(afterNames, n) {
			removed = append(removed, n)
		}
	}
	if len(removed) > 0 {
		return DriftReport{
			Kind:    DriftRemoved,
			Message: "removed tools: " + strings.Join(removed, ", "),
		}
	}

	// Check for reorder (same names, different order).
	if !sameOrder(beforeNames, afterNames) {
		return DriftReport{
			Kind:    DriftReordered,
			Message: "tool order changed",
		}
	}

	// Check for structured changes (description or schema differs).
	beforeMap := make(map[string]McpToolMeta)
	for _, t := range before {
		beforeMap[t.Name] = t
	}
	for _, t := range after {
		b, ok := beforeMap[t.Name]
		if !ok {
			continue
		}
		if b.Description != t.Description {
			return DriftReport{
				Kind:    DriftStructured,
				Message: "description changed for tool: " + t.Name,
			}
		}
		if !schemasEqual(b.InputSchema, t.InputSchema) {
			return DriftReport{
				Kind:    DriftStructured,
				Message: "schema changed for tool: " + t.Name,
			}
		}
	}

	return DriftReport{Kind: DriftNone}
}

// extractNames returns tool names in order.
func extractNames(tools []McpToolMeta) []string {
	names := make([]string, len(tools))
	for i, t := range tools {
		names[i] = t.Name
	}
	return names
}

// contains checks if a string slice contains a value.
func contains(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}

// hasDuplicates checks if a string slice has duplicates.
func hasDuplicates(ss []string) bool {
	seen := make(map[string]bool)
	for _, s := range ss {
		if seen[s] {
			return true
		}
		seen[s] = true
	}
	return false
}

// sameOrder checks if two string slices have the same order.
func sameOrder(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// schemasEqual compares two JSON schemas for structural equality,
// ignoring key order. This prevents false structured drift from
// non-deterministic key ordering.
func schemasEqual(a, b json.RawMessage) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	if len(a) == 0 || len(b) == 0 {
		return false
	}

	// Parse both schemas into canonical form.
	var va, vb any
	if err := json.Unmarshal(a, &va); err != nil {
		return false
	}
	if err := json.Unmarshal(b, &vb); err != nil {
		return false
	}

	return canonicalEqual(va, vb)
}

// canonicalEqual recursively compares two values after canonicalizing
// maps (sorting keys).
func canonicalEqual(a, b any) bool {
	switch va := a.(type) {
	case map[string]any:
		vb, ok := b.(map[string]any)
		if !ok {
			return false
		}
		if len(va) != len(vb) {
			return false
		}
		// Sort keys for deterministic comparison.
		keysA := sortedKeys(va)
		keysB := sortedKeys(vb)
		if !sameOrder(keysA, keysB) {
			return false
		}
		for _, k := range keysA {
			if !canonicalEqual(va[k], vb[k]) {
				return false
			}
		}
		return true
	case []any:
		vb, ok := b.([]any)
		if !ok {
			return false
		}
		if len(va) != len(vb) {
			return false
		}
		for i := range va {
			if !canonicalEqual(va[i], vb[i]) {
				return false
			}
		}
		return true
	default:
		// For primitives, use JSON marshal for comparison.
		ja, _ := json.Marshal(a)
		jb, _ := json.Marshal(b)
		return string(ja) == string(jb)
	}
}

// sortedKeys returns sorted keys of a map.
func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
