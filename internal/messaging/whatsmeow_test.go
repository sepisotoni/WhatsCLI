package messaging

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waAdv"
	"go.mau.fi/whatsmeow/proto/waCommon"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/proto/waHistorySync"
	"go.mau.fi/whatsmeow/proto/waWeb"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"

	"whatscli/internal/history"
)

func TestNewIsLazyAndCancelledConnectDoesNotCreateCredentials(t *testing.T) {
	path := filepath.Join(t.TempDir(), "absent", "session.db")
	client, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	if client.NewMessageID() == "" {
		t.Fatal("empty outgoing ID")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := client.Connect(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled Connect: %v", err)
	}
	if _, err := client.SendText(context.Background(), "200@s.whatsapp.net", "hello", "test"); !errors.Is(err, ErrOffline) {
		t.Fatalf("offline send: %v", err)
	}
	if err := client.Logout(context.Background()); !errors.Is(err, ErrOffline) {
		t.Fatalf("offline logout: %v", err)
	}
	if _, err := os.Stat(filepath.Dir(path)); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("constructor or cancelled operation created state")
	}
	if err := client.Close(); err != nil {
		t.Fatal(err)
	}
	if err := client.Close(); err != nil {
		t.Fatal(err)
	}
	if _, ok := <-client.Events(); ok {
		t.Fatal("Events not closed")
	}
	if err := client.Connect(context.Background()); !errors.Is(err, ErrClosed) {
		t.Fatalf("closed Connect: %v", err)
	}
}

func TestCloseDrainsAcceptedProducersWithoutDropping(t *testing.T) {
	client, _ := New(filepath.Join(t.TempDir(), "session.db"))
	transport := client.(*transport)
	if !transport.begin() {
		t.Fatal("admission failed")
	}
	started := make(chan struct{})
	go func() {
		defer transport.producers.Done()
		for i := 0; i < 300; i++ {
			if i == 128 {
				close(started)
			}
			transport.emit(Event{Type: Incoming, Message: &history.IncomingEvent{MessageID: "accepted"}})
		}
	}()
	<-started
	closed := make(chan struct{})
	go func() { client.Close(); close(closed) }()
	<-transport.ctx.Done()
	if transport.begin() {
		t.Fatal("Close admitted a new producer")
	}
	select {
	case <-closed:
		t.Fatal("Close returned before accepted events drained")
	default:
	}
	count := 0
	for {
		select {
		case _, ok := <-client.Events():
			if !ok {
				if count != 300 {
					t.Fatalf("lost accepted events: %d", count)
				}
				<-closed
				return
			}
			count++
		case <-time.After(3 * time.Second):
			t.Fatal("shutdown blocked despite draining")
		}
	}
}

func TestConcurrentCloseIsIdempotent(t *testing.T) {
	client, _ := New(filepath.Join(t.TempDir(), "session.db"))
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := client.Close(); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
}

func TestQRRefreshExpiryFailureAndCancellation(t *testing.T) {
	client, _ := New(filepath.Join(t.TempDir(), "session.db"))
	transport := client.(*transport)
	ctx, cancel := context.WithCancel(context.Background())
	codes := make(chan whatsmeow.QRChannelItem)
	disconnected := make(chan struct{})
	done := make(chan struct{})
	go func() { transport.watchConnection(ctx, func() { close(disconnected) }, codes); close(done) }()
	for _, code := range []string{"synthetic-code-one", "synthetic-code-two"} {
		codes <- whatsmeow.QRChannelItem{Event: whatsmeow.QRChannelEventCode, Code: code}
		if event := <-client.Events(); event.Type != QR || event.Code != code {
			t.Fatal("QR refresh lost")
		}
	}
	codes <- whatsmeow.QRChannelTimeout
	if event := <-client.Events(); event.Type != PairingExpired {
		t.Fatal("expiry lost")
	}
	codes <- whatsmeow.QRChannelItem{Event: "error", Error: errors.New("secret raw error")}
	if event := <-client.Events(); event.Type != Failure || event.Err == nil || event.Err.Error() == "secret raw error" {
		t.Fatal("failure not sanitized")
	}
	codes <- whatsmeow.QRChannelSuccess
	close(codes)
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("cancelled QR watcher leaked")
	}
	<-disconnected
	select {
	case event := <-client.Events():
		t.Fatalf("QR success falsely reported connection: %s", event.Type)
	default:
	}
	client.Close()
}

