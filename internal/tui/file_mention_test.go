package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/amemiya02/deepseekcode/internal/agent"
	"github.com/amemiya02/deepseekcode/internal/permissions"
	"github.com/amemiya02/deepseekcode/internal/tools"
)

// mentionApp builds a keyflow-style App rooted at a real tree so the `@` menu
// has files to surface, and pre-marks the file index ready (the production path
// fills this from indexFilesCmd's fileIndexMsg, which the index-ready test
// drives explicitly).
func mentionApp(t *testing.T, files ...string) *App {
	t.Helper()
	cwd := t.TempDir()
	writeTree(t, cwd, files...)

	reg := tools.New()
	pol := permissions.New(permissions.ModeDefault, cwd, nil, nil, nil)
	ag := agent.New(nil, reg, pol, "deepseek-v4-flash")

	a := New(Config{Agent: ag, Model: "deepseek-v4-flash", Cwd: cwd})
	a.send = func(tea.Msg) {}
	a = sizeApp(t, a, 100, 40)
	return a
}

// TestAtTriggerShowsIndexingPlaceholderUntilReady: typing '@' before the
// background walk completes opens the popup with the single non-insertable
// "(indexing files…)" row.
func TestAtTriggerShowsIndexingPlaceholderUntilReady(t *testing.T) {
	a := mentionApp(t, "main.go")
	if a.fileIndexReady {
		t.Fatal("file index must not be ready before the fileIndexMsg lands")
	}

	a = drive(t, a, press('@'))
	if !a.completions.Active() || a.completions.Trigger() != '@' {
		t.Fatalf("typing '@' should open the @ popup, active=%v trigger=%q", a.completions.Active(), a.completions.Trigger())
	}
	sel, ok := a.completions.Selected()
	if !ok {
		t.Fatal("the indexing placeholder row should be selectable as the cursor row")
	}
	if sel.insert != "" {
		t.Fatalf("placeholder insert = %q, want empty (non-insertable)", sel.insert)
	}
	if sel.label != "(indexing files…)" {
		t.Fatalf("placeholder label = %q, want the indexing notice", sel.label)
	}
}

// TestAtTriggerProducesFileItems: once the file index is delivered, the @ menu
// is populated with the repo-relative paths as completion rows.
func TestAtTriggerProducesFileItems(t *testing.T) {
	a := mentionApp(t, "main.go", "pkg/util/helper.go")

	// Deliver the background-walk result the way Init's Cmd would.
	a = applyMsg(t, a, fileIndexMsg{paths: walkFiles(a.cwd, fileIndexCap)})
	if !a.fileIndexReady {
		t.Fatal("fileIndexMsg should mark the index ready")
	}

	a = drive(t, a, press('@'))
	if !a.completions.Active() || a.completions.Trigger() != '@' {
		t.Fatalf("typing '@' should open the @ popup, active=%v", a.completions.Active())
	}

	inserts := make(map[string]bool)
	for _, fi := range a.completions.filtered {
		inserts[a.completions.items[fi].insert] = true
	}
	if !inserts["main.go"] || !inserts["pkg/util/helper.go"] {
		t.Fatalf("@ menu should list the walked files, got %v", inserts)
	}
}

// TestAtAcceptInsertsPath: navigating to a file row and accepting splices the
// repo-relative path into the input from the '@' anchor, replacing the trigger.
func TestAtAcceptInsertsPath(t *testing.T) {
	a := mentionApp(t, "main.go", "pkg/util/helper.go")
	a = applyMsg(t, a, fileIndexMsg{paths: walkFiles(a.cwd, fileIndexCap)})

	// Type "@help" so the fuzzy filter pins the cursor on pkg/util/helper.go.
	for _, r := range "@help" {
		a = drive(t, a, press(r))
	}
	if !a.completions.Active() {
		t.Fatal("@help should keep the @ popup open")
	}
	sel, ok := a.completions.Selected()
	if !ok || sel.insert != "pkg/util/helper.go" {
		t.Fatalf("@help should select pkg/util/helper.go, got (%q, %v)", sel.insert, ok)
	}

	a.acceptCompletion()
	if a.completions.Active() {
		t.Fatal("accept should close the popup")
	}
	if got := a.input.Value(); got != "pkg/util/helper.go" {
		t.Fatalf("accept inserted %q, want the bare path (the '@'+query replaced)", got)
	}
}

// TestAtAcceptPlaceholderIsNoOp: accepting while the index is still loading
// leaves the typed "@" + query intact rather than splicing an empty string.
func TestAtAcceptPlaceholderIsNoOp(t *testing.T) {
	a := mentionApp(t, "main.go")
	for _, r := range "@m" {
		a = drive(t, a, press(r))
	}
	// "@m" before the index is ready: the placeholder "(indexing files…)" does
	// not match "m" as a label fuzzy candidate, so the popup may have an empty
	// match set — accept must still be a harmless no-op and preserve the text.
	a.acceptCompletion()
	if got := a.input.Value(); got != "@m" {
		t.Fatalf("placeholder accept mutated the input to %q, want %q", got, "@m")
	}
}

// applyMsg folds an arbitrary tea.Msg through Update and returns the *App, so
// the @-index tests can deliver fileIndexMsg the same way the runtime does.
func applyMsg(t *testing.T, a *App, msg tea.Msg) *App {
	t.Helper()
	m, _ := a.Update(msg)
	got, ok := m.(*App)
	if !ok {
		t.Fatalf("Update returned %T, want *App", m)
	}
	return got
}
