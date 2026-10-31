package messaging

import (
	"reflect"
	"testing"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waCommon"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/proto/waWeb"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"

	"whatscli/internal/history"
)

func fixtureMessage() *events.Message {
	return &events.Message{
		Info: types.MessageInfo{
			ID: "original", Timestamp: time.Unix(1000, 0), PushName: "Peer",
			MessageSource: types.MessageSource{Chat: types.NewJID("200", types.DefaultUserServer), Sender: types.NewJID("200", types.DefaultUserServer)},
		},
		Message: &waE2E.Message{Conversation: proto.String("hello 🙂")},
	}
}

func TestNormalizeTextAndNestedEdit(t *testing.T) {
	source := fixtureMessage()
	original, ok := normalizeMessage("100@s.whatsapp.net", source, "")
	if !ok || original.Body != "hello 🙂" || original.Title != "Peer" || original.EventID != "message:original" || !original.Timestamp.Equal(source.Info.Timestamp) {
		t.Fatalf("incorrect original: %+v", original)
	}
	edit := &waE2E.Message{ProtocolMessage: &waE2E.ProtocolMessage{
		Type:          waE2E.ProtocolMessage_MESSAGE_EDIT.Enum(),
		Key:           &waCommon.MessageKey{ID: proto.String("original")},
		EditedMessage: &waE2E.Message{ExtendedTextMessage: &waE2E.ExtendedTextMessage{Text: proto.String("changed")}},
		TimestampMS:   proto.Int64(1000123),
	}}
	source.RawMessage = &waE2E.Message{EphemeralMessage: &waE2E.FutureProofMessage{Message: &waE2E.Message{EditedMessage: &waE2E.FutureProofMessage{Message: edit}}}}
	source.Info.ID = "edit-envelope"
	first, ok := normalizeMessage("100@s.whatsapp.net", source, "")
	if !ok || first.Kind != history.EditEvent || first.MessageID != "original" || first.Body != "changed" || first.Timestamp.UnixMilli() != 1000123 {
		t.Fatalf("incorrect edit: %+v", first)
	}
	second, _ := normalizeMessage("100@s.whatsapp.net", source, "")
	if !reflect.DeepEqual(first, second) {
		t.Fatal("replayed edit changed identity")
	}
	edit.ProtocolMessage.EditedMessage.ExtendedTextMessage.Text = proto.String("another edit")
	third, _ := normalizeMessage("100@s.whatsapp.net", source, "")
	if third.EventID == first.EventID {
		t.Fatal("different edit content reused identity")
	}
}

func TestNormalizeRevocationAndUnsupported(t *testing.T) {
	source := fixtureMessage()
	source.Message = &waE2E.Message{ProtocolMessage: &waE2E.ProtocolMessage{Type: waE2E.ProtocolMessage_REVOKE.Enum(), Key: &waCommon.MessageKey{ID: proto.String("target")}}}
	event, ok := normalizeMessage("account", source, "")
	if !ok || event.Kind != history.DeleteEvent || event.MessageID != "target" || event.Body != "" {
		t.Fatalf("incorrect revoke: %+v", event)
	}
	source.Message = nil
	source.SourceWebMsg = &waWeb.WebMessageInfo{MessageStubType: waWeb.WebMessageInfo_REVOKE.Enum()}
	event, ok = normalizeMessage("account", source, "")
	if !ok || event.Kind != history.DeleteEvent || event.MessageID != "original" {
		t.Fatalf("incorrect history revoke: %+v", event)
	}
	source.SourceWebMsg = nil
	source.Message = &waE2E.Message{AudioMessage: &waE2E.AudioMessage{}}
	event, ok = normalizeMessage("account", source, "")
	if !ok || event.Body != "[audio; not downloaded]" {
		t.Fatalf("incorrect unsupported body: %+v", event)
	}
	source.Message = &waE2E.Message{ProtocolMessage: &waE2E.ProtocolMessage{Type: waE2E.ProtocolMessage_HISTORY_SYNC_NOTIFICATION.Enum()}}
	if _, ok = normalizeMessage("account", source, ""); ok {
		t.Fatal("internal protocol message became visible content")
	}
	if _, ok = normalizeMessage("", fixtureMessage(), ""); ok {
		t.Fatal("accepted message without account")
	}
}

func TestHistoryAndLiveEditNormalizeIdentically(t *testing.T) {
	jid := types.NewJID("100", types.DefaultUserServer)
	client := whatsmeow.NewClient(&store.Device{ID: &jid}, nil)
	message := &waE2E.Message{EditedMessage: &waE2E.FutureProofMessage{Message: &waE2E.Message{ProtocolMessage: &waE2E.ProtocolMessage{
		Type: waE2E.ProtocolMessage_MESSAGE_EDIT.Enum(), Key: &waCommon.MessageKey{ID: proto.String("original")},
		EditedMessage: &waE2E.Message{Conversation: proto.String("edited")}, TimestampMS: proto.Int64(1500000),
	}}}}
	web := &waWeb.WebMessageInfo{Key: &waCommon.MessageKey{ID: proto.String("edit-envelope"), RemoteJID: proto.String("200@s.whatsapp.net")}, MessageTimestamp: proto.Uint64(1500), Message: message}
	parsed, err := client.ParseWebMessage(types.EmptyJID, web)
	if err != nil {
		t.Fatal(err)
	}
	live := fixtureMessage()
	live.RawMessage = message
	live.Info.ID = "edit-envelope"
	live.Info.Timestamp = time.Unix(1500, 0)
	a, ok := normalizeMessage("100@s.whatsapp.net", parsed, "")
	if !ok {
		t.Fatal("history edit rejected")
	}
	b, ok := normalizeMessage("100@s.whatsapp.net", live, "")
	if !ok || a.EventID != b.EventID || a.MessageID != b.MessageID || a.Body != b.Body || a.Kind != b.Kind {
		t.Fatalf("history/live mismatch: %+v versus %+v", a, b)
	}
}

func TestNormalizeGroupDoesNotUseSenderAsChatTitle(t *testing.T) {
	source := fixtureMessage()
	source.Info.Chat = types.NewJID("123-456", types.GroupServer)
	source.Info.IsGroup = true
	event, ok := normalizeMessage("account", source, "")
	if !ok || event.Title != "" || event.Sender != "200@s.whatsapp.net" {
		t.Fatalf("incorrect group: %+v", event)
	}
	event, _ = normalizeMessage("account", source, "Group name")
	if event.Title != "Group name" {
		t.Fatal("lost history group title")
	}
}
