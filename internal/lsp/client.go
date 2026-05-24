package lsp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/amemiya02/deepseekcode/internal/version"
)

// Querier is the query surface an LSP client exposes. *Client implements it.
type Querier interface {
	Hover(ctx context.Context, uri string, line, character int) (string, error)
	Definition(ctx context.Context, uri string, line, character int) ([]Definition, error)
	References(ctx context.Context, uri string, line, character int) ([]Definition, error)
	Diagnostics(uri string) []Diagnostic
}

// Client is a connected LSP server. It handles the initialize handshake,
// text document sync (didOpen/didClose), and queries (hover, definition,
// references, diagnostics).
type Client struct {
	Name string
	t    *Transport
	caps ServerCapabilities

	mu               sync.RWMutex
	opened           map[string]bool         // URIs we've sent didOpen for
	diagnosticsByURI map[string][]Diagnostic // cached from publishDiagnostics
}

// Definition is a location returned by textDocument/definition or
// textDocument/references.
type Definition struct {
	URI       string
	Line      int
	Character int
}

// NewClient spawns the LSP server, runs initialize, and caches server
// capabilities. rootURI should be a file:// URI.
func NewClient(ctx context.Context, name, command string, args []string, rootURI string) (*Client, error) {
	t, err := NewTransport(ctx, command, args)
	if err != nil {
		return nil, fmt.Errorf("lsp %s: %w", name, err)
	}

	c := &Client{
		Name:             name,
		t:                t,
		opened:           make(map[string]bool),
		diagnosticsByURI: make(map[string][]Diagnostic),
	}

	// Wire notification handler before initialize so we don't miss
	// early publishDiagnostics.
	t.OnNotification = c.handleNotification

	caps, err := c.initialize(ctx, rootURI)
	if err != nil {
		t.Close()
		return nil, fmt.Errorf("lsp %s: %w", name, err)
	}
	c.caps = caps

	return c, nil
}

// initialize runs the LSP initialize handshake.
func (c *Client) initialize(ctx context.Context, rootURI string) (ServerCapabilities, error) {
	params := InitializeParams{
		ProcessID: 0, // not a subprocess of the server
		RootURI:   rootURI,
		Capabilities: ClientCapabilities{
			TextDocument: &TextDocumentClientCapabilities{
				Hover:      &struct{}{},
				Definition: &struct{}{},
				References: &struct{}{},
			},
		},
		ClientInfo: &struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		}{Name: "deepseekcode", Version: version.Version},
	}

	raw, err := c.t.Send(ctx, "initialize", params)
	if err != nil {
		return ServerCapabilities{}, err
	}

	var result InitializeResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return ServerCapabilities{}, err
	}

	if err := c.t.Notify(ctx, "initialized", struct{}{}); err != nil {
		return result.Capabilities, err
	}

	return result.Capabilities, nil
}

// didOpen sends a textDocument/didOpen notification if the URI hasn't
// been opened yet. Reads the file content so the LSP server can build
// its parse tree; falls back to empty text on failure.
func (c *Client) didOpen(ctx context.Context, uri string) error {
	c.mu.Lock()
	if c.opened[uri] {
		c.mu.Unlock()
		return nil
	}
	// Reserve the slot; rollback on Notify failure.
	c.opened[uri] = true
	c.mu.Unlock()

	text := readFileForLSP(URIToPath(uri))

	params := map[string]any{
		"textDocument": map[string]any{
			"uri":        uri,
			"languageId": languageIDFromURI(uri),
			"version":    1,
			"text":       text,
		},
	}
	if err := c.t.Notify(ctx, "textDocument/didOpen", params); err != nil {
		c.mu.Lock()
		delete(c.opened, uri)
		c.mu.Unlock()
		return err
	}
	return nil
}

// maxLSPOpenBytes caps file content sent in didOpen.
const maxLSPOpenBytes = 1 << 20 // 1 MiB

func readFileForLSP(path string) string {
	data, err := os.ReadFile(path)
	if err != nil || len(data) > maxLSPOpenBytes {
		return ""
	}
	return string(data)
}

