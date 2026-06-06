package llm

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDumpWireBody_WritesExactBytes(t *testing.T) {
	dir := t.TempDir()
	body := []byte(`{"model":"deepseek-v4-flash"}`)
	path, err := dumpWireBody(dir, 7, body)
	if err != nil {
		t.Fatal(err)
	}
	if got := filepath.Base(path); got != "turn_0007.json" {
		t.Fatalf("filename = %q, want turn_0007.json", got)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(body) {
		t.Fatalf("dumped bytes = %q, want %q", got, body)
	}
}
