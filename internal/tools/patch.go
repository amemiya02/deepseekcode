package tools

import (
	"fmt"
	"strings"
)

type UpdateChunk struct {
	OldLines    []string // context + removed lines (for locating)
	NewLines    []string // context + added lines (replacement)
	Context     string   // @@ context marker (may be empty)
	IsEndOfFile bool     // chunk terminated by "*** End of File"
}

type Hunk struct {
	Op       string        // "add" | "delete" | "update"
	Path     string        // target file path
	MovePath string        // set only when "*** Move to:" declared
	Contents string        // add: full file content (leading '+' stripped)
	Chunks   []UpdateChunk // update: ordered chunks
}

// ParsePatch parses the *** Begin Patch / *** End Patch envelope into
// ordered hunks. Returns an error if markers are missing or lines are
// malformed.
func ParsePatch(patchText string) ([]Hunk, error) {
	text := stripHeredoc(patchText)
	lines := patchSplitLines(text)
	if len(lines) == 0 {
		return nil, fmt.Errorf("invalid patch format: missing Begin/End markers")
	}
	if !strings.HasPrefix(lines[0], "*** Begin Patch") {
		return nil, fmt.Errorf("invalid patch format: missing Begin/End markers")
	}
	// Find *** End Patch
	endIdx := -1
	for i := len(lines) - 1; i > 0; i-- {
		if strings.HasPrefix(lines[i], "*** End Patch") {
			endIdx = i
			break
		}
	}
	if endIdx < 0 {
		return nil, fmt.Errorf("invalid patch format: missing Begin/End markers")
	}
	lines = lines[1:endIdx] // trim markers

	var hunks []Hunk
	i := 0
	for i < len(lines) {
		line := lines[i]
		switch {
		case strings.HasPrefix(line, "*** Add File: "):
			path := strings.TrimPrefix(line, "*** Add File: ")
			i++
			var content []string
			for i < len(lines) && !strings.HasPrefix(lines[i], "*** ") {
				l := lines[i]
				if len(l) > 0 && l[0] == '+' {
					content = append(content, l[1:])
				} else {
					return nil, fmt.Errorf("invalid patch line %d: expected '+' prefix in Add File, got %q", i+1, l)
				}
				i++
			}
			hunks = append(hunks, Hunk{Op: "add", Path: path, Contents: strings.Join(content, "\n")})

		case strings.HasPrefix(line, "*** Delete File: "):
			path := strings.TrimPrefix(line, "*** Delete File: ")
			hunks = append(hunks, Hunk{Op: "delete", Path: path})
			i++

		case strings.HasPrefix(line, "*** Update File: "):
			path := strings.TrimPrefix(line, "*** Update File: ")
			i++
			var movePath string
			if i < len(lines) && strings.HasPrefix(lines[i], "*** Move to: ") {
				movePath = strings.TrimPrefix(lines[i], "*** Move to: ")
				i++
			}
			chunks, ni, err := parseUpdateChunks(lines, i)
			if err != nil {
				return nil, err
			}
			hunks = append(hunks, Hunk{Op: "update", Path: path, MovePath: movePath, Chunks: chunks})
			i = ni

		default:
			return nil, fmt.Errorf("invalid patch line %d: unexpected %q", i+1, line)
		}
	}
	return hunks, nil
}

func parseUpdateChunks(lines []string, start int) ([]UpdateChunk, int, error) {
	var chunks []UpdateChunk
	var cur *UpdateChunk
	i := start

	flush := func() {
		if cur != nil {
			chunks = append(chunks, *cur)
			cur = nil
		}
	}

	for i < len(lines) {
		line := lines[i]
		// *** End of File marks EOF anchor on current chunk
		if strings.HasPrefix(line, "*** End of File") {
			if cur == nil {
				cur = &UpdateChunk{}
			}
			cur.IsEndOfFile = true
			flush()
			i++
			continue
		}
		// Next file header ends this update
		if strings.HasPrefix(line, "*** ") {
			break
		}
		// New @@ chunk boundary
		if strings.HasPrefix(line, "@@") {
			flush()
			cur = &UpdateChunk{Context: strings.TrimPrefix(line, "@@")}
			i++
			continue
		}
		// First content line without @@ — start implicit chunk
		if cur == nil {
			cur = &UpdateChunk{}
		}
		if len(line) == 0 {
			// Empty line inside chunk — treat as context
			cur.OldLines = append(cur.OldLines, "")
			cur.NewLines = append(cur.NewLines, "")
			i++
			continue
		}
		switch line[0] {
		case ' ':
			cur.OldLines = append(cur.OldLines, line[1:])
			cur.NewLines = append(cur.NewLines, line[1:])
		case '-':
			cur.OldLines = append(cur.OldLines, line[1:])
		case '+':
			cur.NewLines = append(cur.NewLines, line[1:])
		default:
			return nil, i, fmt.Errorf("invalid patch line %d: unexpected prefix %q in update chunk", i+1, line)
		}
		i++
	}
	flush()
	return chunks, i, nil
}

