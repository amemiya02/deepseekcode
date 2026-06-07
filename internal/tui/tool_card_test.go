package tui

import (
	"strings"
	"testing"
)

func TestRenderTool_DefaultFallback(t *testing.T) {
	th := DarkTheme()
	out := RenderTool(th, 80, ToolRenderOpts{
		Tool: "totally_unknown_tool", Args: `{"x":1}`, Result: "ok", Status: ToolSuccess,
	})
	if !strings.Contains(out, "totally_unknown_tool") {
		t.Fatalf("default renderer must show the tool name:\n%s", out)
	}
}

func TestRenderTool_StatusIconForEachState(t *testing.T) {
	th := DarkTheme()
	for _, st := range []ToolStatus{ToolRunning, ToolSuccess, ToolError, ToolAwaitingPermission} {
		out := RenderTool(th, 80, ToolRenderOpts{Tool: "bash", Args: `{"command":"ls"}`, Status: st})
		if strings.TrimSpace(out) == "" {
			t.Fatalf("status %v produced empty card", st)
		}
	}
}

func TestBashCard_ShowsCommand(t *testing.T) {
	th := DarkTheme()
	out := RenderTool(th, 100, ToolRenderOpts{Tool: "bash", Args: `{"command":"go test ./..."}`, Result: "ok\n", Status: ToolSuccess})
	if !strings.Contains(out, "go test ./...") {
		t.Fatalf("bash card must show the command:\n%s", out)
	}
}

func TestBashCard_ExpandedShowsResult(t *testing.T) {
	th := DarkTheme()
	out := RenderTool(th, 100, ToolRenderOpts{Tool: "bash", Args: `{"command":"echo hi"}`, Result: "hi\n", Status: ToolSuccess, Expanded: true})
	if !strings.Contains(out, "hi") {
		t.Fatalf("expanded bash card must show result:\n%s", out)
	}
}

func TestEditCard_EmbedsDiff(t *testing.T) {
	th := DarkTheme()
	args := `{"path":"main.go","diff":"@@ -1 +1 @@\n-old\n+new"}`
	out := RenderTool(th, 100, ToolRenderOpts{Tool: "edit_file", Args: args, Status: ToolSuccess, Expanded: true})
	if !strings.Contains(out, "new") || !strings.Contains(out, "main.go") {
		t.Fatalf("edit card must embed the diff + path:\n%s", out)
	}
}

func TestEditCard_MultiEdit(t *testing.T) {
	th := DarkTheme()
	args := `{"path":"foo.go","diff":"+line"}`
	out := RenderTool(th, 100, ToolRenderOpts{Tool: "multi_edit", Args: args, Status: ToolSuccess})
	if !strings.Contains(out, "foo.go") {
		t.Fatalf("multi_edit card must show the path:\n%s", out)
	}
}

func TestTodoCard_ListsItems(t *testing.T) {
	th := DarkTheme()
	out := RenderTool(th, 100, ToolRenderOpts{Tool: "todo_write", Args: `{"todos":[{"text":"step one","status":"done"}]}`, Status: ToolSuccess})
	if !strings.Contains(out, "step one") {
		t.Fatalf("todo card must list items:\n%s", out)
	}
}

func TestTodoCard_MultipleItems(t *testing.T) {
	th := DarkTheme()
	args := `{"todos":[{"text":"a","status":"done"},{"text":"b","status":"in_progress"},{"text":"c","status":"pending"}]}`
	out := RenderTool(th, 100, ToolRenderOpts{Tool: "todo_write", Args: args, Status: ToolSuccess})
	if !strings.Contains(out, "a") || !strings.Contains(out, "b") || !strings.Contains(out, "c") {
		t.Fatalf("todo card must show all items:\n%s", out)
	}
}

func TestReadCard_ShowsPath(t *testing.T) {
	th := DarkTheme()
	out := RenderTool(th, 100, ToolRenderOpts{Tool: "read_file", Args: `{"path":"main.go"}`, Result: "package main\n", Status: ToolSuccess})
	if !strings.Contains(out, "main.go") {
		t.Fatalf("read card must show the path:\n%s", out)
	}
}

func TestSearchCard_ShowsPattern(t *testing.T) {
	th := DarkTheme()
	out := RenderTool(th, 100, ToolRenderOpts{Tool: "grep", Args: `{"pattern":"TODO"}`, Result: "a.go:1:TODO fix\n", Status: ToolSuccess})
	if !strings.Contains(out, "TODO") {
		t.Fatalf("search card must show the pattern:\n%s", out)
	}
}

func TestFetchCard_ShowsURL(t *testing.T) {
	th := DarkTheme()
	out := RenderTool(th, 100, ToolRenderOpts{Tool: "web_fetch", Args: `{"url":"https://example.com"}`, Status: ToolSuccess})
	if !strings.Contains(out, "example.com") {
		t.Fatalf("fetch card must show the URL:\n%s", out)
	}
}

func TestMCPCard_ParsesServerTool(t *testing.T) {
	th := DarkTheme()
	out := RenderTool(th, 100, ToolRenderOpts{Tool: "mcp_github_pr", Args: `{}`, Result: "ok", Status: ToolSuccess, Expanded: true})
	if !strings.Contains(out, "github") {
		t.Fatalf("mcp card must show server name:\n%s", out)
	}
}

func TestAskCard_ShowsQuestion(t *testing.T) {
	th := DarkTheme()
	out := RenderTool(th, 100, ToolRenderOpts{Tool: "ask", Args: `{"question":"what is 2+2?"}`, Status: ToolSuccess})
	if !strings.Contains(out, "2+2") {
		t.Fatalf("ask card must show the question:\n%s", out)
	}
}

func TestDiagnosticsCard_ShowsLabel(t *testing.T) {
	th := DarkTheme()
	out := RenderTool(th, 100, ToolRenderOpts{Tool: "lsp", Args: `{}`, Result: "err.go:10: error\n", Status: ToolSuccess, Expanded: true})
	if !strings.Contains(out, "diagnostic") {
		t.Fatalf("diagnostics card must show diagnostic label:\n%s", out)
	}
}

func TestLangFromPath(t *testing.T) {
	tests := []struct {
		path, want string
	}{
		{"main.go", "go"},
		{"app.py", "python"},
		{"index.ts", "typescript"},
		{"style.css", "css"},
		{"unknown.xyz", ""},
	}
	for _, tt := range tests {
		got := langFromPath(tt.path)
		if got != tt.want {
			t.Errorf("langFromPath(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}

func TestExtractJSONArray(t *testing.T) {
	args := `{"items":[{"name":"a"},{"name":"b"}]}`
	out := extractJSONArray(args, "items")
	if len(out) != 2 {
		t.Fatalf("expected 2 items, got %d", len(out))
	}
	if out[0]["name"] != "a" || out[1]["name"] != "b" {
		t.Fatalf("unexpected items: %v", out)
	}
}
