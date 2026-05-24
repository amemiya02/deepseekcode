package lsp

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// ---------- T-902: Transport framing ----------

func TestLSPTransportFraming(t *testing.T) {
	cr, sw := io.Pipe()
	sr, cw := io.Pipe()

	serverDone := make(chan struct{})
	go func() {
		defer close(serverDone)
		defer cw.Close()
		body, err := readLSPTestMessage(sr)
		if err != nil {
			t.Errorf("server read: %v", err)
			return
		}
		var req jsonRPCRequest
		if err := json.Unmarshal(body, &req); err != nil {
			t.Errorf("server unmarshal: %v", err)
			return
		}
		if req.Method != "test/ping" {
			t.Errorf("expected test/ping, got %q", req.Method)
		}
		resp := jsonRPCResponse{JSONRPC: "2.0", ID: req.ID, Result: json.RawMessage(`"pong"`)}
		b, _ := json.Marshal(resp)
		writeLSPTestMessage(sw, b)
	}()

	transport := newPipeTransport(cr, cw)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	raw, err := transport.Send(ctx, "test/ping", map[string]string{"key": "value"})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if string(raw) != `"pong"` {
		t.Errorf("result = %s, want \"pong\"", raw)
	}
	transport.Close()
	<-serverDone
}

func TestLSPTransportNotification(t *testing.T) {
	cr, _ := io.Pipe()
	sr, cw := io.Pipe()

	gotNotif := make(chan struct{})
	go func() {
		defer cw.Close()
		body, err := readLSPTestMessage(sr)
		if err != nil {
			t.Errorf("server read: %v", err)
			return
		}
		var notif jsonRPCNotification
		if err := json.Unmarshal(body, &notif); err != nil {
			t.Errorf("server unmarshal: %v", err)
			return
		}
		if notif.Method != "test/notify" {
			t.Errorf("expected test/notify, got %q", notif.Method)
		}
		close(gotNotif)
	}()

	transport := newPipeTransport(cr, cw)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := transport.Notify(ctx, "test/notify", map[string]string{"msg": "hello"}); err != nil {
		t.Fatalf("Notify: %v", err)
	}
	transport.Close()
	<-gotNotif
}

