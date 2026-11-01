package history

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

const schemaVersion = 3

func migrate(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var version int
	if err := tx.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&version); err != nil {
		return err
	}
	if version > schemaVersion {
		return errors.New("history database uses a newer schema")
	}
	if version == schemaVersion {
		return tx.Commit()
	}
	if version == 2 {
		if _, err := tx.ExecContext(ctx, `
CREATE TABLE account_reactions (
 account_id TEXT NOT NULL, chat_id TEXT NOT NULL, target_id TEXT NOT NULL,
 sender TEXT NOT NULL, emoji TEXT NOT NULL, updated_at TEXT NOT NULL,
 PRIMARY KEY(account_id,chat_id,target_id,sender)
);
PRAGMA user_version=3;`); err != nil {
			return err
		}
		return tx.Commit()
	}
	if version == 1 {
		if _, err := tx.ExecContext(ctx, `
ALTER TABLE account_messages ADD COLUMN reply_to_id TEXT NOT NULL DEFAULT '';
ALTER TABLE account_messages ADD COLUMN reply_sender TEXT NOT NULL DEFAULT '';
ALTER TABLE account_messages ADD COLUMN reply_body TEXT NOT NULL DEFAULT '';
CREATE TABLE account_statuses (
 account_id TEXT NOT NULL, id TEXT NOT NULL, sender TEXT NOT NULL, name TEXT NOT NULL,
 body TEXT NOT NULL, created_at TEXT NOT NULL,
 PRIMARY KEY(account_id,id,sender)
);
CREATE INDEX account_status_page ON account_statuses(account_id,created_at DESC,id DESC);
CREATE TABLE account_reactions (
 account_id TEXT NOT NULL, chat_id TEXT NOT NULL, target_id TEXT NOT NULL,
 sender TEXT NOT NULL, emoji TEXT NOT NULL, updated_at TEXT NOT NULL,
 PRIMARY KEY(account_id,chat_id,target_id,sender)
);
PRAGMA user_version=3;`); err != nil {
			return err
		}
		return tx.Commit()
	}
	if _, err := tx.ExecContext(ctx, `
CREATE TABLE account_conversations (
 account_id TEXT NOT NULL, id TEXT NOT NULL, title TEXT NOT NULL, updated_at TEXT NOT NULL,
 PRIMARY KEY(account_id,id)
);
CREATE INDEX account_conversation_page ON account_conversations(account_id,updated_at DESC,id DESC);
CREATE TABLE account_messages (
 account_id TEXT NOT NULL, chat_id TEXT NOT NULL, id TEXT NOT NULL,
 sender TEXT NOT NULL, body TEXT NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL,
 deleted_at TEXT, body_at TEXT NOT NULL, body_event TEXT NOT NULL,
 from_me INTEGER NOT NULL, edited INTEGER NOT NULL, status TEXT NOT NULL, original_seen INTEGER NOT NULL,
 reply_to_id TEXT NOT NULL DEFAULT '', reply_sender TEXT NOT NULL DEFAULT '', reply_body TEXT NOT NULL DEFAULT '',
 PRIMARY KEY(account_id,chat_id,id),
 FOREIGN KEY(account_id,chat_id) REFERENCES account_conversations(account_id,id)
);
CREATE INDEX account_message_page ON account_messages(account_id,chat_id,created_at DESC,id DESC);
CREATE TABLE account_revisions (
 account_id TEXT NOT NULL, chat_id TEXT NOT NULL, message_id TEXT NOT NULL, number INTEGER NOT NULL,
 event_id TEXT NOT NULL, kind TEXT NOT NULL, body TEXT NOT NULL, edited_at TEXT NOT NULL,
 PRIMARY KEY(account_id,chat_id,message_id,number),
 UNIQUE(account_id,chat_id,message_id,kind,event_id),
 FOREIGN KEY(account_id,chat_id,message_id) REFERENCES account_messages(account_id,chat_id,id)
);
CREATE TABLE history_events (
 account_id TEXT NOT NULL, chat_id TEXT NOT NULL, message_id TEXT NOT NULL,
 event_id TEXT NOT NULL, kind TEXT NOT NULL,
 PRIMARY KEY(account_id,chat_id,message_id,event_id,kind)
);
CREATE TABLE account_statuses (
 account_id TEXT NOT NULL, id TEXT NOT NULL, sender TEXT NOT NULL, name TEXT NOT NULL,
 body TEXT NOT NULL, created_at TEXT NOT NULL,
 PRIMARY KEY(account_id,id,sender)
);
CREATE INDEX account_status_page ON account_statuses(account_id,created_at DESC,id DESC);
CREATE TABLE account_reactions (
 account_id TEXT NOT NULL, chat_id TEXT NOT NULL, target_id TEXT NOT NULL,
 sender TEXT NOT NULL, emoji TEXT NOT NULL, updated_at TEXT NOT NULL,
 PRIMARY KEY(account_id,chat_id,target_id,sender)
);`); err != nil {
		return err
	}
	var legacy int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='messages'`).Scan(&legacy); err != nil {
		return err
	}
	if legacy != 0 {
		if _, err := tx.ExecContext(ctx, `
INSERT INTO account_conversations SELECT 'local',id,title,updated_at FROM conversations;
INSERT INTO account_messages
 SELECT 'local',conversation_id,id,sender,body,created_at,updated_at,deleted_at,
 COALESCE((SELECT edited_at FROM message_revisions WHERE message_id=messages.id ORDER BY number DESC LIMIT 1),created_at),
 '',0,(SELECT COUNT(*)>1 FROM message_revisions WHERE message_id=messages.id),'',1,'','','' FROM messages;
INSERT INTO account_revisions
 SELECT 'local',m.conversation_id,r.message_id,r.number,
 CASE WHEN r.number=1 THEN 'original' ELSE 'legacy:'||r.number END,
 CASE WHEN r.number=1 THEN 'message' ELSE 'edit' END,r.body,r.edited_at
 FROM message_revisions r JOIN messages m ON m.id=r.message_id;
DROP TABLE message_revisions;
DROP TABLE messages;
DROP TABLE conversations;`); err != nil {
			return err
		}
		if err := normalizeLegacyTimes(ctx, tx); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `PRAGMA user_version=3`); err != nil {
		return err
	}
	return tx.Commit()
}

func normalizeLegacyTimes(ctx context.Context, tx *sql.Tx) error {
	for table, columns := range map[string][]string{
		"account_conversations": {"updated_at"},
		"account_messages":      {"created_at", "updated_at", "deleted_at", "body_at"},
		"account_revisions":     {"edited_at"},
	} {
		for _, column := range columns {
			rows, err := tx.QueryContext(ctx, fmt.Sprintf("SELECT rowid,%s FROM %s WHERE %s IS NOT NULL", column, table, column))
			if err != nil {
				return err
			}
			type change struct {
				id    int64
				value string
			}
			var changes []change
			for rows.Next() {
				var c change
				if err := rows.Scan(&c.id, &c.value); err != nil {
					rows.Close()
					return err
				}
				at, err := parseTime(c.value)
				if err != nil {
					rows.Close()
					return errors.New("legacy history has an invalid timestamp")
				}
				c.value = formatTime(at)
				changes = append(changes, c)
			}
			err = rows.Err()
			rows.Close()
			if err != nil {
				return err
			}
			for _, c := range changes {
				if _, err := tx.ExecContext(ctx, fmt.Sprintf("UPDATE %s SET %s=? WHERE rowid=?", table, column), c.value, c.id); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