// stripHeredoc removes heredoc wrapper (cat <<'EOF' ... EOF).
func stripHeredoc(s string) string {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "cat ") {
		return s
	}
	// Find the delimiter after <<
	idx := strings.Index(s, "<<")
	if idx < 0 {
		return s
	}
	after := s[idx+2:]
	// Skip optional quote
	if len(after) > 0 && (after[0] == '\'' || after[0] == '"') {
		after = after[1:]
	}
	// Read until end of first line for delimiter
	delimEnd := strings.Index(after, "\n")
	if delimEnd < 0 {
		return s
	}
	delim := strings.TrimSpace(after[:delimEnd])
	body := after[delimEnd+1:]
	// Trim closing delimiter
	if idx := strings.LastIndex(body, delim); idx >= 0 {
		body = body[:idx]
	}
	return strings.TrimSpace(body)
}

// patchSplitLines splits on \n, trimming \r from each line.
func patchSplitLines(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	raw := strings.Split(s, "\n")
	out := make([]string, len(raw))
	for i, l := range raw {
		out[i] = strings.TrimRight(l, "\r")
	}
	return out
}

// DeriveNewContents applies update chunks to original, returning the new
// content. Each chunk's OldLines is located in the original via 4-pass
// fuzzy matching (exact → rstrip → trim → collapse-whitespace). A chunk
// with empty OldLines is a pure insertion.
func DeriveNewContents(path string, chunks []UpdateChunk, original string) (string, error) {
	// Detect and normalize CRLF to avoid mixed line endings in output.
	isCRLF := strings.Contains(original, "\r\n")
	if isCRLF {
		original = strings.ReplaceAll(original, "\r\n", "\n")
	}
	var lines []string
	trailingNL := original != "" && strings.HasSuffix(original, "\n")
	if original != "" {
		lines = strings.Split(original, "\n")
		if trailingNL && len(lines) > 0 && lines[len(lines)-1] == "" {
			lines = lines[:len(lines)-1]
		}
	}

	cursor := 0
	for i, chunk := range chunks {
		if len(chunk.OldLines) == 0 {
			// Pure insertion
			insert := chunk.NewLines
			if chunk.IsEndOfFile {
				lines = append(lines, insert...)
			} else {
				// Insert at cursor; if file ends with blank lines, insert before them
				pos := cursor
				if pos > len(lines) {
					pos = len(lines)
				}
				tail := make([]string, len(lines[pos:]))
				copy(tail, lines[pos:])
				lines = lines[:pos]
				lines = append(lines, insert...)
				lines = append(lines, tail...)
				cursor = pos + len(insert)
			}
			continue
		}
		pos := locateChunk(lines[cursor:], chunk.OldLines)
		if pos < 0 {
			return "", fmt.Errorf("apply_patch: cannot locate chunk %d in %s", i+1, path)
		}
		pos += cursor
		// Replace lines[pos:pos+len(Old)] with NewLines
		replacement := make([]string, len(chunk.NewLines))
		copy(replacement, chunk.NewLines)
		newLines := make([]string, 0, len(lines)-len(chunk.OldLines)+len(chunk.NewLines))
		newLines = append(newLines, lines[:pos]...)
		newLines = append(newLines, replacement...)
		newLines = append(newLines, lines[pos+len(chunk.OldLines):]...)
		lines = newLines
		cursor = pos + len(chunk.NewLines)
	}
	sep := "\n"
	if isCRLF {
		sep = "\r\n"
	}
	result := strings.Join(lines, sep)
	if trailingNL {
		result += sep
	}
	return result, nil
}

// locateChunk finds the first occurrence of old in src using 4-pass matching.
// Returns the index in src or -1 if not found.
func locateChunk(src, old []string) int {
	if len(old) == 0 {
		return 0
	}
	// Pass 1: exact
	if pos := matchLines(src, old, func(s string) string { return s }); pos >= 0 {
		return pos
	}
	// Pass 2: rstrip
	if pos := matchLines(src, old, func(s string) string { return strings.TrimRight(s, " \t") }); pos >= 0 {
		return pos
	}
	// Pass 3: trim
	if pos := matchLines(src, old, func(s string) string { return strings.TrimSpace(s) }); pos >= 0 {
		return pos
	}
	// Pass 4: collapse whitespace
	if pos := matchLines(src, old, collapseWS); pos >= 0 {
		return pos
	}
	return -1
}

func matchLines(src, old []string, norm func(string) string) int {
outer:
	for i := 0; i <= len(src)-len(old); i++ {
		for j := range old {
			if norm(src[i+j]) != norm(old[j]) {
				continue outer
			}
		}
		return i
	}
	return -1
}

func collapseWS(s string) string {
	var b strings.Builder
	prev := false
	for _, r := range s {
		if r == ' ' || r == '\t' {
			if prev {
				continue
			}
			prev = true
			b.WriteByte(' ')
		} else {
			prev = false
			b.WriteRune(r)
		}
	}
	return strings.TrimSpace(b.String())
}