func TestLSPTransportServerNotification(t *testing.T) {
	cr, sw := io.Pipe()
	_, cw := io.Pipe()

	got := make(chan string, 1)
	transport := newPipeTransport(cr, cw)
	transport.OnNotification = func(method string, params json.RawMessage) {
		got <- method
	}

	notif := jsonRPCNotification{
		JSONRPC: "2.0",
		Method:  "textDocument/publishDiagnostics",
		Params: PublishDiagnosticsParams{
			URI: "file:///test.go",
			Diagnostics: []Diagnostic{
				{Range: Range{Start: Position{Line: 0, Character: 0}, End: Position{Line: 0, Character: 5}}, Message: "test error", Severity: 1},
			},
		},
	}
	body, _ := json.Marshal(notif)
	writeLSPTestMessage(sw, body)
	cw.Close()

	select {
	case method := <-got:
		if method != "textDocument/publishDiagnostics" {
			t.Errorf("got method %q, want publishDiagnostics", method)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for notification")
	}
	transport.Close()
}

// ---------- T-903: Initialize handshake ----------

func TestLSPInitialize(t *testing.T) {
	cr, sw := io.Pipe()
	sr, cw := io.Pipe()

	serverDone := make(chan struct{})
	go func() {
		defer close(serverDone)
		defer cw.Close()

		// Read initialize.
		body, _ := readLSPTestMessage(sr)
		var req jsonRPCRequest
		json.Unmarshal(body, &req)

		result := InitializeResult{
			Capabilities: ServerCapabilities{
				HoverProvider:      true,
				DefinitionProvider: true,
				ReferencesProvider: true,
			},
		}
		b, _ := json.Marshal(result)
		resp := jsonRPCResponse{JSONRPC: "2.0", ID: req.ID, Result: b}
		rbody, _ := json.Marshal(resp)
		writeLSPTestMessage(sw, rbody)

		// Read initialized notification.
		body, _ = readLSPTestMessage(sr)
		var notif jsonRPCNotification
		json.Unmarshal(body, &notif)
		if notif.Method != "initialized" {
			t.Errorf("expected initialized, got %q", notif.Method)
		}
	}()

	transport := newPipeTransport(cr, cw)
	c := newTestClient(transport)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	caps, err := c.initialize(ctx, "file:///test")
	if err != nil {
		t.Fatalf("initialize: %v", err)
	}
	if !caps.HoverProvider {
		t.Error("expected HoverProvider true")
	}
	if !caps.DefinitionProvider {
		t.Error("expected DefinitionProvider true")
	}

	transport.Close()
	<-serverDone
}

// ---------- T-904~906: Hover, Definition, References ----------

func TestLSPHover(t *testing.T) {
	cr, sw := io.Pipe()
	sr, cw := io.Pipe()

	serverDone := make(chan struct{})
	go func() {
		defer close(serverDone)
		defer cw.Close()
		// didOpen
		readLSPTestMessage(sr)
		// hover
		body, _ := readLSPTestMessage(sr)
		var req jsonRPCRequest
		json.Unmarshal(body, &req)

		hover := Hover{Contents: MarkupContent{Kind: "markdown", Value: "func Foo() int"}}
		b, _ := json.Marshal(hover)
		resp := jsonRPCResponse{JSONRPC: "2.0", ID: req.ID, Result: b}
		rbody, _ := json.Marshal(resp)
		writeLSPTestMessage(sw, rbody)
	}()

	transport := newPipeTransport(cr, cw)
	c := newTestClient(transport)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	text, err := c.Hover(ctx, "file:///test.go", 10, 5)
	if err != nil {
		t.Fatalf("Hover: %v", err)
	}
	if text != "func Foo() int" {
		t.Errorf("got %q, want 'func Foo() int'", text)
	}
	transport.Close()
	<-serverDone
}

func TestLSPDefinition(t *testing.T) {
	cr, sw := io.Pipe()
	sr, cw := io.Pipe()

	serverDone := make(chan struct{})
	go func() {
		defer close(serverDone)
		defer cw.Close()
		readLSPTestMessage(sr) // didOpen
		body, _ := readLSPTestMessage(sr)
		var req jsonRPCRequest
		json.Unmarshal(body, &req)

		loc := Location{URI: "file:///test.go", Range: Range{Start: Position{Line: 5, Character: 0}, End: Position{Line: 5, Character: 3}}}
		b, _ := json.Marshal(loc)
		resp := jsonRPCResponse{JSONRPC: "2.0", ID: req.ID, Result: b}
		rbody, _ := json.Marshal(resp)
		writeLSPTestMessage(sw, rbody)
	}()

	transport := newPipeTransport(cr, cw)
	c := newTestClient(transport)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	defs, err := c.Definition(ctx, "file:///test.go", 10, 5)
	if err != nil {
		t.Fatalf("Definition: %v", err)
	}
	if len(defs) != 1 {
		t.Fatalf("got %d defs, want 1", len(defs))
	}
	if defs[0].Line != 5 {
		t.Errorf("line = %d, want 5", defs[0].Line)
	}
	transport.Close()
	<-serverDone
}

func TestLSPReferences(t *testing.T) {
	cr, sw := io.Pipe()
	sr, cw := io.Pipe()

	serverDone := make(chan struct{})
	go func() {
		defer close(serverDone)
		defer cw.Close()
		readLSPTestMessage(sr) // didOpen
		body, _ := readLSPTestMessage(sr)
		var req jsonRPCRequest
		json.Unmarshal(body, &req)

		locs := []Location{
			{URI: "file:///a.go", Range: Range{Start: Position{Line: 1, Character: 2}}},
			{URI: "file:///b.go", Range: Range{Start: Position{Line: 3, Character: 4}}},
		}
		b, _ := json.Marshal(locs)
		resp := jsonRPCResponse{JSONRPC: "2.0", ID: req.ID, Result: b}
		rbody, _ := json.Marshal(resp)
		writeLSPTestMessage(sw, rbody)
	}()

	transport := newPipeTransport(cr, cw)
	c := newTestClient(transport)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	refs, err := c.References(ctx, "file:///test.go", 10, 5)
	if err != nil {
		t.Fatalf("References: %v", err)
	}
	if len(refs) != 2 {
		t.Fatalf("got %d refs, want 2", len(refs))
	}
	transport.Close()
	<-serverDone
}

func TestLSPDefinitionNull(t *testing.T) {
	cr, sw := io.Pipe()
	sr, cw := io.Pipe()

	serverDone := make(chan struct{})
	go func() {
		defer close(serverDone)
		defer cw.Close()
		readLSPTestMessage(sr)
		body, _ := readLSPTestMessage(sr)
		var req jsonRPCRequest
		json.Unmarshal(body, &req)

		resp := jsonRPCResponse{JSONRPC: "2.0", ID: req.ID, Result: json.RawMessage("null")}
		rbody, _ := json.Marshal(resp)
		writeLSPTestMessage(sw, rbody)
	}()

	transport := newPipeTransport(cr, cw)
	c := newTestClient(transport)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	defs, err := c.Definition(ctx, "file:///test.go", 10, 5)
	if err != nil {
		t.Fatalf("Definition: %v", err)
	}
	if len(defs) != 0 {
		t.Errorf("got %d defs, want 0 for null response", len(defs))
	}
	transport.Close()
	<-serverDone
}

// ---------- T-907: Diagnostics ----------

func TestLSPDiagnostics(t *testing.T) {
	cr, sw := io.Pipe()
	_, cw := io.Pipe()

	transport := newPipeTransport(cr, cw)
	c := newTestClient(transport)
	transport.OnNotification = c.handleNotification

	notif := jsonRPCNotification{
		JSONRPC: "2.0",
		Method:  "textDocument/publishDiagnostics",
		Params: PublishDiagnosticsParams{
			URI: "file:///test.go",
			Diagnostics: []Diagnostic{
				{Range: Range{Start: Position{Line: 0, Character: 0}, End: Position{Line: 0, Character: 5}}, Message: "unused variable", Severity: 2, Source: "gopls"},
			},
		},
	}
	body, _ := json.Marshal(notif)
	writeLSPTestMessage(sw, body)
	cw.Close()

	time.Sleep(100 * time.Millisecond)

	diags := c.Diagnostics("file:///test.go")
	if len(diags) != 1 {
		t.Fatalf("got %d diagnostics, want 1", len(diags))
	}
	if diags[0].Message != "unused variable" {
		t.Errorf("message = %q", diags[0].Message)
	}
	transport.Close()
}

// ---------- T-908: Language detection ----------

func TestDetectServersNoFiles(t *testing.T) {
	dir := t.TempDir()
	servers := DetectServers(dir)
	if len(servers) != 0 {
		t.Fatalf("expected 0 servers, got %d", len(servers))
	}
}

func TestDetectServersGoMod(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test\n"), 0o644)
	servers := DetectServers(dir)

	_, goplsErr := exec.LookPath("gopls")
	hasGopls := goplsErr == nil
	for _, s := range servers {
		if s.Name == "gopls" && s.Command != "gopls" {
			t.Errorf("gopls command = %q", s.Command)
		}
		if s.Name == "gopls" && !hasGopls {
			t.Error("gopls detected but not in PATH?")
		}
		if s.Name != "gopls" {
			t.Errorf("unexpected server: %s", s.Name)
		}
	}
	if hasGopls && len(servers) == 0 {
		t.Error("go.mod exists and gopls in PATH but not detected")
	}
}

func TestDetectServersDedup(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte("[project]\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "requirements.txt"), []byte("requests\n"), 0o644)

	servers := DetectServers(dir)
	count := 0
	for _, s := range servers {
		if s.Name == "pylsp" {
			count++
		}
	}
	if count > 1 {
		t.Error("pylsp should be detected at most once")
	}
}

// ---------- T-909: Registry ----------

func TestRegistryEmpty(t *testing.T) {
	reg := NewRegistry()
	if _, ok := reg.Get("gopls"); ok {
		t.Error("expected not found")
	}
	if _, ok := reg.ClientForURI("file:///test.go"); ok {
		t.Error("expected not found")
	}
	if len(reg.Servers()) != 0 {
		t.Errorf("expected 0 servers, got %d", len(reg.Servers()))
	}
	reg.Shutdown() // should not panic
}

// ---------- URI helpers ----------

func TestPathToURIRoundtrip(t *testing.T) {
	uri := PathToURI("/home/user/project/main.go")
	if !strings.HasPrefix(uri, "file://") {
		t.Errorf("URI should start with file://, got %q", uri)
	}
	path := URIToPath(uri)
	if path != "/home/user/project/main.go" {
		t.Errorf("roundtrip: got %q, want /home/user/project/main.go", path)
	}
}

func TestPathToURIRelative(t *testing.T) {
	uri := PathToURI("main.go")
	if !strings.HasPrefix(uri, "file://") {
		t.Errorf("URI should start with file:// for relative path, got %q", uri)
	}
}

// ---------- M2: server→client request dispatch ----------

func TestTransportServerRequest(t *testing.T) {
	cr, sw := io.Pipe()
	sr, cw := io.Pipe()

	// The transport writes MethodNotFound responses to cw. We'll read them from sr.
	transport := newPipeTransport(cr, cw)

	// Write a server→client request (has both id and method).
	req := jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      42,
		Method:  "workspace/configuration",
		Params:  map[string]any{"items": []any{}},
	}
	body, _ := json.Marshal(req)
	writeLSPTestMessage(sw, body)

	// Read the transport's response.
	respBody, err := readLSPTestMessage(sr)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}

	var resp jsonRPCResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.ID != 42 {
		t.Errorf("response ID = %d, want 42", resp.ID)
	}
	if resp.Error == nil {
		t.Fatal("expected error response")
	}
	if resp.Error.Code != -32601 {
		t.Errorf("error code = %d, want -32601 (MethodNotFound)", resp.Error.Code)
	}

	transport.Close()
	sw.Close()
}

