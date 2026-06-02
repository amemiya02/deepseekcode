package main

import (
	"testing"
)

// TestRetailSeedUsers verifies the 4 users seeded verbatim from db.ts / tasks.ts.
func TestRetailSeedUsers(t *testing.T) {
	db := retailSeed()

	users, ok := db["users"]
	if !ok {
		t.Fatal("retailSeed: missing 'users' table")
	}
	wantUsers := map[string]map[string]any{
		"u_ari": {"name": "Ari Chen", "email": "ari@example.com"},
		"u_bo":  {"name": "Bo Wang", "email": "bo@example.com"},
		"u_cai": {"name": "Cai Lin", "email": "cai@example.com"},
		"u_dev": {"name": "Dev Patel", "email": "dev@example.com"},
	}
	if len(users) != len(wantUsers) {
		t.Fatalf("users count: got %d, want %d", len(users), len(wantUsers))
	}
	for id, want := range wantUsers {
		row, ok := users[id]
		if !ok {
			t.Errorf("missing user %q", id)
			continue
		}
		for field, wantVal := range want {
			if got := row[field]; got != wantVal {
				t.Errorf("users[%q][%q] = %q, want %q", id, field, got, wantVal)
			}
		}
	}
}

// TestRetailSeedOrders verifies the 5 orders seeded verbatim from tasks.ts retailSeed().
func TestRetailSeedOrders(t *testing.T) {
	db := retailSeed()

	orders, ok := db["orders"]
	if !ok {
		t.Fatal("retailSeed: missing 'orders' table")
	}

	type orderWant struct {
		userId  string
		status  string
		address string
		item    string
		price   float64
	}
	wantOrders := map[string]orderWant{
		"o_1001": {"u_ari", "shipped", "1 Elm St, SF, CA 94110", "wool sweater M", 89.0},
		"o_1002": {"u_bo", "processing", "22 Oak Rd, NYC, NY 10001", "running shoes 10", 140.0},
		"o_1003": {"u_cai", "delivered", "9 Pine Ave, Austin, TX 78701", "desk lamp", 55.0},
		"o_1004": {"u_dev", "processing", "4 Maple Ln, Seattle, WA 98101", "kettle", 45.0},
		"o_1005": {"u_ari", "delivered", "1 Elm St, SF, CA 94110", "notebook pack", 22.0},
	}
	if len(orders) != len(wantOrders) {
		t.Fatalf("orders count: got %d, want %d", len(orders), len(wantOrders))
	}
	for id, want := range wantOrders {
		row, ok := orders[id]
		if !ok {
			t.Errorf("missing order %q", id)
			continue
		}
		if got, _ := row["userId"].(string); got != want.userId {
			t.Errorf("orders[%q].userId = %q, want %q", id, got, want.userId)
		}
		if got, _ := row["status"].(string); got != want.status {
			t.Errorf("orders[%q].status = %q, want %q", id, got, want.status)
		}
		if got, _ := row["address"].(string); got != want.address {
			t.Errorf("orders[%q].address = %q, want %q", id, got, want.address)
		}
		if got, _ := row["item"].(string); got != want.item {
			t.Errorf("orders[%q].item = %q, want %q", id, got, want.item)
		}
		if got, _ := row["price"].(float64); got != want.price {
			t.Errorf("orders[%q].price = %v, want %v", id, got, want.price)
		}
	}
}

// TestRetailSeedRefundsEmpty verifies the refunds table starts empty.
func TestRetailSeedRefundsEmpty(t *testing.T) {
	db := retailSeed()
	refunds, ok := db["refunds"]
	if !ok {
		t.Fatal("retailSeed: missing 'refunds' table")
	}
	if len(refunds) != 0 {
		t.Fatalf("refunds should be empty, got %d entries", len(refunds))
	}
}

// TestCloneDbIsolation verifies cloneDb deep-copies: mutating clone doesn't affect original.
func TestCloneDbIsolation(t *testing.T) {
	orig := retailSeed()
	clone := cloneDb(orig)

	// Mutate the clone.
	clone["orders"]["o_1001"]["status"] = "mutated"

	// Original must be unchanged.
	if got, _ := orig["orders"]["o_1001"]["status"].(string); got != "shipped" {
		t.Errorf("cloneDb: mutation leaked into original; orig status = %q", got)
	}

	// Clone reflects the mutation.
	if got, _ := clone["orders"]["o_1001"]["status"].(string); got != "mutated" {
		t.Errorf("cloneDb: clone did not retain mutation; got %q", got)
	}
}

// TestCloneDbIdempotent verifies N calls to cloneDb on the same input yield identical results.
func TestCloneDbIdempotent(t *testing.T) {
	orig := retailSeed()
	c1 := cloneDb(orig)
	c2 := cloneDb(orig)

	// Both clones should have same data.
	for table, rows := range orig {
		for id, row := range rows {
			for field, wantVal := range row {
				if got := c1[table][id][field]; got != wantVal {
					t.Errorf("c1[%q][%q][%q] = %v, want %v", table, id, field, got, wantVal)
				}
				if got := c2[table][id][field]; got != wantVal {
					t.Errorf("c2[%q][%q][%q] = %v, want %v", table, id, field, got, wantVal)
				}
			}
		}
	}
}

// TestGetRowFound verifies getRow returns the row when it exists.
func TestGetRowFound(t *testing.T) {
	db := retailSeed()
	row := getRow(db, "users", "u_bo")
	if row == nil {
		t.Fatal("getRow: expected row for u_bo, got nil")
	}
	if got, _ := row["name"].(string); got != "Bo Wang" {
		t.Errorf("getRow users/u_bo name = %q, want %q", got, "Bo Wang")
	}
}

// TestGetRowMissing verifies getRow returns nil for missing row.
func TestGetRowMissing(t *testing.T) {
	db := retailSeed()
	row := getRow(db, "users", "u_nobody")
	if row != nil {
		t.Errorf("getRow: expected nil for missing user, got %v", row)
	}
}

// TestGetRowMissingTable verifies getRow returns nil for missing table.
func TestGetRowMissingTable(t *testing.T) {
	db := retailSeed()
	row := getRow(db, "nonexistent", "o_1001")
	if row != nil {
		t.Errorf("getRow: expected nil for missing table, got %v", row)
	}
}

// TestSetFieldFound verifies setField updates the field and returns true.
func TestSetFieldFound(t *testing.T) {
	db := retailSeed()
	ok := setField(db, "orders", "o_1002", "address", "99 New St")
	if !ok {
		t.Fatal("setField: expected true for existing row")
	}
	if got, _ := db["orders"]["o_1002"]["address"].(string); got != "99 New St" {
		t.Errorf("setField: address = %q, want %q", got, "99 New St")
	}
}

// TestSetFieldMissing verifies setField returns false when row doesn't exist.
func TestSetFieldMissing(t *testing.T) {
	db := retailSeed()
	ok := setField(db, "orders", "o_9999", "status", "cancelled")
	if ok {
		t.Fatal("setField: expected false for missing row")
	}
}

// TestSetFieldMissingTable verifies setField returns false when table doesn't exist.
func TestSetFieldMissingTable(t *testing.T) {
	db := retailSeed()
	ok := setField(db, "nonexistent", "o_1001", "field", "val")
	if ok {
		t.Fatal("setField: expected false for missing table")
	}
}
