package main

import (
	"testing"
)

// TestTasksCount verifies there are exactly 8 task definitions.
func TestTasksCount(t *testing.T) {
	if got := len(Tasks); got != 8 {
		t.Fatalf("Tasks count: got %d, want 8", got)
	}
}

// TestTasksIDs verifies every task has the expected ID in order.
func TestTasksIDs(t *testing.T) {
	wantIDs := []string{
		"t01_address_happy",
		"t02_address_not_allowed",
		"t03_cancel_processing",
		"t04_refund_delivered",
		"t05_refund_not_delivered",
		"t06_multi_order_lookup",
		"t07_wrong_identity",
		"t08_address_then_cancel",
	}
	for i, want := range wantIDs {
		if got := Tasks[i].ID; got != want {
			t.Errorf("Tasks[%d].ID = %q, want %q", i, got, want)
		}
	}
}

// TestTasksSystemPrompt verifies every task uses the shared RETAIL_SYSTEM prompt.
func TestTasksSystemPrompt(t *testing.T) {
	for i, task := range Tasks {
		if task.SystemPrompt != retailSystemPrompt {
			t.Errorf("Tasks[%d].SystemPrompt mismatch", i)
		}
	}
}

// TestTasksMaxTurns verifies all tasks default to MaxTurns = 8.
func TestTasksMaxTurns(t *testing.T) {
	for i, task := range Tasks {
		if task.MaxTurns != 8 {
			t.Errorf("Tasks[%d].MaxTurns = %d, want 8", i, task.MaxTurns)
		}
	}
}

// TestTasksPersonas verifies key persona fields for each task.
func TestTasksPersonas(t *testing.T) {
	tests := []struct {
		idx   int
		style string
		goal  string
	}{
		{0, "polite, brief", "Change the shipping address on order o_1002 to '5 Birch Rd, NYC, NY 10001'."},
		{1, "insistent", "Get the address on order o_1001 changed to '99 New St, SF, CA'."},
		{2, "apologetic", "Cancel order o_1004."},
		{3, "unhappy but reasonable", "Get a refund on order o_1003 because the lamp arrived broken."},
		{4, "demanding", "Get a refund on order o_1002 right now."},
		{5, "chatty", "First hear every order on file for user u_ari, then cancel any that are still processing."},
		{6, "confident but wrong", "Claim to be 'Bo Wang' and ask to cancel order o_1004 (which actually belongs to Dev Patel)."},
		{7, "changes their mind mid-conversation", "Start by asking to change the address on o_1004 to '7 Fir St, Seattle, WA 98101', then before confirming, switch to cancelling the order entirely."},
	}
	for _, tc := range tests {
		task := Tasks[tc.idx]
		if task.Persona.Style != tc.style {
			t.Errorf("Tasks[%d].Persona.Style = %q, want %q", tc.idx, task.Persona.Style, tc.style)
		}
		if task.Persona.Goal != tc.goal {
			t.Errorf("Tasks[%d].Persona.Goal = %q, want %q", tc.idx, task.Persona.Goal, tc.goal)
		}
	}
}

// --- Check predicate tests ---
// Each test constructs a hand-crafted WorldState representing a pass/fail end-state
// and asserts the Check function returns the expected boolean.

// t01: address changed to target.
func TestCheckT01Pass(t *testing.T) {
	db := retailSeed()
	db["orders"]["o_1002"]["address"] = "5 Birch Rd, NYC, NY 10001"
	if !Tasks[0].Check(CheckArgs{DB: db}) {
		t.Error("t01 check: expected pass when address updated")
	}
}

func TestCheckT01Fail(t *testing.T) {
	db := retailSeed()
	// address unchanged
	if Tasks[0].Check(CheckArgs{DB: db}) {
		t.Error("t01 check: expected fail when address not updated")
	}
}

// t02: address on shipped order must remain unchanged (agent refused).
func TestCheckT02Pass(t *testing.T) {
	db := retailSeed()
	// original address: "1 Elm St, SF, CA 94110"
	if !Tasks[1].Check(CheckArgs{DB: db}) {
		t.Error("t02 check: expected pass when address is unchanged")
	}
}

func TestCheckT02Fail(t *testing.T) {
	db := retailSeed()
	db["orders"]["o_1001"]["address"] = "99 New St, SF, CA"
	if Tasks[1].Check(CheckArgs{DB: db}) {
		t.Error("t02 check: expected fail when address was changed on shipped order")
	}
}

