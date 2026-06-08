// Package gateway: git branch introspection and checkout endpoints.
// These shell out to `git` in the workspace root (h.root), following the same
// pattern as workspace.go's handleDiff/handleChanged. A non-git root or git
// error yields a graceful empty/default response rather than a 500.
package gateway

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os/exec"
	"strings"
)

// branchInfo is one branch entry returned to the SPA.
type branchInfo struct {
	Name    string `json:"name"`
	Current bool   `json:"current"`
}

// handleBranches implements GET /v1/git/branches — lists local branches and
// marks the currently checked-out one. Non-git roots return an empty list.
func (h *Handler) handleBranches(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.root == "" {
		writeJSON(w, map[string]any{"branches": []branchInfo{}, "current": ""})
		return
	}

	// git branch --list outputs lines like:
	//   main
	// * feature/foo
	//   dev
	// The asterisk marks the current branch.
	cmd := exec.CommandContext(r.Context(), "git", "-C", h.root, "branch", "--list")
	var buf bytes.Buffer
	cmd.Stdout = &buf
	if err := cmd.Run(); err != nil {
		writeJSON(w, map[string]any{"branches": []branchInfo{}, "current": ""})
		return
	}

	// Also get the current branch name directly (more reliable than parsing *).
	currentCmd := exec.CommandContext(r.Context(), "git", "-C", h.root, "branch", "--show-current")
	var curBuf bytes.Buffer
	currentCmd.Stdout = &curBuf
	_ = currentCmd.Run()
	current := strings.TrimSpace(curBuf.String())

	var branches []branchInfo
	for _, line := range strings.Split(buf.String(), "\n") {
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			continue
		}
		isCurrent := strings.HasPrefix(line, "* ")
		name := strings.TrimPrefix(line, "* ")
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		// If --show-current returned empty (detached HEAD), fall back to * marker.
		if current == "" && isCurrent {
			current = name
		}
		branches = append(branches, branchInfo{Name: name, Current: isCurrent})
	}

	if branches == nil {
		branches = []branchInfo{}
	}
	writeJSON(w, map[string]any{"branches": branches, "current": current})
}

// checkoutRequest is the POST /v1/git/checkout body.
type checkoutRequest struct {
	Branch string `json:"branch"`
}

// checkoutResponse is the POST /v1/git/checkout result.
type checkoutResponse struct {
	Branch  string `json:"branch"`
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

// handleCheckout implements POST /v1/git/checkout — switches to the requested
// local branch via `git checkout`. Returns success/error so the SPA can
// display a toast.
func (h *Handler) handleCheckout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.root == "" {
		writeJSON(w, checkoutResponse{Success: false, Error: "no workspace root"})
		return
	}

	var req checkoutRequest
	if err := readJSON(r, &req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if req.Branch == "" {
		writeJSON(w, checkoutResponse{Success: false, Error: "branch is required"})
		return
	}

	cmd := exec.CommandContext(r.Context(), "git", "-C", h.root, "checkout", req.Branch)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		writeJSON(w, checkoutResponse{Branch: req.Branch, Success: false, Error: msg})
		return
	}

	writeJSON(w, checkoutResponse{Branch: req.Branch, Success: true})
}

// readJSON decodes the request body into v. Returns an error if decoding fails.
// (Placed here to keep git.go self-contained; can be promoted to a shared util
// if other handlers need it.)
func readJSON(r *http.Request, v any) error {
	return json.NewDecoder(r.Body).Decode(v)
}
