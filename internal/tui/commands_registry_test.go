package tui

import (
	"sort"
	"testing"

	"github.com/amemiya02/deepseekcode/internal/commands"
	"github.com/amemiya02/deepseekcode/internal/skills"
)

// TestBuiltinCommandsMirrorHandleSlash pins the built-in set to the names
// dispatched in handleSlash / listed in helpText, each with a non-empty
// summary and the expected aliases.
func TestBuiltinCommandsMirrorHandleSlash(t *testing.T) {
	got := builtinCommands()
	byName := make(map[string]slashCmd, len(got))
	for _, c := range got {
		if c.Kind != builtinCmd {
			t.Errorf("builtin %q has Kind %v, want builtinCmd", c.Name, c.Kind)
		}
		if c.Summary == "" {
			t.Errorf("builtin %q has empty Summary", c.Name)
		}
		byName[c.Name] = c
	}

	wantNames := []string{
		"help", "clear", "quit", "models", "effort", "cost", "theme", "tape",
		"sessions", "export", "undo", "compact", "reload-skills", "mcp", "lsp",
		"permissions",
	}
	for _, name := range wantNames {
		if _, ok := byName[name]; !ok {
			t.Errorf("builtin command %q missing", name)
		}
	}
	if len(got) != len(wantNames) {
		t.Errorf("builtin count = %d, want %d (%v)", len(got), len(wantNames), got)
	}

	wantAliases := map[string][]string{
		"quit":          {"exit", "q"},
		"export":        {"scrollback"},
		"reload-skills": {"reload"},
	}
	for name, aliases := range wantAliases {
		if !equalStrings(byName[name].Aliases, aliases) {
			t.Errorf("%q aliases = %v, want %v", name, byName[name].Aliases, aliases)
		}
	}
}

// TestAllCommandsMergesSources checks that custom commands and skills join
// the built-ins with the right Kind and Summary, and that the result is
// sorted by Name.
func TestAllCommandsMergesSources(t *testing.T) {
	custom := map[string]commands.Command{
		"deploy": {Name: "deploy", Description: "ship the build"},
	}
	skillList := []skills.Skill{
		{Name: "diagnose", Description: "disciplined bug hunt"},
	}

	got := allCommands(custom, skillList)

	byName := make(map[string]slashCmd, len(got))
	for _, c := range got {
		byName[c.Name] = c
	}

	if c, ok := byName["deploy"]; !ok {
		t.Error("custom command \"deploy\" missing")
	} else {
		if c.Kind != customCmd {
			t.Errorf("deploy Kind = %v, want customCmd", c.Kind)
		}
		if c.Summary != "ship the build" {
			t.Errorf("deploy Summary = %q, want description", c.Summary)
		}
	}

	if c, ok := byName["diagnose"]; !ok {
		t.Error("skill \"diagnose\" missing")
	} else {
		if c.Kind != skillCmd {
			t.Errorf("diagnose Kind = %v, want skillCmd", c.Kind)
		}
		if c.Summary != "disciplined bug hunt" {
			t.Errorf("diagnose Summary = %q, want description", c.Summary)
		}
	}

	// Every builtin still present.
	for _, b := range builtinCommands() {
		if _, ok := byName[b.Name]; !ok {
			t.Errorf("builtin %q dropped by allCommands", b.Name)
		}
	}

	if !sort.SliceIsSorted(got, func(i, j int) bool { return got[i].Name < got[j].Name }) {
		t.Errorf("allCommands output not sorted by Name: %v", names(got))
	}
}

// TestAllCommandsDedupePrecedence verifies builtin > custom > skill on a
// name clash: the higher tier wins and the duplicate is skipped.
func TestAllCommandsDedupePrecedence(t *testing.T) {
	custom := map[string]commands.Command{
		// clashes with a builtin → builtin must win
		"models": {Name: "models", Description: "custom models override"},
		// clashes with a skill below → custom must win
		"review": {Name: "review", Description: "custom review"},
	}
	skillList := []skills.Skill{
		{Name: "review", Description: "skill review"},
	}

	got := allCommands(custom, skillList)

	// No duplicate names at all.
	seen := make(map[string]int)
	for _, c := range got {
		seen[c.Name]++
	}
	for name, n := range seen {
		if n != 1 {
			t.Errorf("name %q appears %d times, want 1", name, n)
		}
	}

	byName := make(map[string]slashCmd, len(got))
	for _, c := range got {
		byName[c.Name] = c
	}

	// builtin beats custom on "models"
	if c := byName["models"]; c.Kind != builtinCmd || c.Summary == "custom models override" {
		t.Errorf("models = %+v, want builtin winner", c)
	}
	// custom beats skill on "review"
	if c := byName["review"]; c.Kind != customCmd || c.Summary != "custom review" {
		t.Errorf("review = %+v, want custom winner", c)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func names(cs []slashCmd) []string {
	out := make([]string, len(cs))
	for i, c := range cs {
		out[i] = c.Name
	}
	return out
}
