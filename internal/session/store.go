package session

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"

	"github.com/amemiya02/deepseekcode/internal/llm"
)

// Store is the SQLite-backed session store. Safe for concurrent use
// (database/sql handles pooling). Open once per process.
type Store struct {
	db   *sql.DB
	path string
}

// Session is a row in the sessions table.
type Session struct {
	ID                string
	ParentID          string // empty if root
	BranchPoint       int    // valid only when ParentID != ""
	ProjectPath       string
	WorkspaceFP       string // FNV-1a fingerprint of the canonical project path
	Model             string
	DuetEnabled       bool
	CreatedAt         time.Time
	LastUsedAt        time.Time
	Summary           string
	CompactionCount   int
	CompactionSummary string
}

// Message is a row in the messages table.
//
// Blocks is the v2 canonical content; when non-nil the
// Content/ReasoningContent/ToolCalls fields are not read. Until
// T-019 removes them, both representations may coexist on a single
// in-memory row so legacy paths keep working during migration.
type Message struct {
	Idx              int
	Role             string
	Blocks           []llm.ContentBlock
	Content          string
	ReasoningContent string
	ToolCalls        []llm.ToolCall
	ToolCallID       string
	Model            string
	Usage            llm.Usage
	CostYuan         float64
	Timestamp        time.Time
}

// Open opens (and migrates) the store at path. The parent directory is
// created if missing. If path is empty, ~/.deepseek/sessions.db is used.
func Open(path string) (*Store, error) {
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("resolving home dir: %w", err)
		}
		path = filepath.Join(home, ".deepseek", "sessions.db")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("mkdir for db: %w", err)
	}

	// sqlite DSN: enable WAL for better concurrency, foreign keys, and
	// a busy timeout so the rare contention doesn't fail.
	dsn := path + "?_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening sqlite at %s: %w", path, err)
	}
	db.SetMaxOpenConns(1) // sqlite is single-writer; this avoids "database is locked"
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("pinging sqlite: %w", err)
	}
	s := &Store{db: db, path: path}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

// Path returns the on-disk DB path; useful for diagnostics.
func (s *Store) Path() string { return s.path }

// Close releases the DB handle.
func (s *Store) Close() error {
	if s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) migrate() error {
	if _, err := s.db.Exec(schemaV1); err != nil {
		return fmt.Errorf("applying schema v1: %w", err)
	}
	var existing int
	row := s.db.QueryRow(`SELECT COALESCE(MAX(version), 0) FROM schema_version`)
	if err := row.Scan(&existing); err != nil {
		return fmt.Errorf("reading schema_version: %w", err)
	}

	// Brand-new DB: mark v1 before considering the v2 step, but no
	// backup needed since there is no prior user data to lose.
	if existing == 0 {
		if _, err := s.db.Exec(`INSERT INTO schema_version(version) VALUES (1)`); err != nil {
			return fmt.Errorf("recording schema_version v1: %w", err)
		}
		existing = 1
	} else if existing == 1 {
		// Existing v1 DB with user data — back it up before ALTERs so a
		// failed migration can be hand-rolled back.
		if err := s.backupForV2(); err != nil {
			return fmt.Errorf("backing up v1 db: %w", err)
		}
	}

	if existing < 2 {
		if _, err := s.db.Exec(schemaV2); err != nil {
			return fmt.Errorf("applying schema v2: %w", err)
		}
		if _, err := s.db.Exec(`INSERT INTO schema_version(version) VALUES (2)`); err != nil {
			return fmt.Errorf("recording schema_version v2: %w", err)
		}
	}

	if existing < 3 {
		if _, err := s.db.Exec(schemaV3); err != nil {
			return fmt.Errorf("applying schema v3: %w", err)
		}
		if _, err := s.db.Exec(`INSERT INTO schema_version(version) VALUES (3)`); err != nil {
			return fmt.Errorf("recording schema_version v3: %w", err)
		}
	}
	return nil
}

// backupForV2 snapshots the current DB file to <path>.bak.v1 before
// running irreversible ALTER TABLE statements. Uses a WAL checkpoint
// to fold pending writes into the main file, then copies the file
// byte-for-byte via Go IO (no shell `cp`).
func (s *Store) backupForV2() error {
	if _, err := s.db.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		return fmt.Errorf("checkpoint: %w", err)
	}
	bakPath := s.path + ".bak.v1"
	src, err := os.Open(s.path)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer func() { _ = src.Close() }()
	dst, err := os.OpenFile(bakPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("create %s: %w", bakPath, err)
	}
	defer func() { _ = dst.Close() }()
	if _, err := io.Copy(dst, src); err != nil {
		return fmt.Errorf("copy db file: %w", err)
	}
	return dst.Sync()
}

