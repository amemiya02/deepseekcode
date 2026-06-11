# Web Tools

> Looking for the Web SPA (browser UI)? Implementation deep dive (for contributors): [dev/three-surfaces.md](../dev/three-surfaces.md). This page covers the `web_fetch` / `web_search` tools.

dsc provides two web tools: `web_fetch` for retrieving content from URLs, and `web_search` for searching the web.

## 启用

Web tools are enabled by default. To disable them:

```toml
[web]
enabled = false
```

## 切换 SearXNG

For better reliability and control, use a self-hosted SearXNG instance:

```toml
[web]
enabled = true
search_provider = "searxng"
searxng_base_url = "https://searx.example.com"
```

SearXNG provides JSON API access and can aggregate results from multiple search engines.

## web_fetch

Fetches a URL and converts HTML content to markdown.

```json
{"url": "https://example.com", "max_bytes": 1048576, "accept": "text/html"}
```

Parameters:
- `url` (required): The URL to fetch (HTTP/HTTPS only)
- `max_bytes` (optional): Maximum bytes to read (default 1MB)
- `accept` (optional): Accept header to send

The tool:
- Converts HTML to markdown (title, headings, links, paragraphs, code blocks)
- Blocks access to private IP addresses by default
- Returns non-HTML content as-is

### Private IP Access

By default, `web_fetch` blocks access to private IP addresses (RFC1918, loopback, link-local). To allow:

```toml
[web]
allow_private = true
```

## web_search

Searches the web and returns a list of results with title, URL, and snippet.

```json
{"q": "golang pty", "limit": 10}
```

Parameters:
- `q` (required): The search query
- `limit` (optional): Maximum results (default 10, max 25)

### Search Providers

#### DuckDuckGo (default)

No configuration needed:

```toml
[web]
enabled = true
search_provider = "duckduckgo"
```

DuckDuckGo is the default because it requires no API key. Note: it may be rate-limited.

#### SearXNG

For better reliability and control, use a self-hosted SearXNG instance:

```toml
[web]
enabled = true
search_provider = "searxng"
searxng_base_url = "https://searx.example.com"
```

SearXNG provides JSON API access and can aggregate results from multiple search engines.

## Example Usage

### Fetch a documentation page

```json
web_fetch{"url": "https://pkg.go.dev/github.com/creack/pty"}
```

### Search for a library

```json
web_search{"q": "golang terminal ui bubbletea", "limit": 5}
```

### Combine search and fetch

```
1. web_search{"q": "golang pty library"}
2. web_fetch{"url": "https://github.com/creack/pty"}
```

## Security Notes

- `web_fetch` only supports HTTP/HTTPS schemes
- `file://` and `javascript:` URLs are blocked
- Private IP access requires explicit opt-in
- No cookies are persisted between requests
- robots.txt is not honored (agent operates at user direction)
