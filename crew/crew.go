// Package crew provides multi-agent orchestration via tmux sessions.
// It manages worker agent lifecycles, message passing, and session state
// backed by a SQLite database (jeff.db).
package crew

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

const (
	dbFile     = "jeff.db"
	timeLayout = "2006-01-02T15:04:05Z"
)

// Orchestrator represents an orchestrator agent running in a tmux session.
type Orchestrator struct {
	ID          string    `json:"id"`
	TmuxSession string    `json:"tmux_session"`
	TmuxWindow  string    `json:"tmux_window"`
	TmuxPane    string    `json:"tmux_pane"`
	StartedAt   time.Time `json:"started_at"`
	Status      string    `json:"status"` // running, stopped
	Agent       string    `json:"agent,omitempty"`
	Model       string    `json:"model,omitempty"`
	Dir         string    `json:"dir,omitempty"` // project directory (durable identities only)
}

// Session represents a worker agent running in a tmux window.
type Session struct {
	TaskID         string     `json:"task_id"`
	TmuxSession    string     `json:"tmux_session"`
	WindowName     string     `json:"window_name"`
	TmuxPane       string     `json:"tmux_pane,omitempty"`
	TaskDir        string     `json:"task_dir"`
	Persona        string     `json:"persona,omitempty"`
	Agent          string     `json:"agent,omitempty"` // agent CLI command (e.g. "claude", "gemini")
	Model          string     `json:"model,omitempty"`
	Repos          []string   `json:"repos,omitempty"`
	OrchestratorID string     `json:"orchestrator_id,omitempty"`
	PID            int        `json:"pid,omitempty"`
	Status         string     `json:"status"` // starting, running, done, failed, stopped
	StartedAt      time.Time  `json:"started_at"`
	StoppedAt      *time.Time `json:"stopped_at,omitempty"`
	LastSeen       time.Time  `json:"last_seen"`
	SessionIDs     []string   `json:"session_ids,omitempty"` // Claude session IDs captured via SessionStart hook
}

// LatestSessionID returns the most recently captured Claude session ID, or "" if none.
func (s *Session) LatestSessionID() string {
	if len(s.SessionIDs) == 0 {
		return ""
	}
	return s.SessionIDs[len(s.SessionIDs)-1]
}

