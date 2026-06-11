package modelreg

import (
	"context"
	"sort"
	"testing"
	"time"

	"github.com/amemiya02/deepseekcode/internal/config"
	"github.com/amemiya02/deepseekcode/internal/llm"
)

func TestSurfacesShareOneCatalog(t *testing.T) {
	r := New(cfgWith(), Options{
		Fetcher: &fakeFetcher{models: []string{"mimo-pro", "mimo-flash"}},
		Builder: func(_ config.Config, name string) (BuildResult, error) {
			caps, _ := llm.ProviderCapabilities(providerType(name, cfgWith().Providers[name]))
			return BuildResult{Client: &llm.Client{}, Caps: caps}, nil
		},
		Writer: newFakeWriter(), TTL: time.Minute, Now: func() time.Time { return time.Unix(0, 0) },
	})

	// Simulate two surfaces (TUI and GUI) each calling List independently.
	rowsTUI, err := r.List(context.Background())
	if err != nil {
		t.Fatalf("List (TUI): %v", err)
	}
	rowsGUI, err := r.List(context.Background())
	if err != nil {
		t.Fatalf("List (GUI): %v", err)
	}

	tui := projectIDs(rowsTUI)
	gui := projectIDs(rowsGUI)
	sort.Strings(tui)
	sort.Strings(gui)
	if len(tui) != len(gui) {
		t.Fatalf("surface id sets differ: tui=%v gui=%v", tui, gui)
	}
	for i := range tui {
		if tui[i] != gui[i] {
			t.Fatalf("surface id mismatch at %d: %q vs %q", i, tui[i], gui[i])
		}
	}
	if !contains(tui, "mimo-flash") {
		t.Fatalf("custom provider model missing from shared catalog: %v", tui)
	}
}

func projectIDs(rows []ModelInfo) []string {
	out := make([]string, 0, len(rows))
	for _, m := range rows {
		out = append(out, m.ID)
	}
	return out
}
