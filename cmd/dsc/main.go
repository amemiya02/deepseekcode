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
	"strings"
	"syscall"
	"time"

	"github.com/amemiya02/deepseekcode/internal/agent"
	"github.com/amemiya02/deepseekcode/internal/config"
	"github.com/amemiya02/deepseekcode/internal/llm"
	"github.com/amemiya02/deepseekcode/internal/logging"
	"github.com/amemiya02/deepseekcode/internal/permissions"
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
	// Subcommand: dsc doctor. Doctor prints its own report; exit(1) on
	// failure so main doesn't print "dsc: doctor failed" on top.
	if len(os.Args) > 1 && os.Args[1] == "doctor" {
		cfg, loadErr := config.Load()
		if err := runDoctor(cfg, loadErr); err != nil {
			os.Exit(1)
		}
		return nil
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
	tools.RegisterBuiltins(reg)

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
		cfg.Permissions.SecretPathPatterns, cfg.Permissions.AllowBash)

	a := agent.New(client, reg, pol, cfg.Defaults.Model)
	a.Thinking = cfg.Defaults.Thinking
	a.DuetExtraDestructive = cfg.Duet.ExtraDestructive

	// Route retry notices through OnInfo so they appear as chat items
	// instead of stderr writes that would corrupt the TUI.
	client.OnRetry = func(attempt int, err error) {
		if a.Callbacks.OnInfo != nil {
			a.Callbacks.OnInfo(fmt.Sprintf("retry %d/%d: %v", attempt, client.MaxRetries, err))
		}
	}

	if cfg.Duet.Enabled {
		validator := agent.NewProValidator(client, config.ModelPro)
		if cfg.Duet.ValidatorTimeoutMs > 0 {
			validator.Timeout = time.Duration(cfg.Duet.ValidatorTimeoutMs) * time.Millisecond
		}
		a.Validator = validator
	} else {
		a.Validator = nil
	}

	// Persistence (best effort). Notices are collected and rendered at
	// TUI startup, not written to stderr.
	var (
		sessionID  string
		undoFn     func(int) (int, error)
		listFn     func() ([]session.Session, error)
		setModelFn func(string) error
		notices    []string
	)

	store, err := session.Open("")
	if err != nil {
		notices = append(notices, "warning: session store unavailable: "+err.Error())
	} else {
		snaps := snapshots.New(".deepseek/snapshots")
		ctx := context.Background()

		// Resolve session: --new skips resume; -r <id> > -c (last in cwd).
		var sess session.Session
		if !newSession {
			switch {
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
							Role:       m.Role,
							Content:    m.Content,
							ToolCalls:  m.ToolCalls,
							ToolCallID: m.ToolCallID,
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

	app := tui.New(tui.Config{
		Agent:          a,
		Model:          cfg.Defaults.Model,
		Thinking:       cfg.Defaults.Thinking,
		Theme:          cfg.Defaults.Theme,
		Cwd:            cwd,
		SessionID:      sessionID,
		UndoFn:         undoFn,
		ListSessions:   listFn,
		SetModelFn:     setModelFn,
		StartupNotices: notices,
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
	tools.RegisterBuiltins(reg)

	mode := permissions.ModeDefault
	switch {
	case mf.yolo:
		mode = permissions.ModeYolo
	case mf.readOnly:
		mode = permissions.ModeReadOnly
	case mf.askAll:
		mode = permissions.ModeAskAll
	}
	cwd, _ := os.Getwd()
	pol := permissions.New(mode, cwd,
		cfg.Permissions.SecretPathPatterns, cfg.Permissions.AllowBash)

	a := agent.New(client, reg, pol, cfg.Defaults.Model)
	a.Thinking = cfg.Defaults.Thinking
	a.DuetExtraDestructive = cfg.Duet.ExtraDestructive
	if !cfg.Duet.Enabled {
		a.Validator = nil
	}

	a.Callbacks = stdoutCallbacks(cfg)

	reason, err := a.Run(ctx, prompt)
	fmt.Fprintf(os.Stderr, "\n[stop: %s", reason)
	if err != nil {
		fmt.Fprintf(os.Stderr, " err=%v", err)
	}
	fmt.Fprintln(os.Stderr, "]")
	return nil
}

// stdoutCallbacks renders the agent's event stream to stdout (text +
// reasoning + tool calls + cost). Reasoning blocks are bracketed with
// ▸/◂ so the human reading along can fold them mentally.
func stdoutCallbacks(cfg config.Config) agent.Callbacks {
	var (
		inReasoning bool
		stepStart   time.Time
		stepTokens  int
	)
	startStep := time.Now()
	return agent.Callbacks{
		OnReasoningStart: func() {
			inReasoning = true
			stepStart = time.Now()
			stepTokens = 0
			fmt.Fprint(os.Stderr, "\n\033[2m▸ thinking ")
		},
		OnReasoningDelta: func(text string) {
			stepTokens += len(text) / 4 // crude estimate
			fmt.Fprint(os.Stderr, "\033[2m"+text+"\033[0m")
		},
		OnReasoningEnd: func() {
			if !inReasoning {
				return
			}
			inReasoning = false
			fmt.Fprintf(os.Stderr, "\033[2m ◂ (%.1fs · ~%d tok)\033[0m\n",
				time.Since(stepStart).Seconds(), stepTokens)
		},
		OnTextDelta: func(text string) {
			fmt.Print(text)
		},
		OnToolCallStart: func(call llm.ToolCall) {
			fmt.Fprintf(os.Stderr, "\n\033[36m▶ %s(%s)\033[0m\n",
				call.Function.Name, oneline(call.Function.Arguments))
		},
		OnToolCallResult: func(callID string, res tools.Result, dur time.Duration) {
			tag := "✓"
			if res.IsError {
				tag = "✗"
			}
			fmt.Fprintf(os.Stderr, "\033[36m%s %s (%s)\033[0m\n",
				tag, callID, dur.Round(time.Millisecond))
			if len(res.Content) > 0 {
				fmt.Fprintln(os.Stderr, indent(res.Content, "  "))
			}
		},
		OnDuetValidation: func(callID string, approved bool, reasoning string, dur time.Duration) {
			verdict := "approved"
			if !approved {
				verdict = "BLOCKED"
			}
			fmt.Fprintf(os.Stderr, "\n\033[35m◆ pro check (%s): %s — %s\033[0m\n",
				dur.Round(time.Millisecond), verdict, reasoning)
		},
		OnStepFinish: func(reason agent.StopReason, usage llm.Usage) {
			cost := llm.Cost(cfg.Defaults.Model, usage)
			hit := llm.CacheHitRate(usage)
			fmt.Fprintf(os.Stderr,
				"\n\033[2m[step done: %s · in=%d out=%d cache=%.0f%% ¥%.4f · %s]\033[0m\n",
				reason, usage.PromptTokens, usage.CompletionTokens,
				hit*100, cost, time.Since(startStep).Round(time.Millisecond))
		},
		OnPermissionAsk: func(check permissions.Check) agent.PermissionResponse {
			fmt.Fprintf(os.Stderr,
				"\n\033[33m? approve tool call:\033[0m %s\n  args: %s\n  [o]nce [s]ession [a]lways [d]eny > ",
				check.Tool.Name(), oneline(string(check.Args)))
			rd := bufio.NewReader(os.Stdin)
			line, _ := rd.ReadString('\n')
			line = strings.TrimSpace(line)
			switch line {
			case "o", "":
				return agent.PermissionResponse{Allow: true}
			case "s":
				return agent.PermissionResponse{Allow: true} // session is wave-3 persistent
			case "a":
				return agent.PermissionResponse{Allow: true, PersistPattern: true}
			default:
				return agent.PermissionResponse{Allow: false}
			}
		},
		OnInfo: func(msg string) {
			fmt.Fprintf(os.Stderr, "\n\033[2m[info] %s\033[0m\n", msg)
		},
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
