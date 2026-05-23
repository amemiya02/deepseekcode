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

	// Chat scrollback
	items       []chatItem
	streamText  *chatItem // pointer into items for in-progress assistant text
	streamThink *chatItem // pointer into items for in-progress reasoning
	thinkStart  time.Time

	// Per-run cancellation
	runMu     sync.Mutex
	runCtx    context.Context
	runCancel context.CancelFunc
	running   bool

	// Status
	status    statusState
	stepTotal int

	// Permission asks
	pendingPerm *permissionAskMsg

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

// Init satisfies tea.Model.
func (a *App) Init() tea.Cmd {
	a.appendItem(chatItem{
		kind: itemInfo,
		text: "deepseekcode — DeepSeek-native coding agent. Type a prompt and press Enter.",
	})
	for _, n := range a.startupNotices {
		a.appendItem(chatItem{kind: itemInfo, text: n})
	}
	return textarea.Blink
}

// Update folds incoming messages into the model.
func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	// Pending permission prompt eats most key input.
	if a.pendingPerm != nil {
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

	// agent → UI events
	case reasoningStartMsg:
		a.thinkStart = time.Now()
		it := chatItem{kind: itemReasoning, folded: true, timestamp: time.Now()}
		a.appendItem(it)
		a.streamThink = &a.items[len(a.items)-1]
	case reasoningDeltaMsg:
		if a.streamThink != nil {
			a.streamThink.reasoning += m.Text
			a.streamThink.tokens = len(a.streamThink.reasoning) / 4
			a.streamThink.duration = time.Since(a.thinkStart)
			a.refreshView()
		}
	case reasoningEndMsg:
		if a.streamThink != nil {
			a.streamThink.duration = time.Since(a.thinkStart)
			a.streamThink = nil
		}
	case textDeltaMsg:
		if a.streamText == nil {
			a.appendItem(chatItem{kind: itemAssistantText, timestamp: time.Now()})
			a.streamText = &a.items[len(a.items)-1]
		}
		a.streamText.text += m.Text
		a.refreshView()
	case toolCallStartMsg:
		a.streamText = nil // a new tool call ends any in-progress assistant text segment
		a.appendItem(chatItem{
			kind:       itemToolCall,
			tool:       m.Call.Function.Name,
			args:       m.Call.Function.Arguments,
			toolCallID: m.Call.ID,
			timestamp:  time.Now(),
		})
	case toolCallResultMsg:
		// Find the matching tool call to attach duration; fall back to a
		// standalone result item if not found.
		tool := ""
		for i := len(a.items) - 1; i >= 0; i-- {
			if a.items[i].kind == itemToolCall && a.items[i].toolCallID == m.CallID {
				tool = a.items[i].tool
				break
			}
		}
		a.appendItem(chatItem{
			kind:       itemToolResult,
			tool:       tool,
			toolCallID: m.CallID,
			result:     m.Result,
			duration:   m.Dur,
			timestamp:  time.Now(),
		})
	case duetMsg:
		a.appendItem(chatItem{
			kind:          itemDuet,
			toolCallID:    m.CallID,
			approved:      m.Approved,
			duetReasoning: m.Reasoning,
			duration:      m.Dur,
			timestamp:     time.Now(),
		})
		a.status.duetActive = true
		a.status.proCalls++
	case stepFinishMsg:
		a.stepTotal++
		a.status.steps = a.stepTotal
		a.status.usage.PromptTokens += m.Usage.PromptTokens
		a.status.usage.CompletionTokens += m.Usage.CompletionTokens
		a.status.usage.PromptCacheHitTokens += m.Usage.PromptCacheHitTokens
		a.status.usage.PromptCacheMissTokens += m.Usage.PromptCacheMissTokens
		a.status.costYuan += llm.Cost(a.model, m.Usage)
		a.appendItem(chatItem{
			kind:       itemStepFinish,
			stopReason: m.Reason.String(),
			usage:      m.Usage,
			model:      a.model,
			timestamp:  time.Now(),
		})
	case infoMsg:
		a.appendItem(chatItem{kind: itemInfo, text: m.Text, timestamp: time.Now()})
	case permissionAskMsg:
		a.pendingPerm = &m
		a.refreshView()
	case agentErrMsg:
		a.appendItem(chatItem{kind: itemError, text: m.Err.Error(), timestamp: time.Now()})
	case agentDoneMsg:
		a.streamText = nil
		a.streamThink = nil
		a.refreshView()
	}

	// Always update sub-models.
	var c tea.Cmd
	a.input, c = a.input.Update(msg)
	cmds = append(cmds, c)
	a.vp, c = a.vp.Update(msg)
	cmds = append(cmds, c)
	return a, tea.Batch(cmds...)
}

