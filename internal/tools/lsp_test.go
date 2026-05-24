package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/amemiya02/deepseekcode/internal/lsp"
)

// fakeQuerier implements lsp.Querier for testing LSPTool.
type fakeQuerier struct {
	hoverText string
	hoverErr  error
	defs      []lsp.Definition
	defsErr   error
	refs      []lsp.Definition
	refsErr   error
	diags     []lsp.Diagnostic
}

func (f *fakeQuerier) Hover(ctx context.Context, uri string, line, character int) (string, error) {
	return f.hoverText, f.hoverErr
}
func (f *fakeQuerier) Definition(ctx context.Context, uri string, line, character int) ([]lsp.Definition, error) {
	return f.defs, f.defsErr
}
func (f *fakeQuerier) References(ctx context.Context, uri string, line, character int) ([]lsp.Definition, error) {
	return f.refs, f.refsErr
}
func (f *fakeQuerier) Diagnostics(uri string) []lsp.Diagnostic {
	return f.diags
}

// fakeLSPRegistry implements LSPRegistry for testing.
type fakeLSPRegistry struct {
	client lsp.Querier
	ok     bool
}

func (r *fakeLSPRegistry) ClientForURI(uri string) (lsp.Querier, bool) {
	return r.client, r.ok
}

func TestLSPToolHover(t *testing.T) {
	fc := &fakeQuerier{hoverText: "func Foo() int"}
	reg := &fakeLSPRegistry{client: fc, ok: true}
	tool := NewLSPTool(reg)

	args := json.RawMessage(`{"action":"hover","file":"/proj/main.go","line":10,"character":5}`)
	res, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if res.Content != "func Foo() int" {
		t.Errorf("got %q, want 'func Foo() int'", res.Content)
	}
}

func TestLSPToolHoverEmpty(t *testing.T) {
	fc := &fakeQuerier{hoverText: ""}
	reg := &fakeLSPRegistry{client: fc, ok: true}
	tool := NewLSPTool(reg)

	args := json.RawMessage(`{"action":"hover","file":"/proj/main.go","line":1,"character":1}`)
	res, _ := tool.Execute(context.Background(), args)
	if res.Content != "(no hover information)" {
		t.Errorf("got %q", res.Content)
	}
}

func TestLSPToolDefinition(t *testing.T) {
	fc := &fakeQuerier{
		defs: []lsp.Definition{
			{URI: "file:///proj/main.go", Line: 5, Character: 0},
			{URI: "file:///proj/util.go", Line: 10, Character: 2},
		},
	}
	reg := &fakeLSPRegistry{client: fc, ok: true}
	tool := NewLSPTool(reg)

	args := json.RawMessage(`{"action":"definition","file":"/proj/main.go","line":10,"character":5}`)
	res, _ := tool.Execute(context.Background(), args)
	// Should contain both locations, 0-indexed → 1-indexed
	if !strings.Contains(res.Content, "main.go:6:1") {
		t.Errorf("expected main.go:6:1 in output, got: %s", res.Content)
	}
	if !strings.Contains(res.Content, "util.go:11:3") {
		t.Errorf("expected util.go:11:3 in output, got: %s", res.Content)
	}
}

func TestLSPToolDefinitionEmpty(t *testing.T) {
	fc := &fakeQuerier{}
	reg := &fakeLSPRegistry{client: fc, ok: true}
	tool := NewLSPTool(reg)

	args := json.RawMessage(`{"action":"definition","file":"/proj/main.go","line":1,"character":1}`)
	res, _ := tool.Execute(context.Background(), args)
	if res.Content != "(no definition found)" {
		t.Errorf("got %q", res.Content)
	}
}

func TestLSPToolReferences(t *testing.T) {
	fc := &fakeQuerier{
		refs: []lsp.Definition{
			{URI: "file:///proj/a.go", Line: 1, Character: 2},
		},
	}
	reg := &fakeLSPRegistry{client: fc, ok: true}
	tool := NewLSPTool(reg)

	args := json.RawMessage(`{"action":"references","file":"/proj/main.go","line":10,"character":5}`)
	res, _ := tool.Execute(context.Background(), args)
	if !strings.Contains(res.Content, "a.go:2:3") {
		t.Errorf("got %q", res.Content)
	}
}

