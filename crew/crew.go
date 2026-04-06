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
	"time"

	_ "modernc.org/sqlite"
)

const (
	dbFile    = "jeff.db"
	timeLayout = "2006-01-02T15:04:05Z"
)

// Session represents a worker agent running in a tmux window.
type Session struct {
	TaskID      string     `json:"task_id"`
	TmuxSession string     `json:"tmux_session"`
	WindowName  string     `json:"window_name"`
	TaskDir     string     `json:"task_dir"`
	Persona     string     `json:"persona,omitempty"`
	Repos       []string   `json:"repos,omitempty"`
	PID         int        `json:"pid,omitempty"`
	Status      string     `json:"status"` // starting, running, done, failed, stopped
	StartedAt   time.Time  `json:"started_at"`
	StoppedAt   *time.Time `json:"stopped_at,omitempty"`
	LastSeen    time.Time  `json:"last_seen"`
}

// MessageType determines delivery mechanism and expected behavior.
type MessageType string

const (
	// MsgNudge is a one-way instruction delivered via PostToolUse hook.
	// Agent sees it in context, acks it, follows the instruction.
	MsgNudge MessageType = "nudge"

	// MsgStatus asks for status via /btw (sidechain, no context pollution).
	// Delivered by typing "/btw <content>" into the tmux pane.
	MsgStatus MessageType = "status"

	// MsgDivert interrupts the agent (C-c), then sends a new message.
	// Used to redirect focus. Heavy — stops current work.
	MsgDivert MessageType = "divert"

	// MsgNormal is a regular message typed into the agent's input.
	// Full context impact.
	MsgNormal MessageType = "normal"
)

// Message is a communication between orchestrator and worker.
type Message struct {
	ID        string      `json:"id"`
	TaskID    string      `json:"task_id"`
	Direction string      `json:"direction"` // to_worker, to_orchestrator
	Type      MessageType `json:"type"`
	Content   string      `json:"content"`
	Response  string      `json:"response,omitempty"`
	CreatedAt time.Time   `json:"created_at"`
	AckedAt   *time.Time  `json:"acked_at,omitempty"`
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

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open crew database: %w", err)
	}

	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("enable WAL: %w", err)
	}
	if _, err := db.Exec("PRAGMA busy_timeout=5000"); err != nil {
		db.Close()
		return nil, fmt.Errorf("set busy timeout: %w", err)
	}
	if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
		db.Close()
		return nil, fmt.Errorf("enable foreign keys: %w", err)
	}

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
	schema := `
CREATE TABLE IF NOT EXISTS sessions (
    task_id      TEXT PRIMARY KEY,
    tmux_session TEXT NOT NULL DEFAULT 'jeff',
    window_name  TEXT NOT NULL,
    task_dir     TEXT NOT NULL,
    persona      TEXT DEFAULT '',
    repos        TEXT DEFAULT '[]',
    pid          INTEGER DEFAULT 0,
    status       TEXT NOT NULL,
    started_at   TEXT NOT NULL,
    stopped_at   TEXT,
    last_seen    TEXT NOT NULL
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

// --- Session CRUD ---

// PutSession inserts or updates a session.
func (s *Store) PutSession(sess *Session) error {
	repos, err := json.Marshal(sess.Repos)
	if err != nil {
		return fmt.Errorf("marshal repos: %w", err)
	}

	var stoppedAt *string
	if sess.StoppedAt != nil {
		v := sess.StoppedAt.UTC().Format(timeLayout)
		stoppedAt = &v
	}

	_, err = s.db.Exec(`
		INSERT INTO sessions (task_id, tmux_session, window_name, task_dir, persona, repos, pid, status, started_at, stopped_at, last_seen)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(task_id) DO UPDATE SET
			tmux_session = excluded.tmux_session,
			window_name  = excluded.window_name,
			task_dir     = excluded.task_dir,
			persona      = excluded.persona,
			repos        = excluded.repos,
			pid          = excluded.pid,
			status       = excluded.status,
			stopped_at   = excluded.stopped_at,
			last_seen    = excluded.last_seen`,
		sess.TaskID, sess.TmuxSession, sess.WindowName, sess.TaskDir,
		sess.Persona, string(repos), sess.PID, sess.Status,
		sess.StartedAt.UTC().Format(timeLayout), stoppedAt,
		sess.LastSeen.UTC().Format(timeLayout),
	)
	return err
}

// GetSession retrieves a session by task ID.
func (s *Store) GetSession(taskID string) (*Session, error) {
	row := s.db.QueryRow(`
		SELECT task_id, tmux_session, window_name, task_dir, persona, repos,
		       pid, status, started_at, stopped_at, last_seen
		FROM sessions WHERE task_id = ?`, taskID)
	return scanSession(row)
}

// ListSessions returns sessions. If activeOnly is true, excludes terminal statuses.
func (s *Store) ListSessions(activeOnly bool) ([]*Session, error) {
	query := `SELECT task_id, tmux_session, window_name, task_dir, persona, repos,
	                 pid, status, started_at, stopped_at, last_seen
	          FROM sessions`
	if activeOnly {
		query += ` WHERE status NOT IN ('done', 'failed', 'stopped')`
	}
	query += ` ORDER BY started_at DESC`

	rows, err := s.db.Query(query)
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
	_, err := s.db.Exec(`DELETE FROM messages WHERE task_id = ?`, taskID)
	if err != nil {
		return fmt.Errorf("delete messages: %w", err)
	}
	_, err = s.db.Exec(`DELETE FROM sessions WHERE task_id = ?`, taskID)
	return err
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
	var repos, startedAt, lastSeen string
	var stoppedAt *string

	err := row.Scan(
		&sess.TaskID, &sess.TmuxSession, &sess.WindowName, &sess.TaskDir,
		&sess.Persona, &repos, &sess.PID, &sess.Status,
		&startedAt, &stoppedAt, &lastSeen,
	)
	if err != nil {
		return nil, err
	}

	_ = json.Unmarshal([]byte(repos), &sess.Repos)
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

		msg.Type = MessageType(msgType)
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
