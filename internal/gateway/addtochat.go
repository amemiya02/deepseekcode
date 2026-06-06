// Package gateway: Wave-5 add-to-chat endpoint. It formats a uniform context
// pill the SPA composer can append as an @-reference. The contents path is
// root-confined via resolveInRoot (introduced by Wave 1's workspace.go) so a
// malicious payload cannot read outside the workspace root.
package gateway

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
)

// addToChatRequest builds a context-pill payload from a workspace selection. The
// SPA sends one of: a file ref (path), a folder ref (path,is_dir), a file's
// contents (path,include_contents), or a free-text selection (text).
type addToChatRequest struct {
	Path            string `json:"path,omitempty"`
	Text            string `json:"text,omitempty"`
	IsDir           bool   `json:"is_dir,omitempty"`
	IncludeContents bool   `json:"include_contents,omitempty"`
}

type addToChatResponse struct {
	Label   string `json:"label"`   // short pill label, e.g. "pkg/util.go"
	Content string `json:"content"` // the block appended to the prompt
}

func (h *Handler) handleAddToChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req addToChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	var out addToChatResponse
	switch {
	case req.Text != "":
		out.Label = "selection"
		out.Content = "```\n" + req.Text + "\n```"
	case req.Path != "" && req.IsDir:
		out.Label = filepath.ToSlash(req.Path) + "/"
		out.Content = "@" + filepath.ToSlash(req.Path) + "/ (folder)"
	case req.Path != "" && req.IncludeContents:
		abs, err := resolveInRoot(h.root, req.Path)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		data, err := os.ReadFile(abs)
		if err != nil {
			http.Error(w, "read file: "+err.Error(), http.StatusNotFound)
			return
		}
		if len(data) > maxFileBytes {
			data = data[:maxFileBytes]
		}
		out.Label = filepath.ToSlash(req.Path)
		out.Content = "```" + req.Path + "\n" + string(data) + "\n```"
	case req.Path != "":
		out.Label = filepath.ToSlash(req.Path)
		out.Content = "@" + filepath.ToSlash(req.Path)
	default:
		http.Error(w, "nothing to add: provide path or text", http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}
