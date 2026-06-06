package gateway

import (
	"github.com/amemiya02/deepseekcode/internal/mcp"
	"github.com/amemiya02/deepseekcode/internal/session"
	"github.com/amemiya02/deepseekcode/internal/snapshots"
)

// Option configures a Handler at construction time. Options are additive and
// optional: a Handler built without any keeps the pre-Wave-5 behaviour, so
// existing callers and tests are unaffected.
type Option func(*Handler)

// WithStore attaches a session.Store so checkpoint/rewind/branch endpoints can
// read and rewrite persisted message history.
func WithStore(s *session.Store) Option { return func(h *Handler) { h.store = s } }

// WithSnapshots attaches a snapshots.Manager so /v1/rewind with a code scope can
// roll back the working tree via the same pre-edit snapshots /undo uses.
func WithSnapshots(m *snapshots.Manager) Option { return func(h *Handler) { h.snaps = m } }

// WithWorkspaceRoot sets the directory the workspace file read (/v1/add-to-chat
// contents) is confined to. Empty means the process working dir.
func WithWorkspaceRoot(root string) Option { return func(h *Handler) { h.root = root } }

// WithMCPRegistry attaches the running MCP registry so GET /v1/mcp can report
// live connection status + tool counts.
func WithMCPRegistry(r *mcp.Registry) Option { return func(h *Handler) { h.mcpReg = r } }
