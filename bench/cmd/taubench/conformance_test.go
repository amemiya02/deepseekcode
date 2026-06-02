// Package taubench — conformance gate: Go-side strings/schemas must be
// byte-equal to the committed golden fixtures, which are verbatim copies of
// the TypeScript source-of-truth in
// benchmarks/tau-bench/tasks.ts and benchmarks/tau-bench/user-sim.ts.
//
// If this test fails, the harness is unfair and no live run should proceed.
package taubench

import (
	_ "embed"
	"testing"
)

// Golden fixtures — committed copies of the TypeScript originals.
//
//go:embed golden/retail_system_prompt.txt
var goldenRetailSystem string

//go:embed golden/usersim_sys.txt
var goldenUserSimSys string

//go:embed golden/schema_lookup_order.json
var goldenSchemaLookupOrder string

//go:embed golden/schema_lookup_user.json
var goldenSchemaLookupUser string

//go:embed golden/schema_update_address.json
var goldenSchemaUpdateAddress string

//go:embed golden/schema_cancel_order.json
var goldenSchemaCancelOrder string

//go:embed golden/schema_refund_order.json
var goldenSchemaRefundOrder string

//go:embed golden/schema_list_user_orders.json
var goldenSchemaListUserOrders string

// TestConformanceRetailSystemPrompt asserts the Go retailSystemPrompt constant
// is byte-equal to the golden fixture.
func TestConformanceRetailSystemPrompt(t *testing.T) {
	if retailSystemPrompt != goldenRetailSystem {
		t.Errorf("retailSystemPrompt does not match golden fixture\ngot:\n%s\nwant:\n%s",
			retailSystemPrompt, goldenRetailSystem)
	}
}

// TestConformanceUserSimSys asserts the Go userSimSys constant is byte-equal
// to the golden fixture.
func TestConformanceUserSimSys(t *testing.T) {
	if userSimSys != goldenUserSimSys {
		t.Errorf("userSimSys does not match golden fixture\ngot:\n%s\nwant:\n%s",
			userSimSys, goldenUserSimSys)
	}
}

// TestConformanceToolSchemas asserts each tool's Parameters() JSON is
// byte-equal to the corresponding golden fixture.
func TestConformanceToolSchemas(t *testing.T) {
	db := retailSeed()
	tools := newRetailTools(&db)

	cases := []struct {
		toolName string
		golden   string
	}{
		{"lookup_order", goldenSchemaLookupOrder},
		{"lookup_user", goldenSchemaLookupUser},
		{"update_address", goldenSchemaUpdateAddress},
		{"cancel_order", goldenSchemaCancelOrder},
		{"refund_order", goldenSchemaRefundOrder},
		{"list_user_orders", goldenSchemaListUserOrders},
	}

	// Build a name→tool map for O(1) lookup.
	byName := make(map[string]retailTool, len(tools))
	for _, tool := range tools {
		byName[tool.Name()] = tool
	}

	for _, tc := range cases {
		t.Run(tc.toolName, func(t *testing.T) {
			tool, ok := byName[tc.toolName]
			if !ok {
				t.Fatalf("tool %q not found in newRetailTools", tc.toolName)
			}
			got := string(tool.Parameters())
			if got != tc.golden {
				t.Errorf("schema mismatch for %s\ngot:  %s\nwant: %s", tc.toolName, got, tc.golden)
			}
		})
	}
}
