package taubench

import (
	"context"
	"encoding/json"
	"testing"
)

// schemaEqual compares two JSON byte slices for structural equality by
// unmarshalling both and comparing the resulting maps. This is robust against
// insignificant whitespace differences while still validating field-level parity.
func schemaEqual(a, b []byte) bool {
	var ma, mb any
	if err := json.Unmarshal(a, &ma); err != nil {
		return false
	}
	if err := json.Unmarshal(b, &mb); err != nil {
		return false
	}
	ra, _ := json.Marshal(ma)
	rb, _ := json.Marshal(mb)
	return string(ra) == string(rb)
}

// wantSchemas holds the verbatim JSON schemas from tasks.ts (TS object literals
// rendered as JSON). Used to verify byte-equal (structural) parity.
var wantSchemas = map[string]string{
	"lookup_order":     `{"type":"object","properties":{"orderId":{"type":"string"}},"required":["orderId"]}`,
	"lookup_user":      `{"type":"object","properties":{"userId":{"type":"string"}},"required":["userId"]}`,
	"update_address":   `{"type":"object","properties":{"orderId":{"type":"string"},"address":{"type":"string"}},"required":["orderId","address"]}`,
	"cancel_order":     `{"type":"object","properties":{"orderId":{"type":"string"}},"required":["orderId"]}`,
	"refund_order":     `{"type":"object","properties":{"orderId":{"type":"string"},"reason":{"type":"string"}},"required":["orderId","reason"]}`,
	"list_user_orders": `{"type":"object","properties":{"userId":{"type":"string"}},"required":["userId"]}`,
}

// wantDescriptions holds the exact description strings from tasks.ts.
var wantDescriptions = map[string]string{
	"lookup_order":     "Look up an order by id. Returns { userId, status, address, item, price } or null.",
	"lookup_user":      "Look up a user by id. Returns { name, email } or error.",
	"update_address":   "Update the shipping address on an order. Only allowed if status is 'processing'. Returns ok/error.",
	"cancel_order":     "Cancel an order. Only allowed if status is 'processing'. Returns ok/error.",
	"refund_order":     "Issue a refund on a delivered order. Records an entry in refunds. Returns ok/error.",
	"list_user_orders": "List every order belonging to a userId. Returns an array of orders.",
}

func TestToolSchemas(t *testing.T) {
	db := retailSeed()
	tools := newRetailTools(&db)

	for _, tool := range tools {
		name := tool.Name()
		wantSchema, ok := wantSchemas[name]
		if !ok {
			t.Errorf("unexpected tool %q", name)
			continue
		}
		got := tool.Parameters()
		if !schemaEqual(got, []byte(wantSchema)) {
			t.Errorf("tool %q schema mismatch:\n  got:  %s\n  want: %s", name, got, wantSchema)
		}
	}
}

func TestToolDescriptions(t *testing.T) {
	db := retailSeed()
	tools := newRetailTools(&db)

	for _, tool := range tools {
		name := tool.Name()
		want, ok := wantDescriptions[name]
		if !ok {
			t.Errorf("unexpected tool %q", name)
			continue
		}
		if got := tool.Description(); got != want {
			t.Errorf("tool %q description mismatch:\n  got:  %q\n  want: %q", name, got, want)
		}
	}
}

func TestToolCount(t *testing.T) {
	db := retailSeed()
	tools := newRetailTools(&db)
	if len(tools) != 6 {
		t.Errorf("newRetailTools: got %d tools, want 6", len(tools))
	}
}

// --- lookup_order ---

func TestLookupOrderFound(t *testing.T) {
	db := retailSeed()
	tools := newRetailTools(&db)
	tool := findTool(tools, "lookup_order")

	res, err := tool.Execute(context.Background(), json.RawMessage(`{"orderId":"o_1002"}`))
	if err != nil {
		t.Fatalf("lookup_order: unexpected error: %v", err)
	}
	if res.IsError {
		t.Fatalf("lookup_order: IsError=true, content=%q", res.Content)
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(res.Content), &m); err != nil {
		t.Fatalf("lookup_order: content not valid JSON: %v", err)
	}
	if m["orderId"] != "o_1002" {
		t.Errorf("lookup_order: orderId = %v, want o_1002", m["orderId"])
	}
	if m["status"] != "processing" {
		t.Errorf("lookup_order: status = %v, want processing", m["status"])
	}
}

