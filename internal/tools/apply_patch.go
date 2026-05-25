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
}

func NewApplyPatchTool(cwd string, maxWriteBytes int64) *ApplyPatchTool {
	return &ApplyPatchTool{cwd: cwd, maxWriteBytes: maxWriteBytes}
}

func (*ApplyPatchTool) Name() string { return "apply_patch" }

func (*ApplyPatchTool) Description() string {
	return "Apply a multi-hunk patch in *** Begin Patch / *** End Patch format. " +
		"Supports Add, Update (fuzzy-matched), Delete, Move. Prefer over multiple edit_file."
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
	type rp struct{ ap, am string }
	rs := make([]rp, len(hunks))
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
		rs[i] = rp{ap, am}
	}
	// Phase 2: apply hunks (all paths pre-validated).
	var applied []string
	for i, h := range hunks {
		rp := rs[i]
		var path, herr string
		switch h.Op {
		case "add":
			if _, err := os.Stat(rp.ap); err == nil {
				herr = fmt.Sprintf("add %q exists; use update", rp.ap)
			} else if int64(len(h.Contents)) > t.maxWriteBytes {
				herr = fmt.Sprintf("add %q: too large (%d B)", rp.ap, len(h.Contents))
			} else {
				os.MkdirAll(filepath.Dir(rp.ap), 0o755)
				if err := atomicWriteFile(rp.ap, []byte(h.Contents)); err != nil {
					herr = fmt.Sprintf("write %s: %v", rp.ap, err)
				} else {
					path = rp.ap
				}
			}
		case "delete":
			if err := os.Remove(rp.ap); err != nil && !os.IsNotExist(err) {
				herr = fmt.Sprintf("delete %q: %v", rp.ap, err)
			} else {
				path = rp.ap
			}
		case "update":
			orig, err := os.ReadFile(rp.ap)
			if err != nil {
				herr = fmt.Sprintf("update %q: %v", rp.ap, err)
				break
			}
			nw, err := DeriveNewContents(rp.ap, h.Chunks, string(orig))
			if err != nil {
				herr = fmt.Sprintf("%v", err)
				break
			}
			if int64(len(nw)) > t.maxWriteBytes {
				herr = fmt.Sprintf("update %q: result too large (%d B)", rp.ap, len(nw))
				break
			}
			tgt := rp.ap
			if rp.am != "" {
				if _, err := os.Stat(rp.am); err == nil {
					herr = fmt.Sprintf("move target %q exists", rp.am)
					break
				}
				tgt = rp.am
				os.MkdirAll(filepath.Dir(tgt), 0o755)
			}
			if err := atomicWriteFile(tgt, []byte(nw)); err != nil {
				herr = fmt.Sprintf("write %s: %v", tgt, err)
			} else {
				if rp.am != "" {
					_ = os.Remove(rp.ap)
				}
				path = tgt
			}
		}
		if herr != "" {
			return Errf("%s", herr), nil
		}
		if path != "" {
			applied = append(applied, path)
		}
	}
	return Result{Content: fmt.Sprintf("applied %d hunks: %v", len(applied), applied)}, nil
}
