// internal/llm/cache_stable_property_test.go
package llm

import (
	"encoding/json"
	"testing"
)

func TestMarshalCacheStableIsToolOrderIndependent(t *testing.T) {
	a := Tool{Type: "function", Function: ToolFunction{Name: "a", Parameters: json.RawMessage(`{"b":1,"a":2}`)}}
	b := Tool{Type: "function", Function: ToolFunction{Name: "b", Parameters: json.RawMessage(`{"z":1,"y":2}`)}}
	c := Tool{Type: "function", Function: ToolFunction{Name: "c", Parameters: json.RawMessage(`{"m":1}`)}}

	r1 := Request{Model: "deepseek-v4-flash", Tools: []Tool{a, b, c}}
	r2 := Request{Model: "deepseek-v4-flash", Tools: []Tool{c, a, b}}

	b1, err := r1.MarshalCacheStable()
	if err != nil {
		t.Fatal(err)
	}
	b2, err := r2.MarshalCacheStable()
	if err != nil {
		t.Fatal(err)
	}
	if string(b1) != string(b2) {
		t.Fatalf("tool order changed the cache-stable bytes:\n%s\n---\n%s", b1, b2)
	}
}

func TestFingerprintEqualsAcrossToolOrder(t *testing.T) {
	a := Tool{Type: "function", Function: ToolFunction{Name: "a", Parameters: json.RawMessage(`{"x":1}`)}}
	b := Tool{Type: "function", Function: ToolFunction{Name: "b", Parameters: json.RawMessage(`{"y":1}`)}}
	p1 := StaticPrefix{System: "S", Tools: []Tool{a, b}}
	p2 := StaticPrefix{System: "S", Tools: []Tool{b, a}}
	if p1.Fingerprint() != p2.Fingerprint() {
		t.Fatal("fingerprint must be invariant to tool order")
	}
}
