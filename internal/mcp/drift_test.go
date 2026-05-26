package mcp

import (
	"encoding/json"
	"testing"
)

func TestCompareToolLists_DriftNone(t *testing.T) {
	before := []McpToolMeta{
		{Name: "a", Description: "tool a", InputSchema: json.RawMessage(`{"type":"object"}`)},
		{Name: "b", Description: "tool b", InputSchema: json.RawMessage(`{"type":"object"}`)},
	}
	after := []McpToolMeta{
		{Name: "a", Description: "tool a", InputSchema: json.RawMessage(`{"type":"object"}`)},
		{Name: "b", Description: "tool b", InputSchema: json.RawMessage(`{"type":"object"}`)},
	}

	report := CompareToolLists(before, after)
	if report.Kind != DriftNone {
		t.Errorf("expected DriftNone, got %s: %s", report.Kind, report.Message)
	}
}

func TestCompareToolLists_NilAndEmpty(t *testing.T) {
	// Nil and empty are equivalent
	report := CompareToolLists(nil, nil)
	if report.Kind != DriftNone {
		t.Errorf("nil vs nil: expected DriftNone, got %s", report.Kind)
	}

	report = CompareToolLists(nil, []McpToolMeta{})
	if report.Kind != DriftNone {
		t.Errorf("nil vs empty: expected DriftNone, got %s", report.Kind)
	}

	report = CompareToolLists([]McpToolMeta{}, nil)
	if report.Kind != DriftNone {
		t.Errorf("empty vs nil: expected DriftNone, got %s", report.Kind)
	}
}

func TestCompareToolLists_Added(t *testing.T) {
	before := []McpToolMeta{
		{Name: "a", Description: "tool a"},
	}
	after := []McpToolMeta{
		{Name: "a", Description: "tool a"},
		{Name: "b", Description: "tool b"},
	}

	report := CompareToolLists(before, after)
	if report.Kind != DriftAdded {
		t.Errorf("expected DriftAdded, got %s: %s", report.Kind, report.Message)
	}
}

func TestCompareToolLists_Removed(t *testing.T) {
	before := []McpToolMeta{
		{Name: "a", Description: "tool a"},
		{Name: "b", Description: "tool b"},
	}
	after := []McpToolMeta{
		{Name: "a", Description: "tool a"},
	}

	report := CompareToolLists(before, after)
	if report.Kind != DriftRemoved {
		t.Errorf("expected DriftRemoved, got %s: %s", report.Kind, report.Message)
	}
}

func TestCompareToolLists_Reordered(t *testing.T) {
	before := []McpToolMeta{
		{Name: "a", Description: "tool a"},
		{Name: "b", Description: "tool b"},
	}
	after := []McpToolMeta{
		{Name: "b", Description: "tool b"},
		{Name: "a", Description: "tool a"},
	}

	report := CompareToolLists(before, after)
	if report.Kind != DriftReordered {
		t.Errorf("expected DriftReordered, got %s: %s", report.Kind, report.Message)
	}
}

func TestCompareToolLists_StructuredDescription(t *testing.T) {
	before := []McpToolMeta{
		{Name: "a", Description: "tool a"},
	}
	after := []McpToolMeta{
		{Name: "a", Description: "tool a changed"},
	}

	report := CompareToolLists(before, after)
	if report.Kind != DriftStructured {
		t.Errorf("expected DriftStructured, got %s: %s", report.Kind, report.Message)
	}
}

func TestCompareToolLists_StructuredSchema(t *testing.T) {
	before := []McpToolMeta{
		{Name: "a", InputSchema: json.RawMessage(`{"type":"object","properties":{"x":{"type":"string"}}}`)},
	}
	after := []McpToolMeta{
		{Name: "a", InputSchema: json.RawMessage(`{"type":"object","properties":{"x":{"type":"integer"}}}`)},
	}

	report := CompareToolLists(before, after)
	if report.Kind != DriftStructured {
		t.Errorf("expected DriftStructured, got %s: %s", report.Kind, report.Message)
	}
}

func TestCompareToolLists_SchemaKeyOrderIgnored(t *testing.T) {
	// Schema key order differences should NOT count as structured drift.
	before := []McpToolMeta{
		{Name: "a", InputSchema: json.RawMessage(`{"type":"object","properties":{"x":{"type":"string"},"y":{"type":"integer"}}}`)},
	}
	after := []McpToolMeta{
		{Name: "a", InputSchema: json.RawMessage(`{"properties":{"y":{"type":"integer"},"x":{"type":"string"}},"type":"object"}`)},
	}

	report := CompareToolLists(before, after)
	if report.Kind != DriftNone {
		t.Errorf("expected DriftNone for key-order-only change, got %s: %s", report.Kind, report.Message)
	}
}

func TestCompareToolLists_DuplicateNames(t *testing.T) {
	// Duplicate names should be treated as structured drift.
	before := []McpToolMeta{
		{Name: "a", Description: "tool a"},
	}
	after := []McpToolMeta{
		{Name: "a", Description: "tool a"},
		{Name: "a", Description: "tool a duplicate"},
	}

	report := CompareToolLists(before, after)
	if report.Kind != DriftStructured {
		t.Errorf("expected DriftStructured for duplicates, got %s: %s", report.Kind, report.Message)
	}
}

func TestCompareToolLists_DuplicateNamesBefore(t *testing.T) {
	// Duplicate names in before should be treated as structured drift.
	before := []McpToolMeta{
		{Name: "a", Description: "tool a"},
		{Name: "a", Description: "tool a duplicate"},
	}
	after := []McpToolMeta{
		{Name: "a", Description: "tool a"},
	}

	report := CompareToolLists(before, after)
	if report.Kind != DriftStructured {
		t.Errorf("expected DriftStructured for before duplicates, got %s: %s", report.Kind, report.Message)
	}
}

func TestCompareToolLists_DuplicateNamesNewlyAdded(t *testing.T) {
	// Duplicate names in after with a newly added tool should be treated as structured drift.
	before := []McpToolMeta{
		{Name: "a", Description: "tool a"},
	}
	after := []McpToolMeta{
		{Name: "a", Description: "tool a"},
		{Name: "b", Description: "tool b"},
		{Name: "b", Description: "tool b duplicate"},
	}

	report := CompareToolLists(before, after)
	if report.Kind != DriftStructured {
		t.Errorf("expected DriftStructured for newly added duplicates, got %s: %s", report.Kind, report.Message)
	}
}
