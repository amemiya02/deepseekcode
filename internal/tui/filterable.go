package tui

import "sort"

// filterableList is the shared filter+cursor state behind the filterable
// pickers (G6: /models, /sessions) and the command palette (G5). It owns a
// live filter string and the fuzzy-ranked subset of row indices it produces,
// plus a clamped cursor into that subset. Callers supply the row labels once
// (SetRows) and feed it keystrokes (Type / Backspace) and cursor moves
// (Up / Down); Visible() returns the surviving row indices in rank order so
// the renderer maps them back to its own row data.
//
// It deliberately holds only labels + indices, never the row payloads, so the
// same helper drives heterogeneous pickers (modelOption, sessionRow, slashCmd)
// without knowing their types.
type filterableList struct {
	labels  []string
	filter  string
	visible []int // indices into labels, fuzzy-ranked; empty filter == all
	cursor  int   // index into visible
}

// SetRows installs the candidate labels and resets the filter, recomputing the
// visible set (all rows, in order) and parking the cursor at the top.
func (f *filterableList) SetRows(labels []string) {
	f.labels = labels
	f.filter = ""
	f.recompute()
}

// Filter returns the live filter string (for rendering the input line).
func (f *filterableList) Filter() string { return f.filter }

// Type appends a rune to the filter and re-narrows the visible set.
func (f *filterableList) Type(r rune) {
	f.filter += string(r)
	f.recompute()
}

// Backspace drops the last rune of the filter and re-widens the visible set.
// A no-op on an empty filter so callers can treat "backspace on empty" as a
// signal to close.
func (f *filterableList) Backspace() {
	if f.filter == "" {
		return
	}
	rs := []rune(f.filter)
	f.filter = string(rs[:len(rs)-1])
	f.recompute()
}

// ClearFilter resets the filter to empty without dropping the rows. Returns
// true when there was a filter to clear, so a picker can use it to implement
// "first esc clears the filter, second esc closes".
func (f *filterableList) ClearFilter() bool {
	if f.filter == "" {
		return false
	}
	f.filter = ""
	f.recompute()
	return true
}

// Up / Down move the cursor within the visible set, clamped (no wrap) so a
// long filtered list can't scroll the cursor off either end.
func (f *filterableList) Up() {
	if f.cursor > 0 {
		f.cursor--
	}
}

func (f *filterableList) Down() {
	if f.cursor < len(f.visible)-1 {
		f.cursor++
	}
}

// Visible returns the surviving row indices in rank order. The renderer walks
// these to draw rows and maps the cursor through Cursor().
func (f *filterableList) Visible() []int { return f.visible }

// Cursor returns the cursor's index into the visible slice.
func (f *filterableList) Cursor() int { return f.cursor }

// Selected returns the original row index under the cursor, or -1 when the
// visible set is empty (nothing matched the filter).
func (f *filterableList) Selected() int {
	if f.cursor < 0 || f.cursor >= len(f.visible) {
		return -1
	}
	return f.visible[f.cursor]
}

// recompute re-runs the fuzzy filter over every label, sorts the survivors by
// score (lower is better) then label, and clamps the cursor into the new set.
// An empty filter keeps every row in its original order.
func (f *filterableList) recompute() {
	if f.filter == "" {
		f.visible = make([]int, len(f.labels))
		for i := range f.labels {
			f.visible[i] = i
		}
		f.clampCursor()
		return
	}

	type scored struct {
		idx   int
		score int
	}
	hits := make([]scored, 0, len(f.labels))
	for i, label := range f.labels {
		ok, score, _ := fuzzyMatch(f.filter, label)
		if !ok {
			continue
		}
		hits = append(hits, scored{idx: i, score: score})
	}
	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].score != hits[j].score {
			return hits[i].score < hits[j].score
		}
		return f.labels[hits[i].idx] < f.labels[hits[j].idx]
	})
	f.visible = make([]int, len(hits))
	for i, h := range hits {
		f.visible[i] = h.idx
	}
	f.clampCursor()
}

// clampCursor keeps the cursor inside [0, len(visible)) after a filter change.
func (f *filterableList) clampCursor() {
	if f.cursor < 0 {
		f.cursor = 0
	}
	if f.cursor >= len(f.visible) {
		f.cursor = len(f.visible) - 1
	}
	if f.cursor < 0 {
		f.cursor = 0
	}
}
