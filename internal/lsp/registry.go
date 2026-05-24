package lsp

import (
	"context"
	"sync"
	"time"
)

// Registry manages the lifecycle of multiple LSP clients, one per
// detected language.
type Registry struct {
	mu      sync.RWMutex
	clients map[string]*Client
}

// NewRegistry returns an empty Registry.
func NewRegistry() *Registry {
	return &Registry{clients: make(map[string]*Client)}
}

// Start detects language servers for cwd, spawns them, and adds them to
// the registry. Servers that fail to start are skipped with a warning log.
func (r *Registry) Start(ctx context.Context, cwd string) {
	servers := DetectServers(cwd)
	for _, si := range servers {
		rootURI := PathToURI(cwd)
		ctx2, cancel := context.WithTimeout(ctx, 15*time.Second)
		client, err := NewClient(ctx2, si.Name, si.Command, si.Args, rootURI)
		cancel()
		if err != nil {
			continue
		}
		r.mu.Lock()
		r.clients[si.Name] = client
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