// ---------- m1: Cargo / tsconfig fixture tests ----------

func TestDetectServersCargo(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "Cargo.toml"), []byte("[package]\nname = \"test\"\n"), 0o644)

	servers := DetectServers(dir)
	_, raErr := exec.LookPath("rust-analyzer")
	hasRA := raErr == nil
	for _, s := range servers {
		if s.Name == "rust-analyzer" {
			if s.Command != "rust-analyzer" {
				t.Errorf("rust-analyzer command = %q", s.Command)
			}
			if !hasRA {
				t.Error("rust-analyzer detected but not in PATH?")
			}
		}
		if s.Name != "rust-analyzer" {
			t.Errorf("unexpected server for Cargo.toml: %s", s.Name)
		}
	}
	if hasRA && len(servers) == 0 {
		t.Error("Cargo.toml exists and rust-analyzer in PATH but not detected")
	}
}

func TestDetectServersTSConfig(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "tsconfig.json"), []byte("{}\n"), 0o644)

	servers := DetectServers(dir)
	_, tsErr := exec.LookPath("typescript-language-server")
	hasTS := tsErr == nil
	for _, s := range servers {
		if s.Name == "typescript-language-server" {
			if s.Command != "typescript-language-server" {
				t.Errorf("typescript-language-server command = %q", s.Command)
			}
			if fmt := len(s.Args); fmt != 1 || s.Args[0] != "--stdio" {
				t.Errorf("expected --stdio arg, got %v", s.Args)
			}
			if !hasTS {
				t.Error("typescript-language-server detected but not in PATH?")
			}
		}
		if s.Name != "typescript-language-server" {
			t.Errorf("unexpected server for tsconfig.json: %s", s.Name)
		}
	}
	if hasTS && len(servers) == 0 {
		t.Error("tsconfig.json exists and typescript-language-server in PATH but not detected")
	}
}

