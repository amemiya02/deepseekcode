# Distribution-surface strategy

**Status:** Proposed (decision pending)

## Context

`deepseekcode` ships as a single terminal binary by design. A competitive
analysis (`docs/research/deepseek-specialization-vs-reasonix.md` sections 8--9)
found that the most visible gap against the primary competitor is not DeepSeek
specialization depth (where dsc leads) but **product breadth**: the competitor
offers a Tauri desktop app, a web dashboard with REST endpoints, a QQ remote
channel, and an ACP editor protocol -- none of which dsc provides.

The existing `Bus().Subscribe` event fan-out was designed with future daemon
clients in mind, so a lightweight out-of-process consumer is architecturally
feasible without restructuring the agent loop.

The question is whether to respond to the breadth gap, and if so, at what
level of investment.

## Considered options

### Option A: Stay terminal-only

**What it buys:** Zero additional surface to maintain, test, or secure. The
single-binary cross-compile story (pure Go, no CGO) stays clean. All
development effort goes into DeepSeek specialization depth, which is the
proven differentiator.

**Cost:** None beyond the status quo.

**Risk:** The breadth gap widens as the competitor ships more GUI/remote
features. Users who need a visual dashboard or editor integration never
evaluate dsc. The project may be perceived as "niche" despite technical
superiority.

### Option B: Lightweight daemon + editor protocol

**What it buys:** A headless daemon mode that exposes the agent loop over a
local socket or protocol (e.g. ACP-like or LSP-inspired), reusable by editor
plugins, a minimal web UI, or remote clients. The existing `Bus().Subscribe`
fan-out can be extended to stream events to daemon clients without changing the
core loop. The terminal TUI remains the primary interface; the daemon is an
alternate transport.

**Cost:** Moderate. Requires defining a wire protocol, implementing a daemon
process manager, and building at least one reference client (e.g. a Neovim
plugin or minimal web panel). Security surface increases (socket permissions,
authentication). Testing matrix grows.

**Risk:** Protocol design scope creep. Maintaining two interfaces (TUI +
daemon) may split attention. The daemon may become a de facto primary interface
if it gains traction, pulling effort away from terminal UX.

### Option C: Full GUI / desktop application

**What it buys:** Maximum breadth parity with the competitor. A Tauri or
Electron shell wrapping the agent, with visual session history, drag-and-drop
file context, and point-and-click tool approval. Widest potential user base.

**Cost:** High. A full GUI is a separate application with its own release
cycle, platform-specific testing, accessibility requirements, and design
system. The Go core would need a stable FFI or IPC boundary. Likely requires
dedicated frontend engineering.

**Risk:** The project's identity fragments between "terminal agent" and
"desktop IDE." The single-binary distribution story is lost. GUI maintenance
cost may exceed the specialization work that is dsc's actual competitive
advantage.

## Recommendation

**Option B (lightweight daemon + editor protocol)** is the lowest-cost
response to the breadth gap per the competitive analysis. It preserves the
terminal-first identity, leverages existing architecture (`Bus` fan-out), and
opens the door to editor integrations and lightweight web panels without
building a full desktop application. This is a recommendation awaiting the
user's decision.

## Consequences

- **This ADR records the decision framing only.** Implementation of any
  option is explicitly **out of scope** of the current task plan.
- If Option B is accepted later, it will require a follow-up ADR specifying
  the wire protocol, transport, authentication model, and reference client
  scope.
- Option A (no change) is the zero-cost default if no decision is made; the
  terminal binary continues to ship as-is.
- The competitive-analysis reference is
  [`docs/research/deepseek-specialization-vs-reasonix.md`](../research/deepseek-specialization-vs-reasonix.md),
  sections 8 (breadth vs. specialization distinction) and 9 (P2
  recommendation).
