package history

import (
	"context"
	"time"
)

type Conversation struct {
	ID        string
	Title     string
	UpdatedAt time.Time
}

type Message struct {
	ID             string
	ConversationID string
	Sender         string
	Body           string
	CreatedAt      time.Time
	UpdatedAt      time.Time
	Deleted        bool
	DeletedAt      *time.Time
}

type Revision struct {
	MessageID string
	Number    int
	Body      string
	EditedAt  time.Time
}

type Repository interface {
	Close() error
	SaveConversation(context.Context, Conversation) error
	ListConversations(context.Context) ([]Conversation, error)
	AddMessage(context.Context, Message) (Message, error)
	EditMessage(context.Context, string, string) error
	DeleteMessage(context.Context, string) error
	ListMessages(context.Context, string, int) ([]Message, error)
	ListRevisions(context.Context, string) ([]Revision, error)
}
