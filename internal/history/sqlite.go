package history

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
	"whatscli/internal/privatefs"
)

type SQLiteRepository struct{ db *sql.DB }

func Open(path string) (*SQLiteRepository, error) {
	if err := privatefs.EnsureFile(path); err != nil {
		return nil, err
	}
	if err := secureSidecars(path); err != nil {
		return nil, err
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, errors.New("invalid history database path")
	}
	// Encode the literal filename so '?' and URI-like paths cannot change the
	// database SQLite opens after permissions were checked on a different file.
	dsn := (&url.URL{Scheme: "file", Path: absolute}).String()
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, errors.New("open history database failed")
	}
	db.SetMaxOpenConns(1)
	fail := func(err error) (*SQLiteRepository, error) { _ = db.Close(); return nil, err }
	if err := db.Ping(); err != nil {
		return fail(errors.New("connect to history database failed"))
	}
	if _, err := db.Exec(`PRAGMA journal_mode=WAL; PRAGMA foreign_keys=ON; PRAGMA busy_timeout=5000;`); err != nil {
		return fail(errors.New("configure history database failed"))
	}
	if err := migrate(context.Background(), db); err != nil {
		return fail(fmt.Errorf("initialize history database: %w", err))
	}
	if err := secureSidecars(path); err != nil {
		return fail(err)
	}
	return &SQLiteRepository{db: db}, nil
}

func secureSidecars(path string) error {
	for _, suffix := range []string{"-wal", "-shm", "-journal"} {
		_, err := os.Lstat(path + suffix)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return errors.New("inspect history sidecar failed")
		}
		if err := privatefs.EnsureFile(path + suffix); err != nil {
			return err
		}
	}
	return nil
}

func (r *SQLiteRepository) Close() error { return r.db.Close() }

