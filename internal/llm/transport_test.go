package llm_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/amemiya02/deepseekcode/internal/llm"
)

// TestProxyTransport_UsesHTTPSProxy verifies that an HTTPS_PROXY env var is
// honoured by the transport returned from ProxyTransport().
func TestProxyTransport_UsesHTTPSProxy(t *testing.T) {
	// Proxy server that records whether it was called.
	var proxyCalled bool
	proxyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxyCalled = true
		// Return a minimal 200 so the client doesn't error.
		w.WriteHeader(http.StatusOK)
	}))
	defer proxyServer.Close()
	_ = proxyCalled // recorded by closure; not asserted here (proxy fn is tested below)

	os.Setenv("HTTPS_PROXY", proxyServer.URL)
	defer os.Unsetenv("HTTPS_PROXY")
	os.Unsetenv("DEEPSEEKCODE_PROXY")

	tr := llm.ProxyTransport()
	client := &http.Client{Transport: tr}

	// We can't actually CONNECT to a TLS host in a unit test, but we can
	// verify the transport's proxy func returns our proxy URL for an https URL.
	proxyFn := tr.Proxy
	if proxyFn == nil {
		t.Fatal("ProxyTransport().Proxy is nil; expected ProxyFromEnvironment-based func")
	}
	testReq, _ := http.NewRequest("GET", "https://api.deepseek.com/v1/chat/completions", nil)
	got, err := proxyFn(testReq)
	if err != nil {
		t.Fatalf("proxy func returned error: %v", err)
	}
	if got == nil {
		t.Fatal("proxy func returned nil URL; expected proxy to be set")
	}
	if !strings.HasPrefix(got.String(), "http://127.0.0.1") {
		t.Errorf("proxy URL = %q; want local test server", got)
	}
	_ = client // used above indirectly
}

// TestProxyTransport_DeepseekProxyOverride verifies DEEPSEEKCODE_PROXY takes
// precedence over HTTPS_PROXY.
func TestProxyTransport_DeepseekProxyOverride(t *testing.T) {
	os.Setenv("HTTPS_PROXY", "http://should-not-be-used.invalid:3128")
	defer os.Unsetenv("HTTPS_PROXY")
	os.Setenv("DEEPSEEKCODE_PROXY", "http://127.0.0.1:9999")
	defer os.Unsetenv("DEEPSEEKCODE_PROXY")

	tr := llm.ProxyTransport()
	if tr.Proxy == nil {
		t.Fatal("ProxyTransport().Proxy is nil")
	}
	req, _ := http.NewRequest("GET", "https://api.deepseek.com/v1/chat/completions", nil)
	got, err := tr.Proxy(req)
	if err != nil {
		t.Fatalf("proxy func error: %v", err)
	}
	if got == nil || got.Host != "127.0.0.1:9999" {
		t.Errorf("DEEPSEEKCODE_PROXY override failed; got proxy = %v", got)
	}
}

// TestNewClientWithEnv_BaseURLFromEnv verifies that DEEPSEEKCODE_BASE_URL is
// picked up when set.
func TestNewClientWithEnv_BaseURLFromEnv(t *testing.T) {
	os.Setenv("DEEPSEEKCODE_API_KEY", "test-key")
	defer os.Unsetenv("DEEPSEEKCODE_API_KEY")
	os.Setenv("DEEPSEEKCODE_BASE_URL", "https://mirror.example.com")
	defer os.Unsetenv("DEEPSEEKCODE_BASE_URL")
	os.Unsetenv("DEEPSEEKCODE_PROXY")

	c := llm.NewClientWithEnv()
	if c.BaseURL != "https://mirror.example.com" {
		t.Errorf("BaseURL = %q; want https://mirror.example.com", c.BaseURL)
	}
}

// TestNewClientWithEnv_DefaultBaseURL verifies the default API endpoint.
func TestNewClientWithEnv_DefaultBaseURL(t *testing.T) {
	os.Setenv("DEEPSEEKCODE_API_KEY", "test-key")
	defer os.Unsetenv("DEEPSEEKCODE_API_KEY")
	os.Unsetenv("DEEPSEEKCODE_BASE_URL")
	os.Unsetenv("DEEPSEEKCODE_PROXY")

	c := llm.NewClientWithEnv()
	if c.BaseURL != "https://api.deepseek.com" {
		t.Errorf("default BaseURL = %q; want https://api.deepseek.com", c.BaseURL)
	}
}

// TestMirrorRetry_FailoverToSecond verifies that StreamWithMirrors falls over
// to the second mirror when the first returns a 5xx.
func TestMirrorRetry_FailoverToSecond(t *testing.T) {
	callCount := 0
	// First server: always 503.
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer bad.Close()

	// Second server: return a minimal SSE done stream.
	good := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		// Write a minimal finish event so the SSE reader terminates cleanly.
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer good.Close()

	mirrors := []string{bad.URL, good.URL}
	c := llm.NewClientWithMirrors("test-key", mirrors)

	// A real Stream call would parse SSE; here we just confirm no error after
	// failover by checking that StreamWithMirrors eventually succeeds.
	ctx := t.Context()
	ch, err := c.StreamWithMirrors(ctx, llm.Request{
		Model:    "deepseek-chat",
		Messages: []llm.Message{{Role: "user", Blocks: []llm.ContentBlock{llm.TextBlock{Text: "hi"}}}},
	}, mirrors)
	if err != nil {
		t.Fatalf("StreamWithMirrors returned error: %v", err)
	}
	// Drain the channel.
	for range ch {
	}
	if callCount == 0 {
		t.Error("bad server was never called; mirrors not tried in order")
	}
	_ = url.Parse // keep import
}