// Message is a communication between orchestrator and worker.
type Message struct {
	ID        string     `json:"id"`
	TaskID    string     `json:"task_id"`
	Direction string     `json:"direction"` // to_worker, to_orchestrator
	Type      string     `json:"type"`
	Content   string     `json:"content"`
	Response  string     `json:"response,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	AckedAt   *time.Time `json:"acked_at,omitempty"`
}

// Store manages crew state in a SQLite database.
type Store struct {
	db *sql.DB
}

// Open creates or opens the crew database at JEFF_HOME/jeff.db.
func Open(jeffHome string) (*Store, error) {
	dbPath := filepath.Join(jeffHome, dbFile)

	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, fmt.Errorf("create db directory: %w", err)
	}

	// Apply pragmas via DSN so they stick to every pooled connection.
	dsn := "file:" + dbPath + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open crew database: %w", err)
	}
	db.SetMaxOpenConns(1)

	if err := migrate(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("run migrations: %w", err)
	}

	return &Store{db: db}, nil
}

// Close closes the underlying database connection.
func (s *Store) Close() error {
	return s.db.Close()
}

// DB returns the underlying *sql.DB for advanced use cases.
func (s *Store) DB() *sql.DB {
	return s.db
}

func migrate(db *sql.DB) error {
	var v int
	if err := db.QueryRow("PRAGMA user_version").Scan(&v); err != nil {
		return fmt.Errorf("read schema version: %w", err)
	}
	steps := []func(*sql.DB) error{
		migrateV1,
		migrateV2,
		migrateV3,
	}
	for i := v; i < len(steps); i++ {
		if err := steps[i](db); err != nil {
			return fmt.Errorf("migration to v%d: %w", i+1, err)
		}
		if _, err := db.Exec(fmt.Sprintf("PRAGMA user_version = %d", i+1)); err != nil {
			return fmt.Errorf("bump schema version: %w", err)
		}
	}
	return nil
}

func migrateV1(db *sql.DB) error {
	schema := `
CREATE TABLE IF NOT EXISTS orchestrators (
    id           TEXT PRIMARY KEY,
    tmux_session TEXT NOT NULL,
    tmux_window  TEXT NOT NULL DEFAULT '',
    tmux_pane    TEXT NOT NULL DEFAULT '',
    started_at   TEXT NOT NULL,
    status       TEXT NOT NULL DEFAULT 'running',
    agent        TEXT DEFAULT '',
    model        TEXT DEFAULT ''
);

CREATE TABLE IF NOT EXISTS sessions (
    task_id         TEXT PRIMARY KEY,
    tmux_session    TEXT NOT NULL DEFAULT 'jeff',
    window_name     TEXT NOT NULL,
    tmux_pane       TEXT NOT NULL DEFAULT '',
    task_dir        TEXT NOT NULL,
    persona         TEXT DEFAULT '',
    repos           TEXT DEFAULT '[]',
    orchestrator_id TEXT DEFAULT '',
    pid             INTEGER DEFAULT 0,
    status          TEXT NOT NULL,
    started_at      TEXT NOT NULL,
    stopped_at      TEXT,
    last_seen       TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS messages (
    id         TEXT PRIMARY KEY,
    task_id    TEXT NOT NULL,
    direction  TEXT NOT NULL,
    msg_type   TEXT NOT NULL,
    content    TEXT NOT NULL,
    response   TEXT DEFAULT '',
    created_at TEXT NOT NULL,
    acked_at   TEXT,
    FOREIGN KEY (task_id) REFERENCES sessions(task_id)
);

CREATE INDEX IF NOT EXISTS idx_messages_pending
    ON messages(task_id, direction, acked_at);
CREATE INDEX IF NOT EXISTS idx_messages_task
    ON messages(task_id, created_at);
`
	_, err := db.Exec(schema)
	return err
}

func migrateV2(db *sql.DB) error {
	// Additive migrations for existing databases.
	addCol := []string{
		"ALTER TABLE sessions ADD COLUMN orchestrator_id TEXT DEFAULT ''",
		"ALTER TABLE sessions ADD COLUMN tmux_pane TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE sessions ADD COLUMN model TEXT DEFAULT ''",
		"ALTER TABLE sessions ADD COLUMN session_ids TEXT DEFAULT '[]'",
		"ALTER TABLE sessions ADD COLUMN agent TEXT DEFAULT ''",
		"ALTER TABLE orchestrators ADD COLUMN agent TEXT DEFAULT ''",
		"ALTER TABLE orchestrators ADD COLUMN model TEXT DEFAULT ''",
	}
	for _, stmt := range addCol {
		_, err := db.Exec(stmt)
		if err != nil && !strings.Contains(err.Error(), "duplicate column") {
			return err
		}
	}

	// One-time cleanup (gig-be5c §5 / gig-9c92 Option D): older rows stored an
	// unsanitized window_name (e.g. "gig-f4e8.2") while the real tmux window is
	// "gig-f4e8-2" (tmux turns dots into hyphens). Sanitize in place so Stop/Send
	// targeting resolves. Idempotent — once dots are gone the WHERE clause matches
	// nothing — and a no-op on empty databases.
	if _, err := db.Exec(
		`UPDATE sessions SET window_name = REPLACE(window_name, '.', '-') WHERE window_name LIKE '%.%'`,
	); err != nil {
		return fmt.Errorf("sanitize dotted window_name rows: %w", err)
	}

	return nil
}

func migrateV3(db *sql.DB) error {
	_, err := db.Exec("ALTER TABLE orchestrators ADD COLUMN dir TEXT DEFAULT ''")
	if err != nil && !strings.Contains(err.Error(), "duplicate column") {
		return err
	}
	return nil
}

// --- Session CRUD ---

// PutSession inserts or updates a session.
// session_ids is intentionally excluded from the DO UPDATE SET clause so that
// AppendSessionID is the only way to grow the list after initial insert.
func (s *Store) PutSession(sess *Session) error {
	repos, err := json.Marshal(sess.Repos)
	if err != nil {
		return fmt.Errorf("marshal repos: %w", err)
	}
	sessionIDs, err := json.Marshal(sess.SessionIDs)
	if err != nil {
		return fmt.Errorf("marshal session_ids: %w", err)
	}

	var stoppedAt *string
	if sess.StoppedAt != nil {
		v := sess.StoppedAt.UTC().Format(timeLayout)
		stoppedAt = &v
	}

	_, err = s.db.Exec(`
		INSERT INTO sessions (task_id, tmux_session, window_name, tmux_pane, task_dir, persona, agent, model, repos, orchestrator_id, pid, status, started_at, stopped_at, last_seen, session_ids)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(task_id) DO UPDATE SET
			tmux_session    = excluded.tmux_session,
			window_name     = excluded.window_name,
			tmux_pane       = excluded.tmux_pane,
			task_dir        = excluded.task_dir,
			persona         = excluded.persona,
			agent           = excluded.agent,
			model           = excluded.model,
			repos           = excluded.repos,
			orchestrator_id = excluded.orchestrator_id,
			pid             = excluded.pid,
			status          = excluded.status,
			stopped_at      = excluded.stopped_at,
			last_seen       = excluded.last_seen`,
		sess.TaskID, sess.TmuxSession, sess.WindowName, sess.TmuxPane, sess.TaskDir,
		sess.Persona, sess.Agent, sess.Model, string(repos), sess.OrchestratorID, sess.PID, sess.Status,
		sess.StartedAt.UTC().Format(timeLayout), stoppedAt,
		sess.LastSeen.UTC().Format(timeLayout), string(sessionIDs),
	)
	return err
}

// AppendSessionID appends a Claude session ID to the session_ids array for a task.
func (s *Store) AppendSessionID(taskID, sessionID string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Read existing array, append, write back atomically.
	var raw string
	err = tx.QueryRow(`SELECT COALESCE(session_ids, '[]') FROM sessions WHERE task_id = ?`, taskID).Scan(&raw)
	if err != nil {
		return fmt.Errorf("get session_ids for %s: %w", taskID, err)
	}

	var ids []string
	if err := json.Unmarshal([]byte(raw), &ids); err != nil {
		ids = []string{}
	}
	ids = append(ids, sessionID)

	updated, err := json.Marshal(ids)
	if err != nil {
		return fmt.Errorf("marshal session_ids: %w", err)
	}

	if _, err = tx.Exec(`UPDATE sessions SET session_ids = ? WHERE task_id = ?`, string(updated), taskID); err != nil {
		return err
	}
	return tx.Commit()
}

// GetSession retrieves a session by task ID.
func (s *Store) GetSession(taskID string) (*Session, error) {
	row := s.db.QueryRow(`
		SELECT task_id, tmux_session, window_name, tmux_pane, task_dir, persona, COALESCE(agent, ''), COALESCE(model, ''), repos,
		       orchestrator_id, pid, status, started_at, stopped_at, last_seen,
		       COALESCE(session_ids, '[]')
		FROM sessions WHERE task_id = ?`, taskID)
	return scanSession(row)
}

// ListSessions returns sessions, with optional filters.
// If activeOnly is true, excludes terminal statuses (done, failed, stopped).
// If orchestratorID is non-empty, only sessions belonging to that orchestrator are returned.
func (s *Store) ListSessions(activeOnly bool, orchestratorID string) ([]*Session, error) {
	query := `SELECT task_id, tmux_session, window_name, tmux_pane, task_dir, persona, COALESCE(agent, ''), COALESCE(model, ''), repos,
	                 orchestrator_id, pid, status, started_at, stopped_at, last_seen,
	                 COALESCE(session_ids, '[]')
	          FROM sessions`

	var args []any
	switch {
	case activeOnly && orchestratorID != "":
		query += ` WHERE status NOT IN ('done', 'failed', 'stopped') AND orchestrator_id = ?`
		args = append(args, orchestratorID)
	case activeOnly:
		query += ` WHERE status NOT IN ('done', 'failed', 'stopped')`
	case orchestratorID != "":
		query += ` WHERE orchestrator_id = ?`
		args = append(args, orchestratorID)
	}
	query += ` ORDER BY started_at DESC`

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}
	defer rows.Close()

	var sessions []*Session
	for rows.Next() {
		sess, err := scanSessionRow(rows)
		if err != nil {
			return nil, err
		}
		sessions = append(sessions, sess)
	}
	return sessions, rows.Err()
}

// RemoveSession deletes a session and its messages.
func (s *Store) RemoveSession(taskID string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec(`DELETE FROM messages WHERE task_id = ?`, taskID); err != nil {
		return fmt.Errorf("delete messages: %w", err)
	}
	if _, err = tx.Exec(`DELETE FROM sessions WHERE task_id = ?`, taskID); err != nil {
		return err
	}
	return tx.Commit()
}

// UpdateStatus updates a session's status and optionally sets stopped_at.
func (s *Store) UpdateStatus(taskID, status string) error {
	now := time.Now().UTC().Format(timeLayout)
	var stoppedAt *string
	if status == "done" || status == "failed" || status == "stopped" {
		stoppedAt = &now
	}
	_, err := s.db.Exec(`
		UPDATE sessions SET status = ?, stopped_at = COALESCE(?, stopped_at), last_seen = ?
		WHERE task_id = ?`,
		status, stoppedAt, now, taskID)
	return err
}

// TouchSession updates last_seen for a session.
func (s *Store) TouchSession(taskID string) error {
	now := time.Now().UTC().Format(timeLayout)
	_, err := s.db.Exec(`UPDATE sessions SET last_seen = ? WHERE task_id = ?`, now, taskID)
	return err
}

// --- Orchestrator CRUD ---

// PutOrchestrator inserts or updates an orchestrator record.
func (s *Store) PutOrchestrator(o *Orchestrator) error {
	_, err := s.db.Exec(`
		INSERT INTO orchestrators (id, tmux_session, tmux_window, tmux_pane, started_at, status, agent, model, dir)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			tmux_session = excluded.tmux_session,
			tmux_window  = excluded.tmux_window,
			tmux_pane    = excluded.tmux_pane,
			status       = excluded.status,
			agent        = excluded.agent,
			model        = excluded.model,
			dir          = excluded.dir`,
		o.ID, o.TmuxSession, o.TmuxWindow, o.TmuxPane,
		o.StartedAt.UTC().Format(timeLayout), o.Status,
		o.Agent, o.Model, o.Dir,
	)
	return err
}

// GetOrchestrator retrieves an orchestrator by ID.
func (s *Store) GetOrchestrator(id string) (*Orchestrator, error) {
	var o Orchestrator
	var startedAt string
	err := s.db.QueryRow(`
		SELECT id, tmux_session, tmux_window, tmux_pane, started_at, status,
		       COALESCE(agent, ''), COALESCE(model, ''), COALESCE(dir, '')
		FROM orchestrators WHERE id = ?`, id).Scan(
		&o.ID, &o.TmuxSession, &o.TmuxWindow, &o.TmuxPane, &startedAt, &o.Status,
		&o.Agent, &o.Model, &o.Dir,
	)
	if err != nil {
		return nil, err
	}
	o.StartedAt = parseTime(startedAt)
	return &o, nil
}

// OrchestratorByPane returns the ID of the running orchestrator bound to the
// given tmux pane, or "" if none is bound. This is the durable per-pane identity
// binding (gig-9c92 Option A): because $TMUX_PANE is stable across shell restarts
// and Claude Code relaunches within the same pane, looking up the orchestrator by
// pane survives the process churn that broke session-name-based detection.
func (s *Store) OrchestratorByPane(paneID string) (string, error) {
	if paneID == "" {
		return "", nil
	}
	var id string
	err := s.db.QueryRow(
		`SELECT id FROM orchestrators
		 WHERE tmux_pane = ? AND status = 'running'
		 ORDER BY started_at DESC LIMIT 1`, paneID).Scan(&id)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return id, nil
}

// ListOrchestrators returns orchestrators. If activeOnly, filters to status='running'.
func (s *Store) ListOrchestrators(activeOnly bool) ([]*Orchestrator, error) {
	query := `SELECT id, tmux_session, tmux_window, tmux_pane, started_at, status,
	                 COALESCE(agent, ''), COALESCE(model, ''), COALESCE(dir, '') FROM orchestrators`
	if activeOnly {
		query += ` WHERE status = 'running'`
	}
	query += ` ORDER BY started_at DESC`

	rows, err := s.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*Orchestrator
	for rows.Next() {
		var o Orchestrator
		var startedAt string
		if err := rows.Scan(&o.ID, &o.TmuxSession, &o.TmuxWindow, &o.TmuxPane, &startedAt, &o.Status, &o.Agent, &o.Model, &o.Dir); err != nil {
			return nil, err
		}
		o.StartedAt = parseTime(startedAt)
		result = append(result, &o)
	}
	return result, rows.Err()
}

// NextOrchestratorID returns the next auto-increment session name (jeff-1, jeff-2, ...).
func (s *Store) NextOrchestratorID() (string, error) {
	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM orchestrators`).Scan(&count)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("jeff-%d", count+1), nil
}

