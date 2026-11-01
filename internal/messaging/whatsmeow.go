package messaging

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waCommon"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/proto"
	_ "modernc.org/sqlite"

	"whatscli/internal/history"
	"whatscli/internal/privatefs"
)

var (
	ErrClosed  = errors.New("messaging client is closed")
	ErrOffline = errors.New("WhatsApp is not connected")
)

// transport owns one lazy session and lets whatsmeow alone handle reconnection.
// Accepted producers block on the bounded queue rather than dropping history.
// Callers must drain Events concurrently with Close, including during shutdown.
type transport struct {
	path             string
	events           chan Event
	ctx              context.Context
	cancel           context.CancelFunc
	mu               sync.Mutex
	closing          bool
	account          string
	revoked          bool
	closed           chan struct{}
	closeErr         error
	producers        sync.WaitGroup
	opMu             sync.Mutex
	client           *whatsmeow.Client
	live             atomic.Pointer[whatsmeow.Client] // lock-free view for avatar lookups
	store            *sqlstore.Container
	handler          uint32
	connectionCancel context.CancelFunc
	connectionDone   <-chan struct{}
}

// New does not touch disk, generate device credentials, or contact WhatsApp.
// The application must obtain consent before calling Connect.
func New(sessionPath string) (Client, error) {
	if sessionPath == "" {
		return nil, errors.New("session path is required")
	}
	path, err := filepath.Abs(sessionPath)
	if err != nil {
		return nil, errors.New("invalid session path")
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &transport{path: path, events: make(chan Event, 128), ctx: ctx, cancel: cancel, closed: make(chan struct{})}, nil
}

func (t *transport) Events() <-chan Event { return t.events }

func (t *transport) begin() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closing {
		return false
	}
	t.producers.Add(1)
	return true
}

// emit is only called by admitted producers. Close waits for these sends before
// closing the channel. Cancellation deliberately does not discard queued history.
func (t *transport) emit(e Event) { t.events <- e }

func (t *transport) sessionState() (string, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.account, t.revoked
}

func (t *transport) setAccount(account string) {
	t.mu.Lock()
	t.account, t.revoked = account, false
	t.mu.Unlock()
}

func (t *transport) markLoggedOut() {
	t.mu.Lock()
	t.revoked = true
	t.mu.Unlock()
}

func (t *transport) initialize(ctx context.Context) error {
	_, revoked := t.sessionState()
	if t.client != nil && !revoked {
		return nil
	}
	if t.client != nil {
		t.client.RemoveEventHandler(t.handler)
		if err := t.store.Close(); err != nil {
			return errors.New("could not close previous session store")
		}
		t.client, t.store = nil, nil
		t.live.Store(nil)
	}
	container, err := openSession(ctx, t.path)
	if err != nil {
		return err
	}
	device, err := container.GetFirstDevice(ctx)
	if err != nil {
		container.Close()
		return errors.New("could not read device session")
	}
	client := whatsmeow.NewClient(device, waLog.Noop)
	client.EnableAutoReconnect = true
	client.InitialAutoReconnect = false
	t.client, t.store = client, container
	t.live.Store(client)
	t.setAccount(accountID(client))
	t.handler = client.AddEventHandler(func(event any) {
		if !t.begin() {
			return
		}
		defer t.producers.Done()
		t.handle(client, event)
	})
	return nil
}

func openSession(ctx context.Context, path string) (*sqlstore.Container, error) {
	if err := privatefs.EnsureFile(path); err != nil {
		return nil, errors.New("could not secure session database")
	}
	// SQLite may reopen existing sidecars. Reject unsafe ones before opening it.
	for _, suffix := range []string{"-wal", "-shm", "-journal"} {
		if _, err := os.Lstat(path + suffix); err == nil {
			if err = privatefs.EnsureFile(path + suffix); err != nil {
				return nil, errors.New("could not secure session sidecar")
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, errors.New("could not inspect session sidecar")
		}
	}
	u := url.URL{Scheme: "file", Path: path}
	q := u.Query()
	q.Add("_pragma", "foreign_keys(1)")
	q.Add("_pragma", "busy_timeout(5000)")
	u.RawQuery = q.Encode()
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return nil, errors.New("could not open session database")
	}
	db.SetMaxOpenConns(1)
	container := sqlstore.NewWithDB(db, "sqlite", waLog.Noop)
	if err := container.Upgrade(ctx); err != nil {
		container.Close()
		return nil, errors.New("could not initialize session schema")
	}
	return container, nil
}

