package session

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"sort"
	"testing"
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
