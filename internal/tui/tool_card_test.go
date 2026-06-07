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
