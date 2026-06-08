package tui

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/amemiya02/deepseekcode/internal/agent"
	"github.com/amemiya02/deepseekcode/internal/permissions"
)

func TestPermissionCard_ShowsEditDiff(t *testing.T) {
	reply := make(chan agent.PermissionResponse, 1)
	diff := `--- a/foo.go
+++ b/foo.go
@@ -10,3 +10,4 @@ func main() {
 	fmt.Println("hello")
+	fmt.Println("world")
 }
`
	args, _ := json.Marshal(map[string]string{
		"path": "/tmp/foo.go",
		"diff": diff,
	})

	flow := NewPermissionFlow()
	flow.Open(agent.EventPermissionAsk{
		Check: permissions.Check{
			Tool: &stubTool{name: "edit_file"},
			Args: json.RawMessage(args),
		},
		Reply: reply,
	})
	out := stripANSI(flow.Render(DarkTheme(), 120))

	if !strings.Contains(out, "permission required") {
		t.Fatalf("card header missing:\n%s", out)
	}
	// The diff preview must include at least one added line marker.
	if !strings.Contains(out, "world") {
		t.Fatalf("edit approval must preview the diff:\n%s", out)
	}
}

func TestPermissionCard_NoDiffForReadOnlyTool(t *testing.T) {
	reply := make(chan agent.PermissionResponse, 1)
	args, _ := json.Marshal(map[string]string{
		"path": "/tmp/foo.go",
	})

	flow := NewPermissionFlow()
	flow.Open(agent.EventPermissionAsk{
		Check: permissions.Check{
			Tool: &stubTool{name: "read_file"},
			Args: json.RawMessage(args),
		},
		Reply: reply,
	})
	out := stripANSI(flow.Render(DarkTheme(), 120))

	if !strings.Contains(out, "permission required") {
		t.Fatalf("card header missing:\n%s", out)
	}
	// read_file should NOT produce a diff block.
	if strings.Contains(out, "---") || strings.Contains(out, "+++") {
		t.Fatalf("read-only tool should not show diff preview:\n%s", out)
	}
}