// Hover requests hover information at the given position.
func (c *Client) Hover(ctx context.Context, uri string, line, character int) (string, error) {
	if err := c.didOpen(ctx, uri); err != nil {
		return "", err
	}

	params := TextDocumentPositionParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
		Position:     Position{Line: line, Character: character},
	}
	raw, err := c.t.Send(ctx, "textDocument/hover", params)
	if err != nil {
		return "", err
	}

	var hover Hover
	if err := json.Unmarshal(raw, &hover); err != nil {
		return "", err
	}
	if hover.Contents.Value == "" {
		return "", nil
	}
	return hover.Contents.Value, nil
}

// Definition requests the definition location(s) of the symbol at the
// given position.
func (c *Client) Definition(ctx context.Context, uri string, line, character int) ([]Definition, error) {
	if err := c.didOpen(ctx, uri); err != nil {
		return nil, err
	}

	params := TextDocumentPositionParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
		Position:     Position{Line: line, Character: character},
	}
	raw, err := c.t.Send(ctx, "textDocument/definition", params)
	if err != nil {
		return nil, err
	}

	return parseDefinitions(raw)
}

// References requests all references to the symbol at the given position.
func (c *Client) References(ctx context.Context, uri string, line, character int) ([]Definition, error) {
	if err := c.didOpen(ctx, uri); err != nil {
		return nil, err
	}

	params := map[string]any{
		"textDocument": map[string]any{"uri": uri},
		"position":     map[string]any{"line": line, "character": character},
		"context":      map[string]any{"includeDeclaration": false},
	}
	raw, err := c.t.Send(ctx, "textDocument/references", params)
	if err != nil {
		return nil, err
	}

	return parseDefinitions(raw)
}

// Diagnostics returns cached diagnostics for the given URI.
func (c *Client) Diagnostics(uri string) []Diagnostic {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.diagnosticsByURI[uri]
}

// Close sends a shutdown request, an exit notification, and tears down
// the transport. Each step has a 2s timeout so a hung server doesn't
// block dsc exit.
func (c *Client) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, _ = c.t.Send(ctx, "shutdown", nil)
	_ = c.t.Notify(ctx, "exit", nil)
	return c.t.Close()
}

// handleNotification is called by the transport for server→client
// notifications.
func (c *Client) handleNotification(method string, params json.RawMessage) {
	switch method {
	case "textDocument/publishDiagnostics":
		var p PublishDiagnosticsParams
		if err := json.Unmarshal(params, &p); err != nil {
			return
		}
		c.mu.Lock()
		c.diagnosticsByURI[p.URI] = p.Diagnostics
		c.mu.Unlock()
	}
}

// parseDefinitions unmarshals definition/references results. LSP returns
// a single Location, an array of Locations, or null.
func parseDefinitions(raw json.RawMessage) ([]Definition, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}

	// Try array first.
	var locs []Location
	if err := json.Unmarshal(raw, &locs); err == nil {
		out := make([]Definition, len(locs))
		for i, l := range locs {
			out[i] = Definition{
				URI:       l.URI,
				Line:      l.Range.Start.Line,
				Character: l.Range.Start.Character,
			}
		}
		return out, nil
	}

	// Single location.
	var loc Location
	if err := json.Unmarshal(raw, &loc); err != nil {
		return nil, nil
	}
	return []Definition{{
		URI:       loc.URI,
		Line:      loc.Range.Start.Line,
		Character: loc.Range.Start.Character,
	}}, nil
}

// PathToURI converts a filesystem path to a file:// URI.
func PathToURI(path string) string {
	// On non-Windows, drive-letter paths (e.g. "C:/foo") won't be
	// resolved by filepath.Abs; treat them as already absolute.
	isAbs := filepath.IsAbs(path)
	if !isAbs {
		abs, err := filepath.Abs(path)
		if err == nil {
			path = abs
		}
	}
	path = filepath.ToSlash(path)
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return "file://" + path
}

// URIToPath converts a file:// URI to a filesystem path.
func URIToPath(uri string) string {
	path := strings.TrimPrefix(uri, "file://")
	return filepath.FromSlash(path)
}

func languageIDFromURI(uri string) string {
	ext := filepath.Ext(uri)
	switch strings.ToLower(ext) {
	case ".go":
		return "go"
	case ".rs":
		return "rust"
	case ".ts", ".tsx":
		return "typescript"
	case ".js", ".jsx":
		return "javascript"
	case ".py":
		return "python"
	case ".md":
		return "markdown"
	case ".json":
		return "json"
	case ".toml":
		return "toml"
	case ".yaml", ".yml":
		return "yaml"
	default:
		return ""
	}
}
