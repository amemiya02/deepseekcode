// Package llm — transport.go provides a proxy-aware http.Transport constructor
// and environment-driven Client constructors for China-friendly deployments.
//
// Priority chain for proxy selection:
//  1. DEEPSEEKCODE_PROXY  (explicit per-tool override)
//  2. HTTPS_PROXY / HTTP_PROXY / NO_PROXY  (standard Go / curl conventions)
//
// Priority chain for base URL:
//  1. DEEPSEEKCODE_BASE_URL
//  2. Default: https://api.deepseek.com
//
// No new external dependencies. Uses only net/http, net/url, os.
package llm

import (
	"context"
	"crypto/tls"
	"golang.org/x/net/http/httpproxy"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const defaultBaseURL = "https://api.deepseek.com"

// ProxyTransport returns an *http.Transport configured to:
//   - Use DEEPSEEKCODE_PROXY if set (highest priority).
//   - Fall back to http.ProxyFromEnvironment (HTTPS_PROXY / HTTP_PROXY / NO_PROXY).
//   - Apply China-latency-appropriate dial and TLS timeouts.
func ProxyTransport() *http.Transport {
	proxyFunc := proxyFromEnv()
	return &http.Transport{
		Proxy: proxyFunc,
		DialContext: (&net.Dialer{
			Timeout:   15 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 0, // streaming: governed by Client timeouts
		ExpectContinueTimeout: 1 * time.Second,
		MaxIdleConns:          10,
		IdleConnTimeout:       90 * time.Second,
		TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS12},
	}
}

// proxyFromEnv returns a proxy function that checks DEEPSEEKCODE_PROXY first,
// then delegates to the standard env-based proxy resolution. The
// httpproxy.Config for the standard HTTP(S)_PROXY / NO_PROXY vars is captured
// once at construction time (matching http.ProxyFromEnvironment). The
// DEEPSEEKCODE_PROXY override, however, is read per request so it keeps the
// late-binding behaviour callers (and tests) rely on: setting that one var
// after the transport is built still takes effect without rebuilding it.
func proxyFromEnv() func(*http.Request) (*url.URL, error) {
	cfg := httpproxy.FromEnvironment()
	proxyFn := cfg.ProxyFunc()
	return func(req *http.Request) (*url.URL, error) {
		if override := os.Getenv("DEEPSEEKCODE_PROXY"); override != "" {
			return url.Parse(override)
		}
		return proxyFn(req.URL)
	}
}

// NewClientWithEnv constructs a Client whose transport honours proxy env vars
// and whose base URL is read from DEEPSEEKCODE_BASE_URL (fallback:
// https://api.deepseek.com). API key is read from DEEPSEEKCODE_API_KEY.
func NewClientWithEnv() *Client {
	apiKey := os.Getenv("DEEPSEEKCODE_API_KEY")
	baseURL := os.Getenv("DEEPSEEKCODE_BASE_URL")
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	c := NewClient(apiKey, baseURL)
	c.HTTPClient = &http.Client{
		Transport: ProxyTransport(),
		Timeout:   0, // streaming: no global timeout
	}
	return c
}

// WithProxyTransport installs a ProxyTransport() on the client's HTTPClient
// in-place, so proxy env vars (DEEPSEEKCODE_PROXY / HTTPS_PROXY / HTTP_PROXY)
// are honoured for every request made by this client. It returns the receiver
// so callers can chain: prov.BaseClient().WithProxyTransport().
func (c *Client) WithProxyTransport() *Client {
	c.HTTPClient = &http.Client{
		Transport: ProxyTransport(),
		Timeout:   0, // streaming: no global timeout
	}
	return c
}

// NewClientWithMirrors constructs a Client that will try each mirror URL in
// order when StreamWithMirrors is called. The first mirror is also the
// primary BaseURL for regular Stream calls. All mirrors are stored on the
// Client so that StreamWithMirrors can use c.Mirrors when called with nil.
func NewClientWithMirrors(apiKey string, mirrors []string) *Client {
	base := defaultBaseURL
	if len(mirrors) > 0 {
		base = mirrors[0]
	}
	c := NewClient(apiKey, base)
	c.Mirrors = mirrors
	c.HTTPClient = &http.Client{
		Transport: ProxyTransport(),
		Timeout:   0,
	}
	return c
}

// StreamWithMirrors attempts Stream against each mirror in turn, stopping at
// the first success. It returns the first non-transient error or the last
// error if all mirrors fail. If mirrors is nil or empty, c.Mirrors is used.
// c.BaseURL is never mutated; each attempt constructs its endpoint inline.
func (c *Client) StreamWithMirrors(ctx context.Context, req Request, mirrors []string) (<-chan Event, error) {
	if len(mirrors) == 0 {
		mirrors = c.Mirrors
	}
	if len(mirrors) == 0 {
		return c.Stream(ctx, req)
	}

	req.Stream = true
	if req.StreamOptions == nil {
		req.StreamOptions = &StreamOptions{IncludeUsage: true}
	}
	body, err := req.MarshalCacheStable()
	if err != nil {
		return nil, err
	}

	var lastErr error
	for _, mirror := range mirrors {
		endpoint := strings.TrimRight(mirror, "/") + "/v1/chat/completions"
		ch, err := c.doStreamURL(ctx, body, endpoint)
		if err == nil {
			return ch, nil
		}
		lastErr = err
		// Continue to the next mirror only for transient errors (HTTP 429/5xx
		// or network-level failures such as connection refused / DNS error).
		if !IsTransient(err) && !isNetworkError(err) {
			return nil, err
		}
	}
	return nil, lastErr
}
