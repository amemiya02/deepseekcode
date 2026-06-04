package gateway_test

import (
	"context"
	"net/http"
	"net/http/httptest"
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