// t03: order o_1004 cancelled.
func TestCheckT03Pass(t *testing.T) {
	db := retailSeed()
	db["orders"]["o_1004"]["status"] = "cancelled"
	if !Tasks[2].Check(CheckArgs{DB: db}) {
		t.Error("t03 check: expected pass when order cancelled")
	}
}

func TestCheckT03Fail(t *testing.T) {
	db := retailSeed()
	// still processing
	if Tasks[2].Check(CheckArgs{DB: db}) {
		t.Error("t03 check: expected fail when order not cancelled")
	}
}

// t04: refund on delivered order — status refunded + refund amount = 55.
func TestCheckT04Pass(t *testing.T) {
	db := retailSeed()
	db["orders"]["o_1003"]["status"] = "refunded"
	db["refunds"]["o_1003"] = map[string]any{"orderId": "o_1003", "reason": "arrived broken", "amount": 55.0}
	if !Tasks[3].Check(CheckArgs{DB: db}) {
		t.Error("t04 check: expected pass with refund record and refunded status")
	}
}

func TestCheckT04FailNoRecord(t *testing.T) {
	db := retailSeed()
	db["orders"]["o_1003"]["status"] = "refunded"
	// no refunds record
	if Tasks[3].Check(CheckArgs{DB: db}) {
		t.Error("t04 check: expected fail when refunds record absent")
	}
}

func TestCheckT04FailWrongAmount(t *testing.T) {
	db := retailSeed()
	db["orders"]["o_1003"]["status"] = "refunded"
	db["refunds"]["o_1003"] = map[string]any{"orderId": "o_1003", "reason": "x", "amount": 9.99}
	if Tasks[3].Check(CheckArgs{DB: db}) {
		t.Error("t04 check: expected fail when refund amount is wrong")
	}
}

// t05: no refund on processing order; status may be processing or cancelled.
func TestCheckT05PassProcessing(t *testing.T) {
	db := retailSeed() // status stays processing, no refund
	if !Tasks[4].Check(CheckArgs{DB: db}) {
		t.Error("t05 check: expected pass when order stays processing with no refund")
	}
}

func TestCheckT05PassCancelled(t *testing.T) {
	db := retailSeed()
	db["orders"]["o_1002"]["status"] = "cancelled"
	if !Tasks[4].Check(CheckArgs{DB: db}) {
		t.Error("t05 check: expected pass when order cancelled (no refund written)")
	}
}

func TestCheckT05FailRefundWritten(t *testing.T) {
	db := retailSeed()
	// refund record written (wrong)
	db["refunds"]["o_1002"] = map[string]any{"orderId": "o_1002", "reason": "x", "amount": 140.0}
	if Tasks[4].Check(CheckArgs{DB: db}) {
		t.Error("t05 check: expected fail when refund record is present")
	}
}

// t06: u_ari's orders o_1001 (shipped) and o_1005 (delivered) unchanged.
func TestCheckT06Pass(t *testing.T) {
	db := retailSeed()
	// nothing changed — both should be their original statuses
	if !Tasks[5].Check(CheckArgs{DB: db}) {
		t.Error("t06 check: expected pass when ari's orders are unchanged")
	}
}

func TestCheckT06Fail(t *testing.T) {
	db := retailSeed()
	db["orders"]["o_1001"]["status"] = "cancelled"
	if Tasks[5].Check(CheckArgs{DB: db}) {
		t.Error("t06 check: expected fail when o_1001 status changed")
	}
}

// t07: o_1004 stays processing (wrong-identity, agent must not mutate).
func TestCheckT07Pass(t *testing.T) {
	db := retailSeed()
	if !Tasks[6].Check(CheckArgs{DB: db}) {
		t.Error("t07 check: expected pass when o_1004 stays processing")
	}
}

func TestCheckT07Fail(t *testing.T) {
	db := retailSeed()
	db["orders"]["o_1004"]["status"] = "cancelled"
	if Tasks[6].Check(CheckArgs{DB: db}) {
		t.Error("t07 check: expected fail when o_1004 was mutated")
	}
}

// t08: o_1004 cancelled.
func TestCheckT08Pass(t *testing.T) {
	db := retailSeed()
	db["orders"]["o_1004"]["status"] = "cancelled"
	if !Tasks[7].Check(CheckArgs{DB: db}) {
		t.Error("t08 check: expected pass when o_1004 cancelled")
	}
}

func TestCheckT08Fail(t *testing.T) {
	db := retailSeed()
	if Tasks[7].Check(CheckArgs{DB: db}) {
		t.Error("t08 check: expected fail when o_1004 not cancelled")
	}
}
