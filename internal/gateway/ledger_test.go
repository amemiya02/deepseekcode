package gateway_test

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestCacheLedger(t *testing.T) {
	ts := newTestServer(t, writeFixture(t)) // reuses gateway_test.go fixture+server helpers

	resp, err := http.Get(ts.URL + "/v1/cache/ledger")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var out struct {
		Rows []struct {
			Turn         int     `json:"turn"`
			EpochID      string  `json:"epoch_id"`
			HitTokens    int     `json:"hit_tokens"`
			MissTokens   int     `json:"miss_tokens"`
			OutputTokens int     `json:"output_tokens"`
			CostCNY      float64 `json:"cost_cny"`
			Evicted      bool    `json:"evicted"`
			Why          string  `json:"why"`
		} `json:"rows"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode ledger: %v", err)
	}
	// The fixture has 3 usage turns; turn 3 is an eviction after compaction.
	if len(out.Rows) != 3 {
		t.Fatalf("rows = %d, want 3", len(out.Rows))
	}
	if out.Rows[0].Why != "expected-miss" {
		t.Errorf("row 0 why = %q, want expected-miss", out.Rows[0].Why)
	}
	if !out.Rows[2].Evicted || out.Rows[2].Why != "compaction" {
		t.Errorf("row 2 = {evicted:%v why:%q}, want {true compaction}", out.Rows[2].Evicted, out.Rows[2].Why)
	}
}

func TestCacheLedgerNoTrace(t *testing.T) {
	ts := newTestServer(t, "")
	resp, err := http.Get(ts.URL + "/v1/cache/ledger")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("no-trace ledger: got %d, want 200", resp.StatusCode)
	}
	var out struct {
		Rows []any `json:"rows"`
	}
	json.NewDecoder(resp.Body).Decode(&out)
	if len(out.Rows) != 0 {
		t.Fatalf("no-trace rows = %d, want 0", len(out.Rows))
	}
}
