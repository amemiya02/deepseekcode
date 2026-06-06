package tui

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/amemiya02/deepseekcode/internal/agent"
	"github.com/amemiya02/deepseekcode/internal/commands"
	"github.com/amemiya02/deepseekcode/internal/i18n"
	"github.com/amemiya02/deepseekcode/internal/llm"
	"github.com/amemiya02/deepseekcode/internal/permissions"
	"github.com/amemiya02/deepseekcode/internal/session"
	"github.com/amemiya02/deepseekcode/internal/skills"
	"github.com/amemiya02/deepseekcode/internal/tools"
	"github.com/amemiya02/deepseekcode/internal/version"
)

// App is the root Bubble Tea Model.
//
// One App per running TUI process. The Agent runs in a goroutine
// owned by the App; a separate pumpEvents goroutine consumes
// agent.Events() and forwards each event as an agentEventMsg via
// App.send (the tea.Program.Send func, captured in Run).
type App struct {
	// Wiring
	agent    *agent.Agent
	model    string
	thinking bool
	cwd      string

	// UI state
	theme            Theme
	themePreviewOrig Theme // theme to restore if the /theme picker is cancelled
	keymap           keymap
	vp               viewport.Model
	input            textarea.Model
	width            int
	height           int
	mode             appMode

	// Sub-modules. Each owns the state it renders/mutates; App
	// orchestrates by calling their methods rather than reaching
	// into fields.
	scrollback *Scrollback     // chat history + streams + selection
	chrome     *Chrome         // live activity band + redraw ticker flag
	overlay    *Overlay        // modal pickers (tape / models / sessions)
	permission *PermissionFlow // modal permission card
	question   *QuestionFlow   // modal question card

	// Pager overlay (modal; takes the body band when non-nil).
	pager *pagerState

	// Per-run cancellation.
	runMu     sync.Mutex
	runCtx    context.Context
	runCancel context.CancelFunc
	running   bool

	// Bottom status bar (model name, cumulative usage, step count).
	status    statusState
	stepTotal int

	// Inline completion popup (the `/` and `@` menus, G1/G3) and the
	// readline prompt history (G2). completions is derived from the input
	// buffer by syncCompletions on every insert-mode keystroke; history is
	// the cross-session recall ring loaded from / appended to historyPath.
	// popupLines is the number of terminal rows the popup currently occupies,
	// which layout() subtracts from the viewport height so the popup never
	// pushes the input off-screen.
	completions completions
	history     *promptHistory
	popupLines  int
	historyPath string

	// `@` file-mention index (G4/§6.1). fileIndex is the cached repo-relative
	// path list backing the @ menu; it is built off the UI goroutine by
	// indexFilesCmd and delivered via fileIndexMsg, so fileIndexReady gates
	// whether the @ popup shows real rows or the single "(indexing files…)"
	// placeholder. Built eagerly at startup from Init.
	fileIndex      []string
	fileIndexReady bool

	// Transient toast (G8/§6.5) and idle ^C double-tap quit guard (G9/§6.6).
	// toastState is the single one-line notice slot rendered between status and
	// the input box; quitArmed is set by the first idle ^C and cleared either by
	// a confirming second ^C (which quits) or the disarm tick. quitSeq pairs the
	// arm with its disarm tick so a re-arm can't be cancelled by a stale tick.
	toastState toastState
	quitArmed  bool
	quitSeq    int

	// P2 polish input affordances.
	//
	// uiRunning is the UI-goroutine's view of "a run is active": set when
	// runStartMsg lands and cleared on agent.EventDone. It is distinct from
	// a.running (flipped inside the agent goroutine, which can lag) so the submit
	// path can decide queue-vs-run synchronously without racing the worker.
	//
	// G10 (dynamic placeholder): placeholderTurn indexes the idle-hint rotation
	// (readyHints); it advances once per completed turn so each fresh idle prompt
	// shows the next deterministic hint — never a clock/random source.
	//
	// G11 (prompt queueing): queued holds prompts submitted while a run was
	// active, FIFO; the Done path drains the head into a new run.
	//
	// G12 (large-paste collapse): pasteStore maps a one-line "[pasted N lines]"
	// chip (shown in the input) to the full pasted text; the chip is expanded
	// back to its content at submit. pasteSeq keeps chip tokens unique per
	// session so two pastes never alias the same store entry.
	uiRunning       bool
	placeholderTurn int
	queued          []string
	pasteStore      map[string]string
	pasteSeq        int

	// Session / snapshot integration. nil when ephemeral. The four
	// callbacks travel together so we pack them in one struct field
	// rather than four flat ones.
	session sessionIntegration

	// MCP/LSP status callbacks for the /mcp and /lsp overlays.
	mcpStatus func() []McpServerRow
	lspStatus func() []LspServerRow

	// Permission status callback for the /permissions overlay.
	permStatus func() []PermissionRow

	// Notifier fires desktop/terminal notifications for long-running agent
	// completion and permission requests. nil = no-op.
	notifier Notifier

	// Wiring back to the tea.Program for callbacks running off the UI loop.
	send func(tea.Msg)

	// User-defined slash commands loaded from .deepseek/command/*.md.
	customCmds map[string]commands.Command

	// Notices shown once at startup (resume confirmation, warnings).
	startupNotices []string

	// lastRenderSeq is the scrollback Seq value at the last refreshView
	// call. The redraw tick refreshes only when the live seq has drifted,
	// replacing the older "dirty bool" pattern.
	lastRenderSeq uint64

	// errLang is the resolved language for API error messages ("en" or "zh").
	errLang string
}

// sessionIntegration bundles the optional persistence hooks. All four
// nil ⇒ TUI runs ephemeral (no /undo, /sessions, model persistence).
type sessionIntegration struct {
	id       string
	undo     func(n int) (int, error)
	list     func() ([]session.Session, error)
	setModel func(model string) error
	setTheme func(theme string) error
}

// Config bundles construction params for New.
type Config struct {
	Agent    *agent.Agent
	Model    string
	Thinking bool
	Theme    string
	Cwd      string

	// TransparentBackground (ui.transparent_background) disables ALL background
	// fills — the bg-tier panels (tool results, diffs, reasoning) degrade to
	// left-bars / separators with no opaque fills. The full-screen canvas is no
	// longer painted regardless (it bled through glamour/viewport resets; see
	// ADR-0002), so this flag is now a "fully flat / fg-only" toggle.
	TransparentBackground bool

	// Optional persistence integration. Provide all three or none.
	SessionID    string
	UndoFn       func(n int) (int, error)
	ListSessions func() ([]session.Session, error)
	SetModelFn   func(model string) error
	SetThemeFn   func(theme string) error

	// StartupNotices are shown as info chat items at TUI start. Used for
	// resume confirmations, warnings about degraded persistence, etc. —
	// anything the caller would have written to stderr in CLI mode.
	StartupNotices []string

	// CompactionCount initialises the status-line compaction counter from
	// a resumed session's history. Populate from sess.CompactionCount.
	CompactionCount int

	// Commands are user-defined slash commands loaded from .deepseek/command/*.md.
	Commands map[string]commands.Command

	// HistoryPath is the per-project prompt-history ring file (G2). The caller
	// (cmd/dsc) owns the path construction so the TUI stays filesystem-light.
	// Empty disables persistence: history lives only for the session and no
	// file is read or written.
	HistoryPath string

	// HistorySeed folds extra recall entries (oldest → newest) into the ring on
	// top of whatever HistoryPath loads — e.g. a resumed session's prior user
	// prompts. Deduped against the file by the ring's push rules.
	HistorySeed []string

	// Language overrides the error message language. "" uses auto-detect.
	Language string

	// LSPReady reports whether at least one LSP server is attached.
	LSPReady bool

	// MCPStatus returns MCP server snapshots for the /mcp overlay.
	MCPStatus func() []McpServerRow

	// LSPStatus returns LSP server snapshots for the /lsp overlay.
	LSPStatus func() []LspServerRow

	// PermStatus returns permission policy rows for the /permissions overlay.
	PermStatus func() []PermissionRow

	// Notifier fires desktop/terminal notifications. nil means no
	// notifications (no-op).
	Notifier Notifier

	// InitialBalance seeds the status-line account balance display.
	// Populated from llm.Client.GetUserBalance at TUI launch. Empty
	// means balance unavailable or not a DeepSeek provider.
	InitialBalance string

	// InitialUsageSummary seeds the status-line cost/cache totals from a
	// resumed session's persisted usage. Populated from
	// Persister.UsageSummary at launch. Zero value means no prior usage.
	InitialUsageSummary session.UsageSummary
}

