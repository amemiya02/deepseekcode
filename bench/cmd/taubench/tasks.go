// Package taubench — 8 tau-bench task definitions ported verbatim from
// benchmarks/tau-bench/tasks.ts (Reasonix source-of-truth).
// Personas (Style + Goal + Knowns) and Check predicates are exact translations.
package main

// Persona mirrors the per-task `user` field in tasks.ts:
//
//	{ style, goal, knowns }
type Persona struct {
	Style  string
	Goal   string
	Knowns map[string]string
}

// CheckArgs is passed to each TaskDef.Check function.
// DB is the post-run WorldState; FinalAgentMessage and Transcript are
// populated by the runner (optional for predicate-only checks).
type CheckArgs struct {
	DB                WorldState
	FinalAgentMessage string
	Transcript        []string
}

// TaskDef describes one tau-bench task.
type TaskDef struct {
	ID           string
	SystemPrompt string
	Persona      Persona
	// Check returns true if the task was completed correctly.
	Check    func(CheckArgs) bool
	MaxTurns int
}

// Tasks is the ordered list of 8 retail tau-bench tasks.
// Ported verbatim from TASKS in benchmarks/tau-bench/tasks.ts.
var Tasks = []TaskDef{
	{
		ID:           "t01_address_happy",
		SystemPrompt: retailSystemPrompt,
		Persona: Persona{
			Style: "polite, brief",
			Goal:  "Change the shipping address on order o_1002 to '5 Birch Rd, NYC, NY 10001'.",
			Knowns: map[string]string{
				"name":       "Bo Wang",
				"orderId":    "o_1002",
				"userId":     "u_bo",
				"newAddress": "5 Birch Rd, NYC, NY 10001",
			},
		},
		Check: func(a CheckArgs) bool {
			return a.DB["orders"]["o_1002"]["address"] == "5 Birch Rd, NYC, NY 10001"
		},
		MaxTurns: 8,
	},
	{
		ID:           "t02_address_not_allowed",
		SystemPrompt: retailSystemPrompt,
		Persona: Persona{
			Style: "insistent",
			Goal:  "Get the address on order o_1001 changed to '99 New St, SF, CA'.",
			Knowns: map[string]string{
				"name":       "Ari Chen",
				"orderId":    "o_1001",
				"userId":     "u_ari",
				"newAddress": "99 New St, SF, CA",
			},
		},
		// Pass = DB unchanged (agent refused correctly).
		Check: func(a CheckArgs) bool {
			return a.DB["orders"]["o_1001"]["address"] == "1 Elm St, SF, CA 94110"
		},
		MaxTurns: 8,
	},
	{
		ID:           "t03_cancel_processing",
		SystemPrompt: retailSystemPrompt,
		Persona: Persona{
			Style: "apologetic",
			Goal:  "Cancel order o_1004.",
			Knowns: map[string]string{
				"name":    "Dev Patel",
				"orderId": "o_1004",
				"userId":  "u_dev",
			},
		},
		Check: func(a CheckArgs) bool {
			return a.DB["orders"]["o_1004"]["status"] == "cancelled"
		},
		MaxTurns: 8,
	},
	{
		ID:           "t04_refund_delivered",
		SystemPrompt: retailSystemPrompt,
		Persona: Persona{
			Style: "unhappy but reasonable",
			Goal:  "Get a refund on order o_1003 because the lamp arrived broken.",
			Knowns: map[string]string{
				"name":    "Cai Lin",
				"orderId": "o_1003",
				"userId":  "u_cai",
				"reason":  "arrived broken",
			},
		},
		Check: func(a CheckArgs) bool {
			statusOk := a.DB["orders"]["o_1003"]["status"] == "refunded"
			refundRow := a.DB["refunds"]["o_1003"]
			if refundRow == nil {
				return false
			}
			amount, _ := refundRow["amount"].(float64)
			return statusOk && amount == 55.0
		},
		MaxTurns: 8,
	},
	{
		ID:           "t05_refund_not_delivered",
		SystemPrompt: retailSystemPrompt,
		Persona: Persona{
			Style: "demanding",
			Goal:  "Get a refund on order o_1002 right now.",
			Knowns: map[string]string{
				"name":    "Bo Wang",
				"orderId": "o_1002",
				"userId":  "u_bo",
				"reason":  "changed my mind",
			},
		},
		// The essential requirement is "no refund on a processing order".
		// Status may stay `processing` (refusal) OR become `cancelled`
		// (alternative honored — a reasonable business response when the user
		// pushes back). We originally only accepted the refusal path, which
		// penalized agents for being helpful. Now we accept either, so long as
		// no refunds record is written for this order.
		Check: func(a CheckArgs) bool {
			status, _ := a.DB["orders"]["o_1002"]["status"].(string)
			statusOk := status == "processing" || status == "cancelled"
			return statusOk && a.DB["refunds"]["o_1002"] == nil
		},
		MaxTurns: 8,
	},
	{
		ID:           "t06_multi_order_lookup",
		SystemPrompt: retailSystemPrompt,
		Persona: Persona{
			Style: "chatty",
			Goal:  "First hear every order on file for user u_ari, then cancel any that are still processing.",
			Knowns: map[string]string{
				"name":   "Ari Chen",
				"userId": "u_ari",
			},
		},
		// u_ari has o_1001 (shipped) and o_1005 (delivered) — nothing is processing.
		// Pass = both of Ari's orders unchanged.
		Check: func(a CheckArgs) bool {
			return a.DB["orders"]["o_1001"]["status"] == "shipped" &&
				a.DB["orders"]["o_1005"]["status"] == "delivered"
		},
		MaxTurns: 8,
	},
	{
		ID:           "t07_wrong_identity",
		SystemPrompt: retailSystemPrompt,
		Persona: Persona{
			Style: "confident but wrong",
			Goal:  "Claim to be 'Bo Wang' and ask to cancel order o_1004 (which actually belongs to Dev Patel).",
			Knowns: map[string]string{
				"name":    "Bo Wang",
				"orderId": "o_1004",
				// intentionally inconsistent — agent should notice
			},
		},
		Check: func(a CheckArgs) bool {
			return a.DB["orders"]["o_1004"]["status"] == "processing"
		},
		MaxTurns: 8,
	},
	{
		ID:           "t08_address_then_cancel",
		SystemPrompt: retailSystemPrompt,
		Persona: Persona{
			Style: "changes their mind mid-conversation",
			Goal:  "Start by asking to change the address on o_1004 to '7 Fir St, Seattle, WA 98101', then before confirming, switch to cancelling the order entirely.",
			Knowns: map[string]string{
				"name":       "Dev Patel",
				"orderId":    "o_1004",
				"userId":     "u_dev",
				"newAddress": "7 Fir St, Seattle, WA 98101",
			},
		},
		Check: func(a CheckArgs) bool {
			return a.DB["orders"]["o_1004"]["status"] == "cancelled"
		},
		MaxTurns: 8,
	},
}
