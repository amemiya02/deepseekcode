package repair

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/amemiya02/deepseekcode/internal/llm"
)

// CorpusCase is one entry in an offline repair corpus (JSONL).
type CorpusCase struct {
	Name      string         `json:"name"`
	Reasoning string         `json:"reasoning"`
	Content   string         `json:"content"`
	Declared  []llm.ToolCall `json:"declared"`
	Allowed   []string       `json:"allowed"`
	WantCalls int            `json:"want_calls"`
	WantKinds []Kind         `json:"want_kinds"`
}

// CorpusResult is the outcome of running one corpus case.
type CorpusResult struct {
	Name    string
	Passed  bool
	Reports []Report
	Err     error
}

// RunCorpus reads newline-delimited JSON corpus cases from r and
// exercises the repair pipeline on each one.
func RunCorpus(r io.Reader) ([]CorpusResult, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	var results []CorpusResult
	lineNo := 0
	for sc.Scan() {
		lineNo++
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var c CorpusCase
		if err := json.Unmarshal([]byte(line), &c); err != nil {
			return nil, fmt.Errorf("line %d: %w", lineNo, err)
		}
		if c.Name == "" {
			c.Name = fmt.Sprintf("case-%d", lineNo)
		}
		res := runOneCorpusCase(c)
		results = append(results, res)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return results, nil
}

func runOneCorpusCase(c CorpusCase) CorpusResult {
	res := CorpusResult{Name: c.Name}

	allowed := make(map[string]struct{}, len(c.Allowed))
	for _, name := range c.Allowed {
		allowed[name] = struct{}{}
	}

	// Scavenge tool calls from reasoning/content.
	scav := ScavengeToolCalls(c.Reasoning, c.Content, allowed, ScavengeOptions{})
	calls := scav.Calls
	res.Reports = append(res.Reports, scav.Reports...)

	// Merge declared calls.
	calls = append(calls, c.Declared...)

	// Run RepairJSONArgs on every call.
	for _, call := range calls {
		repair := RepairJSONArgs(call.Function.Arguments)
		if repair.Changed {
			res.Reports = append(res.Reports, Report{
				Kind:    KindArgsCompleted,
				Tool:    call.Function.Name,
				CallID:  call.ID,
				Message: "arguments repaired",
			})
		}
		if !repair.Valid {
			res.Reports = append(res.Reports, Report{
				Kind:    KindArgsInvalid,
				Tool:    call.Function.Name,
				CallID:  call.ID,
				Message: "arguments invalid",
			})
		}
	}

	// Check pass/fail: call count and kind multiset.
	if len(calls) != c.WantCalls {
		res.Passed = false
		return res
	}
	gotKinds := make(map[Kind]int)
	for _, r := range res.Reports {
		gotKinds[r.Kind]++
	}
	wantKinds := make(map[Kind]int)
	for _, k := range c.WantKinds {
		wantKinds[k]++
	}
	res.Passed = len(gotKinds) == len(wantKinds)
	if res.Passed {
		for k, v := range wantKinds {
			if gotKinds[k] != v {
				res.Passed = false
				break
			}
		}
	}
	return res
}