// New constructs an App. The returned App is a tea.Model; pass it to
// tea.NewProgram and call .Run().
func New(cfg Config) *App {
	ta := textarea.New()
	// Placeholder is set dynamically by refreshPlaceholder (G10) once the App is
	// built — it reflects idle vs. working state and rotates idle hints.
	ta.Prompt = "› "
	ta.SetWidth(80)
	ta.SetHeight(3)
	ta.ShowLineNumbers = false
	ta.CharLimit = 0
	// Theme the cursor to the brand block so the caret reads as part of the
	// Ocean identity rather than the terminal default. The textarea exposes a
	// Cursor style on its Styles bag; everything else stays at library default.
	theme := PickTheme(cfg.Theme).WithTransparent(cfg.TransparentBackground)
	st := ta.Styles()
	st.Cursor.Color = theme.BrandDeep
	st.Cursor.Shape = tea.CursorBlock
	ta.SetStyles(st)
	ta.Focus()
	// Add "shift+enter" to the InsertNewline binding so the textarea
	// recognises it as a newline key.  Without this, bubbletea's default
	// binding only matches "enter" and "ctrl+m"; Shift+Enter falls through
	// as an unrecognised key and nothing happens.
	ta.KeyMap.InsertNewline = key.NewBinding(
		key.WithKeys("enter", "shift+enter", "ctrl+m"),
		key.WithHelp("enter", "insert newline"),
	)

	vp := viewport.New(viewport.WithWidth(80), viewport.WithHeight(20))
	vp.MouseWheelEnabled = true
	vp.SetHorizontalStep(0) // disable horizontal scroll: scrollback is up/down only

	app := &App{
		agent:          cfg.Agent,
		model:          cfg.Model,
		thinking:       cfg.Thinking,
		cwd:            cfg.Cwd,
		theme:          theme,
		keymap:         defaultKeymap(),
		vp:             vp,
		input:          ta,
		scrollback:     NewScrollback(),
		chrome:         NewChrome(),
		overlay:        NewOverlay(),
		permission:     NewPermissionFlow(),
		question:       NewQuestionFlow(),
		customCmds:     cfg.Commands,
		startupNotices: cfg.StartupNotices,
		mcpStatus:      cfg.MCPStatus,
		lspStatus:      cfg.LSPStatus,
		permStatus:     cfg.PermStatus,
		notifier:       cfg.Notifier,
		session: sessionIntegration{
			id:       cfg.SessionID,
			undo:     cfg.UndoFn,
			list:     cfg.ListSessions,
			setModel: cfg.SetModelFn,
			setTheme: cfg.SetThemeFn,
		},
		status: statusState{
			model:           cfg.Model,
			thinking:        cfg.Thinking,
			compactionCount: cfg.CompactionCount,
			usage: llm.Usage{
				PromptCacheHitTokens:  cfg.InitialUsageSummary.CacheHitTokens,
				PromptCacheMissTokens: cfg.InitialUsageSummary.CacheMissTokens,
				CompletionTokens:      cfg.InitialUsageSummary.OutputTokens,
			},
			costYuan:    cfg.InitialUsageSummary.CostYuan,
			savedYuan:   cfg.InitialUsageSummary.SavedYuan,
			costKnown:   llm.CostKnown(cfg.Model),
			steps:       cfg.InitialUsageSummary.Turns,
			balanceNote: cfg.InitialBalance,
			activeAgent: "coding-default",
		},
	}
	app.errLang = llm.ResolveLang(cfg.Language)
	// Thread the agent's real context window and first-token timeout into the
	// status HUD / cold-start caption. Both are nil-safe so tests that build
	// an App without an Agent (or a provider Client) still construct cleanly.
	if cfg.Agent != nil {
		app.status.contextLimit = cfg.Agent.MaxContextTokens
		app.status.reasoningEffort = cfg.Agent.ReasoningEffort
		if cfg.Agent.Client != nil {
			app.chrome.SetFirstTokenTimeout(cfg.Agent.Client.FirstTokenTimeout)
		}
		// Capability counters for the status-line segment.
		if cfg.Agent.MCPRegistry != nil {
			app.status.mcpTools = len(cfg.Agent.MCPRegistry.Tools())
		}
		if cfg.Agent.Skills != nil {
			app.status.skills = len(cfg.Agent.Skills.List())
		}
		app.status.lspReady = cfg.LSPReady
	}
	// Seed the scrollback's model context so tool cards get the right per-model
	// accent bar from the first turn (kept in sync on /models switches).
	app.scrollback.SetModel(cfg.Model)

	// Build the prompt-history ring (G2): the cross-session file first, then
	// any per-session seed folded in (oldest → newest, deduped by push rules).
	// loadPromptHistory tolerates a missing / empty path, so this is safe even
	// in ephemeral mode where HistoryPath is "".
	const historyCap = 500
	seed := loadPromptHistory(cfg.HistoryPath, historyCap)
	if len(cfg.HistorySeed) > 0 {
		seed = append(seed, cfg.HistorySeed...)
	}
	app.history = newPromptHistory(seed, historyCap)
	app.historyPath = cfg.HistoryPath

	// Seed the dynamic placeholder (G10) from the idle state so the very first
	// frame shows a "Ready" hint rather than the library default. It then flips
	// to "Working…" on runStartMsg and back (advancing the rotation) at Done.
	app.refreshPlaceholder()
	return app
}

// Run starts the Bubble Tea program. Blocks until quit.
//
// Mouse cell-motion is enabled so we receive click + drag + release
// events. We use them to drive a TUI-managed selection (handleMouse):
// click-drag highlights lines, dragging past the top/bottom edge
// auto-scrolls the viewport so the selection can span the entire
// scrollback, and releasing the button auto-yanks the selected text
// to the clipboard via OSC 52. Mouse wheel still scrolls the viewport
// (bubbles/viewport handles that).
//
// Why not native terminal drag-select: alt-screen mode means the
// terminal has no scrollback above our rendered frame, so native
// drag tops out at the visible window. TUI-managed selection is the
// only way to extend across content the user has scrolled past.
//
// If the user wants native terminal behavior (mouse drag-select +
// terminal's own scrollback), `P` in Normal mode (or /export) opens
// the full session in $PAGER (default `less -R`) which owns the TTY
// completely while running.
func (a *App) Run() error {
	prog := tea.NewProgram(a)
	a.send = prog.Send
	// Spawn the agent-event pump. Reads the agent's lifetime event
	// stream, wraps each event into a single agentEventMsg, and hands
	// it to the tea.Program. Replaces the old Callbacks-of-9-funcs
	// design: now there's one consumer and one envelope.
	go a.pumpEvents()
	_, err := prog.Run()
	return err
}

// pumpEvents bridges agent.Events() to the tea.Program by wrapping
// each event into an agentEventMsg. Runs for the lifetime of the
// process; exits when the channel is closed (which the agent does
// not currently do — process exit cleans it up).
func (a *App) pumpEvents() {
	for ev := range a.agent.Events() {
		a.send(agentEventMsg{Event: ev})
	}
}

// runAgent runs one prompt turn in the goroutine spawned by
// submitPromptCmd. The "Run finished" signal travels through the
// agent's event channel as agent.EventDone — runAgent itself just
// owns the cancellation handles and the running flag.
func (a *App) runAgent(prompt string) {
	ctx, cancel := context.WithCancel(context.Background())

	a.runMu.Lock()
	a.runCtx = ctx
	a.runCancel = cancel
	a.running = true
	a.runMu.Unlock()

	_, _ = a.agent.Run(ctx, prompt)

	a.runMu.Lock()
	a.runCancel = nil
	a.runCtx = nil
	a.running = false
	a.runMu.Unlock()
}

// Init satisfies tea.Model. The first scrollback entry is an ASCII
// welcome banner (whale mascot + DEEPSEEKCODE wordmark); startup
// notices like resume confirmations follow.
func (a *App) Init() tea.Cmd {
	a.scrollback.AppendWelcome()
	for _, n := range a.startupNotices {
		a.scrollback.AppendInfo(n)
	}
	// Build the `@` file-mention index up front (G4/§6.1) so the first '@' the
	// user types already has candidates. The walk runs off the UI goroutine in
	// indexFilesCmd; until its fileIndexMsg lands the @ popup shows a single
	// "(indexing files…)" row. Skip the Cmd when cwd isn't a readable dir.
	cmds := []tea.Cmd{textarea.Blink}
	if readableDir(a.cwd) {
		cmds = append(cmds, indexFilesCmd(a.cwd))
	}
	return tea.Batch(cmds...)
}

// Update folds incoming messages into the model.
func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	// Mode-based key routing: Permission and Pager modes are modal and
	// eat all key input until dismissed. Handled here at the top so the
	// per-mode switch below stays simple.
	if a.mode == modePermission {
		if km, ok := msg.(tea.KeyPressMsg); ok {
			return a, a.handlePermissionKey(km)
		}
	}
	if a.mode == modeQuestion {
		if km, ok := msg.(tea.KeyPressMsg); ok {
			return a, a.handleQuestionKey(km)
		}
	}

	switch m := msg.(type) {
	case tea.WindowSizeMsg:
		a.width = m.Width
		a.height = m.Height
		a.layout()
	case tea.PasteMsg:
		// Large-paste collapse (G12). A bracketed paste over the line threshold
		// is held in full and replaced in the input by a one-line chip; when
		// handlePaste reports it collapsed the paste, we DON'T forward the raw
		// PasteMsg to the textarea (which would re-insert the wall of text).
		// Smaller pastes fall through to the textarea unchanged.
		if a.handlePaste(m.Content) {
			a.syncCompletions()
			return a, nil
		}
	case tea.KeyPressMsg:
		// Intercept special keys (overlay nav, ctrl+c, Enter, slash, etc).
		// If not intercepted, fall through so the textarea sees the key.
		if cmd, intercepted := a.handleKey(m); intercepted {
			return a, cmd
		}
		// In Normal mode, printable characters auto-switch to Insert
		// mode and flow through to the textarea on the same Update cycle.
		if a.mode == modeNormal && isPrintableKey(m) {
			cmds = append(cmds, a.setMode(modeInsert))
		}

	// agent → UI events
	case runStartMsg:
		// Fired immediately after submitPromptCmd; the agent goroutine
		// is up but no callback has arrived yet. Spin up the chrome
		// indicator so the user sees activity during the cold-start gap.
		// Mark the UI-side running flag and flip the placeholder to "Working…"
		// (G10) here rather than waiting on the worker goroutine's a.running.
		a.uiRunning = true
		a.refreshPlaceholder()
		a.chrome.BeginThinking()
		cmds = append(cmds, a.ensureTick())
	case slashExpandedMsg:
		a.scrollback.AppendUser(m.text)
		a.refreshView()
		return a, a.submitPromptCmd(m.text)
	case spawnExpandedMsg:
		a.scrollback.AppendUser(m.text)
		a.refreshView()
		return a, a.spawnCmd(m.text, m.agent)
	case spawnResultMsg:
		a.scrollback.AppendInfo(m.summary)
		a.refreshView()
	case clearToastMsg:
		// Deferred toast expiry (G8/§6.5). Guard on seq so a stale tick from a
		// toast that has since been superseded can't wipe the current one.
		a.clearToastIf(m.seq)
	case disarmQuitMsg:
		// Deferred disarm of the idle ^C double-tap (G9/§6.6). Guard on seq so a
		// re-arm (a fresh first ^C) isn't disarmed by the previous arm's tick.
		if a.quitSeq == m.seq {
			a.quitArmed = false
		}
	case fileIndexMsg:
		// The background `@` file walk completed (G4/§6.1). Cache the paths and
		// mark the index ready. If an @ popup is already open showing the
		// "(indexing files…)" placeholder, re-derive it now so the real rows
		// appear without another keystroke.
		a.fileIndex = m.paths
		a.fileIndexReady = true
		if a.completions.Active() && a.completions.Trigger() == '@' {
			// syncCompletions only rebuilds the candidate set when the trigger or
			// anchor moved; close first so it re-opens with the now-ready file
			// rows instead of keeping the stale "(indexing files…)" placeholder.
			a.completions.Close()
			a.syncCompletions()
		}
	case redrawMsg:
		// Coalesced re-render. Streaming deltas don't refresh on
		// arrival; instead they bump scrollback.Seq() and we check
		// for drift here at the tick. Spinner frame advances every
		// tick regardless so the indicator animates smoothly.
		a.chrome.AdvanceFrame()
		if a.scrollback.Seq() != a.lastRenderSeq {
			a.refreshView()
		}
		if a.chrome.Active() || a.running {
			cmds = append(cmds, a.scheduleRedraw())
		} else {
			a.chrome.MarkTickStopped()
		}
	case agentEventMsg:
		cmds = append(cmds, a.dispatchAgentEvent(m.Event)...)
	case tea.MouseClickMsg:
		if cmd := a.handleMouse(m.Mouse(), mouseActionPress); cmd != nil {
			cmds = append(cmds, cmd)
		}
	case tea.MouseMotionMsg:
		if cmd := a.handleMouse(m.Mouse(), mouseActionMotion); cmd != nil {
			cmds = append(cmds, cmd)
		}
	case tea.MouseReleaseMsg:
		if cmd := a.handleMouse(m.Mouse(), mouseActionRelease); cmd != nil {
			cmds = append(cmds, cmd)
		}
	case tea.MouseWheelMsg:
		// Wheel events go to viewport via vp.Update below.
	}

	// Forward to sub-models. Mouse events are routed to the viewport only
	// (for wheel scrolling); sending them to the textarea would let raw
	// SGR escape sequences leak into the input box as garbled characters
	// like "[<66;101;9M".
	//
	// Only Insert mode forwards key events to the textarea. Normal,
	// Permission, and Pager are all input-blurred modes — letting their
	// keystrokes reach the textarea would re-introduce the routing bug
	// where 'j'/'k' showed up as literal text after a /clear.
	var c tea.Cmd
	if !isV2MouseMsg(msg) {
		if a.mode == modeInsert {
			a.input, c = a.input.Update(msg)
			cmds = append(cmds, c)
			// Re-derive the inline `/`/`@` popup from the post-keystroke buffer
			// (§5.1/§6). Only for actual key presses in Insert mode: the popup
			// is a function of the text + caret, and only typing moves those.
			if _, ok := msg.(tea.KeyPressMsg); ok {
				a.syncCompletions()
			}
		}
	}
	a.vp, c = a.vp.Update(msg)
	cmds = append(cmds, c)
	return a, tea.Batch(cmds...)
}

