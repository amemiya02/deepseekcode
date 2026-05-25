# LSP Client

deepseekcode integrates with Language Server Protocol (LSP) servers to provide IDE-like code intelligence
from the terminal.

## Supported Languages

| Language | Server | Detection File |
|----------|--------|---------------|
| Go | gopls | go.mod |
| Rust | rust-analyzer | Cargo.toml |
| TypeScript/JavaScript | typescript-language-server | tsconfig.json |
| Python | pylsp | pyproject.toml or requirements.txt |

## How It Works

1. On startup, deepseekcode scans the project directory for language detection files.
2. For each detected language, it spawns the corresponding LSP server via stdio.
3. The `lsp` tool is registered and can be called by the model or via slash commands.

## The `lsp` Tool

Parameters:
- `action` (required): One of `hover`, `definition`, `references`, `diagnostics`
- `file` (required): File path to query
- `line` (optional): 1-indexed line number (for hover, definition, references)
- `character` (optional): 1-indexed character offset (for hover, definition, references)

### Actions

**hover** — Show type information and documentation at a position.
**definition** — Find where the symbol at a position is defined.
**references** — Find all references to the symbol at a position.
**diagnostics** — Show compiler/linter errors and warnings for a file.

## Requirements

- The LSP server binary must be installed and in PATH.
- Install with:
  - `go install golang.org/x/tools/gopls@latest`
  - `rustup component add rust-analyzer`
  - `npm install -g typescript-language-server typescript`
  - `pip install python-lsp-server`

## Doctor Check

Run `dsc doctor` to see which LSP servers are detected for the current project:

```
deepseekcode doctor
──────────────────────────────────────────────────
  ✓ lsp                 available: gopls
```

If no language servers are detected, the check shows:

```
  ✓ lsp                 no language servers detected for this project
```
