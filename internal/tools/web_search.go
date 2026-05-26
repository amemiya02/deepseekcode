package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/net/html"
)

// SearchHit represents a single search result.
type SearchHit struct {
	Title   string
	URL     string
	Snippet string
}

// WebSearchProvider is the interface for search backends.
type WebSearchProvider interface {
	Name() string
	Search(ctx context.Context, query string, limit int) ([]SearchHit, error)
}

// WebSearchTool searches the web using the configured provider.
type WebSearchTool struct {
	p WebSearchProvider
}

// NewWebSearchTool creates a WebSearchTool with the given provider.
func NewWebSearchTool(p WebSearchProvider) *WebSearchTool {
	return &WebSearchTool{p: p}
}

func (*WebSearchTool) Name() string { return "web_search" }

func (*WebSearchTool) Description() string {
	return "Search the web and return a list of results. " +
		"Each result includes title, URL, and snippet. " +
		"Use for finding information, documentation, or answers to questions. " +
		"Default limit is 10, max 25 results."
}

func (*WebSearchTool) Parameters() json.RawMessage {
	return MustParams(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"q": map[string]any{
				"type":        "string",
				"description": "The search query.",
			},
			"limit": map[string]any{
				"type":        "integer",
				"description": "Maximum number of results (default 10, max 25).",
			},
		},
		"required": []string{"q"},
	})
}

func (*WebSearchTool) IsReadOnly() bool { return true }

