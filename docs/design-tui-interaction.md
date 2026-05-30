# Design: TUI Interaction Polish

**Status:** proposed (2026-05-30)
**Scope:** input/keyboard/affordance layer of the TUI only. Presentational
beautification is covered separately in [`design-tui-beautify.md`](design-tui-beautify.md).
**Guardrail:** this is TUI-only work. It never touches the wire / prompt-cache
path — `TestCacheStableGolden` and `TestFingerprintTracksWireStaticHead` must
keep passing with no `-update`.

---

## 1. Overview & goals

The look has been beautified; the *interaction* still trails claude-code-class
tools (and our Go reference, `crush`). This doc inventories every interaction
gap and specifies the fixes, prioritized P0 → P2.

The two flagship gaps the user named:

1. **Typing `/` should pop a live, filterable list of slash commands and
   skills** — today `/` shows nothing; you must press Tab, and Tab only cycles
   *custom* commands (built-ins like `/models` aren't even completable).
2. **Up-arrow in the input should recall the previous prompt** (terminal/readline
   history) — today there is no prompt history at all.

…plus a longer tail of "small things that add up": `@`-file mentions, a
fuzzy command palette, filterable pickers, a `?` help overlay, transient
toasts, a "press ^C again to quit" affordance, dynamic placeholder, and prompt
queueing.

### Principles

- **Anchor, don't take over.** A `/` or `@` menu is a small popup pinned just
  above the input — not a full-screen overlay. Full-screen overlays
  (`/tape`, `/models`, `/sessions`) stay full-screen; the new inline menus do not.
- **Readline muscle memory.** Up/Down recall history the way a shell does;
  Ctrl+P opens a palette the way an editor does; `?` shows help; Esc backs out
  one layer at a time. No surprises.
- **One key, one owner.** Reasoning folds keep `ctrl+r` / `ctrl+t`. The palette
  takes `ctrl+p` (currently unbound). The input glyphs stay `›` / `>`. No
  reassigning load-bearing keys.
- **Golden-safe & cache-safe.** New surfaces render only when active, so the
  existing render goldens (which strip ANSI) don't move; the cache goldens are
  untouched by construction.

---

## 2. Current state (what exists today)

| Area | Implementation | File:line |
|---|---|---|
| Slash dispatch | `handleSlash` — `/help /clear /quit /models /tape /sessions /export /undo /compact /reload-skills` + custom commands | `app.go:1012` |
| Built-in command list | hard-coded text dump in `helpText()` (not a structured registry) | `app.go:1179` |
| Tab completion | `cycleCompletion` — cycles **custom commands only**, prefix match, no popup, no descriptions | `app.go:847` |
| Custom commands | `map[string]commands.Command` (`Description`, `Agent`, `Model`, `Subtask`, `Template`) | `app.go:86`, `commands/command.go:9` |
| Skills | `a.agent.Skills *skills.Store`, `.List()`/`.Names()`, `Skill{Name, Description}` — **not user-invocable today** | `skills/store.go:221` |
| Input | `textarea.Model`, 3 rows, prompt `› `, static placeholder | `app.go:44,143` |
| Insert keys | `handleInsertKey` — intercepts `enter`, `ctrl+r/t`, `esc`, `tab`; everything else → textarea | `app.go:800` |
| Up/Down in insert | fall through to textarea (cursor move only) — **no history** | `app.go:842` |
| Overlays | full-screen replacements (`modeTape/Models/Sessions`), no filter input, no fuzzy match | `overlay.go` |
| Help | static text appended to scrollback by `/help` | `app.go:1019` |
| Status messages | appended to scrollback (`AppendInfo/Error`) — **no TTL toasts** | scrollback |
| Quit | `ctrl+c` cancels a run, else quits immediately — **no "press again"** | `app.go:763` |
| View layout | `lipgloss.JoinVertical` of `[body, chrome, divider, status, inputBox, hint]` — no Z-compositor | `app.go:641` |

---

## 3. Gap inventory & roadmap

| # | Gap | Target behavior | Priority |
|---|---|---|---|
| G1 | No `/` menu | Typing `/` pops a filterable list (built-ins + custom cmds + skills) with descriptions; filters as you type; ⏎/Tab accepts | **P0** |
| G2 | No input history | ↑/↓ recall previous/next prompt (cursor-row-aware), draft preserved, persisted across sessions | **P0** |
| G3 | Tab can't complete built-ins | Built-ins join the same completion source as custom cmds (fixed for free by G1) | **P0** |
| G4 | No `@` file mention | Typing `@` pops a file/dir completion popup rooted at cwd; inserts the path | P1 |
| G5 | No command palette | `ctrl+p` opens a centered, fuzzy-filterable palette of all actions | P1 |
| G6 | Pickers can't filter | `/models` & `/sessions` get a filter input + fuzzy match | P1 |
| G7 | Help is a text dump | `?` (Normal mode) / `ctrl+g` opens a structured, scrollable keybinding overlay | P1 |
| G8 | No transient toasts | `AppendInfo`-class notices can render as a 1-line toast above the status bar with a TTL | P1 |
| G9 | Instant quit on ^C | First idle `^C` arms a 2s "press again to quit"; second confirms | P1 |
| G10 | Static placeholder | Placeholder reflects state ("Ready" / "Working…") and rotates hints | P2 |
| G11 | No prompt queueing | Submitting while a run is active queues the prompt; drains on completion | P2 |
| G12 | Large paste = wall of text | Paste over N lines collapses to a `[pasted N lines]` chip expanded on send | P2 |

P0 closes the two named gaps (and G3 falls out of G1). P1 is the "feels like a
real tool" tier. P2 is delight.

---

## 4. Architecture additions

Three new pieces, each a small module owning its own state (matching the
existing sub-module pattern where `App` orchestrates by calling methods).

### 4.1 Command registry (`internal/tui/commands_registry.go`, new)

Replace the hard-coded `helpText()` string and the `availableModels()`-style
scattering with a single structured source of truth:

```go
type slashCmd struct {
    Name    string // "models" (no leading slash)
    Aliases []string
    Summary string // one line, shown in the menu + help
    Kind    cmdKind // builtinCmd | customCmd | skillCmd
}
```

- `builtinCommands()` returns the static list (mirrors today's `helpText`).
- `App.allCommands()` merges builtins + `customCmds` (with their `Description`)
  + skills (`a.agent.Skills.List()`, `Name`+`Description`), deduped by name,
  sorted. This one function feeds **both** the `/` menu (G1) and the `?` help
  overlay (G7), so they can never drift.

**Skill invocation decision (sub-decision of G1):** skills aren't directly
user-invocable today (the agent reads them via `skill_read`). Recommended:
selecting a skill row inserts the literal `/<skill-name>` line, and `handleSlash`
gains a branch that, for a known skill, submits a short user-visible directive
(`use the "<name>" skill for this: `) leaving the cursor for the user to finish
the sentence — i.e. a guided prompt, not a hidden behavior change. This keeps
the agent's skill mechanism intact and makes skills *discoverable* from `/`,
which is the user's actual ask. (Alternative considered: expand the full skill
body into the prompt — rejected, it's verbose and duplicates what the agent
already pulls in on demand.)

### 4.2 Completions popup (`internal/tui/completions.go`, new)

A reusable, self-contained popup list that the input owns. Not a full-screen
overlay — it renders as a bordered panel and is **spliced into the `View()`
`parts` slice directly above `inputBox`** (see §4.4). Models crush's
`internal/ui/completions` but adapted to our `JoinVertical` layout instead of an
Ultraviolet Z-compositor.

```go
type completions struct {
    active   bool
    trigger  rune     // '/' or '@'
    query    string   // text typed after the trigger
    items    []complItem // full candidate set for this trigger
    filtered []int    // indices into items, after fuzzy filter
    cursor   int      // index into filtered
    anchorX  int      // cursor column where the trigger was typed
}

type complItem struct {
    insert  string // text inserted on accept ("/models", "@path/to/file")
    label   string // left column
    detail  string // right column (dimmed) — the description
}
```

Behavior:
- **Open** when `handleInsertKey` sees the trigger char typed at a word
  boundary (start of input, or preceded by whitespace). Capture `anchorX`.
- **Filter** on every subsequent keystroke: recompute `query` from the input
  text after the trigger; run the fuzzy filter (§4.5); reset `cursor` to 0.
- **Navigate** with ↑/↓ (and `ctrl+p`/`ctrl+n`); wraps. ↑/↓ are *stolen* from
  history/textarea while the popup is open (precedence in §7).
- **Accept** with ⏎ or Tab: replace from `anchorX` to cursor with
  `items[...].insert`, close popup. (⏎ accepts the completion; it does **not**
  also submit — a second ⏎ submits. This matches crush and avoids fat-finger
  sends.)
- **Dismiss** with Esc (closes popup only; does not leave Insert mode — see §7),
  or automatically when the query no longer matches anything, or when the
  trigger char is deleted.

Max 10 visible rows; scrolls internally if more (reuse `scrollbar.go`). Width =
`clamp(longest row, 24, width-4)`. Rendered with `t.Panel(TierRaised)` so it
reads as a card, consistent with the beautify ladder.

### 4.3 Prompt history (`internal/tui/history.go`, new)

```go
type promptHistory struct {
    entries []string // oldest → newest
    idx     int      // -1 = editing live draft; 0..len-1 = browsing
    draft   string   // live text saved when browsing begins
}
```

- `Prev()` / `Next()` mirror crush's `historyPrev/historyNext`: on first `Prev`
  save the live `draft`, walk toward older entries; `Next` walks back toward
  newer and finally restores `draft` at `idx == -1`. Consecutive duplicates and
  empty strings are not stored. Cap at 500 entries.
- **Seeding & persistence (decision):**
  - *Primary, cross-session:* a per-project ring file at
    `~/.deepseek/history/<cwd-hash>.txt` (one prompt per line, newest last,
    truncated to the cap). Loaded on startup, appended on every submit. This
    gives shell-like recall even when starting a fresh session in the same repo,
    and works in **ephemeral** mode (no SQLite).
  - *Resume bonus:* when a session is resumed, also fold in that session's prior
    user messages via `session.Replay`/`LoadMessages` (role `user`,
    `Content`), deduped against the file. The session DB is the source of truth
    for *that* conversation; the file is the source for *recall across*
    conversations.
  - Wire the file path + loader through `Config` (like `Commands`) so
    `cmd/dsc/main.go` owns the path construction and the TUI stays
    filesystem-light and testable.

### 4.4 Rendering the popup in `View()` (no Z-compositor needed)

Our `View()` builds `parts := []string{body, chrome, divider, status, inputBox, hint}`
and `JoinVertical`s them (`app.go:641`). The input is pinned at the bottom, so a
popup that should *float above the input* is visually identical to one *spliced
directly above `inputBox`* — both grow upward from the prompt. So:

```go
parts := []string{body, chrome, divider, status}
if pop := a.completions.View(a.theme, a.width); pop != "" {
    parts = append(parts, pop)
}
parts = append(parts, inputBox, hint)
```

The viewport `body` is laid out to a height that already reserves the input
rows; for the frames where the popup is open we shrink the body by the popup's
line count in `layout()` so nothing overflows the screen. This avoids pulling in
a compositing layer and stays within the proven `JoinVertical` model.

> **Alternative considered:** `lipgloss/v2` Canvas/Layer compositing for a true
> floating overlay positioned at the cursor column. Deferred — the splice-above
> approach is simpler, robust against the ANSI-reset bleed we already fought in
> ADR-0002, and the visual difference at the bottom of the screen is nil. Revisit
> only if we want mid-screen popups.

### 4.5 Fuzzy filter (`internal/tui/fuzzy.go`, new, ~40 LOC)

A small subsequence matcher with priority tiers (mirrors crush's
`tierExactName / tierPrefixName / tierPathSegment / tierFallback`): exact > prefix
> word-boundary subsequence > anywhere subsequence; ties broken by shorter
candidate then lexicographic. Returns matched-rune indices so the menu can bold
the matched characters. No dependency — the project forbids gratuitous deps and
this is trivially testable in-package.

---

## 5. P0 designs

### 5.1 G1 — the `/` command menu

**Trigger.** In `handleInsertKey`, when the input becomes exactly `/` (typed at
the start), open `completions` with `trigger='/'`, populated from
`App.allCommands()` rendered as `complItem{insert:"/"+name, label:"/"+name,
detail:summary}`. Skills appear with a dimmed `skill` tag in `label`.

**Filter.** As the user types `/mod`, `query="mod"`, fuzzy-filtered live. Empty
query shows the full list (the discovery case the user asked for).

**Accept.** ⏎/Tab inserts the full `/name` and closes the popup; the user then
adds args and presses ⏎ to run. For a no-arg command (e.g. `/clear`) the user
just presses ⏎ twice.

**Replaces** `cycleCompletion`/`clearCompletion`/`completionHint` (delete them;
the popup supersedes Tab-cycling). G3 is closed automatically because builtins
are now in the candidate set.

**Edge cases.** `/` mid-word (e.g. a path `a/b`) does **not** trigger — only at
a word boundary. Deleting back past the `/` closes the popup. Esc closes the
popup without leaving Insert mode.

**Tests.** `completions_test.go`: open-on-slash; live filter narrows; ⏎ inserts
not submits; Esc closes & stays in Insert; builtins+custom+skills all present
and deduped; mid-word `/` is inert.

### 5.2 G2 — input history (↑/↓ recall)

**Keys (Insert mode).**
- `↑`: if popup open → popup up. Else if textarea cursor is on the **first row**
  → `history.Prev()` and load it. Else → textarea moves the cursor up (multiline
  editing preserved).
- `↓`: symmetric — popup down; else if cursor on **last row** → `history.Next()`;
  else textarea cursor down.

This is the readline/crush rule: history only kicks in at the top/bottom edge,
so editing a multiline draft still works.

**Lifecycle.** On submit (`handleInsertKey` enter branch, both the prompt and
slash paths): `history.Add(text)`; append to the ring file; reset `idx=-1`.
Typing any character while browsing commits the current entry as the new draft
(`idx=-1`) so further edits don't get clobbered by the next ↑.

**Tests.** `history_test.go`: Prev/Next walk + draft save/restore (port crush's
semantics); dedupe consecutive; cap; cursor-row gating (↑ on row 2 of a 3-line
draft moves the cursor, doesn't recall); file round-trip; resume-seed merge.

---

## 6. P1 designs

### 6.1 G4 — `@` file mention

Same `completions` machinery, `trigger='@'`. Candidates = a depth-limited,
gitignore-aware walk of `cwd` (lazy, cached per session; cap ~2000 entries),
ranked by the path-segment tier of §4.5. Accept inserts the repo-relative path.
Purpose: let the user *name* files in a prompt cheaply (the agent still reads
them via tools). No image/attachment handling — DeepSeek is text-only, so we
explicitly skip crush's filepicker/Kitty-graphics path.

### 6.2 G5 — command palette (`ctrl+p`)

A centered modal (reuse `wrapPane` + a filter input) listing **every** action:
all slash commands, plus verbs that have no slash today (toggle thinking, switch
theme, open help, yank, undo). Fuzzy filter input at top; ↑/↓ + ⏎; Esc closes.
This is the "I don't remember the command" entry point; `/` is the "I'm already
typing a command" entry point. New overlay mode `modePalette` in `overlay.go`.

### 6.3 G6 — filterable pickers

Add a one-line filter input to `renderModelsPicker` / `renderSessionsPicker` and
fuzzy-filter the rows live. Mechanically the palette and the pickers share a
`filterableList` helper (filter string + fuzzy + cursor clamp) so there's one
implementation. Low rows today (4 models) but `/sessions` can be long.

### 6.4 G7 — `?` help overlay

Replace the `/help` scrollback dump with a structured, scrollable overlay
(`modeHelp`) built from the §4.1 registry + a static keybinding table. `?` opens
it in Normal mode; `/help` and `ctrl+g` also open it. Esc/`q` closes. Keeps the
keybinding doc and the command list in one generated place.

### 6.5 G8 — transient toasts

A `toast` field on `App` (`text`, `kind`, `expiry`) rendered as a single styled
line between `status` and `inputBox`, auto-cleared by a `tea.Tick` (TTL ~4s,
mirrors crush's `DefaultStatusTTL`). Route ephemeral notices (model switched,
yanked N bytes, reload result, unknown command) to the toast instead of
polluting scrollback; keep durable content (errors worth scrolling back to) in
scrollback. Color by kind via the existing `Theme.Badge` tokens.

### 6.6 G9 — `^C` double-tap to quit

When idle, first `^C` sets `quitArmed=true`, shows a toast "press ^C again to
quit", and starts a 2s `tea.Tick`; a second `^C` within the window quits; the
tick disarms. When a run **is** active, `^C` still cancels immediately (current
behavior, unchanged) — the double-tap only guards the idle quit so a stray ^C
doesn't drop the session.

---

## 7. Focus & Esc precedence

A single ordered chain resolves every "back out" key so layers peel
predictably. `handleKey` consults them top-down:

1. **Full-screen overlay open** (`/tape /models /sessions` + new palette/help) →
   its own key handler; Esc closes it.
2. **Permission / question modal active** → its handler (unchanged).
3. **Completions popup open** → ↑/↓/⏎/Tab drive the popup; Esc closes the popup
   *only* (stay in Insert).
4. **Insert mode** → history-aware ↑/↓ (§5.2); Esc → Normal mode (unchanged).
5. **Normal mode** → scroll/yank/etc (unchanged).

The popup is deliberately *below* the modals in precedence and *above* history,
so Esc never skips a layer and ↑ never recalls history while the menu is up.

---

## 8. Final keybinding map

| Key | Insert | Normal | Popup open | Overlay/Modal |
|---|---|---|---|---|
| `↑` / `↓` | history (edge) / cursor | scroll | **nav popup** | nav list |
| `⏎` | submit (or accept popup, no submit) | — | **accept** | select |
| `Tab` | accept popup | — | accept | cycle category (palette) |
| `Esc` | → Normal | (no-op) | **close popup** | close overlay |
| `/` | open `/` menu | — | — | — |
| `@` | open `@` menu | — | — | — |
| `ctrl+p` | open palette | open palette | — | — |
| `?` | (literal `?`) | **help overlay** | — | — |
| `ctrl+g` | help overlay | help overlay | — | — |
| `ctrl+r` / `ctrl+t` | reasoning fold (unchanged) | reasoning fold | — | — |
| `ctrl+c` | cancel / arm-quit | cancel / arm-quit | close popup→then cancel | cancel |

No existing binding is reassigned. `ctrl+p`, `?`(Normal), `ctrl+g`, `@` are all
currently free. `?` stays a literal character in Insert mode (only Normal-mode
`?` opens help) so typing prose is unaffected.

---

## 9. Files touched

| File | Change |
|---|---|
| `internal/tui/commands_registry.go` | **new** — structured builtin list + `allCommands()` merge (builtins + custom + skills) |
| `internal/tui/completions.go` | **new** — `/` and `@` popup component |
| `internal/tui/history.go` | **new** — prompt history ring + file persistence |
| `internal/tui/fuzzy.go` | **new** — tiered fuzzy matcher with match indices |
| `internal/tui/app.go` | `handleInsertKey` (trigger detection, history ↑/↓, accept), `View()` splice popup + toast, `handleSlash` (skill branch, registry), delete `cycleCompletion/clearCompletion/completionHint`, `^C` double-tap, `?`/`ctrl+p`/`ctrl+g`, `layout()` body-height reserve |
| `internal/tui/overlay.go` | `modePalette`, `modeHelp`; filter input on models/sessions; shared `filterableList` |
| `internal/tui/app.go` `Config` + `cmd/dsc/main.go` | thread history file path + session-seed loader |
| `internal/tui/*_test.go` | new test files per §5/§6; update `keyflow_test.go` for the new precedence chain |
| `docs/design.md` | cross-link this doc once landed |
| `CONTEXT.md` | add **Completions popup**, **Prompt history** to the glossary if they earn domain status |

`README.md` / `README.zh-CN.md` get a short "keys & commands" refresh **together**
(bilingual rule) once P0 lands.

---

## 10. Testing & guardrails

- **Cache goldens untouched.** No file under `internal/llm` is edited; verify
  `TestCacheStableGolden` + `TestFingerprintTracksWireStaticHead` pass with no
  `-update` after every increment.
- **Render goldens.** New surfaces (popup, toast, palette) render only when
  active and aren't in the `TestRenderGoldenPerWidth` scenarios, so existing
  goldens don't move. Add dedicated golden cases for the popup + palette at the
  standard widths.
- **Pure-logic tests carry the weight.** `fuzzy`, `history`, `allCommands`, and
  the trigger/precedence logic are all pure functions or small state machines —
  table-test them directly (the `keyflow_test.go` / `qa_harness_test.go` pattern
  already in the package).
- **`-race ./internal/tui/`** clean (the history file writer must not race the
  UI goroutine — do file I/O in a `tea.Cmd`, never inline in `Update`).
- Format/build/vet gate as usual.

---

## 11. Non-goals & guardrails

- **No image/attachment/Kitty-graphics path** — DeepSeek is text-only; crush's
  filepicker preview is explicitly out of scope.
- **No external dependency** — fuzzy match and history are hand-rolled (~80 LOC
  total), consistent with the "no external LLM SDK / minimal deps" house rule.
- **No mid-screen floating compositor** in v1 — popups splice above the input
  (§4.4). Revisit only if mid-screen popups are needed.
- **Don't reassign** `ctrl+r`/`ctrl+t` (reasoning folds) or change the `›`/`>`
  prompt glyphs.
- **Don't touch** the agent loop, wire format, or session schema — this is
  presentation/input only. History persistence is a *new* sidecar file, not a
  schema change.

---

## 12. Suggested landing order

1. **G1 + G3** (`/` menu, builtins completable) — the headline feature; ships the
   `completions` + `fuzzy` + `commands_registry` modules.
2. **G2** (history) — small, high-value, reuses nothing but the new key plumbing.
3. **G7 + G8** (help overlay, toasts) — cheap once the registry exists.
4. **G5 + G6** (palette, filterable pickers) — share `filterableList`.
5. **G4** (`@` mention) — reuses the popup; needs the file walk.
6. **G9 / G10 / G11 / G12** — polish, independent, land opportunistically.
