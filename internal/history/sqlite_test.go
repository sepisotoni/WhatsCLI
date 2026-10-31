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
