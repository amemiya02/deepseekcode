package acp_test

import (
	"encoding/json"
	"testing"

	"github.com/amemiya02/deepseekcode/internal/acp"
)

func TestRequestRoundTrip(t *testing.T) {
	req := acp.Request{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`1`),
		Method:  "session/new",
		Params:  json.RawMessage(`{"workingDir":"/tmp"}`),
	}
	b, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	var got acp.Request
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got.Method != "session/new" {
		t.Fatalf("method mismatch: %s", got.Method)
	}
	if string(got.ID) != `1` {
		t.Fatalf("ID mismatch: got %s, want 1", got.ID)
	}
	if string(got.Params) != `{"workingDir":"/tmp"}` {
		t.Fatalf("Params mismatch: %s", got.Params)
	}
	if got.JSONRPC != "2.0" {
		t.Fatalf("JSONRPC mismatch: %s", got.JSONRPC)
	}
}

func TestRequestStringID(t *testing.T) {
	// JSON-RPC 2.0 §4 permits string IDs; must unmarshal without error.
	raw := `{"jsonrpc":"2.0","id":"abc","method":"session/cancel"}`
	var req acp.Request
	if err := json.Unmarshal([]byte(raw), &req); err != nil {
		t.Fatalf("string ID unmarshal failed: %v", err)
	}
	if string(req.ID) != `"abc"` {
		t.Fatalf("ID mismatch: got %s, want \"abc\"", req.ID)
	}
	if req.Method != "session/cancel" {
		t.Fatalf("Method mismatch: %s", req.Method)
	}
}

func TestNotificationNoID(t *testing.T) {
	n := acp.Notification{
		JSONRPC: "2.0",
		Method:  "session/textDelta",
		Params:  json.RawMessage(`{"sessionId":"s1","delta":"hello"}`),
	}
	b, err := json.Marshal(n)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if _, hasID := m["id"]; hasID {
		t.Fatal("notification must not have id field")
	}
	// Verify remaining fields survive intact.
	if string(m["jsonrpc"]) != `"2.0"` {
		t.Fatalf("jsonrpc mismatch: %s", m["jsonrpc"])
	}
	if string(m["method"]) != `"session/textDelta"` {
		t.Fatalf("method mismatch: %s", m["method"])
	}
	if string(m["params"]) != `{"sessionId":"s1","delta":"hello"}` {
		t.Fatalf("params mismatch: %s", m["params"])
	}
}

func TestResponseID(t *testing.T) {
	r := acp.Response{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`42`),
		Result:  json.RawMessage(`{"sessionId":"s1"}`),
	}
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if _, hasMethod := m["method"]; hasMethod {
		t.Fatal("response must not have method field")
	}
	// Verify remaining fields survive intact.
	if string(m["jsonrpc"]) != `"2.0"` {
		t.Fatalf("jsonrpc mismatch: %s", m["jsonrpc"])
	}
	if string(m["id"]) != `42` {
		t.Fatalf("id mismatch: %s", m["id"])
	}
	if string(m["result"]) != `{"sessionId":"s1"}` {
		t.Fatalf("result mismatch: %s", m["result"])
	}
	if _, hasErr := m["error"]; hasErr {
		t.Fatal("response with no Error must omit error field")
	}
}

func TestRPCErrorInterface(t *testing.T) {
	e := &acp.RPCError{Code: -32601, Message: "method not found"}
	got := e.Error()
	want := "rpc error -32601: method not found"
	if got != want {
		t.Fatalf("Error() = %q, want %q", got, want)
	}
}

func TestRPCErrorData(t *testing.T) {
	payload := `{"code":-32602,"message":"invalid params","data":{"field":"x"}}`
	var e acp.RPCError
	if err := json.Unmarshal([]byte(payload), &e); err != nil {
		t.Fatal(err)
	}
	if string(e.Data) != `{"field":"x"}` {
		t.Fatalf("Data mismatch: %s", e.Data)
	}
}

func TestValidateVersion(t *testing.T) {
	req := acp.Request{JSONRPC: "1.0", ID: json.RawMessage(`1`), Method: "x"}
	if err := req.Validate(); err == nil {
		t.Fatal("expected error for wrong jsonrpc version in Request")
	}

	n := acp.Notification{JSONRPC: "", Method: "x"}
	if err := n.Validate(); err == nil {
		t.Fatal("expected error for missing jsonrpc version in Notification")
	}

	resp := acp.Response{JSONRPC: "2.0", ID: json.RawMessage(`1`), Result: json.RawMessage(`{}`)}
	if err := resp.Validate(); err != nil {
		t.Fatalf("valid response rejected: %v", err)
	}
}

func TestValidateResponseMutualExclusion(t *testing.T) {
	resp := acp.Response{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`1`),
		Result:  json.RawMessage(`{}`),
		Error:   &acp.RPCError{Code: -32603, Message: "internal"},
	}
	if err := resp.Validate(); err == nil {
		t.Fatal("expected error when both Result and Error are set")
	}
}

func TestValidateResponseNeitherSet(t *testing.T) {
	// JSON-RPC §5 requires exactly one of Result or Error. Neither set is invalid.
	resp := acp.Response{JSONRPC: "2.0", ID: json.RawMessage(`1`)}
	if err := resp.Validate(); err == nil {
		t.Fatal("expected error when neither Result nor Error is set")
	}
}

func TestDoneParamsErrorPointer(t *testing.T) {
	// nil Error must omit the field entirely.
	d := acp.DoneParams{SessionID: "s1", StopReason: "end_turn"}
	b, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if _, has := m["error"]; has {
		t.Fatal("DoneParams with nil Error must omit error field")
	}

	// Non-nil pointer (including empty string) must be present.
	errMsg := ""
	d2 := acp.DoneParams{SessionID: "s2", StopReason: "error", Error: &errMsg}
	b2, err := json.Marshal(d2)
	if err != nil {
		t.Fatal(err)
	}
	var m2 map[string]json.RawMessage
	if err := json.Unmarshal(b2, &m2); err != nil {
		t.Fatal(err)
	}
	if _, has := m2["error"]; !has {
		t.Fatal("DoneParams with non-nil Error (empty string) must include error field")
	}
}
