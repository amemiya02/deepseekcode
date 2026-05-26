// Command dsc is the deepseekcode CLI entrypoint.
//
// v0.1 is a terminal-native coding agent purpose-built for DeepSeek
// models. See docs/design.md for the full design.
//
// Modes:
//
//	dsc                — launch the TUI (wave 3, not yet wired)
//	dsc -p "prompt"    — one-shot prompt to stdout (works today)
//	dsc -version       — print version and exit
package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/amemiya02/deepseekcode/internal/agent"
	"github.com/amemiya02/deepseekcode/internal/commands"
	"github.com/amemiya02/deepseekcode/internal/config"
	"github.com/amemiya02/deepseekcode/internal/hooks"
	"github.com/amemiya02/deepseekcode/internal/llm"
	"github.com/amemiya02/deepseekcode/internal/logging"
	"github.com/amemiya02/deepseekcode/internal/lsp"
	"github.com/amemiya02/deepseekcode/internal/mcp"
	"github.com/amemiya02/deepseekcode/internal/permissions"
	promptpkg "github.com/amemiya02/deepseekcode/internal/prompt"
	"github.com/amemiya02/deepseekcode/internal/session"
	"github.com/amemiya02/deepseekcode/internal/snapshots"
	"github.com/amemiya02/deepseekcode/internal/tools"
	"github.com/amemiya02/deepseekcode/internal/tui"
	"github.com/amemiya02/deepseekcode/internal/version"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "dsc:", err)
		os.Exit(1)
	}
}