// ---------- m2: Populated Registry test ----------

func TestRegistryPopulated(t *testing.T) {
	reg := NewRegistry()

	// Simulate an injected client by adding directly to the map
	// (same-package test can access private fields).
	c := &Client{Name: "gopls"}
	reg.mu.Lock()
	reg.clients["gopls"] = c
	reg.mu.Unlock()

	// Get by name.
	got, ok := reg.Get("gopls")
	if !ok {
		t.Fatal("expected gopls client")
	}
	if got.Name != "gopls" {
		t.Errorf("got %q, want gopls", got.Name)
	}

	// Servers list.
	names := reg.Servers()
	if len(names) != 1 || names[0] != "gopls" {
		t.Errorf("Servers() = %v", names)
	}

	// ClientForURI by extension.
	got2, ok := reg.ClientForURI("file:///proj/main.go")
	if !ok {
		t.Fatal("expected client for .go file via ClientForURI")
	}
	c2 := got2.(*Client)
	if c2.Name != "gopls" {
		t.Errorf("got %q via ClientForURI", c2.Name)
	}

	// Unknown extension → not found.
	_, ok = reg.ClientForURI("file:///proj/README.md")
	if ok {
		t.Error("expected not found for .md file")
	}
}

// ---------- m5: Windows path roundtrip ----------

