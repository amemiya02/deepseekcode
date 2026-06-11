package tools

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/amemiya02/deepseekcode/internal/textenc"
)

// defaultWholeReadLineCap bounds how many lines a *whole-file* read (no
// explicit range) emits. Whole-file reads of large files bloat the
// conversation body; once the body is large, API-side prompt-cache eviction
// becomes very expensive to recover from (a single evicted turn re-sends the
// whole body as a cache miss). Capping whole reads and nudging the model
// toward grep / a narrow start_line+end_line keeps the cacheable body small —
// the same surgical-read discipline that keeps steady-state cost low. Ranged
// reads are never capped: an explicit start_line/end_line is honored exactly.
const defaultWholeReadLineCap = 500

// ReadFile reads a UTF-8 text file and returns it with cat -n style line
// numbers. Models do better at reasoning about positions when they see
// numbers next to lines.
type ReadFile struct {
	MaxBytes int64  // default 5_242_880 (5 MiB); 0 means use default
	MaxLines int    // whole-file read line cap; 0 means defaultWholeReadLineCap
	CWD      string // project root for path safety; empty means os.Getwd
	// Tracker, when set, records the file's on-disk state on a successful read
	// so the write tools can detect out-of-band changes before editing (T3.2).
	// nil disables the freshness guard.
	Tracker *FileTracker
}

func (ReadFile) Name() string { return "read_file" }

func (ReadFile) Description() string {
	return "Read a UTF-8 text file from the filesystem and return its contents " +
		"with cat -n style line numbers (e.g. '  42\\tcontent'). " +
		"Optional start_line and end_line (1-indexed, inclusive) restrict the range. " +
		"If end_line exceeds the file length, returns up to the last line. " +
		"A whole-file read (no range) is capped at the first 500 lines to keep context " +
		"small; for larger files, grep for the symbol you need and read a narrow " +
		"start_line/end_line range around it. " +
		"On binary files or non-UTF8 content, returns an error result describing the situation."
}

func (ReadFile) Parameters() json.RawMessage {
	return MustParams(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Absolute or relative path to the file.",
			},
			"start_line": map[string]any{
				"type":        "integer",
				"description": "Optional 1-indexed inclusive start line.",
			},
			"end_line": map[string]any{
				"type":        "integer",
				"description": "Optional 1-indexed inclusive end line.",
			},
		},
		"required": []string{"path"},
	})
}

func (ReadFile) IsReadOnly() bool { return true }

func (ReadFile) AffectedPaths(args json.RawMessage) []string {
	var p struct {
		Path string `json:"path"`
	}
	_ = json.Unmarshal(args, &p)
	if p.Path == "" {
		return nil
	}
	return []string{p.Path}
}

