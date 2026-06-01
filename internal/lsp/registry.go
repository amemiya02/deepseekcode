package lsp

import (
	"context"
	"sort"
	"sync"
	"time"
)

// ServerSnapshot is a point-in-time summary of a single language server's
// status. Returned by Registry.Snapshots.
type ServerSnapshot struct {
	Name            string
	Command         string
	Connected       bool
	DiagnosticCount int
	LastError       string
}

// failedServer records a server that failed to start.
type failedServer struct {
	Name      string
	Command   string
	LastError string
}

// Registry manages the lifecycle of multiple LSP clients, one per
// detected language.
type Registry struct {
	mu            sync.RWMutex
	clients       map[string]*Client
	commands      map[string]string // server name → command binary
	failedServers []failedServer
}

// NewRegistry returns an empty Registry.
func NewRegistry() *Registry {
	return &Registry{clients: make(map[string]*Client), commands: make(map[string]string)}
}

// Start detects language servers for cwd, spawns them, and adds them to
// the registry. Servers that fail to start are recorded as failed entries.
func (r *Registry) Start(ctx context.Context, cwd string) {
	servers := DetectServers(cwd)
	for _, si := range servers {
		rootURI := PathToURI(cwd)
		ctx2, cancel := context.WithTimeout(ctx, 15*time.Second)
		client, err := NewClient(ctx2, si.Name, si.Command, si.Args, rootURI)
		cancel()
		if err != nil {
			r.mu.Lock()
			r.failedServers = append(r.failedServers, failedServer{
				Name:      si.Name,
				Command:   si.Command,
				LastError: err.Error(),
			})
			r.mu.Unlock()
			continue
		}
		r.mu.Lock()
		r.clients[si.Name] = client
		r.commands[si.Name] = si.Command
		r.mu.Unlock()
	}
}

// Get returns the client for the given language server name, or false
// if no such server is connected.
func (r *Registry) Get(name string) (*Client, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	c, ok := r.clients[name]
	return c, ok
}

// ClientForURI returns the best LSP client for the file at the given
// path, determined by file extension.
func (r *Registry) ClientForURI(uri string) (Querier, bool) {
	path := URIToPath(uri)
	name := langFromPath(path)
	return r.Get(name)
}

// Shutdown closes all client connections. Each gets up to 5s to exit.
func (r *Registry) Shutdown() {
	r.mu.RLock()
	clients := make([]*Client, 0, len(r.clients))
	for _, c := range r.clients {
		clients = append(clients, c)
	}
	r.mu.RUnlock()

	var wg sync.WaitGroup
	for _, c := range clients {
		wg.Add(1)
		go func(client *Client) {
			defer wg.Done()
			_ = client.Close()
		}(c)
	}
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
	}
}

// Servers returns a snapshot of connected server names.
func (r *Registry) Servers() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.clients))
	for name := range r.clients {
		out = append(out, name)
	}
	return out
}

// Snapshots returns a deterministic summary of all known servers — both
// connected and failed — sorted by name. Diagnostic counts are read from
// each client's cache under its own lock, so the result may include
// concurrently-arriving diagnostics.
func (r *Registry) Snapshots() []ServerSnapshot {
	r.mu.RLock()
	connected := make(map[string]*Client, len(r.clients))
	cmds := make(map[string]string, len(r.commands))
	for name, c := range r.clients {
		connected[name] = c
	}
	for name, cmd := range r.commands {
		cmds[name] = cmd
	}
	failed := make([]failedServer, len(r.failedServers))
	copy(failed, r.failedServers)
	r.mu.RUnlock()

	seen := make(map[string]bool, len(connected)+len(failed))
	out := make([]ServerSnapshot, 0, len(connected)+len(failed))

	for name, c := range connected {
		out = append(out, ServerSnapshot{
			Name:            name,
			Command:         cmds[name],
			Connected:       true,
			DiagnosticCount: c.DiagnosticCount(),
		})
		seen[name] = true
	}

	for _, f := range failed {
		if seen[f.Name] {
			continue
		}
		out = append(out, ServerSnapshot{
			Name:      f.Name,
			Command:   f.Command,
			Connected: false,
			LastError: f.LastError,
		})
		seen[f.Name] = true
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].Name < out[j].Name
	})

	return out
}

// langFromPath maps a file path to a language server name.
func langFromPath(path string) string {
	base := fileExt(path)
	switch base {
	case ".go":
		return "gopls"
	case ".rs":
		return "rust-analyzer"
	case ".ts", ".tsx", ".js", ".jsx":
		return "typescript-language-server"
	case ".py":
		return "pylsp"
	default:
		return ""
	}
}

func fileExt(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '.' {
			return path[i:]
		}
	}
	return ""
}
