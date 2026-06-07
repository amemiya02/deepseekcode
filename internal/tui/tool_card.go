package tui

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
)

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

func init() {
	toolRegistry["bash"] = renderBashCard
	toolRegistry["edit_file"] = renderEditCard
	toolRegistry["multi_edit"] = renderEditCard
	toolRegistry["write_file"] = renderEditCard
	toolRegistry["todo_write"] = renderTodoCard
	toolRegistry["read_file"] = renderReadCard
	toolRegistry["grep"] = renderSearchCard
	toolRegistry["glob"] = renderSearchCard
	toolRegistry["ls"] = renderSearchCard
	toolRegistry["web_fetch"] = renderFetchCard
	toolRegistry["web_search"] = renderFetchCard
	toolRegistry["lsp"] = renderDiagnosticsCard
	toolRegistry["ask"] = renderAskCard
	toolRegistry["mcp_*"] = renderMCPCard
}

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

// cardHead builds the common head line: status glyph + label + trailing detail.
func cardHead(t Theme, o ToolRenderOpts, label, detail string) string {
	head := statusGlyph(t, o.Status) + " " + t.StatusModel.Render(label)
	if detail != "" {
		head += " " + t.Hint.Render(oneline(detail, 80))
	}
	return head
}

// cardBody wraps a body string with the CardBar left margin.
func cardBody(t Theme, body string) string {
	if body == "" {
		return ""
	}
	bar := t.CardBar.Render("│ ")
	lines := strings.Split(body, "\n")
	for i, l := range lines {
		lines[i] = bar + l
	}
	return strings.Join(lines, "\n")
}

// renderDefaultCard is the fallback: status glyph + tool name + the existing
// one-line summary as the body.
func renderDefaultCard(t Theme, width int, o ToolRenderOpts) string {
	head := statusGlyph(t, o.Status) + " " + t.StatusModel.Render(o.Tool)
	body := RenderToolSummary(o.Tool, o.Args, o.Result, o.Status == ToolError, width)
	return head + "\n" + body
}

// ── bash ────────────────────────────────────────────────────────────────────

func renderBashCard(t Theme, width int, o ToolRenderOpts) string {
	cmd := extractJSONString(o.Args, "command")
	head := cardHead(t, o, "bash", oneline(cmd, 60))
	if !o.Expanded || o.Result == "" {
		return head
	}
	return head + "\n" + cardBody(t, oneline(o.Result, width-4))
}

// ── edit_file / multi_edit / write_file ─────────────────────────────────────

func renderEditCard(t Theme, width int, o ToolRenderOpts) string {
	path := extractJSONString(o.Args, "path")
	if path == "" {
		path = extractJSONString(o.Args, "file")
	}
	head := cardHead(t, o, o.Tool, path)
	if !o.Expanded {
		return head
	}
	diff := extractJSONString(o.Args, "diff")
	if diff == "" {
		diff = extractJSONString(o.Args, "content")
	}
	if diff == "" {
		return head
	}
	lang := langFromPath(path)
	rendered := renderDiffAuto(t, diff, lang, width-4)
	if rendered == "" {
		rendered = renderDiff(t, diff, lang, width-4)
	}
	if rendered == "" {
		rendered = oneline(diff, width-4)
	}
	return head + "\n" + cardBody(t, rendered)
}

// ── todo_write ──────────────────────────────────────────────────────────────

func renderTodoCard(t Theme, width int, o ToolRenderOpts) string {
	head := cardHead(t, o, "todos", "")
	items := extractJSONArray(o.Args, "todos")
	if len(items) == 0 {
		return head
	}
	var lines []string
	for _, it := range items {
		text, _ := it["text"].(string)
		status, _ := it["status"].(string)
		if text == "" {
			continue
		}
		icon := "○"
		if status == "done" {
			icon = "●"
		} else if status == "in_progress" {
			icon = "◐"
		}
		lines = append(lines, icon+" "+oneline(text, width-6))
	}
	if len(lines) == 0 {
		return head
	}
	body := strings.Join(lines, "\n")
	if !o.Expanded {
		// Show first 3 items collapsed
		if len(lines) > 3 {
			body = strings.Join(lines[:3], "\n") + "\n" + t.Hint.Render(fmt.Sprintf("  +%d more", len(lines)-3))
		}
	}
	return head + "\n" + cardBody(t, body)
}

// ── read_file ───────────────────────────────────────────────────────────────

func renderReadCard(t Theme, width int, o ToolRenderOpts) string {
	path := extractJSONString(o.Args, "path")
	if path == "" {
		path = "unknown"
	}
	offset := extractJSONString(o.Args, "offset")
	limit := extractJSONString(o.Args, "limit")
	detail := path
	if offset != "" || limit != "" {
		detail += " " + offset + ":" + limit
	}
	head := cardHead(t, o, "read", detail)
	if !o.Expanded || o.Result == "" {
		return head
	}
	// Show first N lines of the result
	lines := strings.Split(o.Result, "\n")
	const maxLines = 12
	if len(lines) > maxLines {
		lines = append(lines[:maxLines], t.Hint.Render(fmt.Sprintf("  …%d more lines", len(lines)-maxLines)))
	}
	return head + "\n" + cardBody(t, strings.Join(lines, "\n"))
}

