// Package permissions implements the tiered approval model described in
// docs/design.md §8.
package permissions

import (
	"encoding/json"
	"fmt"
	"log"
	"path/filepath"
	"regexp"
)

// PermissionRule matches a tool name (exact, glob, or "*") and optionally
// its JSON arguments (regex). The first matching rule in Deny→Ask→Allow
// order wins.
type PermissionRule struct {
	ToolPattern string // exact name, "*", or shell glob (filepath.Match)
	ArgsPattern string // regex on JSON string; empty matches any
	Decision    string // "allow" | "deny" | "ask"
}

// RuleEngine holds ordered rule lists. Evaluate checks Deny first, then
// Ask, then Allow, returning the first match.
type RuleEngine struct {
	Allow []PermissionRule
	Deny  []PermissionRule
	Ask   []PermissionRule
}

// Evaluate returns the first matching rule's decision and a human-readable
// reason. If no rule matches, returns ("", "").
func (e *RuleEngine) Evaluate(toolName string, argsJSON json.RawMessage) (decision, reason string) {
	if e == nil {
		return "", ""
	}

	check := func(rules []PermissionRule) (string, string) {
		for _, r := range rules {
			if !matchTool(r.ToolPattern, toolName) {
				continue
			}
			if !matchArgs(r.ArgsPattern, argsJSON) {
				continue
			}
			desc := formatRuleReason(r)
			return r.Decision, desc
		}
		return "", ""
	}

	if d, reason := check(e.Deny); d != "" {
		return d, reason
	}
	if d, reason := check(e.Ask); d != "" {
		return d, reason
	}
	if d, reason := check(e.Allow); d != "" {
		return d, reason
	}
	return "", ""
}

func matchTool(pattern, toolName string) bool {
	if pattern == "*" {
		return true
	}
	ok, err := filepath.Match(pattern, toolName)
	if err != nil {
		return false
	}
	return ok
}

func matchArgs(pattern string, argsJSON json.RawMessage) bool {
	if pattern == "" {
		return true
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		log.Printf("permissions: invalid ArgsPattern regex %q: %v", pattern, err)
		return false
	}
	return re.Match(argsJSON)
}

func formatRuleReason(r PermissionRule) string {
	argsNote := ""
	if r.ArgsPattern != "" {
		argsNote = fmt.Sprintf(" (args: %s)", r.ArgsPattern)
	}
	return fmt.Sprintf("matched %s rule: %s%s", r.Decision, r.ToolPattern, argsNote)
}
