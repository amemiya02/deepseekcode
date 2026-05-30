package repair

import (
	"strings"
	"testing"
)

func TestRunCorpus(t *testing.T) {
	input := `{"name":"hidden-read","content":"{\"name\":\"read_file\",\"arguments\":{\"path\":\"README.md\"}}","allowed":["read_file"],"want_calls":1,"want_kinds":[]}
{"name":"truncated-args","declared":[{"id":"d1","type":"function","function":{"name":"write_file","arguments":"{\"path\":\"f.go\",\"cont"}}],"allowed":["write_file"],"want_calls":1,"want_kinds":["args_completed","args_invalid"]}
{"name":"invalid-args","declared":[{"id":"d2","type":"function","function":{"name":"bash","arguments":"not-json"}}],"allowed":["bash"],"want_calls":1,"want_kinds":["args_invalid"]}
`
	results, err := RunCorpus(strings.NewReader(input))
	if err != nil {
		t.Fatalf("RunCorpus: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("got %d results, want 3", len(results))
	}
	for _, r := range results {
		if r.Err != nil {
			t.Errorf("%s: unexpected error: %v", r.Name, r.Err)
		}
		if !r.Passed {
			t.Errorf("%s: expected PASS, got FAIL (reports=%+v)", r.Name, r.Reports)
		}
	}
}

func TestRunCorpusMalformedLine(t *testing.T) {
	input := `{"name":"ok","allowed":[],"want_calls":0,"want_kinds":[]}
{bad json
`
	_, err := RunCorpus(strings.NewReader(input))
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}
	if !strings.Contains(err.Error(), "line") {
		t.Errorf("error %q should contain 'line'", err.Error())
	}
}

func TestRunCorpusEmpty(t *testing.T) {
	results, err := RunCorpus(strings.NewReader(""))
	if err != nil {
		t.Fatalf("RunCorpus: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("got %d results, want 0", len(results))
	}
}