// dispatchAgentEvent routes one agent.Event to the right sub-module
// and returns any tea.Cmds to schedule. Single type-switch replaces
// the ten-case fan-out that used to live in Update.
func (a *App) dispatchAgentEvent(ev agent.Event) []tea.Cmd {
	var cmds []tea.Cmd
	switch e := ev.(type) {
	case agent.EventReasoningStart:
		_ = e
		a.chrome.BeginThinking()
		a.scrollback.StartReasoning()
		a.refreshView()
		cmds = append(cmds, a.ensureTick())
	case agent.EventReasoningDelta:
		a.chrome.UpdateTokens(a.scrollback.AppendReasoning(e.Text))
	case agent.EventReasoningEnd:
		_ = e
		a.scrollback.EndReasoning()
	case agent.EventTextDelta:
		created, toks := a.scrollback.AppendText(e.Text)
		if created {
			a.chrome.BeginWriting()
			a.refreshView()
			cmds = append(cmds, a.ensureTick())
		}
		a.chrome.UpdateTokens(toks)
	case agent.EventToolCallStart:
		a.chrome.BeginTool(e.Call.Function.Name)
		a.scrollback.AppendToolCall(e.Call.ID, e.Call.Function.Name, e.Call.Function.Arguments)
		a.refreshView()
		cmds = append(cmds, a.ensureTick())
	case agent.EventToolCallResult:
		a.chrome.ToolReady()
		a.scrollback.AppendToolResult(e.CallID, e.Result, e.Dur)
		a.refreshView()
	case agent.EventHookFired:
		a.scrollback.AppendHookFired(e.HookName, e.Event, e.Decision, e.Reason, e.Dur)
		a.refreshView()
	case agent.EventStepFinish:
		a.stepTotal++
		// Close the current tool batch at the step boundary so the next
		// tool turn's "N ready" counter starts fresh even when the model's
		// next turn is a pure tool-call turn (no reasoning/text to flip the
		// chrome phase away from phaseTool).
		a.chrome.EndToolBatch()
		a.status.steps = a.stepTotal
		a.status.usage.PromptTokens += e.Usage.PromptTokens
		a.status.usage.CompletionTokens += e.Usage.CompletionTokens
		a.status.usage.PromptCacheHitTokens += e.Usage.PromptCacheHitTokens
		a.status.usage.PromptCacheMissTokens += e.Usage.PromptCacheMissTokens
		a.status.costYuan += llm.Cost(a.model, e.Usage)
		a.status.savedYuan += llm.CacheSavings(a.model, e.Usage)
		a.status.costKnown = llm.CostKnown(a.model)
		a.scrollback.AppendStepFinish(e.Reason.String(), e.Usage, a.model)
		a.refreshView()
	case agent.EventCompaction:
		a.status.compactionCount++
		a.scrollback.AppendInfo(fmt.Sprintf("compacted %d message(s)", e.RemovedCount))
		a.refreshView()
	case agent.EventInfo:
		a.scrollback.AppendInfo(e.Text)
		// The prefix-cache invalidation warning has its own slot
		// (cacheNote) so it no longer clobbers the generic transient hint;
		// the two coexist on the status line.
		if strings.HasPrefix(e.Text, "prefix cache invalidated: ") {
			a.status.cacheNote = "⚠ cache:" + strings.TrimPrefix(e.Text, "prefix cache invalidated: ")
		}
		a.refreshView()
	case agent.EventBudget:
		// T1.3: budget-gate decisions are typed now; render them as info lines
		// so the prior user-visible behavior is preserved.
		switch e.Kind {
		case agent.BudgetKindBlocked:
			a.scrollback.AppendInfo("budget blocked: projected session cost reached hard threshold")
		case agent.BudgetKindUnpriced:
			a.scrollback.AppendInfo("budget gate: model " + e.Model + " has no known pricing; turns cannot be cost-gated")
		default:
			a.scrollback.AppendInfo("budget warning: projected session cost reached warning threshold")
		}
		a.refreshView()
	case agent.EventPermissionDenied:
		if e.ByRule {
			a.scrollback.AppendInfo("denied by rule: " + e.Reason)
		} else {
			a.scrollback.AppendInfo("denied by permissions policy: " + e.Reason)
		}
		a.refreshView()
	case agent.EventRepair:
		a.scrollback.AppendRepair(e.Kind, e.Tool, e.Message)
		a.refreshView()
	case agent.EventPermissionAsk:
		// Close any open overlay so the permission card is visible.
		// Without this, /mcp, /lsp, palette, or /permissions would
		// render on top while key input routes to the hidden prompt.
		if a.overlay.IsOpen() {
			a.overlay.Close()
		}
		a.permission.Open(e)
		cmds = append(cmds, a.setMode(modePermission))
		// Re-layout so the hidden input box gives its rows back to the
		// viewport — the permission card is the active surface now.
		a.layout()
		a.notifySafe("DeepSeekCode", "Permission requested")
	case agent.EventQuestionAsk:
		a.question.Open(e)
		cmds = append(cmds, a.setMode(modeQuestion))
		a.layout()
	case agent.EventDone:
		// Terminator for this Run. Arrives AFTER every other event from
		// this turn because Run defers the emit. Reset chrome here so
		// the "writing…" caption can't linger past trailing deltas.
		if e.Err != nil {
			a.scrollback.AppendError(llm.LocalizeError(e.Err, a.errLang))
		}
		a.scrollback.EndStreams()
		a.chrome.Reset()
		a.refreshView()
		a.notifySafe("DeepSeekCode", "Task finished")
		// This run is finished UI-side. Advance the idle-hint rotation (G10) and
		// drain the next queued prompt (G11): if one is waiting, submitting it
		// flips uiRunning back on via the runStartMsg it returns; otherwise the
		// placeholder settles on the next "Ready" hint.
		a.placeholderTurn++
		if drain := a.drainQueue(); drain != nil {
			cmds = append(cmds, drain)
		} else {
			a.uiRunning = false
		}
		a.refreshPlaceholder()
	case agent.EventBackgroundJobStart:
		a.status.runningJobs++
		a.refreshView()
	case agent.EventBackgroundJobFinish:
		if a.status.runningJobs > 0 {
			a.status.runningJobs--
		}
		a.refreshView()
	}
	return cmds
}

// scheduleRedraw returns a tea.Cmd that fires one redrawMsg after the
// coalescing interval. We aim for ~12 fps — fast enough that the
// spinner looks smooth and streaming text feels live, slow enough that
// fast token bursts don't dominate the CPU with viewport reflows.
func (a *App) scheduleRedraw() tea.Cmd {
	return tea.Tick(80*time.Millisecond, func(time.Time) tea.Msg {
		return redrawMsg{}
	})
}

// ensureTick starts the redraw ticker if one isn't already pending.
// Idempotent — safe to call from any message handler that observes
// new activity. The tick stops itself on the next redrawMsg after
// chrome.Active() and a.running both flip false.
func (a *App) ensureTick() tea.Cmd {
	if a.chrome.TickActive() {
		return nil
	}
	a.chrome.MarkTickStarted()
	return a.scheduleRedraw()
}

// View renders the whole UI.
//
// Note: we do NOT paint a full-screen background. An outer lipgloss Background
// cannot reliably fill behind glamour / viewport output — those emit ANSI
// resets (\x1b[0m) that snap the background to the terminal default mid-line,
// leaving ragged black gaps (see ADR-0002). Backgrounds live only on explicit
// panels / badges / diff bands, which render their own full-width per-line
// fills and so never bleed; the screen sits on the terminal's own background.
func (a *App) View() tea.View {
	if a.width == 0 || a.height == 0 {
		return tea.View{Content: "starting…", AltScreen: true, MouseMode: tea.MouseModeCellMotion}
	}

	// Cursor: only visible in insert mode with no overlay/modal.
	var cur *tea.Cursor
	if a.mode == modeInsert && !a.overlay.IsOpen() && !a.permission.Active() && !a.question.Active() {
		cur = a.input.Cursor()
	}

	if a.overlay.IsOpen() {
		return tea.View{Content: a.renderOverlay(), AltScreen: true, MouseMode: tea.MouseModeCellMotion, Cursor: cur}
	}

	// Pager is a modal body replacement: it takes the entire body band
	// while the status / input / hint rows below stay anchored. Keeps
	// the user oriented and the input ready for a follow-up prompt
	// after they dismiss with q.
	var body string
	if a.pager != nil {
		body = a.pager.render(a.theme, a.width, a.vp.Height())
	} else {
		body = a.overlayScrollbar(a.vp.View())
	}

	// Reserved chrome row — always one line tall. Streaming spinner
	// when active; "new content below" indicator when the user has
	// scrolled away from the bottom mid-run; blank otherwise.
	showNewBelow := a.running && !a.vp.AtBottom() && a.pager == nil
	chrome := a.chrome.Render(a.theme, showNewBelow)
	if chrome == "" {
		chrome = " " // reserve the row to prevent reflow when state flips
	}

	var permView string
	if a.permission.Active() {
		permView = a.permission.Render(a.theme, a.width)
	}
	var questionView string
	if a.question.Active() {
		questionView = a.question.Render(a.theme, a.width)
	}

	status := a.status.render(a.theme)
	divider := a.theme.Hint.Render(strings.Repeat("─", a.width))
	header := renderHeaderBar(a.theme, "deepseekcode", version.Display(), a.width)

	// Permission card replaces the input box: while it is up, the
	// modal is the active surface and the textarea is irrelevant.
	// Skip the input-box / hint rows entirely so the card sits flush
	// above the status line.
	if permView != "" {
		parts := []string{header, body, chrome, permView, divider, status}
		return tea.View{Content: lipgloss.JoinVertical(lipgloss.Left, parts...), AltScreen: true, MouseMode: tea.MouseModeCellMotion, Cursor: cur}
	}
	if questionView != "" {
		parts := []string{header, body, chrome, questionView, divider, status}
		return tea.View{Content: lipgloss.JoinVertical(lipgloss.Left, parts...), AltScreen: true, MouseMode: tea.MouseModeCellMotion, Cursor: cur}
	}

	// Choose input border style based on mode.
	// Insert mode uses brand blue; Normal/Visual uses dim.
	borderStyle := lipgloss.NewStyle().Foreground(a.theme.BrandDeep)
	if a.mode != modeInsert {
		borderStyle = a.theme.InputBorderDim
	}
	inputBox := borderStyle.
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderStyle.GetForeground()).
		Padding(0, 1).
		Width(a.width - 2).
		MaxHeight(a.input.Height() + 2).
		Render(a.input.View())

	// Mode chip: a small filled badge surfaced just above the input border
	// when the launch mode is anything other than the tiered default, so the
	// elevated/lowered permission posture is always visible at the prompt.
	if chip := a.modeChip(); chip != "" {
		inputBox = lipgloss.JoinVertical(lipgloss.Left, a.theme.Gutter()+chip, inputBox)
	}

	// Mode-aware hint text.
	var hint string
	switch a.mode {
	case modeNormal:
		hint = a.theme.Hint.Render(
			"  j/k scroll · v select · y yank · p pager · P less (mouse copy) · i type · ^C quit",
		)
	case modeVisual:
		hint = a.theme.Hint.Render(
			"  j/k extend · ^U/^D page · g/G jump · o swap ends · y yank · esc cancel",
		)
	case modePager:
		hint = a.theme.Hint.Render(
			"  j/k scroll · ^U/^D page · G/gg jump · q close pager",
		)
	default:
		hint = a.theme.Hint.Render("  " + i18n.T("app.input.hint"))
	}

	// Splice the completions popup (G1/§4.4) directly above the input box so it
	// reads as a card floating just over the prompt. layout() has already
	// reserved a.popupLines rows for it, so nothing overflows the screen. The
	// transient toast (G8/§6.5) sits one row below the status bar, above both
	// the popup and the input, colored by kind.
	parts := []string{header, body, chrome, divider, status}
	if toast := a.renderToast(); toast != "" {
		parts = append(parts, toast)
	}
	if pop := a.completions.View(a.theme, a.width); pop != "" {
		parts = append(parts, pop)
	}
	parts = append(parts, inputBox, hint)
	return tea.View{Content: lipgloss.JoinVertical(lipgloss.Left, parts...), AltScreen: true, MouseMode: tea.MouseModeCellMotion, Cursor: cur}
}

