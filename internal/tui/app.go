package tui

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/amemiya02/deepseekcode/internal/agent"
	"github.com/amemiya02/deepseekcode/internal/llm"
	"github.com/amemiya02/deepseekcode/internal/permissions"
	"github.com/amemiya02/deepseekcode/internal/session"
	"github.com/amemiya02/deepseekcode/internal/tools"
)

// App is the root Bubble Tea Model.
//
// One App per running TUI process. The Agent runs in a goroutine
// owned by the App; its Callbacks Send() typed messages back to the
// program via App.send (set in Start).
type App struct {
	// Wiring
	agent    *agent.Agent
	model    string
	thinking bool
	cwd      string

	// UI state
	theme  Theme
	keymap keymap
	vp     viewport.Model
	input  textarea.Model
	width  int
	height int
	mode   appMode

	// Chat scrollback. Owns items, in-progress stream cursors, the
	// rendered-line cache, and visual-mode selection. Mutations only
	// through its methods — direct slice access is read-only.
	scrollback *Scrollback
	thinkStart time.Time // mirrored for chrome's startedAt

	// Per-run cancellation
	runMu     sync.Mutex
	runCtx    context.Context
	runCancel context.CancelFunc
	running   bool

	// Live activity band rendered between viewport and status.
	chrome     chromeState
	dirty      bool // a delta arrived; redraw on next tick
	tickActive bool // a redrawMsg tick is scheduled

	// Pager overlay (modal; takes the body band when non-nil).
	pager *pagerState

	// Status
	status    statusState
	stepTotal int

	// Permission asks
	pendingPerm *permissionAskMsg
	// permCursor indexes into the 2×2 button grid:
	//   0 = allow once   1 = session
	//   2 = always       3 = deny
	// Reset to 0 on each new permissionAskMsg so the default focus is
	// the safest fast-path: allow once.
	permCursor int

	// Overlays (modeTape / modeModels / modeSessions)
	overlay       overlayMode
	overlayCursor int
	models        []modelOption
	sessionsRows  []sessionRow

	// Session/snapshot integration. May be nil; the TUI degrades to
	// ephemeral mode (no /undo, no /sessions, no resume).
	sessionID    string
	undoFn       func(n int) (int, error)
	listSessions func() ([]session.Session, error)
	setModelFn   func(model string) error

	// Wiring back to the tea.Program for callbacks running off the UI loop
	send func(tea.Msg)

	// Notices shown once at startup (resume confirmation, warnings).
	startupNotices []string
}

// Config bundles construction params for New.
type Config struct {
	Agent    *agent.Agent
	Model    string
	Thinking bool
	Theme    string
	Cwd      string

	// Optional persistence integration. Provide all three or none.
	SessionID    string
	UndoFn       func(n int) (int, error)
	ListSessions func() ([]session.Session, error)
	SetModelFn   func(model string) error

	// StartupNotices are shown as info chat items at TUI start. Used for
	// resume confirmations, warnings about degraded persistence, etc. —
	// anything the caller would have written to stderr in CLI mode.
	StartupNotices []string
}

// New constructs an App. The returned App is a tea.Model; pass it to
// tea.NewProgram and call .Run().
func New(cfg Config) *App {
	ta := textarea.New()
	ta.Placeholder = "Ask anything…  (⏎ send · ⇧⏎ newline · ^D quit · /help)"
	ta.Prompt = "› "
	ta.SetWidth(80)
	ta.SetHeight(3)
	ta.ShowLineNumbers = false
	ta.CharLimit = 0
	ta.Focus()

	vp := viewport.New(80, 20)
	vp.MouseWheelEnabled = true

	app := &App{
		agent:          cfg.Agent,
		model:          cfg.Model,
		thinking:       cfg.Thinking,
		cwd:            cfg.Cwd,
		theme:          PickTheme(cfg.Theme),
		keymap:         defaultKeymap(),
		vp:             vp,
		input:          ta,
		scrollback:     NewScrollback(),
		sessionID:      cfg.SessionID,
		undoFn:         cfg.UndoFn,
		listSessions:   cfg.ListSessions,
		setModelFn:     cfg.SetModelFn,
		startupNotices: cfg.StartupNotices,
		status: statusState{
			model:    cfg.Model,
			thinking: cfg.Thinking,
		},
	}
	app.wireCallbacks()
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
	prog := tea.NewProgram(a, tea.WithAltScreen(), tea.WithMouseCellMotion())
	a.send = prog.Send
	_, err := prog.Run()
	return err
}

