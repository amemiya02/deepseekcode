package tui

import "strings"

// ToolStatus is the lifecycle state of a tool call as shown on its card.
type ToolStatus int

const (
	ToolRunning ToolStatus = iota
	ToolSuccess
	ToolError
	ToolAwaitingPermission
)

// ToolRenderOpts is the input to a per-tool card renderer.
type ToolRenderOpts struct {
	Tool     string
	Args     string // raw JSON args
	Result   string // tool result content ("" while running)
	Status   ToolStatus
	Expanded bool
	Width    int // optional override; RenderTool passes width through
}

// toolRenderer renders one tool's card.
type toolRenderer func(t Theme, width int, o ToolRenderOpts) string

// toolRegistry maps an exact tool name to its renderer. "mcp_*" is matched by
// prefix in RenderTool. Unregistered tools fall through to renderDefaultCard.
var toolRegistry = map[string]toolRenderer{}

// statusGlyph returns a status icon styled with a semantic theme token.
func statusGlyph(t Theme, s ToolStatus) string {
	switch s {
	case ToolRunning:
		return t.Info.Render("◐")
	case ToolSuccess:
		return t.Info.Render("●")
	case ToolError:
		return t.Error.Render("✖")
	case ToolAwaitingPermission:
		return t.HookInfo.Render("◆")
	default:
		return t.Info.Render("•")
	}
}

// RenderTool dispatches to the registered renderer for o.Tool, falling back to
// the default card. mcp_* tools use the shared MCP renderer.
func RenderTool(t Theme, width int, o ToolRenderOpts) string {
	if o.Width == 0 {
		o.Width = width
	}
	if r, ok := toolRegistry[o.Tool]; ok {
		return r(t, width, o)
	}
	if strings.HasPrefix(o.Tool, "mcp_") {
		if r, ok := toolRegistry["mcp_*"]; ok {
			return r(t, width, o)
		}
	}
	return renderDefaultCard(t, width, o)
}

// renderDefaultCard is the fallback: status glyph + tool name + the existing
// one-line summary as the body.
func renderDefaultCard(t Theme, width int, o ToolRenderOpts) string {
	head := statusGlyph(t, o.Status) + " " + t.StatusModel.Render(o.Tool)
	body := RenderToolSummary(o.Tool, o.Args, o.Result, o.Status == ToolError, width)
	return head + "\n" + body
}
