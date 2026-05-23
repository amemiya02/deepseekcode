// Package session implements the SQLite-backed conversation store.
//
// Schema and design rationale live in docs/design.md §9. Key choices:
//   - pure-Go sqlite (modernc.org/sqlite) so cross-compile stays CGO-free
//   - one global DB at ~/.deepseek/sessions.db (not per-project)
//   - branching by reference: child sessions store parent_id +
//     branch_point and replay the parent's messages up to that index
//     instead of copying them
//
// Migrations are applied idempotently at Open time. The current schema
// is v1; future bumps live in migrations.go (not yet written).
package session

const schemaV1 = `
CREATE TABLE IF NOT EXISTS schema_version (
    version INTEGER PRIMARY KEY
);

CREATE TABLE IF NOT EXISTS sessions (
    id              TEXT PRIMARY KEY,
    parent_id       TEXT,
    branch_point    INTEGER,
    project_path    TEXT NOT NULL,
    model           TEXT NOT NULL,
    duet_enabled    INTEGER NOT NULL,
    created_at      INTEGER NOT NULL,
    last_used_at    INTEGER NOT NULL,
    summary         TEXT,
    FOREIGN KEY (parent_id) REFERENCES sessions(id) ON DELETE SET NULL
);

CREATE TABLE IF NOT EXISTS messages (
    session_id        TEXT NOT NULL,
    idx               INTEGER NOT NULL,
    role              TEXT NOT NULL,
    content           TEXT,
    reasoning_content TEXT,
    tool_calls        TEXT,
    tool_results      TEXT,
    tool_call_id      TEXT,
    model             TEXT,
    cache_hit_tokens  INTEGER,
    miss_tokens       INTEGER,
    output_tokens     INTEGER,
    cost_yuan         REAL,
    ts                INTEGER NOT NULL,
    PRIMARY KEY (session_id, idx),
    FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_sessions_project  ON sessions(project_path, last_used_at DESC);
CREATE INDEX IF NOT EXISTS idx_sessions_parent   ON sessions(parent_id);
CREATE INDEX IF NOT EXISTS idx_messages_session  ON messages(session_id, idx);
`

const currentSchemaVersion = 1
