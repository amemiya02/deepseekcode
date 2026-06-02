// Package taubench — retail tool implementations over *WorldState.
// Schemas are byte-equal to the TypeScript originals in
// benchmarks/tau-bench/tasks.ts (ToolFactory definitions).
package taubench

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/amemiya02/deepseekcode/internal/tools"
)

// retailTool is the internal/tools.Tool interface, re-aliased here for
// clarity in test helper signatures.
type retailTool = tools.Tool

// newRetailTools returns the 6 retail tools bound to db.
// Callers should pass a pointer to a cloned WorldState so mutations
// stay isolated to that task run.
func newRetailTools(db *WorldState) []retailTool {
	return []retailTool{
		&lookupOrderTool{db: db},
		&lookupUserTool{db: db},
		&updateAddressTool{db: db},
		&cancelOrderTool{db: db},
		&refundOrderTool{db: db},
		&listUserOrdersTool{db: db},
	}
}

// jsonResult marshals v to a JSON string and wraps it in a tools.Result.
// Panics are impossible in practice because v is always a map[string]any
// or []any built from primitive values.
func jsonResult(v any) tools.Result {
	b, err := json.Marshal(v)
	if err != nil {
		return tools.Result{Content: fmt.Sprintf(`{"error":"marshal failed: %v"}`, err), IsError: true}
	}
	return tools.Result{Content: string(b)}
}

// --- lookup_order ---

type lookupOrderTool struct{ db *WorldState }

func (t *lookupOrderTool) Name() string { return "lookup_order" }
func (t *lookupOrderTool) Description() string {
	return "Look up an order by id. Returns { userId, status, address, item, price } or null."
}
func (t *lookupOrderTool) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"orderId":{"type":"string"}},"required":["orderId"]}`)
}
func (t *lookupOrderTool) Execute(_ context.Context, args json.RawMessage) (tools.Result, error) {
	var p struct {
		OrderID string `json:"orderId"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return tools.Result{}, err
	}
	row := getRow(*t.db, "orders", p.OrderID)
	if row == nil {
		return jsonResult(map[string]any{"error": "order not found"}), nil
	}
	out := map[string]any{"orderId": p.OrderID}
	for k, v := range row {
		out[k] = v
	}
	return jsonResult(out), nil
}

// --- lookup_user ---

type lookupUserTool struct{ db *WorldState }

func (t *lookupUserTool) Name() string { return "lookup_user" }
func (t *lookupUserTool) Description() string {
	return "Look up a user by id. Returns { name, email } or error."
}
func (t *lookupUserTool) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"userId":{"type":"string"}},"required":["userId"]}`)
}
func (t *lookupUserTool) Execute(_ context.Context, args json.RawMessage) (tools.Result, error) {
	var p struct {
		UserID string `json:"userId"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return tools.Result{}, err
	}
	row := getRow(*t.db, "users", p.UserID)
	if row == nil {
		return jsonResult(map[string]any{"error": "user not found"}), nil
	}
	out := map[string]any{"userId": p.UserID}
	for k, v := range row {
		out[k] = v
	}
	return jsonResult(out), nil
}

// --- update_address ---

type updateAddressTool struct{ db *WorldState }

func (t *updateAddressTool) Name() string { return "update_address" }
func (t *updateAddressTool) Description() string {
	return "Update the shipping address on an order. Only allowed if status is 'processing'. Returns ok/error."
}
func (t *updateAddressTool) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"orderId":{"type":"string"},"address":{"type":"string"}},"required":["orderId","address"]}`)
}
func (t *updateAddressTool) Execute(_ context.Context, args json.RawMessage) (tools.Result, error) {
	var p struct {
		OrderID string `json:"orderId"`
		Address string `json:"address"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return tools.Result{}, err
	}
	row := getRow(*t.db, "orders", p.OrderID)
	if row == nil {
		return jsonResult(map[string]any{"error": "order not found"}), nil
	}
	status, _ := row["status"].(string)
	if status != "processing" {
		return jsonResult(map[string]any{"error": fmt.Sprintf("cannot edit: status=%s", status)}), nil
	}
	setField(*t.db, "orders", p.OrderID, "address", p.Address)
	return jsonResult(map[string]any{"ok": true, "orderId": p.OrderID, "newAddress": p.Address}), nil
}