// ── grep / glob / ls ────────────────────────────────────────────────────────

func renderSearchCard(t Theme, width int, o ToolRenderOpts) string {
	pattern := extractJSONString(o.Args, "pattern")
	if pattern == "" {
		pattern = extractJSONString(o.Args, "path")
	}
	if pattern == "" {
		pattern = extractJSONString(o.Args, "query")
	}
	matches := lineCount(o.Result)
	detail := oneline(pattern, 40)
	if matches > 0 {
		detail += fmt.Sprintf(" (%d matches)", matches)
	}
	head := cardHead(t, o, o.Tool, detail)
	if !o.Expanded || o.Result == "" {
		return head
	}
	lines := strings.Split(o.Result, "\n")
	const maxLines = 10
	if len(lines) > maxLines {
		lines = append(lines[:maxLines], t.Hint.Render(fmt.Sprintf("  …%d more", len(lines)-maxLines)))
	}
	return head + "\n" + cardBody(t, strings.Join(lines, "\n"))
}

// ── web_fetch / web_search ──────────────────────────────────────────────────

func renderFetchCard(t Theme, width int, o ToolRenderOpts) string {
	target := extractJSONString(o.Args, "url")
	if target == "" {
		target = extractJSONString(o.Args, "query")
	}
	head := cardHead(t, o, o.Tool, oneline(target, 60))
	if !o.Expanded || o.Result == "" {
		return head
	}
	excerpt := oneline(o.Result, width-4)
	return head + "\n" + cardBody(t, excerpt)
}

// ── lsp ─────────────────────────────────────────────────────────────────────

func renderDiagnosticsCard(t Theme, width int, o ToolRenderOpts) string {
	head := cardHead(t, o, "diagnostics", "")
	if !o.Expanded || o.Result == "" {
		return head
	}
	// Count lines as a rough diagnostic count
	count := lineCount(o.Result)
	summary := fmt.Sprintf("%d diagnostic(s)", count)
	lines := strings.Split(o.Result, "\n")
	const maxLines = 8
	if len(lines) > maxLines {
		lines = append(lines[:maxLines], t.Hint.Render(fmt.Sprintf("  …%d more", len(lines)-maxLines)))
	}
	body := summary + "\n" + strings.Join(lines, "\n")
	return head + "\n" + cardBody(t, body)
}

// ── ask ─────────────────────────────────────────────────────────────────────

func renderAskCard(t Theme, width int, o ToolRenderOpts) string {
	question := extractJSONString(o.Args, "question")
	if question == "" {
		question = extractJSONString(o.Args, "prompt")
	}
	head := cardHead(t, o, "ask", oneline(question, 60))
	if !o.Expanded || o.Result == "" {
		return head
	}
	return head + "\n" + cardBody(t, oneline(o.Result, width-4))
}

// ── mcp_* ───────────────────────────────────────────────────────────────────

func renderMCPCard(t Theme, width int, o ToolRenderOpts) string {
	// Parse server and tool from "mcp_<server>_<tool>"
	parts := strings.SplitN(strings.TrimPrefix(o.Tool, "mcp_"), "_", 2)
	label := o.Tool
	if len(parts) == 2 {
		label = parts[0] + "/" + parts[1]
	}
	head := cardHead(t, o, label, "")
	if !o.Expanded || o.Result == "" {
		return head
	}
	return head + "\n" + cardBody(t, oneline(o.Result, width-4))
}

// ── helpers ─────────────────────────────────────────────────────────────────

// extractJSONArray returns a slice of maps from a JSON array field.
func extractJSONArray(args, key string) []map[string]any {
	var m map[string]any
	if json.Unmarshal([]byte(args), &m) != nil {
		return nil
	}
	arr, ok := m[key].([]any)
	if !ok {
		return nil
	}
	var out []map[string]any
	for _, item := range arr {
		if obj, ok := item.(map[string]any); ok {
			out = append(out, obj)
		}
	}
	return out
}

// langFromPath maps a file extension to a chroma lexer name.
func langFromPath(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".go":
		return "go"
	case ".py":
		return "python"
	case ".js", ".mjs":
		return "javascript"
	case ".ts":
		return "typescript"
	case ".tsx":
		return "tsx"
	case ".jsx":
		return "jsx"
	case ".rs":
		return "rust"
	case ".java":
		return "java"
	case ".c":
		return "c"
	case ".cpp", ".cc", ".cxx":
		return "cpp"
	case ".h", ".hpp":
		return "c"
	case ".rb":
		return "ruby"
	case ".sh", ".bash":
		return "bash"
	case ".json":
		return "json"
	case ".yaml", ".yml":
		return "yaml"
	case ".toml":
		return "toml"
	case ".md":
		return "markdown"
	case ".html", ".htm":
		return "html"
	case ".css":
		return "css"
	case ".sql":
		return "sql"
	case ".xml":
		return "xml"
	case ".proto":
		return "protobuf"
	default:
		return ""
	}
}