// modeChip returns a filled badge string for the active permission mode, or
// "" for the tiered default (no chip). YOLO is an error-colored chip,
// READ-ONLY rides the brand chip, and PLAN uses an fgSubtle-backed chip built
// on the same filled-chip contract as Theme.Badge. nil-safe: an App built
// without a Permissions policy (test fixtures) shows no chip.
func (a *App) modeChip() string {
	if a.agent == nil || a.agent.Permissions == nil {
		return ""
	}
	switch a.agent.Permissions.Mode {
	case permissions.ModeYolo:
		return a.theme.Badge(BadgeErr).Render("YOLO")
	case permissions.ModeReadOnly:
		return a.theme.Badge(BadgeBrand).Render("READ-ONLY")
	case permissions.ModePlan:
		return lipgloss.NewStyle().
			Background(a.theme.FgSubtle).
			Foreground(a.theme.OnAccent).
			Padding(0, 1).
			Bold(true).
			Render("PLAN")
	default:
		return ""
	}
}

// renderOverlay renders the active fullscreen overlay (tape, models,
// sessions). The bottom hint line + status still show for context.
func (a *App) renderOverlay() string {
	// The painted footer takes the last row; the surface fills the rest. body +
	// footer therefore renders exactly a.height rows, all painted (no black gap).
	h := a.height - 1
	var body, footerText string
	switch a.overlay.Mode() {
	case modeTape:
		body = renderTape(a.theme, a.scrollback.Items(), a.overlay.Cursor(), a.width, h)
		footerText = "j/k move · esc back · q quit"
	case modeModels:
		body = renderModelsPicker(a.theme, a.overlay.Models(), a.overlay.VisibleRows(), a.overlay.FilterCursor(), a.overlay.FilterString(), a.model, a.width, h)
		footerText = "j/k move · ⏎ switch · esc back · q quit"
	case modeSessions:
		body = renderSessionsPicker(a.theme, a.overlay.SessionsRows(), a.overlay.VisibleRows(), a.overlay.FilterCursor(), a.overlay.FilterString(), a.session.id, a.width, h)
		footerText = "j/k move · ⏎ switch · esc back · q quit"
	case modePalette:
		body = renderPalette(a.theme, a.overlay.Palette(), a.overlay.VisibleRows(), a.overlay.FilterCursor(), a.overlay.FilterString(), a.width, h)
		footerText = "j/k move · ⏎ run · esc back · q quit"
	case modeHelp:
		body = renderHelp(a.theme, a.helpCommandRows(), a.overlay.HelpTab(), a.overlay.Cursor(), a.width, h)
		footerText = "tab/←→/hl switch · j/k scroll · esc close"
	case modeThemes:
		body = renderThemesPicker(a.theme, a.overlay.Themes(), a.overlay.VisibleRows(), a.overlay.FilterCursor(), a.overlay.FilterString(), a.theme.Name, a.width, h)
		footerText = "j/k move · ⏎ apply · esc cancel · live preview"
	case modeMCP:
		body = renderMCP(a.theme, a.overlay.MCPServers(), a.overlay.VisibleRows(), a.overlay.FilterCursor(), a.overlay.FilterString(), a.width, h)
		footerText = "type to filter · esc close"
	case modeLSP:
		body = renderLSP(a.theme, a.overlay.LSPServers(), a.overlay.VisibleRows(), a.overlay.FilterCursor(), a.overlay.FilterString(), a.width, h)
		footerText = "type to filter · esc close"
	case modePermissions:
		body = renderPermissions(a.theme, a.overlay.Permissions(), a.width, h)
		footerText = "esc close"
	}
	return body + "\n" + overlayFooter(a.theme, footerText, a.width)
}

// layout recomputes viewport / input geometry on window resize.
//
// Vertical budget (top to bottom):
//
//	viewport (bodyH) · chrome (1, always reserved) ·
//	permission prompt (permH, if active) · divider (1) ·
//	status (1 or 2) · input box (textarea + border = 5) · hint (1).
//
// The chrome row is reserved even when idle so the viewport doesn't
// jump up and down as streaming state flips on and off.
func (a *App) layout() {
	const (
		statusH  = 1
		chromeH  = 1
		dividerH = 1
		headerH  = 1 // brand header strip
	)
	// Use the textarea's actual height, not a hardcoded 3 — the
	// textarea grows when the user types multi-line content, and a
	// fixed inputH would make the viewport render into the input area
	// (garbled overlap on scroll).
	inputH := a.input.Height() + 2 // textarea + rounded border(2)
	if a.modeChip() != "" {
		inputH++ // mode chip rides one row above the input border
	}
	hintH := 1
	permH := 0
	if a.permission.Active() {
		// Modal: 7 body lines + 2 border ≈ 9. Match this to the row
		// count rendered by PermissionFlow.Render.
		permH = 9
		// Hide input box + hint rows while the modal is up — the View
		// layer also skips them, but we have to reclaim the rows here
		// so the viewport gets the height back.
		inputH = 0
		hintH = 0
	}
	// A live toast (G8/§6.5) takes one extra row between status and the input;
	// reserve it so it never pushes the input off-screen.
	toastH := 0
	if a.toastState.active && a.toastState.text != "" {
		toastH = 1
	}
	// Reserve the completions popup's rows (G1/§4.4) so it grows upward from the
	// prompt without pushing the input off-screen. popupLines == 0 reproduces
	// exactly the pre-popup geometry. The popup is the only flexible band, so
	// layout is its single authority: cap its visible rows at whatever vertical
	// space is left once the fixed rows (chrome/divider/status/input/hint/perm/
	// toast) and the minimum body floor are accounted for, then derive popupLines
	// from the now-clamped Lines(). Without this clamp a tall match set on a short
	// terminal would overflow the View stack and shove the input box off-screen.
	const minBodyH = 5
	popupBudget := a.height - headerH - chromeH - statusH - dividerH - inputH - hintH - permH - toastH - minBodyH
	a.completions.SetMaxRows(popupBudget - 2) // budget is card lines; -2 for the borders
	a.popupLines = a.completions.Lines()
	bodyH := a.height - headerH - chromeH - statusH - dividerH - inputH - hintH - permH - a.popupLines - toastH
	if bodyH < minBodyH {
		bodyH = minBodyH
	}
	a.vp.SetWidth(a.width)
	a.vp.SetHeight(bodyH)
	// Textarea content width = total width − 2 border cols − 2 padding cols.
	a.input.SetWidth(a.width - 4)
	a.refreshView()
}

// notifySafe fires a notification if a Notifier is wired. Errors are silently
// discarded — notifications are best-effort and must never disrupt the UI loop.
func (a *App) notifySafe(title, body string) {
	if a.notifier != nil {
		_ = a.notifier.Notify(title, body)
	}
}

// refreshView pushes the scrollback's rendered content into the
// viewport. Preserves scroll position: if the viewport was at the
// bottom we auto-scroll; if the user has scrolled up we keep it.
// Auto-scroll is suppressed while a selection is active so streaming
// output doesn't yank focus away from what the user is highlighting.
func (a *App) refreshView() {
	if a.width == 0 {
		return
	}
	wasAtBottom := a.vp.AtBottom()
	content := a.scrollback.Render(a.theme, a.width)
	a.vp.SetContent(content)
	a.lastRenderSeq = a.scrollback.Seq()
	if wasAtBottom && !a.scrollback.SelectionActive() {
		a.vp.GotoBottom()
	}
}

// handleKey dispatches a keypress based on the current mode.
// Returns (cmd, intercepted) — when intercepted is false, the caller
// falls through so the textarea receives the key.
func (a *App) handleKey(km tea.KeyPressMsg) (tea.Cmd, bool) {
	// Overlays consume nearly all keys until ESC.
	if a.overlay.IsOpen() {
		return a.handleOverlayKey(km), true
	}

	// An armed idle-quit (G9/§6.6) is cancelled by ANY key other than ^C: the
	// user changed their mind, so disarm and dismiss the "press ^C again" prompt
	// immediately rather than leaving it primed until the disarm tick. Bumping
	// quitSeq makes the pending disarm tick a no-op. We then fall through so the
	// key still performs its normal action this cycle.
	if a.quitArmed && km.String() != "ctrl+c" {
		a.quitArmed = false
		a.quitSeq++
		a.dismissToast()
	}

	// Global keys that work in any mode.
	switch km.String() {
	case "ctrl+c":
		// While a run is active, ctrl+c cancels it immediately (unchanged
		// behavior). When idle, ctrl+c is a guarded quit (G9/§6.6): the first
		// press arms quit and shows a toast, a second press within the window
		// quits, and a disarm tick drops the arm so a lone stray ^C never kills
		// an idle session.
		a.runMu.Lock()
		running := a.running
		cancel := a.runCancel
		a.runMu.Unlock()
		if running && cancel != nil {
			// Mark the stop as user-requested before cancelling so the run
			// reports StopUserRequested rather than ambient StopContextCancel.
			// Drop any stale idle-quit arm so the next idle ^C re-arms cleanly
			// instead of quitting on a leftover flag.
			a.quitArmed = false
			a.agent.RequestStop()
			cancel()
			// Cancel is an explicit "stop" (G11): drop any queued follow-ups
			// rather than auto-running them once the cancelled run reaches Done —
			// the user pressed ^C to halt, not to advance the queue. clearQueue
			// returns a toast Cmd reporting the drop (nil when nothing queued).
			return a.clearQueue(), true
		}
		if a.quitArmed {
			return tea.Quit, true
		}
		a.quitArmed = true
		a.quitSeq++
		seq := a.quitSeq
		toastCmd := a.toast(BadgeWarn, "press ^C again to quit")
		disarm := tea.Tick(quitArmWindow, func(time.Time) tea.Msg {
			return disarmQuitMsg{seq: seq}
		})
		return tea.Batch(toastCmd, disarm), true
	case "ctrl+d":
		return tea.Quit, true
	case "ctrl+p":
		// Command palette (G5) — from both Insert and Normal mode. ctrl+p is
		// otherwise unbound; this never collides with the reasoning folds on
		// ctrl+r/ctrl+t. Per the precedence chain (§7) the inline completions
		// popup outranks the palette: while the / or @ menu is up, ctrl+p is
		// inert so it can't yank focus out from under an active completion.
		if a.completions.Active() {
			return nil, true
		}
		return a.openPalette(), true
	case "ctrl+g":
		// Help overlay (G7) — from any mode.
		return a.openHelp(), true
	}

	// Mode-specific dispatch.
	switch a.mode {
	case modeNormal:
		return a.handleNormalKey(km)
	case modeInsert:
		return a.handleInsertKey(km)
	case modePager:
		return a.handlePagerKey(km)
	case modeVisual:
		return a.handleVisualKey(km)
	default:
		return nil, false
	}
}