// wireCallbacks plugs the agent's Callbacks into program.Send. The
// callbacks all run on the agent goroutine; they translate events to
// typed messages and hand them to the UI thread.
func (a *App) wireCallbacks() {
	a.agent.Callbacks = agent.Callbacks{
		OnReasoningStart: func() { a.send(reasoningStartMsg{}) },
		OnReasoningDelta: func(s string) { a.send(reasoningDeltaMsg{Text: s}) },
		OnReasoningEnd:   func() { a.send(reasoningEndMsg{}) },
		OnTextDelta:      func(s string) { a.send(textDeltaMsg{Text: s}) },
		OnToolCallStart:  func(c llm.ToolCall) { a.send(toolCallStartMsg{Call: c}) },
		OnToolCallResult: func(id string, r tools.Result, d time.Duration) {
			a.send(toolCallResultMsg{CallID: id, Result: r, Dur: d})
		},
		OnDuetValidation: func(id string, ok bool, reason string, d time.Duration) {
			a.send(duetMsg{CallID: id, Approved: ok, Reasoning: reason, Dur: d})
		},
		OnStepFinish: func(reason agent.StopReason, u llm.Usage) {
			a.send(stepFinishMsg{Reason: reason, Usage: u})
		},
		OnInfo: func(s string) { a.send(infoMsg{Text: s}) },
		OnPermissionAsk: func(check permissions.Check) agent.PermissionResponse {
			reply := make(chan agent.PermissionResponse, 1)
			a.send(permissionAskMsg{Check: check, Reply: reply})
			return <-reply
		},
	}
}

// runAgent runs one prompt turn in the goroutine spawned by submitPromptCmd.
func (a *App) runAgent(prompt string) {
	ctx, cancel := context.WithCancel(context.Background())

	a.runMu.Lock()
	a.runCtx = ctx
	a.runCancel = cancel
	a.running = true
	a.runMu.Unlock()

	reason, err := a.agent.Run(ctx, prompt)

	a.runMu.Lock()
	a.runCancel = nil
	a.runCtx = nil
	a.running = false
	a.runMu.Unlock()

	if err != nil {
		a.send(agentErrMsg{Err: err})
	}
	a.send(agentDoneMsg{Reason: reason, Err: err})
}

// Init satisfies tea.Model. The first scrollback entry is an ASCII
// welcome banner (whale mascot + DEEPSEEKCODE wordmark); startup
// notices like resume confirmations follow.
func (a *App) Init() tea.Cmd {
	a.scrollback.AppendWelcome()
	for _, n := range a.startupNotices {
		a.scrollback.AppendInfo(n)
	}
	return textarea.Blink
}