func TestLSPToolDiagnostics(t *testing.T) {
	fc := &fakeQuerier{
		diags: []lsp.Diagnostic{
			{Range: lsp.Range{Start: lsp.Position{Line: 0, Character: 0}, End: lsp.Position{Line: 0, Character: 5}}, Message: "unused var", Severity: 1, Source: "gopls"},
			{Range: lsp.Range{Start: lsp.Position{Line: 3, Character: 1}, End: lsp.Position{Line: 3, Character: 4}}, Message: "cannot use", Severity: 2},
		},
	}
	reg := &fakeLSPRegistry{client: fc, ok: true}
	tool := NewLSPTool(reg)

	args := json.RawMessage(`{"action":"diagnostics","file":"/proj/main.go"}`)
	res, _ := tool.Execute(context.Background(), args)
	if !strings.Contains(res.Content, "[error]") {
		t.Errorf("expected [error] severity, got: %s", res.Content)
	}
	if !strings.Contains(res.Content, "[warn]") {
		t.Errorf("expected [warn] severity, got: %s", res.Content)
	}
	if !strings.Contains(res.Content, "gopls: unused var") {
		t.Errorf("expected source prefix, got: %s", res.Content)
	}
}

func TestLSPToolDiagnosticsEmpty(t *testing.T) {
	fc := &fakeQuerier{}
	reg := &fakeLSPRegistry{client: fc, ok: true}
	tool := NewLSPTool(reg)

	args := json.RawMessage(`{"action":"diagnostics","file":"/proj/main.go"}`)
	res, _ := tool.Execute(context.Background(), args)
	if !strings.Contains(res.Content, "(no diagnostics for") {
		t.Errorf("got %q", res.Content)
	}
}

func TestLSPToolUnknownAction(t *testing.T) {
	reg := &fakeLSPRegistry{client: &fakeQuerier{}, ok: true}
	tool := NewLSPTool(reg)

	args := json.RawMessage(`{"action":"rename","file":"/proj/main.go","line":1,"character":1}`)
	res, _ := tool.Execute(context.Background(), args)
	if !res.IsError {
		t.Error("expected error for unknown action")
	}
	if !strings.Contains(res.Content, "unknown action") {
		t.Errorf("got %q", res.Content)
	}
}

func TestLSPToolNoServer(t *testing.T) {
	reg := &fakeLSPRegistry{ok: false}
	tool := NewLSPTool(reg)

	args := json.RawMessage(`{"action":"hover","file":"/proj/main.go","line":1,"character":1}`)
	res, _ := tool.Execute(context.Background(), args)
	if !strings.Contains(res.Content, "(no LSP server available") {
		t.Errorf("got %q", res.Content)
	}
}

func TestLSPToolLineCharZeroClamp(t *testing.T) {
	// line=0, character=0 → both clamped to 0 (already 0-indexed)
	fc := &fakeQuerier{hoverText: "zero-based"}
	reg := &fakeLSPRegistry{client: fc, ok: true}
	tool := NewLSPTool(reg)

	args := json.RawMessage(`{"action":"hover","file":"/proj/main.go","line":0,"character":0}`)
	res, _ := tool.Execute(context.Background(), args)
	if res.Content != "zero-based" {
		t.Errorf("got %q", res.Content)
	}
}

func TestLSPToolInvalidArgs(t *testing.T) {
	reg := &fakeLSPRegistry{client: &fakeQuerier{}, ok: true}
	tool := NewLSPTool(reg)

	args := json.RawMessage(`not json}`)
	res, _ := tool.Execute(context.Background(), args)
	if !res.IsError {
		t.Error("expected error for invalid JSON")
	}
}

func TestLSPToolIsReadOnly(t *testing.T) {
	tool := NewLSPTool(nil)
	if !tool.IsReadOnly() {
		t.Error("LSP tool should be read-only")
	}
}
