package tools

import (
	"fmt"
	"os"
	"sync"
)

// FileTracker records, per session, the on-disk state of files at the moment
// the agent last read (or wrote) them, so the write tools can reject an edit to
// a file that was never read or that changed underfoot — a formatter, linter,
// the user, or a parallel tool editing between the read and the write. It
// guards against silently clobbering those out-of-band changes (T3.2; mirrors
// Claude Code readFileState / crush filetracker / Reasonix ReadTracker).
//
// The freshness signal is (mtime, size): every ordinary write updates mtime, so
// this catches the threat model cheaply with no extra file read. A change that
// preserves BOTH mtime and size (e.g. a deliberate mtime-restoring rewrite of
// equal length) is the one blind spot — acceptable for the cost.
//
// All methods are safe for concurrent use (runToolCalls executes tools in
// parallel) and nil-safe: a nil *FileTracker is a no-op, so tools constructed
// without a tracker (unit tests) simply skip the guard and behave exactly as
// before.
//
// One tracker is shared by every tool in a registry AND propagated to its
// Subset/TierTools/CloneForCWD derivatives, so a subagent built from the parent
// registry shares the parent's tracker (and its compaction Clear()). The
// records are keyed by canonical absolute path. Consequence: a parent and a
// concurrently-running async child can weaken or over-trip each other's guard
// when they touch the same path — a parent Clear() that wipes a child's stamp
// only forces a (safe) re-read, so the interaction is fail-toward-reread; it is
// not a data race (the mutex serializes all access).
type FileTracker struct {
	mu      sync.Mutex
	records map[string]fileStamp
}

type fileStamp struct {
	modUnixNano int64
	size        int64
}

// NewFileTracker returns an empty tracker.
func NewFileTracker() *FileTracker {
	return &FileTracker{records: make(map[string]fileStamp)}
}

// RecordRead stamps path with its current on-disk state. Call it after a
// successful read so a later write can detect drift. path must be the
// canonical (resolved) path the write tools will also check. A stat failure
// (path missing/unstattable) is ignored — there is nothing to stamp.
func (t *FileTracker) RecordRead(path string) {
	if t == nil {
		return
	}
	fi, err := os.Stat(path)
	if err != nil {
		return
	}
	t.mu.Lock()
	t.records[path] = fileStamp{modUnixNano: fi.ModTime().UnixNano(), size: fi.Size()}
	t.mu.Unlock()
}

// RecordWrite re-stamps path after a successful write so a subsequent edit in
// the same session sees the just-written state as fresh rather than tripping
// the changed-since-read guard on the agent's own edit.
func (t *FileTracker) RecordWrite(path string) { t.RecordRead(path) }

// CheckFresh returns a model-actionable error when path may not be safely
// written:
//   - the file exists on disk but was never read this session, or
//   - it exists and its (mtime, size) differ from what was recorded.
//
// It returns nil when the file does not exist (a legitimate create) or matches
// the recorded stamp. nil tracker → nil (guard disabled).
func (t *FileTracker) CheckFresh(path string) error {
	if t == nil {
		return nil
	}
	fi, err := os.Stat(path)
	if err != nil {
		// Not on disk (a new-file create) or unstattable → nothing to clobber.
		return nil
	}
	t.mu.Lock()
	rec, seen := t.records[path]
	t.mu.Unlock()
	if !seen {
		return fmt.Errorf("%s exists but has not been read this session — read_file it first so you edit against its current contents", path)
	}
	if rec.modUnixNano != fi.ModTime().UnixNano() || rec.size != fi.Size() {
		return fmt.Errorf("%s changed on disk since you last read it (a formatter, linter, the user, or another tool may have edited it) — re-read it before writing so you don't clobber those changes", path)
	}
	return nil
}

// Clear drops all records. Called when the conversation is compacted: once the
// read_file results are folded out of the live transcript the model no longer
// holds the files' contents, so it must re-read before editing. Clearing all
// records (rather than only the folded range) is intentionally conservative —
// compaction is rare on a 1M window, and an extra re-read is cheaper than a
// clobbered edit.
func (t *FileTracker) Clear() {
	if t == nil {
		return
	}
	t.mu.Lock()
	t.records = make(map[string]fileStamp)
	t.mu.Unlock()
}
