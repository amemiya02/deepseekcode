package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/amemiya02/deepseekcode/internal/session"
)

func renderSessionReceipts(receipts []session.TranscriptReceipt) string {
	var b strings.Builder
	for _, r := range receipts {
		payload := string(r.Payload)
		var compact any
		if json.Unmarshal(r.Payload, &compact) == nil {
			if pretty, err := json.Marshal(compact); err == nil {
				payload = string(pretty)
			}
		}
		fmt.Fprintf(&b, "%d %s %s\n", r.Seq, r.Kind, payload)
	}
	return b.String()
}
