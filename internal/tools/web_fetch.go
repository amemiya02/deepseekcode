package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/microcosm-cc/bluemonday"
	"golang.org/x/net/html"
)

// WebFetchTool fetches a URL and converts HTML to markdown.
type WebFetchTool struct {
	HTTPClient   *http.Client
	MaxBytes     int64
	AllowPrivate bool
}

// NewWebFetchTool creates a WebFetchTool with default settings.
func NewWebFetchTool(allowPrivate bool) *WebFetchTool {
	return &WebFetchTool{
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		MaxBytes:     1 << 20, // 1MB
		AllowPrivate: allowPrivate,
	}
}

func (*WebFetchTool) Name() string { return "web_fetch" }

func (*WebFetchTool) Description() string {
	return "Fetch a URL and return its content. HTML is converted to markdown. " +
		"Supports HTTP/HTTPS only. Max response size 1MB by default. " +
		"Use for retrieving web pages, API responses, or any HTTP-accessible content."
}

func (*WebFetchTool) Parameters() json.RawMessage {
	return MustParams(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"url": map[string]any{
				"type":        "string",
				"description": "The URL to fetch (HTTP or HTTPS only).",
			},
			"max_bytes": map[string]any{
				"type":        "integer",
				"description": "Maximum bytes to read (default 1048576 = 1MB).",
			},
			"accept": map[string]any{
				"type":        "string",
				"description": "Optional Accept header to send.",
			},
		},
		"required": []string{"url"},
	})
}

func (*WebFetchTool) IsReadOnly() bool { return true }

