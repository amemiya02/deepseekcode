package tools

import (
	"context"
	"encoding/json"
	"sort"
	"testing"
)

func TestSubset(t *testing.T) {
	reg := New()
	reg.Register(&stubTool{name: "read_file"})
	reg.Register(&stubTool{name: "bash"})
	reg.Register(&stubTool{name: "grep"})
	reg.Register(&stubTool{name: "write_file"})

	t.Run("returns matching tools sorted by name", func(t *testing.T) {
		sub := reg.Subset([]string{"grep", "read_file"})
		all := sub.All()
		if len(all) != 2 {
			t.Fatalf("expected 2 tools, got %d", len(all))
		}
		names := []string{all[0].Name(), all[1].Name()}
		expected := []string{"grep", "read_file"}
		if names[0] != expected[0] || names[1] != expected[1] {
			t.Errorf("got %v, want %v", names, expected)
		}
	})

	t.Run("nil names returns empty registry", func(t *testing.T) {
		sub := reg.Subset(nil)
		if len(sub.All()) != 0 {
			t.Errorf("expected empty, got %d", len(sub.All()))
		}
	})

	t.Run("empty names returns empty registry", func(t *testing.T) {
		sub := reg.Subset([]string{})
		if len(sub.All()) != 0 {
			t.Errorf("expected empty, got %d", len(sub.All()))
		}
	})

	t.Run("unknown names ignored", func(t *testing.T) {
		sub := reg.Subset([]string{"nonexistent", "also_missing"})
		if len(sub.All()) != 0 {
			t.Errorf("expected empty, got %d", len(sub.All()))
		}
	})

	t.Run("mixed known and unknown", func(t *testing.T) {
		sub := reg.Subset([]string{"read_file", "grep", "ls"})
		all := sub.All()
		if len(all) != 2 {
			t.Fatalf("expected 2, got %d", len(all))
		}
	})

	t.Run("duplicates are idempotent", func(t *testing.T) {
		sub := reg.Subset([]string{"grep", "grep", "grep"})
		if len(sub.All()) != 1 {
			t.Errorf("expected 1, got %d", len(sub.All()))
		}
	})

	t.Run("parent unchanged after subset", func(t *testing.T) {
		before := len(reg.All())
		_ = reg.Subset([]string{"grep"})
		after := len(reg.All())
		if before != after {
			t.Errorf("parent changed: before=%d after=%d", before, after)
		}
	})

	t.Run("subset shares tool pointers", func(t *testing.T) {
		sub := reg.Subset([]string{"grep"})
		orig, _ := reg.Get("grep")
		got, ok := sub.Get("grep")
		if !ok {
			t.Fatal("grep not in subset")
		}
		if orig != got {
			t.Error("subset should share tool pointers with parent")
		}
	})
}

func TestSubsetAllSorted(t *testing.T) {
	reg := New()
	reg.Register(&stubTool{name: "read_file"})
	reg.Register(&stubTool{name: "bash"})
	reg.Register(&stubTool{name: "grep"})
	reg.Register(&stubTool{name: "write_file"})

	sub := reg.Subset([]string{"write_file", "bash", "grep"})
	names := make([]string, 0)
	for _, tool := range sub.All() {
		names = append(names, tool.Name())
	}
	sorted := make([]string, len(names))
	copy(sorted, names)
	sort.Strings(sorted)
	for i := range names {
		if names[i] != sorted[i] {
			t.Errorf("All() not sorted: got %v, want %v", names, sorted)
			break
		}
	}
}

type stubTool struct {
	name     string
	readOnly bool
}

func (s *stubTool) Name() string        { return s.name }
func (s *stubTool) Description() string { return "stub " + s.name }
func (s *stubTool) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{}}`)
}
func (s *stubTool) Execute(_ context.Context, _ json.RawMessage) (Result, error) {
	return Result{}, nil
}
func (s *stubTool) IsReadOnly() bool { return s.readOnly }