// NewSession inserts a new (root) session and returns its row.
// Use NewBranch for child sessions.
func (s *Store) NewSession(ctx context.Context, projectPath, model string, duetEnabled bool) (Session, error) {
	fp, err := Fingerprint(projectPath)
	if err != nil {
		return Session{}, fmt.Errorf("fingerprint: %w", err)
	}
	now := time.Now().UTC()
	sess := Session{
		ID:          uuid.NewString(),
		ProjectPath: projectPath,
		WorkspaceFP: fp,
		Model:       model,
		DuetEnabled: duetEnabled,
		CreatedAt:   now,
		LastUsedAt:  now,
	}
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO sessions(id, parent_id, branch_point, project_path, workspace_fp, model, duet_enabled, created_at, last_used_at, summary)
		 VALUES (?, NULL, NULL, ?, ?, ?, ?, ?, ?, '')`,
		sess.ID, sess.ProjectPath, sess.WorkspaceFP, sess.Model, boolInt(sess.DuetEnabled),
		sess.CreatedAt.Unix(), sess.LastUsedAt.Unix())
	if err != nil {
		return Session{}, fmt.Errorf("insert session: %w", err)
	}
	return sess, nil
}

// NewBranch creates a child session referencing (parentID, branchPoint).
// Messages are NOT copied — Replay() walks parents at read time.
func (s *Store) NewBranch(ctx context.Context, parentID string, branchPoint int) (Session, error) {
	parent, err := s.GetSession(ctx, parentID)
	if err != nil {
		return Session{}, fmt.Errorf("loading parent: %w", err)
	}
	now := time.Now().UTC()
	child := Session{
		ID:          uuid.NewString(),
		ParentID:    parent.ID,
		BranchPoint: branchPoint,
		ProjectPath: parent.ProjectPath,
		WorkspaceFP: parent.WorkspaceFP,
		Model:       parent.Model,
		DuetEnabled: parent.DuetEnabled,
		CreatedAt:   now,
		LastUsedAt:  now,
	}
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO sessions(id, parent_id, branch_point, project_path, workspace_fp, model, duet_enabled, created_at, last_used_at, summary)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, '')`,
		child.ID, child.ParentID, child.BranchPoint, child.ProjectPath,
		child.WorkspaceFP, child.Model, boolInt(child.DuetEnabled),
		child.CreatedAt.Unix(), child.LastUsedAt.Unix())
	if err != nil {
		return Session{}, fmt.Errorf("insert branch: %w", err)
	}
	return child, nil
}

// GetSession reads one session by ID.
func (s *Store) GetSession(ctx context.Context, id string) (Session, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, COALESCE(parent_id, ''), COALESCE(branch_point, 0),
		        project_path, COALESCE(workspace_fp, ''), model, duet_enabled, created_at, last_used_at,
		        COALESCE(summary, ''), compaction_count, compaction_summary
		 FROM sessions WHERE id = ?`, id)
	var (
		sess          Session
		duet          int
		created, used int64
	)
	if err := row.Scan(&sess.ID, &sess.ParentID, &sess.BranchPoint,
		&sess.ProjectPath, &sess.WorkspaceFP, &sess.Model, &duet, &created, &used,
		&sess.Summary, &sess.CompactionCount, &sess.CompactionSummary); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Session{}, fmt.Errorf("session %s not found", id)
		}
		return Session{}, err
	}
	sess.DuetEnabled = duet != 0
	sess.CreatedAt = time.Unix(created, 0).UTC()
	sess.LastUsedAt = time.Unix(used, 0).UTC()
	return sess, nil
}

// MostRecentInProject returns the most recently used session for a given
// project path, or sql.ErrNoRows if none.
func (s *Store) MostRecentInProject(ctx context.Context, projectPath string) (Session, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id FROM sessions WHERE project_path = ? ORDER BY last_used_at DESC LIMIT 1`,
		projectPath)
	var id string
	if err := row.Scan(&id); err != nil {
		return Session{}, err
	}
	return s.GetSession(ctx, id)
}

// LatestInProject returns the most recently used session whose workspace
// fingerprint matches projectPath. Unlike MostRecentInProject, this uses
// the canonical fingerprint so symlinked or renamed paths match.
func (s *Store) LatestInProject(ctx context.Context, projectPath string) (Session, error) {
	fp, err := Fingerprint(projectPath)
	if err != nil {
		return Session{}, fmt.Errorf("fingerprint: %w", err)
	}
	row := s.db.QueryRowContext(ctx,
		`SELECT id FROM sessions WHERE workspace_fp = ? ORDER BY last_used_at DESC LIMIT 1`, fp)
	var id string
	if err := row.Scan(&id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Session{}, fmt.Errorf("no session found for workspace")
		}
		return Session{}, err
	}
	return s.GetSession(ctx, id)
}

