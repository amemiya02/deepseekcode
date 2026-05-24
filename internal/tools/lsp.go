package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/amemiya02/deepseekcode/internal/lsp"
)

// LSPRegistry is the subset of lsp.Registry that the LSP tool needs.
type LSPRegistry interface {
	ClientForURI(uri string) (lsp.Querier, bool)
}

// LSPTool exposes LSP features (hover, definition, references,
// diagnostics) as a single tool with an action parameter.
type LSPTool struct {
	reg LSPRegistry
}

// NewLSPTool creates an LSP tool backed by the given registry.
func NewLSPTool(reg LSPRegistry) *LSPTool {
	return &LSPTool{reg: reg}
}

func (LSPTool) Name() string { return "lsp" }

func (LSPTool) Description() string {
	return "Query the Language Server Protocol for symbol information. " +
		"Supports four actions: " +
		"hover — show type/doc at a position; " +
		"definition — go to definition; " +
		"references — find all references; " +
		"diagnostics — show diagnostics for a file. " +
		"Requires an LSP server for the file's language to be running."
}

func (LSPTool) Parameters() json.RawMessage {
	return MustParams(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action": map[string]any{
				"type":        "string",
				"enum":        []string{"hover", "definition", "references", "diagnostics"},
				"description": "The LSP operation to perform.",
			},
			"file": map[string]any{
				"type":        "string",
				"description": "File path to query (absolute or relative to cwd).",
			},
			"line": map[string]any{
				"type":        "integer",
				"description": "1-indexed line number. Required for hover, definition, and references.",
			},
			"character": map[string]any{
				"type":        "integer",
				"description": "1-indexed character (column) offset. Required for hover, definition, and references.",
			},
		},
		"required": []string{"action", "file"},
	})
}

func (LSPTool) IsReadOnly() bool { return true }

func (t *LSPTool) Execute(ctx context.Context, args json.RawMessage) (Result, error) {
	var p struct {
		Action    string `json:"action"`
		File      string `json:"file"`
		Line      int    `json:"line"`
		Character int    `json:"character"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return Errf("invalid args: %v", err), nil
	}

	uri := lsp.PathToURI(p.File)
	client, ok := t.reg.ClientForURI(uri)
	if !ok {
		return Result{Content: fmt.Sprintf("(no LSP server available for %s)", p.File)}, nil
	}

	// LSP uses 0-indexed positions; accept 1-indexed from the model.
	line := p.Line - 1
	char := p.Character - 1
	if line < 0 {
		line = 0
	}
	if char < 0 {
		char = 0
	}

	switch p.Action {
	case "hover":
		text, err := client.Hover(ctx, uri, line, char)
		if err != nil {
			return Errf("hover failed: %v", err), nil
		}
		if text == "" {
			return Result{Content: "(no hover information)"}, nil
		}
		return Result{Content: text}, nil

	case "definition":
		defs, err := client.Definition(ctx, uri, line, char)
		if err != nil {
			return Errf("definition failed: %v", err), nil
		}
		if len(defs) == 0 {
			return Result{Content: "(no definition found)"}, nil
		}
		return Result{Content: formatDefinitions(defs)}, nil

	case "references":
		refs, err := client.References(ctx, uri, line, char)
		if err != nil {
			return Errf("references failed: %v", err), nil
		}
		if len(refs) == 0 {
			return Result{Content: "(no references found)"}, nil
		}
		return Result{Content: formatDefinitions(refs)}, nil

	case "diagnostics":
		diags := client.Diagnostics(uri)
		if len(diags) == 0 {
			return Result{Content: fmt.Sprintf("(no diagnostics for %s)", p.File)}, nil
		}
		return Result{Content: formatDiagnostics(diags)}, nil

	default:
		return Errf("unknown action: %q (expected hover, definition, references, or diagnostics)", p.Action), nil
	}
}

func formatDefinitions(defs []lsp.Definition) string {
	var b strings.Builder
	for i, d := range defs {
		path := lsp.URIToPath(d.URI)
		if i > 0 {
			fmt.Fprintln(&b)
		}
		fmt.Fprintf(&b, "%s:%d:%d", path, d.Line+1, d.Character+1)
	}
	return b.String()
}

func formatDiagnostics(diags []lsp.Diagnostic) string {
	var b strings.Builder
	for i, d := range diags {
		if i > 0 {
			fmt.Fprintln(&b)
		}
		severity := "info"
		switch d.Severity {
		case 1:
			severity = "error"
		case 2:
			severity = "warn"
		case 3:
			severity = "info"
		case 4:
			severity = "hint"
		}
		msg := d.Message
		if d.Source != "" {
			msg = d.Source + ": " + msg
		}
		fmt.Fprintf(&b, "L%d:%d [%s] %s",
			d.Range.Start.Line+1, d.Range.Start.Character+1, severity, msg)
	}
	return b.String()
}
