package session

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/amemiya02/deepseekcode/internal/llm"
)

func TestStoreNewAndLoad(t *testing.T) {
	dir := t.TempDir()
	store, err := Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	ctx := context.Background()
	sess, err := store.NewSession(ctx, "/proj", "deepseek-v4-flash", true)
	if err != nil {
		t.Fatal(err)
	}
	got, err := store.GetSession(ctx, sess.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Model != "deepseek-v4-flash" || got.ProjectPath != "/proj" {
		t.Fatalf("unexpected session: %+v", got)
	}
}

func TestStoreAppendAndLoadMessages(t *testing.T) {
	dir := t.TempDir()
	store, err := Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	ctx := context.Background()
	sess, _ := store.NewSession(ctx, "/p", "deepseek-v4-flash", true)

	for i, role := range []string{"user", "assistant", "tool"} {
		idx, err := store.AppendMessage(ctx, sess.ID, Message{
			Role:    role,
			Content: "msg " + role,
		})
		if err != nil {
			t.Fatal(err)
		}
		if idx != i {
			t.Fatalf("idx=%d want %d", idx, i)
		}
	}

	msgs, err := store.LoadMessages(ctx, sess.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 3 {
		t.Fatalf("got %d msgs, want 3", len(msgs))
	}
	if msgs[1].Role != "assistant" {
		t.Fatalf("msg[1].Role=%q", msgs[1].Role)
	}
}

func TestStoreBranchReplay(t *testing.T) {
	dir := t.TempDir()
	store, err := Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	ctx := context.Background()
	root, _ := store.NewSession(ctx, "/p", "deepseek-v4-flash", true)

	// Root has 5 messages.
	for i := 0; i < 5; i++ {
		_, _ = store.AppendMessage(ctx, root.ID, Message{Role: "user", Content: "r" + itoa(i)})
	}

	// Branch at message 3 (so child sees root[0..2]).
	child, err := store.NewBranch(ctx, root.ID, 3)
	if err != nil {
		t.Fatal(err)
	}
	// Child adds 2 of its own.
	for i := 0; i < 2; i++ {
		_, _ = store.AppendMessage(ctx, child.ID, Message{Role: "user", Content: "c" + itoa(i)})
	}

	replay, err := store.Replay(ctx, child.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(replay) != 5 { // 3 from root (truncated) + 2 from child
		t.Fatalf("got %d, want 5", len(replay))
	}
	if replay[0].Content != "r0" || replay[2].Content != "r2" ||
		replay[3].Content != "c0" || replay[4].Content != "c1" {
		t.Fatalf("unexpected replay: %v", contents(replay))
	}
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b []byte
	for i > 0 {
		b = append([]byte{byte('0' + i%10)}, b...)
		i /= 10
	}
	return string(b)
}

func contents(ms []Message) []string {
	out := make([]string, len(ms))
	for i, m := range ms {
		out[i] = m.Content
	}
	return out
}

// TestMigrateV1ToV2 simulates a user upgrading: hand-craft a v1 DB,
// then Open() and assert v2 columns appear and a .bak.v1 snapshot
// exists. The backup path is load-bearing — a botched ALTER without
// it would lose user history.
func TestMigrateV1ToV2(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "v1.db")

	// Hand-build a v1 DB outside the Open()/migrate() machinery so we
	// can pin the schema_version at 1 deliberately.
	dsn := dbPath + "?_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)"
	rawDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("raw open: %v", err)
	}
	if _, err := rawDB.Exec(schemaV1); err != nil {
		t.Fatalf("apply schema v1: %v", err)
	}
	if _, err := rawDB.Exec(`INSERT INTO schema_version(version) VALUES (1)`); err != nil {
		t.Fatalf("seed schema_version: %v", err)
	}
	if _, err := rawDB.Exec(`INSERT INTO sessions(id, project_path, model, duet_enabled, created_at, last_used_at) VALUES ('s1', '/p', 'm', 0, 0, 0)`); err != nil {
		t.Fatalf("seed session: %v", err)
	}
	if err := rawDB.Close(); err != nil {
		t.Fatalf("close raw: %v", err)
	}

	// Migration happens inside Open().
	s, err := Open(dbPath)
	if err != nil {
		t.Fatalf("open with migration: %v", err)
	}
	defer s.Close()

	wantSessionCols := []string{"compaction_count", "compaction_summary", "workspace_fp"}
	if missing := missingColumns(t, s.db, "sessions", wantSessionCols); len(missing) > 0 {
		t.Errorf("sessions missing columns after migrate: %v", missing)
	}
	if missing := missingColumns(t, s.db, "messages", []string{"blocks"}); len(missing) > 0 {
		t.Errorf("messages missing column after migrate: %v", missing)
	}
	if _, err := os.Stat(dbPath + ".bak.v1"); err != nil {
		t.Errorf("backup file missing: %v", err)
	}

	// schema_version table should now report 2 as the highest applied.
	var got int
	if err := s.db.QueryRow(`SELECT MAX(version) FROM schema_version`).Scan(&got); err != nil {
		t.Fatalf("read version: %v", err)
	}
	if got != 2 {
		t.Errorf("schema_version max: want 2, got %d", got)
	}
}

// TestLoadMessagesBlocksFromNew pins the fast path: a row written
// via AppendMessage with Blocks populated is read back with the
// same blocks.
func TestLoadMessagesBlocksFromNew(t *testing.T) {
	s, sessID := newStoreWithSession(t)
	ctx := context.Background()

	want := []llm.ContentBlock{
		llm.ThinkingBlock{Text: "plan"},
		llm.TextBlock{Text: "answer"},
	}
	if _, err := s.AppendMessage(ctx, sessID, Message{Role: "assistant", Blocks: want}); err != nil {
		t.Fatalf("append: %v", err)
	}
	msgs, err := s.LoadMessages(ctx, sessID)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("want 1 msg, got %d", len(msgs))
	}
	if len(msgs[0].Blocks) != 2 {
		t.Fatalf("want 2 blocks, got %d", len(msgs[0].Blocks))
	}
	if _, ok := msgs[0].Blocks[0].(llm.ThinkingBlock); !ok {
		t.Errorf("blocks[0] not ThinkingBlock: %T", msgs[0].Blocks[0])
	}
	if tb, ok := msgs[0].Blocks[1].(llm.TextBlock); !ok || tb.Text != "answer" {
		t.Errorf("blocks[1] wrong: %#v", msgs[0].Blocks[1])
	}
}

