package gateway

import (
	"errors"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// uploadDir is the workspace-relative directory where chat attachments are
// saved. It lives INSIDE the workspace root on purpose: the agent's tools
// (read_file, bash, glob) are root-confined via ResolveAndCheck, so an
// attachment is only reachable by the model if it lands under the root.
const uploadDir = ".deepseek/uploads"

// maxUploadBytes caps one POST /v1/upload request (all parts combined).
const maxUploadBytes = 64 << 20 // 64 MiB

// handleUpload implements POST /v1/upload (multipart/form-data; one or more
// "file" parts). Each part is written under <root>/.deepseek/uploads/ with a
// sanitized, collision-free name. The response lists the saved files as
// workspace-relative paths — exactly the strings the SPA should hand to the
// model so its root-confined tools can read them:
//
//	{"files":[{"name":"report.pdf","path":".deepseek/uploads/report.pdf"}]}
func (h *Handler) handleUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.root == "" {
		http.Error(w, "no workspace root", http.StatusBadRequest)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes)
	mr, err := r.MultipartReader()
	if err != nil {
		http.Error(w, "bad multipart request: "+err.Error(), http.StatusBadRequest)
		return
	}

	dir := filepath.Join(h.root, filepath.FromSlash(uploadDir))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		http.Error(w, "create upload dir: "+err.Error(), http.StatusInternalServerError)
		return
	}
	excludeUploadsFromGit(h.root)

	type uploadedFile struct {
		Name string `json:"name"`
		Path string `json:"path"`
	}
	files := []uploadedFile{}
	for {
		part, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			http.Error(w, "read multipart: "+err.Error(), http.StatusBadRequest)
			return
		}
		if part.FormName() != "file" {
			continue
		}
		name := sanitizeUploadName(part.FileName())
		dst, finalName, err := createUniqueFile(dir, name)
		if err != nil {
			http.Error(w, "save attachment: "+err.Error(), http.StatusInternalServerError)
			return
		}
		if _, err := io.Copy(dst, part); err != nil {
			dst.Close()
			os.Remove(dst.Name())
			http.Error(w, "save attachment: "+err.Error(), http.StatusInternalServerError)
			return
		}
		if err := dst.Close(); err != nil {
			http.Error(w, "save attachment: "+err.Error(), http.StatusInternalServerError)
			return
		}
		files = append(files, uploadedFile{
			Name: finalName,
			Path: path.Join(uploadDir, finalName),
		})
	}
	if len(files) == 0 {
		http.Error(w, "no file parts in request", http.StatusBadRequest)
		return
	}
	writeJSON(w, map[string]any{"files": files})
}

// sanitizeUploadName reduces a client-supplied filename to a safe basename:
// no directories, no NULs, never empty and never a dot-only name.
func sanitizeUploadName(name string) string {
	name = strings.ReplaceAll(name, "\\", "/")
	name = path.Base(name)
	name = strings.Map(func(r rune) rune {
		if r == 0 || r == '/' {
			return -1
		}
		return r
	}, name)
	if name == "" || name == "." || name == ".." {
		return "attachment"
	}
	return name
}

// createUniqueFile opens a new file named `name` in dir, appending "-1",
// "-2", … before the extension on collision (O_EXCL guarantees no overwrite).
func createUniqueFile(dir, name string) (*os.File, string, error) {
	ext := path.Ext(name)
	stem := strings.TrimSuffix(name, ext)
	candidate := name
	for i := 1; ; i++ {
		f, err := os.OpenFile(filepath.Join(dir, candidate), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err == nil {
			return f, candidate, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, "", err
		}
		if i > 10000 { // defensive bound; unreachable in practice
			return nil, "", errors.New("could not find a free attachment name")
		}
		candidate = stem + "-" + itoa(i) + ext
	}
}

// itoa avoids importing strconv for a two-line loop helper.
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b [20]byte
	pos := len(b)
	for i > 0 {
		pos--
		b[pos] = byte('0' + i%10)
		i /= 10
	}
	return string(b[pos:])
}

// excludeUploadsFromGit best-effort adds ".deepseek/" to .git/info/exclude so
// attachments never pollute the user's `git status` (and therefore the review
// pane). info/exclude is local-only — unlike .gitignore it never touches the
// working tree. All failures are silently ignored: a non-git root is fine.
func excludeUploadsFromGit(root string) {
	gitDir := filepath.Join(root, ".git")
	if st, err := os.Stat(gitDir); err != nil || !st.IsDir() {
		return
	}
	infoDir := filepath.Join(gitDir, "info")
	excludePath := filepath.Join(infoDir, "exclude")
	const line = ".deepseek/"
	if raw, err := os.ReadFile(excludePath); err == nil {
		for _, l := range strings.Split(string(raw), "\n") {
			if strings.TrimSpace(l) == line {
				return
			}
		}
	}
	if err := os.MkdirAll(infoDir, 0o755); err != nil {
		return
	}
	f, err := os.OpenFile(excludePath, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.WriteString("\n# deepseekcode chat attachments\n" + line + "\n")
}
