package gateway_test

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestSessionTimeline(t *testing.T) {
	ts := newTestServer(t, writeFixture(t)) // reuses the Task 9 fixture+server helpers
	resp, err := http.Get(ts.URL + "/v1/sessions/any/timeline")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("timeline: got %d, want 200", resp.StatusCode)
	}
	var out struct {
		Epochs []struct {
			EpochID string `json:"epoch_id"`
			Turns   []struct {
				Turn    int    `json:"turn"`
				Why     string `json:"why"`
				Evicted bool   `json:"evicted"`
			} `json:"turns"`
		} `json:"epochs"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode timeline: %v", err)
	}
	if len(out.Epochs) == 0 {
		t.Fatal("expected at least one epoch in the timeline")
	}
	// The fixture's 3 turns all belong to epoch(s); the last turn is the eviction.
	var sawEvict bool
	for _, ep := range out.Epochs {
		for _, tn := range ep.Turns {
			if tn.Evicted && tn.Why == "compaction" {
				sawEvict = true
			}
		}
	}
	if !sawEvict {
		t.Error("expected the compaction eviction turn in the timeline")
	}
}

func TestSessionTimelineNoTrace(t *testing.T) {
	ts := newTestServer(t, "")
	resp, err := http.Get(ts.URL + "/v1/sessions/any/timeline")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("no-trace timeline: got %d, want 200", resp.StatusCode)
	}
	var out struct {
		Epochs []any `json:"epochs"`
	}
	json.NewDecoder(resp.Body).Decode(&out)
	if len(out.Epochs) != 0 {
		t.Fatalf("no-trace epochs = %d, want 0", len(out.Epochs))
	}
}
