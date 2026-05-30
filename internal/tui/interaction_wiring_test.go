package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/amemiya02/deepseekcode/internal/agent"
	"github.com/amemiya02/deepseekcode/internal/permissions"
	"github.com/amemiya02/deepseekcode/internal/skills"
	"github.com/amemiya02/deepseekcode/internal/tools"
)

// These cover the two App-level wiring branches that need real fixtures: the
// guided skill directive (§4.1/§10) prefilled by /<skill-name>, and the
// off-UI-goroutine history file append (§10/§11) that backs cross-session
// recall.

// writeSkill drops a minimal valid SKILL.md under cwd so skills.LoadScan picks
// it up, returning the loaded store.
func writeSkill(t *testing.T, cwd, name, desc string) *skills.Store {
	t.Helper()
	dir := filepath.Join(cwd, ".deepseek", "skills", name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir skill dir: %v", err)
	}
	body := "---\nname: " + name + "\ndescription: " + desc + "\n---\n# " + name + "\nbody\n"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(body), 0o644); err != nil {
		t.Fatalf("write SKILL.md: %v", err)
	}
	store, err := skills.LoadScan(cwd, "")
	if err != nil {
		t.Fatalf("LoadScan: %v", err)
	}
	return store
}

// skillApp builds a keyflow-style App whose agent carries a one-skill store
// and a fixed history path under tmp.
func skillApp(t *testing.T, skillName, desc string) *App {
	t.Helper()
	cwd := t.TempDir()
	store := writeSkill(t, cwd, skillName, desc)

	reg := tools.New()
	pol := permissions.New(permissions.ModeDefault, cwd, nil, nil, nil)
	ag := agent.New(nil, reg, pol, "deepseek-v4-flash")
	ag.Skills = store

	a := New(Config{
		Agent:       ag,
		Model:       "deepseek-v4-flash",
		Cwd:         cwd,
		HistoryPath: filepath.Join(t.TempDir(), "history", "ring.txt"),
	})
	return sizeApp(t, a, 100, 40)
}

// TestSlashSkillPrefillsDirective: a bare /<skill-name> that is neither a
// builtin nor a custom command prefills the input with a guided directive and
// stays in Insert mode — it must not echo "unknown command".
func TestSlashSkillPrefillsDirective(t *testing.T) {
	a := skillApp(t, "refactor", "Refactor a module")

	// The popup-derived menu must surface the skill row with its tag.
	items := a.completionItems('/')
	var found bool
	for _, it := range items {
		if it.insert == "/refactor" {
			found = true
			if it.label != "/refactor (skill)" {
				t.Fatalf("skill row label = %q, want %q", it.label, "/refactor (skill)")
			}
		}
	}
	if !found {
		t.Fatal("the / menu should include the skill /refactor")
	}

	before := len(a.scrollback.Items())
	a.handleSlash("/refactor")

	if got := len(a.scrollback.Items()); got != before {
		t.Fatalf("a known skill must not echo 'unknown command'; scrollback grew by %d", got-before)
	}
	want := "use the \"refactor\" skill for this: "
	if got := a.input.Value(); got != want {
		t.Fatalf("skill prefill = %q, want %q", got, want)
	}
	if a.mode != modeInsert {
		t.Fatalf("skill prefill should stay in Insert mode, got %v", a.mode)
	}
}

// TestUnknownSlashStillErrors: a /<name> that is neither command nor skill
// still surfaces the unknown-command notice (the skill branch must not swallow
// genuine typos). The notice is ephemeral, so it rides the toast (G8/§6.5) and
// must NOT touch the scrollback.
func TestUnknownSlashStillErrors(t *testing.T) {
	a := skillApp(t, "refactor", "Refactor a module")
	before := len(a.scrollback.Items())
	a.handleSlash("/nonsense")
	if got := len(a.scrollback.Items()); got != before {
		t.Fatalf("unknown /nonsense should not touch the scrollback, grew by %d", got-before)
	}
	if !a.toastState.active || !strings.Contains(a.toastState.text, "unknown command") {
		t.Fatalf("unknown /nonsense should arm the unknown-command toast, got active=%v text=%q", a.toastState.active, a.toastState.text)
	}
}

// TestHistoryFilePersistsViaCmd: submitting a prompt records it in the ring
// and returns a tea.Cmd that, when run (off the UI goroutine), appends the
// entry to the on-disk ring so a later session reloads it. We run the Cmd
// directly to exercise the file write without touching the UI loop.
func TestHistoryFilePersistsViaCmd(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history", "ring.txt")
	reg := tools.New()
	pol := permissions.New(permissions.ModeDefault, t.TempDir(), nil, nil, nil)
	ag := agent.New(nil, reg, pol, "deepseek-v4-flash")
	a := New(Config{Agent: ag, Model: "deepseek-v4-flash", Cwd: t.TempDir(), HistoryPath: path})

	cmd := a.recordHistory("persist me")
	if cmd == nil {
		t.Fatal("recordHistory with a path should return a persistence Cmd")
	}
	// Running the Cmd performs the file write (this is what tea schedules off
	// the UI goroutine).
	_ = cmd()

	got := loadPromptHistory(path, 500)
	if len(got) != 1 || got[0] != "persist me" {
		t.Fatalf("history file round-trip = %v, want [persist me]", got)
	}

	// A fresh App seeded from the same path recalls the entry.
	a2 := New(Config{Agent: ag, Model: "deepseek-v4-flash", Cwd: t.TempDir(), HistoryPath: path})
	ents := a2.history.Entries()
	if len(ents) != 1 || ents[0] != "persist me" {
		t.Fatalf("reload from file = %v, want [persist me]", ents)
	}
}

// TestHistoryEmptyPathNoCmd: with no history path (ephemeral mode) the ring
// still records in memory but no file Cmd is scheduled.
func TestHistoryEmptyPathNoCmd(t *testing.T) {
	reg := tools.New()
	pol := permissions.New(permissions.ModeDefault, t.TempDir(), nil, nil, nil)
	ag := agent.New(nil, reg, pol, "deepseek-v4-flash")
	a := New(Config{Agent: ag, Model: "deepseek-v4-flash", Cwd: t.TempDir()})

	if cmd := a.recordHistory("in memory only"); cmd != nil {
		t.Fatal("an empty history path must not schedule a file write")
	}
	ents := a.history.Entries()
	if len(ents) != 1 || ents[0] != "in memory only" {
		t.Fatalf("in-memory ring = %v, want [in memory only]", ents)
	}
}
