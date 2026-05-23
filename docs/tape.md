# Reasoning Tape (`/tape`)

The Reasoning Tape is `deepseekcode`'s headline feature. It turns
DeepSeek's `reasoning_content` channel into a first-class, browsable
timeline of the model's thinking.

## Inline view (default)

Every reasoning span the model produces is rendered into the scrollback
as a folded line:

```
▸ thinking (3.1s · ~450 tok)
```

Keys (when the input is empty):

- `r` — toggle the most recent reasoning block
- `R` — toggle every reasoning block to the same state

Expanded blocks render the raw `reasoning_content` in dimmed italic so
they don't compete visually with the assistant's final answer.

## Fullscreen view (`/tape`)

Type `/tape` (or hit the configured hotkey) to open a scrubbable
timeline of the entire session:

```
/tape  23 entries · cursor 5/23
▶ ◇ flash  reasoning  3.1s  ~450 tok
  ◇ flash  tool_call  read_file
  ◆ pro    validation  1.2s  approved
  ◇ flash  tool_call  edit_file
  ◇ flash  reasoning  collapsed
  ...
  j/k move · ⏎ select · esc back · q quit
```

The glyph attributes each entry to its model:

- `◇` flash
- `◆` pro

Keys:

- `j` / `k` — step forward / back
- `⏎` — expand the cursor entry (v0.1 returns to chat; deeper inline
  expansion lands in v0.2)
- `esc` / `q` — back to chat

## Why this exists

Other agents throw `reasoning_content` away or dump it raw. The Tape
treats it as a structured signal you can rewind, fold, and learn from.
For DeepSeek specifically, where `reasoning_content` is a separate API
field, this is essentially free differentiation — every interaction
becomes inspectable without the model paying extra tokens for the
"explain your reasoning" prompt.

## Coming in v0.2

- **Branching** (`b` from the Tape) — fork a child session at any prior
  step via SQLite parent_id + branch_point. Architecture is already in
  place (`internal/session/branch.go`); only the keybinding wiring
  remains.
- **Sharing** — `dsc tape export <sessionID> > tape.html` for embedding
  reasoning timelines in PRs, blog posts, or bug reports.
