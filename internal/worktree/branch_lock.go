package worktree

import (
	"context"
	"errors"
	"sync"
)

// ErrBranchBusy is returned by TryAcquire when the branch is already locked.
var ErrBranchBusy = errors.New("branch is in use by another worktree session")

// BranchLocker is the narrow interface for branch-level mutual exclusion.
// Implemented by *BranchLock.
type BranchLocker interface {
	Acquire(ctx context.Context, branch string) (release func(), err error)
	TryAcquire(branch string) (release func(), err error)
}

// BranchLock provides in-process mutual exclusion by branch name.
// Acquire blocks until the lock is available; TryAcquire returns
// immediately with ErrBranchBusy if held. Both return a release function.
// Release is idempotent — calling it multiple times is a no-op.
// BranchLock is NOT reentrant: calling Acquire twice on the same branch
// from the same goroutine will deadlock.
//
// Lock ownership is transferred directly from one holder to the next
// waiter without an unlocked window, so TryAcquire cannot interpose
// between a release and the queued waiter acquiring.
type BranchLock struct {
	mu    sync.Mutex
	inUse map[string]bool
	waits map[string][]chan struct{}
}

var _ BranchLocker = (*BranchLock)(nil)

// NewBranchLock creates a new, empty BranchLock.
func NewBranchLock() *BranchLock {
	return &BranchLock{
		inUse: make(map[string]bool),
		waits: make(map[string][]chan struct{}),
	}
}

// Acquire blocks until the given branch is available or ctx is cancelled.
// Returns a release function that must be called exactly once to unlock.
func (b *BranchLock) Acquire(ctx context.Context, branch string) (release func(), err error) {
	b.mu.Lock()
	if !b.inUse[branch] {
		b.inUse[branch] = true
		b.mu.Unlock()
		return b.makeRelease(branch), nil
	}

	// Branch is in use — enqueue a waiter.
	ch := make(chan struct{}, 1)
	b.waits[branch] = append(b.waits[branch], ch)
	b.mu.Unlock()

	// Wait for either wake (lock transferred) or ctx cancellation.
	// When release() pops us from the queue and closes ch, both
	// <-ch and <-ctx.Done() can be ready simultaneously. Go select
	// picks randomly, so we must NOT assume which branch won.
	select {
	case <-ch:
	case <-ctx.Done():
	}

	// Determine state under lock: are we still in the queue, or did
	// release() already pop us (transferring ownership)?
	b.mu.Lock()
	if b.removeWaiter(branch, ch) {
		// Still in queue — ctx cancelled before we got the lock.
		b.mu.Unlock()
		return nil, ctx.Err()
	}
	// Not in queue — release() already transferred ownership to us.
	// inUse is still true and we hold the lock.
	b.mu.Unlock()

	if ctx.Err() != nil {
		// ctx cancelled after ownership transfer; pass it on.
		b.release(branch)
		return nil, ctx.Err()
	}
	return b.makeRelease(branch), nil
}

// TryAcquire attempts to acquire the lock non-blockingly.
// Returns ErrBranchBusy if the branch is already locked.
func (b *BranchLock) TryAcquire(branch string) (release func(), err error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.inUse[branch] {
		return nil, ErrBranchBusy
	}
	b.inUse[branch] = true
	return b.makeRelease(branch), nil
}

// makeRelease returns a sync.Once-guarded release function.
func (b *BranchLock) makeRelease(branch string) func() {
	var once sync.Once
	return func() {
		once.Do(func() {
			b.release(branch)
		})
	}
}

// release unlocks the branch and wakes the next waiter, if any.
// When waiters exist, ownership is transferred directly to the next
// waiter by keeping inUse=true — no TryAcquire window.
func (b *BranchLock) release(branch string) {
	b.mu.Lock()

	if waiters := b.waits[branch]; len(waiters) > 0 {
		// Transfer ownership to the next waiter — keep inUse true.
		ch := waiters[0]
		b.waits[branch] = waiters[1:]
		if len(b.waits[branch]) == 0 {
			delete(b.waits, branch)
		}
		b.mu.Unlock()
		close(ch) // wake the waiter, which already "owns" the lock
	} else {
		b.inUse[branch] = false
		delete(b.waits, branch)
		b.mu.Unlock()
	}
}

// removeWaiter removes a specific channel from the wait queue.
// Returns true if the channel was found and removed, false if it was
// already gone (e.g. release() already popped it).
// Called under b.mu.
func (b *BranchLock) removeWaiter(branch string, target chan struct{}) bool {
	waiters := b.waits[branch]
	for i, ch := range waiters {
		if ch == target {
			b.waits[branch] = append(waiters[:i], waiters[i+1:]...)
			if len(b.waits[branch]) == 0 {
				delete(b.waits, branch)
			}
			return true
		}
	}
	return false
}
