package repair

import (
	"strings"
	"testing"
)

func TestRepairJSONArgs_ValidInput(t *testing.T) {
	input := `{"path":"README.md"}`
	result := RepairJSONArgs(input)
	if !result.Valid {
		t.Errorf("expected Valid=true for valid JSON")
	}
	if result.Changed {
		t.Errorf("expected Changed=false for valid JSON")
	}
	if result.Repaired != input {
		t.Errorf("expected Repaired=input for valid JSON")
	}
}

func TestRepairJSONArgs_MissingClosingBrace(t *testing.T) {
	input := `{"path":"README.md"`
	result := RepairJSONArgs(input)
	if !result.Valid {
		t.Errorf("expected Valid=true after repair, got Notes: %v", result.Notes)
	}
	if !result.Changed {
		t.Errorf("expected Changed=true")
	}
	expected := `{"path":"README.md"}`
	if result.Repaired != expected {
		t.Errorf("expected %q, got %q", expected, result.Repaired)
	}
}

func TestRepairJSONArgs_TrailingComma(t *testing.T) {
	input := `{"a":1,}`
	result := RepairJSONArgs(input)
	if !result.Valid {
		t.Errorf("expected Valid=true after repair, got Notes: %v", result.Notes)
	}
	expected := `{"a":1}`
	if result.Repaired != expected {
		t.Errorf("expected %q, got %q", expected, result.Repaired)
	}
}

func TestRepairJSONArgs_TrailingCommaInArray(t *testing.T) {
	input := `{"a":[1,2,],}`
	result := RepairJSONArgs(input)
	if !result.Valid {
		t.Errorf("expected Valid=true after repair, got Notes: %v", result.Notes)
	}
}

func TestRepairJSONArgs_AmbiguousCloserOrder(t *testing.T) {
	// {"a":[1,2} - missing ] before } makes order ambiguous
	input := `{"a":[1,2}`
	result := RepairJSONArgs(input)
	// This should NOT be repairable deterministically
	if result.Valid {
		t.Errorf("expected Valid=false for ambiguous closer order, got Repaired=%q", result.Repaired)
	}
	if !result.Fallback {
		t.Errorf("expected Fallback=true for unrecoverable input")
	}
	if result.Repaired != input {
		t.Errorf("expected Repaired to equal original input for unrecoverable case")
	}
}

func TestRepairJSONArgs_UnterminatedString(t *testing.T) {
	input := `{"path":"README.md`
	result := RepairJSONArgs(input)
	if !result.Valid {
		t.Errorf("expected Valid=true after repair, got Notes: %v", result.Notes)
	}
	expected := `{"path":"README.md"}`
	if result.Repaired != expected {
		t.Errorf("expected %q, got %q", expected, result.Repaired)
	}
}

func TestRepairJSONArgs_NewlineInString(t *testing.T) {
	// Literal newline inside a quoted string
	input := "{\"path\":\"README.md\n\"}"
	result := RepairJSONArgs(input)
	if !result.Valid {
		t.Errorf("expected Valid=true after stripping control char, got Notes: %v", result.Notes)
	}
	if !result.Changed {
		t.Errorf("expected Changed=true")
	}
}

func TestRepairJSONArgs_Oversized(t *testing.T) {
	// Create input larger than 1 MiB
	input := strings.Repeat("x", 2<<20) // 2 MiB
	result := RepairJSONArgs(input)
	if result.Valid {
		t.Errorf("expected Valid=false for oversized input")
	}
	if !result.Fallback {
		t.Errorf("expected Fallback=true for oversized input")
	}
	if result.Repaired != input {
		t.Errorf("expected Repaired=input for oversized")
	}
}

func TestRepairJSONArgs_Unrecoverable(t *testing.T) {
	input := `{this is not json at all`
	result := RepairJSONArgs(input)
	if result.Valid {
		t.Errorf("expected Valid=false for unrecoverable input")
	}
	if !result.Fallback {
		t.Errorf("expected Fallback=true for unrecoverable input")
	}
	if result.Repaired != input {
		t.Errorf("expected Repaired=input for unrecoverable, got %q", result.Repaired)
	}
}

func TestRepairJSONArgs_NoFallbackEmptyObject(t *testing.T) {
	// Non-{} input should never return {} as fallback
	input := `{"missing":`
	result := RepairJSONArgs(input)
	// This is recoverable: {"missing":} is invalid, but we add null? No, we don't infer values.
	// Actually {"missing": would become {"missing":} which is still invalid.
	// So it should be fallback, but NOT with {}
	if result.Fallback && result.Repaired == "{}" && input != "{}" {
		t.Errorf("fallback should not return {} for non-{} input")
	}
}

func TestRepairJSONArgs_EmptyObject(t *testing.T) {
	input := `{}`
	result := RepairJSONArgs(input)
	if !result.Valid {
		t.Errorf("expected Valid=true for empty object")
	}
	if result.Changed {
		t.Errorf("expected Changed=false for already-valid input")
	}
}

