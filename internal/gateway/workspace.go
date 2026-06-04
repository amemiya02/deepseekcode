package gateway

import (
	"bytes"
	"errors"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

// maxFileBytes caps a single /v1/file read so the SPA never pulls a huge blob.
const maxFileBytes = 512 * 1024

// resolveInRoot joins rel to the workspace root and confirms the result stays
// inside root (defense in depth on top of loopback+bearer). It rejects absolute
// paths and any ".." escape. Returns the cleaned absolute path or an error.
func resolveInRoot(root, rel string) (string, error) {
	if root == "" {
		return "", errors.New("no workspace root")
	}
	if filepath.IsAbs(rel) {
		return "", errors.New("absolute path not allowed")
	}
	abs := filepath.Join(root, rel)
	cleanRoot := filepath.Clean(root)
	// abs must equal root or be under root + separator.
	if abs != cleanRoot && !strings.HasPrefix(abs, cleanRoot+string(os.PathSeparator)) {
		return "", errors.New("path escapes workspace root")
	}
	return abs, nil
}

// fileEntry is one directory listing row (snake_case JSON).
type fileEntry struct {
	Name  string `json:"name"`
	Path  string `json:"path"`
	IsDir bool   `json:"is_dir"`
}

// handleFiles implements GET /v1/files?path= — a single directory level listing.
func (h *Handler) handleFiles(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	rel := r.URL.Query().Get("path") // "" = root
	dir, err := resolveInRoot(h.root, rel)
	if err != nil {
		http.Error(w, "bad path: "+err.Error(), http.StatusBadRequest)
		return
	}
	ents, err := os.ReadDir(dir)
	if err != nil {
		http.Error(w, "read dir: "+err.Error(), http.StatusInternalServerError)
		return
	}
	out := make([]fileEntry, 0, len(ents))
	for _, e := range ents {
		out = append(out, fileEntry{
			Name:  e.Name(),
			Path:  filepath.ToSlash(filepath.Join(rel, e.Name())),
			IsDir: e.IsDir(),
		})
	}
	writeJSON(w, map[string]any{"entries": out})
}

// handleFile implements GET /v1/file?path= — a single file's content.
func (h *Handler) handleFile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	rel := r.URL.Query().Get("path")
	if rel == "" {
		http.Error(w, "missing path", http.StatusBadRequest)
		return
	}
	abs, err := resolveInRoot(h.root, rel)
	if err != nil {
		http.Error(w, "bad path: "+err.Error(), http.StatusBadRequest)
		return
	}
	raw, err := os.ReadFile(abs)
	if err != nil {
		http.Error(w, "read file: "+err.Error(), http.StatusNotFound)
		return
	}
	truncated := false
	if len(raw) > maxFileBytes {
		raw = raw[:maxFileBytes]
		truncated = true
	}
	binary := bytes.IndexByte(raw, 0) >= 0 || !utf8.Valid(raw)
	content := ""
	if !binary {
		content = string(raw)
	}
	writeJSON(w, map[string]any{
		"path": filepath.ToSlash(rel), "content": content,
		"binary": binary, "truncated": truncated,
	})
}

// changedEntry is one changed-file row (snake_case JSON).
type changedEntry struct {
	Path    string `json:"path"`
	Status  string `json:"status"`
	Deleted bool   `json:"deleted"`
}

// handleChanged implements GET /v1/changed — the workspace's changed files via
// `git status --porcelain -z`. A non-git root (or git error) yields an empty
// list (200), never a 500 — the SPA shows "no changes" rather than an error.
func (h *Handler) handleChanged(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	entries := []changedEntry{}
	if h.root != "" {
		cmd := exec.CommandContext(r.Context(), "git", "-C", h.root, "status", "--porcelain", "-z")
		var buf bytes.Buffer
		cmd.Stdout = &buf
		if err := cmd.Run(); err == nil {
			for _, rec := range strings.Split(buf.String(), "\x00") {
				if len(rec) < 4 {
					continue
				}
				code := rec[:2]
				path := rec[3:]
				entries = append(entries, changedEntry{
					Path:    filepath.ToSlash(path),
					Status:  strings.TrimSpace(code),
					Deleted: strings.Contains(code, "D"),
				})
			}
		}
	}
	writeJSON(w, map[string]any{"entries": entries})
}
