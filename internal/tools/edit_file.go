package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/amemiya02/deepseekcode/internal/textenc"
)

// EditFile is a string-replace editor. It refuses ambiguous matches so
// the model can't accidentally edit the wrong occurrence. If the user
// wants every occurrence replaced they must pass replace_all=true.
//
// This mirrors Claude Code's Edit tool semantics, which is now the de
// facto standard for agent edits.
type EditFile struct {
	CWD string // project root for path safety; empty means os.Getwd
	// Tracker, when set, rejects an edit to a file that was never read or that
	// changed on disk since the last read (T3.2). nil disables the guard.
	Tracker *FileTracker
}

func (EditFile) Name() string { return "edit_file" }

func (EditFile) Description() string {
	return "Surgically edit a file by replacing exactly one occurrence of old_string " +
		"with new_string. Fails if old_string appears zero times or more than once " +
		"(use replace_all=true to replace every occurrence). " +
		"Preserve exact indentation and surrounding context when constructing old_string. " +
		"For new files or full rewrites, use write_file instead. " +
		"The agent automatically snapshots affected files before this tool runs."
}

func (EditFile) Parameters() json.RawMessage {
	return MustParams(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Absolute or relative path of the file to edit.",
			},
			"old_string": map[string]any{
				"type":        "string",
				"description": "Exact string to find. Must match exactly, including whitespace.",
			},
			"new_string": map[string]any{
				"type":        "string",
				"description": "Replacement string. Empty string deletes the match.",
			},
			"replace_all": map[string]any{
				"type":        "boolean",
				"description": "If true, replace every occurrence. Otherwise old_string must match exactly once.",
			},
		},
		"required": []string{"path", "old_string", "new_string"},
	})
}

func (EditFile) AffectedPaths(args json.RawMessage) []string {
	var p struct {
		Path string `json:"path"`
	}
	_ = json.Unmarshal(args, &p)
	if p.Path == "" {
		return nil
	}
	return []string{p.Path}
}

func (e EditFile) Execute(ctx context.Context, args json.RawMessage) (Result, error) {
	var p struct {
		Path       string `json:"path"`
		OldString  string `json:"old_string"`
		NewString  string `json:"new_string"`
		ReplaceAll bool   `json:"replace_all"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return Errf("invalid args: %v", err), nil
	}
	if p.Path == "" {
		return Errf("path is required"), nil
	}
	if p.OldString == "" {
		return Errf("old_string must be non-empty"), nil
	}
	if p.OldString == p.NewString {
		return Errf("old_string and new_string are identical; nothing to do"), nil
	}

	cwd := e.CWD
	if cwd == "" {
		cwd = "."
	}
	checkedPath, err := ResolveAndCheck(p.Path, cwd)
	if err != nil {
		return Errf("%v", err), nil
	}
	p.Path = checkedPath

	// Read-before-write freshness guard (T3.2): refuse to edit a file that was
	// never read or that changed on disk since the read.
	if err := e.Tracker.CheckFresh(p.Path); err != nil {
		return Errf("%v", err), nil
	}

	b, err := os.ReadFile(p.Path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Errf("file not found: %s (use write_file to create)", p.Path), nil
		}
		return Result{}, fmt.Errorf("reading %s: %w", p.Path, err)
	}

	// Detect CJK / UTF-16 encoding; decode to UTF-8 for editing, re-encode
	// after producing the replacement so the file's original charset is
	// preserved. UTF-8 files take the existing byte-identical path.
	enc := textenc.Detect(b)
	var content string
	if enc != textenc.UTF8 {
		content = string(textenc.Decode(b, enc))
	} else {
		content = string(b)
	}

	// Mirror opencode: normalize CRLF -> LF for matching, then convert the
	// replacement back to the file's detected line ending. This lets the fuzzy
	// strategies match against a uniform "\n" form while the bytes we write
	// preserve the file's original ending. detectLineEnding inspects the
	// ORIGINAL content (pre-normalization); a file with no CRLF stays LF and
	// the conversions are no-ops, so existing LF behavior is byte-identical.
	ending := detectLineEnding(content)
	normContent := normalizeLineEndings(content)
	normOld := normalizeLineEndings(p.OldString)
	normNew := normalizeLineEndings(p.NewString)

	updated, matched, ambiguous := applyReplace(normContent, normOld, normNew, p.ReplaceAll)
	if !matched {
		if ambiguous {
			return Errf("old_string matched multiple locations in %s. Provide a longer, unique snippet "+
				"(include more surrounding lines) or set replace_all=true.", p.Path), nil
		}
		// Genuine not-found: fall back to the fuzzy hint, which suggests the
		// closest region. Match on the normalized content so line numbers and
		// previews are consistent with what the cascade saw.
		return fuzzyHint(normContent, normOld, p.Path)
	}

	out := convertToLineEnding(updated, ending)
	if enc != textenc.UTF8 {
		if err := atomicWriteFile(p.Path, textenc.Encode(out, enc)); err != nil {
			return Result{}, err
		}
	} else {
		if err := atomicWriteFile(p.Path, []byte(out)); err != nil {
			return Result{}, err
		}
	}
	// Re-stamp so a follow-up edit in the same session isn't tripped by the
	// agent's own write (T3.2).
	e.Tracker.RecordWrite(p.Path)
	return Result{Content: fmt.Sprintf("edited %s", p.Path)}, nil
}

// plural returns "s" for counts other than 1 (used by todo_write).
func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// detectLineEnding reports the dominant line ending of text. A single CRLF is
// enough to treat the whole file as CRLF, matching opencode.
func detectLineEnding(text string) string {
	if strings.Contains(text, "\r\n") {
		return "\r\n"
	}
	return "\n"
}

// normalizeLineEndings converts CRLF to LF so matching operates on one form.
func normalizeLineEndings(text string) string {
	return strings.ReplaceAll(text, "\r\n", "\n")
}

// convertToLineEnding converts LF back to the file's detected ending.
func convertToLineEnding(text, ending string) string {
	if ending == "\n" {
		return text
	}
	return strings.ReplaceAll(text, "\n", "\r\n")
}

const fuzzyMaxDist = 3

// fuzzyHint tries to find a close match for old in the file content using
// line-level Levenshtein distance. If a candidate within fuzzyMaxDist lines is
// found, it returns a hint error with the match location and preview.
// Otherwise it falls back to the plain "not found" error.
func fuzzyHint(content, old, path string) (Result, error) {
	fileLines := splitLines(content)
	oldLines := splitLines(old)
	if len(oldLines) == 0 {
		return Errf("old_string not found in %s. The exact substring (including whitespace) must match.", path), nil
	}

	// Slide a window of len(oldLines) across the file to find the closest region.
	windowSize := len(oldLines)
	if windowSize > len(fileLines) {
		windowSize = len(fileLines)
	}

	var candidates []string
	for i := 0; i <= len(fileLines)-windowSize; i++ {
		candidates = append(candidates, strings.Join(fileLines[i:i+windowSize], "\n"))
	}

	idx, dist := ClosestMatch(old, candidates, fuzzyMaxDist)
	if idx < 0 || dist > fuzzyMaxDist {
		return Errf("old_string not found in %s. The exact substring (including whitespace) must match.", path), nil
	}

	// Convert window index back to 1-based line number.
	lineNo := idx + 1
	preview := candidates[idx]
	if len(preview) > 80 {
		preview = preview[:80] + "..."
	}

	return Errf("old_string not found in %s. Closest match at line %d (distance %d): %q",
		path, lineNo, dist, preview), nil
}
