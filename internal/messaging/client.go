// Package messaging isolates the unofficial messaging transport from the UI.
package messaging

import (
	"context"

	"whatscli/internal/history"
)

type EventType string

const (
	QR             EventType = "qr"
	Connecting     EventType = "connecting"
	Connected      EventType = "connected"
	Disconnected   EventType = "disconnected"
	LoggedOut      EventType = "logged-out"
	PairingExpired EventType = "pairing-expired"
	Failure        EventType = "failure"
	Incoming       EventType = "incoming"
)

// Event.Code is pairing material: display it locally but never log it.
type Event struct {
	Type      EventType
	AccountID string
	Code      string
	Err       error
	Message   *history.IncomingEvent
	Status    *history.StatusUpdate
}

type Client interface {
	Connect(context.Context) error
	Close() error
	Logout(context.Context) error
	Events() <-chan Event
	NewMessageID() string
	SendText(context.Context, string, string, string) (history.IncomingEvent, error)
	SendFile(context.Context, string, string, string) (history.IncomingEvent, error)
}

type StatusClient interface {
	SendStatus(context.Context, string, string) (history.StatusUpdate, error)
}

type ReplyClient interface {
	SendReply(context.Context, string, string, string, history.Message) (history.IncomingEvent, error)
}

type ReactionClient interface {
	SendReaction(context.Context, string, history.Message, string, string) (history.IncomingEvent, error)
}
