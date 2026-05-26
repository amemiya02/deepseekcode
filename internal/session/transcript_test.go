package session

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestTranscript_AppendAndList(t *testing.T) {
	dir := t.TempDir()
	store, err := Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	ctx := context.Background()

	// Create a session
	sess, err := store.NewSession(ctx, "/test", "test-model", false)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}

	// Append first receipt
	payload1 := json.RawMessage(`{"tool":"read_file","kind":"args_completed"}`)
	seq1, err := store.AppendReceipt(ctx, sess.ID, ReceiptRepair, payload1)
	if err != nil {
		t.Fatalf("AppendReceipt 1: %v", err)
	}
	if seq1 != 1 {
		t.Errorf("expected seq 1, got %d", seq1)
	}

	// Append second receipt
	payload2 := json.RawMessage(`{"model":"deepseek-v4","usage":{"prompt_tokens":100}}`)
	seq2, err := store.AppendReceipt(ctx, sess.ID, ReceiptModelFinal, payload2)
	if err != nil {
		t.Fatalf("AppendReceipt 2: %v", err)
	}
	if seq2 != 2 {
		t.Errorf("expected seq 2, got %d", seq2)
	}

	// List receipts
	receipts, err := store.ListReceipts(ctx, sess.ID)
	if err != nil {
		t.Fatalf("ListReceipts: %v", err)
	}

	if len(receipts) != 2 {
		t.Fatalf("expected 2 receipts, got %d", len(receipts))
	}

	// Check order and content
	if receipts[0].Seq != 1 {
		t.Errorf("receipt[0].Seq = %d, want 1", receipts[0].Seq)
	}
	if receipts[0].Kind != ReceiptRepair {
		t.Errorf("receipt[0].Kind = %q, want %q", receipts[0].Kind, ReceiptRepair)
	}
	if string(receipts[0].Payload) != string(payload1) {
		t.Errorf("receipt[0].Payload = %s, want %s", receipts[0].Payload, payload1)
	}

	if receipts[1].Seq != 2 {
		t.Errorf("receipt[1].Seq = %d, want 2", receipts[1].Seq)
	}
	if receipts[1].Kind != ReceiptModelFinal {
		t.Errorf("receipt[1].Kind = %q, want %q", receipts[1].Kind, ReceiptModelFinal)
	}
	if string(receipts[1].Payload) != string(payload2) {
		t.Errorf("receipt[1].Payload = %s, want %s", receipts[1].Payload, payload2)
	}
}

func TestTranscript_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	store, err := Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	ctx := context.Background()

	sess, err := store.NewSession(ctx, "/test", "test-model", false)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}

	// Invalid JSON should return error
	_, err = store.AppendReceipt(ctx, sess.ID, ReceiptRepair, json.RawMessage(`{invalid`))
	if err == nil {
		t.Error("expected error for invalid JSON, got nil")
	}
}

func TestTranscript_EmptySession(t *testing.T) {
	dir := t.TempDir()
	store, err := Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	ctx := context.Background()

	sess, err := store.NewSession(ctx, "/test", "test-model", false)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}

	// List receipts for session with no receipts
	receipts, err := store.ListReceipts(ctx, sess.ID)
	if err != nil {
		t.Fatalf("ListReceipts: %v", err)
	}

	if receipts == nil {
		t.Error("expected empty slice, got nil")
	}
	if len(receipts) != 0 {
		t.Errorf("expected 0 receipts, got %d", len(receipts))
	}
}

func TestTranscript_PersisterAppend(t *testing.T) {
	dir := t.TempDir()
	store, err := Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	ctx := context.Background()

	sess, err := store.NewSession(ctx, "/test", "test-model", false)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}

	p := NewPersister(store, nil, sess.ID)

	payload := json.RawMessage(`{"tool":"bash","kind":"args_completed"}`)
	seq, err := p.AppendReceipt(ctx, ReceiptRepair, payload)
	if err != nil {
		t.Fatalf("Persister.AppendReceipt: %v", err)
	}
	if seq != 1 {
		t.Errorf("expected seq 1, got %d", seq)
	}

	// Verify via store
	receipts, err := store.ListReceipts(ctx, sess.ID)
	if err != nil {
		t.Fatalf("ListReceipts: %v", err)
	}
	if len(receipts) != 1 {
		t.Fatalf("expected 1 receipt, got %d", len(receipts))
	}
	if receipts[0].Kind != ReceiptRepair {
		t.Errorf("receipt.Kind = %q, want %q", receipts[0].Kind, ReceiptRepair)
	}
}
