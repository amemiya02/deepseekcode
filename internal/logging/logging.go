// Package logging provides structured logging for deepseekcode.
//
// When debug is off, logs are discarded (callers can still use slog.X
// without conditionals). When on, logs go to .deepseek/log/<date>.jsonl
// under the supplied dir.
//
// Mode controls whether stderr is also a sink. ModeCLI writes to both
// (stderr + file); ModeTUI is file-only because Bubble Tea's AltScreen
// would corrupt the display if anything else wrote to the terminal.
package logging

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Mode controls whether logs are mirrored to stderr alongside the file.
type Mode int

const (
	ModeCLI Mode = iota // stderr + file
	ModeTUI             // file only (avoids corrupting AltScreen)
)

// Setup configures the global slog logger. dir is the directory under
// which .deepseek/log/ is created.
func Setup(debug bool, mode Mode, dir string) *slog.Logger {
	if !debug {
		logger := slog.New(slog.NewTextHandler(io.Discard, nil))
		slog.SetDefault(logger)
		return logger
	}

	var writers []io.Writer
	if mode == ModeCLI {
		writers = append(writers, os.Stderr)
	}

	logDir := filepath.Join(dir, ".deepseek", "log")
	if err := os.MkdirAll(logDir, 0o755); err == nil {
		path := filepath.Join(logDir, time.Now().Format("2006-01-02")+".jsonl")
		if f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644); err == nil {
			writers = append(writers, f)
		}
	}

	var w io.Writer
	switch len(writers) {
	case 0:
		w = io.Discard
	case 1:
		w = writers[0]
	default:
		w = io.MultiWriter(writers...)
	}

	logger := slog.New(slog.NewJSONHandler(w, &slog.HandlerOptions{Level: slog.LevelDebug}))
	slog.SetDefault(logger)
	return logger
}

// RedactAPIKey returns a shortened representation of a key-like string.
func RedactAPIKey(key string) string {
	if len(key) <= 8 {
		return "***"
	}
	return key[:4] + "..." + key[len(key)-4:]
}

// RedactPath returns "[redacted:<ext>]" if the basename looks like a
// secrets file; otherwise returns the path unchanged.
func RedactPath(path string) string {
	base := strings.ToLower(filepath.Base(path))
	patterns := []string{
		".env", ".pem", ".key", "id_rsa", "id_ed25519",
		"credentials", "secret", "token",
	}
	for _, pat := range patterns {
		if strings.Contains(base, pat) {
			return "[redacted:" + filepath.Ext(path) + "]"
		}
	}
	return path
}