func run() error {
	// Subcommand: dsc init. Creates DEEPSEEK.md and .deepseek/config.toml.
	if len(os.Args) > 1 && os.Args[1] == "init" {
		if err := runInit(); err != nil {
			return err
		}
		return nil
	}

	// Subcommand: dsc doctor. Doctor prints its own report; exit(1) on
	// failure so main doesn't print "dsc: doctor failed" on top.
	if len(os.Args) > 1 && os.Args[1] == "doctor" {
		cfg, loadErr := config.Load()
		if err := runDoctor(cfg, loadErr); err != nil {
			os.Exit(1)
		}
		return nil
	}

	// Subcommand: dsc upgrade. Checks for updates and prints (or applies)
	// the upgrade command for the detected install method.
	if len(os.Args) > 1 && os.Args[1] == "upgrade" {
		return runUpgrade(os.Args[2:])
	}

	var (
		showVersion bool
		yolo        bool
		readOnly    bool
		askAll      bool
		noDuet      bool
		model       string
		newSession  bool
		continueSes bool
		resumeSes   string
		prompt      string
		debug       bool
	)
	flag.BoolVar(&showVersion, "version", false, "print version and exit")
	flag.BoolVar(&yolo, "yolo", false, "auto-approve all tool calls (DANGEROUS)")
	flag.BoolVar(&readOnly, "read-only", false, "block all write/edit/bash tools")
	flag.BoolVar(&askAll, "ask-all", false, "prompt for every tool, ignoring allowlist")
	flag.BoolVar(&noDuet, "no-duet", false, "disable the Two-Model Duet (Pro Validator)")
	flag.StringVar(&model, "model", "", "override main-loop model (e.g. deepseek-v4-pro)")
	flag.BoolVar(&newSession, "new", false, "force new session, even if a recent one exists")
	flag.BoolVar(&continueSes, "c", false, "continue last session in cwd")
	flag.StringVar(&resumeSes, "r", "", "resume session by ID (empty opens picker)")
	flag.StringVar(&prompt, "p", "", "one-shot: send PROMPT to the model, print result, exit")
	flag.BoolVar(&debug, "debug", false, "enable structured logging to .deepseek/log/")
	flag.Parse()

	if showVersion {
		fmt.Println("dsc", version.String())
		return nil
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	if model != "" {
		cfg.Defaults.Model = model
	}
	if noDuet {
		cfg.Duet.Enabled = false
	}
	if err := cfg.Validate(); err != nil {
		return err
	}
	if errs := config.ValidateStrict(&cfg); len(errs) > 0 {
		for _, e := range errs {
			fmt.Fprintln(os.Stderr, "config error:", e.Error())
		}
		return fmt.Errorf("config validation failed (%d error(s))", len(errs))
	}

	// Read prompt from stdin if -p was given as empty and stdin is a pipe.
	if prompt == "" {
		if stdinIsPipe() {
			b, err := io.ReadAll(os.Stdin)
			if err == nil && len(b) > 0 {
				prompt = strings.TrimSpace(string(b))
			}
		}
	}

	// Logging mode depends on whether we'll own the terminal. TUI mode
	// must NOT write to stderr — that corrupts Bubble Tea's AltScreen.
	tuiMode := prompt == ""
	cwd, _ := os.Getwd()
	logMode := logging.ModeCLI
	if tuiMode {
		logMode = logging.ModeTUI
	}
	logging.Setup(debug, logMode, cwd)
	slog.Debug("dsc starting", "model", model, "debug", debug, "tui", tuiMode)

	if !tuiMode {
		return runOneShot(cfg, prompt, modeFlags{yolo: yolo, readOnly: readOnly, askAll: askAll})
	}

	return runTUI(cfg, cwd, modeFlags{yolo: yolo, readOnly: readOnly, askAll: askAll}, newSession, continueSes, resumeSes)
}

// runTUI launches the Bubble Tea TUI. Persistence (session store +
// snapshot manager) is best-effort: if the store can't open, the TUI
// still runs in ephemeral mode so the user always gets *something*.
//
// All user-facing notices route through tui.Config.StartupNotices and
// agent.Callbacks.OnInfo so nothing writes to stderr after this point —
// stderr writes would corrupt Bubble Tea's AltScreen.
func runTUI(cfg config.Config, cwd string, mf modeFlags, newSession bool, continueSes bool, resumeSes string) error {
	client := llm.NewClient(cfg.API.Key, cfg.API.BaseURL)
	if cfg.API.FirstTokenTimeoutMs > 0 {
		client.FirstTokenTimeout = time.Duration(cfg.API.FirstTokenTimeoutMs) * time.Millisecond
	}
	if cfg.API.ChunkStallTimeoutMs > 0 {
		client.ChunkStallTimeout = time.Duration(cfg.API.ChunkStallTimeoutMs) * time.Millisecond
	}

	reg := tools.New()
	tools.RegisterBuiltins(reg, cfg.Tools.MaxReadBytes, cfg.Tools.MaxWriteBytes, cwd)

	// MCP servers: connect, bridge tools into the registry.
	mcpReg := mcp.NewRegistry()
	defer mcpReg.Shutdown()
	var mcpNotices []string
	for name, srv := range cfg.MCPServers {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		err := mcpReg.Connect(ctx, name, srv.Command, srv.Args, srv.Env)
		cancel()
		if err != nil {
			slog.Warn("mcp server failed to start", "name", name, "err", err)
			mcpNotices = append(mcpNotices, fmt.Sprintf("mcp[%s]: failed — %v", name, err))
			continue
		}
		if srv.TimeoutSeconds > 0 {
			mcpReg.SetTimeout(name, srv.TimeoutSeconds)
		}
	}
	for _, t := range mcp.BridgeAll(mcpReg) {
		reg.Register(t)
	}
	for _, s := range mcpReg.Servers() {
		if s.State == mcp.StateConnected {
			mcpNotices = append(mcpNotices, fmt.Sprintf("mcp: connected to %s (%d tools)", s.Name, len(s.Tools)))
		}
	}

	// LSP: detect language servers, connect, expose as a tool.
	lspReg := lsp.NewRegistry()
	defer lspReg.Shutdown()
	lspReg.Start(context.Background(), cwd)
	reg.Register(tools.NewLSPTool(lspReg))
	for _, name := range lspReg.Servers() {
		mcpNotices = append(mcpNotices, fmt.Sprintf("lsp: connected to %s", name))
	}

	mode := permissions.ModeDefault
	switch {
	case mf.yolo:
		mode = permissions.ModeYolo
	case mf.readOnly:
		mode = permissions.ModeReadOnly
	case mf.askAll:
		mode = permissions.ModeAskAll
	}
	pol := permissions.New(mode, cwd,
		cfg.Permissions.SecretPathPatterns, cfg.Permissions.AllowBash, buildRuleEngine(cfg.Permissions.Rules))
	// Load skills once; shared between prompt builder and command table.
	home4skills, _ := os.UserHomeDir()
	skills, _ := promptpkg.LoadSkills(cwd, home4skills)
	a := agent.New(client, reg, pol, cfg.Defaults.Model)
	reg.Register(tools.NewQuestionTool(a))
		reg.Register(tools.NewBackgroundBashTool(a))
		reg.Register(tools.NewTaskStatusTool(a))
		a.Thinking = cfg.Defaults.Thinking
		a.AutoReasoning = cfg.Defaults.AutoReasoning
		a.PromptBuilder = newPromptBuilder(cwd, skills)

		// Hooks: assemble configs from TOML, then add the Duet builtin
	// when enabled. Only create a Runner if there is work for it.
	var hookConfigs []hooks.HookConfig
	for _, hi := range cfg.Hooks {
		hc := hooks.HookConfig{
			Event:   hooks.HookEvent(hi.Event),
			Type:    hooks.HookType(hi.Type),
			Command: hi.Command,
			Name:    hi.Name,
		}
		if hc.Type == "" {
			hc.Type = hooks.TypeSubprocess
		}
		if !validHookEvent(hc.Event) {
			slog.Warn("skipping hook with unknown event", "event", hi.Event)
			continue
		}
		if hi.Timeout > 0 {
			hc.Timeout = time.Duration(hi.Timeout) * time.Second
		}
		hookConfigs = append(hookConfigs, hc)
	}

	if cfg.Duet.Enabled {
		hasDuetPreTool := false
		for _, hc := range hookConfigs {
			if hc.Name == "duet" && hc.Event == hooks.EventPreToolUse {
				hasDuetPreTool = true
				break
			}
		}
		if !hasDuetPreTool {
			hookConfigs = append(hookConfigs, hooks.HookConfig{
				Event: hooks.EventPreToolUse,
				Type:  hooks.TypeBuiltin,
				Name:  "duet",
			})
		}
	}

	if len(hookConfigs) > 0 {
		hookRunner := hooks.NewRunner()
		if cfg.Duet.Enabled {
			hookRunner.Register("duet", hooks.NewDuetHook(
				client,
				cfg.Duet.ExtraDestructive,
				cwd,
				cfg.Permissions.SecretPathPatterns,
				func() string { return a.Model },
				func() []byte { return a.Transcript() },
			))
		}
		hookRunner.Configure(hookConfigs)
		a.HookRunner = hookRunner
	}

	// Route retry notices through agent.EmitInfo so they appear as
	// chat items instead of stderr writes that would corrupt the TUI.
	client.OnRetry = func(attempt int, err error) {
		a.EmitInfo(fmt.Sprintf("retry %d/%d: %v", attempt, client.MaxRetries, err))
	}

	// Persistence (best effort). Notices are collected and rendered at
	// TUI startup, not written to stderr.
	var (
		sessionID  string
		undoFn     func(int) (int, error)
		listFn     func() ([]session.Session, error)
		setModelFn func(string) error
		notices    []string
		sess       session.Session
	)

	store, err := session.Open("")
	if err != nil {
		notices = append(notices, "warning: session store unavailable: "+err.Error())
	} else {
		snaps := snapshots.New(".deepseek/snapshots")
		ctx := context.Background()

		// Resolve session: --new skips resume; -r <id> > -c (last in cwd).
		if !newSession {
			switch {
			case resumeSes == "latest" || resumeSes == "last":
				sess, err = store.LatestInProject(ctx, cwd)
				if err != nil {
					notices = append(notices, "warning: no latest session for this project, creating new")
				}
			case resumeSes != "":
				sess, err = store.GetSession(ctx, resumeSes)
				if err != nil {
					notices = append(notices, fmt.Sprintf("warning: session %s not found, creating new", resumeSes))
				}
			case continueSes:
				sess, err = store.MostRecentInProject(ctx, cwd)
				if err != nil {
					notices = append(notices, "warning: no previous session in cwd, creating new")
				}
			}
		}

		if sess.ID == "" {
			sess, err = store.NewSession(ctx, cwd, cfg.Defaults.Model, cfg.Duet.Enabled)
			if err != nil {
				notices = append(notices, "warning: creating session: "+err.Error())
			}
		}

		if sess.ID != "" {
			// Cross-workspace warning (Phase 8).
			if sess.WorkspaceFP != "" {
				cwdFP, _ := session.Fingerprint(cwd)
				if cwdFP != "" && cwdFP != sess.WorkspaceFP {
					warn := "warning: session was created in different workspace"
					fmt.Fprintln(os.Stderr, warn)
					notices = append(notices, warn)
				}
			}

			persister := session.NewPersister(store, snaps, sess.ID)
			a.Persister = persister
			sessionID = sess.ID

			if continueSes || resumeSes != "" {
				msgs, loadErr := store.Replay(ctx, sess.ID)
				if loadErr != nil {
					notices = append(notices, "warning: loading messages: "+loadErr.Error())
				} else {
					for _, m := range msgs {
						a.Messages = append(a.Messages, llm.Message{
							Role:   m.Role,
							Blocks: m.Blocks,
						})
					}
					if len(msgs) > 0 {
						notices = append(notices, fmt.Sprintf("resumed session %s (%d messages)", sess.ID[:8], len(msgs)))
					}
				}
			}

			// Drop a project pointer for `dsc -c`.
			_ = os.MkdirAll(".deepseek", 0o755)
			_ = os.WriteFile(".deepseek/last_session", []byte(sess.ID), 0o644)
			_ = os.WriteFile(".deepseek/.gitignore", []byte("*\n"), 0o644)

			undoFn = persister.Undo
			listFn = func() ([]session.Session, error) {
				return store.ListByProject(context.Background(), cwd)
			}
			setModelFn = func(m string) error {
				a.Model = m
				return store.UpdateModel(context.Background(), sess.ID, m)
			}
		}
	}

	notices = append(mcpNotices, notices...)
	home, _ := os.UserHomeDir()
	customCmds, _ := commands.Load(cwd, home)

	// Promote skills as slash commands (user commands take priority).
	for _, sk := range skills {
		if _, taken := customCmds[sk.Name]; taken {
			continue // user command takes priority
		}
		customCmds[sk.Name] = commands.Command{
			Name:        sk.Name,
			Description: sk.Description,
			Template:    sk.Body,
			Path:        sk.Path,
		}
	}

	app := tui.New(tui.Config{
		Agent:           a,
		Model:           cfg.Defaults.Model,
		Thinking:        cfg.Defaults.Thinking,
		Theme:           cfg.Defaults.Theme,
		Cwd:             cwd,
		SessionID:       sessionID,
		UndoFn:          undoFn,
		ListSessions:    listFn,
		SetModelFn:      setModelFn,
		Commands:        customCmds,
		StartupNotices:  notices,
		CompactionCount: sess.CompactionCount,
	})
	return app.Run()
}

type modeFlags struct {
	yolo, readOnly, askAll bool
}

// runOneShot wires the agent end-to-end and runs a single turn against
// the model with stdout-streaming callbacks. Permission prompts read
// from stdin so this mode is usable in a real terminal.
func runOneShot(cfg config.Config, prompt string, mf modeFlags) error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	client := llm.NewClient(cfg.API.Key, cfg.API.BaseURL)
	if cfg.API.FirstTokenTimeoutMs > 0 {
		client.FirstTokenTimeout = time.Duration(cfg.API.FirstTokenTimeoutMs) * time.Millisecond
	}
	if cfg.API.ChunkStallTimeoutMs > 0 {
		client.ChunkStallTimeout = time.Duration(cfg.API.ChunkStallTimeoutMs) * time.Millisecond
	}
	client.OnRetry = func(attempt int, err error) {
		fmt.Fprintf(os.Stderr, "\n\033[33m[retry %d/%d: %v]\033[0m\n", attempt, client.MaxRetries, err)
	}

	reg := tools.New()
	cwd, _ := os.Getwd()
	tools.RegisterBuiltins(reg, cfg.Tools.MaxReadBytes, cfg.Tools.MaxWriteBytes, cwd)

	// LSP: one-shot also gets the lsp tool; servers shut down after the turn.
	lspReg := lsp.NewRegistry()
	defer lspReg.Shutdown()
	lspReg.Start(context.Background(), cwd)
	reg.Register(tools.NewLSPTool(lspReg))

	mode := permissions.ModeDefault
	switch {
	case mf.yolo:
		mode = permissions.ModeYolo
	case mf.readOnly:
		mode = permissions.ModeReadOnly
	case mf.askAll:
		mode = permissions.ModeAskAll
	}
	pol := permissions.New(mode, cwd,
		cfg.Permissions.SecretPathPatterns, cfg.Permissions.AllowBash, buildRuleEngine(cfg.Permissions.Rules))
	// Load skills once; shared between prompt builder and command table.
	home4skills, _ := os.UserHomeDir()
	skills, _ := promptpkg.LoadSkills(cwd, home4skills)
	a := agent.New(client, reg, pol, cfg.Defaults.Model)
	reg.Register(tools.NewQuestionTool(a))
	reg.Register(tools.NewBackgroundBashTool(a))
	reg.Register(tools.NewTaskStatusTool(a))
	a.Thinking = cfg.Defaults.Thinking
	a.AutoReasoning = cfg.Defaults.AutoReasoning
	a.PromptBuilder = newPromptBuilder(cwd, skills)

	// Consumer goroutine: pulls events from the agent's lifetime stream
	// and renders each to stdout/stderr. Mirrors the TUI's pumpEvents
	// adapter — same channel, different sink.
	go consumeAgentEvents(a, cfg.Defaults.Model)

	reason, err := a.Run(ctx, prompt)
	fmt.Fprintf(os.Stderr, "\n[stop: %s", reason)
	if err != nil {
		fmt.Fprintf(os.Stderr, " err=%v", err)
	}
	fmt.Fprintln(os.Stderr, "]")
	return nil
}

