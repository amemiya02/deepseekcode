// Package taubench is a Go port of the Reasonix tau-bench harness.
// WorldState + retailSeed + DB helpers are ported verbatim from
// benchmarks/tau-bench/db.ts and benchmarks/tau-bench/tasks.ts.
package taubench

import "encoding/json"

// WorldState is a nested map: table → id → field → value.
// Shape is JSON-compatible by contract (all values are primitive or nested maps).
type WorldState map[string]map[string]map[string]any

// cloneDb deep-copies a WorldState via JSON round-trip so mutations to the
// clone never touch the original. Idempotent: N calls on the same input yield
// identical copies.
func cloneDb(db WorldState) WorldState {
	b, err := json.Marshal(db)
	if err != nil {
		panic("cloneDb: marshal: " + err.Error())
	}
	var clone WorldState
	if err := json.Unmarshal(b, &clone); err != nil {
		panic("cloneDb: unmarshal: " + err.Error())
	}
	return clone
}

// getRow returns db[table][id] or nil if either key is absent.
func getRow(db WorldState, table, id string) map[string]any {
	rows, ok := db[table]
	if !ok {
		return nil
	}
	row, ok := rows[id]
	if !ok {
		return nil
	}
	return row
}

// setField sets db[table][id][field] = value in-place. Returns true if the
// row was found and updated, false if the table or row is absent.
func setField(db WorldState, table, id, field string, value any) bool {
	row := getRow(db, table, id)
	if row == nil {
		return false
	}
	row[field] = value
	return true
}

// retailSeed returns the canonical starting WorldState for all 8 retail tasks.
// Values are copied verbatim from benchmarks/tau-bench/tasks.ts retailSeed().
func retailSeed() WorldState {
	return WorldState{
		"users": {
			"u_ari": {"name": "Ari Chen", "email": "ari@example.com"},
			"u_bo":  {"name": "Bo Wang", "email": "bo@example.com"},
			"u_cai": {"name": "Cai Lin", "email": "cai@example.com"},
			"u_dev": {"name": "Dev Patel", "email": "dev@example.com"},
		},
		"orders": {
			"o_1001": {
				"userId":  "u_ari",
				"status":  "shipped",
				"address": "1 Elm St, SF, CA 94110",
				"item":    "wool sweater M",
				"price":   89.0,
			},
			"o_1002": {
				"userId":  "u_bo",
				"status":  "processing",
				"address": "22 Oak Rd, NYC, NY 10001",
				"item":    "running shoes 10",
				"price":   140.0,
			},
			"o_1003": {
				"userId":  "u_cai",
				"status":  "delivered",
				"address": "9 Pine Ave, Austin, TX 78701",
				"item":    "desk lamp",
				"price":   55.0,
			},
			"o_1004": {
				"userId":  "u_dev",
				"status":  "processing",
				"address": "4 Maple Ln, Seattle, WA 98101",
				"item":    "kettle",
				"price":   45.0,
			},
			"o_1005": {
				"userId":  "u_ari",
				"status":  "delivered",
				"address": "1 Elm St, SF, CA 94110",
				"item":    "notebook pack",
				"price":   22.0,
			},
		},
		"refunds": {},
	}
}