// ListByProject returns all sessions for a project, newest first.
func (s *Store) ListByProject(ctx context.Context, projectPath string) ([]Session, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id FROM sessions WHERE project_path = ? ORDER BY last_used_at DESC`,
		projectPath)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	out := make([]Session, 0, len(ids))
	for _, id := range ids {
		s, err := s.GetSession(ctx, id)
		if err == nil {
			out = append(out, s)
		}
	}
	return out, nil
}

// ListAll returns all sessions globally, newest first. Used by `dsc -r`.
func (s *Store) ListAll(ctx context.Context) ([]Session, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id FROM sessions ORDER BY last_used_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	out := make([]Session, 0, len(ids))
	for _, id := range ids {
		s, err := s.GetSession(ctx, id)
		if err == nil {
			out = append(out, s)
		}
	}
	return out, nil
}

// TouchLastUsed updates last_used_at to now. Call on every meaningful
// interaction so the auto-resume heuristic stays accurate.
func (s *Store) TouchLastUsed(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE sessions SET last_used_at = ? WHERE id = ?`,
		time.Now().UTC().Unix(), id)
	return err
}

// UpdateModel persists a /models switch.
func (s *Store) UpdateModel(ctx context.Context, id, model string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE sessions SET model = ?, last_used_at = ? WHERE id = ?`,
		model, time.Now().UTC().Unix(), id)
	return err
}

// UpdateSummary persists an autogenerated short title.
func (s *Store) UpdateSummary(ctx context.Context, id, summary string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE sessions SET summary = ? WHERE id = ?`, summary, id)
	return err
}

