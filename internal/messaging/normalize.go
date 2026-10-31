package messaging

import (
	"crypto/sha256"
	"fmt"
	"time"

	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/proto/waWeb"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"

	"whatscli/internal/history"
)

// normalizeMessage preserves source identities and times, not receive times.
// It does not download media or claim to reconstruct content not supplied by WA.
func normalizeMessage(account string, source *events.Message, title string) (history.IncomingEvent, bool) {
	if source == nil || account == "" || source.Info.ID == "" {
		return history.IncomingEvent{}, false
	}
	chat := source.Info.Chat.ToNonAD()
	if chat.Server != types.DefaultUserServer && chat.Server != types.HiddenUserServer && chat.Server != types.GroupServer {
		return history.IncomingEvent{}, false
	}
	if title == "" && !source.Info.IsGroup && !source.Info.IsFromMe {
		title = source.Info.PushName
	}
	event := history.IncomingEvent{
		Kind: history.MessageEvent, AccountID: account, ChatID: chat.String(), Title: title,
		MessageID: source.Info.ID, Sender: source.Info.Sender.ToNonAD().String(),
		Timestamp: source.Info.Timestamp.UTC(), FromMe: source.Info.IsFromMe,
	}
	if event.FromMe {
		event.Status = "sent"
	}
	message := source.RawMessage
	if message == nil {
		message = source.Message
	}
	message, edited := unwrap(message)
	if protocol := message.GetProtocolMessage(); protocol != nil {
		switch protocol.GetType() {
		case waE2E.ProtocolMessage_REVOKE:
			event.Kind = history.DeleteEvent
			event.MessageID = protocol.GetKey().GetID()
		case waE2E.ProtocolMessage_MESSAGE_EDIT:
			event.Kind = history.EditEvent
			event.MessageID = protocol.GetKey().GetID()
			message, _ = unwrap(protocol.GetEditedMessage())
		default:
			return history.IncomingEvent{}, false // Key distribution and other internal protocol messages.
		}
		if protocol.GetTimestampMS() > 0 {
			event.Timestamp = time.UnixMilli(protocol.GetTimestampMS()).UTC()
		}
	} else if edited || source.IsEdit {
		event.Kind = history.EditEvent
	}
	stub := source.SourceWebMsg.GetMessageStubType()
	if stub == waWeb.WebMessageInfo_REVOKE || stub == waWeb.WebMessageInfo_ADMIN_REVOKE {
		event.Kind = history.DeleteEvent
	}
	if event.MessageID == "" {
		return history.IncomingEvent{}, false
	}
	if event.Kind != history.DeleteEvent {
		event.Body = messageBody(message)
		if message == nil && source.SourceWebMsg != nil {
			event.Body = "[unavailable message or system event]"
		}
	}
	switch event.Kind {
	case history.MessageEvent:
		event.EventID = "message:" + event.MessageID
	case history.EditEvent:
		// Some WA edit envelopes reuse the original ID. Include source time and
		// content, making both replayed history and successive edits idempotent.
		event.EventID = fmt.Sprintf("edit:%s:%d:%x", event.MessageID, event.Timestamp.UnixMilli(), sha256.Sum256([]byte(event.Body)))
	case history.DeleteEvent:
		event.EventID = fmt.Sprintf("delete:%s:%d", event.MessageID, event.Timestamp.UnixMilli())
	}
	return event, true
}

func unwrap(message *waE2E.Message) (*waE2E.Message, bool) {
	edited := false
	for depth := 0; message != nil && depth < 16; depth++ {
		var inner *waE2E.Message
		switch {
		case message.GetDeviceSentMessage().GetMessage() != nil:
			inner = message.GetDeviceSentMessage().GetMessage()
		case message.GetEphemeralMessage().GetMessage() != nil:
			inner = message.GetEphemeralMessage().GetMessage()
		case message.GetViewOnceMessage().GetMessage() != nil:
			inner = message.GetViewOnceMessage().GetMessage()
		case message.GetViewOnceMessageV2().GetMessage() != nil:
			inner = message.GetViewOnceMessageV2().GetMessage()
		case message.GetViewOnceMessageV2Extension().GetMessage() != nil:
			inner = message.GetViewOnceMessageV2Extension().GetMessage()
		case message.GetDocumentWithCaptionMessage().GetMessage() != nil:
			inner = message.GetDocumentWithCaptionMessage().GetMessage()
		case message.GetEditedMessage().GetMessage() != nil:
			inner, edited = message.GetEditedMessage().GetMessage(), true
		case message.GetBotInvokeMessage().GetMessage() != nil:
			inner = message.GetBotInvokeMessage().GetMessage()
		default:
			return message, edited
		}
		message = inner
	}
	return message, edited
}

func messageBody(message *waE2E.Message) string {
	if message == nil {
		return "[unavailable message]"
	}
	switch {
	case message.Conversation != nil:
		return message.GetConversation()
	case message.ExtendedTextMessage != nil:
		return message.GetExtendedTextMessage().GetText()
	case message.DocumentMessage != nil:
		return "[document: " + message.GetDocumentMessage().GetFileName() + "]"
	case message.ImageMessage != nil:
		return caption("image", message.GetImageMessage().GetCaption())
	case message.VideoMessage != nil:
		return caption("video", message.GetVideoMessage().GetCaption())
	case message.AudioMessage != nil:
		return "[audio; not downloaded]"
	case message.StickerMessage != nil:
		return "[sticker; not downloaded]"
	case message.ReactionMessage != nil:
		return "[reaction: " + message.GetReactionMessage().GetText() + "]"
	default:
		return "[unsupported message]"
	}
}

func caption(kind, text string) string {
	body := "[" + kind + "; not downloaded]"
	if text != "" {
		body += " " + text
	}
	return body
}