func (t *WebFetchTool) Execute(ctx context.Context, args json.RawMessage) (Result, error) {
	var p struct {
		URL      string `json:"url"`
		MaxBytes int64  `json:"max_bytes"`
		Accept   string `json:"accept"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return Errf("web_fetch: invalid args: %v", err), nil
	}
	if p.URL == "" {
		return Errf("web_fetch: url is required"), nil
	}
	if p.MaxBytes <= 0 {
		p.MaxBytes = t.MaxBytes
	}

	// Parse and validate URL
	u, err := url.Parse(p.URL)
	if err != nil {
		return Errf("web_fetch: invalid url: %v", err), nil
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return Errf("web_fetch: only http and https schemes are allowed (got %s)", u.Scheme), nil
	}

	// Check for private IP (unless explicitly allowed)
	if !t.AllowPrivate {
		if err := checkPrivateHost(u.Hostname()); err != nil {
			return Errf("web_fetch: %v", err), nil
		}
	}

	// Build request
	req, err := http.NewRequestWithContext(ctx, "GET", p.URL, nil)
	if err != nil {
		return Result{}, fmt.Errorf("web_fetch: building request: %w", err)
	}
	req.Header.Set("User-Agent", "dsc/1.0 (+web_fetch)")
	if p.Accept != "" {
		req.Header.Set("Accept", p.Accept)
	}

	// Create a client with redirect checker that validates private IPs
	client := t.HTTPClient
	if !t.AllowPrivate {
		client = &http.Client{
			Timeout:   t.HTTPClient.Timeout,
			Transport: t.HTTPClient.Transport,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if err := checkPrivateHost(req.URL.Hostname()); err != nil {
					return fmt.Errorf("redirect to private IP blocked: %w", err)
				}
				// Keep default 10-redirect limit
				if len(via) >= 10 {
					return http.ErrUseLastResponse
				}
				return nil
			},
		}
	}

	// Execute request
	resp, err := client.Do(req)
	if err != nil {
		if urlErr, ok := err.(*url.Error); ok {
			return Errf("web_fetch: request failed: %v", urlErr.Err), nil
		}
		return Errf("web_fetch: request failed: %v", err), nil
	}
	defer resp.Body.Close()

	// Read response body (with max bytes limit)
	limited := io.LimitReader(resp.Body, p.MaxBytes+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		return Errf("web_fetch: reading response: %v", err), nil
	}

	truncated := len(body) > int(p.MaxBytes)
	if truncated {
		body = body[:p.MaxBytes]
	}

	// Process content based on content-type
	contentType := resp.Header.Get("Content-Type")
	var content string
	if strings.HasPrefix(contentType, "text/html") {
		content = htmlToMarkdown(body)
	} else {
		content = string(body)
	}

	// Build result
	result := fmt.Sprintf("[status %d] %s\n\n%s", resp.StatusCode, resp.Request.URL.String(), content)
	if truncated {
		result += fmt.Sprintf("\n\n(truncated to %d bytes)", p.MaxBytes)
	}

	return Result{
		Content: result,
		IsError: resp.StatusCode >= 400,
	}, nil
}

// checkPrivateHost returns an error if the hostname resolves to a private IP.
func checkPrivateHost(hostname string) error {
	// Try to resolve the hostname
	ips, err := net.LookupIP(hostname)
	if err != nil {
		// DNS lookup failed - return a dns: error as per task card
		return fmt.Errorf("dns: %w", err)
	}
	for _, ip := range ips {
		if isPrivateIP(ip) {
			return fmt.Errorf("access to private IP %s is blocked (set allow_private=true to allow)", ip)
		}
	}
	return nil
}

// isPrivateIP checks if an IP is private/loopback/link-local.
func isPrivateIP(ip net.IP) bool {
	if ip.IsLoopback() {
		return true
	}
	if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return true
	}
	if ip.IsPrivate() {
		return true
	}
	// Check for IPv4-mapped IPv6 addresses
	if ip4 := ip.To4(); ip4 != nil {
		return ip4.IsPrivate() || ip4.IsLoopback()
	}
	return false
}

// htmlToMarkdown converts HTML to a simple markdown representation.
func htmlToMarkdown(htmlBytes []byte) string {
	// First, extract the title from raw HTML (before sanitization removes head)
	var title string
	if doc, err := html.Parse(strings.NewReader(string(htmlBytes))); err == nil {
		title = extractTitle(doc)
	}

	// Sanitize HTML with bluemonday
	sanitized := bluemonday.UGCPolicy().SanitizeBytes(htmlBytes)

	// Parse sanitized HTML
	doc, err := html.Parse(strings.NewReader(string(sanitized)))
	if err != nil {
		// If parsing fails, return as-is
		return string(sanitized)
	}

	// Walk the DOM and extract text with markdown formatting
	var b strings.Builder

	// Add title first if found
	if title != "" {
		b.WriteString("# ")
		b.WriteString(title)
		b.WriteString("\n\n")
	}

	var walk func(*html.Node, int)
	walk = func(n *html.Node, depth int) {
		switch n.Type {
		case html.TextNode:
			b.WriteString(n.Data)
		case html.ElementNode:
			switch n.Data {
			case "h1", "h2", "h3", "h4", "h5", "h6":
				level := n.Data[1]
				for i := 0; i < int(level-'0'); i++ {
					b.WriteByte('#')
				}
				b.WriteByte(' ')
				for c := n.FirstChild; c != nil; c = c.NextSibling {
					walk(c, depth+1)
				}
				b.WriteString("\n\n")
				return
			case "p":
				for c := n.FirstChild; c != nil; c = c.NextSibling {
					walk(c, depth+1)
				}
				b.WriteString("\n\n")
				return
			case "a":
				// Extract href
				var href string
				for _, attr := range n.Attr {
					if attr.Key == "href" {
						href = attr.Val
						break
					}
				}
				b.WriteByte('[')
				for c := n.FirstChild; c != nil; c = c.NextSibling {
					walk(c, depth+1)
				}
				b.WriteByte(']')
				if href != "" {
					b.WriteString("(" + href + ")")
				}
				return
			case "code":
				b.WriteByte('`')
				for c := n.FirstChild; c != nil; c = c.NextSibling {
					walk(c, depth+1)
				}
				b.WriteByte('`')
				return
			case "pre":
				b.WriteString("```\n")
				for c := n.FirstChild; c != nil; c = c.NextSibling {
					walk(c, depth+1)
				}
				b.WriteString("\n```\n\n")
				return
			case "br":
				b.WriteString("\n")
				return
			case "li":
				b.WriteString("- ")
				for c := n.FirstChild; c != nil; c = c.NextSibling {
					walk(c, depth+1)
				}
				b.WriteByte('\n')
				return
			case "strong", "b":
				b.WriteString("**")
				for c := n.FirstChild; c != nil; c = c.NextSibling {
					walk(c, depth+1)
				}
				b.WriteString("**")
				return
			case "em", "i":
				b.WriteByte('*')
				for c := n.FirstChild; c != nil; c = c.NextSibling {
					walk(c, depth+1)
				}
				b.WriteByte('*')
				return
			}
		}
		// Default: walk children
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c, depth+1)
		}
	}
	walk(doc, 0)

	// Clean up extra whitespace
	result := b.String()
	// Collapse multiple newlines
	for strings.Contains(result, "\n\n\n") {
		result = strings.ReplaceAll(result, "\n\n\n", "\n\n")
	}
	return strings.TrimSpace(result)
}

// extractTitle finds the <title> element content from HTML.
func extractTitle(n *html.Node) string {
	if n.Type == html.ElementNode && n.Data == "title" {
		var text strings.Builder
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if c.Type == html.TextNode {
				text.WriteString(c.Data)
			}
		}
		return strings.TrimSpace(text.String())
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if t := extractTitle(c); t != "" {
			return t
		}
	}
	return ""
}
