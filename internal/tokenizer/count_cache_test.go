package tokenizer

import (
	"strings"
	"sync"
	"testing"

	"github.com/amemiya02/deepseekcode/internal/llm"
)

// resetStatsForTest zeroes only the hit/miss counters, preserving cached
// entries. TEST-ONLY. Use this to measure the delta from a single call.
func resetStatsForTest() {
	cacheMu.Lock()
	defer cacheMu.Unlock()
	cacheHits = 0
	cacheMisses = 0
}

// corpus is a ≥12-case test corpus covering ASCII, CJK, fenced code,
// DSML tool_calls, tool_result, special tokens, and a >256KB input.
var corpus = []struct {
	name string
	text string
}{
	{"empty", ""},
	{"ascii_hello", "Hello, world!"},
	{"cjk", "你好世界，这是一个测试。"},
	{"mixed_en_cn", "The answer is 42，答案是四十二。"},
	{"emoji", "emoji 😀 in sentence"},
	{"fenced_code", "```go\nfunc main() {\n\tfmt.Println(\"hello\")\n}\n```"},
	{"dsml_tool_calls", "<｜DSML｜tool_calls><｜tool▁call▁begin｜>function<｜tool▁name▁begin｜>bash<｜tool▁name▁end｜><｜tool▁call▁argument▁begin｜>{\"command\":\"ls -la\"}<｜tool▁call▁argument▁end｜><｜tool▁call▁end｜>"},
	{"tool_result", "<tool_result>\n<name>bash</name>\n<output>total 0\ndrwxr-xr-x  2 user  staff  64 Jan  1 00:00 .\n</output>\n</tool_result>"},
	{"user_marker", "<｜User｜>hello"},
	{"assistant_think", "<｜Assistant｜><think>\nreasoning here\n</think>\nanswer here"},
	{"long_paragraph", strings.Repeat("The quick brown fox jumps over the lazy dog. ", 200)},
	{"code_plus_text", "package main\n\nimport \"fmt\"\n\n// This function prints a greeting\nfunc greet(name string) {\n\tfmt.Printf(\"Hello, %s!\\n\", name)\n}\n\nfunc main() {\n\tgreet(\"World\")\n}\n"},
	{"special_tokens_mix", "<｜begin▁of▁sentence｜>System prompt<｜end▁of▁sentence｜><｜User｜>What is Go?<｜Assistant｜>Go is a programming language.<｜end▁of▁sentence｜>"},
	{"unicode_mixed", "日本語テスト 🎌 Über café naïve résumé"},
}

// buildLarge256KB returns a >256KB string mixing CJK and code.
func buildLarge256KB() string {
	var b strings.Builder
	code := "func compute(x int) int {\n\tif x < 0 {\n\t\treturn -x\n\t}\n\treturn x * 2\n}\n"
	cjk := "这是一段中文文本，用于测试分词器的处理能力。"
	// Each iteration ~135 bytes; 2000 iterations ≈ 270KB
	for i := 0; i < 2000; i++ {
		b.WriteString(code)
		b.WriteString(cjk)
	}
	return b.String()
}

func TestCountExactEquivalence(t *testing.T) {
	if !Available() {
		t.Skip("tokenizer not available")
	}

	for _, tc := range corpus {
		t.Run(tc.name, func(t *testing.T) {
			got := CountExact(tc.text)
			want := Count(tc.text)
			if got != want {
				t.Errorf("CountExact(%q) = %d, Count(%q) = %d", tc.text, got, tc.text, want)
			}
		})
	}

	// >256KB case
	t.Run("large_256kb", func(t *testing.T) {
		large := buildLarge256KB()
		if len(large) < 256*1024 {
			t.Fatalf("large input is only %d bytes", len(large))
		}
		got := CountExact(large)
		want := Count(large)
		if got != want {
			t.Errorf("CountExact(large %d bytes) = %d, Count = %d", len(large), got, want)
		}
	})
}

func TestCountExactCacheHit(t *testing.T) {
	if !Available() {
		t.Skip("tokenizer not available")
	}

	resetCacheForTest()

	// Build a ~50KB string.
	text := strings.Repeat("Hello, world! This is a cache test. 你好世界。", 2000)
	if len(text) < 50000 {
		t.Fatalf("test string is only %d bytes", len(text))
	}

	// First count: should cause misses.
	_ = CountExact(text)
	_, misses1, _ := cacheStatsForTest()
	if misses1 == 0 {
		t.Fatal("expected misses > 0 on first count")
	}

	// Second count: misses should NOT increase.
	_ = CountExact(text)
	_, misses2, _ := cacheStatsForTest()
	if misses2 != misses1 {
		t.Errorf("misses increased on second count: %d → %d (delta should be 0)", misses1, misses2)
	}
}

func TestCountExactConcurrent(t *testing.T) {
	if !Available() {
		t.Skip("tokenizer not available")
	}

	resetCacheForTest()

	text := "Concurrent safety test string with some 中文 and code: func main() {}"
	var wg sync.WaitGroup
	n := 50
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			_ = CountExact(text)
		}()
	}
	wg.Wait()
	// No race detector failure = pass. Also verify we got a reasonable count.
	c := CountExact(text)
	if c == 0 {
		t.Error("CountExact returned 0 for non-empty text")
	}
}

