package history

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

type SQLiteRepository struct {
	db *sql.DB
}

func Open(path string) (*SQLiteRepository, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create history directory: %w", err)
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open history database: %w", err)
	}
	db.SetMaxOpenConns(1)
	fail := func(err error) (*SQLiteRepository, error) {
		_ = db.Close()
		return nil, err
	}
	if err := db.Ping(); err != nil {
		return fail(fmt.Errorf("connect to history database: %w", err))
	}
	if _, err := db.Exec(`PRAGMA journal_mode=WAL; PRAGMA foreign_keys=ON; PRAGMA busy_timeout=5000;`); err != nil {
		return fail(fmt.Errorf("configure history database: %w", err))
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return fail(fmt.Errorf("secure history database permissions: %w", err))
	}
	if err := migrate(context.Background(), db); err != nil {
		return fail(fmt.Errorf("initialize history database: %w", err))
	}
	return &SQLiteRepository{db: db}, nil
}

func (r *SQLiteRepository) Close() error {
	return r.db.Close()
}

func migrate(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS conversations (
	id TEXT PRIMARY KEY,
	title TEXT NOT NULL,
	updated_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS messages (
	id TEXT PRIMARY KEY,
	conversation_id TEXT NOT NULL REFERENCES conversations(id),
	sender TEXT NOT NULL,
	body TEXT NOT NULL,
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL,
	deleted_at TEXT
);
CREATE INDEX IF NOT EXISTS messages_conversation_created
	ON messages(conversation_id, created_at DESC);
CREATE TABLE IF NOT EXISTS message_revisions (
	message_id TEXT NOT NULL REFERENCES messages(id),
	number INTEGER NOT NULL,
	body TEXT NOT NULL,
	edited_at TEXT NOT NULL,
	PRIMARY KEY (message_id, number)
);
`)
	return err
}

func (r *SQLiteRepository) SaveConversation(ctx context.Context, conversation Conversation) error {
	if conversation.ID == "" {
		return errors.New("conversation ID is required")
	}
	if conversation.Title == "" {
		return errors.New("conversation title is required")
	}
	if conversation.UpdatedAt.IsZero() {
		conversation.UpdatedAt = time.Now().UTC()
	}
	_, err := r.db.ExecContext(ctx, `
INSERT INTO conversations(id, title, updated_at) VALUES(?, ?, ?)
ON CONFLICT(id) DO UPDATE SET title=excluded.title, updated_at=excluded.updated_at`,
		conversation.ID, conversation.Title, formatTime(conversation.UpdatedAt))
	return err
}

func (r *SQLiteRepository) ListConversations(ctx context.Context) ([]Conversation, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, title, updated_at FROM conversations ORDER BY updated_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var conversations []Conversation
	for rows.Next() {
		var conversation Conversation
		var updatedAt string
		if err := rows.Scan(&conversation.ID, &conversation.Title, &updatedAt); err != nil {
			return nil, err
		}
		conversation.UpdatedAt, err = parseTime(updatedAt)
		if err != nil {
			return nil, err
		}
		conversations = append(conversations, conversation)
	}
	return conversations, rows.Err()
}

func (r *SQLiteRepository) AddMessage(ctx context.Context, message Message) (Message, error) {
	if message.ConversationID == "" || message.Sender == "" {
		return Message{}, errors.New("conversation ID and sender are required")
	}
	if message.ID == "" {
		var err error
		message.ID, err = newID()
		if err != nil {
			return Message{}, fmt.Errorf("generate message ID: %w", err)
		}
	}
	if message.CreatedAt.IsZero() {
		message.CreatedAt = time.Now().UTC()
	}
	message.CreatedAt = message.CreatedAt.UTC()
	if message.UpdatedAt.IsZero() {
		message.UpdatedAt = message.CreatedAt
	}
	message.UpdatedAt = message.UpdatedAt.UTC()

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return Message{}, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `INSERT INTO messages(id, conversation_id, sender, body, created_at, updated_at) VALUES(?, ?, ?, ?, ?, ?)`,
		message.ID, message.ConversationID, message.Sender, message.Body, formatTime(message.CreatedAt), formatTime(message.UpdatedAt)); err != nil {
		return Message{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO message_revisions(message_id, number, body, edited_at) VALUES(?, 1, ?, ?)`,
		message.ID, message.Body, formatTime(message.CreatedAt)); err != nil {
		return Message{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE conversations SET updated_at=? WHERE id=?`, formatTime(message.CreatedAt), message.ConversationID); err != nil {
		return Message{}, err
	}
	if err := tx.Commit(); err != nil {
		return Message{}, err
	}
	return message, nil
}

func (r *SQLiteRepository) EditMessage(ctx context.Context, messageID, body string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var deletedAt sql.NullString
	if err := tx.QueryRowContext(ctx, `SELECT deleted_at FROM messages WHERE id=?`, messageID).Scan(&deletedAt); err != nil {
		return err
	}
	if deletedAt.Valid {
		return errors.New("cannot edit a deleted message")
	}
	var next int
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(number), 0) + 1 FROM message_revisions WHERE message_id=?`, messageID).Scan(&next); err != nil {
		return err
	}
	now := time.Now().UTC()
	if _, err := tx.ExecContext(ctx, `INSERT INTO message_revisions(message_id, number, body, edited_at) VALUES(?, ?, ?, ?)`, messageID, next, body, formatTime(now)); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE messages SET body=?, updated_at=? WHERE id=?`, body, formatTime(now), messageID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE conversations SET updated_at=? WHERE id=(SELECT conversation_id FROM messages WHERE id=?)`, formatTime(now), messageID); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *SQLiteRepository) DeleteMessage(ctx context.Context, messageID string) error {
	now := time.Now().UTC()
	result, err := r.db.ExecContext(ctx, `UPDATE messages SET deleted_at=?, updated_at=? WHERE id=? AND deleted_at IS NULL`, formatTime(now), formatTime(now), messageID)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		var exists int
		if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM messages WHERE id=?`, messageID).Scan(&exists); err != nil {
			return err
		}
		if exists == 0 {
			return sql.ErrNoRows
		}
	}
	return nil
}

func (r *SQLiteRepository) ListMessages(ctx context.Context, conversationID string, limit int) ([]Message, error) {
	if limit <= 0 {
		return nil, errors.New("message limit must be positive")
	}
	rows, err := r.db.QueryContext(ctx, `
SELECT id, conversation_id, sender, body, created_at, updated_at, deleted_at
FROM messages WHERE conversation_id=? ORDER BY created_at DESC LIMIT ?`, conversationID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []Message
	for rows.Next() {
		var message Message
		var createdAt, updatedAt string
		var deletedAt sql.NullString
		if err := rows.Scan(&message.ID, &message.ConversationID, &message.Sender, &message.Body, &createdAt, &updatedAt, &deletedAt); err != nil {
			return nil, err
		}
		if message.CreatedAt, err = parseTime(createdAt); err != nil {
			return nil, err
		}
		if message.UpdatedAt, err = parseTime(updatedAt); err != nil {
			return nil, err
		}
		if deletedAt.Valid {
			message.Deleted = true
			t, err := parseTime(deletedAt.String)
			if err != nil {
				return nil, err
			}
			message.DeletedAt = &t
		}
		messages = append(messages, message)
	}
	return messages, rows.Err()
}

func (r *SQLiteRepository) ListRevisions(ctx context.Context, messageID string) ([]Revision, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT message_id, number, body, edited_at FROM message_revisions WHERE message_id=? ORDER BY number`, messageID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var revisions []Revision
	for rows.Next() {
		var revision Revision
		var editedAt string
		if err := rows.Scan(&revision.MessageID, &revision.Number, &revision.Body, &editedAt); err != nil {
			return nil, err
		}
		if revision.EditedAt, err = parseTime(editedAt); err != nil {
			return nil, err
		}
		revisions = append(revisions, revision)
	}
	return revisions, rows.Err()
}

func newID() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw[:]), nil
}

func formatTime(t time.Time) string {
	return t.UTC().Format("2006-01-02T15:04:05.000000000Z07:00")
}

func parseTime(value string) (time.Time, error) {
	return time.Parse(time.RFC3339Nano, value)
}
