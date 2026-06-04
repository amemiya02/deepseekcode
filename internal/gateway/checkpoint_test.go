package gateway_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/amemiya02/deepseekcode/internal/acp"
	"github.com/amemiya02/deepseekcode/internal/gateway"
	"github.com/amemiya02/deepseekcode/internal/session"
	"github.com/amemiya02/deepseekcode/internal/snapshots"
)

// cpStubFactory is the no-key stub agent factory reused across Wave-5 tests.
func cpStubFactory(workingDir string) (acp.AgentRunner, error) { return cpStubAgent{}, nil }

type cpStubAgent struct{}

func (cpStubAgent) Run(ctx context.Context, prompt string, onEvent func(acp.AgentEvent)) error {
	onEvent(acp.AgentEvent{Kind: acp.EventKindDone, StopReason: "end_turn"})
	return nil
}

// newCheckpointServer builds a gateway wired to a real temp Store + snapshots
// Manager rooted under t.TempDir(), plus a seeded root session with 4 messages.
// It returns the server, the store, the session id, and the snapshots root dir
// (so the code-rewind test can construct a Manager at the SAME root).
func newCheckpointServer(t *testing.T) (ts *httptest.Server, store *session.Store, sid, snapRoot string) {
	t.Helper()
	dir := t.TempDir()
	store, err := session.Open(dir + "/sessions.db")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	snapRoot = dir + "/snapshots"
	snaps := snapshots.New(snapRoot)

	ctx := context.Background()
	root, err := store.NewSession(ctx, dir, "deepseek-chat", false)
	if err != nil {
		t.Fatalf("new session: %v", err)
	}
	for i, role := range []string{"user", "assistant", "user", "assistant"} {
		if _, err := store.AppendMessage(ctx, root.ID, session.Message{Role: role, Content: "m" + string(rune('0'+i))}); err != nil {
			t.Fatalf("append: %v", err)
		}
	}

	sm := acp.NewSessionManager(cpStubFactory)
	h := gateway.NewHandler(sm, "",
		gateway.WithStore(store),
		gateway.WithSnapshots(snaps),
		gateway.WithWorkspaceRoot(dir),
	)
	ts = httptest.NewServer(h)
	t.Cleanup(ts.Close)
	return ts, store, root.ID, snapRoot
}

func TestNewHandlerWithOptionsServesCache(t *testing.T) {
	ts, _, _, _ := newCheckpointServer(t)
	resp, err := http.Get(ts.URL + "/v1/cache")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestRewindConversationTruncates(t *testing.T) {
	ts, store, sid, _ := newCheckpointServer(t)

	body := `{"session_id":"` + sid + `","keep_messages":2,"scope":"conversation"}`
	resp, err := http.Post(ts.URL+"/v1/rewind", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var out struct {
		RemovedMessages int `json:"removed_messages"`
		RestoredFiles   int `json:"restored_files"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.RemovedMessages != 2 {
		t.Errorf("removed_messages = %d, want 2", out.RemovedMessages)
	}
	n, err := store.CountMessages(context.Background(), sid)
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Errorf("messages left = %d, want 2", n)
	}
}

func TestRewindCodeRestoresSnapshot(t *testing.T) {
	ts, _, sid, snapRoot := newCheckpointServer(t)

	// Build a Manager at the SAME root the server uses so Take here and the
	// endpoint's Undo share on-disk state.
	snaps := snapshots.New(snapRoot)

	work := t.TempDir()
	f := filepath.Join(work, "tracked.txt")
	if err := os.WriteFile(f, []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Snapshot the pre-edit state for this session (stepIdx=0), then mutate the file.
	// Real API: Take(sessionID string, stepIdx int, paths []string) (int, error)
	if _, err := snaps.Take(sid, 0, []string{f}); err != nil {
		t.Fatalf("take snapshot: %v", err)
	}
	if err := os.WriteFile(f, []byte("edited"), 0o644); err != nil {
		t.Fatal(err)
	}

	body := `{"session_id":"` + sid + `","keep_messages":4,"scope":"code"}`
	resp, err := http.Post(ts.URL+"/v1/rewind", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var out struct {
		RestoredFiles int `json:"restored_files"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.RestoredFiles < 1 {
		t.Errorf("restored_files = %d, want >= 1", out.RestoredFiles)
	}
	got, err := os.ReadFile(f)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "original" {
		t.Errorf("file content = %q, want %q (snapshot not restored)", got, "original")
	}
}
