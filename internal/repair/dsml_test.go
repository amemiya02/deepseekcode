package repair

import (
	"encoding/json"
	"testing"
)

func TestParseDSML_SingleStringParam(t *testing.T) {
	input := "<｜DSML｜tool_calls><｜DSML｜invoke name=\"read_file\"><｜DSML｜parameter name=\"path\" string=\"true\">README.md</｜DSML｜parameter></｜DSML｜invoke></｜DSML｜tool_calls>"

	calls := parseDSMLToolCalls(input)
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}
	if calls[0].Function.Name != "read_file" {
		t.Errorf("expected name read_file, got %s", calls[0].Function.Name)
	}
	// Arguments should be {"path":"README.md"}
	var args map[string]any
	if err := json.Unmarshal([]byte(calls[0].Function.Arguments), &args); err != nil {
		t.Fatalf("failed to parse arguments: %v", err)
	}
	if args["path"] != "README.md" {
		t.Errorf("expected path=README.md, got %v", args["path"])
	}
}

func TestParseDSML_NonStringParam(t *testing.T) {
	input := "<｜DSML｜invoke name=\"tool\"><｜DSML｜parameter name=\"count\" string=\"false\">5</｜DSML｜parameter></｜DSML｜invoke>"

	calls := parseDSMLToolCalls(input)
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}

	var args map[string]any
	if err := json.Unmarshal([]byte(calls[0].Function.Arguments), &args); err != nil {
		t.Fatalf("failed to parse arguments: %v", err)
	}
	// string="false" with value "5" should be parsed as JSON number.
	count, ok := args["count"].(float64)
	if !ok {
		t.Fatalf("expected count to be a number, got %T (%v)", args["count"], args["count"])
	}
	if count != 5 {
		t.Errorf("expected count=5, got %v", count)
	}
}

func TestParseDSML_NoMarkup(t *testing.T) {
	input := "This is plain prose with no DSML markup."

	calls := parseDSMLToolCalls(input)
	if calls != nil {
		t.Errorf("expected nil for no markup, got %d calls", len(calls))
	}
}

func TestParseDSML_Truncated(t *testing.T) {
	// Unterminated invoke (no close tag) should yield 0 calls.
	input := "<｜DSML｜invoke name=\"tool\"><｜DSML｜parameter name=\"key\" string=\"true\">value"

	calls := parseDSMLToolCalls(input)
	if len(calls) != 0 {
		t.Errorf("expected 0 calls for truncated invoke, got %d", len(calls))
	}
}

func TestParseDSML_MultiInvoke(t *testing.T) {
	input := "<｜DSML｜invoke name=\"read_file\"><｜DSML｜parameter name=\"path\" string=\"true\">a.go</｜DSML｜parameter></｜DSML｜invoke>" +
		"<｜DSML｜invoke name=\"write_file\"><｜DSML｜parameter name=\"path\" string=\"true\">b.go</｜DSML｜parameter>" +
		"<｜DSML｜parameter name=\"content\" string=\"true\">hello</｜DSML｜parameter></｜DSML｜invoke>"

	calls := parseDSMLToolCalls(input)
	if len(calls) != 2 {
		t.Fatalf("expected 2 calls, got %d", len(calls))
	}
	if calls[0].Function.Name != "read_file" {
		t.Errorf("expected first call name read_file, got %s", calls[0].Function.Name)
	}
	if calls[1].Function.Name != "write_file" {
		t.Errorf("expected second call name write_file, got %s", calls[1].Function.Name)
	}
}

func TestParseDSML_MixedParams(t *testing.T) {
	// Mix of string and non-string parameters.
	input := "<｜DSML｜invoke name=\"grep\"><｜DSML｜parameter name=\"pattern\" string=\"true\">foo</｜DSML｜parameter>" +
		"<｜DSML｜parameter name=\"n\" string=\"false\">10</｜DSML｜parameter></｜DSML｜invoke>"

	calls := parseDSMLToolCalls(input)
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}
	if calls[0].Function.Name != "grep" {
		t.Errorf("expected name grep, got %s", calls[0].Function.Name)
	}

	var args map[string]any
	if err := json.Unmarshal([]byte(calls[0].Function.Arguments), &args); err != nil {
		t.Fatalf("failed to parse arguments: %v", err)
	}
	if args["pattern"] != "foo" {
		t.Errorf("expected pattern=foo, got %v", args["pattern"])
	}
	n, ok := args["n"].(float64)
	if !ok {
		t.Fatalf("expected n to be a number, got %T", args["n"])
	}
	if n != 10 {
		t.Errorf("expected n=10, got %v", n)
	}
}

func TestParseDSML_ZeroParams(t *testing.T) {
	input := "<｜DSML｜invoke name=\"list_files\"></｜DSML｜invoke>"

	calls := parseDSMLToolCalls(input)
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}
	if calls[0].Function.Name != "list_files" {
		t.Errorf("expected name list_files, got %s", calls[0].Function.Name)
	}
	if calls[0].Function.Arguments != "{}" {
		t.Errorf("expected empty args {}, got %s", calls[0].Function.Arguments)
	}
}

func TestParseDSML_IDPrefix(t *testing.T) {
	input := "<｜DSML｜invoke name=\"tool\"><｜DSML｜parameter name=\"k\" string=\"true\">v</｜DSML｜parameter></｜DSML｜invoke>"

	calls := parseDSMLToolCalls(input)
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}
	if len(calls[0].ID) < 10 || calls[0].ID[:10] != "recovered_" {
		t.Errorf("expected ID prefix 'recovered_', got %s", calls[0].ID)
	}
}

func TestParseDSML_InvalidJSONNonString(t *testing.T) {
	// string="false" with value that is not valid JSON should store raw string.
	input := "<｜DSML｜invoke name=\"tool\"><｜DSML｜parameter name=\"val\" string=\"false\">not-json</｜DSML｜parameter></｜DSML｜invoke>"

	calls := parseDSMLToolCalls(input)
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}

	var args map[string]any
	if err := json.Unmarshal([]byte(calls[0].Function.Arguments), &args); err != nil {
		t.Fatalf("failed to parse arguments: %v", err)
	}
	if args["val"] != "not-json" {
		t.Errorf("expected val=not-json (raw fallback), got %v", args["val"])
	}
}
