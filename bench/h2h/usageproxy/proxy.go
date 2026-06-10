// Package usageproxy is a loopback pass-through HTTP proxy that
// forwards an OpenAI-compatible agent's API traffic to its upstream
// and harvests the provider-reported `usage` object from each
// response. It exists because reasonix v1.0.0 exposes no token usage
// through ACP, its transcripts, or stderr (verified live 2026-06-10):
// the provider response on the wire is the only ground truth.
package usageproxy

import (
	"bufio"
	"bytes"
	"encoding/json"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
)

// Usage is one chat completion's provider-reported token counters
// (DeepSeek response `usage` object; same keys as acpclient.Usage).
type Usage struct {
	HitTokens  int `json:"prompt_cache_hit_tokens"`
	MissTokens int `json:"prompt_cache_miss_tokens"`
	OutTokens  int `json:"completion_tokens"`
}

// Proxy is a single-upstream reverse proxy bound to 127.0.0.1.
type Proxy struct {
	upstream  *url.URL
	ln        net.Listener
	srv       *http.Server
	transport *http.Transport

	mu     sync.Mutex
	usages []Usage
}

// Start listens on an ephemeral loopback port and forwards all
// requests to upstream (e.g. "https://api.deepseek.com").
func Start(upstream string) (*Proxy, error) {
	u, err := url.Parse(upstream)
	if err != nil {
		return nil, err
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	p := &Proxy{
		upstream: u,
		ln:       ln,
		// DisableCompression keeps response bodies parseable; agent
		// turns can stream for minutes, so no client-side timeouts.
		transport: &http.Transport{DisableCompression: true},
	}
	p.srv = &http.Server{Handler: http.HandlerFunc(p.handle)}
	go p.srv.Serve(ln)
	return p, nil
}

// URL returns the proxy's base URL, e.g. "http://127.0.0.1:54321".
func (p *Proxy) URL() string {
	return "http://" + p.ln.Addr().String()
}

// Usages returns a copy of all usage records harvested so far.
func (p *Proxy) Usages() []Usage {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]Usage(nil), p.usages...)
}

// Close shuts the listener down. In-flight requests are aborted.
func (p *Proxy) Close() error {
	err := p.srv.Close()
	p.transport.CloseIdleConnections()
	return err
}

// hop-by-hop headers must not be forwarded (RFC 9110 §7.6.1).
var hopByHop = []string{
	"Connection", "Keep-Alive", "Proxy-Authenticate", "Proxy-Authorization",
	"Proxy-Connection", "Te", "Trailer", "Transfer-Encoding", "Upgrade",
}

func (p *Proxy) handle(w http.ResponseWriter, r *http.Request) {
	out := *p.upstream
	out.Path = singleJoin(p.upstream.Path, r.URL.Path)
	out.RawQuery = r.URL.RawQuery

	req, err := http.NewRequestWithContext(r.Context(), r.Method, out.String(), r.Body)
	if err != nil {
		http.Error(w, "proxy: "+err.Error(), http.StatusBadGateway)
		return
	}
	req.Header = r.Header.Clone()
	for _, h := range hopByHop {
		req.Header.Del(h)
	}
	// Force identity encoding so the response body is parseable here.
	req.Header.Del("Accept-Encoding")
	req.Host = p.upstream.Host

	resp, err := p.transport.RoundTrip(req)
	if err != nil {
		http.Error(w, "proxy: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	hdr := w.Header()
	for k, vv := range resp.Header {
		for _, v := range vv {
			hdr.Add(k, v)
		}
	}
	for _, h := range hopByHop {
		hdr.Del(h)
	}
	w.WriteHeader(resp.StatusCode)

	isSSE := strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream")
	flusher, _ := w.(http.Flusher)
	br := bufio.NewReader(resp.Body)
	var jsonBody bytes.Buffer // accumulated only for non-SSE responses (small)
	var last *Usage           // last usage object seen in this response

	for {
		line, rerr := br.ReadBytes('\n')
		if len(line) > 0 {
			if _, werr := w.Write(line); werr != nil {
				return // client gone; stop relaying but keep what we parsed
			}
			if flusher != nil && isSSE {
				flusher.Flush()
			}
			if isSSE {
				if u := parseSSELine(line); u != nil {
					last = u
				}
			} else {
				jsonBody.Write(line)
			}
		}
		if rerr != nil {
			break
		}
	}
	if !isSSE {
		last = parseUsage(jsonBody.Bytes())
	}
	if last != nil && resp.StatusCode < 300 {
		p.mu.Lock()
		p.usages = append(p.usages, *last)
		p.mu.Unlock()
	}
}

// parseSSELine extracts a usage object from one "data: {...}" line.
func parseSSELine(line []byte) *Usage {
	s := bytes.TrimSpace(line)
	if !bytes.HasPrefix(s, []byte("data:")) {
		return nil
	}
	payload := bytes.TrimSpace(s[len("data:"):])
	if len(payload) == 0 || bytes.Equal(payload, []byte("[DONE]")) {
		return nil
	}
	return parseUsage(payload)
}

func parseUsage(b []byte) *Usage {
	var v struct {
		Usage *Usage `json:"usage"`
	}
	if json.Unmarshal(b, &v) != nil {
		return nil
	}
	return v.Usage
}

func singleJoin(a, b string) string {
	switch {
	case a == "":
		return b
	case strings.HasSuffix(a, "/") && strings.HasPrefix(b, "/"):
		return a + b[1:]
	case !strings.HasSuffix(a, "/") && !strings.HasPrefix(b, "/") && b != "":
		return a + "/" + b
	default:
		return a + b
	}
}