// UpdateOrchestratorStatus updates an orchestrator's status.
func (s *Store) UpdateOrchestratorStatus(id, status string) error {
	_, err := s.db.Exec(`UPDATE orchestrators SET status = ? WHERE id = ?`, status, id)
	return err
}

// DeleteOrchestrator removes an orchestrator's row entirely (used by
// `jeff orchestrator rm`). Callers are responsible for checking and warning
// about bound live workers first — this only removes the DB row, it does not
// touch sessions.orchestrator_id on any worker rows that referenced it.
func (s *Store) DeleteOrchestrator(id string) error {
	_, err := s.db.Exec(`DELETE FROM orchestrators WHERE id = ?`, id)
	return err
}

// Orchestrator status values (#86: derived from reality, not stored as a
// wish). A durable identity (TmuxSession == "", created by `orchestrator
// init` rather than `orchestrator start`) has three honest states; a
// tmux-session-bound orchestrator (`orchestrator start`) uses only Running/
// Stopped, reconciled by Refresh.
const (
	// OrchStatusRegistered: an identity exists on disk; no process is bound
	// to it. This is a real, useful state on its own — not a bug — for a
	// durable identity created outside tmux (Cursor, VS Code, a plain
	// terminal, CI) that survives shell restarts.
	OrchStatusRegistered = "registered"
	// OrchStatusRunning: a live tmux pane hosts it (pane probe says alive).
	OrchStatusRunning = "running"
	// OrchStatusStopped: it ran and its pane is gone.
	OrchStatusStopped = "stopped"
)

