package usageproxy

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func startUpstream(t *testing.T, h http.HandlerFunc) *httptest.Server {
	t.Helper()
	s := httptest.NewServer(h)
	t.Cleanup(s.Close)
	return s
}

func startProxy(t *testing.T, upstream string) *Proxy {
	t.Helper()
	p, err := Start(upstream)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { p.Close() })
	return p
}

func TestNonStreamUsageCaptured(t *testing.T) {
	body := `{"choices":[{"message":{"content":"hi"}}],"usage":{"prompt_cache_hit_tokens":900,"prompt_cache_miss_tokens":100,"completion_tokens":42}}`
	up := startUpstream(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, body)
	})
	p := startProxy(t, up.URL)

	resp, err := http.Post(p.URL()+"/chat/completions", "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if string(got) != body {
		t.Fatalf("body not passed through:\nwant %q\ngot  %q", body, got)
	}
	u := p.Usages()
	if len(u) != 1 || u[0].HitTokens != 900 || u[0].MissTokens != 100 || u[0].OutTokens != 42 {
		t.Fatalf("bad usage records: %+v", u)
	}
}

// waitForUsage polls p.Usages() until it has at least n records or times out.
// This guards against the race where the HTTP response reaches the client
// before the proxy handler goroutine has appended the usage record.
func waitForUsage(t *testing.T, p *Proxy, n int) []Usage {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if u := p.Usages(); len(u) >= n {
			return u
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %d usage record(s), got %d", n, len(p.Usages()))
	return nil
}

func TestSSEUsageCaptured(t *testing.T) {
	chunks := []string{
		`data: {"choices":[{"delta":{"content":"h"}}]}`,
		`data: {"choices":[{"delta":{"content":"i"}}],"usage":null}`,
		`data: {"choices":[],"usage":{"prompt_cache_hit_tokens":1100,"prompt_cache_miss_tokens":10,"completion_tokens":80}}`,
		`data: [DONE]`,
	}
	sse := strings.Join(chunks, "\n\n") + "\n\n"
	up := startUpstream(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, sse)
	})
	p := startProxy(t, up.URL)

	resp, err := http.Post(p.URL()+"/chat/completions", "application/json", strings.NewReader(`{"stream":true}`))
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if string(got) != sse {
		t.Fatalf("SSE not passed through:\nwant %q\ngot  %q", sse, got)
	}
	u := waitForUsage(t, p, 1)
	if len(u) != 1 || u[0].HitTokens != 1100 || u[0].MissTokens != 10 || u[0].OutTokens != 80 {
		t.Fatalf("bad usage records: %+v", u)
	}
}

func TestErrorResponseNoUsage(t *testing.T) {
	up := startUpstream(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		io.WriteString(w, `{"error":"boom"}`)
	})
	p := startProxy(t, up.URL)

	resp, err := http.Post(p.URL()+"/chat/completions", "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 500 {
		t.Fatalf("status not passed through: %d", resp.StatusCode)
	}
	if u := p.Usages(); len(u) != 0 {
		t.Fatalf("unexpected usage from error response: %+v", u)
	}
}

func TestAuthHeaderForwardedAndAcceptEncodingStripped(t *testing.T) {
	var auth, accEnc string
	up := startUpstream(t, func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		accEnc = r.Header.Get("Accept-Encoding")
		io.WriteString(w, `{}`)
	})
	p := startProxy(t, up.URL)

	req, _ := http.NewRequest("POST", p.URL()+"/chat/completions", strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer sk-test")
	req.Header.Set("Accept-Encoding", "gzip")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if auth != "Bearer sk-test" {
		t.Fatalf("Authorization not forwarded: %q", auth)
	}
	if accEnc != "" {
		t.Fatalf("Accept-Encoding should be stripped, got %q", accEnc)
	}
}

func TestMultipleRequestsAccumulate(t *testing.T) {
	up := startUpstream(t, func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"usage":{"prompt_cache_hit_tokens":1,"prompt_cache_miss_tokens":2,"completion_tokens":3}}`)
	})
	p := startProxy(t, up.URL)
	for i := 0; i < 3; i++ {
		resp, err := http.Post(p.URL()+"/chat/completions", "application/json", strings.NewReader(`{}`))
		if err != nil {
			t.Fatal(err)
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}
	if u := p.Usages(); len(u) != 3 {
		t.Fatalf("want 3 usage records, got %d", len(u))
	}
}
