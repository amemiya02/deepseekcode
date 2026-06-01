# Notifications

deepseekcode can fire desktop or terminal notifications when the agent
finishes a long-running task or when a permission prompt needs attention.
This keeps you informed without staring at the terminal.

## When notifications fire

| Event | Title | Body |
|-------|-------|------|
| Agent run completes | DeepSeekCode | Task finished |
| Permission prompt appears | DeepSeekCode | Permission requested |

The notifier implementation decides how the platform surfaces them (desktop
toast, terminal bell, or silent discard). Notifications are best-effort and
never block the UI loop.

## Sensitive content policy

Notifications **never** include:

- File paths or directory listings
- Command output or tool results
- Secret patterns or credentials
- Model responses or reasoning content

Only the generic title and body above are sent. This is a deliberate design
choice — notification systems may log, display on lock screens, or forward
to other services.

## Disabling notifications

Notifications use a `Notifier` interface. By default, deepseekcode uses a
no-op notifier that discards all notifications. No configuration is needed
to disable them — they are off unless an integration provides a real
notifier.

To implement a custom notifier (e.g. for `terminal-notifier` on macOS,
`notify-send` on Linux, or Windows toast), satisfy the `Notifier`
interface:

```go
type Notifier interface {
    Notify(title, body string) error
}
```

Errors from `Notify` are silently discarded — notifications must never
disrupt the agent loop.

## Platform notes

- **macOS**: Terminal.app and iTerm2 support OSC 9 (terminal notifications)
  natively. Third-party notifiers like `terminal-notifier` can be wrapped.
- **Linux**: `notify-send` works on most desktop environments. Terminal
  notifications depend on the terminal emulator.
- **Windows**: Windows Terminal supports toast notifications for unfocused
  tabs.

No platform-specific dependencies are compiled into deepseekcode. The
no-op default works everywhere.