// TestLoadMessagesBlocksFromLegacy verifies the back-compat path:
// a row inserted directly with legacy columns and an empty blocks
// column is reconstructed into the typed Blocks form on load.
func TestLoadMessagesBlocksFromLegacy(t *testing.T) {
	s, sessID := newStoreWithSession(t)
	ctx := context.Background()
	now := time.Now().UTC().Unix()

	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO messages
		 (session_id, idx, role, content, reasoning_content, tool_calls,
		  tool_results, tool_call_id, model,
		  cache_hit_tokens, miss_tokens, output_tokens, cost_yuan, ts, blocks)
		 VALUES (?, 0, 'assistant', 'hi', 'think', '', '', '', '', 0, 0, 0, 0, ?, '')`,
		sessID, now); err != nil {
		t.Fatalf("legacy insert: %v", err)
	}

	msgs, err := s.LoadMessages(ctx, sessID)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("want 1 msg, got %d", len(msgs))
	}
	got := msgs[0].Blocks
	if len(got) != 2 {
		t.Fatalf("want 2 reconstructed blocks, got %d (%#v)", len(got), got)
	}
	if tb, ok := got[0].(llm.ThinkingBlock); !ok || tb.Text != "think" {
		t.Errorf("blocks[0] = %#v, want ThinkingBlock{think}", got[0])
	}
	if tb, ok := got[1].(llm.TextBlock); !ok || tb.Text != "hi" {
		t.Errorf("blocks[1] = %#v, want TextBlock{hi}", got[1])
	}
}

// TestUpgradeV1ToV2Roundtrip exercises the end-to-end migration:
// craft a v1 DB with one legacy-shape message, Open() it (triggers
// v2 migration), confirm the row reads back as typed Blocks, then
// append a Blocks-shape row and confirm both coexist correctly.
func TestUpgradeV1ToV2Roundtrip(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "v1.db")

	rawDB, err := sql.Open("sqlite", dbPath+"?_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatalf("raw open: %v", err)
	}
	if _, err := rawDB.Exec(schemaV1); err != nil {
		t.Fatalf("schema v1: %v", err)
	}
	if _, err := rawDB.Exec(`INSERT INTO schema_version(version) VALUES (1)`); err != nil {
		t.Fatalf("seed version: %v", err)
	}
	if _, err := rawDB.Exec(`INSERT INTO sessions(id, project_path, model, duet_enabled, created_at, last_used_at) VALUES ('s1', '/p', 'm', 0, 0, 0)`); err != nil {
		t.Fatalf("seed session: %v", err)
	}
	now := time.Now().UTC().Unix()
	if _, err := rawDB.Exec(
		`INSERT INTO messages
		 (session_id, idx, role, content, reasoning_content, tool_calls,
		  tool_results, tool_call_id, model,
		  cache_hit_tokens, miss_tokens, output_tokens, cost_yuan, ts)
		 VALUES ('s1', 0, 'assistant', 'hi', 'plan', '', '', '', '', 0, 0, 0, 0, ?)`, now); err != nil {
		t.Fatalf("seed legacy row: %v", err)
	}
	if err := rawDB.Close(); err != nil {
		t.Fatalf("close raw: %v", err)
	}

	s, err := Open(dbPath)
	if err != nil {
		t.Fatalf("open with migrate: %v", err)
	}
	defer s.Close()

	ctx := context.Background()
	first, err := s.LoadMessages(ctx, "s1")
	if err != nil {
		t.Fatalf("load after migrate: %v", err)
	}
	if len(first) != 1 {
		t.Fatalf("want 1 msg, got %d", len(first))
	}
	if len(first[0].Blocks) != 2 {
		t.Fatalf("want 2 reconstructed blocks, got %d: %#v", len(first[0].Blocks), first[0].Blocks)
	}
	if _, ok := first[0].Blocks[0].(llm.ThinkingBlock); !ok {
		t.Errorf("blocks[0] type: got %T", first[0].Blocks[0])
	}

	if _, err := s.AppendMessage(ctx, "s1", Message{
		Role:   "user",
		Blocks: []llm.ContentBlock{llm.TextBlock{Text: "follow-up"}},
	}); err != nil {
		t.Fatalf("append v2 row: %v", err)
	}
	all, err := s.LoadMessages(ctx, "s1")
	if err != nil {
		t.Fatalf("load after append: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("want 2 msgs, got %d", len(all))
	}
	if tb, ok := all[1].Blocks[0].(llm.TextBlock); !ok || tb.Text != "follow-up" {
		t.Errorf("appended msg blocks[0]: got %#v", all[1].Blocks[0])
	}
}

func newStoreWithSession(t *testing.T) (*Store, string) {
	t.Helper()
	dir := t.TempDir()
	s, err := Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	sess, err := s.NewSession(context.Background(), "/proj", "m", false)
	if err != nil {
		t.Fatalf("new session: %v", err)
	}
	return s, sess.ID
}

func missingColumns(t *testing.T, db *sql.DB, table string, want []string) []string {
	t.Helper()
	rows, err := db.Query(`SELECT name FROM pragma_table_info(?)`, table)
	if err != nil {
		t.Fatalf("pragma_table_info %s: %v", table, err)
	}
	defer rows.Close()
	have := map[string]bool{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan: %v", err)
		}
		have[name] = true
	}
	var missing []string
	for _, w := range want {
		if !have[w] {
			missing = append(missing, w)
		}
	}
	sort.Strings(missing)
	return missing
}