// View renders the whole UI.
func (a *App) View() string {
	if a.width == 0 || a.height == 0 {
		return "starting…"
	}

	if a.overlay != modeChat {
		return a.renderOverlay()
	}

	body := a.vp.View()

	var permView string
	if a.pendingPerm != nil {
		permView = a.renderPermissionPrompt()
	}

	status := a.status.render(a.theme)
	divider := a.theme.Hint.Render(strings.Repeat("─", a.width))

	inputBox := a.theme.InputBorder.
		Border(lipgloss.RoundedBorder()).
		BorderForeground(a.theme.InputBorder.GetForeground()).
		Padding(0, 1).
		Width(a.width - 2).
		Render(a.input.View())

	hint := a.theme.Hint.Render(
		"  ⏎ send · ⇧⏎ newline · ^C cancel/quit · ^D quit · ^R toggle thinking · /help",
	)

	parts := []string{body}
	if permView != "" {
		parts = append(parts, permView)
	}
	parts = append(parts, divider, status, inputBox, hint)
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

// renderOverlay renders the active fullscreen overlay (tape, models,
// sessions). The bottom hint line + status still show for context.
func (a *App) renderOverlay() string {
	var body string
	switch a.overlay {
	case modeTape:
		body = renderTape(a.theme, a.items, a.overlayCursor, a.width, a.height-2)
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
//	viewport (bodyH) · permission prompt (permH, if active) ·
//	divider (1) · status (1 or 2) · input box (textarea + border = 5) ·
//	hint (1).
func (a *App) layout() {
	statusH := 1
	if a.status.duetActive {
		statusH = 2
	}
	const (
		dividerH = 1
		inputH   = 5 // textarea(3) + rounded border(2)
		hintH    = 1
	)
	permH := 0
	if a.pendingPerm != nil {
		permH = 4
	}
	bodyH := a.height - statusH - dividerH - inputH - hintH - permH
	if bodyH < 5 {
		bodyH = 5
	}
	a.vp.Width = a.width
	a.vp.Height = bodyH
	// Textarea content width = total width − 2 border cols − 2 padding cols.
	a.input.SetWidth(a.width - 4)
	a.refreshView()
}

// refreshView re-renders the scrollback into the viewport content.
func (a *App) refreshView() {
	if a.width == 0 {
		return
	}
	var b strings.Builder
	for _, it := range a.items {
		b.WriteString(it.render(a.theme, a.width))
	}
	a.vp.SetContent(b.String())
	a.vp.GotoBottom()
}

// appendItem mutates items and re-renders.
func (a *App) appendItem(it chatItem) {
	a.items = append(a.items, it)
	// Reset the streamText pointer if it pointed into items that may have moved.
	// (append may reallocate; we always re-grab via index in callers that need it.)
	a.streamText = nil
	a.streamThink = nil
	a.rebindStreamPointers()
	a.refreshView()
}

// rebindStreamPointers reattaches streamText/streamThink to the last
// in-progress item of each kind after a slice realloc.
func (a *App) rebindStreamPointers() {
	for i := len(a.items) - 1; i >= 0; i-- {
		switch a.items[i].kind {
		case itemAssistantText:
			if a.streamText == nil {
				a.streamText = &a.items[i]
			}
		case itemReasoning:
			if a.streamThink == nil && a.items[i].folded {
				// still streaming reasoning if not yet ended
				a.streamThink = &a.items[i]
			}
		}
	}
}

// handleKey dispatches a keypress when no permission prompt is up.
// Returns (cmd, intercepted) — when intercepted is false, the caller
// falls through so the textarea receives the key.
func (a *App) handleKey(km tea.KeyMsg) (tea.Cmd, bool) {
	// Overlays consume nearly all keys until ESC.
	if a.overlay != modeChat {
		return a.handleOverlayKey(km), true
	}

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
	case "ctrl+r":
		a.toggleLastReasoning()
		return nil, true
	case "ctrl+t":
		a.toggleAllReasoning()
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
		a.appendItem(chatItem{kind: itemUser, text: text, timestamp: time.Now()})
		return a.submitPromptCmd(text), true
	}

	return nil, false
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
		a.appendItem(chatItem{kind: itemInfo, text: helpText()})
	case "/clear":
		a.items = nil
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
			a.appendItem(chatItem{kind: itemInfo, text: "/sessions unavailable (no persistence wired)"})
			return nil
		}
		sessList, err := a.listSessions()
		if err != nil {
			a.appendItem(chatItem{kind: itemError, text: "loading sessions: " + err.Error()})
			return nil
		}
		a.sessionsRows = a.sessionsRows[:0]
		for _, s := range sessList {
			a.sessionsRows = append(a.sessionsRows, sessionRow{Sess: s})
		}
		a.overlay = modeSessions
		a.overlayCursor = 0
	case "/undo":
		n := 1
		if len(fields) >= 2 {
			if parsed := parsePositiveInt(fields[1]); parsed > 0 {
				n = parsed
			}
		}
		a.applyUndo(n)
	default:
		a.appendItem(chatItem{kind: itemInfo, text: "unknown command: " + cmd + " (/help for list)"})
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
		"  /undo      revert the last edit step",
	}, "\n")
}

