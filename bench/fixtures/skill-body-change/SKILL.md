---
name: gozer
description: Tiny demo CLI used by the deepseekcode skill-body-change benchmark.
---

# gozer

`gozer` is a small command-line tool for inspecting Go source files.
The bench harness uses it as a fixture to verify that the agent can
locate, read, and extend a `SKILL.md` document while preserving its
existing style.

## Synopsis

```sh
gozer <subcommand> [flags] [args]
```

## Subcommands

### `add`

Add two integers and print the result.

```sh
gozer add 2 3      # → 5
gozer add 10 -4    # → 6
```

Flags:

- `--zero-ok` Treat a zero operand as valid input (default: error).

### `mul`

Multiply two integers and print the result.

```sh
gozer mul 6 7      # → 42
```

Flags:

- `--saturate` Clamp overflow to `math.MaxInt32` instead of wrapping.

## Style Notes

- Each subcommand gets its own `###` heading.
- Synopsis snippets use the fenced `sh` code block.
- Flags are listed as a `-` bullet with `name` ` ` description.

Follow these conventions when adding new subcommands so the file stays
machine-greppable for tooling that scans `SKILL.md` for capabilities.
