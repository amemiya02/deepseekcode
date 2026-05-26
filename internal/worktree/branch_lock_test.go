package worktree

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestBranchLockSequential(t *testing.T) {
	bl := NewBranchLock()
	ctx := context.Background()

	// Acquire then release — TryAcquire should succeed.
	r1, err := bl.Acquire(ctx, "x")
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}

	// TryAcquire while held should fail.
	_, err = bl.TryAcquire("x")
	if err != ErrBranchBusy {
		t.Errorf("expected ErrBranchBusy, got %v", err)
	}

	r1()

	// After release, TryAcquire should succeed.
	r2, err := bl.TryAcquire("x")
	if err != nil {
		t.Fatalf("TryAcquire after release: %v", err)
	}
	r2()
}

func TestBranchLockConcurrent(t *testing.T) {
	bl := NewBranchLock()
	ctx := context.Background()

	var mu sync.Mutex
	var order []string

	// A acquires first.
	rA, err := bl.Acquire(ctx, "x")
	if err != nil {
		t.Fatalf("A Acquire: %v", err)
	}

	// B tries to acquire in a goroutine — should block.
	done := make(chan struct{})
	go func() {
		rB, err := bl.Acquire(ctx, "x")
		if err != nil {
			t.Errorf("B Acquire: %v", err)
		}
		mu.Lock()
		order = append(order, "B")
		mu.Unlock()
		rB()
		close(done)
	}()

	// Give B time to queue.
	time.Sleep(50 * time.Millisecond)

	// B should be blocked — TryAcquire should still fail.
	_, err = bl.TryAcquire("x")
	if err != ErrBranchBusy {
		t.Errorf("TryAcquire while A holds: expected ErrBranchBusy, got %v", err)
	}

	// A releases.
	mu.Lock()
	order = append(order, "A")
	mu.Unlock()
	rA()

	// Wait for B to finish.
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for B")
	}

	// A should finish before B.
	if order[0] != "A" || order[1] != "B" {
		t.Errorf("order = %v, want [A B]", order)
	}
}

func TestBranchLockContextCancel(t *testing.T) {
	bl := NewBranchLock()

	// A holds the lock.
	rA, err := bl.Acquire(context.Background(), "x")
	if err != nil {
		t.Fatalf("A Acquire: %v", err)
	}
	defer rA()

	// B tries to acquire with a cancellable context.
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		_, err := bl.Acquire(ctx, "x")
		done <- err
	}()

	// Cancel after a short delay.
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != context.Canceled {
			t.Errorf("expected context.Canceled, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for cancelled Acquire")
	}

	// After B is cancelled and A releases, TryAcquire should work
	// (lock should not be stuck).
	rA()
	rC, err := bl.TryAcquire("x")
	if err != nil {
		t.Fatalf("TryAcquire after cancel+release: %v", err)
	}
	rC()
}

func TestBranchLockReleaseIdempotent(t *testing.T) {
	bl := NewBranchLock()
	ctx := context.Background()

	r, err := bl.Acquire(ctx, "x")
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}

	// Release multiple times — should not panic.
	r()
	r()
	r()

	// Lock should be available.
	r2, err := bl.TryAcquire("x")
	if err != nil {
		t.Fatalf("TryAcquire after multiple releases: %v", err)
	}
	r2()
}

func TestBranchLockTryAcquireBusy(t *testing.T) {
	bl := NewBranchLock()
	ctx := context.Background()

	r, err := bl.Acquire(ctx, "x")
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	defer r()

	// TryAcquire should immediately return ErrBranchBusy.
	_, err = bl.TryAcquire("x")
	if err != ErrBranchBusy {
		t.Errorf("expected ErrBranchBusy, got %v", err)
	}

	// Different branch should be fine.
	r2, err := bl.TryAcquire("y")
	if err != nil {
		t.Fatalf("TryAcquire on different branch: %v", err)
	}
	r2()
}

func TestBranchLockDifferentBranchesIndependent(t *testing.T) {
	bl := NewBranchLock()
	ctx := context.Background()

	r1, err := bl.Acquire(ctx, "feat/a")
	if err != nil {
		t.Fatalf("Acquire feat/a: %v", err)
	}
	r2, err := bl.Acquire(ctx, "feat/b")
	if err != nil {
		t.Fatalf("Acquire feat/b: %v", err)
	}

	// TryAcquire on "feat/a" should fail — still held.
	_, err = bl.TryAcquire("feat/a")
	if err != ErrBranchBusy {
		t.Errorf("expected ErrBranchBusy for feat/a, got %v", err)
	}

	// "feat/b" also busy.
	_, err = bl.TryAcquire("feat/b")
	if err != ErrBranchBusy {
		t.Errorf("expected ErrBranchBusy for feat/b, got %v", err)
	}

	r1()
	r2()

	// Both now available.
	r3, err := bl.TryAcquire("feat/a")
	if err != nil {
		t.Fatalf("TryAcquire feat/a after release: %v", err)
	}
	r3()
	r4, err := bl.TryAcquire("feat/b")
	if err != nil {
		t.Fatalf("TryAcquire feat/b after release: %v", err)
	}
	r4()
}
