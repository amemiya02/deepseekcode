package tui

import (
	"sort"

	"github.com/amemiya02/deepseekcode/internal/commands"
	"github.com/amemiya02/deepseekcode/internal/skills"
)

// cmdKind classifies a slash command by its origin so the menu (G1) and
// the help overlay (G7) can tag and prioritize rows from one source.
type cmdKind int

const (
	builtinCmd cmdKind = iota // hard-coded handler in handleSlash
	customCmd                 // user-defined commands.Command
	skillCmd                  // discoverable skills.Skill
	fileCmd                   // repo-relative path row in the @ menu (G4)
)

// slashCmd is the structured representation of a single slash command.
// Name carries no leading slash (e.g. "models").
type slashCmd struct {
	Name    string
	Aliases []string
	Summary string // one line, shown in the menu + help
	Kind    cmdKind
}

// builtinCommands returns the static built-in command list. It mirrors
// exactly the commands handled in handleSlash (and surfaced by the /help
// overlay via the tab body builders): names carry no leading slash and aliases match the
// alternate forms accepted by the dispatcher.
func builtinCommands() []slashCmd {
	return []slashCmd{
		{Name: "help", Summary: "show this help", Kind: builtinCmd},
		{Name: "clear", Summary: "clear scrollback", Kind: builtinCmd},
		{Name: "quit", Aliases: []string{"exit", "q"}, Summary: "exit", Kind: builtinCmd},
		{Name: "models", Summary: "list / switch the main-loop model", Kind: builtinCmd},
		{Name: "effort", Summary: "show / set DeepSeek reasoning effort", Kind: builtinCmd},
		{Name: "cost", Summary: "show current session cost/cache report", Kind: builtinCmd},
		{Name: "theme", Summary: "switch the color theme", Kind: builtinCmd},
		{Name: "tape", Summary: "open the reasoning tape", Kind: builtinCmd},
		{Name: "sessions", Summary: "list this project's sessions", Kind: builtinCmd},
		{Name: "export", Aliases: []string{"scrollback"}, Summary: "open full scrollback in $PAGER", Kind: builtinCmd},
		{Name: "undo", Summary: "revert the last edit step", Kind: builtinCmd},
		{Name: "compact", Summary: "force-compact the running message list", Kind: builtinCmd},
		{Name: "reload-skills", Aliases: []string{"reload"}, Summary: "re-scan skills mid-session (mints a new cache epoch)", Kind: builtinCmd},
		{Name: "mcp", Summary: "show MCP server status", Kind: builtinCmd},
		{Name: "lsp", Summary: "show LSP server status", Kind: builtinCmd},
		{Name: "permissions", Summary: "show effective permission policy", Kind: builtinCmd},
	}
}

// allCommands merges the built-in commands with user-defined custom
// commands and discoverable skills into one sorted, deduped list. This
// single source feeds both the / menu (G1) and the ? help overlay (G7)
// so they can never drift.
//
// Dedup precedence is builtin > custom > skill: a name already claimed by
// a higher tier wins, and the lower-tier duplicate is skipped. The result
// is sorted by Name.
func allCommands(custom map[string]commands.Command, skillList []skills.Skill) []slashCmd {
	out := builtinCommands()
	seen := make(map[string]bool, len(out))
	for _, c := range out {
		seen[c.Name] = true
	}

	for name, c := range custom {
		if seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, slashCmd{Name: name, Summary: c.Description, Kind: customCmd})
	}

	for _, sk := range skillList {
		if seen[sk.Name] {
			continue
		}
		seen[sk.Name] = true
		out = append(out, slashCmd{Name: sk.Name, Summary: sk.Description, Kind: skillCmd})
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
