package permissions

import (
	"encoding/json"
	"testing"
)

// ConfirmEdits is the GUI "Ask" mode: ModeDefault tiering, except the
// "safe write inside cwd" auto-allow is downgraded to Ask so each edit is
// confirmed. Auto-edit mode is plain ModeDefault (ConfirmEdits=false).
func TestConfirmEditsGatesSafeWrites(t *testing.T) {
	cwd := realCwd(t, t.TempDir())
	inside := cwd + "/file.go"

	pol := New(ModeDefault, cwd, nil, nil, nil)
	pol.ConfirmEdits = true

	for _, name := range []string{"write_file", "edit_file"} {
		dec, reason := pol.Decide(Check{
			Tool: &pathAwareTool{name: name, paths: []string{inside}},
			Args: json.RawMessage(`{}`),
		})
		if dec != Ask {
			t.Errorf("ConfirmEdits: %s inside cwd = %v (%s), want Ask", name, dec, reason)
		}
	}

	// Read-only tools stay auto-allowed — ConfirmEdits is about edits only.
	dec, _ := pol.Decide(Check{
		Tool: &fakeTool{name: "read_file", readOnly: true},
		Args: json.RawMessage(`{}`),
	})
	if dec != Allow {
		t.Errorf("ConfirmEdits: read_file = %v, want Allow", dec)
	}

	// Off (auto-edit / default): safe writes inside cwd auto-allow as before.
	pol.ConfirmEdits = false
	dec, reason := pol.Decide(Check{
		Tool: &pathAwareTool{name: "write_file", paths: []string{inside}},
		Args: json.RawMessage(`{}`),
	})
	if dec != Allow {
		t.Errorf("no ConfirmEdits: write_file inside cwd = %v (%s), want Allow", dec, reason)
	}
}