func TestModerncSessionStoreRoundTripAndPrivatePermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "private", "session with spaces?.db")
	container, err := openSession(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	device, err := container.GetFirstDevice(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	jid := types.NewADJID("100", 0, 7)
	device.ID = &jid
	// Synthetic signatures only: this fixture never connects to a service.
	device.Account = &waAdv.ADVSignedDeviceIdentity{Details: []byte{}, AccountSignature: make([]byte, 64), AccountSignatureKey: make([]byte, 32), DeviceSignature: make([]byte, 64)}
	if err := device.Save(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := container.Close(); err != nil {
		t.Fatal(err)
	}
	container, err = openSession(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer container.Close()
	loaded, err := container.GetFirstDevice(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if loaded.GetJID() != jid || *loaded.IdentityKey.Pub != *device.IdentityKey.Pub || *loaded.IdentityKey.Priv != *device.IdentityKey.Priv {
		t.Fatal("session did not survive restart")
	}
	if got := accountID(whatsmeow.NewClient(loaded, nil)); got != "100@s.whatsapp.net" {
		t.Fatalf("account contains device suffix: %s", got)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("session permissions: %v %v", info, err)
	}
}

func TestHistorySyncUsesBoundedEventPath(t *testing.T) {
	client, _ := New(filepath.Join(t.TempDir(), "session.db"))
	transport := client.(*transport)
	if err := transport.initialize(context.Background()); err != nil {
		t.Fatal(err)
	}
	jid := types.NewJID("100", types.DefaultUserServer)
	transport.client.Store.ID = &jid
	transport.setAccount(jid.String())
	web := &waWeb.WebMessageInfo{Key: &waCommon.MessageKey{ID: proto.String("history"), FromMe: proto.Bool(false)}, MessageTimestamp: proto.Uint64(1000), Message: &waE2E.Message{Conversation: proto.String("old text")}}
	transport.handle(transport.client, &events.HistorySync{Data: &waHistorySync.HistorySync{Conversations: []*waHistorySync.Conversation{{ID: proto.String("200@s.whatsapp.net"), Name: proto.String("Chat"), Messages: []*waHistorySync.HistorySyncMsg{{Message: web}}}}}})
	select {
	case event := <-client.Events():
		if event.Type != Incoming || event.Message.Body != "old text" || event.Message.Title != "Chat" || event.Message.Timestamp.Unix() != 1000 {
			t.Fatalf("incorrect history event: %+v", event)
		}
	default:
		t.Fatal("history produced no event")
	}
	client.Close()
}

func TestLifecycleRetainsAccountAfterUpstreamDeviceDeletion(t *testing.T) {
	client, err := New(filepath.Join(t.TempDir(), "session.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	transport := client.(*transport)
	if err := transport.initialize(context.Background()); err != nil {
		t.Fatal(err)
	}
	jid := types.NewADJID("100", 0, 7)
	transport.handle(transport.client, &events.PairSuccess{ID: jid})
	select {
	case event := <-client.Events():
		t.Fatalf("pairing reported readiness before Connected: %+v", event)
	default:
	}
	transport.handle(transport.client, &events.Connected{})
	if event := <-client.Events(); event.Type != Connected || event.AccountID != "100@s.whatsapp.net" {
		t.Fatalf("connected account: %+v", event)
	}
	transport.handle(transport.client, &events.Disconnected{})
	if event := <-client.Events(); event.Type != Disconnected || event.AccountID != "100@s.whatsapp.net" {
		t.Fatalf("disconnected account: %+v", event)
	}
	// Upstream clears Store.ID concurrently with dispatching LoggedOut. These
	// callbacks must not read mutable store identity or lose its account scope.
	changed := make(chan struct{})
	go func() {
		defer close(changed)
		for i := 0; i < 1000; i++ {
			transport.client.Store.ID = &jid
			transport.client.Store.ID = nil
		}
	}()
	transport.handle(transport.client, &events.LoggedOut{})
	<-changed
	if event := <-client.Events(); event.Type != LoggedOut || event.AccountID != "100@s.whatsapp.net" {
		t.Fatalf("logged-out account lost after device deletion: %+v", event)
	}
	if account, revoked := transport.sessionState(); account != "100@s.whatsapp.net" || !revoked {
		t.Fatalf("session state: account=%q revoked=%v", account, revoked)
	}
	if _, _, _, err := transport.sendClient(context.Background()); !errors.Is(err, ErrOffline) {
		t.Fatalf("revoked session accepted send: %v", err)
	}
	if err := client.Logout(context.Background()); !errors.Is(err, ErrOffline) {
		t.Fatalf("revoked session accepted logout: %v", err)
	}
}

func TestRevokedSessionInitializesFreshDeviceOffline(t *testing.T) {
	client, err := New(filepath.Join(t.TempDir(), "session.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	transport := client.(*transport)
	if err := transport.initialize(context.Background()); err != nil {
		t.Fatal(err)
	}
	oldClient := transport.client
	transport.setAccount("100@s.whatsapp.net")
	transport.handle(oldClient, &events.LoggedOut{})
	<-client.Events()
	if err := transport.initialize(context.Background()); err != nil {
		t.Fatal(err)
	}
	if transport.client == oldClient {
		t.Fatal("revoked client reused")
	}
	if account, revoked := transport.sessionState(); account != "" || revoked {
		t.Fatalf("new device inherited revoked account: %q %v", account, revoked)
	}
	if !transport.client.Store.GetJID().IsEmpty() {
		t.Fatal("new unpaired device has an account")
	}
}

func TestPermanentDisconnectMarksConnectionOffline(t *testing.T) {
	client, err := New(filepath.Join(t.TempDir(), "session.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	transport := client.(*transport)
	transport.setAccount("100@s.whatsapp.net")
	transport.handle(nil, &events.StreamReplaced{})
	if event := <-client.Events(); event.Type != Disconnected || event.AccountID != "100@s.whatsapp.net" {
		t.Fatal("permanent disconnect did not clear online state")
	}
	if event := <-client.Events(); event.Type != Failure || event.Err == nil {
		t.Fatal("permanent disconnect did not report failure")
	}
}

func TestDestinationRejectsUnsupportedTargets(t *testing.T) {
	for _, value := range []string{"", "200", "@s.whatsapp.net", "status@broadcast", "123@newsletter", "123@example.com"} {
		if _, err := destination(value); err == nil {
			t.Fatalf("accepted unsupported target %q", value)
		}
	}
	for _, value := range []string{"200@s.whatsapp.net", "200@lid", "123-456@g.us"} {
		if _, err := destination(value); err != nil {
			t.Fatalf("rejected chat target %q", value)
		}
	}
}