// Update folds incoming messages into the model.
func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	// Mode-based key routing: Permission and Pager modes are modal and
	// eat all key input until dismissed. Handled here at the top so the
	// per-mode switch below stays simple.
	if a.mode == modePermission {
		if km, ok := msg.(tea.KeyMsg); ok {
			return a, a.handlePermissionKey(km)
		}
	}

	switch m := msg.(type) {
	case tea.WindowSizeMsg:
		a.width = m.Width
		a.height = m.Height
		a.layout()
	case tea.KeyMsg:
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
		a.chrome.active = true
		a.chrome.phase = phaseThinking
		a.chrome.activeTool = ""
		a.chrome.tokens = 0
		a.chrome.startedAt = time.Now()
		cmds = append(cmds, a.ensureTick())
	case redrawMsg:
		// Coalesced re-render: deltas flag a.dirty; we rebuild the
		// viewport at most once per tick. Spinner frame advances every
		// tick regardless so the indicator animates smoothly.
		a.chrome.frame++
		if a.dirty {
			a.refreshView()
			a.dirty = false
		}
		if a.chrome.active || a.running {
			cmds = append(cmds, a.scheduleRedraw())
		} else {
			a.tickActive = false
		}
	case reasoningStartMsg:
		a.thinkStart = time.Now()
		a.chrome.active = true
		a.chrome.phase = phaseThinking
		a.chrome.startedAt = a.thinkStart
		a.chrome.tokens = 0
		a.scrollback.StartReasoning()
		a.refreshView()
		cmds = append(cmds, a.ensureTick())
	case reasoningDeltaMsg:
		a.chrome.tokens = a.scrollback.AppendReasoning(m.Text)
		a.dirty = true
	case reasoningEndMsg:
		a.scrollback.EndReasoning()
	case textDeltaMsg:
		created, toks := a.scrollback.AppendText(m.Text)
		if created {
			a.chrome.phase = phaseWriting
			a.chrome.startedAt = time.Now()
			a.chrome.tokens = 0
			a.refreshView()
			cmds = append(cmds, a.ensureTick())
		}
		a.chrome.tokens = toks
		a.dirty = true
	case toolCallStartMsg:
		a.chrome.phase = phaseTool
		a.chrome.activeTool = m.Call.Function.Name
		a.chrome.startedAt = time.Now()
		a.scrollback.AppendToolCall(m.Call.ID, m.Call.Function.Name, m.Call.Function.Arguments)
		a.refreshView()
		cmds = append(cmds, a.ensureTick())
	case toolCallResultMsg:
		a.scrollback.AppendToolResult(m.CallID, m.Result, m.Dur)
		a.refreshView()
	case duetMsg:
		a.scrollback.AppendDuet(m.CallID, m.Approved, m.Reasoning, m.Dur)
		a.status.duetActive = true
		a.status.proCalls++
		a.refreshView()
	case stepFinishMsg:
		a.stepTotal++
		a.status.steps = a.stepTotal
		a.status.usage.PromptTokens += m.Usage.PromptTokens
		a.status.usage.CompletionTokens += m.Usage.CompletionTokens
		a.status.usage.PromptCacheHitTokens += m.Usage.PromptCacheHitTokens
		a.status.usage.PromptCacheMissTokens += m.Usage.PromptCacheMissTokens
		a.status.costYuan += llm.Cost(a.model, m.Usage)
		a.scrollback.AppendStepFinish(m.Reason.String(), m.Usage, a.model)
		a.refreshView()
	case infoMsg:
		a.scrollback.AppendInfo(m.Text)
		a.refreshView()
	case permissionAskMsg:
		a.pendingPerm = &m
		a.permCursor = 0
		cmds = append(cmds, a.setMode(modePermission))
		// Re-layout so the hidden input box gives its rows back to the
		// viewport — the permission card is the active surface now.
		a.layout()
	case agentErrMsg:
		a.scrollback.AppendError(m.Err.Error())
		a.refreshView()
	case agentDoneMsg:
		a.scrollback.EndStreams()
		a.chrome.reset()
		a.refreshView()
		a.dirty = false
	case tea.MouseMsg:
		// Mouse drives visual selection (click-drag-release) and
		// wheel-scrolls the viewport. handleMouse only intercepts
		// left-button events; wheel falls through to vp.Update below.
		if cmd := a.handleMouse(m); cmd != nil {
			cmds = append(cmds, cmd)
		}
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
	if _, isMouse := msg.(tea.MouseMsg); !isMouse {
		if a.mode == modeInsert {
			a.input, c = a.input.Update(msg)
			cmds = append(cmds, c)
		}
	}
	a.vp, c = a.vp.Update(msg)
	cmds = append(cmds, c)
	return a, tea.Batch(cmds...)
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
// chrome.active and a.running both flip false.
func (a *App) ensureTick() tea.Cmd {
	if a.tickActive {
		return nil
	}
	a.tickActive = true
	return a.scheduleRedraw()
}

// View renders the whole UI.
func (a *App) View() string {
	if a.width == 0 || a.height == 0 {
		return "starting…"
	}

	if a.overlay != modeChat {
		return a.renderOverlay()
	}

	// Pager is a modal body replacement: it takes the entire body band
	// while the status / input / hint rows below stay anchored. Keeps
	// the user oriented and the input ready for a follow-up prompt
	// after they dismiss with q.
	var body string
	if a.pager != nil {
		body = a.pager.render(a.theme, a.width, a.vp.Height)
	} else {
		body = a.vp.View()
	}

	// Reserved chrome row — always one line tall. Streaming spinner
	// when active; "new content below" indicator when the user has
	// scrolled away from the bottom mid-run; blank otherwise.
	showNewBelow := a.running && !a.vp.AtBottom() && a.pager == nil
	chrome := a.chrome.render(a.theme, showNewBelow)
	if chrome == "" {
		chrome = " " // reserve the row to prevent reflow when state flips
	}

	var permView string
	if a.pendingPerm != nil {
		permView = a.renderPermissionPrompt()
	}

	status := a.status.render(a.theme)
	divider := a.theme.Hint.Render(strings.Repeat("─", a.width))

	// Permission card replaces the input box: while it is up, the
	// modal is the active surface and the textarea is irrelevant.
	// Skip the input-box / hint rows entirely so the card sits flush
	// above the status line.
	if permView != "" {
		parts := []string{body, chrome, permView, divider, status}
		return lipgloss.JoinVertical(lipgloss.Left, parts...)
	}

	// Choose input border style based on mode.
	borderStyle := a.theme.InputBorder
	if a.mode != modeInsert {
		borderStyle = a.theme.InputBorderDim
	}
	inputBox := borderStyle.
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderStyle.GetForeground()).
		Padding(0, 1).
		Width(a.width - 2).
		Render(a.input.View())

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
		hint = a.theme.Hint.Render(
			"  ⏎ send · ⇧⏎ newline · esc scroll · ^C cancel · ^R thinking · /help",
		)
	}

	parts := []string{body, chrome, divider, status, inputBox, hint}
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

