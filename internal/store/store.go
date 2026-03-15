package store

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

type Token struct {
	ID         int64
	Name       string
	TokenHash  string
	CreatedAt  time.Time
	LastUsedAt *time.Time
	IsActive   bool
}

type Message struct {
	ID          int64
	TokenID     *int64
	TokenName   string
	Channel     string
	To          string
	Subject     string
	TextBody    string
	HTMLBody    string
	Status      string
	Attempts    int
	MaxAttempts int
	LastError   string
	CreatedAt   time.Time
	SentAt      *time.Time
	NextRetryAt *time.Time
}

type DashboardStats struct {
	TotalSent   int64
	TotalFailed int64
	TodaySent   int64
	TodayFailed int64
	QueueDepth  int64
	SuccessRate float64
}

type HourlyStat struct {
	Hour   time.Time
	Sent   int
	Failed int
	Total  int
}

func New(dsn string) (*Store, error) {
	// For in-memory databases, use shared cache so all connections see the same data
	if dsn == ":memory:" {
		dsn = "file::memory:?cache=shared"
	}

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	// SQLite serializes writes; limit connections to avoid lock contention
	db.SetMaxOpenConns(4)

	// Enable WAL mode for better concurrent access
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("setting WAL mode: %w", err)
	}
	if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
		db.Close()
		return nil, fmt.Errorf("enabling foreign keys: %w", err)
	}

	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("running migrations: %w", err)
	}
	return s, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) migrate() error {
	schema := `
	CREATE TABLE IF NOT EXISTS tokens (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL,
		token_hash TEXT NOT NULL UNIQUE,
		created_at DATETIME DEFAULT (datetime('now')),
		last_used_at DATETIME,
		is_active INTEGER DEFAULT 1
	);

	CREATE TABLE IF NOT EXISTS messages (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		token_id INTEGER REFERENCES tokens(id) ON DELETE SET NULL,
		to_addr TEXT NOT NULL,
		subject TEXT NOT NULL,
		text_body TEXT DEFAULT '',
		html_body TEXT DEFAULT '',
		status TEXT DEFAULT 'queued',
		attempts INTEGER DEFAULT 0,
		max_attempts INTEGER DEFAULT 3,
		last_error TEXT DEFAULT '',
		created_at DATETIME DEFAULT (datetime('now')),
		sent_at DATETIME,
		next_retry_at DATETIME
	);

	CREATE INDEX IF NOT EXISTS idx_messages_status ON messages(status, next_retry_at);
	CREATE INDEX IF NOT EXISTS idx_messages_created ON messages(created_at DESC);

	CREATE TABLE IF NOT EXISTS settings (
		key TEXT PRIMARY KEY,
		value TEXT NOT NULL
	);
	`
	if _, err := s.db.Exec(schema); err != nil {
		return err
	}
	// Add channel column (idempotent migration for existing databases)
	_, _ = s.db.Exec("ALTER TABLE messages ADD COLUMN channel TEXT DEFAULT 'mail'")
	return nil
}

// --- Token operations ---

func (s *Store) CreateToken(name string) (rawToken string, token Token, err error) {
	raw := make([]byte, 32)
	if _, err = rand.Read(raw); err != nil {
		return "", Token{}, fmt.Errorf("generating token: %w", err)
	}
	rawToken = hex.EncodeToString(raw)
	hash := hashToken(rawToken)

	result, err := s.db.Exec(
		"INSERT INTO tokens (name, token_hash) VALUES (?, ?)",
		name, hash,
	)
	if err != nil {
		return "", Token{}, fmt.Errorf("inserting token: %w", err)
	}

	id, _ := result.LastInsertId()
	token = Token{
		ID:       id,
		Name:     name,
		IsActive: true,
	}
	return rawToken, token, nil
}

