package tools

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func BenchmarkClassifyBash(b *testing.B) {
	commands := []string{
		"git status",
		"go test -race ./...",
		"rm -rf node_modules",
		"git push --force origin main",
		"cat file.txt | grep pattern",
		"mkdir -p build && go build ./...",
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ClassifyBash(commands[i%len(commands)])
	}
}

func BenchmarkLevenshteinLines(b *testing.B) {
	a := strings.Split(strings.Repeat("line\n", 50), "\n")
	c := make([]string, len(a))
	copy(c, a)
	c[25] = "modified_line"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		LevenshteinLines(a, c)
	}
}

func BenchmarkClosestMatch(b *testing.B) {
	haystack := make([]string, 100)
	for i := range haystack {
		haystack[i] = strings.Repeat("line content\n", 5)
	}
	needle := strings.Repeat("line content\n", 4) + "different\n"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ClosestMatch(needle, haystack, 3)
	}
}

func BenchmarkTokenize(b *testing.B) {
	cmd := "git push --force-with-lease origin main --tags -v --some-long-flag=value"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tokenize(cmd)
	}
}

func BenchmarkHasRedirect(b *testing.B) {
	cmds := []string{
		`echo "hello world" > file.txt`,
		`echo "a > b"`,
		`cat file | grep pattern >> output.log`,
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		hasRedirect(cmds[i%len(cmds)])
	}
}

func BenchmarkEditFileExecute(b *testing.B) {
	dir := b.TempDir()
	content := strings.Repeat("hello world\n", 100)
	path := dir + "/bench.txt"
	b.ResetTimer()
	for i := 0; i < b.N; {
		b.StopTimer()
		_ = os.WriteFile(path, []byte(content), 0o644)
		b.StartTimer()
		_, _ = (&EditFile{CWD: dir}).Execute(nil, mustBenchJSON(map[string]any{
			"path":       path,
			"old_string": "hello world",
			"new_string": "hello benchmark",
		}))
		i++
	}
}

func mustBenchJSON(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}
