package cache

import "testing"

func TestReceiptLine_Steady(t *testing.T) {
	r := CacheReceipt{Turn: 4, Epoch: 1, Dominant: CauseSteady, HitTokens: 1000, MissTokens: 44, ResidualEst: 44, CostCNY: 0.001, SavedCNY: 0.05}
	got := ReceiptLine(r)
	want := "turn=4 epoch=1 cause=steady hit=1000 miss=44 residual=44 cost=¥0.0010 saved=¥0.0500"
	if got != want {
		t.Fatalf("ReceiptLine =\n  %q\nwant\n  %q", got, want)
	}
}