// newPromptBuilder assembles the SystemPromptBuilder both flows wire
// onto the agent. Returns nil only on a misconfigured cwd, which the
// callers treat as "no builder — fall back to DefaultSystemPrompt".
var validHookEvents = map[hooks.HookEvent]bool{
	hooks.EventPreToolUse:         true,
	hooks.EventPostToolUse:        true,
	hooks.EventPostToolUseFailure: true,
	hooks.EventSessionStart:       true,
	hooks.EventSessionEnd:         true,
}

func validHookEvent(e hooks.HookEvent) bool { return validHookEvents[e] }

func newPromptBuilder(cwd string, skills []promptpkg.Skill) *promptpkg.SystemPromptBuilder {
	if cwd == "" {
		return nil
	}
	home, _ := os.UserHomeDir()
	files, _ := promptpkg.LoadInstructionFiles(cwd, home)
	project := promptpkg.DiscoverProjectContext(cwd)
	return &promptpkg.SystemPromptBuilder{
		StaticBase:   promptpkg.BasePromptV1,
		Instructions: files,
		Skills:       skills,
		Project:      &project,
	}
}

// consumeAgentEvents drives the CLI's stdout rendering off the agent's
// Events() channel. Each event maps to a small fprintf — reasoning
// dimmed and bracketed with ▸/◂, text streamed to stdout, tool calls /
// results in cyan, duet validations in magenta, step footers in dim.
// Permission asks read a single character from stdin and answer on
// the embedded Reply chan.
func consumeAgentEvents(a *agent.Agent, model string) {
	var (
		inReasoning bool
		stepStart   time.Time
		stepTokens  int
	)
	startStep := time.Now()
	for ev := range a.Events() {
		switch e := ev.(type) {
		case agent.EventReasoningStart:
			_ = e
			inReasoning = true
			stepStart = time.Now()
			stepTokens = 0
			fmt.Fprint(os.Stderr, "\n\033[2m▸ thinking ")
		case agent.EventReasoningDelta:
			stepTokens += len(e.Text) / 4
			fmt.Fprint(os.Stderr, "\033[2m"+e.Text+"\033[0m")
		case agent.EventReasoningEnd:
			_ = e
			if inReasoning {
				inReasoning = false
				fmt.Fprintf(os.Stderr, "\033[2m ◂ (%.1fs · ~%d tok)\033[0m\n",
					time.Since(stepStart).Seconds(), stepTokens)
			}
		case agent.EventTextDelta:
			fmt.Print(e.Text)
		case agent.EventToolCallStart:
			fmt.Fprintf(os.Stderr, "\n\033[36m▶ %s(%s)\033[0m\n",
				e.Call.Function.Name, oneline(e.Call.Function.Arguments))
		case agent.EventToolCallResult:
			tag := "✓"
			if e.Result.IsError {
				tag = "✗"
			}
			fmt.Fprintf(os.Stderr, "\033[36m%s %s (%s)\033[0m\n",
				tag, e.CallID, e.Dur.Round(time.Millisecond))
			if len(e.Result.Content) > 0 {
				fmt.Fprintln(os.Stderr, indent(e.Result.Content, "  "))
			}
		case agent.EventStepFinish:
			cost := llm.Cost(model, e.Usage)
			hit := llm.CacheHitRate(e.Usage)
			costStr := "¥?"
			if llm.CostKnown(model) {
				costStr = fmt.Sprintf("¥%.4f", cost)
			}
			fmt.Fprintf(os.Stderr,
				"\n\033[2m[step done: %s · in=%d out=%d cache=%.0f%% %s · %s]\033[0m\n",
				e.Reason, e.Usage.PromptTokens, e.Usage.CompletionTokens,
				hit*100, costStr, time.Since(startStep).Round(time.Millisecond))
		case agent.EventInfo:
			fmt.Fprintf(os.Stderr, "\n\033[2m[info] %s\033[0m\n", e.Text)
		case agent.EventPermissionAsk:
			fmt.Fprintf(os.Stderr,
				"\n\033[33m? approve tool call:\033[0m %s\n  args: %s\n  [o]nce [s]ession [a]lways [d]eny > ",
				e.Check.Tool.Name(), oneline(string(e.Check.Args)))
			rd := bufio.NewReader(os.Stdin)
			line, _ := rd.ReadString('\n')
			line = strings.TrimSpace(line)
			var resp agent.PermissionResponse
			switch line {
			case "o", "":
				resp = agent.PermissionResponse{Allow: true}
			case "s":
				resp = agent.PermissionResponse{Allow: true}
			case "a":
				resp = agent.PermissionResponse{Allow: true, PersistPattern: true}
			default:
				resp = agent.PermissionResponse{Allow: false}
			}
			e.Reply <- resp
		case agent.EventQuestionAsk:
			answers := make([][]string, len(e.Questions))
			for i, q := range e.Questions {
				fmt.Fprintf(os.Stderr, "\n\033[33m? %s\033[0m", q.Header)
				if q.Question != "" {
					fmt.Fprintf(os.Stderr, " — %s", q.Question)
				}
				fmt.Fprintln(os.Stderr)
				if len(q.Options) == 0 {
					fmt.Fprintln(os.Stderr, "  (no options — press Enter to skip)")
					rd := bufio.NewReader(os.Stdin)
					rd.ReadString('\n')
					answers[i] = nil
					continue
				}
				for j, opt := range q.Options {
					desc := ""
					if opt.Description != "" {
						desc = " — " + opt.Description
					}
					fmt.Fprintf(os.Stderr, "  %d) %s%s\n", j+1, opt.Label, desc)
				}
				if q.Multiple {
					fmt.Fprintf(os.Stderr, "  (comma-separated numbers, e.g. 1,3) > ")
				} else {
					fmt.Fprintf(os.Stderr, "  > ")
				}
				rd := bufio.NewReader(os.Stdin)
				line, err := rd.ReadString('\n')
				if err != nil {
					// EOF or read error — return empty answer.
					answers[i] = nil
					continue
				}
				line = strings.TrimSpace(line)
				if line == "" {
					answers[i] = nil
					continue
				}
				if q.Multiple {
					parts := strings.Split(line, ",")
					var labels []string
					for _, p := range parts {
						p = strings.TrimSpace(p)
						idx, err := strconv.Atoi(p)
						if err != nil || idx < 1 || idx > len(q.Options) {
							continue
						}
						labels = append(labels, q.Options[idx-1].Label)
					}
					answers[i] = labels
				} else {
					idx, err := strconv.Atoi(line)
					if err != nil || idx < 1 || idx > len(q.Options) {
						answers[i] = nil
						continue
					}
					answers[i] = []string{q.Options[idx-1].Label}
				}
			}
			e.Reply <- tools.QuestionResponse{Answers: answers}
		}
	}
}

