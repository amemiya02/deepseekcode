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

func (cpStubAgent) Steer(_ string) {}

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

func TestBranchCreatesChild(t *testing.T) {
	ts, store, sid, _ := newCheckpointServer(t)

	body := `{"session_id":"` + sid + `","branch_point":2}`
	resp, err := http.Post(ts.URL+"/v1/branch", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var out struct {
		SessionID   string `json:"session_id"`
		ParentID    string `json:"parent_id"`
		BranchPoint int    `json:"branch_point"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.ParentID != sid {
		t.Errorf("parent_id = %q, want %q", out.ParentID, sid)
	}
	if out.BranchPoint != 2 {
		t.Errorf("branch_point = %d, want 2", out.BranchPoint)
	}
	child, err := store.GetSession(context.Background(), out.SessionID)
	if err != nil {
		t.Fatalf("get child: %v", err)
	}
	if child.ParentID != sid || child.BranchPoint != 2 {
		t.Errorf("child = {parent:%q bp:%d}, want {%q 2}", child.ParentID, child.BranchPoint, sid)
	}
}

func TestForkBranchesAtEnd(t *testing.T) {
	ts, store, sid, _ := newCheckpointServer(t)

	resp, err := http.Post(ts.URL+"/v1/fork", "application/json",
		strings.NewReader(`{"session_id":"`+sid+`"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var out struct {
		SessionID   string `json:"session_id"`
		BranchPoint int    `json:"branch_point"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.BranchPoint != 4 { // seed has 4 messages
		t.Errorf("branch_point = %d, want 4", out.BranchPoint)
	}
	// The fork shares all 4 messages via Replay (no copy).
	msgs, err := store.Replay(context.Background(), out.SessionID)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if len(msgs) != 4 {
		t.Errorf("replayed %d messages, want 4", len(msgs))
	}
}

func TestSwitchReturnsTranscript(t *testing.T) {
	ts, _, sid, _ := newCheckpointServer(t)

	resp, err := http.Post(ts.URL+"/v1/switch", "application/json",
		strings.NewReader(`{"session_id":"`+sid+`"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var out struct {
		SessionID string `json:"session_id"`
		Messages  []struct {
			Role string `json:"role"`
			Text string `json:"text"`
		} `json:"messages"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.SessionID != sid {
		t.Errorf("session_id = %q, want %q", out.SessionID, sid)
	}
	if len(out.Messages) != 4 {
		t.Fatalf("messages = %d, want 4", len(out.Messages))
	}
	if out.Messages[0].Role != "user" {
		t.Errorf("first role = %q, want user", out.Messages[0].Role)
	}
}

func TestSummarizeUpToCollapsesRange(t *testing.T) {
	ts, store, sid, _ := newCheckpointServer(t)

	// Collapse messages [0,3) into one summary row: "up to" message index 3.
	body := `{"session_id":"` + sid + `","mode":"upto","index":3,"summary":"earlier work"}`
	resp, err := http.Post(ts.URL+"/v1/summarize", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var out struct {
		SummaryIdx int `json:"summary_idx"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.SummaryIdx != 0 {
		t.Errorf("summary_idx = %d, want 0", out.SummaryIdx)
	}
	// 4 originals → [0,3) collapsed to 1 + the trailing message = 2 rows.
	n, err := store.CountMessages(context.Background(), sid)
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Errorf("messages left = %d, want 2", n)
	}
}

func TestDefaultHandlerWiresStore(t *testing.T) {
	t.Setenv("HOME", t.TempDir()) // isolate ~/.deepseek/sessions.db
	h, err := gateway.DefaultHandler()
	if err != nil {
		t.Fatalf("default handler: %v", err)
	}
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)

	resp, err := http.Post(ts.URL+"/v1/switch", "application/json",
		strings.NewReader(`{"session_id":"nope"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	// Store present → unknown session is 404 (Replay fails), NOT 501 (no store).
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 (store wired), got %d", resp.StatusCode)
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