func (r *SQLiteRepository) ApplyEvent(ctx context.Context, event IncomingEvent) error {
	if event.AccountID == "" || event.ChatID == "" || event.MessageID == "" || event.Timestamp.IsZero() {
		return errors.New("event account, chat, message and source timestamp are required")
	}
	switch event.Kind {
	case MessageEvent:
		if event.Sender == "" {
			return errors.New("message sender is required")
		}
	case EditEvent, DeleteEvent, DeliveryEvent:
	default:
		return errors.New("unsupported history event kind")
	}
	if _, ok := statusRank[event.Status]; !ok {
		return errors.New("unsupported delivery status")
	}
	if event.Kind == DeliveryEvent && event.Status == "" {
		return errors.New("delivery status is required")
	}
	event.Timestamp = event.Timestamp.UTC()
	if event.EventID == "" {
		// Titles and sender display metadata do not determine event identity.
		data, _ := json.Marshal([]any{event.Kind, formatTime(event.Timestamp), event.Body, event.Status})
		sum := sha256.Sum256(data)
		event.EventID = hex.EncodeToString(sum[:])
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO history_events(account_id,chat_id,message_id,event_id,kind) VALUES(?,?,?,?,?)`, event.AccountID, event.ChatID, event.MessageID, event.EventID, string(event.Kind))
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		// A server/self echo may carry the same source identity as a pending
		// outgoing event. Deduplicate content, but still advance delivery state.
		if event.Status != "" {
			var current string
			if err := tx.QueryRowContext(ctx, `SELECT status FROM account_messages WHERE account_id=? AND chat_id=? AND id=?`, event.AccountID, event.ChatID, event.MessageID).Scan(&current); err != nil {
				return err
			}
			if statusRank[event.Status] > statusRank[current] {
				if _, err := tx.ExecContext(ctx, `UPDATE account_messages SET status=?,updated_at=MAX(updated_at,?) WHERE account_id=? AND chat_id=? AND id=?`, event.Status, formatTime(event.Timestamp), event.AccountID, event.ChatID, event.MessageID); err != nil {
					return err
				}
			}
		}
		return tx.Commit()
	}
	at := formatTime(event.Timestamp)
	title := event.Title
	if title == "" {
		title = event.ChatID
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO account_conversations(account_id,id,title,updated_at) VALUES(?,?,?,?)
		ON CONFLICT(account_id,id) DO UPDATE SET
		title=CASE WHEN ?<>'' AND excluded.updated_at>=account_conversations.updated_at THEN excluded.title ELSE account_conversations.title END,
		updated_at=MAX(account_conversations.updated_at,excluded.updated_at)`, event.AccountID, event.ChatID, title, at, event.Title)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO account_messages(account_id,chat_id,id,sender,body,created_at,updated_at,body_at,body_event,from_me,edited,status,original_seen)
		VALUES(?,?,?,'','',?,?,'','',0,0,'',0)`, event.AccountID, event.ChatID, event.MessageID, at, at)
	if err != nil {
		return err
	}
	var status string
	err = tx.QueryRowContext(ctx, `SELECT status FROM account_messages WHERE account_id=? AND chat_id=? AND id=?`, event.AccountID, event.ChatID, event.MessageID).Scan(&status)
	if err != nil {
		return err
	}
	key := []any{event.AccountID, event.ChatID, event.MessageID}
	if event.Kind == MessageEvent || event.Kind == EditEvent {
		revisionID := event.EventID
		if event.Kind == MessageEvent {
			revisionID = "original"
		}
		_, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO account_revisions(account_id,chat_id,message_id,number,event_id,kind,body,edited_at)
			VALUES(?,?,?,(SELECT COALESCE(MAX(number),0)+1 FROM account_revisions WHERE account_id=? AND chat_id=? AND message_id=?),?,?,?,?)`,
			event.AccountID, event.ChatID, event.MessageID, event.AccountID, event.ChatID, event.MessageID, revisionID, string(event.Kind), event.Body, at)
		if err != nil {
			return err
		}
		if event.Kind == MessageEvent {
			_, err = tx.ExecContext(ctx, `UPDATE account_messages SET sender=?,from_me=?,created_at=CASE WHEN original_seen=0 THEN ? ELSE MIN(created_at,?) END,original_seen=1 WHERE account_id=? AND chat_id=? AND id=?`, event.Sender, event.FromMe, at, at, event.AccountID, event.ChatID, event.MessageID)
			if err != nil {
				return err
			}
		}
	}
	if event.Kind == DeleteEvent {
		_, err = tx.ExecContext(ctx, `UPDATE account_messages SET deleted_at=CASE WHEN deleted_at IS NULL THEN ? ELSE MIN(deleted_at,?) END WHERE account_id=? AND chat_id=? AND id=?`, at, at, event.AccountID, event.ChatID, event.MessageID)
		if err != nil {
			return err
		}
	}
	if event.Kind != DeliveryEvent {
		// Resolve by source time, not arrival order. A revoke is permanent; text
		// observed after its timestamp remains a revision, not current content.
		var body, bodyAt, bodyEvent, kind string
		err = tx.QueryRowContext(ctx, `SELECT r.body,r.edited_at,r.event_id,r.kind FROM account_revisions r
			JOIN account_messages m ON m.account_id=r.account_id AND m.chat_id=r.chat_id AND m.id=r.message_id
			WHERE r.account_id=? AND r.chat_id=? AND r.message_id=? AND (m.deleted_at IS NULL OR r.edited_at<=m.deleted_at)
			ORDER BY r.edited_at DESC,CASE WHEN r.kind='edit' THEN 1 ELSE 0 END DESC,r.event_id DESC LIMIT 1`, key...).Scan(&body, &bodyAt, &bodyEvent, &kind)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		_, err = tx.ExecContext(ctx, `UPDATE account_messages SET body=?,body_at=?,body_event=?,edited=? WHERE account_id=? AND chat_id=? AND id=?`, body, bodyAt, bodyEvent, kind == string(EditEvent), event.AccountID, event.ChatID, event.MessageID)
		if err != nil {
			return err
		}
	}
	if event.Status != "" && statusRank[event.Status] >= statusRank[status] {
		_, err = tx.ExecContext(ctx, `UPDATE account_messages SET status=? WHERE account_id=? AND chat_id=? AND id=?`, append([]any{event.Status}, key...)...)
		if err != nil {
			return err
		}
	}
	_, err = tx.ExecContext(ctx, `UPDATE account_messages SET updated_at=MAX(updated_at,?) WHERE account_id=? AND chat_id=? AND id=?`, append([]any{at}, key...)...)
	if err != nil {
		return err
	}
	return tx.Commit()
}

var statusRank = map[string]int{"": 0, "pending": 1, "unknown": 2, "failed": 3, "sent": 4, "delivered": 5, "read": 6}

func pageLimit(limit int) (int, error) {
	if limit <= 0 {
		return 0, errors.New("page limit must be positive")
	}
	if limit > 200 {
		limit = 200
	}
	return limit, nil
}