// handleInsertKey handles keys in Insert (typing) mode. The precedence
// chain is: the completions popup, when
// open, owns ↑/↓ (navigate), ⏎/Tab (accept, no submit) and Esc (close);
// otherwise ↑/↓ recall history at the top/bottom cursor edge, Esc leaves
// to Normal mode, ⏎ submits, and ctrl+r/t toggle reasoning folds.
// Everything else falls through to the textarea. The popup itself is
// (re)derived from the post-keystroke buffer by syncCompletions, called at
// the end of Update — handleInsertKey only consumes its current state.
func (a *App) handleInsertKey(km tea.KeyPressMsg) (tea.Cmd, bool) {
	// Popup-open precedence: while the menu is up it steals navigation,
	// accept, and dismiss keys before history or the textarea see them.
	if a.completions.Active() {
		switch km.String() {
		case "up":
			a.completions.Up()
			return nil, true
		case "down":
			a.completions.Down()
			return nil, true
		case "enter":
			if a.exactSlashCompletionReady() {
				return a.submitInput()
			}
			a.acceptCompletion()
			return nil, true
		case "tab":
			a.acceptCompletion()
			return nil, true
		case "esc":
			a.completions.Close()
			a.syncPopupLayout()
			return nil, true
		}
	}

	switch km.String() {
	case "ctrl+r":
		a.toggleLastReasoning()
		return nil, true
	case "ctrl+t":
		a.toggleAllReasoning()
		return nil, true
	case "esc":
		_ = a.setMode(modeNormal)
		return nil, true
	case "up":
		// History recall only at the top edge; otherwise the textarea moves
		// the cursor up within a multi-line draft.
		if a.input.Line() == 0 {
			if v, ok := a.history.Prev(a.input.Value()); ok {
				a.input.SetValue(v)
				// Keep the popup an exact mirror of the buffer: a recalled
				// entry that begins with / or @ re-opens the menu; anything
				// else closes it. The intercepted path skips Update's trailing
				// sync, so derive it here.
				a.syncCompletions()
			}
			return nil, true
		}
		return nil, false
	case "down":
		// Symmetric: history step-newer only at the bottom edge.
		if a.input.Line() == a.input.LineCount()-1 {
			if v, ok := a.history.Next(); ok {
				a.input.SetValue(v)
				a.syncCompletions()
			}
			return nil, true
		}
		return nil, false
	}

	if km.String() == "enter" {
		// Plain Enter submits; Shift+Enter / Alt+Enter inserts a newline.
		if km.Mod&tea.ModAlt != 0 || hasShiftEnter(km) {
			return nil, false
		}
		return a.submitInput()
	}

	// Any other printable key starts a fresh draft: reset the history browse
	// cursor so a later ↑ recalls from newest rather than clobbering the edit.
	a.history.Reset()
	return nil, false
}

// exactSlashCompletionReady reports whether Enter should submit instead of
// accepting a completion: the popup is on '/', the selected row exactly matches
// the whole trimmed input, and the row is insertable. Partial queries like
// "/mod" still accept "/models" first; a fully typed "/models" or "/theme"
// executes immediately.
func (a *App) exactSlashCompletionReady() bool {
	if !a.completions.Active() || a.completions.Trigger() != '/' {
		return false
	}
	sel, ok := a.completions.Selected()
	if !ok || sel.insert == "" {
		return false
	}
	return strings.TrimSpace(a.input.Value()) == sel.insert
}

func (a *App) submitInput() (tea.Cmd, bool) {
	// Expand any collapsed-paste chips (G12) back to their full text BEFORE
	// trimming/recording so the submitted prompt and the history entry carry
	// the real paste, not the "[pasted N lines]" placeholder.
	text := strings.TrimSpace(a.expandPaste(a.input.Value()))
	if text == "" {
		return nil, true
	}
	// Record the prompt for recall BEFORE Reset wipes the buffer, then
	// persist it off the UI goroutine (§10/§11: the file writer must not
	// race Update). history.Add resets the browsing cursor.
	histCmd := a.recordHistory(text)
	a.input.Reset()
	a.resetPasteState()
	a.completions.Close()
	a.syncPopupLayout()
	if strings.HasPrefix(text, "/") {
		return tea.Batch(a.handleSlash(text), histCmd), true
	}
	// Prompt queueing (G11): if a run is already active, queue the prompt
	// instead of starting a second concurrent run. The Done path drains it.
	// Slash commands are exempted above — they are local UI actions, not
	// agent turns, so they run immediately even mid-run.
	if a.uiRunning {
		return tea.Batch(a.enqueuePrompt(text), histCmd), true
	}
	a.scrollback.AppendUser(text)
	a.refreshView()
	return tea.Batch(a.submitPromptCmd(text), histCmd), true
}

// recordHistory adds text to the recall ring and returns a tea.Cmd that
// appends it to the on-disk ring file, or nil when persistence is disabled
// (empty path). The file write runs in the returned Cmd so it stays off the
// UI goroutine (§10/§11) — go test -race must see no write racing Update.
func (a *App) recordHistory(text string) tea.Cmd {
	a.history.Add(text)
	if a.historyPath == "" {
		return nil
	}
	path := a.historyPath
	return func() tea.Msg {
		_ = appendPromptHistory(path, text)
		return nil
	}
}

// acceptCompletion splices the selected candidate into the input, replacing
// the text from the popup's anchor (the trigger char) up to the cursor, and
// leaves the caret at the end of the insertion. It does NOT submit (§4.2/§7):
// a second ⏎ runs the now-complete line. The popup is closed and the layout
// reserve recomputed afterward.
func (a *App) acceptCompletion() {
	sel, ok := a.completions.Selected()
	if !ok || sel.insert == "" {
		// No real candidate (empty match set) or a non-insertable placeholder row
		// such as the @ menu's "(indexing files…)": just close without splicing,
		// leaving the user's typed text (the trigger + query) intact.
		a.completions.Close()
		a.syncPopupLayout()
		return
	}
	val := a.input.Value()
	anchor := a.completions.AnchorStart()
	cursor := a.cursorByteOffset()
	if anchor < 0 || anchor > len(val) || cursor < anchor || cursor > len(val) {
		// Defensive: a stale anchor (buffer mutated out from under us) — just
		// close without splicing rather than corrupting the input.
		a.completions.Close()
		a.syncPopupLayout()
		return
	}
	next := val[:anchor] + sel.insert + val[cursor:]
	a.input.SetValue(next)
	// Park the caret right after the inserted text. We can't use a bare
	// SetCursorColumn: it counts within the *current* row only, so on a
	// multi-line draft (the prefix spans rows) it would land on the wrong row.
	// setCursorByteOffset derives the (row, col) for the prefix+insert boundary
	// and navigates there, so the single-line and multi-line cases both land
	// exactly between the insert and any trailing text.
	a.setCursorByteOffset(anchor + len(sel.insert))
	a.completions.Close()
	a.syncPopupLayout()
}

// setCursorByteOffset positions the textarea caret at byte offset off within
// the current Value(). It converts the offset to a (row, col) pair — the row is
// the count of newlines before off, the column is the rune width of the text
// after the last newline — then walks the caret to that row and sets the
// column. Defensively clamps off into range so a stale offset can't panic.
func (a *App) setCursorByteOffset(off int) {
	val := a.input.Value()
	if off < 0 {
		off = 0
	}
	if off > len(val) {
		off = len(val)
	}
	before := val[:off]
	targetRow := strings.Count(before, "\n")
	lineStart := strings.LastIndexByte(before, '\n') + 1 // 0 when no newline
	targetCol := len([]rune(before[lineStart:]))
	// After SetValue the caret sits at end-of-buffer (the last row); walk it to
	// the target row. The loops are no-ops at the edges, so either direction is
	// safe regardless of where the caret currently is.
	for a.input.Line() > targetRow {
		a.input.CursorUp()
	}
	for a.input.Line() < targetRow {
		a.input.CursorDown()
	}
	a.input.SetCursorColumn(targetCol)
}

// cursorByteOffset returns the byte offset of the textarea caret within
// Value(). Value() joins the per-row rune slices with '\n', so the offset is
// the bytes of every prior row (each plus one newline) followed by the bytes
// of the runes left of the caret on the cursor row. It clamps defensively so
// a transient row/column out of range can never index out of bounds.
func (a *App) cursorByteOffset() int {
	rows := strings.Split(a.input.Value(), "\n")
	row := a.input.Line()
	col := a.input.Column()
	if row < 0 {
		row = 0
	}
	if row >= len(rows) {
		row = len(rows) - 1
	}
	off := 0
	for i := 0; i < row; i++ {
		off += len(rows[i]) + 1 // +1 for the joining newline
	}
	rr := []rune(rows[row])
	if col > len(rr) {
		col = len(rr)
	}
	if col < 0 {
		col = 0
	}
	off += len(string(rr[:col]))
	return off
}

// syncCompletions (re)derives the inline popup from the post-keystroke input
// buffer (§5.1). It finds the token immediately left of the caret that begins
// with a trigger ('/' or '@') at a word boundary (input start or after
// whitespace) with no whitespace between the trigger and the caret. When such
// a token exists the popup is opened (or kept open) for that trigger with the
// right candidate set and filtered by the text after the trigger; otherwise it
// is closed. It then recomputes the layout reserve so the viewport makes room.
func (a *App) syncCompletions() {
	val := a.input.Value()
	cursor := a.cursorByteOffset()
	if cursor > len(val) {
		cursor = len(val)
	}
	left := val[:cursor]

	trigger, anchor, ok := triggerToken(left)
	if !ok {
		if a.completions.Active() {
			a.completions.Close()
		}
		a.syncPopupLayout()
		return
	}

	// (Re)open for this trigger when it changed or the popup is closed; the
	// candidate set is fixed per trigger, so we only rebuild it on (re)open and
	// just re-filter otherwise.
	if !a.completions.Active() || a.completions.Trigger() != trigger || a.completions.AnchorStart() != anchor {
		a.completions.Open(trigger, a.completionItems(trigger), anchor)
	}
	a.completions.SetQuery(left[anchor+1:]) // text after the trigger rune
	a.syncPopupLayout()
}

