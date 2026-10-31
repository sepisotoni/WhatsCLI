// Package messaging defines the boundary between the terminal app and a messaging transport.
package messaging

import (
	"context"

	"whatscli/internal/history"
)

type Event struct {
	ConversationID string
	Message        history.Message
}

type Client interface {
	Connect(context.Context) error
	Close() error
	ListConversations(context.Context) ([]history.Conversation, error)
	SendMessage(context.Context, string, string) (history.Message, error)
	Events() <-chan Event
}