func (t *transport) Connect(ctx context.Context) error {
	if !t.begin() {
		return ErrClosed
	}
	defer t.producers.Done()
	t.opMu.Lock()
	defer t.opMu.Unlock()
	if err := t.ctx.Err(); err != nil {
		return ErrClosed
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	_, revoked := t.sessionState()
	if t.client != nil && !revoked && t.client.IsConnected() {
		return nil
	}
	if t.connectionCancel != nil {
		t.connectionCancel()
		<-t.connectionDone
	}
	connCtx, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(t.ctx, cancel)
	fail := func(err error) error { cancel(); stop(); return err }
	if err := t.initialize(connCtx); err != nil {
		return fail(err)
	}
	client := t.client
	var codes <-chan whatsmeow.QRChannelItem
	account, _ := t.sessionState()
	if account == "" {
		var err error
		codes, err = client.GetQRChannel(connCtx)
		if err != nil {
			return fail(errors.New("could not start pairing"))
		}
	}
	done := make(chan struct{})
	t.connectionCancel, t.connectionDone = cancel, done
	// This worker belongs to the already admitted Connect operation, so adding it
	// remains safe even if Close has just stopped admitting new operations.
	t.producers.Add(1)
	go func() {
		defer t.producers.Done()
		defer close(done)
		defer stop()
		t.watchConnection(connCtx, client.Disconnect, codes)
	}()
	t.emit(Event{Type: Connecting, AccountID: account})
	if err := client.ConnectContext(connCtx); err != nil {
		cancel()
		<-done
		t.emit(Event{Type: Failure, Err: errors.New("WhatsApp connection failed; check connectivity and retry")})
		return errors.New("WhatsApp connection failed")
	}
	return nil
}

func (t *transport) watchConnection(ctx context.Context, disconnect func(), codes <-chan whatsmeow.QRChannelItem) {
	for {
		select {
		case <-ctx.Done():
			disconnect()
			return
		case item, ok := <-codes:
			if !ok {
				codes = nil
				continue
			}
			switch item.Event {
			case whatsmeow.QRChannelEventCode:
				t.emit(Event{Type: QR, Code: item.Code})
			case "success": // Connected, not QR success, means authentication is ready.
			case "timeout":
				t.emit(Event{Type: PairingExpired})
			case "error", "err-unexpected-state", "err-client-outdated", "err-scanned-without-multidevice", "passkey-request", "passkey-confirmation":
				t.emit(Event{Type: Failure, Err: errors.New("pairing failed or requires an unsupported verification step; retry linking")})
			}
		}
	}
}

func accountID(client *whatsmeow.Client) string {
	jid := client.Store.GetJID()
	if jid.IsEmpty() {
		return ""
	}
	return jid.ToNonAD().String()
}

// Close stops admission, cancels network operations, and waits for accepted
// callbacks and sends. Events closes exactly once, only after those producers
// finish. The caller must continue consuming Events until it closes.
func (t *transport) Close() error {
	t.mu.Lock()
	if t.closing {
		t.mu.Unlock()
		<-t.closed
		return t.closeErr
	}
	t.closing = true
	t.cancel()
	t.mu.Unlock()
	t.opMu.Lock()
	if t.connectionCancel != nil {
		t.connectionCancel()
	}
	if t.client != nil {
		t.client.Disconnect()
		t.client.RemoveEventHandler(t.handler)
	}
	t.opMu.Unlock()
	t.producers.Wait()
	if t.store != nil {
		t.closeErr = t.store.Close()
	}
	close(t.events)
	close(t.closed)
	return t.closeErr
}

func (t *transport) Logout(ctx context.Context) error {
	if !t.begin() {
		return ErrClosed
	}
	defer t.producers.Done()
	t.opMu.Lock()
	defer t.opMu.Unlock()
	_, revoked := t.sessionState()
	if t.client == nil || revoked || !t.client.IsLoggedIn() {
		return ErrOffline
	}
	ctx, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(t.ctx, cancel)
	defer cancel()
	defer stop()
	account, _ := t.sessionState()
	if err := t.client.Logout(ctx); err != nil {
		return errors.New("unlink failed; the session may still be linked")
	}
	if t.connectionCancel != nil {
		t.connectionCancel()
	}
	t.markLoggedOut()
	t.emit(Event{Type: LoggedOut, AccountID: account})
	return nil
}

// NewMessageID mirrors the upstream web-ID scheme (hash of time, own user and
// random bytes) without taking the operation lock, so it can never stall behind
// a long Connect. It works before consent: no store or network is touched.
func (t *transport) NewMessageID() string {
	account, _ := t.sessionState()
	data := make([]byte, 8, 8+len(account)+5+16)
	binary.BigEndian.PutUint64(data, uint64(time.Now().Unix()))
	if user, _, ok := strings.Cut(account, "@"); ok && user != "" {
		data = append(data, user...)
		data = append(data, "@c.us"...)
	}
	random := make([]byte, 16)
	if _, err := rand.Read(random); err != nil {
		panic("system random source unavailable")
	}
	data = append(data, random...)
	sum := sha256.Sum256(data)
	return whatsmeow.WebMessageIDPrefix + strings.ToUpper(hex.EncodeToString(sum[:9]))
}

func destination(chat string) (types.JID, error) {
	jid, err := types.ParseJID(chat)
	if err != nil || !supportedChat(jid) {
		return types.EmptyJID, errors.New("destination must be a direct, group, or bot chat ID")
	}
	return jid.ToNonAD(), nil
}

func (t *transport) sendClient(ctx context.Context) (*whatsmeow.Client, context.Context, func(), error) {
	if !t.begin() {
		return nil, nil, nil, ErrClosed
	}
	t.opMu.Lock()
	client := t.client
	_, revoked := t.sessionState()
	if client == nil || revoked || !client.IsLoggedIn() {
		t.opMu.Unlock()
		t.producers.Done()
		return nil, nil, nil, ErrOffline
	}
	ctx, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(t.ctx, cancel)
	return client, ctx, func() { cancel(); stop(); t.opMu.Unlock(); t.producers.Done() }, nil
}

func (t *transport) SendText(ctx context.Context, chat, text, id string) (history.IncomingEvent, error) {
	if text == "" || len(text) > 65536 || id == "" {
		return history.IncomingEvent{}, errors.New("text and message ID are required; text limit is 64 KiB")
	}
	jid, err := destination(chat)
	if err != nil {
		return history.IncomingEvent{}, err
	}
	client, ctx, done, err := t.sendClient(ctx)
	if err != nil {
		return history.IncomingEvent{}, err
	}
	defer done()
	return t.send(ctx, client, jid, &waE2E.Message{Conversation: proto.String(text)}, text, id)
}

func (t *transport) SendReply(ctx context.Context, chat, text, id string, target history.Message) (history.IncomingEvent, error) {
	if text == "" || len(text) > 65536 || id == "" || target.ID == "" {
		return history.IncomingEvent{}, errors.New("text, message ID, and reply target are required")
	}
	jid, err := destination(chat)
	if err != nil {
		return history.IncomingEvent{}, err
	}
	client, ctx, done, err := t.sendClient(ctx)
	if err != nil {
		return history.IncomingEvent{}, err
	}
	defer done()
	contextInfo := &waE2E.ContextInfo{
		StanzaID:      proto.String(target.ID),
		RemoteJID:     proto.String(chat),
		QuotedMessage: &waE2E.Message{Conversation: proto.String(target.Body)},
	}
	if strings.HasSuffix(chat, "@g.us") {
		contextInfo.Participant = proto.String(target.Sender)
	}
	message := &waE2E.Message{ExtendedTextMessage: &waE2E.ExtendedTextMessage{
		Text: proto.String(text), ContextInfo: contextInfo,
	}}
	result, err := t.send(ctx, client, jid, message, text, id)
	if err != nil {
		return history.IncomingEvent{}, err
	}
	result.ReplyToID, result.ReplySender, result.ReplyBody = target.ID, target.Sender, target.Body
	return result, nil
}

func (t *transport) SendReaction(ctx context.Context, chat string, target history.Message, emoji, id string) (history.IncomingEvent, error) {
	if target.ID == "" || emoji == "" || len([]rune(emoji)) > 8 || id == "" {
		return history.IncomingEvent{}, errors.New("reaction, message ID, and target are required")
	}
	jid, err := destination(chat)
	if err != nil {
		return history.IncomingEvent{}, err
	}
	client, ctx, done, err := t.sendClient(ctx)
	if err != nil {
		return history.IncomingEvent{}, err
	}
	defer done()
	key := &waCommon.MessageKey{
		RemoteJID: proto.String(jid.String()),
		FromMe:    proto.Bool(target.FromMe),
		ID:        proto.String(target.ID),
	}
	if strings.HasSuffix(chat, "@g.us") {
		key.Participant = proto.String(target.Sender)
	}
	resp, err := client.SendMessage(ctx, jid, &waE2E.Message{ReactionMessage: &waE2E.ReactionMessage{
		Key: key, Text: proto.String(emoji), SenderTimestampMS: proto.Int64(time.Now().UnixMilli()),
	}}, whatsmeow.SendRequestExtra{ID: id})
	if err != nil {
		return history.IncomingEvent{}, errors.New("reaction was not acknowledged; check WhatsApp before retrying")
	}
	account, _ := t.sessionState()
	return history.IncomingEvent{
		Kind: history.ReactionEvent, AccountID: account, ChatID: jid.String(),
		MessageID: id, EventID: "reaction:" + id, Sender: account, Timestamp: resp.Timestamp,
		FromMe: true, ReactionToID: target.ID, Reaction: emoji,
	}, nil
}

func (t *transport) SendStatus(ctx context.Context, text, id string) (history.StatusUpdate, error) {
	if strings.TrimSpace(text) == "" || len([]rune(text)) > 700 || id == "" {
		return history.StatusUpdate{}, errors.New("status text must contain 1–700 characters")
	}
	client, ctx, done, err := t.sendClient(ctx)
	if err != nil {
		return history.StatusUpdate{}, err
	}
	defer done()
	resp, err := client.SendMessage(ctx, types.StatusBroadcastJID,
		&waE2E.Message{Conversation: proto.String(text)},
		whatsmeow.SendRequestExtra{ID: id})
	if err != nil {
		return history.StatusUpdate{}, errors.New("status was not acknowledged; check WhatsApp before retrying")
	}
	account, _ := t.sessionState()
	return history.StatusUpdate{AccountID: account, ID: id, Sender: account, Name: "You", Body: text, CreatedAt: resp.Timestamp}, nil
}

func (t *transport) send(ctx context.Context, client *whatsmeow.Client, jid types.JID, message *waE2E.Message, body, id string) (history.IncomingEvent, error) {
	account, _ := t.sessionState()
	resp, err := client.SendMessage(ctx, jid, message, whatsmeow.SendRequestExtra{ID: id})
	if err != nil {
		return history.IncomingEvent{}, errors.New("send was not acknowledged; delivery is uncertain, do not automatically resend")
	}
	return history.IncomingEvent{
		Kind: history.MessageEvent, AccountID: account, ChatID: jid.String(),
		MessageID: id, EventID: "message:" + id, Sender: account, Body: body,
		Status: "sent", Timestamp: resp.Timestamp, FromMe: true,
	}, nil
}

func (t *transport) handle(client *whatsmeow.Client, event any) {
	// Pairing and logout mutate Store.ID upstream. Lifecycle callbacks use the
	// cached identity, including after upstream has deleted the device record.
	account, _ := t.sessionState()
	switch e := event.(type) {
	case *events.PairSuccess:
		if !e.ID.IsEmpty() {
			t.setAccount(e.ID.ToNonAD().String())
		}
	case *events.Connected:
		t.emit(Event{Type: Connected, AccountID: account})
	case *events.Disconnected:
		t.emit(Event{Type: Disconnected, AccountID: account})
	case *events.LoggedOut:
		t.markLoggedOut()
		t.emit(Event{Type: LoggedOut, AccountID: account})
	case *events.PairError:
		t.emit(Event{Type: Failure, Err: errors.New("could not save paired device session")})
	case events.PermanentDisconnect:
		t.emit(Event{Type: Disconnected, AccountID: account})
		t.emit(Event{Type: Failure, AccountID: account, Err: errors.New("WhatsApp stopped the connection; inspect linked devices and client compatibility before retrying")})
	case *events.Message:
		if status, ok := normalizeStatus(account, e); ok {
			t.emit(Event{Type: Incoming, AccountID: account, Status: &status})
		} else {
			t.message(account, e, "")
		}
	case *events.HistorySync:
		for _, conversation := range e.Data.GetConversations() {
			jid, err := destination(conversation.GetID())
			if err != nil {
				continue
			} // Status, broadcast and newsletter history is out of scope.
			for _, item := range conversation.GetMessages() {
				if item.GetMessage() == nil {
					continue
				}
				message, err := client.ParseWebMessage(jid, item.GetMessage())
				if err != nil {
					t.emit(Event{Type: Failure, Err: errors.New("a history message could not be decoded")})
					continue
				}
				t.message(account, message, conversation.GetName())
			}
		}
	case *events.Receipt:
		if e.Type != types.ReceiptTypeDelivered && e.Type != types.ReceiptTypeRead && e.Type != types.ReceiptTypePlayed {
			return
		}
		status := "delivered"
		if e.Type == types.ReceiptTypeRead || e.Type == types.ReceiptTypePlayed {
			status = "read"
		}
		for _, id := range e.MessageIDs {
			message := history.IncomingEvent{Kind: history.DeliveryEvent, AccountID: account, ChatID: e.Chat.ToNonAD().String(), MessageID: id, EventID: "receipt:" + string(e.Type) + ":" + id, Timestamp: e.Timestamp, Status: status, FromMe: true}
			t.emit(Event{Type: Incoming, AccountID: account, Message: &message})
		}
	}
}

func (t *transport) message(account string, source *events.Message, title string) {
	message, ok := normalizeMessage(account, source, title)
	if ok {
		t.emit(Event{Type: Incoming, AccountID: account, Message: &message})
	}
}