func stdinIsPipe() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) == 0
}

func oneline(s string) string {
	if len(s) > 120 {
		s = s[:117] + "..."
	}
	return strings.ReplaceAll(strings.ReplaceAll(s, "\n", "\\n"), "\t", "  ")
}

func indent(s, prefix string) string {
	lines := strings.Split(s, "\n")
	for i := range lines {
		lines[i] = prefix + lines[i]
	}
	return strings.Join(lines, "\n")
}

func buildRuleEngine(rc config.RulesConfig) *permissions.RuleEngine {
	if len(rc.Allow) == 0 && len(rc.Deny) == 0 && len(rc.Ask) == 0 {
		return nil
	}
	conv := func(items []config.RuleItemConfig) []permissions.PermissionRule {
		var out []permissions.PermissionRule
		for _, r := range items {
			out = append(out, permissions.PermissionRule{
				ToolPattern: r.Tool,
				ArgsPattern: r.Args,
			})
		}
		return out
	}
	engine := &permissions.RuleEngine{
		Allow: conv(rc.Allow),
		Deny:  conv(rc.Deny),
		Ask:   conv(rc.Ask),
	}
	for i := range engine.Allow {
		engine.Allow[i].Decision = "allow"
	}
	for i := range engine.Deny {
		engine.Deny[i].Decision = "deny"
	}
	for i := range engine.Ask {
		engine.Ask[i].Decision = "ask"
	}
	return engine
}
