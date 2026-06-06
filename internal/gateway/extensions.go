package gateway

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/amemiya02/deepseekcode/internal/config"
	"github.com/amemiya02/deepseekcode/internal/memory"
	"github.com/amemiya02/deepseekcode/internal/skills"
)

// extItem is one row in the Settings > Extensions list. The shape mirrors the
// SPA's ExtItem ({id,name,enabled}) and the mockGateway fixture
// ({"items":[...]}), so each real endpoint is a drop-in for the mock.
type extItem struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
}

// extResponse is the wire envelope every Extensions endpoint returns.
type extResponse struct {
	Items []extItem `json:"items"`
}

// writeExtItems writes a 200 with {"items":[...]}, coercing a nil slice to []
// so the JSON is always `{"items":[]}` (never `{"items":null}`). These
// read-only endpoints never 404: an absent/unenumerable subsystem yields an
// honest empty list, which the SPA renders as the "Nothing configured yet."
// empty state.
func writeExtItems(w http.ResponseWriter, items []extItem) {
	if items == nil {
		items = []extItem{}
	}
	writeJSON(w, extResponse{Items: items})
}

// workspaceRoot returns the directory used to discover on-disk subsystems
// (skills, memory). It prefers the gateway's configured workspace root and
// falls back to the process working directory so the endpoints still enumerate
// in `dsc serve` even when WithWorkspaceRoot was not wired.
func (h *Handler) workspaceRoot() string {
	if h.root != "" {
		return h.root
	}
	if wd, err := os.Getwd(); err == nil {
		return wd
	}
	return ""
}

// handleHooks implements GET /v1/hooks. It enumerates the configured lifecycle
// hooks from config ([[hooks]] tables). Each configured hook is reported
// enabled. The display name combines the event with the builtin name or
// subprocess command so the list is human-readable.
func (h *Handler) handleHooks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	cfg, err := loadConfig()
	if err != nil {
		writeExtItems(w, nil)
		return
	}
	items := make([]extItem, 0, len(cfg.Hooks))
	for i, hk := range cfg.Hooks {
		items = append(items, extItem{
			ID:      fmt.Sprintf("hook-%d", i),
			Name:    hookDisplayName(hk),
			Enabled: true,
		})
	}
	writeExtItems(w, items)
}

// hookDisplayName renders a HookItemConfig as "<event>: <name-or-command>".
func hookDisplayName(hk config.HookItemConfig) string {
	target := hk.Name
	if target == "" {
		target = hk.Command
	}
	if target == "" {
		target = hk.Type
	}
	event := hk.Event
	if event == "" {
		event = "hook"
	}
	if target == "" {
		return event
	}
	return event + ": " + target
}

// handleSkills implements GET /v1/skills. It discovers the on-disk skills under
// the workspace root (and the user home) via the same canonical loader the
// agent uses, so the Settings list matches the model-visible capability set.
// When no root is resolvable or discovery fails, it returns an empty list.
func (h *Handler) handleSkills(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	root := h.workspaceRoot()
	if root == "" {
		writeExtItems(w, nil)
		return
	}
	home, _ := os.UserHomeDir()
	store, err := skills.LoadScan(root, home)
	if err != nil {
		writeExtItems(w, nil)
		return
	}
	list := store.List()
	items := make([]extItem, 0, len(list))
	for _, sk := range list {
		items = append(items, extItem{ID: sk.Name, Name: sk.Name, Enabled: true})
	}
	writeExtItems(w, items)
}

// handleMemory implements GET /v1/memory. It enumerates the long-term memory
// records from the per-project JSONL store (<root>/.deepseek/memory.jsonl),
// the same file the memory-capture hook writes. Each non-deleted record becomes
// a row whose name is a short content snippet. A missing store yields an empty
// list.
func (h *Handler) handleMemory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	root := h.workspaceRoot()
	if root == "" {
		writeExtItems(w, nil)
		return
	}
	path := filepath.Join(root, ".deepseek", "memory.jsonl")
	items, err := readMemoryItems(path)
	if err != nil {
		writeExtItems(w, nil)
		return
	}
	writeExtItems(w, items)
}

// readMemoryItems reads the JSONL memory store at path and returns one extItem
// per live (non-deleted) record, applying the same last-write-wins compaction
// the store uses on load. A non-existent file is not an error (empty list).
func readMemoryItems(path string) ([]extItem, error) {
	f, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()

	// Compact in order: later lines for the same ID win; a Deleted line removes.
	type rec struct {
		mem   memory.Memory
		order int
	}
	live := map[string]rec{}
	order := 0
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for sc.Scan() {
		var m memory.Memory
		if err := json.Unmarshal(sc.Bytes(), &m); err != nil {
			continue // skip malformed lines, mirroring the store loader
		}
		if m.ID == "" {
			continue
		}
		if m.Deleted {
			delete(live, m.ID)
			continue
		}
		order++
		live[m.ID] = rec{mem: m, order: order}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}

	recs := make([]rec, 0, len(live))
	for _, rv := range live {
		recs = append(recs, rv)
	}
	sort.Slice(recs, func(i, j int) bool { return recs[i].order < recs[j].order })

	items := make([]extItem, 0, len(recs))
	for _, rv := range recs {
		items = append(items, extItem{
			ID:      rv.mem.ID,
			Name:    memorySnippet(rv.mem.Content),
			Enabled: true,
		})
	}
	return items, nil
}

// memorySnippet collapses a memory's content to a single-line, length-capped
// label for the Settings list.
func memorySnippet(content string) string {
	s := strings.TrimSpace(content)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = strings.TrimSpace(s[:i])
	}
	if r := []rune(s); len(r) > 80 {
		s = strings.TrimSpace(string(r[:80])) + "…"
	}
	if s == "" {
		return "(empty)"
	}
	return s
}