// AppendMessage inserts a message at the next available index. The
// caller is expected to construct a Message in send-order; idx is
// auto-assigned (length of current messages).
func (s *Store) AppendMessage(ctx context.Context, sessionID string, m Message) (int, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	var maxIdx sql.NullInt64
	if err := tx.QueryRowContext(ctx,
		`SELECT MAX(idx) FROM messages WHERE session_id = ?`, sessionID).Scan(&maxIdx); err != nil {
		return 0, err
	}
	nextIdx := 0
	if maxIdx.Valid {
		nextIdx = int(maxIdx.Int64) + 1
	}

	toolCallsJSON := ""
	if len(m.ToolCalls) > 0 {
		b, err := json.Marshal(m.ToolCalls)
		if err != nil {
			return 0, fmt.Errorf("marshal tool_calls: %w", err)
		}
		toolCallsJSON = string(b)
	}
	blocksJSON := ""
	if len(m.Blocks) > 0 {
		b, err := llm.MarshalBlocks(m.Blocks)
		if err != nil {
			return 0, fmt.Errorf("marshal blocks: %w", err)
		}
		blocksJSON = string(b)
	}

	ts := m.Timestamp
	if ts.IsZero() {
		ts = time.Now().UTC()
	}
	cost := m.CostYuan
	if cost == 0 && m.Model != "" {
		cost = llm.Cost(m.Model, m.Usage)
	}

	_, err = tx.ExecContext(ctx,
		`INSERT INTO messages
		 (session_id, idx, role, content, reasoning_content, tool_calls,
		  tool_results, tool_call_id, model,
		  cache_hit_tokens, miss_tokens, output_tokens, cost_yuan, ts, blocks)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		sessionID, nextIdx, m.Role, m.Content, m.ReasoningContent, toolCallsJSON,
		"" /* tool_results — reserved */, m.ToolCallID, m.Model,
		m.Usage.PromptCacheHitTokens, m.Usage.PromptCacheMissTokens, m.Usage.CompletionTokens,
		cost, ts.Unix(), blocksJSON)
	if err != nil {
		return 0, fmt.Errorf("insert message: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE sessions SET last_used_at = ? WHERE id = ?`, ts.Unix(), sessionID); err != nil {
		return 0, fmt.Errorf("touch session: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return nextIdx, nil
}

// ReplaceWithCompaction atomically collapses messages in [fromIdx,
// toIdx) into a single synthetic system message containing the
// summary, then renumbers later messages so idx stays contiguous.
//
// Snapshots in this project are filesystem-backed (no SQL table to
// UPDATE), so step_idx shifting for snapshots lives in
// snapshots.Manager rather than here — see T-217.
//
// Returns the idx of the inserted summary row (== fromIdx). A
// no-op call (fromIdx == toIdx) returns fromIdx with nil error.
// toIdx larger than the current message count is clamped to count+1.
func (s *Store) ReplaceWithCompaction(ctx context.Context, sessionID string, fromIdx, toIdx int, summary string) (int, error) {
	if fromIdx < 0 || toIdx < fromIdx {
		return 0, fmt.Errorf("ReplaceWithCompaction: invalid range [%d,%d)", fromIdx, toIdx)
	}
	if fromIdx == toIdx {
		return fromIdx, nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	var maxIdx sql.NullInt64
	if err := tx.QueryRowContext(ctx,
		`SELECT MAX(idx) FROM messages WHERE session_id = ?`, sessionID).Scan(&maxIdx); err != nil {
		return 0, fmt.Errorf("read max idx: %w", err)
	}
	if maxIdx.Valid {
		if upper := int(maxIdx.Int64) + 1; toIdx > upper {
			toIdx = upper
		}
	}
	if fromIdx == toIdx {
		if err := tx.Commit(); err != nil {
			return 0, err
		}
		return fromIdx, nil
	}

	if _, err := tx.ExecContext(ctx,
		`DELETE FROM messages WHERE session_id = ? AND idx >= ? AND idx < ?`,
		sessionID, fromIdx, toIdx); err != nil {
		return 0, fmt.Errorf("delete range: %w", err)
	}

	shift := toIdx - fromIdx - 1
	if shift > 0 {
		if _, err := tx.ExecContext(ctx,
			`UPDATE messages SET idx = idx - ? WHERE session_id = ? AND idx >= ?`,
			shift, sessionID, toIdx); err != nil {
			return 0, fmt.Errorf("renumber: %w", err)
		}
	}

	blocksJSON, err := llm.MarshalBlocks([]llm.ContentBlock{llm.TextBlock{Text: summary}})
	if err != nil {
		return 0, fmt.Errorf("marshal summary blocks: %w", err)
	}
	ts := time.Now().UTC()
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO messages
		 (session_id, idx, role, content, reasoning_content, tool_calls,
		  tool_results, tool_call_id, model,
		  cache_hit_tokens, miss_tokens, output_tokens, cost_yuan, ts, blocks)
		 VALUES (?, ?, 'system', '', '', '', '', '', '', 0, 0, 0, 0, ?, ?)`,
		sessionID, fromIdx, ts.Unix(), string(blocksJSON)); err != nil {
		return 0, fmt.Errorf("insert summary: %w", err)
	}

	if _, err := tx.ExecContext(ctx,
		`UPDATE sessions SET compaction_count = compaction_count + 1, compaction_summary = ?, last_used_at = ? WHERE id = ?`,
		summary, ts.Unix(), sessionID); err != nil {
		return 0, fmt.Errorf("update session metadata: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return fromIdx, nil
}

// LoadMessages returns the message list for a session in idx order.
// Does NOT replay parents — use Replay for that.
//
// Blocks resolution: if the v2 blocks column is non-empty,
// UnmarshalBlocks restores the typed slice. Otherwise the row is
// pre-v2 and rebuildBlocksFromLegacy synthesizes blocks from
// content/reasoning/tool_calls so callers see a uniform Blocks view
// regardless of when the row was written.
func (s *Store) LoadMessages(ctx context.Context, sessionID string) ([]Message, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT idx, role, COALESCE(content,''), COALESCE(reasoning_content,''),
		        COALESCE(tool_calls,''), COALESCE(tool_call_id,''), COALESCE(model,''),
		        COALESCE(cache_hit_tokens,0), COALESCE(miss_tokens,0),
		        COALESCE(output_tokens,0), COALESCE(cost_yuan,0), ts,
		        COALESCE(blocks,'')
		 FROM messages WHERE session_id = ? ORDER BY idx ASC`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Message
	for rows.Next() {
		var (
			m         Message
			toolCalls string
			blocksRaw string
			ts        int64
		)
		if err := rows.Scan(&m.Idx, &m.Role, &m.Content, &m.ReasoningContent,
			&toolCalls, &m.ToolCallID, &m.Model,
			&m.Usage.PromptCacheHitTokens, &m.Usage.PromptCacheMissTokens,
			&m.Usage.CompletionTokens, &m.CostYuan, &ts, &blocksRaw); err != nil {
			return nil, err
		}
		if toolCalls != "" {
			_ = json.Unmarshal([]byte(toolCalls), &m.ToolCalls)
		}
		if blocksRaw != "" {
			blocks, err := llm.UnmarshalBlocks([]byte(blocksRaw))
			if err != nil {
				return nil, fmt.Errorf("session=%s idx=%d: unmarshal blocks: %w", sessionID, m.Idx, err)
			}
			m.Blocks = blocks
		} else {
			m.Blocks = rebuildBlocksFromLegacy(m.Role, m.Content, m.ReasoningContent, toolCalls, m.ToolCallID)
		}
		m.Timestamp = time.Unix(ts, 0).UTC()
		out = append(out, m)
	}
	return out, rows.Err()
}

// PruneOlderThan deletes sessions older than the given duration. The
// cascade also removes their messages.
func (s *Store) PruneOlderThan(ctx context.Context, older time.Duration) (int64, error) {
	cutoff := time.Now().Add(-older).Unix()
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM sessions WHERE last_used_at < ?`, cutoff)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
