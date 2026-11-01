package history

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

var testTime = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

func testRepository(t *testing.T) *SQLiteRepository {
	t.Helper()
	r, err := Open(filepath.Join(t.TempDir(), "history.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { r.Close() })
	return r
}
func testEvent(kind EventKind, id string, seconds int, body string) IncomingEvent {
	return IncomingEvent{Kind: kind, AccountID: "account", ChatID: "chat", MessageID: "message", EventID: id, Sender: "sender", Body: body, Timestamp: testTime.Add(time.Duration(seconds) * time.Second)}
}
func apply(t *testing.T, r *SQLiteRepository, e IncomingEvent) {
	t.Helper()
	if err := r.ApplyEvent(context.Background(), e); err != nil {
		t.Fatal(err)
	}
}

func TestOutOfOrderEventsAndPermanentTombstones(t *testing.T) {
	for _, order := range [][]int{{0, 1, 2, 3, 4}, {3, 2, 1, 0, 4}, {4, 3, 2, 1, 0}, {1, 4, 0, 3, 2}} {
		t.Run(fmt.Sprint(order), func(t *testing.T) {
			r := testRepository(t)
			events := []IncomingEvent{
				testEvent(MessageEvent, "original", 0, "original"),
				testEvent(EditEvent, "edit-one", 10, "first edit"),
				testEvent(EditEvent, "edit-two", 20, "latest edit"),
				testEvent(DeleteEvent, "revoke", 30, ""),
				testEvent(EditEvent, "post-delete", 40, "not resurrected"),
			}
			for _, i := range order {
				apply(t, r, events[i])
				apply(t, r, events[i])
			}
			messages, err := r.Messages(context.Background(), "account", "chat", nil, 10)
			if err != nil || len(messages) != 1 {
				t.Fatalf("messages: %v %v", messages, err)
			}
			m := messages[0]
			if m.Body != "latest edit" || !m.Edited || !m.Deleted || !m.CreatedAt.Equal(testTime) || !m.DeletedAt.Equal(testTime.Add(30*time.Second)) {
				t.Fatalf("projection: %#v", m)
			}
			revisions, err := r.Revisions(context.Background(), "account", "chat", "message", 0, 10)
			if err != nil || len(revisions) != 4 {
				t.Fatalf("revisions: %v %v", revisions, err)
			}
			conversations, err := r.Conversations(context.Background(), "account", nil, 10)
			if err != nil || !conversations[0].UpdatedAt.Equal(testTime.Add(40*time.Second)) {
				t.Fatalf("activity: %v %v", conversations, err)
			}
		})
	}
}

func TestAccountChatIsolationAndStablePages(t *testing.T) {
	r := testRepository(t)
	ctx := context.Background()
	for _, account := range []string{"one", "two"} {
		for _, chat := range []string{"chat-a", "chat-b", "chat-c"} {
			for _, id := range []string{"a", "b", "c"} {
				e := testEvent(MessageEvent, "event", 0, account+chat+id)
				e.AccountID, e.ChatID, e.MessageID = account, chat, id
				apply(t, r, e)
			}
		}
	}
	page, err := r.Messages(ctx, "one", "chat-a", nil, 2)
	if err != nil || len(page) != 2 || page[0].ID != "c" || page[1].ID != "b" {
		t.Fatalf("page: %v %v", page, err)
	}
	next, err := r.Messages(ctx, "one", "chat-a", &Cursor{Time: page[1].CreatedAt, ID: page[1].ID}, 2)
	if err != nil || len(next) != 1 || next[0].ID != "a" || next[0].Body != "onechat-aa" {
		t.Fatalf("next: %v %v", next, err)
	}
	chats, err := r.Conversations(ctx, "two", nil, 2)
	if err != nil || len(chats) != 2 || chats[0].ID != "chat-c" {
		t.Fatalf("chats: %v %v", chats, err)
	}
	more, err := r.Conversations(ctx, "two", &Cursor{Time: chats[1].UpdatedAt, ID: chats[1].ID}, 2)
	if err != nil || len(more) != 1 || more[0].ID != "chat-a" {
		t.Fatalf("more chats: %v %v", more, err)
	}
	legacy, err := r.ListConversations(ctx)
	if err != nil || len(legacy) != 0 {
		t.Fatal("live accounts leaked into legacy view")
	}
}

func TestOutgoingEchoDeliveryAndRevisionPagination(t *testing.T) {
	r := testRepository(t)
	ctx := context.Background()
	pending := testEvent(MessageEvent, "pending", 0, "outgoing")
	pending.Status, pending.FromMe = "pending", true
	apply(t, r, pending)
	identicalEcho := pending
	identicalEcho.Status = "sent"
	apply(t, r, identicalEcho)
	echoMessages, _ := r.Messages(ctx, "account", "chat", nil, 10)
	if echoMessages[0].Status != "sent" {
		t.Fatal("same-identity echo did not advance pending status")
	}
	sent := testEvent(DeliveryEvent, "receipt", 1, "")
	sent.Status = "sent"
	apply(t, r, sent)
	pending.EventID = "late-pending"
	apply(t, r, pending)
	echo := pending
	echo.EventID, echo.Status = "echo", "sent"
	apply(t, r, echo)
	messages, _ := r.Messages(ctx, "account", "chat", nil, 10)
	if messages[0].Status != "sent" || !messages[0].FromMe {
		t.Fatalf("status regressed: %#v", messages[0])
	}
	revisions, _ := r.Revisions(ctx, "account", "chat", "message", 0, 10)
	if len(revisions) != 1 {
		t.Fatalf("echo or delivery created revisions: %v", revisions)
	}
	for i := 1; i <= 3; i++ {
		apply(t, r, testEvent(EditEvent, fmt.Sprint(i), i, fmt.Sprint(i)))
	}
	first, _ := r.Revisions(ctx, "account", "chat", "message", 0, 2)
	second, _ := r.Revisions(ctx, "account", "chat", "message", first[1].Number, 2)
	if len(first) != 2 || len(second) != 2 || first[0].Number != 4 || second[0].Number != 2 {
		t.Fatalf("revision pages: %v %v", first, second)
	}
	// A newly observed older edit must not change any previously issued cursor.
	apply(t, r, testEvent(EditEvent, "old-arrival", 0, "older"))
	secondAgain, _ := r.Revisions(ctx, "account", "chat", "message", first[1].Number, 2)
	if fmt.Sprint(secondAgain) != fmt.Sprint(second) {
		t.Fatal("revision cursor changed after late history")
	}
}

func TestRepliesStatusesAndReactionProjection(t *testing.T) {
	r := testRepository(t)
	ctx := context.Background()
	original := testEvent(MessageEvent, "original", 0, "source text")
	apply(t, r, original)
	reply := testEvent(MessageEvent, "reply", 1, "reply text")
	reply.MessageID = "reply"
	reply.ReplyToID, reply.ReplySender, reply.ReplyBody = original.MessageID, original.Sender, original.Body
	apply(t, r, reply)
	react := testEvent(ReactionEvent, "reaction-1", 2, "")
	react.ReactionToID, react.Reaction = original.MessageID, "👍"
	apply(t, r, react)
	updated := react
	updated.MessageID, updated.EventID, updated.Reaction, updated.Timestamp = "reaction-2", "reaction:2", "❤️", testTime.Add(3*time.Second)
	apply(t, r, updated)
	stale := react
	stale.MessageID, stale.EventID, stale.Reaction, stale.Timestamp = "reaction-stale", "reaction:stale", "👍", testTime.Add(2*time.Second)
	apply(t, r, stale)
	statusAt := time.Now().UTC()
	status := StatusUpdate{AccountID: "account", ID: "status-1", Sender: "peer", Name: "Peer", Body: "available", CreatedAt: statusAt}
	if err := r.SaveStatus(ctx, status); err != nil {
		t.Fatal(err)
	}

	messages, err := r.Messages(ctx, "account", "chat", nil, 10)
	if err != nil || len(messages) != 2 {
		t.Fatalf("messages: %v %v", messages, err)
	}
	if messages[0].ID != "reply" || messages[0].ReplyToID != "message" || messages[0].ReplySender != "sender" || messages[0].ReplyBody != "source text" {
		t.Fatalf("reply context lost: %+v", messages[0])
	}
	if len(messages[1].Reactions) != 1 || messages[1].Reactions[0] != (MessageReaction{Sender: "sender", Emoji: "❤️"}) {
		t.Fatalf("reaction projection: %+v", messages[1].Reactions)
	}
	statuses, err := r.Statuses(ctx, "account", statusAt.Add(-time.Hour), 10)
	if err != nil || len(statuses) != 1 || statuses[0].Body != "available" {
		t.Fatalf("statuses: %+v %v", statuses, err)
	}
	removed := updated
	removed.MessageID, removed.EventID, removed.Reaction = "reaction-remove", "reaction:remove", ""
	removed.Timestamp = testTime.Add(4 * time.Second)
	apply(t, r, removed)
	messages, err = r.Messages(ctx, "account", "chat", nil, 10)
	if err != nil || len(messages[1].Reactions) != 0 {
		t.Fatalf("reaction removal: %+v %v", messages, err)
	}
}

func TestMalformedEventsAndTransactionRollback(t *testing.T) {
	r := testRepository(t)
	ctx := context.Background()
	for _, e := range []IncomingEvent{{}, testEvent("invalid", "bad", 0, ""), func() IncomingEvent { e := testEvent(DeliveryEvent, "bad", 0, ""); e.Status = "bogus"; return e }()} {
		if err := r.ApplyEvent(ctx, e); err == nil {
			t.Fatal("malformed event accepted")
		}
	}
	if _, err := r.db.Exec(`CREATE TRIGGER fail_revision BEFORE INSERT ON account_revisions BEGIN SELECT RAISE(ABORT,'test failure'); END`); err != nil {
		t.Fatal(err)
	}
	e := testEvent(MessageEvent, "rollback", 0, "secret")
	if err := r.ApplyEvent(ctx, e); err == nil {
		t.Fatal("expected failed write")
	}
	chats, _ := r.Conversations(ctx, "account", nil, 10)
	if len(chats) != 0 {
		t.Fatal("partial conversation persisted")
	}
	if _, err := r.db.Exec(`DROP TRIGGER fail_revision`); err != nil {
		t.Fatal(err)
	}
	apply(t, r, e)
	messages, _ := r.Messages(ctx, "account", "chat", nil, 10)
	if len(messages) != 1 {
		t.Fatal("rollback poisoned event idempotency")
	}
}

func TestLegacyMigrationAndReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}

	_, err = db.Exec(`
CREATE TABLE conversations(id TEXT PRIMARY KEY,title TEXT NOT NULL,updated_at TEXT NOT NULL);
CREATE TABLE messages(id TEXT PRIMARY KEY,conversation_id TEXT NOT NULL REFERENCES conversations(id),sender TEXT NOT NULL,body TEXT NOT NULL,created_at TEXT NOT NULL,updated_at TEXT NOT NULL,deleted_at TEXT);
CREATE TABLE message_revisions(message_id TEXT NOT NULL REFERENCES messages(id),number INTEGER NOT NULL,body TEXT NOT NULL,edited_at TEXT NOT NULL,PRIMARY KEY(message_id,number));
INSERT INTO conversations VALUES('chat','Old chat','2026-01-01T12:00:10Z');
INSERT INTO messages VALUES('message','chat','sender','edited','2026-01-01T12:00:00Z','2026-01-01T12:00:10Z','2026-01-01T12:00:10Z');
INSERT INTO message_revisions VALUES('message',1,'original','2026-01-01T12:00:00Z'),('message',2,'edited','2026-01-01T12:00:05Z');`)
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	for i := 0; i < 2; i++ {
		r, err := Open(path)
		if err != nil {
			t.Fatal(err)
		}
		messages, err := r.Messages(context.Background(), LegacyAccountID, "chat", nil, 10)
		if err != nil || len(messages) != 1 || messages[0].Body != "edited" || !messages[0].Deleted {
			t.Fatalf("migration: %v %v", messages, err)
		}
		revisions, err := r.ListRevisions(context.Background(), "message")
		if err != nil || len(revisions) != 2 || revisions[0].Body != "original" {
			t.Fatalf("legacy revisions: %v %v", revisions, err)
		}
		live, _ := r.Conversations(context.Background(), "new-account", nil, 10)
		if len(live) != 0 {
			t.Fatal("legacy attributed to live account")
		}
		var version int
		if err := r.db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil || version != schemaVersion {
			t.Fatalf("version %d %v", version, err)
		}
		r.Close()
	}
}

