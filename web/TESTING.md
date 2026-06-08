# GUI test strategy

Manual click-through is slow and missed real bugs (chat buttons that rendered
but had no backend; components that pass unit tests but are never mounted). The
suite below catches those *classes* of bug automatically. Each layer targets a
distinct failure mode unit tests are blind to.

## The three failure modes

| # | Failure mode | Example we actually shipped | Caught by |
|---|---|---|---|
| A | **Orphan** — renders, unit-tests pass, never mounted | `ModeTabs`, `RuntimeBanner`, `DiffIsland` | **knip** (L0) |
| B | **No-op wiring** — mounted, but handler is empty / props hardcoded | chat-area buttons `onClick={() => {}}` | **no-op guard** (L1a) |
| C | **Missing backend** — FE calls `/v1/x` that 404s; mock hides it | `/v1/capabilities`, `/v1/runtime` | **contract test** (L2) |

> Why unit tests + Playwright missed all three: unit tests render a component
> in isolation (can't see mounting or behaviour), and the Playwright specs run
> against `src/lib/mockGateway.ts` — the mock always answers, so a missing real
> backend is invisible.

## Layers

### L0 — Reachability (knip)
`npm run knip` lists files/exports not reachable from the app entry. An orphan
component is reported in seconds (what used to take manual diff review).

Config: `knip.json`. **Policy** (`rules`): unused **files** are an `error`
(this is the orphan-component bug class — it blocks `gates`); unused
**exports/types** are `warn` (reported for triage, non-blocking).

Resolved during setup: `RuntimeBanner.tsx` was wired into the App shell (its
backend `/v1/runtime` now exists); `DiffIsland.tsx` is a deliberate spec-name
re-export kept via `ignore`. Remaining **warnings** to clean up when convenient
(all confirmed 0-reference): `fetchCacheReport`, `mcpDescribe`, `loadMonaco`,
`TIER_TO_MODE`/`MODE_TO_TIER` (WIP permission-tier), and types `AskOption`,
`PlanUpdate`, `JobStatus` in `src/lib/api.ts` / `autonomy.ts` / `monaco.tsx`.

### L1a — No-op handler guard
`src/lib/no-op-handlers.test.ts` statically fails the build on any
`onX={() => {}}` / `onX={noop}` handler. Intentional no-ops go in its
`ALLOWLIST` *with a reason* (visible debt, not silent). Prevents mode B.

### L1b — Interaction inventory
`src/lib/interaction-inventory.test.ts` enumerates **every** JSX event-handler
prop across all components and pins it to a committed snapshot
(`src/lib/__snapshots__/interaction-inventory.json`, 58 components today).
Adding a new interactive control fails the test until you run
`npx vitest run -u src/lib/interaction-inventory.test.ts` — the forcing
function that makes a new interaction get noticed and (by convention) get a
behaviour test. This is how "cover every interaction" becomes enforceable.

### L2 — FE↔BE contract
`src/lib/api-contract.test.ts` extracts every `/v1/*` literal the frontend
fetches and every route the Go gateway registers (`mux.HandleFunc`), and
asserts FE ⊆ BE (honouring ServeMux subtree semantics). A button wired to a
non-existent endpoint fails here. Deterministic, mock-proof.

## Commands

```bash
npm run gates    # typecheck + knip + full vitest (contract + L1 guards) — the green gate
npm run knip     # dead-code / reachability report (triage)
npm test         # vitest only
npm run typecheck
```

Backend: `go test ./internal/gateway/` covers the gateway handlers, including
`/v1/runtime` and `/v1/capabilities`.

## What's NOT covered (deferred: L3)

These gates prove *reachability*, *non-no-op wiring*, and *endpoint existence*.
They do **not** prove a button end-to-end against the real backend, because the
Playwright specs use `mockGateway`. The authoritative layer — boot the real Go
gateway, disable the mock, drive Playwright, assert real responses — is the
next investment for critical flows. Add it as a separate, slower CI lane.

## Behaviour-test discipline (mode B, defence in depth)

When adding/reviewing an interactive control, the unit test must **click it and
assert a side effect** — a store mutation, or a mocked `lib/api` function called
with expected args — not just `toBeInTheDocument()`. The no-op guard is the
backstop; the behaviour assertion is the real coverage.