func (s *Store) ListTokens() ([]Token, error) {
	rows, err := s.db.Query(
		"SELECT id, name, token_hash, created_at, last_used_at, is_active FROM tokens ORDER BY created_at DESC",
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tokens []Token
	for rows.Next() {
		var t Token
		var lastUsed sql.NullTime
		var active int
		if err := rows.Scan(&t.ID, &t.Name, &t.TokenHash, &t.CreatedAt, &lastUsed, &active); err != nil {
			return nil, err
		}
		if lastUsed.Valid {
			t.LastUsedAt = &lastUsed.Time
		}
		t.IsActive = active == 1
		tokens = append(tokens, t)
	}
	return tokens, rows.Err()
}

func (s *Store) ValidateToken(rawToken string) (*Token, error) {
	hash := hashToken(rawToken)
	var t Token
	var lastUsed sql.NullTime
	var active int
	err := s.db.QueryRow(
		"SELECT id, name, token_hash, created_at, last_used_at, is_active FROM tokens WHERE token_hash = ?",
		hash,
	).Scan(&t.ID, &t.Name, &t.TokenHash, &t.CreatedAt, &lastUsed, &active)
	if err != nil {
		return nil, err
	}
	if lastUsed.Valid {
		t.LastUsedAt = &lastUsed.Time
	}
	t.IsActive = active == 1

	if !t.IsActive {
		return nil, fmt.Errorf("token is disabled")
	}

	// Update last_used_at
	_, _ = s.db.Exec("UPDATE tokens SET last_used_at = datetime('now') WHERE id = ?", t.ID)

	return &t, nil
}

func (s *Store) ToggleToken(id int64) error {
	_, err := s.db.Exec("UPDATE tokens SET is_active = CASE WHEN is_active = 1 THEN 0 ELSE 1 END WHERE id = ?", id)
	return err
}

func (s *Store) DeleteToken(id int64) error {
	_, err := s.db.Exec("DELETE FROM tokens WHERE id = ?", id)
	return err
}

// --- Message operations ---

func (s *Store) EnqueueMessage(tokenID *int64, channel, to, subject, textBody, htmlBody string, maxRetries int) (*Message, error) {
	if channel == "" {
		channel = "mail"
	}
	result, err := s.db.Exec(
		"INSERT INTO messages (token_id, channel, to_addr, subject, text_body, html_body, max_attempts) VALUES (?, ?, ?, ?, ?, ?, ?)",
		tokenID, channel, to, subject, textBody, htmlBody, maxRetries,
	)
	if err != nil {
		return nil, fmt.Errorf("enqueuing message: %w", err)
	}

	id, _ := result.LastInsertId()
	return &Message{
		ID:          id,
		TokenID:     tokenID,
		Channel:     channel,
		To:          to,
		Subject:     subject,
		TextBody:    textBody,
		HTMLBody:    htmlBody,
		Status:      "queued",
		MaxAttempts: maxRetries,
	}, nil
}

func (s *Store) ClaimPendingMessages(limit int) ([]Message, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	// First, find the IDs to claim
	rows, err := tx.Query(`
		SELECT id FROM messages
		WHERE status = 'queued'
		AND (next_retry_at IS NULL OR next_retry_at <= datetime('now'))
		ORDER BY created_at ASC
		LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, tx.Commit()
	}

	// Build placeholders for IN clause
	placeholders := make([]string, len(ids))
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}
	inClause := strings.Join(placeholders, ",")

	// Update status
	_, err = tx.Exec(
		fmt.Sprintf("UPDATE messages SET status = 'sending' WHERE id IN (%s)", inClause),
		args...)
	if err != nil {
		return nil, err
	}

	// Fetch the claimed messages
	msgRows, err := tx.Query(
		fmt.Sprintf(`SELECT id, token_id, channel, to_addr, subject, text_body, html_body, attempts, max_attempts
		FROM messages WHERE id IN (%s) ORDER BY created_at ASC`, inClause),
		args...)
	if err != nil {
		return nil, err
	}
	defer msgRows.Close()

	var msgs []Message
	for msgRows.Next() {
		var m Message
		var tokenID sql.NullInt64
		if err := msgRows.Scan(&m.ID, &tokenID, &m.Channel, &m.To, &m.Subject, &m.TextBody, &m.HTMLBody, &m.Attempts, &m.MaxAttempts); err != nil {
			return nil, err
		}
		if tokenID.Valid {
			m.TokenID = &tokenID.Int64
		}
		msgs = append(msgs, m)
	}
	if err := msgRows.Err(); err != nil {
		return nil, err
	}

	return msgs, tx.Commit()
}

func (s *Store) MarkSent(id int64) error {
	_, err := s.db.Exec(
		"UPDATE messages SET status = 'sent', sent_at = datetime('now'), attempts = attempts + 1 WHERE id = ?",
		id,
	)
	return err
}

func (s *Store) MarkFailed(id int64, errMsg string, retryDelay time.Duration) error {
	// Check if we should retry
	var attempts, maxAttempts int
	err := s.db.QueryRow("SELECT attempts, max_attempts FROM messages WHERE id = ?", id).
		Scan(&attempts, &maxAttempts)
	if err != nil {
		return err
	}

	attempts++
	if attempts >= maxAttempts {
		_, err = s.db.Exec(
			"UPDATE messages SET status = 'failed', attempts = ?, last_error = ? WHERE id = ?",
			attempts, errMsg, id,
		)
	} else {
		retryAt := time.Now().Add(retryDelay * time.Duration(1<<uint(attempts-1))) // exponential backoff
		_, err = s.db.Exec(
			"UPDATE messages SET status = 'queued', attempts = ?, last_error = ?, next_retry_at = ? WHERE id = ?",
			attempts, errMsg, retryAt.UTC().Format("2006-01-02 15:04:05"), id,
		)
	}
	return err
}

func (s *Store) ListMessages(status string, limit, offset int) ([]Message, int64, error) {
	var countQuery, listQuery string
	var countArgs, listArgs []interface{}

	if status != "" && status != "all" {
		countQuery = "SELECT COUNT(*) FROM messages WHERE status = ?"
		countArgs = []interface{}{status}
		listQuery = `SELECT m.id, m.token_id, COALESCE(t.name, ''), m.channel, m.to_addr, m.subject,
			m.status, m.attempts, m.last_error, m.created_at, m.sent_at
			FROM messages m LEFT JOIN tokens t ON m.token_id = t.id
			WHERE m.status = ? ORDER BY m.created_at DESC LIMIT ? OFFSET ?`
		listArgs = []interface{}{status, limit, offset}
	} else {
		countQuery = "SELECT COUNT(*) FROM messages"
		listQuery = `SELECT m.id, m.token_id, COALESCE(t.name, ''), m.channel, m.to_addr, m.subject,
			m.status, m.attempts, m.last_error, m.created_at, m.sent_at
			FROM messages m LEFT JOIN tokens t ON m.token_id = t.id
			ORDER BY m.created_at DESC LIMIT ? OFFSET ?`
		listArgs = []interface{}{limit, offset}
	}

	var total int64
	if err := s.db.QueryRow(countQuery, countArgs...).Scan(&total); err != nil {
		return nil, 0, err
	}

	rows, err := s.db.Query(listQuery, listArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var msgs []Message
	for rows.Next() {
		var m Message
		var tokenID sql.NullInt64
		var sentAt sql.NullTime
		if err := rows.Scan(&m.ID, &tokenID, &m.TokenName, &m.Channel, &m.To, &m.Subject,
			&m.Status, &m.Attempts, &m.LastError, &m.CreatedAt, &sentAt); err != nil {
			return nil, 0, err
		}
		if tokenID.Valid {
			m.TokenID = &tokenID.Int64
		}
		if sentAt.Valid {
			m.SentAt = &sentAt.Time
		}
		msgs = append(msgs, m)
	}
	return msgs, total, rows.Err()
}

func (s *Store) DeleteMessage(id int64) error {
	_, err := s.db.Exec("DELETE FROM messages WHERE id = ?", id)
	return err
}

func (s *Store) RetryMessage(id int64) error {
	_, err := s.db.Exec(
		"UPDATE messages SET status = 'queued', next_retry_at = NULL, last_error = '' WHERE id = ?",
		id,
	)
	return err
}

func (s *Store) ResetStuckMessages() error {
	_, err := s.db.Exec("UPDATE messages SET status = 'queued' WHERE status = 'sending'")
	return err
}

// --- Stats ---

func (s *Store) GetDashboardStats() (*DashboardStats, error) {
	var stats DashboardStats
	err := s.db.QueryRow(`
		SELECT
			(SELECT COUNT(*) FROM messages WHERE status = 'sent'),
			(SELECT COUNT(*) FROM messages WHERE status = 'failed'),
			(SELECT COUNT(*) FROM messages WHERE status = 'sent' AND date(sent_at) = date('now')),
			(SELECT COUNT(*) FROM messages WHERE status = 'failed' AND date(created_at) = date('now')),
			(SELECT COUNT(*) FROM messages WHERE status IN ('queued', 'sending'))
	`).Scan(&stats.TotalSent, &stats.TotalFailed, &stats.TodaySent, &stats.TodayFailed, &stats.QueueDepth)
	if err != nil {
		return nil, err
	}

	total := stats.TotalSent + stats.TotalFailed
	if total > 0 {
		stats.SuccessRate = float64(stats.TotalSent) / float64(total) * 100
	}
	return &stats, nil
}

func (s *Store) GetHourlyStats(hours int) ([]HourlyStat, error) {
	rows, err := s.db.Query(`
		SELECT
			strftime('%Y-%m-%d %H:00:00', created_at) as hour,
			SUM(CASE WHEN status = 'sent' THEN 1 ELSE 0 END) as sent,
			SUM(CASE WHEN status = 'failed' THEN 1 ELSE 0 END) as failed,
			COUNT(*) as total
		FROM messages
		WHERE created_at >= datetime('now', ? || ' hours')
		GROUP BY strftime('%Y-%m-%d %H:00:00', created_at)
		ORDER BY hour`,
		fmt.Sprintf("-%d", hours),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stats []HourlyStat
	for rows.Next() {
		var h HourlyStat
		var hourStr string
		if err := rows.Scan(&hourStr, &h.Sent, &h.Failed, &h.Total); err != nil {
			return nil, err
		}
		h.Hour, _ = time.Parse("2006-01-02 15:04:05", hourStr)
		stats = append(stats, h)
	}
	return stats, rows.Err()
}

func (s *Store) GetRecentMessages(limit int) ([]Message, error) {
	rows, err := s.db.Query(`
		SELECT m.id, m.token_id, COALESCE(t.name, ''), m.channel, m.to_addr, m.subject,
			m.status, m.attempts, m.last_error, m.created_at, m.sent_at
		FROM messages m LEFT JOIN tokens t ON m.token_id = t.id
		ORDER BY m.created_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var msgs []Message
	for rows.Next() {
		var m Message
		var tokenID sql.NullInt64
		var sentAt sql.NullTime
		if err := rows.Scan(&m.ID, &tokenID, &m.TokenName, &m.Channel, &m.To, &m.Subject,
			&m.Status, &m.Attempts, &m.LastError, &m.CreatedAt, &sentAt); err != nil {
			return nil, err
		}
		if tokenID.Valid {
			m.TokenID = &tokenID.Int64
		}
		if sentAt.Valid {
			m.SentAt = &sentAt.Time
		}
		msgs = append(msgs, m)
	}
	return msgs, rows.Err()
}

// --- Per-token stats ---

type TokenStats struct {
	TokenID   int64
	TokenName string
	Sent      int64
	Failed    int64
	Queued    int64
	Total     int64
}

func (s *Store) GetTokenStats() ([]TokenStats, error) {
	rows, err := s.db.Query(`
		SELECT t.id, t.name,
			SUM(CASE WHEN m.status = 'sent' THEN 1 ELSE 0 END) as sent,
			SUM(CASE WHEN m.status = 'failed' THEN 1 ELSE 0 END) as failed,
			SUM(CASE WHEN m.status IN ('queued','sending') THEN 1 ELSE 0 END) as queued,
			COUNT(*) as total
		FROM tokens t
		LEFT JOIN messages m ON m.token_id = t.id
		GROUP BY t.id, t.name
		ORDER BY total DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stats []TokenStats
	for rows.Next() {
		var ts TokenStats
		if err := rows.Scan(&ts.TokenID, &ts.TokenName, &ts.Sent, &ts.Failed, &ts.Queued, &ts.Total); err != nil {
			return nil, err
		}
		stats = append(stats, ts)
	}
	return stats, rows.Err()
}

func (s *Store) GetTokenMessages(tokenID int64, limit, offset int) ([]Message, int64, error) {
	var total int64
	if err := s.db.QueryRow("SELECT COUNT(*) FROM messages WHERE token_id = ?", tokenID).Scan(&total); err != nil {
		return nil, 0, err
	}

	rows, err := s.db.Query(`
		SELECT m.id, m.token_id, COALESCE(t.name, ''), m.channel, m.to_addr, m.subject,
			m.status, m.attempts, m.last_error, m.created_at, m.sent_at
		FROM messages m LEFT JOIN tokens t ON m.token_id = t.id
		WHERE m.token_id = ?
		ORDER BY m.created_at DESC LIMIT ? OFFSET ?`, tokenID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var msgs []Message
	for rows.Next() {
		var m Message
		var tokenIDn sql.NullInt64
		var sentAt sql.NullTime
		if err := rows.Scan(&m.ID, &tokenIDn, &m.TokenName, &m.Channel, &m.To, &m.Subject,
			&m.Status, &m.Attempts, &m.LastError, &m.CreatedAt, &sentAt); err != nil {
			return nil, 0, err
		}
		if tokenIDn.Valid {
			m.TokenID = &tokenIDn.Int64
		}
		if sentAt.Valid {
			m.SentAt = &sentAt.Time
		}
		msgs = append(msgs, m)
	}
	return msgs, total, rows.Err()
}

// --- Settings ---

func (s *Store) GetSetting(key string) (string, error) {
	var val string
	err := s.db.QueryRow("SELECT value FROM settings WHERE key = ?", key).Scan(&val)
	if err != nil {
		return "", err
	}
	return val, nil
}

func (s *Store) SetSetting(key, value string) error {
	_, err := s.db.Exec(
		"INSERT INTO settings (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value",
		key, value,
	)
	return err
}

func (s *Store) GetAllSettings() (map[string]string, error) {
	rows, err := s.db.Query("SELECT key, value FROM settings")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	m := make(map[string]string)
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		m[k] = v
	}
	return m, rows.Err()
}

func hashToken(raw string) string {
	h := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(h[:])
}