// completionItems returns the candidate rows for a trigger. '/' merges
// built-ins, custom commands, and discoverable skills via allCommands; '@'
// returns the cached repo-relative file paths (G4/§6.1), or a single
// "(indexing files…)" placeholder until the background walk's fileIndexMsg
// lands. The skill list is read nil-safely: a.agent and a.agent.Skills can
// both be nil in tests and in ephemeral mode.
func (a *App) completionItems(trigger rune) []complItem {
	if trigger == '@' {
		if !a.fileIndexReady {
			return fileIndexingItems()
		}
		return fileCompletionItems(a.fileIndex)
	}
	if trigger != '/' {
		return nil
	}
	var skillList []skills.Skill
	if a.agent != nil && a.agent.Skills != nil {
		skillList = a.agent.Skills.List()
	}
	cmds := allCommands(a.customCmds, skillList)
	items := make([]complItem, 0, len(cmds))
	for _, c := range cmds {
		label := "/" + c.Name
		if c.Kind == skillCmd {
			label += " (skill)"
		}
		items = append(items, complItem{
			insert: "/" + c.Name,
			label:  label,
			detail: c.Summary,
			kind:   c.Kind,
		})
	}
	return items
}

// syncPopupLayout re-lays-out so the viewport shrinks or grows to make room for
// the popup. Called whenever the popup opens, closes, or refilters. layout() is
// the single authority for the popup's reserved height: it caps the popup's
// rows against the terminal height (so a tall match set on a short terminal
// can't shove the input off-screen) and then sets a.popupLines from the clamped
// Lines(), so there is nothing to recompute here.
func (a *App) syncPopupLayout() {
	a.layout()
}

// triggerToken scans the text to the left of the caret for an inline-menu
// trigger ('/' or '@') that begins a token at a word boundary (the input
// start, or immediately after whitespace) with no whitespace between the
// trigger and the caret. It returns the trigger rune, its byte offset in
// left, and whether one was found. A trigger mid-word (e.g. the '/' in a
// path "a/b") is inert, matching §4.2's edge cases.
func triggerToken(left string) (rune, int, bool) {
	// Walk back from the caret to the start of the current whitespace-delimited
	// token. The token's first byte decides whether a menu should be open.
	start := strings.LastIndexAny(left, " \t\n")
	tokenStart := start + 1 // 0 when no whitespace precedes the token
	if tokenStart >= len(left) {
		return 0, 0, false // caret sits right after whitespace: no token
	}
	switch left[tokenStart] {
	case '/':
		return '/', tokenStart, true
	case '@':
		return '@', tokenStart, true
	}
	return 0, 0, false
}

// handleNormalKey handles keys in Normal (scroll) mode.
// Navigation keys (j/k/g/G/Ctrl+U/Ctrl+D) scroll the viewport.
// 'i' returns to Insert mode. Ctrl+R/T toggle reasoning.
// 'e' expands the last collapsed tool result.
// 'p' opens the pager for the last tool result.
// Printable characters auto-switch to Insert (handled in Update).
func (a *App) handleNormalKey(km tea.KeyPressMsg) (tea.Cmd, bool) {
	switch km.String() {
	case "j", "down":
		a.vp.ScrollDown(1)
		return nil, true
	case "k", "up":
		a.vp.ScrollUp(1)
		return nil, true
	case "ctrl+u":
		a.vp.HalfPageUp()
		return nil, true
	case "ctrl+d":
		a.vp.HalfPageDown()
		return nil, true
	case "g":
		a.vp.GotoTop()
		return nil, true
	case "G":
		a.vp.GotoBottom()
		return nil, true
	case "ctrl+r":
		a.toggleLastReasoning()
		return nil, true
	case "ctrl+t":
		a.toggleAllReasoning()
		return nil, true
	case "i":
		return a.setMode(modeInsert), true
	case "v":
		return a.enterVisual(), true
	case "e":
		if a.scrollback.ExpandLastResult() {
			a.refreshView()
		}
		return nil, true
	case "p":
		a.openLastResultPager()
		return nil, true
	case "P":
		// Capital P → external pager (less) over the full scrollback.
		// Terminal-native mouse-drag selection + scroll work across
		// the entire chat history while less owns the TTY.
		return a.openExternalPager(), true
	case "y":
		// Yank the most recent assistant text to the system clipboard.
		// On terminals without OSC 52 support this silently no-ops; we
		// still emit the info line so the user has feedback either way.
		// Shared with the palette's "yank" verb via yankLastAssistant.
		return a.yankLastAssistant(), true
	case "?":
		// Help overlay (G7), Normal mode only. In Insert mode `?` stays a
		// literal character (handleInsertKey never maps it), so typing prose
		// is unaffected — this branch is reached only when the input is
		// blurred. Intercept BEFORE the printable fallthrough below so `?`
		// opens help instead of auto-switching to Insert.
		return a.openHelp(), true
	case "esc":
		// Already in Normal mode; no-op.
		return nil, true
	}

	// Printable characters: switch to Insert and let the key flow
	// through to textarea (handled by the caller in Update).
	if isPrintableKey(km) {
		return nil, false
	}

	return nil, true // swallow non-printable keys in Normal mode
}

// openLastResultPager opens the pager for the most recent tool result.
func (a *App) openLastResultPager() {
	if tool, content, ok := a.scrollback.LastToolResult(); ok {
		a.openPager(tool, content)
	}
}

// hasShiftEnter detects "shift+enter" across bubbletea's key-string
// variants. Terminals report it inconsistently, so we just look for the
// "shift" substring.
func hasShiftEnter(km tea.KeyPressMsg) bool {
	return strings.Contains(strings.ToLower(km.String()), "shift")
}

// formatReloadResult renders a /reload-skills outcome for the chat log. It
// distinguishes a real, prefix-moving reload (one deliberate cache miss next
// turn) from a no-op or a disk-only change that left the cached prefix intact.
func formatReloadResult(res agent.ReloadResult) string {
	var added, removed, changed int
	for _, c := range res.Changes {
		switch c.Kind {
		case "added":
			added++
		case "removed":
			removed++
		case "body_changed":
			changed++
		}
	}
	if !res.FingerprintMoved {
		if added+removed+changed == 0 {
			return "skills reloaded — no changes (prefix unchanged, no cache miss)"
		}
		return fmt.Sprintf("skills reloaded: +%d -%d ~%d (prefix unchanged, no cache miss)", added, removed, changed)
	}
	return fmt.Sprintf("skills reloaded: +%d added, -%d removed, ~%d changed — new prefix epoch minted (one cache miss expected next turn)", added, removed, changed)
}

// handleSlash dispatches /commands. Unknown slash commands are echoed
// to the chat for visibility.
func (a *App) handleSlash(line string) tea.Cmd {
	fields := strings.Fields(line)
	cmd := fields[0]
	switch cmd {
	case "/quit", "/exit", "/q":
		return tea.Quit
	case "/help", "/?":
		// Open the structured, scrollable help overlay (G7) instead of dumping
		// a static block into the scrollback. Its content is generated from the
		// shared command registry + a static keybinding table so it can't drift.
		return a.openHelp()
	case "/clear":
		a.scrollback.Clear()
		a.refreshView()
	case "/models":
		// Direct switch: /models <id>
		if len(fields) >= 2 {
			return a.applyModelSwitch(fields[1])
		}
		a.overlay.OpenModels(a.model)
	case "/effort":
		if len(fields) < 2 {
			// Show current effort and allowed values.
			cur := a.agent.ReasoningEffort
			if !cur.Valid() {
				cur = llm.ReasoningEffortMax
			}
			a.scrollback.AppendInfo(fmt.Sprintf("effort: %s (allowed: low, medium, high, max)", cur))
			a.refreshView()
			return nil
		}
		// Reject while running.
		a.runMu.Lock()
		running := a.running
		a.runMu.Unlock()
		if running {
			a.scrollback.AppendInfo("/effort unavailable while the agent is running — retry when idle")
			a.refreshView()
			return nil
		}
		newEffort, ok := llm.ParseReasoningEffort(fields[1])
		if !ok {
			a.scrollback.AppendError(fmt.Sprintf("invalid effort %q; allowed: low, medium, high, max", fields[1]))
			a.refreshView()
			return nil
		}
		old := a.agent.ReasoningEffort
		a.agent.ReasoningEffort = newEffort
		a.status.reasoningEffort = newEffort
		a.scrollback.AppendInfo(fmt.Sprintf("effort: %s -> %s", old, newEffort))
		a.refreshView()
	case "/cost":
		report := RenderCostReport(CostReportInput{
			Model:        a.status.model,
			Steps:        a.status.steps,
			Usage:        a.status.usage,
			CostYuan:     a.status.costYuan,
			SavedYuan:    a.status.savedYuan,
			ContextLimit: a.status.contextLimit,
			CostKnown:    a.status.costKnown,
			Balance:      a.status.balanceNote,
		})
		a.scrollback.AppendInfo(report)
		a.refreshView()
	case "/theme":
		return a.openThemes()
	case "/tape":
		a.overlay.OpenTape()
	case "/sessions":
		if a.session.list == nil {
			a.scrollback.AppendInfo("/sessions unavailable (no persistence wired)")
			a.refreshView()
			return nil
		}
		sessList, err := a.session.list()
		if err != nil {
			a.scrollback.AppendError("loading sessions: " + err.Error())
			a.refreshView()
			return nil
		}
		rows := make([]sessionRow, 0, len(sessList))
		for _, s := range sessList {
			rows = append(rows, sessionRow{Sess: s})
		}
		a.overlay.OpenSessions(rows)
	case "/mcp":
		if a.mcpStatus == nil {
			a.scrollback.AppendInfo("/mcp unavailable (no MCP servers configured)")
			a.refreshView()
			return nil
		}
		a.overlay.OpenMCP(a.mcpStatus())
	case "/lsp":
		if a.lspStatus == nil {
			a.scrollback.AppendInfo("/lsp unavailable (no LSP servers detected)")
			a.refreshView()
			return nil
		}
		a.overlay.OpenLSP(a.lspStatus())
	case "/permissions":
		if a.permStatus == nil {
			a.scrollback.AppendInfo("/permissions unavailable (no policy loaded)")
			a.refreshView()
			return nil
		}
		a.overlay.OpenPermissions(a.permStatus())
	case "/export", "/scrollback":
		// Same as `P` in Normal mode — dump scrollback to $PAGER so
		// the user gets terminal-native drag-select across the whole
		// session.
		return a.openExternalPager()
	case "/undo":
		n := 1
		if len(fields) >= 2 {
			if parsed := parsePositiveInt(fields[1]); parsed > 0 {
				n = parsed
			}
		}
		a.applyUndo(n)
	case "/reload-skills", "/reload":
		// Re-scan skills mid-session. The agent is not concurrency-safe, and
		// ReloadSkills mutates a.System / the shared store / the epoch state. The
		// shared skill store is also read by skill_read inside a still-live async
		// subagent, which outlives the main loop — so refuse while EITHER the main
		// loop (a.running) or any background job is in flight, not just the former.
		a.runMu.Lock()
		running := a.running
		a.runMu.Unlock()
		if running || a.agent.HasActiveBackgroundWork() {
			return a.toast(BadgeWarn, "/reload-skills unavailable while the agent or a background job is running — retry when idle")
		}
		home, _ := os.UserHomeDir()
		// A reload failure is a durable error worth scrolling back to; the
		// success summary is ephemeral, so it rides a toast (G8/§6.5).
		if res, err := a.agent.ReloadSkills(a.cwd, home); err != nil {
			a.scrollback.AppendError("reload-skills: " + err.Error())
			a.refreshView()
			return nil
		} else {
			return a.toast(BadgeOk, formatReloadResult(res))
		}
	case "/compact":
		// Force a compaction now, regardless of the auto threshold.
		// Runs off the UI goroutine so a slow ReplaceWithCompaction
		// transaction can't freeze the input loop.
		go a.agent.ForceCompact(context.Background())
		return a.toast(BadgeInfo, "compaction requested")
	default:
		if cmd := a.lookupCustomCommand(line); cmd != nil {
			return cmd
		}
		// A bare /<skill-name> that is neither a builtin nor a custom command
		// is a guided skill invocation (§4.1): prefill the input with a
		// directive and leave the cursor at the end so the user finishes the
		// sentence. Skills aren't directly user-invocable — this makes them
		// discoverable from / without a hidden behavior change.
		if a.prefillSkillDirective(strings.TrimPrefix(cmd, "/")) {
			return nil
		}
		// An unknown command is an ephemeral mistake, not durable content — so
		// it surfaces as a toast (G8/§6.5) rather than polluting the scrollback.
		return a.toast(BadgeWarn, "unknown command: "+cmd+" (/help for list)")
	}
	return nil
}

