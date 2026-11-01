package history

import (
	"context"
	"time"
)

// LegacyAccountID is reserved for history created before account-aware storage.
const LegacyAccountID = "local"

type EventKind string

const (
	MessageEvent  EventKind = "message"
	EditEvent     EventKind = "edit"
	DeleteEvent   EventKind = "delete"
	DeliveryEvent EventKind = "delivery"
	ReactionEvent EventKind = "reaction"
)

type IncomingEvent struct {
	Kind                                                               EventKind
	AccountID, ChatID, Title, MessageID, EventID, Sender, Body, Status string
	Timestamp                                                          time.Time
	FromMe                                                             bool
	ReplyToID, ReplySender, ReplyBody                                  string
	ReactionToID, Reaction                                             string
}

type StatusUpdate struct {
	AccountID, ID, Sender, Name, Body string
	CreatedAt                         time.Time
}

type Cursor struct {
	Time time.Time
	ID   string
}

type Conversation struct {
	AccountID string
	ID        string
	Title     string
	UpdatedAt time.Time
	// Latest-message preview fields, filled by Conversations.
	LastBody, LastSender, LastStatus string
	LastFromMe                       bool
}

// Stamp is the time of one incoming message, used for unread counts.
type Stamp struct {
	ChatID string
	At     time.Time
}

type Message struct {
	AccountID      string
	ID             string
	ConversationID string
	Sender         string
	Body           string
	CreatedAt      time.Time
	UpdatedAt      time.Time
	Deleted        bool
	DeletedAt      *time.Time
	FromMe         bool
	Edited         bool
	Status         string
	ReplyToID      string
	ReplySender    string
	ReplyBody      string
	Reactions      []MessageReaction
}

type MessageReaction struct {
	Sender string
	Emoji  string
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
	ApplyEvent(context.Context, IncomingEvent) error
	SaveStatus(context.Context, StatusUpdate) error
	Statuses(context.Context, string, time.Time, int) ([]StatusUpdate, error)
	Conversations(context.Context, string, *Cursor, int) ([]Conversation, error)
	Messages(context.Context, string, string, *Cursor, int) ([]Message, error)
	Revisions(context.Context, string, string, string, int, int) ([]Revision, error)
}