func BenchmarkCountMessages(b *testing.B) {
	if !Available() {
		b.Skip("tokenizer not available")
	}

	// Build a ~200-turn synthetic conversation text.
	var parts []string
	parts = append(parts, "<｜begin▁of▁sentence｜>You are a helpful assistant.<｜end▁of▁sentence｜>")
	for i := 0; i < 200; i++ {
		parts = append(parts, "<｜User｜>Turn "+strings.Repeat("x", 50)+"<｜end▁of▁sentence｜>")
		parts = append(parts, "<｜Assistant｜>Response "+strings.Repeat("y", 100)+"<｜end▁of▁sentence｜>")
	}
	text := strings.Join(parts, "")

	b.Run("CountExact", func(b *testing.B) {
		resetCacheForTest()
		for i := 0; i < b.N; i++ {
			_ = CountExact(text)
		}
	})
	b.Run("Count_full", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = Count(text)
		}
	})
	b.Run("CountBounded", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = CountBounded(text, DefaultBoundedTokenizeChars)
		}
	})
}

// TestCountMessagesCacheBehavior proves O(changed-tail): after counting a
// 100-message conversation, resetting stats (not cache), and appending one
// message, the miss-count delta equals the new runs from the appended message
// (the unchanged prefix is all cache hits).
func TestCountMessagesCacheBehavior(t *testing.T) {
	if !Available() {
		t.Skip("tokenizer not available")
	}

	// Build a 100-message conversation.
	msgs := make([]llm.Message, 0, 201)
	msgs = append(msgs, llm.Message{Role: "system", Blocks: []llm.ContentBlock{
		llm.TextBlock{Text: "You are a helpful assistant."},
	}})
	for i := 0; i < 100; i++ {
		msgs = append(msgs, llm.Message{Role: "user", Blocks: []llm.ContentBlock{
			llm.TextBlock{Text: strings.Repeat("x", 50)},
		}})
		msgs = append(msgs, llm.Message{Role: "assistant", Blocks: []llm.ContentBlock{
			llm.TextBlock{Text: strings.Repeat("y", 100)},
		}})
	}

	// Warm the cache and measure baseline misses.
	_, ok := CountMessages(msgs)
	if !ok {
		t.Fatal("CountMessages returned ok=false")
	}
	// Baseline stats (not needed for assertion, but verified non-zero).
	_, missesBaseline, _ := cacheStatsForTest()
	if missesBaseline == 0 {
		t.Fatal("expected baseline misses > 0 after warming cache")
	}

	// Reset stats only (cache entries stay) so we can measure the delta.
	resetStatsForTest()

	// Append ONE user message.
	msgs = append(msgs, llm.Message{Role: "user", Blocks: []llm.ContentBlock{
		llm.TextBlock{Text: "new content here"},
	}})

	_, ok = CountMessages(msgs)
	if !ok {
		t.Fatal("CountMessages returned ok=false on appended convo")
	}

	_, missesAfter, _ := cacheStatsForTest()

	// The appended message contributes 1 new non-added run: "new content here".
	// The "<think>" suffix from FormatPrompt's assistant-turn placeholder is
	// matched by addedPattern as an added token (count=1, no run encoding).
	// The "<｜User｜>" and "<｜Assistant｜>" markers are also added tokens.
	// The unchanged prefix is all cache hits → misses delta == 1.
	if missesAfter != 1 {
		t.Errorf("expected 1 new cache miss from appended message, got %d", missesAfter)
	}
}

// TestCountMessagesExactProvesEquality verifies CountMessages(msgs) ==
// Count(FormatPrompt(msgs,false)) for conversations larger than the old 2KB cap.
func TestCountMessagesExactProvesEquality(t *testing.T) {
	if !Available() {
		t.Skip("tokenizer not available")
	}

	// Build a >2KB conversation.
	msgs := make([]llm.Message, 0, 41)
	msgs = append(msgs, llm.Message{Role: "system", Blocks: []llm.ContentBlock{
		llm.TextBlock{Text: "You are a coding agent."},
	}})
	for i := 0; i < 20; i++ {
		msgs = append(msgs, llm.Message{Role: "user", Blocks: []llm.ContentBlock{
			llm.TextBlock{Text: strings.Repeat("x", 100)},
		}})
		msgs = append(msgs, llm.Message{Role: "assistant", Blocks: []llm.ContentBlock{
			llm.TextBlock{Text: strings.Repeat("y", 200)},
		}})
	}

	prompt := FormatPrompt(msgs, false)
	if len(prompt) <= 2048 {
		t.Fatalf("prompt is only %d bytes, want >2048", len(prompt))
	}

	cm, ok := CountMessages(msgs)
	if !ok {
		t.Fatal("CountMessages returned ok=false")
	}
	exact := Count(prompt)
	if cm != exact {
		t.Errorf("CountMessages=%d, Count(FormatPrompt)=%d — should be exact equal", cm, exact)
	}

	// Build a >256KB conversation.
	bigMsgs := make([]llm.Message, 0, 501)
	bigMsgs = append(bigMsgs, llm.Message{Role: "system", Blocks: []llm.ContentBlock{
		llm.TextBlock{Text: "System."},
	}})
	for i := 0; i < 250; i++ {
		bigMsgs = append(bigMsgs, llm.Message{Role: "user", Blocks: []llm.ContentBlock{
			llm.TextBlock{Text: strings.Repeat("x", 500)},
		}})
		bigMsgs = append(bigMsgs, llm.Message{Role: "assistant", Blocks: []llm.ContentBlock{
			llm.TextBlock{Text: strings.Repeat("y", 500)},
		}})
	}

	bigPrompt := FormatPrompt(bigMsgs, false)
	if len(bigPrompt) <= 256*1024 {
		t.Fatalf("big prompt is only %d bytes, want >256KB", len(bigPrompt))
	}

	bigCM, ok := CountMessages(bigMsgs)
	if !ok {
		t.Fatal("CountMessages returned ok=false for big convo")
	}
	bigExact := Count(bigPrompt)
	if bigCM != bigExact {
		t.Errorf("big: CountMessages=%d, Count(FormatPrompt)=%d — should be exact equal", bigCM, bigExact)
	}
}