func TestLookupOrderNotFound(t *testing.T) {
	db := retailSeed()
	tools := newRetailTools(&db)
	tool := findTool(tools, "lookup_order")

	res, err := tool.Execute(context.Background(), json.RawMessage(`{"orderId":"o_9999"}`))
	if err != nil {
		t.Fatalf("lookup_order: unexpected error: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(res.Content), &m); err != nil {
		t.Fatalf("lookup_order: content not valid JSON: %v", err)
	}
	if _, hasErr := m["error"]; !hasErr {
		t.Errorf("lookup_order: expected error key, got %v", m)
	}
}

// --- lookup_user ---

func TestLookupUserFound(t *testing.T) {
	db := retailSeed()
	tools := newRetailTools(&db)
	tool := findTool(tools, "lookup_user")

	res, err := tool.Execute(context.Background(), json.RawMessage(`{"userId":"u_ari"}`))
	if err != nil {
		t.Fatalf("lookup_user: unexpected error: %v", err)
	}
	if res.IsError {
		t.Fatalf("lookup_user: IsError=true, content=%q", res.Content)
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(res.Content), &m); err != nil {
		t.Fatalf("lookup_user: content not valid JSON: %v", err)
	}
	if m["userId"] != "u_ari" {
		t.Errorf("lookup_user: userId = %v, want u_ari", m["userId"])
	}
	if m["name"] != "Ari Chen" {
		t.Errorf("lookup_user: name = %v, want Ari Chen", m["name"])
	}
}

func TestLookupUserNotFound(t *testing.T) {
	db := retailSeed()
	tools := newRetailTools(&db)
	tool := findTool(tools, "lookup_user")

	res, err := tool.Execute(context.Background(), json.RawMessage(`{"userId":"u_nobody"}`))
	if err != nil {
		t.Fatalf("lookup_user: unexpected error: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(res.Content), &m); err != nil {
		t.Fatalf("lookup_user: content not valid JSON: %v", err)
	}
	if _, hasErr := m["error"]; !hasErr {
		t.Errorf("lookup_user: expected error key, got %v", m)
	}
}

// --- update_address ---

func TestUpdateAddressProcessing(t *testing.T) {
	db := retailSeed()
	tools := newRetailTools(&db)
	tool := findTool(tools, "update_address")

	args := json.RawMessage(`{"orderId":"o_1002","address":"5 Birch Rd, NYC, NY 10001"}`)
	res, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("update_address: unexpected error: %v", err)
	}
	if res.IsError {
		t.Fatalf("update_address: IsError=true, content=%q", res.Content)
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(res.Content), &m); err != nil {
		t.Fatalf("update_address: content not valid JSON: %v", err)
	}
	if m["ok"] != true {
		t.Errorf("update_address: ok = %v, want true", m["ok"])
	}
	if m["newAddress"] != "5 Birch Rd, NYC, NY 10001" {
		t.Errorf("update_address: newAddress = %v", m["newAddress"])
	}
	// Verify DB mutation.
	if got, _ := db["orders"]["o_1002"]["address"].(string); got != "5 Birch Rd, NYC, NY 10001" {
		t.Errorf("update_address: DB address = %q, want new address", got)
	}
}

func TestUpdateAddressRejectsShipped(t *testing.T) {
	db := retailSeed()
	tools := newRetailTools(&db)
	tool := findTool(tools, "update_address")

	args := json.RawMessage(`{"orderId":"o_1001","address":"99 New St"}`)
	res, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("update_address: unexpected error: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(res.Content), &m); err != nil {
		t.Fatalf("update_address: content not valid JSON: %v", err)
	}
	if _, hasErr := m["error"]; !hasErr {
		t.Errorf("update_address: expected error for shipped order, got %v", m)
	}
	// DB must be unchanged.
	if got, _ := db["orders"]["o_1001"]["address"].(string); got != "1 Elm St, SF, CA 94110" {
		t.Errorf("update_address: DB mutated despite rejection; address = %q", got)
	}
}

func TestUpdateAddressRejectsDelivered(t *testing.T) {
	db := retailSeed()
	tools := newRetailTools(&db)
	tool := findTool(tools, "update_address")

	args := json.RawMessage(`{"orderId":"o_1003","address":"new addr"}`)
	res, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("update_address: unexpected error: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(res.Content), &m); err != nil {
		t.Fatalf("update_address: content not valid JSON: %v", err)
	}
	if _, hasErr := m["error"]; !hasErr {
		t.Errorf("update_address: expected error for delivered order, got %v", m)
	}
}

func TestUpdateAddressNotFound(t *testing.T) {
	db := retailSeed()
	tools := newRetailTools(&db)
	tool := findTool(tools, "update_address")

	args := json.RawMessage(`{"orderId":"o_9999","address":"somewhere"}`)
	res, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("update_address: unexpected error: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(res.Content), &m); err != nil {
		t.Fatalf("update_address: content not valid JSON: %v", err)
	}
	if _, hasErr := m["error"]; !hasErr {
		t.Errorf("update_address: expected error for missing order, got %v", m)
	}
}

// --- cancel_order ---

func TestCancelOrderProcessing(t *testing.T) {
	db := retailSeed()
	tools := newRetailTools(&db)
	tool := findTool(tools, "cancel_order")

	res, err := tool.Execute(context.Background(), json.RawMessage(`{"orderId":"o_1004"}`))
	if err != nil {
		t.Fatalf("cancel_order: unexpected error: %v", err)
	}
	if res.IsError {
		t.Fatalf("cancel_order: IsError=true, content=%q", res.Content)
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(res.Content), &m); err != nil {
		t.Fatalf("cancel_order: content not valid JSON: %v", err)
	}
	if m["ok"] != true {
		t.Errorf("cancel_order: ok = %v, want true", m["ok"])
	}
	if m["status"] != "cancelled" {
		t.Errorf("cancel_order: status = %v, want cancelled", m["status"])
	}
	// Verify DB mutation.
	if got, _ := db["orders"]["o_1004"]["status"].(string); got != "cancelled" {
		t.Errorf("cancel_order: DB status = %q, want cancelled", got)
	}
}

func TestCancelOrderRejectsShipped(t *testing.T) {
	db := retailSeed()
	tools := newRetailTools(&db)
	tool := findTool(tools, "cancel_order")

	res, err := tool.Execute(context.Background(), json.RawMessage(`{"orderId":"o_1001"}`))
	if err != nil {
		t.Fatalf("cancel_order: unexpected error: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(res.Content), &m); err != nil {
		t.Fatalf("cancel_order: content not valid JSON: %v", err)
	}
	if _, hasErr := m["error"]; !hasErr {
		t.Errorf("cancel_order: expected error for shipped order, got %v", m)
	}
	// DB unchanged.
	if got, _ := db["orders"]["o_1001"]["status"].(string); got != "shipped" {
		t.Errorf("cancel_order: DB mutated; status = %q", got)
	}
}

func TestCancelOrderRejectsDelivered(t *testing.T) {
	db := retailSeed()
	tools := newRetailTools(&db)
	tool := findTool(tools, "cancel_order")

	res, err := tool.Execute(context.Background(), json.RawMessage(`{"orderId":"o_1003"}`))
	if err != nil {
		t.Fatalf("cancel_order: unexpected error: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(res.Content), &m); err != nil {
		t.Fatalf("cancel_order: content not valid JSON: %v", err)
	}
	if _, hasErr := m["error"]; !hasErr {
		t.Errorf("cancel_order: expected error for delivered order, got %v", m)
	}
}

func TestCancelOrderNotFound(t *testing.T) {
	db := retailSeed()
	tools := newRetailTools(&db)
	tool := findTool(tools, "cancel_order")

	res, err := tool.Execute(context.Background(), json.RawMessage(`{"orderId":"o_9999"}`))
	if err != nil {
		t.Fatalf("cancel_order: unexpected error: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(res.Content), &m); err != nil {
		t.Fatalf("cancel_order: content not valid JSON: %v", err)
	}
	if _, hasErr := m["error"]; !hasErr {
		t.Errorf("cancel_order: expected error for missing order, got %v", m)
	}
}

// --- refund_order ---

func TestRefundOrderDelivered(t *testing.T) {
	db := retailSeed()
	tools := newRetailTools(&db)
	tool := findTool(tools, "refund_order")

	args := json.RawMessage(`{"orderId":"o_1003","reason":"arrived broken"}`)
	res, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("refund_order: unexpected error: %v", err)
	}
	if res.IsError {
		t.Fatalf("refund_order: IsError=true, content=%q", res.Content)
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(res.Content), &m); err != nil {
		t.Fatalf("refund_order: content not valid JSON: %v", err)
	}
	if m["ok"] != true {
		t.Errorf("refund_order: ok = %v, want true", m["ok"])
	}
	if m["amount"] != 55.0 {
		t.Errorf("refund_order: amount = %v, want 55.0", m["amount"])
	}
	// Verify DB: status set to "refunded".
	if got, _ := db["orders"]["o_1003"]["status"].(string); got != "refunded" {
		t.Errorf("refund_order: order status = %q, want refunded", got)
	}
	// Verify refunds record: {orderId, reason, amount}.
	refund := db["refunds"]["o_1003"]
	if refund == nil {
		t.Fatal("refund_order: no refunds[o_1003] entry created")
	}
	if refund["orderId"] != "o_1003" {
		t.Errorf("refunds[o_1003].orderId = %v, want o_1003", refund["orderId"])
	}
	if refund["reason"] != "arrived broken" {
		t.Errorf("refunds[o_1003].reason = %v, want arrived broken", refund["reason"])
	}
	if refund["amount"] != 55.0 {
		t.Errorf("refunds[o_1003].amount = %v, want 55.0", refund["amount"])
	}
}

func TestRefundOrderRejectsProcessing(t *testing.T) {
	db := retailSeed()
	tools := newRetailTools(&db)
	tool := findTool(tools, "refund_order")

	args := json.RawMessage(`{"orderId":"o_1002","reason":"changed mind"}`)
	res, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("refund_order: unexpected error: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(res.Content), &m); err != nil {
		t.Fatalf("refund_order: content not valid JSON: %v", err)
	}
	if _, hasErr := m["error"]; !hasErr {
		t.Errorf("refund_order: expected error for processing order, got %v", m)
	}
	// DB must not have a refund record.
	if db["refunds"]["o_1002"] != nil {
		t.Errorf("refund_order: spurious refunds[o_1002] created")
	}
}

func TestRefundOrderRejectsShipped(t *testing.T) {
	db := retailSeed()
	tools := newRetailTools(&db)
	tool := findTool(tools, "refund_order")

	args := json.RawMessage(`{"orderId":"o_1001","reason":"wrong item"}`)
	res, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("refund_order: unexpected error: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(res.Content), &m); err != nil {
		t.Fatalf("refund_order: content not valid JSON: %v", err)
	}
	if _, hasErr := m["error"]; !hasErr {
		t.Errorf("refund_order: expected error for shipped order, got %v", m)
	}
}

func TestRefundOrderNotFound(t *testing.T) {
	db := retailSeed()
	tools := newRetailTools(&db)
	tool := findTool(tools, "refund_order")

	args := json.RawMessage(`{"orderId":"o_9999","reason":"test"}`)
	res, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("refund_order: unexpected error: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(res.Content), &m); err != nil {
		t.Fatalf("refund_order: content not valid JSON: %v", err)
	}
	if _, hasErr := m["error"]; !hasErr {
		t.Errorf("refund_order: expected error for missing order, got %v", m)
	}
}

// --- list_user_orders ---

func TestListUserOrdersAri(t *testing.T) {
	db := retailSeed()
	tools := newRetailTools(&db)
	tool := findTool(tools, "list_user_orders")

	res, err := tool.Execute(context.Background(), json.RawMessage(`{"userId":"u_ari"}`))
	if err != nil {
		t.Fatalf("list_user_orders: unexpected error: %v", err)
	}
	if res.IsError {
		t.Fatalf("list_user_orders: IsError=true, content=%q", res.Content)
	}
	var orders []map[string]any
	if err := json.Unmarshal([]byte(res.Content), &orders); err != nil {
		t.Fatalf("list_user_orders: content not valid JSON array: %v\ncontent: %s", err, res.Content)
	}
	// u_ari has o_1001 and o_1005.
	if len(orders) != 2 {
		t.Errorf("list_user_orders u_ari: got %d orders, want 2", len(orders))
	}
	ids := make(map[string]bool)
	for _, o := range orders {
		if id, ok := o["orderId"].(string); ok {
			ids[id] = true
		}
	}
	if !ids["o_1001"] || !ids["o_1005"] {
		t.Errorf("list_user_orders u_ari: expected o_1001 and o_1005, got %v", ids)
	}
}

func TestListUserOrdersBo(t *testing.T) {
	db := retailSeed()
	tools := newRetailTools(&db)
	tool := findTool(tools, "list_user_orders")

	res, err := tool.Execute(context.Background(), json.RawMessage(`{"userId":"u_bo"}`))
	if err != nil {
		t.Fatalf("list_user_orders: unexpected error: %v", err)
	}
	var orders []map[string]any
	if err := json.Unmarshal([]byte(res.Content), &orders); err != nil {
		t.Fatalf("list_user_orders: content not valid JSON array: %v", err)
	}
	// u_bo has only o_1002.
	if len(orders) != 1 {
		t.Errorf("list_user_orders u_bo: got %d orders, want 1", len(orders))
	}
}

func TestListUserOrdersNoOrders(t *testing.T) {
	db := retailSeed()
	tools := newRetailTools(&db)
	tool := findTool(tools, "list_user_orders")

	res, err := tool.Execute(context.Background(), json.RawMessage(`{"userId":"u_unknown"}`))
	if err != nil {
		t.Fatalf("list_user_orders: unexpected error: %v", err)
	}
	var orders []map[string]any
	if err := json.Unmarshal([]byte(res.Content), &orders); err != nil {
		t.Fatalf("list_user_orders: content not valid JSON array: %v", err)
	}
	if len(orders) != 0 {
		t.Errorf("list_user_orders unknown user: got %d orders, want 0", len(orders))
	}
}

// findTool is a helper to locate a named tool in a slice.
func findTool(tools []retailTool, name string) retailTool {
	for _, t := range tools {
		if t.Name() == name {
			return t
		}
	}
	panic("tool not found: " + name)
}