func TestVersionOneAndTwoMigrationsAddReplyStatusAndReactionStorage(t *testing.T) {
	ctx := context.Background()
	v1Path := filepath.Join(t.TempDir(), "v1.db")
	db, err := sql.Open("sqlite", v1Path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`
CREATE TABLE account_messages(
 account_id TEXT NOT NULL, chat_id TEXT NOT NULL, id TEXT NOT NULL,
 sender TEXT NOT NULL, body TEXT NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL,
 deleted_at TEXT, body_at TEXT NOT NULL, body_event TEXT NOT NULL,
 from_me INTEGER NOT NULL, edited INTEGER NOT NULL, status TEXT NOT NULL, original_seen INTEGER NOT NULL,
 PRIMARY KEY(account_id,chat_id,id)
);
PRAGMA user_version=1;`)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	v1, err := Open(v1Path)
	if err != nil {
		t.Fatal(err)
	}
	var version int
	if err := v1.db.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&version); err != nil || version != schemaVersion {
		t.Fatalf("v1 migration version: %d %v", version, err)
	}
	var tables int
	if err := v1.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name IN ('account_statuses','account_reactions')`).Scan(&tables); err != nil || tables != 2 {
		t.Fatalf("v1 migration tables: %d %v", tables, err)
	}
	if err := v1.Close(); err != nil {
		t.Fatal(err)
	}

	v2Path := filepath.Join(t.TempDir(), "v2.db")
	v2, err := Open(v2Path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := v2.db.Exec(`DROP TABLE account_reactions; PRAGMA user_version=2;`); err != nil {
		t.Fatal(err)
	}
	if err := v2.Close(); err != nil {
		t.Fatal(err)
	}
	v2, err = Open(v2Path)
	if err != nil {
		t.Fatal(err)
	}
	defer v2.Close()
	if err := v2.db.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&version); err != nil || version != schemaVersion {
		t.Fatalf("v2 migration version: %d %v", version, err)
	}
}

func TestDatabasePermissionsAndSymlinkRejection(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "db")
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	r, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, suffix := range []string{"", "-wal", "-shm"} {
		info, err := os.Stat(path + suffix)
		if err != nil || info.Mode().Perm() != 0o600 {
			t.Fatalf("unsafe mode %s: %v %v", suffix, info, err)
		}
	}
	r.Close()
	link := filepath.Join(root, "symlink")
	if err := os.Symlink(path, link); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(link); err == nil {
		t.Fatal("accepted database symlink")
	}
	if err := os.Symlink(path, path+"-wal"); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(path); err == nil {
		t.Fatal("accepted WAL symlink")
	}
}

func TestLiteralFilenameAndClosedRepository(t *testing.T) {
	path := filepath.Join(t.TempDir(), "file:history?mode=memory&cache=shared.db")
	r, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	apply(t, r, testEvent(MessageEvent, "original", 0, "persisted"))
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	if err := r.ApplyEvent(context.Background(), testEvent(EditEvent, "closed", 1, "no write")); err == nil {
		t.Fatal("closed repository accepted write")
	}
	r, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	messages, err := r.Messages(context.Background(), "account", "chat", nil, 10)
	if err != nil || len(messages) != 1 || messages[0].Body != "persisted" {
		t.Fatalf("literal filename was interpreted as DSN: %v %v", messages, err)
	}
}

func TestBoundedReadsAndDeterministicEqualTimeEdits(t *testing.T) {
	r := testRepository(t)
	ctx := context.Background()
	for i := 0; i < 203; i++ {
		e := testEvent(MessageEvent, "original", 0, "body")
		e.MessageID = fmt.Sprintf("%03d", i)
		apply(t, r, e)
	}
	messages, err := r.Messages(ctx, "account", "chat", nil, 10000)
	if err != nil || len(messages) != 200 {
		t.Fatalf("unbounded read: %d %v", len(messages), err)
	}
	if _, err := r.Messages(ctx, "account", "chat", nil, 0); err == nil {
		t.Fatal("zero page accepted")
	}
	apply(t, r, testEvent(EditEvent, "z", 2, "winner"))
	apply(t, r, testEvent(EditEvent, "a", 2, "loser"))
	messages, _ = r.Messages(ctx, "account", "chat", nil, 1)
	if messages[0].Body != "winner" {
		t.Fatalf("unstable equal-time edit: %#v", messages[0])
	}
}
