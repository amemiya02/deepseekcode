// history.go is a readline-style prompt-history ring plus pure file helpers
// for cross-session recall (G2). The ring mirrors crush's historyPrev/historyNext
// semantics: ↑ walks toward older entries, ↓ walks back toward newer and finally
// restores the live draft. Consecutive duplicates and empty strings are never
// stored, and the ring is capped to the newest `max` entries.
//
// The file helpers (loadPromptHistory/appendPromptHistory) are intentionally
// pure — they touch no shared mutable state, so they are safe to call from a
// tea.Cmd off the UI goroutine (§10: the writer must not race Update).
package tui

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// histNewlineSentinel stands in for a literal newline inside a stored entry so
// the one-entry-per-line file format round-trips multi-line prompts exactly.
// It is a Unit Separator byte, which never appears in user prose.
const histNewlineSentinel = "\x1f"

// promptHistory is the in-memory recall ring. idx == -1 means the user is
// editing the live draft (not browsing); 0..len-1 indexes entries oldest→newest
// while browsing. draft holds the live text saved when browsing begins so ↓ can
// restore it after stepping past the newest entry.
type promptHistory struct {
	entries []string // oldest → newest
	idx     int      // -1 = editing live draft; 0..len-1 = browsing
	draft   string   // live text saved when browsing begins
	max     int      // cap; entries keeps the newest max
}

// newPromptHistory builds a ring from seed (oldest → newest), dropping empty and
// consecutive-duplicate entries and capping to the newest max. A non-positive
// max disables the cap.
func newPromptHistory(seed []string, max int) *promptHistory {
	h := &promptHistory{idx: -1, max: max}
	for _, s := range seed {
		h.push(s)
	}
	return h
}

// push appends s to entries applying the empty / consecutive-dup / cap rules.
// It does not touch browsing state; callers that mutate the ring at runtime
// (Add) reset browsing themselves. Returns whether s was stored.
func (h *promptHistory) push(s string) bool {
	if s == "" {
		return false
	}
	if n := len(h.entries); n > 0 && h.entries[n-1] == s {
		return false
	}
	h.entries = append(h.entries, s)
	if h.max > 0 && len(h.entries) > h.max {
		h.entries = h.entries[len(h.entries)-h.max:]
	}
	return true
}

// Add trims s, stores it (skipping empty and consecutive duplicates), caps to
// the newest max, and resets browsing. It returns whether the entry was stored.
func (h *promptHistory) Add(s string) bool {
	stored := h.push(strings.TrimSpace(s))
	h.Reset()
	return stored
}

// Prev steps to an older entry and returns it. On the first call (idx == -1) it
// saves live as the draft before stepping. It returns ok == false when there is
// nothing older to show (empty ring, or already at the oldest entry).
func (h *promptHistory) Prev(live string) (string, bool) {
	if len(h.entries) == 0 {
		return "", false
	}
	if h.idx == -1 {
		h.draft = live
		h.idx = len(h.entries) - 1
		return h.entries[h.idx], true
	}
	if h.idx == 0 {
		return "", false
	}
	h.idx--
	return h.entries[h.idx], true
}

// Next steps to a newer entry and returns it. Stepping past the newest entry
// restores the saved draft and returns to idx == -1. It returns ok == false
// when not currently browsing.
func (h *promptHistory) Next() (string, bool) {
	if h.idx == -1 {
		return "", false
	}
	if h.idx == len(h.entries)-1 {
		h.idx = -1
		return h.draft, true
	}
	h.idx++
	return h.entries[h.idx], true
}

// Reset returns to editing the live draft: idx == -1 and the saved draft is
// cleared. Call it after a submit, or when the user edits while browsing.
func (h *promptHistory) Reset() {
	h.idx = -1
	h.draft = ""
}

// Entries returns the stored entries oldest → newest. The slice is the ring's
// own backing array; callers must not mutate it.
func (h *promptHistory) Entries() []string {
	return h.entries
}

// loadPromptHistory reads a history file (one escaped entry per line, oldest
// first) and returns the newest max entries unescaped, applying the same
// empty / consecutive-dup rules as the ring. A missing file returns nil. This
// function is pure: it only reads the named path.
func loadPromptHistory(path string, max int) []string {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	h := newPromptHistory(nil, max)
	sc := bufio.NewScanner(f)
	// Allow long single-line prompts; the default 64KiB token limit is too small.
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for sc.Scan() {
		h.push(histUnescape(sc.Text()))
	}
	if err := sc.Err(); err != nil {
		return nil
	}
	if len(h.entries) == 0 {
		return nil
	}
	return h.entries
}

// appendPromptHistory appends one escaped entry to the history file, creating
// the parent directory if needed. It is pure with respect to process state
// (no shared mutable globals) and safe to call from a tea.Cmd.
func appendPromptHistory(path, line string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.WriteString(histEscape(line) + "\n"); err != nil {
		return err
	}
	return nil
}

// histEscape replaces literal newlines with the sentinel so a multi-line prompt
// occupies a single file line. histUnescape is its exact inverse.
func histEscape(s string) string {
	return strings.ReplaceAll(s, "\n", histNewlineSentinel)
}

func histUnescape(s string) string {
	return strings.ReplaceAll(s, histNewlineSentinel, "\n")
}