func (r *SQLiteRepository) Conversations(ctx context.Context, account string, before *Cursor, limit int) ([]Conversation, error) {
	if account == "" {
		return nil, errors.New("account is required")
	}
	limit, err := pageLimit(limit)
	if err != nil {
		return nil, err
	}
	query := `SELECT account_id,id,title,updated_at,
		COALESCE((SELECT CASE WHEN m.deleted_at IS NOT NULL THEN '[deleted; retained]' ELSE m.body END
		FROM account_messages m WHERE m.account_id=account_conversations.account_id
		AND m.chat_id=account_conversations.id ORDER BY m.created_at DESC,m.id DESC LIMIT 1),'')
		FROM account_conversations WHERE account_id=?`
	args := []any{account}
	if before != nil {
		query += ` AND (updated_at<? OR (updated_at=? AND id<?))`
		args = append(args, formatTime(before.Time), formatTime(before.Time), before.ID)
	}
	query += ` ORDER BY updated_at DESC,id DESC LIMIT ?`
	rows, err := r.db.QueryContext(ctx, query, append(args, limit)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []Conversation
	for rows.Next() {
		var c Conversation
		var at string
		if err := rows.Scan(&c.AccountID, &c.ID, &c.Title, &at, &c.LastBody); err != nil {
			return nil, err
		}
		c.UpdatedAt, err = parseTime(at)
		if err != nil {
			return nil, err
		}
		result = append(result, c)
	}
	return result, rows.Err()
}

func (r *SQLiteRepository) Messages(ctx context.Context, account, chat string, before *Cursor, limit int) ([]Message, error) {
	if account == "" || chat == "" {
		return nil, errors.New("account and chat are required")
	}
	limit, err := pageLimit(limit)
	if err != nil {
		return nil, err
	}
	query := `SELECT account_id,id,chat_id,sender,body,created_at,updated_at,deleted_at,from_me,edited,status FROM account_messages WHERE account_id=? AND chat_id=?`
	args := []any{account, chat}
	if before != nil {
		query += ` AND (created_at<? OR (created_at=? AND id<?))`
		args = append(args, formatTime(before.Time), formatTime(before.Time), before.ID)
	}
	query += ` ORDER BY created_at DESC,id DESC LIMIT ?`
	rows, err := r.db.QueryContext(ctx, query, append(args, limit)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []Message
	for rows.Next() {
		var m Message
		var created, updated string
		var deleted sql.NullString
		if err := rows.Scan(&m.AccountID, &m.ID, &m.ConversationID, &m.Sender, &m.Body, &created, &updated, &deleted, &m.FromMe, &m.Edited, &m.Status); err != nil {
			return nil, err
		}
		if m.CreatedAt, err = parseTime(created); err != nil {
			return nil, err
		}
		if m.UpdatedAt, err = parseTime(updated); err != nil {
			return nil, err
		}
		if deleted.Valid {
			at, err := parseTime(deleted.String)
			if err != nil {
				return nil, err
			}
			m.Deleted, m.DeletedAt = true, &at
		}
		result = append(result, m)
	}
	return result, rows.Err()
}

// Revisions are numbered in observation order, not source-time order. Numbers
// never change when an older revision arrives, so the cursor remains stable.
func (r *SQLiteRepository) Revisions(ctx context.Context, account, chat, message string, before, limit int) ([]Revision, error) {
	if account == "" || chat == "" || message == "" || before < 0 {
		return nil, errors.New("invalid revision scope or cursor")
	}
	limit, err := pageLimit(limit)
	if err != nil {
		return nil, err
	}
	query := `SELECT message_id,number,body,edited_at FROM account_revisions WHERE account_id=? AND chat_id=? AND message_id=?`
	args := []any{account, chat, message}
	if before > 0 {
		query += ` AND number<?`
		args = append(args, before)
	}
	query += ` ORDER BY number DESC LIMIT ?`
	rows, err := r.db.QueryContext(ctx, query, append(args, limit)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []Revision
	for rows.Next() {
		var revision Revision
		var at string
		if err := rows.Scan(&revision.MessageID, &revision.Number, &revision.Body, &at); err != nil {
			return nil, err
		}
		if revision.EditedAt, err = parseTime(at); err != nil {
			return nil, err
		}
		result = append(result, revision)
	}
	return result, rows.Err()
}

func newID() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw[:]), nil
}

func formatTime(t time.Time) string             { return t.UTC().Format("2006-01-02T15:04:05.000000000Z07:00") }
func parseTime(value string) (time.Time, error) { return time.Parse(time.RFC3339Nano, value) }
