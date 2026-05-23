package session

import (
	"context"
	"path/filepath"
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
