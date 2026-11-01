package history

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestHistoryKeepsMessageRevisionsAndDeletionMarker(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "history.db")
	repo, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.SaveConversation(ctx, Conversation{ID: "chat-1", Title: "Example"}); err != nil {
		t.Fatal(err)
	}
	_, err = repo.AddMessage(ctx, Message{
		ID:             "message-1",
		ConversationID: "chat-1",
		Sender:         "me",
		Body:           "first draft",
		CreatedAt:      time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.EditMessage(ctx, "message-1", "corrected text"); err != nil {
		t.Fatal(err)
	}
	if err := repo.DeleteMessage(ctx, "message-1"); err != nil {
		t.Fatal(err)
	}
	if err := repo.Close(); err != nil {
		t.Fatal(err)
	}

	repo, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()

	messages, err := repo.ListMessages(ctx, "chat-1", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 1 || messages[0].Body != "corrected text" || !messages[0].Deleted {
		t.Fatalf("unexpected persisted message: %#v", messages)
	}
	revisions, err := repo.ListRevisions(ctx, "message-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(revisions) != 2 || revisions[0].Body != "first draft" || revisions[1].Body != "corrected text" {
		t.Fatalf("unexpected revisions: %#v", revisions)
	}
}

func TestConversationPreviewAndIncomingSince(t *testing.T) {
	r, err := Open(filepath.Join(t.TempDir(), "h.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	ctx := context.Background()
	base := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	add := func(chat, id, body string, at time.Duration, me bool, status string) {
		if err := r.ApplyEvent(ctx, IncomingEvent{Kind: MessageEvent, AccountID: "acc", ChatID: chat, Title: chat, MessageID: id, Sender: "peer", Body: body, Timestamp: base.Add(at), FromMe: me, Status: status}); err != nil {
			t.Fatal(err)
		}
	}
	add("c1", "1", "first", 0, false, "")
	add("c1", "2", "mine", time.Minute, true, "read")
	add("c2", "3", "incoming", 2*time.Minute, false, "")
	add("c2", "4", "newer incoming", 3*time.Minute, false, "")
	chats, err := r.Conversations(ctx, "acc", nil, 10)
	if err != nil || len(chats) != 2 {
		t.Fatal(chats, err)
	}
	byID := map[string]Conversation{}
	for _, c := range chats {
		byID[c.ID] = c
	}
	if c := byID["c1"]; c.LastBody != "mine" || !c.LastFromMe || c.LastStatus != "read" {
		t.Fatalf("c1 preview: %+v", c)
	}
	if c := byID["c2"]; c.LastBody != "newer incoming" || c.LastFromMe {
		t.Fatalf("c2 preview: %+v", c)
	}
	stamps, err := r.IncomingSince(ctx, "acc", base.Add(time.Minute+time.Second))
	if err != nil || len(stamps) != 2 || stamps[0].ChatID != "c2" {
		t.Fatalf("unread stamps: %+v %v", stamps, err)
	}
	if stamps, _ := r.IncomingSince(ctx, "other-account", base.Add(-time.Hour)); len(stamps) != 0 {
		t.Fatal("account isolation broken")
	}
}