// renderOverlay renders the active fullscreen overlay (tape, models,
// sessions). The bottom hint line + status still show for context.
func (a *App) renderOverlay() string {
	var body string
	switch a.overlay {
	case modeTape:
		body = renderTape(a.theme, a.scrollback.Items(), a.overlayCursor, a.width, a.height-2)
	case modeModels:
		body = renderModelsPicker(a.theme, a.models, a.overlayCursor, a.model, a.width, a.height-2)
	case modeSessions:
		body = renderSessionsPicker(a.theme, a.sessionsRows, a.overlayCursor, a.sessionID, a.width, a.height-2)
	}
	hint := a.theme.Hint.Render("  j/k move · ⏎ select · esc back · q quit")
	return body + "\n" + hint
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
	statusH := 1
	if a.status.duetActive {
		statusH = 2
	}
	const (
		chromeH  = 1
		dividerH = 1
	)
	inputH := 5 // textarea(3) + rounded border(2)
	hintH := 1
	permH := 0
	if a.pendingPerm != nil {
		// Modal: 7 body lines + 2 border ≈ 9. Match this to the row
		// count rendered by renderPermissionPrompt.
		permH = 9
		// Hide input box + hint rows while the modal is up — the View
		// layer also skips them, but we have to reclaim the rows here
		// so the viewport gets the height back.
		inputH = 0
		hintH = 0
	}
	bodyH := a.height - chromeH - statusH - dividerH - inputH - hintH - permH
	if bodyH < 5 {
		bodyH = 5
	}
	a.vp.Width = a.width
	a.vp.Height = bodyH
	// Textarea content width = total width − 2 border cols − 2 padding cols.
	a.input.SetWidth(a.width - 4)
	a.refreshView()
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
	if wasAtBottom && !a.scrollback.SelectionActive() {
		a.vp.GotoBottom()
	}
}