// --- cancel_order ---

type cancelOrderTool struct{ db *WorldState }

func (t *cancelOrderTool) Name() string { return "cancel_order" }
func (t *cancelOrderTool) Description() string {
	return "Cancel an order. Only allowed if status is 'processing'. Returns ok/error."
}
func (t *cancelOrderTool) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"orderId":{"type":"string"}},"required":["orderId"]}`)
}
func (t *cancelOrderTool) Execute(_ context.Context, args json.RawMessage) (tools.Result, error) {
	var p struct {
		OrderID string `json:"orderId"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return tools.Result{}, err
	}
	row := getRow(*t.db, "orders", p.OrderID)
	if row == nil {
		return jsonResult(map[string]any{"error": "order not found"}), nil
	}
	status, _ := row["status"].(string)
	if status != "processing" {
		return jsonResult(map[string]any{"error": fmt.Sprintf("cannot cancel: status=%s", status)}), nil
	}
	setField(*t.db, "orders", p.OrderID, "status", "cancelled")
	return jsonResult(map[string]any{"ok": true, "orderId": p.OrderID, "status": "cancelled"}), nil
}

// --- refund_order ---

type refundOrderTool struct{ db *WorldState }

func (t *refundOrderTool) Name() string { return "refund_order" }
func (t *refundOrderTool) Description() string {
	return "Issue a refund on a delivered order. Records an entry in refunds. Returns ok/error."
}
func (t *refundOrderTool) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"orderId":{"type":"string"},"reason":{"type":"string"}},"required":["orderId","reason"]}`)
}
func (t *refundOrderTool) Execute(_ context.Context, args json.RawMessage) (tools.Result, error) {
	var p struct {
		OrderID string `json:"orderId"`
		Reason  string `json:"reason"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return tools.Result{}, err
	}
	row := getRow(*t.db, "orders", p.OrderID)
	if row == nil {
		return jsonResult(map[string]any{"error": "order not found"}), nil
	}
	status, _ := row["status"].(string)
	if status != "delivered" {
		return jsonResult(map[string]any{"error": fmt.Sprintf("cannot refund: status=%s", status)}), nil
	}
	price := row["price"]
	// Write refund record: {orderId, reason, amount: price}
	if (*t.db)["refunds"] == nil {
		(*t.db)["refunds"] = make(map[string]map[string]any)
	}
	(*t.db)["refunds"][p.OrderID] = map[string]any{
		"orderId": p.OrderID,
		"reason":  p.Reason,
		"amount":  price,
	}
	setField(*t.db, "orders", p.OrderID, "status", "refunded")
	return jsonResult(map[string]any{"ok": true, "orderId": p.OrderID, "amount": price}), nil
}

// --- list_user_orders ---

type listUserOrdersTool struct{ db *WorldState }

func (t *listUserOrdersTool) Name() string { return "list_user_orders" }
func (t *listUserOrdersTool) Description() string {
	return "List every order belonging to a userId. Returns an array of orders."
}
func (t *listUserOrdersTool) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"userId":{"type":"string"}},"required":["userId"]}`)
}
func (t *listUserOrdersTool) Execute(_ context.Context, args json.RawMessage) (tools.Result, error) {
	var p struct {
		UserID string `json:"userId"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return tools.Result{}, err
	}
	all := (*t.db)["orders"]
	out := make([]any, 0)
	for orderID, row := range all {
		if uid, _ := row["userId"].(string); uid == p.UserID {
			entry := map[string]any{"orderId": orderID}
			for k, v := range row {
				entry[k] = v
			}
			out = append(out, entry)
		}
	}
	return jsonResult(out), nil
}