// renderPermissionPrompt draws the inline permission dialog.
func (a *App) renderPermissionPrompt() string {
	if a.pendingPerm == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString(a.theme.PermPrompt.Render("? approve tool call:"))
	b.WriteByte(' ')
	b.WriteString(a.pendingPerm.Check.Tool.Name())
	b.WriteByte('\n')
	b.WriteString(a.theme.Hint.Render("  args: " + oneline(string(a.pendingPerm.Check.Args), 200)))
	b.WriteByte('\n')
	b.WriteString(a.theme.PermPrompt.Render("  [o]nce  [s]ession  [a]lways  [d]eny"))
	return b.String()
}

// handlePermissionKey processes a single keystroke while a permission
// prompt is up. Returns a tea.Cmd; the reply is sent on the request
// channel asynchronously so the UI doesn't block.
func (a *App) handlePermissionKey(km tea.KeyMsg) tea.Cmd {
	if a.pendingPerm == nil {
		return nil
	}
	key := strings.ToLower(km.String())
	resp := agent.PermissionResponse{}
	consumed := false
	switch key {
	case "o", "enter":
		resp = agent.PermissionResponse{Allow: true}
		consumed = true
	case "s":
		resp = agent.PermissionResponse{Allow: true} // session-scope persistence handled by agent for now
		consumed = true
	case "a":
		resp = agent.PermissionResponse{Allow: true, PersistPattern: true}
		consumed = true
	case "d":
		resp = agent.PermissionResponse{Allow: false}
		consumed = true
	}
	if !consumed {
		return nil
	}
	reply := a.pendingPerm.Reply
	check := a.pendingPerm.Check
	a.pendingPerm = nil
	a.appendItem(chatItem{
		kind: itemInfo,
		text: fmt.Sprintf("permission %s for %s", decisionLabel(resp), check.Tool.Name()),
	})
	go func() { reply <- resp }()
	return nil
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
	for i := len(a.items) - 1; i >= 0; i-- {
		if a.items[i].kind == itemReasoning {
			a.items[i].folded = !a.items[i].folded
			a.refreshView()
			return
		}
	}
}

// toggleAllReasoning expands/collapses every reasoning block (all to
// same target state — collapse if any are open, else expand).
func (a *App) toggleAllReasoning() {
	anyOpen := false
	for _, it := range a.items {
		if it.kind == itemReasoning && !it.folded {
			anyOpen = true
			break
		}
	}
	for i := range a.items {
		if a.items[i].kind == itemReasoning {
			a.items[i].folded = anyOpen
		}
	}
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
				a.appendItem(chatItem{kind: itemInfo,
					text: "to switch sessions, restart with: dsc -r " + picked})
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
		a.appendItem(chatItem{kind: itemError, text: "unknown model: " + id})
		return
	}
	a.model = id
	a.status.model = id
	a.agent.Model = id
	if a.setModelFn != nil {
		if err := a.setModelFn(id); err != nil {
			a.appendItem(chatItem{kind: itemInfo, text: "model switched (warning: persist failed: " + err.Error() + ")"})
			return
		}
	}
	a.appendItem(chatItem{kind: itemInfo, text: "active model → " + id})
}

// applyUndo reverts the last n snapshot steps.
func (a *App) applyUndo(n int) {
	if a.undoFn == nil {
		a.appendItem(chatItem{kind: itemInfo, text: "/undo unavailable (no snapshots wired)"})
		return
	}
	restored, err := a.undoFn(n)
	if err != nil {
		a.appendItem(chatItem{kind: itemError, text: "/undo: " + err.Error()})
		return
	}
	a.appendItem(chatItem{kind: itemInfo,
		text: fmt.Sprintf("/undo restored %d file(s) across %d step(s)", restored, n)})
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