// DeriveDurableOrchestratorStatus computes the live status of a durable
// orchestrator identity — one with no bound tmux session (TmuxSession == "",
// created by `orchestrator init` rather than `orchestrator start`; a
// session-bound orchestrator is Refresh's territory and reconciled there).
//
// Mirrors the pane-probe pattern Refresh uses for workers: a probe error is
// "unknown, not death" and never downgrades a live-looking orchestrator to
// stopped on a single transient tmux hiccup — it returns o.Status unchanged.
// paneIsDead is injected (pass PaneIsDead in production) so this is
// unit-testable without a real tmux.
func DeriveDurableOrchestratorStatus(o *Orchestrator, paneIsDead func(target string) (bool, error)) string {
	if o.TmuxPane == "" {
		return OrchStatusRegistered
	}
	dead, err := paneIsDead(o.TmuxPane)
	if err != nil {
		return o.Status
	}
	if dead {
		return OrchStatusStopped
	}
	return OrchStatusRunning
}

// WorkersForOrchestrator returns sessions belonging to an orchestrator.
func (s *Store) WorkersForOrchestrator(orchestratorID string) ([]*Session, error) {
	query := `SELECT task_id, tmux_session, window_name, tmux_pane, task_dir, persona, COALESCE(agent, ''), COALESCE(model, ''), repos,
	                 orchestrator_id, pid, status, started_at, stopped_at, last_seen,
	                 COALESCE(session_ids, '[]')
	          FROM sessions WHERE orchestrator_id = ? ORDER BY started_at DESC`
	rows, err := s.db.Query(query, orchestratorID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []*Session
	for rows.Next() {
		sess, err := scanSessionRow(rows)
		if err != nil {
			return nil, err
		}
		sessions = append(sessions, sess)
	}
	return sessions, rows.Err()
}

// PendingOrchestratorMessages returns unacked to_orchestrator messages
// for all workers belonging to the given orchestrator.
func (s *Store) PendingOrchestratorMessages(orchestratorID string) ([]*Message, error) {
	rows, err := s.db.Query(`
		SELECT m.id, m.task_id, m.direction, m.msg_type, m.content, m.response, m.created_at, m.acked_at
		FROM messages m
		JOIN sessions s ON m.task_id = s.task_id
		WHERE s.orchestrator_id = ? AND m.direction = 'to_orchestrator' AND m.acked_at IS NULL
		ORDER BY m.created_at ASC`, orchestratorID)
	if err != nil {
		return nil, fmt.Errorf("query orchestrator messages: %w", err)
	}
	defer rows.Close()
	return scanMessages(rows)
}

// --- Message CRUD ---

// SendMessage inserts a new message.
func (s *Store) SendMessage(msg *Message) error {
	_, err := s.db.Exec(`
		INSERT INTO messages (id, task_id, direction, msg_type, content, response, created_at, acked_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		msg.ID, msg.TaskID, msg.Direction, string(msg.Type),
		msg.Content, msg.Response,
		msg.CreatedAt.UTC().Format(timeLayout), nil,
	)
	return err
}

// PendingMessages returns unacknowledged messages for a task in a direction.
func (s *Store) PendingMessages(taskID, direction string) ([]*Message, error) {
	rows, err := s.db.Query(`
		SELECT id, task_id, direction, msg_type, content, response, created_at, acked_at
		FROM messages
		WHERE task_id = ? AND direction = ? AND acked_at IS NULL
		ORDER BY created_at ASC`, taskID, direction)
	if err != nil {
		return nil, fmt.Errorf("query pending messages: %w", err)
	}
	defer rows.Close()
	return scanMessages(rows)
}

// PendingCount returns the number of unacknowledged messages for a task in a direction.
func (s *Store) PendingCount(taskID, direction string) (int, error) {
	var count int
	err := s.db.QueryRow(`
		SELECT COUNT(*) FROM messages
		WHERE task_id = ? AND direction = ? AND acked_at IS NULL`,
		taskID, direction).Scan(&count)
	return count, err
}

// PendingCountByType returns the number of unacknowledged messages for a task
// in a direction that also match a given msg_type. Used to de-duplicate durable
// worker→orchestrator signals so repeated turn-end pings collapse into one row.
func (s *Store) PendingCountByType(taskID, direction, msgType string) (int, error) {
	var count int
	err := s.db.QueryRow(`
		SELECT COUNT(*) FROM messages
		WHERE task_id = ? AND direction = ? AND msg_type = ? AND acked_at IS NULL`,
		taskID, direction, msgType).Scan(&count)
	return count, err
}

// AckMessage acknowledges a single message and optionally stores a response.
func (s *Store) AckMessage(msgID, response string) error {
	now := time.Now().UTC().Format(timeLayout)
	_, err := s.db.Exec(`
		UPDATE messages SET acked_at = ?, response = CASE WHEN ? != '' THEN ? ELSE response END
		WHERE id = ?`,
		now, response, response, msgID)
	return err
}

// AckAll acknowledges all pending messages for a task in a direction.
func (s *Store) AckAll(taskID, direction string) error {
	now := time.Now().UTC().Format(timeLayout)
	_, err := s.db.Exec(`
		UPDATE messages SET acked_at = ?
		WHERE task_id = ? AND direction = ? AND acked_at IS NULL`,
		now, taskID, direction)
	return err
}

// RecentMessages returns the most recent messages for a task (any direction).
func (s *Store) RecentMessages(taskID string, limit int) ([]*Message, error) {
	rows, err := s.db.Query(`
		SELECT id, task_id, direction, msg_type, content, response, created_at, acked_at
		FROM messages
		WHERE task_id = ?
		ORDER BY created_at DESC
		LIMIT ?`, taskID, limit)
	if err != nil {
		return nil, fmt.Errorf("query recent messages: %w", err)
	}
	defer rows.Close()
	return scanMessages(rows)
}

// --- Scanning helpers ---

type scannable interface {
	Scan(dest ...any) error
}

func scanSession(row scannable) (*Session, error) {
	var sess Session
	var repos, sessionIDs, startedAt, lastSeen string
	var stoppedAt *string

	err := row.Scan(
		&sess.TaskID, &sess.TmuxSession, &sess.WindowName, &sess.TmuxPane, &sess.TaskDir,
		&sess.Persona, &sess.Agent, &sess.Model, &repos, &sess.OrchestratorID, &sess.PID, &sess.Status,
		&startedAt, &stoppedAt, &lastSeen, &sessionIDs,
	)
	if err != nil {
		return nil, err
	}

	_ = json.Unmarshal([]byte(repos), &sess.Repos)
	_ = json.Unmarshal([]byte(sessionIDs), &sess.SessionIDs)
	sess.StartedAt = parseTime(startedAt)
	sess.LastSeen = parseTime(lastSeen)
	if stoppedAt != nil {
		t := parseTime(*stoppedAt)
		sess.StoppedAt = &t
	}
	return &sess, nil
}

func scanSessionRow(rows *sql.Rows) (*Session, error) {
	return scanSession(rows)
}

func scanMessages(rows *sql.Rows) ([]*Message, error) {
	var msgs []*Message
	for rows.Next() {
		var msg Message
		var msgType, createdAt string
		var ackedAt, response *string

		err := rows.Scan(
			&msg.ID, &msg.TaskID, &msg.Direction, &msgType,
			&msg.Content, &response, &createdAt, &ackedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan message: %w", err)
		}

		msg.Type = msgType
		msg.CreatedAt = parseTime(createdAt)
		if ackedAt != nil {
			t := parseTime(*ackedAt)
			msg.AckedAt = &t
		}
		if response != nil {
			msg.Response = *response
		}
		msgs = append(msgs, &msg)
	}
	return msgs, rows.Err()
}

func parseTime(s string) time.Time {
	t, _ := time.Parse(timeLayout, s)
	return t
}
