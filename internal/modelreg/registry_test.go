package modelreg

import (
	"context"
	"testing"
	"time"

	"github.com/amemiya02/deepseekcode/internal/config"
	"github.com/amemiya02/deepseekcode/internal/llm"
)

type fakeWriter struct {
	activeP   string
	modelOf   map[string]string
	failWrite bool
}

func newFakeWriter() *fakeWriter { return &fakeWriter{modelOf: map[string]string{}} }
func (w *fakeWriter) SetActiveProvider(n string) error {
	if w.failWrite {
		return context.DeadlineExceeded
	}
	w.activeP = n
	return nil
}
func (w *fakeWriter) SetProviderModel(p, m string) error {
	if w.failWrite {
		return context.DeadlineExceeded
	}
	w.modelOf[p] = m
	return nil
}

func testRegistry(t *testing.T, w ConfigWriter) *Registry {
	t.Helper()
	now := time.Unix(0, 0)
	return New(cfgWith(), Options{
		Fetcher: &fakeFetcher{models: []string{"mimo-pro", "mimo-flash"}},
		Writer:  w,
		Builder: func(_ config.Config, name string) (BuildResult, error) {
			caps, _ := llm.ProviderCapabilities(providerType(name, cfgWith().Providers[name]))
			return BuildResult{Client: &llm.Client{}, Caps: caps}, nil
		},
		TTL: time.Minute,
		Now: func() time.Time { return now },
	})
}

func TestListSpansAllProviders(t *testing.T) {
	r := testRegistry(t, newFakeWriter())
	all, err := r.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	var hasDS, hasMimo bool
	for _, m := range all {
		hasDS = hasDS || m.Provider == "deepseek"
		hasMimo = hasMimo || (m.Provider == "mimo" && m.ID == "mimo-flash")
	}
	if !hasDS || !hasMimo {
		t.Fatalf("List missing providers: %+v", all)
	}
}

func TestSwitchPersistsAndReturnsClient(t *testing.T) {
	w := newFakeWriter()
	r := testRegistry(t, w)
	res, err := r.Switch(context.Background(), "deepseek", "deepseek-v4-pro")
	if err != nil {
		t.Fatalf("Switch: %v", err)
	}
	if res.Client == nil || res.Model != "deepseek-v4-pro" || res.Provider != "deepseek" {
		t.Fatalf("res = %+v", res)
	}
	if w.activeP != "deepseek" || w.modelOf["deepseek"] != "deepseek-v4-pro" {
		t.Fatalf("not persisted: active=%q models=%v", w.activeP, w.modelOf)
	}
	if r.Active().Model != "deepseek-v4-pro" {
		t.Fatalf("Active not updated: %+v", r.Active())
	}
}

func TestSwitchUnknownModelErrorsNoSideEffects(t *testing.T) {
	w := newFakeWriter()
	r := testRegistry(t, w)
	if _, err := r.Switch(context.Background(), "mimo", "nope"); err == nil {
		t.Fatalf("expected error for unknown model")
	}
	if w.activeP != "" || len(w.modelOf) != 0 {
		t.Fatalf("unknown switch should not persist: %q %v", w.activeP, w.modelOf)
	}
}

func TestSwitchWriteFailWarnsButStillSwaps(t *testing.T) {
	w := newFakeWriter()
	w.failWrite = true
	r := testRegistry(t, w)
	res, err := r.Switch(context.Background(), "deepseek", "deepseek-v4-pro")
	if err != nil {
		t.Fatalf("Switch should not hard-fail on write error: %v", err)
	}
	if res.Warning == "" || res.Client == nil {
		t.Fatalf("expected warning + live client, got %+v", res)
	}
}
