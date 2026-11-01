package messaging

import (
	"testing"
	"time"

	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"
)

func TestPhoneRecipient(t *testing.T) {
	for _, input := range []string{"+27 82 123 4567", "27821234567", "+27 (82) 123-4567"} {
		got, err := PhoneRecipient(input)
		if err != nil || got != "27821234567@s.whatsapp.net" {
			t.Fatalf("%q: %q %v", input, got, err)
		}
	}
	for _, input := range []string{"", "+", "0821234567", "123", "1234567890123456", "abc", "12+345678", "1234567@bot", "1234567\nx"} {
		if _, err := PhoneRecipient(input); err == nil {
			t.Fatalf("accepted invalid phone %q", input)
		}
	}
}

func TestMetaAIReceiveAndDestination(t *testing.T) {
	for _, jid := range []types.JID{types.NewMetaAIJID, types.MetaAIJID} {
		if _, err := destination(jid.String()); err != nil {
			t.Fatal("bot target rejected", err)
		}
		source := fixtureMessage()
		source.Info.Chat, source.Info.Sender = jid, jid
		source.Message = &waE2E.Message{BotInvokeMessage: &waE2E.FutureProofMessage{
			Message: &waE2E.Message{ExtendedTextMessage: &waE2E.ExtendedTextMessage{Text: proto.String("Synthetic bot response")}},
		}}
		event, ok := normalizeMessage("account", source, "")
		if !ok || event.Body != "Synthetic bot response" || event.Title != "Meta AI" {
			t.Fatalf("bot reply discarded: %+v", event)
		}
	}
}

func TestReceiptsDistinguishDeliveryAndRead(t *testing.T) {
	client, err := New(t.TempDir() + "/session.db")
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	transport := client.(*transport)
	transport.setAccount("account")
	for _, tc := range []struct {
		kind types.ReceiptType
		want string
	}{{types.ReceiptTypeDelivered, "delivered"}, {types.ReceiptTypeRead, "read"}, {types.ReceiptTypePlayed, "read"}} {
		transport.handle(nil, &events.Receipt{
			MessageSource: types.MessageSource{Chat: types.NewJID("200", types.DefaultUserServer)},
			MessageIDs:    []types.MessageID{"test"}, Timestamp: time.Now(), Type: tc.kind,
		})
		event := <-client.Events()
		if event.Message == nil || event.Message.Status != tc.want {
			t.Fatalf("receipt %s: %+v", tc.kind, event)
		}
	}
}