func TestPathToURIWindowsDrive(t *testing.T) {
	// On non-Windows, drive-letter paths won't be recognised as absolute
	// by filepath.IsAbs. Verify the URI construction handles slashes
	// and that URIToPath roundtrips correctly for absolute paths.
	uri := PathToURI("/Users/test/main.go")
	if !strings.HasPrefix(uri, "file:///Users/") {
		t.Errorf("got %q", uri)
	}
	path := URIToPath(uri)
	if path != "/Users/test/main.go" {
		t.Errorf("roundtrip: got %q, want /Users/test/main.go", path)
	}
}

// ---------- m8: Diagnostics with sync signal ----------

func TestLSPDiagnosticsSync(t *testing.T) {
	cr, sw := io.Pipe()
	_, cw := io.Pipe()

	transport := newPipeTransport(cr, cw)
	notified := make(chan struct{})

	c := newTestClient(transport)
	transport.OnNotification = func(method string, params json.RawMessage) {
		c.handleNotification(method, params)
		close(notified)
	}

	notif := jsonRPCNotification{
		JSONRPC: "2.0",
		Method:  "textDocument/publishDiagnostics",
		Params: PublishDiagnosticsParams{
			URI: "file:///test.go",
			Diagnostics: []Diagnostic{
				{Range: Range{Start: Position{Line: 0, Character: 0}, End: Position{Line: 0, Character: 5}}, Message: "sync test", Severity: 1},
			},
		},
	}
	body, _ := json.Marshal(notif)
	writeLSPTestMessage(sw, body)
	cw.Close()

	select {
	case <-notified:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for notification")
	}

	diags := c.Diagnostics("file:///test.go")
	if len(diags) != 1 {
		t.Fatalf("got %d diagnostics, want 1", len(diags))
	}
	if diags[0].Message != "sync test" {
		t.Errorf("message = %q", diags[0].Message)
	}
	transport.Close()
}

// ---------- Helpers ----------

func newTestClient(t *Transport) *Client {
	return &Client{
		Name:             "test",
		t:                t,
		opened:           make(map[string]bool),
		diagnosticsByURI: make(map[string][]Diagnostic),
	}
}

// readLSPTestMessage reads a Content-Length framed message.
func readLSPTestMessage(rd io.Reader) ([]byte, error) {
	buf := make([]byte, 1)
	var header []byte
	for {
		if _, err := io.ReadFull(rd, buf); err != nil {
			return nil, err
		}
		header = append(header, buf[0])
		if len(header) >= 4 && string(header[len(header)-4:]) == "\r\n\r\n" {
			break
		}
	}
	var contentLength int
	for _, line := range strings.Split(string(header[:len(header)-4]), "\r\n") {
		if after, ok := strings.CutPrefix(line, "Content-Length:"); ok {
			contentLength, _ = strconv.Atoi(strings.TrimSpace(after))
		}
	}
	if contentLength <= 0 {
		return nil, io.ErrUnexpectedEOF
	}
	body := make([]byte, contentLength)
	if _, err := io.ReadFull(rd, body); err != nil {
		return nil, err
	}
	return body, nil
}

func writeLSPTestMessage(w io.Writer, body []byte) error {
	header := "Content-Length: " + strconv.Itoa(len(body)) + "\r\n\r\n"
	if _, err := io.WriteString(w, header); err != nil {
		return err
	}
	_, err := w.Write(body)
	return err
}