func TestRepairJSONArgs_Nested(t *testing.T) {
	input := `{"outer":{"inner":"value"`
	result := RepairJSONArgs(input)
	if !result.Valid {
		t.Errorf("expected Valid=true after repair, got Notes: %v", result.Notes)
	}
	expected := `{"outer":{"inner":"value"}}`
	if result.Repaired != expected {
		t.Errorf("expected %q, got %q", expected, result.Repaired)
	}
}

func TestRepairJSONArgs_Array(t *testing.T) {
	input := `{"items":[1,2,3`
	result := RepairJSONArgs(input)
	if !result.Valid {
		t.Errorf("expected Valid=true after repair, got Notes: %v", result.Notes)
	}
	expected := `{"items":[1,2,3]}`
	if result.Repaired != expected {
		t.Errorf("expected %q, got %q", expected, result.Repaired)
	}
}

func TestRepairJSONArgs_MultipleTrailingCommas(t *testing.T) {
	// Double comma is not deterministically repairable
	input := `{"a":1,,"b":2,}`
	result := RepairJSONArgs(input)
	// This should be fallback - double comma is not recoverable deterministically
	if result.Valid {
		t.Errorf("expected Valid=false for double comma, got Repaired=%q", result.Repaired)
	}
	if !result.Fallback {
		t.Errorf("expected Fallback=true for unrecoverable input")
	}
}

func TestRepairJSONArgs_EscapedQuote(t *testing.T) {
	// String with escaped quote should not toggle string state
	input := `{"path":"say \"hello\""`
	result := RepairJSONArgs(input)
	if !result.Valid {
		t.Errorf("expected Valid=true after repair, got Notes: %v", result.Notes)
	}
	expected := `{"path":"say \"hello\""}`
	if result.Repaired != expected {
		t.Errorf("expected %q, got %q", expected, result.Repaired)
	}
}

func TestRepairTruncatedJSON_MissingBraces(t *testing.T) {
	input := `{"path":"README.md"`
	result := RepairTruncatedJSON(input)
	if !result.Valid {
		t.Errorf("expected Valid=true after repair, got Notes: %v", result.Notes)
	}
	if !result.Changed {
		t.Errorf("expected Changed=true")
	}
	if result.NeedMore {
		t.Errorf("expected NeedMore=false for missing braces (safe to auto-repair)")
	}
	expected := `{"path":"README.md"}`
	if result.Repaired != expected {
		t.Errorf("expected %q, got %q", expected, result.Repaired)
	}
}

func TestRepairTruncatedJSON_StringInternalTruncation(t *testing.T) {
	input := `{"path":"README`
	result := RepairTruncatedJSON(input)
	if !result.Valid {
		t.Errorf("expected Valid=true after repair, got Notes: %v", result.Notes)
	}
	if !result.Changed {
		t.Errorf("expected Changed=true")
	}
	if !result.NeedMore {
		t.Errorf("expected NeedMore=true for string-internal truncation (unsafe to auto-repair)")
	}
	// Even though we repaired it, we signal that the repair was unsafe
	expected := `{"path":"README"}`
	if result.Repaired != expected {
		t.Errorf("expected %q, got %q", expected, result.Repaired)
	}
}

func TestRepairTruncatedJSON_ValidPassthrough(t *testing.T) {
	input := `{"path":"README.md"}`
	result := RepairTruncatedJSON(input)
	if !result.Valid {
		t.Errorf("expected Valid=true for valid JSON")
	}
	if result.Changed {
		t.Errorf("expected Changed=false for valid JSON")
	}
	if result.NeedMore {
		t.Errorf("expected NeedMore=false for valid JSON")
	}
	if result.Repaired != input {
		t.Errorf("expected Repaired=input for valid JSON")
	}
}

func TestRepairTruncatedJSON_Unrecoverable(t *testing.T) {
	input := `{this is not json at all`
	result := RepairTruncatedJSON(input)
	if result.Valid {
		t.Errorf("expected Valid=false for unrecoverable input")
	}
	if !result.Fallback {
		t.Errorf("expected Fallback=true for unrecoverable input")
	}
	// NeedMore depends on string state - in this case, truncation is not in a string
	if result.NeedMore {
		t.Errorf("expected NeedMore=false when not truncated in string")
	}
	if result.Repaired != input {
		t.Errorf("expected Repaired=input for unrecoverable, got %q", result.Repaired)
	}
}

func TestRepairTruncatedJSON_StringInternalTruncation_EscapedQuote(t *testing.T) {
	// Test that escaped quotes don't confuse string state detection
	input := `{"path":"say \"hello`
	result := RepairTruncatedJSON(input)
	if !result.Valid {
		t.Errorf("expected Valid=true after repair, got Notes: %v", result.Notes)
	}
	if !result.NeedMore {
		t.Errorf("expected NeedMore=true for string-internal truncation with escaped quote")
	}
}