// prefillSkillDirective handles a bare /<name> that names a known skill but is
// not a builtin/custom command (§4.1/§10). It sets the input to a guided
// directive and parks the caret at the end, staying in Insert mode so the user
// can complete the prompt and submit. Returns false when name is not a skill
// (nil-safe on a.agent / a.agent.Skills) so the caller can fall back to the
// unknown-command path.
func (a *App) prefillSkillDirective(name string) bool {
	if a.agent == nil || a.agent.Skills == nil {
		return false
	}
	if _, ok := a.agent.Skills.Body(name); !ok {
		return false
	}
	directive := "use the \"" + name + "\" skill for this: "
	a.input.SetValue(directive)
	a.input.SetCursorColumn(len([]rune(directive)))
	a.completions.Close()
	a.syncPopupLayout()
	return true
}

// lookupCustomCommand checks if the slash line matches a user-defined
// command. If so, it returns a tea.Cmd that expands the template off
// the UI goroutine (safe for !`cmd` shell injection). Returns nil if
// no match.
func (a *App) lookupCustomCommand(line string) tea.Cmd {
	sp := strings.IndexByte(line, ' ')
	var name, argline string
	if sp < 0 {
		name = strings.TrimPrefix(line, "/")
	} else {
		name = strings.TrimPrefix(line[:sp], "/")
		argline = line[sp+1:]
	}
	cmd, ok := a.customCmds[name]
	if !ok {
		return nil
	}
	args := commands.Tokenize(argline)
	if cmd.Model != "" {
		a.agent.Model = cmd.Model
		a.model = cmd.Model
		a.status.model = cmd.Model
		a.scrollback.AppendInfo("model → " + cmd.Model + " (via /" + name + ")")
	}
	tmpl := cmd.Template
	return func() tea.Msg {
		readFile := func(path string) (string, error) {
			data, err := os.ReadFile(path)
			return string(data), err
		}
		text, _ := commands.Expand(context.Background(), tmpl, args, shellRunner, readFile)
		if cmd.Agent != "" || cmd.Subtask {
			return spawnExpandedMsg{text: text, agent: cmd.Agent}
		}
		return slashExpandedMsg{text: text}
	}
}

// spawnCmd starts a sub-agent spawn in a goroutine. The result is
// delivered as a spawnResultMsg. If no spawner is configured, the
// expanded text is submitted to the main agent loop as a fallback.
func (a *App) spawnCmd(text, agentName string) tea.Cmd {
	return func() tea.Msg {
		sp := a.agent.Spawner
		if sp == nil {
			// No spawner configured; fall back to main loop.
			go a.runAgent(text)
			return runStartMsg{}
		}
		go func() {
			res, _ := sp.Spawn(context.Background(), tools.SpawnRequest{
				Agent:       agentName,
				Description: text,
			})
			summary := res.Summary
			if summary == "" {
				summary = "(subagent returned no summary)"
			}
			a.send(spawnResultMsg{summary: summary})
		}()
		return runStartMsg{}
	}
}

// shellRunner executes a shell command with a 10s timeout. Stderr is
// discarded to avoid polluting the TUI's alt-screen.
func shellRunner(ctx context.Context, cmd string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	c := exec.CommandContext(ctx, "bash", "-c", cmd)
	c.Stderr = io.Discard
	out, err := c.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimRight(string(out), "\n"), nil
}

// handlePermissionKey processes a single keystroke while the permission
// modal is up. Nav keys move the focus within the 2×2 grid; resolution
// keys (y/s/a/n/enter) commit a decision via PermissionFlow.Resolve.
func (a *App) handlePermissionKey(km tea.KeyPressMsg) tea.Cmd {
	if !a.permission.Active() {
		return nil
	}
	switch strings.ToLower(km.String()) {
	case "up", "k":
		a.permission.MoveUp()
		return nil
	case "down", "j":
		a.permission.MoveDown()
		return nil
	case "left", "h":
		a.permission.MoveLeft()
		return nil
	case "right", "l":
		a.permission.MoveRight()
		return nil
	case "tab":
		a.permission.Tab()
		return nil
	case "shift+tab":
		a.permission.ShiftTab()
		return nil
	}
	resp, reply, check, ok := a.permission.Resolve(km.String())
	if !ok {
		// Swallow — modal modes must not leak keys to the textarea
		// even when the user mashes something unmapped.
		return nil
	}
	focusCmd := a.setMode(modeInsert)
	a.layout()
	a.scrollback.AppendInfo(fmt.Sprintf("permission %s for %s", decisionLabel(resp), check.Tool.Name()))
	a.refreshView()
	go func() { reply <- resp }()
	return focusCmd
}

func decisionLabel(r agent.PermissionResponse) string {
	if !r.Allow {
		return "denied"
	}
	if r.PersistPattern {
		return "always-allowed"
	}
	return "allowed"
}

// handleQuestionKey processes a single keystroke while the question
// modal is up.
func (a *App) handleQuestionKey(km tea.KeyPressMsg) tea.Cmd {
	if !a.question.Active() {
		return nil
	}
	qu := a.question.current()
	switch strings.ToLower(km.String()) {
	case "up", "k":
		a.question.MoveDelta(-1)
		return nil
	case "down", "j":
		a.question.MoveDelta(1)
		return nil
	case " ":
		if qu.Multiple {
			a.question.Toggle()
		}
		return nil
	case "enter":
		done, resp, reply := a.question.Next()
		if !done {
			return nil
		}
		focusCmd := a.setMode(modeInsert)
		a.layout()
		a.refreshView()
		go func() { reply <- resp }()
		return focusCmd
	case "esc":
		reply := a.question.Cancel()
		focusCmd := a.setMode(modeInsert)
		a.layout()
		a.refreshView()
		if reply != nil {
			go func() { reply <- tools.QuestionResponse{} }()
		}
		return focusCmd
	}
	return nil
}

// toggleLastReasoning expands/collapses the most recent reasoning block.
func (a *App) toggleLastReasoning() {
	a.scrollback.ToggleLastReasoning()
	a.refreshView()
}

// toggleAllReasoning expands/collapses every reasoning block (all to
// same target state — collapse if any are open, else expand).
func (a *App) toggleAllReasoning() {
	a.scrollback.ToggleAllReasoning()
	a.refreshView()
}

// handleOverlayKey processes keys in the fullscreen overlays. The filterable
// overlays (/models, /sessions, palette) route printable characters into the
// shared filter and use ↑/↓ (not j/k, which are now filter input) for
// navigation; esc clears a non-empty filter before it closes. modeTape and
// modeHelp keep j/k navigation and a plain esc/q close.
func (a *App) handleOverlayKey(km tea.KeyPressMsg) tea.Cmd {
	if a.overlay.Filterable() {
		return a.handleFilterableOverlayKey(km)
	}

	switch km.String() {
	case "esc", "q":
		a.overlay.Close()
		return nil
	case "ctrl+c":
		return tea.Quit
	case "tab", "right", "l":
		if a.overlay.Mode() == modeHelp {
			a.overlay.NextHelpTab()
		}
		return nil
	case "shift+tab", "left", "h":
		if a.overlay.Mode() == modeHelp {
			a.overlay.PrevHelpTab()
		}
		return nil
	case "j", "down":
		a.overlay.MoveDown()
	case "k", "up":
		a.overlay.MoveUp()
	case "enter":
		// modeTape / modeHelp have no enter action today; modeHelp closes.
		if a.overlay.Mode() == modeHelp {
			a.overlay.Close()
		}
	}
	return nil
}

// handleFilterableOverlayKey drives the filterable overlays (/models,
// /sessions, palette, /theme): ↑/↓ navigate the narrowed set, ⏎ selects/executes,
// a printable rune extends the filter, backspace trims it, and esc clears a
// non-empty filter before a second esc closes. For modeThemes, cursor/filter
// changes trigger live preview; esc on an empty filter restores the original theme.
func (a *App) handleFilterableOverlayKey(km tea.KeyPressMsg) tea.Cmd {
	switch km.String() {
	case "ctrl+c":
		return tea.Quit
	case "esc":
		if a.overlay.FilterClear() {
			// Filter was non-empty and is now cleared; re-preview the current
			// selection so the theme matches the now-visible row.
			if a.overlay.Mode() == modeThemes {
				a.previewTheme(a.overlay.SelectedThemeID())
			}
			return nil // first esc clears the filter; a second one closes
		}
		// Empty filter: restore original theme on cancel, then close.
		if a.overlay.Mode() == modeThemes {
			a.setActiveTheme(a.themePreviewOrig.Name)
		}
		a.overlay.Close()
		return nil
	case "up":
		a.overlay.MoveUp()
		a.maybePreviewTheme()
		return nil
	case "down":
		a.overlay.MoveDown()
		a.maybePreviewTheme()
		return nil
	case "backspace":
		a.overlay.FilterBackspace()
		a.maybePreviewTheme()
		return nil
	case "enter":
		return a.acceptOverlaySelection()
	}
	if isPrintableKey(km) {
		a.overlay.FilterType(rune(km.String()[0]))
		a.maybePreviewTheme()
		return nil
	}
	return nil
}

