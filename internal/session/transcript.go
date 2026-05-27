package session

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// ReceiptKind identifies the type of transcript receipt.
type ReceiptKind string

const (
	ReceiptModelFinal ReceiptKind = "model_final" // model usage and prefix hash
	ReceiptRepair     ReceiptKind = "repair"       // repair report from tool-call pipeline
	ReceiptPermission ReceiptKind = "permission"   // permission decision
	ReceiptCompaction ReceiptKind = "compaction"   // compaction event
	ReceiptEpoch      ReceiptKind = "epoch"        // epoch lifecycle event
)

// TranscriptReceipt is an append-only entry in the transcript log.
type TranscriptReceipt struct {
	SessionID string
	Seq       int64
	Kind      ReceiptKind
	Payload   json.RawMessage
}

// AppendReceipt adds a transcript receipt to the session. The seq is
// auto-assigned as max(seq)+1 for the session. The payload must be
// valid JSON.
func (s *Store) AppendReceipt(ctx context.Context, sessionID string, kind ReceiptKind, payload json.RawMessage) (int64, error) {
	if !json.Valid(payload) {
		return 0, fmt.Errorf("invalid JSON payload")
	}

	// Get next sequence number
	var nextSeq int64
	err := s.db.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(seq), 0) + 1 FROM transcript_receipts WHERE session_id = ?`,
		sessionID).Scan(&nextSeq)
	if err != nil {
		return 0, fmt.Errorf("getting next seq: %w", err)
	}

	now := time.Now().UTC()
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO transcript_receipts(session_id, seq, kind, payload, created_at)
		 VALUES (?, ?, ?, ?, ?)`,
		sessionID, nextSeq, string(kind), string(payload), now.Unix())
	if err != nil {
		return 0, fmt.Errorf("inserting receipt: %w", err)
	}

	return nextSeq, nil
}

// ListReceipts returns all receipts for a session in sequence order.
// Returns an empty slice (not nil) for sessions with no receipts.
func (s *Store) ListReceipts(ctx context.Context, sessionID string) ([]TranscriptReceipt, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT session_id, seq, kind, payload
		 FROM transcript_receipts
		 WHERE session_id = ?
		 ORDER BY seq ASC`,
		sessionID)
	if err != nil {
		return nil, fmt.Errorf("querying receipts: %w", err)
	}
	defer rows.Close()

	var receipts []TranscriptReceipt
	for rows.Next() {
		var r TranscriptReceipt
		var payload string
		if err := rows.Scan(&r.SessionID, &r.Seq, &r.Kind, &payload); err != nil {
			return nil, fmt.Errorf("scanning receipt: %w", err)
		}
		r.Payload = json.RawMessage(payload)
		receipts = append(receipts, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating receipts: %w", err)
	}

	// Return empty slice instead of nil for consistency
	if receipts == nil {
		receipts = []TranscriptReceipt{}
	}
	return receipts, nil
}

// AppendReceipt adds a transcript receipt for the persister's session.
func (p *Persister) AppendReceipt(ctx context.Context, kind ReceiptKind, payload json.RawMessage) (int64, error) {
	return p.store.AppendReceipt(ctx, p.sessID, kind, payload)
}
