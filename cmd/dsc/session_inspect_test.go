package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/amemiya02/deepseekcode/internal/session"
)

func TestRenderSessionReceipts(t *testing.T) {
	receipts := []session.TranscriptReceipt{
		{Seq: 1, Kind: session.ReceiptEpoch, Payload: json.RawMessage(`{"epoch_id":"e1","static_prefix_hash":"abcdef"}`)},
		{Seq: 2, Kind: session.ReceiptModelFinal, Payload: json.RawMessage(`{"cache_hit_tokens":10,"cache_miss_tokens":90,"cost_cny":0.001}`)},
	}
	got := renderSessionReceipts(receipts)
	if !strings.Contains(got, "1 epoch") || !strings.Contains(got, "2 model_final") {
		t.Fatalf("unexpected receipt render:\n%s", got)
	}
	if !strings.Contains(got, "abcdef") || !strings.Contains(got, "cache_miss_tokens") {
		t.Fatalf("payload missing:\n%s", got)
	}
}
