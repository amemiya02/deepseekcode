package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type ApplyPatchTool struct {
	cwd           string
	maxWriteBytes int64
	// tracker, when set, is re-stamped after each write so a subsequent
	// edit_file sees the patched file as fresh (T3.2). apply_patch does NOT
	// gate on it — its hunk context lines already detect drift. nil disables.
	tracker *FileTracker
}

func NewApplyPatchTool(cwd string, maxWriteBytes int64) *ApplyPatchTool {
	return &ApplyPatchTool{cwd: cwd, maxWriteBytes: maxWriteBytes}
}

func (*ApplyPatchTool) Name() string { return "apply_patch" }
func (*ApplyPatchTool) Description() string {
	return "Apply a multi-hunk patch in *** Begin Patch / *** End Patch envelope. Supports Add, Update (fuzzy-matched), Delete, Move."
}

func (*ApplyPatchTool) Parameters() json.RawMessage {
	return MustParams(map[string]any{"type": "object", "properties": map[string]any{
		"patchText": map[string]any{"type": "string", "description": "Patch in *** Begin Patch / *** End Patch envelope."},
	}, "required": []string{"patchText"}})
}

func (t *ApplyPatchTool) AffectedPaths(args json.RawMessage) []string {
	var p struct {
		PatchText string `json:"patchText"`
	}
	if json.Unmarshal(args, &p) != nil || p.PatchText == "" {
		return nil
	}
	hunks, err := ParsePatch(p.PatchText)
	if err != nil {
		return nil
	}
	cwd := t.cwd
	if cwd == "" {
		cwd = "."
	}
	var paths []string
	for _, h := range hunks {
		if h.Path != "" {
			paths = append(paths, filepath.Join(cwd, h.Path))
		}
		if h.MovePath != "" {
			paths = append(paths, filepath.Join(cwd, h.MovePath))
		}
	}
	return paths
}

func (t *ApplyPatchTool) Execute(ctx context.Context, args json.RawMessage) (Result, error) {
	var p struct {
		PatchText string `json:"patchText"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return Errf("invalid args: %v", err), nil
	}
	if p.PatchText == "" {
		return Errf("apply_patch: empty patchText"), nil
	}
	hunks, err := ParsePatch(p.PatchText)
	if err != nil {
		return Errf("%v", err), nil
	}
	if len(hunks) == 0 {
		return Errf("apply_patch: no hunks"), nil
	}
	cwd := t.cwd
	if cwd == "" {
		cwd = "."
	}
	// Phase 1: resolve all paths; reject entire patch if any escapes cwd.
	rs := make([]struct{ ap, am string }, len(hunks))
	for i, h := range hunks {
		ap, err := ResolveAndCheck(filepath.Join(cwd, h.Path), cwd)
		if err != nil {
			return Errf("hunk %d path %q: %v", i+1, h.Path, err), nil
		}
		var am string
		if h.MovePath != "" {
			am, err = ResolveAndCheck(filepath.Join(cwd, h.MovePath), cwd)
			if err != nil {
				return Errf("hunk %d move %q: %v", i+1, h.MovePath, err), nil
			}
		}
		rs[i] = struct{ ap, am string }{ap, am}
	}
	// Phase 2: apply hunks (all paths pre-validated).
	var applied []string
	for i, h := range hunks {
		path, herr := t.applyHunk(h, rs[i].ap, rs[i].am)
		if herr != nil {
			return Errf("%s", herr.Error()), nil
		}
		if path != "" {
			applied = append(applied, path)
		}
	}
	return Result{Content: fmt.Sprintf("applied %d hunks: %v", len(applied), applied)}, nil
}

func (t *ApplyPatchTool) applyHunk(h Hunk, absPath, absMove string) (string, error) {
	switch h.Op {
	case "add":
		if _, err := os.Stat(absPath); err == nil {
			return "", fmt.Errorf("add %q exists; use update", absPath)
		}
		if int64(len(h.Contents)) > t.maxWriteBytes {
			return "", fmt.Errorf("add %q: too large (%d B)", absPath, len(h.Contents))
		}
		os.MkdirAll(filepath.Dir(absPath), 0o755)
		if err := atomicWriteFile(absPath, []byte(h.Contents)); err != nil {
			return "", fmt.Errorf("write %s: %v", absPath, err)
		}
		t.tracker.RecordWrite(absPath)
		return absPath, nil
	case "delete":
		if err := os.Remove(absPath); err != nil && !os.IsNotExist(err) {
			return "", fmt.Errorf("delete %q: %v", absPath, err)
		}
		return absPath, nil
	case "update":
		// No read-before-write guard here (unlike edit_file/write_file, T3.2):
		// an update hunk carries its own context lines, which DeriveNewContents
		// fuzzy-matches against the current file — so a drifted file is rejected
		// by a context mismatch already. apply_patch is self-validating; the
		// explicit guard is reserved for the tools that carry little or no
		// context (write_file overwrites blind; edit_file matches one snippet).
		orig, err := os.ReadFile(absPath)
		if err != nil {
			return "", fmt.Errorf("update %q: %v", absPath, err)
		}
		nw, err := DeriveNewContents(absPath, h.Chunks, string(orig))
		if err != nil {
			return "", err
		}
		if int64(len(nw)) > t.maxWriteBytes {
			return "", fmt.Errorf("update %q: result too large (%d B)", absPath, len(nw))
		}
		tgt := absPath
		if absMove != "" {
			if _, err := os.Stat(absMove); err == nil {
				return "", fmt.Errorf("move target %q exists", absMove)
			}
			tgt = absMove
			os.MkdirAll(filepath.Dir(tgt), 0o755)
		}
		if err := atomicWriteFile(tgt, []byte(nw)); err != nil {
			return "", fmt.Errorf("write %s: %v", tgt, err)
		}
		if absMove != "" {
			_ = os.Remove(absPath)
		}
		t.tracker.RecordWrite(tgt)
		return tgt, nil
	}
	return "", nil
}
