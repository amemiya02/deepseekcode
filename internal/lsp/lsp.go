// Package lsp implements a lightweight LSP (Language Server Protocol) client.
//
// It speaks JSON-RPC 2.0 over stdio with Content-Length header framing
// (RFC-style, same as LSP spec). The transport layer handles the framing;
// callers see a request/response API similar to internal/mcp but adapted
// for LSP's framing protocol.
//
// Multiple language servers are managed through a Registry, one per
// detected language. Language detection is file-driven: go.mod → gopls,
// Cargo.toml → rust-analyzer, tsconfig.json → typescript-language-server,
// pyproject.toml / requirements.txt → pylsp.
package lsp
