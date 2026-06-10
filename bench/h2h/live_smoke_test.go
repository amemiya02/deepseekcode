package h2h

import (
	"context"
	"os"
	"testing"
)

// TestLiveReasonixUsageSmoke drives the REAL reasonix binary through
// the usage proxy with one trivial prompt and asserts that provider
// usage is captured. Guarded: needs H2H_LIVE_SMOKE=1, a reasonix
// binary path in H2H_REASONIX_BIN, and REASONIX_BENCH_API_KEY. Costs
// one tiny billed completion; run after any reasonix upgrade.
func TestLiveReasonixUsageSmoke(t *testing.T) {
	if os.Getenv("H2H_LIVE_SMOKE") != "1" {
		t.Skip("set H2H_LIVE_SMOKE=1 (plus H2H_REASONIX_BIN, REASONIX_BENCH_API_KEY) to run")
	}
	bin := os.Getenv("H2H_REASONIX_BIN")
	if bin == "" || os.Getenv("REASONIX_BENCH_API_KEY") == "" {
		t.Fatal("H2H_LIVE_SMOKE=1 but H2H_REASONIX_BIN or REASONIX_BENCH_API_KEY unset")
	}
	ws := &Workspace{Dir: t.TempDir()}
	task := TaskSpec{ID: "smoke", Prompt: "Reply with exactly: hi", WallclockCapMin: 3}
	res, err := RunReasonix(context.Background(), bin, task, ws)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("turns=%d hit=%.1f%% billable=%d dnf=%v err=%q",
		len(res.Turns), 100*res.HitRate(), res.Billable(), res.DNF, res.Err)
	if res.DNF {
		t.Fatalf("smoke run DNF: %s", res.Err)
	}
	if len(res.Turns) == 0 || res.Billable() == 0 {
		t.Fatalf("no usage captured: turns=%+v err=%q", res.Turns, res.Err)
	}
}
