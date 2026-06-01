package tui

// Notifier fires desktop or terminal notifications. Implementations must be
// safe for concurrent use. A nil Notifier is treated as a no-op by the App.
//
// The App calls Notify for two events:
//   - Agent run completion (EventDone)
//   - Permission request (EventPermissionAsk)
//
// The notifier implementation decides how the platform surfaces them
// (e.g. desktop toast, terminal bell, or silent discard).
//
// Notifications must NOT include sensitive content (file paths, command
// output, secret patterns). The title and body are intentionally generic.
type Notifier interface {
	Notify(title, body string) error
}

// NoopNotifier is a Notifier that silently discards all notifications.
// Used as the default when no desktop/terminal notification system is
// available.
type NoopNotifier struct{}

// Notify discards the notification. Always returns nil.
func (NoopNotifier) Notify(title, body string) error { return nil }