// maybePreviewTheme triggers a live preview if the active overlay is modeThemes.
func (a *App) maybePreviewTheme() {
	if a.overlay.Mode() == modeThemes {
		a.previewTheme(a.overlay.SelectedThemeID())
	}
}

// acceptOverlaySelection commits the cursor row of a filterable overlay: a
// model switch, a session-switch hint, or a palette action. It closes the
// overlay first so a palette verb that opens another overlay (e.g. /models)
// lands cleanly.
func (a *App) acceptOverlaySelection() tea.Cmd {
	switch a.overlay.Mode() {
	case modeModels:
		id := a.overlay.SelectedModelID()
		a.overlay.Close()
		if id != "" {
			return a.applyModelSwitch(id)
		}
	case modeSessions:
		// Session switching is a wave-6 polish — for v0.1 it just records
		// intent and asks the user to restart with -r <id>.
		id := a.overlay.SelectedSessionID()
		a.overlay.Close()
		if id != "" {
			a.scrollback.AppendInfo("to switch sessions, restart with: dsc -r " + id)
			a.refreshView()
		}
	case modePalette:
		act, ok := a.overlay.SelectedAction()
		a.overlay.Close()
		if ok {
			return a.runPaletteAction(act.id)
		}
	case modeThemes:
		id := a.overlay.SelectedThemeID()
		a.overlay.Close()
		if id != "" {
			return a.applyThemeSwitch(id)
		}
	}
	return nil
}

// helpCommandRows is the command half of the help overlay (G7), drawn from the
// same allCommands merge that feeds the / menu so the two can never drift.
// nil-safe on a.agent / a.agent.Skills (test fixtures, ephemeral mode).
func (a *App) helpCommandRows() []slashCmd {
	var skillList []skills.Skill
	if a.agent != nil && a.agent.Skills != nil {
		skillList = a.agent.Skills.List()
	}
	return allCommands(a.customCmds, skillList)
}

// openHelp puts up the help overlay (G7), blurring the input so its keys don't
// leak to the textarea. Returns the focus Cmd from the mode switch.
func (a *App) openHelp() tea.Cmd {
	cmd := a.setMode(modeNormal)
	a.overlay.OpenHelp()
	return cmd
}

// openThemes puts up the /theme picker with live preview. It snapshots the
// current theme so Esc can restore it exactly.
func (a *App) openThemes() tea.Cmd {
	cmd := a.setMode(modeNormal)
	a.themePreviewOrig = a.theme
	a.overlay.OpenThemes(a.theme.Name)
	return cmd
}

// openPalette puts up the command palette (G5) over every action: the merged
// slash commands plus the non-slash verbs. It switches to Normal mode so the
// textarea is blurred while the modal owns the keys.
func (a *App) openPalette() tea.Cmd {
	cmd := a.setMode(modeNormal)
	a.overlay.OpenPalette(a.paletteActions())
	return cmd
}

// paletteActions builds the palette row set (G5): every merged slash command
// (built-ins + custom + skills) rendered as a "/name" verb, plus the extra
// verbs that have no slash today (toggle thinking, open help, yank). The
// slash-backed rows route through handleSlash on accept; the extra verbs call
// the same handler their key would. Built so the palette is a superset of the
// / menu and never drifts from it.
func (a *App) paletteActions() []paletteAction {
	cmds := a.helpCommandRows()
	out := make([]paletteAction, 0, len(cmds)+3)
	for _, c := range cmds {
		tag := ""
		switch c.Kind {
		case skillCmd:
			tag = " (skill)"
		case customCmd:
			tag = " (custom)"
		}
		out = append(out, paletteAction{
			id:     "/" + c.Name,
			label:  "/" + c.Name + tag,
			detail: c.Summary,
		})
	}
	// Non-slash verbs: these have a key today but no slash command, so the
	// palette is their discoverable home. id values are distinct from any
	// "/name" so runPaletteAction routes them to the same handler the key does.
	out = append(out,
		paletteAction{id: "toggle-thinking", label: "toggle thinking", detail: "flip per-turn reasoning on/off"},
		paletteAction{id: "open-help", label: "open help", detail: "show the keybinding + command overlay"},
		paletteAction{id: "yank-last", label: "yank last assistant text", detail: "copy the last reply to the clipboard (OSC 52)"},
	)
	// /theme is a slash command but also lives in the palette for discoverability.
	out = append(out, paletteAction{
		id: "/theme", label: "/theme", detail: "switch the color theme (live preview)",
	})
	return out
}

// runPaletteAction executes a chosen palette row by its stable id. Slash-backed
// ids ("/name") route through handleSlash exactly as if typed; the non-slash
// verbs call the same handler their key would. Returns the tea.Cmd the action
// produces, if any.
func (a *App) runPaletteAction(id string) tea.Cmd {
	switch id {
	case "toggle-thinking":
		a.toggleThinking()
		return nil
	case "open-help":
		return a.openHelp()
	case "yank-last":
		return a.yankLastAssistant()
	}
	if strings.HasPrefix(id, "/") {
		return a.handleSlash(id)
	}
	return nil
}

// toggleThinking flips the per-turn reasoning flag used by the agent on the
// next turn (the palette's "toggle thinking" verb, G5). Keeps the App field
// and the agent field in lockstep and surfaces a notice so the change is
// visible. nil-safe on a.agent (test fixtures).
func (a *App) toggleThinking() {
	a.thinking = !a.thinking
	if a.agent != nil {
		a.agent.Thinking = a.thinking
	}
	state := "off"
	if a.thinking {
		state = "on"
	}
	a.scrollback.AppendInfo("thinking → " + state)
	a.refreshView()
}

// yankLastAssistant copies the most recent assistant text to the system
// clipboard (the palette's "yank" verb and Normal-mode `y`). The yank outcome
// is an ephemeral notice, so it rides a toast (G8/§6.5) instead of the
// scrollback. Returns a tea.Cmd batching the OSC-52 clipboard write with the
// toast's expiry tick (or just the toast tick when there is nothing to yank).
func (a *App) yankLastAssistant() tea.Cmd {
	text := a.scrollback.LastAssistantText()
	if text == "" {
		return a.toast(BadgeWarn, "nothing to yank (no assistant text yet)")
	}
	toastCmd := a.toast(BadgeOk, fmt.Sprintf("yanked %d bytes to clipboard (OSC 52)", len(text)))
	return tea.Batch(yankToClipboardCmd(text), toastCmd)
}

// applyModelSwitch updates the live model used for the next turn and
// (when persistence is wired) records it to the session row. The success
// notice is ephemeral, so it rides a toast (G8/§6.5) rather than the
// scrollback; the returned tea.Cmd schedules that toast's expiry (nil when no
// toast was shown). Durable signals — an unknown-model error and a
// persist-failure warning — stay in the scrollback so they can be scrolled
// back to.
func (a *App) applyModelSwitch(id string) tea.Cmd {
	if id == "" {
		return nil
	}
	known := false
	for _, m := range availableModels() {
		if m.ID == id {
			known = true
			break
		}
	}
	if !known {
		a.scrollback.AppendError("unknown model: " + id)
		a.refreshView()
		return nil
	}
	a.model = id
	a.status.model = id
	a.agent.Model = id
	a.scrollback.SetModel(id) // tool cards appended after the switch get the new accent
	if a.session.setModel != nil {
		if err := a.session.setModel(id); err != nil {
			a.scrollback.AppendInfo("model switched (warning: persist failed: " + err.Error() + ")")
			a.refreshView()
			return nil
		}
	}
	return a.toast(BadgeBrand, "model → "+id)
}

// setActiveTheme swaps the running theme by id, preserving the current
// transparent/truecolor flags, re-applies the textarea cursor color,
// invalidates the render cache, and refreshes the viewport.
func (a *App) setActiveTheme(id string) {
	orig := a.theme
	a.theme = themeByID(id).WithTransparent(orig.Transparent()).WithTruecolor(orig.Truecolor())
	st := a.input.Styles()
	st.Cursor.Color = a.theme.BrandDeep
	a.input.SetStyles(st)
	a.scrollback.InvalidateRenderCache()
	a.refreshView()
}

// previewTheme applies a theme live without persisting (used by the picker
// cursor). No-op on empty id.
func (a *App) previewTheme(id string) {
	if id != "" {
		a.setActiveTheme(id)
	}
}

// applyThemeSwitch commits a theme: validates id against availableThemes,
// setActiveTheme, persists via a.session.setTheme (nil-safe; error => non-fatal
// info line), returns a toast Cmd.
func (a *App) applyThemeSwitch(id string) tea.Cmd {
	if id == "" {
		return nil
	}
	known := false
	for _, th := range availableThemes() {
		if th.ID == id {
			known = true
			break
		}
	}
	if !known {
		a.scrollback.AppendError("unknown theme: " + id)
		a.refreshView()
		return nil
	}
	a.setActiveTheme(id)
	if a.session.setTheme != nil {
		if err := a.session.setTheme(id); err != nil {
			a.scrollback.AppendInfo("theme switched (warning: persist failed: " + err.Error() + ")")
			a.refreshView()
			return nil
		}
	}
	return a.toast(BadgeBrand, "theme → "+id)
}

// applyUndo reverts the last n snapshot steps.
func (a *App) applyUndo(n int) {
	if a.session.undo == nil {
		a.scrollback.AppendInfo("/undo unavailable (no snapshots wired)")
		a.refreshView()
		return
	}
	// The transcript reconcile below mutates a.Messages, which the agent
	// goroutine reads while running — refuse mid-turn (T3.5). Read the flag
	// under the lock, matching the ctrl+c handler.
	a.runMu.Lock()
	running := a.running
	a.runMu.Unlock()
	if running {
		a.scrollback.AppendInfo("/undo unavailable while the agent is running")
		a.refreshView()
		return
	}
	restored, err := a.session.undo(n)
	if err != nil {
		a.scrollback.AppendError("/undo: " + err.Error())
		a.refreshView()
		return
	}
	msg := fmt.Sprintf("/undo restored %d file(s) across %d step(s)", restored, n)
	// Reconcile the transcript so the model's view matches the reverted files
	// (T3.5). The files are already reverted; surface a reconcile failure but
	// do not try to roll them back.
	if a.agent != nil {
		switch removed, rerr := a.agent.ReconcileUndo(context.Background(), n); {
		case rerr != nil:
			msg += "; transcript NOT reconciled: " + rerr.Error()
		case removed > 0:
			msg += fmt.Sprintf("; rewound %d message(s)", removed)
		case restored > 0:
			// Files reverted but no step history to rewind (e.g. a resumed
			// session, whose in-memory step records start empty). Warn so the
			// file/model divergence is not silent.
			msg += "; transcript NOT reconciled (no step history this session — resumed?); the model's view may now disagree with the reverted files"
		}
	}
	a.scrollback.AppendInfo(msg)
	a.refreshView()
}

func parsePositiveInt(s string) int {
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0
		}
		n = n*10 + int(r-'0')
	}
	return n
}