func (t *WebSearchTool) Execute(ctx context.Context, args json.RawMessage) (Result, error) {
	var p struct {
		Query string `json:"q"`
		Limit int    `json:"limit"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return Errf("web_search: invalid args: %v", err), nil
	}
	if p.Query == "" {
		return Errf("web_search: query (q) is required"), nil
	}
	if p.Limit <= 0 {
		p.Limit = 10
	}
	if p.Limit > 25 {
		p.Limit = 25
	}

	if t.p == nil {
		return Errf("web_search: no provider configured"), nil
	}

	hits, err := t.p.Search(ctx, p.Query, p.Limit)
	if err != nil {
		return Errf("web_search: %v", err), nil
	}

	if len(hits) == 0 {
		return Result{Content: "(no results — provider may be rate-limited; consider searxng_url)"}, nil
	}

	// Render results
	var b strings.Builder
	for i, hit := range hits {
		if i > 0 {
			b.WriteString("\n\n")
		}
		fmt.Fprintf(&b, "%d. %s\n   %s\n   %s", i+1, hit.Title, hit.URL, hit.Snippet)
	}

	return Result{Content: b.String()}, nil
}

// DuckDuckGoHTML is a search provider that scrapes DuckDuckGo HTML results.
type DuckDuckGoHTML struct {
	HTTPClient *http.Client
}

// NewDuckDuckGoHTML creates a DuckDuckGo HTML scraper provider.
func NewDuckDuckGoHTML() *DuckDuckGoHTML {
	return &DuckDuckGoHTML{
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (*DuckDuckGoHTML) Name() string { return "duckduckgo" }

func (d *DuckDuckGoHTML) Search(ctx context.Context, query string, limit int) ([]SearchHit, error) {
	// POST to DuckDuckGo HTML endpoint
	formData := url.Values{}
	formData.Set("q", query)

	req, err := http.NewRequestWithContext(ctx, "POST", "https://html.duckduckgo.com/html/", strings.NewReader(formData.Encode()))
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; dsc/1.0; +web_search)")

	resp, err := d.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	// Parse HTML and extract results
	doc, err := html.Parse(strings.NewReader(string(body)))
	if err != nil {
		return nil, fmt.Errorf("parsing HTML: %w (first 500 bytes: %s)", err, truncateString(string(body), 500))
	}

	var hits []SearchHit
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "a" {
			// Check if this is a result link
			var class string
			for _, attr := range n.Attr {
				if attr.Key == "class" {
					class = attr.Val
					break
				}
			}
			if strings.Contains(class, "result__a") {
				// Extract title and URL
				var title strings.Builder
				for c := n.FirstChild; c != nil; c = c.NextSibling {
					if c.Type == html.TextNode {
						title.WriteString(c.Data)
					}
				}

				var rawURL string
				for _, attr := range n.Attr {
					if attr.Key == "href" {
						rawURL = attr.Val
						break
					}
				}

				// Extract snippet from sibling
				snippet := extractDDGSnippet(n.Parent)

				// Decode DDG redirect URL
				finalURL := decodeDDGURL(rawURL)

				if title.Len() > 0 && finalURL != "" {
					hits = append(hits, SearchHit{
						Title:   strings.TrimSpace(title.String()),
						URL:     finalURL,
						Snippet: snippet,
					})
					if len(hits) >= limit {
						return
					}
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
			if len(hits) >= limit {
				return
			}
		}
	}
	walk(doc)

	return hits, nil
}

// extractDDGSnippet extracts the snippet text from a DDG result container.
func extractDDGSnippet(n *html.Node) string {
	if n == nil {
		return ""
	}
	// Look for result__snippet class in siblings/children
	var walk func(*html.Node) string
	walk = func(n *html.Node) string {
		if n.Type == html.ElementNode && n.Data == "a" {
			var class string
			for _, attr := range n.Attr {
				if attr.Key == "class" {
					class = attr.Val
					break
				}
			}
			if strings.Contains(class, "result__snippet") {
				var text strings.Builder
				var extractText func(*html.Node)
				extractText = func(n *html.Node) {
					if n.Type == html.TextNode {
						text.WriteString(n.Data)
					}
					for c := n.FirstChild; c != nil; c = c.NextSibling {
						extractText(c)
					}
				}
				for c := n.FirstChild; c != nil; c = c.NextSibling {
					extractText(c)
				}
				return strings.TrimSpace(text.String())
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if s := walk(c); s != "" {
				return s
			}
		}
		return ""
	}

	// Walk siblings
	for s := n; s != nil; s = s.NextSibling {
		if snippet := walk(s); snippet != "" {
			return snippet
		}
	}
	// Walk children
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if snippet := walk(c); snippet != "" {
			return snippet
		}
	}
	return ""
}

// decodeDDGURL extracts the actual URL from DuckDuckGo's redirect URL.
func decodeDDGURL(rawURL string) string {
	if rawURL == "" {
		return ""
	}
	// DDG URLs look like /l/?uddg=<encoded-url>
	if !strings.HasPrefix(rawURL, "/l/?") {
		return rawURL
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	uddg := u.Query().Get("uddg")
	if uddg == "" {
		return rawURL
	}
	decoded, err := url.QueryUnescape(uddg)
	if err != nil {
		return uddg
	}
	return decoded
}

// SearXNG is a search provider that uses a SearXNG JSON API.
type SearXNG struct {
	HTTPClient *http.Client
	BaseURL    string
}

// NewSearXNG creates a SearXNG provider with the given base URL.
func NewSearXNG(baseURL string) *SearXNG {
	return &SearXNG{
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		BaseURL: baseURL,
	}
}

func (*SearXNG) Name() string { return "searxng" }

func (s *SearXNG) Search(ctx context.Context, query string, limit int) ([]SearchHit, error) {
	if s.BaseURL == "" {
		return nil, fmt.Errorf("searxng: base_url not configured")
	}

	// Build URL
	u, err := url.Parse(s.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("parsing base_url: %w", err)
	}
	u.Path = "/search"
	q := u.Query()
	q.Set("q", query)
	q.Set("format", "json")
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}

	req.Header.Set("User-Agent", "dsc/1.0 (+web_search)")

	resp, err := s.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	// Parse JSON response
	var searxResp struct {
		Results []struct {
			Title   string `json:"title"`
			URL     string `json:"url"`
			Content string `json:"content"`
		} `json:"results"`
	}
	if err := json.Unmarshal(body, &searxResp); err != nil {
		return nil, fmt.Errorf("parsing JSON: %w", err)
	}

	var hits []SearchHit
	for i, r := range searxResp.Results {
		if i >= limit {
			break
		}
		hits = append(hits, SearchHit{
			Title:   r.Title,
			URL:     r.URL,
			Snippet: r.Content,
		})
	}

	return hits, nil
}

func truncateString(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max]
}