// handleKey dispatches a keypress based on the current mode.
// Returns (cmd, intercepted) — when intercepted is false, the caller
// falls through so the textarea receives the key.
func (a *App) handleKey(km tea.KeyMsg) (tea.Cmd, bool) {
	// Overlays consume nearly all keys until ESC.
	if a.overlay != modeChat {
		return a.handleOverlayKey(km), true
	}

	// Global keys that work in any mode.
	switch km.String() {
	case "ctrl+c":
		// First ctrl+c cancels a run; otherwise it quits.
		a.runMu.Lock()
		running := a.running
		cancel := a.runCancel
		a.runMu.Unlock()
		if running && cancel != nil {
			cancel()
			return nil, true
		}
		return tea.Quit, true
	case "ctrl+d":
		return tea.Quit, true
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

// handleInsertKey handles keys in Insert (typing) mode.
// Only intercepts Enter (submit), Ctrl+R/T (toggle reasoning), and Esc
// (enter Normal mode). Everything else falls through to the textarea.
func (a *App) handleInsertKey(km tea.KeyMsg) (tea.Cmd, bool) {
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
	}

	if km.Type == tea.KeyEnter {
		// Plain Enter submits; Shift+Enter / Alt+Enter inserts a newline.
		if km.Alt || hasShiftEnter(km) {
			return nil, false
		}
		text := strings.TrimSpace(a.input.Value())
		if text == "" {
			return nil, true
		}
		a.input.Reset()
		if strings.HasPrefix(text, "/") {
			return a.handleSlash(text), true
		}
		a.scrollback.AppendUser(text)
		a.refreshView()
		return a.submitPromptCmd(text), true
	}

	return nil, false
}

// handleNormalKey handles keys in Normal (scroll) mode.
// Navigation keys (j/k/g/G/Ctrl+U/Ctrl+D) scroll the viewport.
// 'i' returns to Insert mode. Ctrl+R/T toggle reasoning.
// 'e' expands the last collapsed tool result.
// 'p' opens the pager for the last tool result.
// Printable characters auto-switch to Insert (handled in Update).
func (a *App) handleNormalKey(km tea.KeyMsg) (tea.Cmd, bool) {
	switch km.String() {
	case "j", "down":
		a.vp.LineDown(1)
		return nil, true
	case "k", "up":
		a.vp.LineUp(1)
		return nil, true
	case "ctrl+u":
		a.vp.HalfViewUp()
		return nil, true
	case "ctrl+d":
		a.vp.HalfViewDown()
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
		text := a.scrollback.LastAssistantText()
		if text == "" {
			a.scrollback.AppendInfo("nothing to yank (no assistant text yet)")
			a.refreshView()
			return nil, true
		}
		a.scrollback.AppendInfo(fmt.Sprintf("yanked %d bytes to clipboard (OSC 52)", len(text)))
		a.refreshView()
		return yankToClipboardCmd(text), true
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
func hasShiftEnter(km tea.KeyMsg) bool {
	return strings.Contains(strings.ToLower(km.String()), "shift")
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
		a.scrollback.AppendInfo(helpText())
		a.refreshView()
	case "/clear":
		a.scrollback.Clear()
		a.refreshView()
	case "/models":
		// Direct switch: /models <id>
		if len(fields) >= 2 {
			a.applyModelSwitch(fields[1])
			return nil
		}
		a.models = availableModels()
		a.overlay = modeModels
		a.overlayCursor = a.activeModelCursor()
	case "/tape":
		a.overlay = modeTape
		a.overlayCursor = 0
	case "/sessions":
		if a.listSessions == nil {
			a.scrollback.AppendInfo("/sessions unavailable (no persistence wired)")
			a.refreshView()
			return nil
		}
		sessList, err := a.listSessions()
		if err != nil {
			a.scrollback.AppendError("loading sessions: " + err.Error())
			a.refreshView()
			return nil
		}
		a.sessionsRows = a.sessionsRows[:0]
		for _, s := range sessList {
			a.sessionsRows = append(a.sessionsRows, sessionRow{Sess: s})
		}
		a.overlay = modeSessions
		a.overlayCursor = 0
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
	default:
		a.scrollback.AppendInfo("unknown command: " + cmd + " (/help for list)")
		a.refreshView()
	}
	return nil
}

func helpText() string {
	return strings.Join([]string{
		"keys:",
		"  ⏎          send prompt",
		"  ⇧⏎ / ⌥⏎    insert newline in the input",
		"  ^C         cancel current run (or quit if idle)",
		"  ^D         quit",
		"  ^R         toggle most recent thinking block",
		"  ^T         toggle all thinking blocks",
		"  pgup/pgdn  scroll",
		"slash commands:",
		"  /help      this help",
		"  /clear     clear scrollback",
		"  /quit      exit",
		"  /models    list / switch the main-loop model",
		"  /tape      open the reasoning tape",
		"  /sessions  list this project's sessions",
		"  /export    open full scrollback in $PAGER for native mouse copy",
		"  /undo      revert the last edit step",
	}, "\n")
}

// renderPermissionPrompt draws the modal permission card.
//
// Layout: a 2×2 grid of buttons. The currently focused button gets a
// solid background so it reads as actionable; the rest dim. Arrow
// keys / tab move focus; Enter activates. Single-letter shortcuts
// (y/n/s/a) still work as accelerators.
//
// The input box is hidden in View() while this card is up — it is
// the active surface, not a passive notice.
func (a *App) renderPermissionPrompt() string {
	if a.pendingPerm == nil {
		return ""
	}
	innerW := a.width - 4
	if innerW < 30 {
		innerW = 30
	}
	tool := a.pendingPerm.Check.Tool.Name()
	args := oneline(string(a.pendingPerm.Check.Args), innerW-10)

	title := a.theme.PermPrompt.Render("⚠ permission required")
	toolLine := a.theme.AssistantText.Render("  tool  ") +
		a.theme.StatusModel.Render(tool)
	argsLine := a.theme.AssistantText.Render("  args  ") +
		a.theme.Hint.Render(args)

	// Each button is fixed-width so the grid stays aligned regardless
	// of label length. Padding-1 around the label gives the focused
	// background visual weight without looking cramped.
	const btnW = 20
	btns := []struct{ key, label string }{
		{"Y", "allow once"},
		{"S", "session"},
		{"A", "always"},
		{"N", "deny"},
	}
	renderBtn := func(idx int) string {
		b := btns[idx]
		text := " [" + b.key + "] " + b.label
		// Right-pad to btnW columns so focus/unfocus are the same width.
		pad := btnW - lipglossWidth(text)
		if pad > 0 {
			text += strings.Repeat(" ", pad)
		}
		if idx == a.permCursor {
			return a.theme.PermFocus.Render(text)
		}
		return a.theme.PermButton.Render(text)
	}

	row1 := "  " + renderBtn(0) + "  " + renderBtn(1)
	row2 := "  " + renderBtn(2) + "  " + renderBtn(3)
	hint := a.theme.Hint.Render("  ←↑↓→ / tab move · ⏎ select · esc deny · y/s/a/n direct")

	body := strings.Join([]string{
		title,
		toolLine,
		argsLine,
		"",
		row1,
		row2,
		hint,
	}, "\n")

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(a.theme.PermPrompt.GetForeground()).
		Padding(0, 1).
		Width(a.width - 2).
		Render(body)
}

// lipglossWidth returns the visible cell width of s, ignoring ANSI
// escape sequences. Used to compute padding for fixed-width buttons
// where the input string might include styled fragments.
func lipglossWidth(s string) int {
	// The strings we pass here are plain ASCII labels (no styling and
	// no double-width runes), so a byte count suffices and avoids the
	// dependency surface of a runewidth library.
	return len(s)
}

// handlePermissionKey processes a single keystroke while a permission
// prompt is up. Returns a tea.Cmd; the reply is sent on the request
// channel asynchronously so the UI doesn't block.
func (a *App) handlePermissionKey(km tea.KeyMsg) tea.Cmd {
	if a.pendingPerm == nil {
		return nil
	}
	key := strings.ToLower(km.String())

	// Grid navigation. The 2×2 layout maps to permCursor like so:
	//   0 1
	//   2 3
	// Up/down moves by 2, left/right by 1, tab cycles.
	switch key {
	case "up", "k":
		if a.permCursor >= 2 {
			a.permCursor -= 2
		}
		return nil
	case "down", "j":
		if a.permCursor < 2 {
			a.permCursor += 2
		}
		return nil
	case "left", "h":
		if a.permCursor%2 == 1 {
			a.permCursor--
		}
		return nil
	case "right", "l":
		if a.permCursor%2 == 0 && a.permCursor < 3 {
			a.permCursor++
		}
		return nil
	case "tab":
		a.permCursor = (a.permCursor + 1) % 4
		return nil
	case "shift+tab":
		a.permCursor = (a.permCursor + 3) % 4
		return nil
	}

	// Resolve the chosen button index. Enter activates focus; the
	// single-letter shortcuts (y/o/s/a/n/d) still work directly so
	// muscle-memory users skip the grid entirely.
	btn := -1
	switch key {
	case "enter":
		btn = a.permCursor
	case "y", "o":
		btn = 0
	case "s":
		btn = 1
	case "a":
		btn = 2
	case "n", "d", "esc":
		btn = 3
	}
	if btn < 0 {
		// Swallow — modal modes must not leak keys to the textarea
		// even when the user mashes something unmapped.
		return nil
	}

	var resp agent.PermissionResponse
	switch btn {
	case 0: // allow once
		resp = agent.PermissionResponse{Allow: true}
	case 1: // session
		resp = agent.PermissionResponse{Allow: true}
	case 2: // always
		resp = agent.PermissionResponse{Allow: true, PersistPattern: true}
	case 3: // deny
		resp = agent.PermissionResponse{Allow: false}
	}
	reply := a.pendingPerm.Reply
	check := a.pendingPerm.Check
	a.pendingPerm = nil
	focusCmd := a.setMode(modeInsert)
	// Re-layout so the input box and hint row return to the budget.
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

// handleOverlayKey processes keys in tape/models/sessions overlay mode.
func (a *App) handleOverlayKey(km tea.KeyMsg) tea.Cmd {
	switch km.String() {
	case "esc", "q":
		a.overlay = modeChat
		return nil
	case "ctrl+c":
		return tea.Quit
	case "j", "down":
		a.overlayCursor++
	case "k", "up":
		a.overlayCursor--
		if a.overlayCursor < 0 {
			a.overlayCursor = 0
		}
	case "enter":
		switch a.overlay {
		case modeModels:
			if a.overlayCursor < len(a.models) {
				a.applyModelSwitch(a.models[a.overlayCursor].ID)
			}
			a.overlay = modeChat
		case modeSessions:
			// Session switching is a wave-6 polish — for v0.1 it just
			// records intent and asks the user to restart with -r <id>.
			if a.overlayCursor < len(a.sessionsRows) {
				picked := a.sessionsRows[a.overlayCursor].Sess.ID
				a.scrollback.AppendInfo("to switch sessions, restart with: dsc -r " + picked)
				a.refreshView()
			}
			a.overlay = modeChat
		case modeTape:
			// Future: branch from cursor. For v0.1 wave-5 we just exit.
			a.overlay = modeChat
		}
	}
	return nil
}

// applyModelSwitch updates the live model used for the next turn and
// (when persistence is wired) records it to the session row.
func (a *App) applyModelSwitch(id string) {
	if id == "" {
		return
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
		return
	}
	a.model = id
	a.status.model = id
	a.agent.Model = id
	if a.setModelFn != nil {
		if err := a.setModelFn(id); err != nil {
			a.scrollback.AppendInfo("model switched (warning: persist failed: " + err.Error() + ")")
			a.refreshView()
			return
		}
	}
	a.scrollback.AppendInfo("active model → " + id)
	a.refreshView()
}

// applyUndo reverts the last n snapshot steps.
func (a *App) applyUndo(n int) {
	if a.undoFn == nil {
		a.scrollback.AppendInfo("/undo unavailable (no snapshots wired)")
		a.refreshView()
		return
	}
	restored, err := a.undoFn(n)
	if err != nil {
		a.scrollback.AppendError("/undo: " + err.Error())
		a.refreshView()
		return
	}
	a.scrollback.AppendInfo(fmt.Sprintf("/undo restored %d file(s) across %d step(s)", restored, n))
	a.refreshView()
}

func (a *App) activeModelCursor() int {
	for i, m := range a.models {
		if m.ID == a.model {
			return i
		}
	}
	return 0
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
