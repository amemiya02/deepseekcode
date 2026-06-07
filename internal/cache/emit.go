package cache

import "fmt"

// ReceiptLine renders a one-line, human-readable summary of a receipt for
// `dsc trace inspect` and the Cost HUD. Stable format -> safe to assert in tests.
func ReceiptLine(r CacheReceipt) string {
	return fmt.Sprintf("turn=%d epoch=%d cause=%s hit=%d miss=%d residual=%d cost=¥%.4f saved=¥%.4f",
		r.Turn, r.Epoch, r.Dominant, r.HitTokens, r.MissTokens, r.ResidualEst, r.CostCNY, r.SavedCNY)
}
