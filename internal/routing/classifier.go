// internal/routing/classifier.go
// Package routing decides, per turn and without an LLM call, which model and
// reasoning effort to use. It never touches the Static Prefix (system+tools),
// so routing can vary model/effort turn-to-turn without moving the DeepSeek
// cache key. Decisions are sticky to avoid flash<->pro flapping.
package routing

import "strings"

// Config holds the model ids and stickiness.
type Config struct {
	FlashModel  string
	ProModel    string
	StickyTurns int // how many turns to stay on pro after an escalation
}

// Signals are the per-turn inputs to the classifier.
type Signals struct {
	UserText             string
	RepairErrorsLastTurn int
}

// Decision is the per-turn routing verdict. StickyLeft carries forward how many
// more turns to remain on the escalated model.
type Decision struct {
	Model      string
	Thinking   bool
	Effort     string // "", "low", "medium", "high", "max"
	StickyLeft int
	Reason     string
}

// hardWords signal genuine reasoning/ambiguity worth pro + max effort.
var hardWords = []string{"why", "design", "architect", "refactor", "debug",
	"prove", "root cause", "race", "deadlock", "redesign", "trade-off", "tradeoff"}

// Classify returns the routing decision for this turn given the previous one.
func Classify(s Signals, cfg Config, prev Decision) Decision {
	// 1. Honor stickiness: stay on the escalated model until it decays.
	if prev.Model == cfg.ProModel && prev.StickyLeft > 0 {
		return Decision{Model: cfg.ProModel, Thinking: true, Effort: "max",
			StickyLeft: prev.StickyLeft - 1, Reason: "sticky"}
	}

	// 2. Repeated repair errors → escalate and re-arm stickiness.
	if s.RepairErrorsLastTurn >= 3 {
		return Decision{Model: cfg.ProModel, Thinking: true, Effort: "max",
			StickyLeft: cfg.StickyTurns, Reason: "repair_errors"}
	}

	text := strings.ToLower(s.UserText)
	hard := len(s.UserText) > 240
	for _, w := range hardWords {
		if strings.Contains(text, w) {
			hard = true
			break
		}
	}
	if hard {
		return Decision{Model: cfg.ProModel, Thinking: true, Effort: "max",
			StickyLeft: cfg.StickyTurns, Reason: "hard_reasoning"}
	}

	// 3. Default: mechanical/short turn → flash, no thinking.
	return Decision{Model: cfg.FlashModel, Thinking: false, Effort: "", Reason: "mechanical"}
}
