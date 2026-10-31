package history

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// The original API remains a bounded view of the local, unlinked namespace.
func (r *SQLiteRepository) ListConversations(ctx context.Context) ([]Conversation, error) {
	return r.Conversations(ctx, LegacyAccountID, nil, 200)
}

func (r *SQLiteRepository) SaveConversation(ctx context.Context, c Conversation) error {
	if c.ID == "" || c.Title == "" {
		return errors.New("conversation ID and title are required")
	}
	if c.AccountID == "" {
		c.AccountID = LegacyAccountID
	}
	if c.UpdatedAt.IsZero() {
		c.UpdatedAt = time.Now().UTC()
	}
	_, err := r.db.ExecContext(ctx, `INSERT INTO account_conversations(account_id,id,title,updated_at) VALUES(?,?,?,?)
		ON CONFLICT(account_id,id) DO UPDATE SET title=excluded.title,updated_at=MAX(account_conversations.updated_at,excluded.updated_at)`, c.AccountID, c.ID, c.Title, formatTime(c.UpdatedAt))
	return err
}

func (r *SQLiteRepository) AddMessage(ctx context.Context, m Message) (Message, error) {
	if m.ConversationID == "" || m.Sender == "" {
		return Message{}, errors.New("conversation ID and sender are required")
	}
	if m.AccountID == "" {
		m.AccountID = LegacyAccountID
	}
	if m.ID == "" {
		var err error
		m.ID, err = newID()
		if err != nil {
			return Message{}, err
		}
	}
	if m.CreatedAt.IsZero() {
		m.CreatedAt = time.Now().UTC()
	}
	m.CreatedAt = m.CreatedAt.UTC()
	if m.UpdatedAt.IsZero() {
		m.UpdatedAt = m.CreatedAt
	}
	err := r.ApplyEvent(ctx, IncomingEvent{Kind: MessageEvent, AccountID: m.AccountID, ChatID: m.ConversationID, MessageID: m.ID, Sender: m.Sender, Body: m.Body, Timestamp: m.CreatedAt, FromMe: m.FromMe, Status: m.Status})
	return m, err
}

func (r *SQLiteRepository) legacyMessage(ctx context.Context, id string) (string, bool, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT chat_id,deleted_at IS NOT NULL FROM account_messages WHERE account_id=? AND id=? LIMIT 2`, LegacyAccountID, id)
	if err != nil {
		return "", false, err
	}
	defer rows.Close()
	var chat string
	var deleted bool
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return "", false, err
		}
		return "", false, sql.ErrNoRows
	}
	if err := rows.Scan(&chat, &deleted); err != nil {
		return "", false, err
	}
	if rows.Next() {
		return "", false, errors.New("message ID is ambiguous; use the account-scoped API")
	}
	return chat, deleted, rows.Err()
}

func (r *SQLiteRepository) EditMessage(ctx context.Context, id, body string) error {
	chat, deleted, err := r.legacyMessage(ctx, id)
	if err != nil {
		return err
	}
	if deleted {
		return errors.New("cannot edit a deleted message")
	}
	return r.ApplyEvent(ctx, IncomingEvent{Kind: EditEvent, AccountID: LegacyAccountID, ChatID: chat, MessageID: id, Body: body, Timestamp: time.Now().UTC()})
}

func (r *SQLiteRepository) DeleteMessage(ctx context.Context, id string) error {
	chat, deleted, err := r.legacyMessage(ctx, id)
	if err != nil || deleted {
		return err
	}
	return r.ApplyEvent(ctx, IncomingEvent{Kind: DeleteEvent, AccountID: LegacyAccountID, ChatID: chat, MessageID: id, Timestamp: time.Now().UTC()})
}

func (r *SQLiteRepository) ListMessages(ctx context.Context, chat string, limit int) ([]Message, error) {
	return r.Messages(ctx, LegacyAccountID, chat, nil, limit)
}

func (r *SQLiteRepository) ListRevisions(ctx context.Context, id string) ([]Revision, error) {
	chat, _, err := r.legacyMessage(ctx, id)
	if err != nil {
		return nil, err
	}
	revisions, err := r.Revisions(ctx, LegacyAccountID, chat, id, 0, 200)
	for i, j := 0, len(revisions)-1; i < j; i, j = i+1, j-1 {
		revisions[i], revisions[j] = revisions[j], revisions[i]
	}
	return revisions, err
}