func (r ReadFile) Execute(ctx context.Context, args json.RawMessage) (Result, error) {
	var p struct {
		Path      string `json:"path"`
		StartLine int    `json:"start_line"`
		EndLine   int    `json:"end_line"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return Errf("invalid args: %v", err), nil
	}
	if p.Path == "" {
		return Errf("path is required"), nil
	}

	cwd := r.CWD
	if cwd == "" {
		cwd = "."
	}
	checkedPath, err := ResolveAndCheck(p.Path, cwd)
	if err != nil {
		return Errf("%v", err), nil
	}
	p.Path = checkedPath

	f, err := os.Open(p.Path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Errf("file not found: %s", p.Path), nil
		}
		return Result{}, fmt.Errorf("opening %s: %w", p.Path, err)
	}
	defer f.Close()

	// Binary detection and CJK encoding: read up to 8 KB head, detect
	// encoding FIRST, then branch.  For UTF-8 files the NUL-byte check
	// runs on raw bytes (existing behaviour).  For non-UTF8 (UTF-16,
	// GBK, GB18030) the raw bytes are full of NULs by construction, so
	// the NUL check must happen AFTER decoding to UTF-8 — otherwise
	// UTF-16 files are falsely classified as binary.
	head := make([]byte, 8192)
	n, _ := io.ReadFull(f, head)
	if n == 0 {
		// read nothing at all — edge case for empty files or read errors.
		// Fall through to the normal scan path.
	}

	maxBytes := r.MaxBytes
	if maxBytes <= 0 {
		maxBytes = 5 * 1024 * 1024 // 5 MiB default
	}
	stat, err := f.Stat()
	if err == nil && stat.Size() > maxBytes {
		r.Tracker.RecordRead(p.Path)
		return Errf("file too large (%d bytes; cap is %d). Use grep or a narrower start_line/end_line.",
			stat.Size(), maxBytes), nil
	}

	// Detect encoding from the head bytes BEFORE any NUL check so that
	// UTF-16 (which is NUL-heavy) is not misclassified as binary.
	headKind := textenc.Detect(head[:n])
	var scanReader io.Reader
	if headKind != textenc.UTF8 {
		// Re-read entire file for decoding.
		if _, err := f.Seek(0, io.SeekStart); err != nil {
			return Result{}, fmt.Errorf("seeking %s: %w", p.Path, err)
		}
		all, err := io.ReadAll(io.LimitReader(f, maxBytes+1))
		if err != nil {
			return Result{}, fmt.Errorf("reading %s: %w", p.Path, err)
		}
		if int64(len(all)) > maxBytes {
			r.Tracker.RecordRead(p.Path)
			return Errf("file too large (%d bytes; cap is %d). Use grep or a narrower start_line/end_line.",
				len(all), maxBytes), nil
		}
		decoded := textenc.Decode(all, headKind)
		// NUL-byte check on DECODED content — a non-UTF8 file whose decoded
		// form still contains NUL is genuinely binary.
		if bytes.IndexByte(decoded, 0) >= 0 {
			return Errf("binary file (NUL byte detected after decoding from %v). "+
				"consider using bash 'file %s' or 'xxd %s | head' to check the type, "+
				"or read it with an offset/range.", headKind, p.Path, p.Path), nil
		}
		// Control-character sanity: C0 controls U+0001-U+0008 (encoded
		// as single bytes 0x01-0x08 in UTF-8) are extremely unlikely in
		// legitimate text but common when binary content is misdecoded.
		// (Multi-byte UTF-8 continuations are 0x80-0xBF, so no overlap.)
		if hasBinaryControls(decoded) {
			return Errf("binary file (control characters detected after decoding from %v). "+
				"consider using bash 'file %s' or 'xxd %s | head' to check the type, "+
				"or read it with an offset/range.", headKind, p.Path, p.Path), nil
		}
		scanReader = bytes.NewReader(decoded)
	} else {
		// UTF-8: NUL-byte check on raw bytes catches genuine binaries.
		if n > 0 && bytes.IndexByte(head[:n], 0) >= 0 {
			return Errf("binary file (NUL byte detected at or before offset %d). "+
				"consider using bash 'file %s' or 'xxd %s | head' to check the type, "+
				"or read it with an offset/range.", n, p.Path, p.Path), nil
		}
		// Seek back so the line scanner sees the full file.
		if _, err := f.Seek(0, io.SeekStart); err != nil {
			return Result{}, fmt.Errorf("seeking %s: %w", p.Path, err)
		}
		scanReader = f
	}

	start := p.StartLine
	end := p.EndLine
	if start < 0 || end < 0 || (end > 0 && start > end) {
		return Errf("invalid line range: start=%d end=%d", start, end), nil
	}
	if start == 0 {
		start = 1
	}

	// A whole-file read (no explicit range) is capped at lineCap lines so a
	// large file cannot flood the cacheable conversation body. An explicit
	// range (any end_line, or a start_line past line 1) opts out of the cap.
	lineCap := r.MaxLines
	if lineCap <= 0 {
		lineCap = defaultWholeReadLineCap
	}
	wholeRead := p.EndLine == 0 && p.StartLine <= 1

	var out strings.Builder
	sc := bufio.NewScanner(scanReader)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	lineNum := 0
	capped := false
	for sc.Scan() {
		lineNum++
		if lineNum < start {
			continue
		}
		if end != 0 && lineNum > end {
			break
		}
		if wholeRead && lineNum > lineCap {
			// Stop emitting but keep scanning so the notice can report the
			// true total line count.
			capped = true
			continue
		}
		// cat -n format: 6-wide right-aligned line number, tab, content.
		fmt.Fprintf(&out, "%6d\t%s\n", lineNum, sc.Text())
		select {
		case <-ctx.Done():
			return Errf("cancelled at line %d", lineNum), nil
		default:
		}
	}
	if err := sc.Err(); err != nil && !errors.Is(err, io.EOF) {
		return Errf("read error at line %d: %v", lineNum, err), nil
	}
	if lineNum == 0 {
		// Empty file is still a successful read — stamp it so a later write
		// against an unchanged empty file is allowed.
		r.Tracker.RecordRead(p.Path)
		return Result{Content: fmt.Sprintf("(empty file: %s)", p.Path)}, nil
	}
	if start > lineNum {
		return Errf("start_line=%d is past end of file (%d lines)", start, lineNum), nil
	}
	// Stamp the file's on-disk state so the write tools can detect a change
	// between this read and a subsequent edit (T3.2). A partial (ranged) read
	// still counts — the stamp tracks disk freshness, not coverage.
	r.Tracker.RecordRead(p.Path)
	content := out.String()
	if capped {
		content += fmt.Sprintf(
			"\n... (showing lines 1-%d of %d. File truncated to keep the context small. "+
				"Read a specific region with start_line/end_line, or use grep to locate symbols.) ...\n",
			lineCap, lineNum)
	}
	return Result{Content: content}, nil
}

// hasBinaryControls reports whether b contains C0 control characters
// U+0001–U+0008. These are encoded as single bytes 0x01–0x08 in UTF-8
// and are extremely unlikely in legitimate text, but common when raw
// binary content is misdecoded as UTF-16 or CJK.
func hasBinaryControls(b []byte) bool {
	for _, c := range b {
		if c >= 0x01 && c <= 0x08 {
			return true
		}
	}
	return false
}
