package app

import (
	"context"
	"errors"
	"sync"
	"time"

	"whatscli/internal/history"
	"whatscli/internal/messaging"
)

type Connection struct {
	AccountID   string
	Status      string
	QR          string
	Error       string
	Online      bool
	StoreFailed bool
}

// Runtime persists all admitted events independently of terminal redraw speed.
// Wakeups may coalesce, but message history and the latest status never do.
type Runtime struct {
	Repo       history.Repository
	Client     messaging.Client
	ctx        context.Context
	cancel     context.CancelFunc
	mu         sync.Mutex
	state      Connection
	consent    bool
	connecting bool
	closing    bool
	wake       chan struct{}
	done       chan struct{}
	ops        sync.WaitGroup
	closeOnce  sync.Once
	closeErr   error
}

func NewRuntime(repo history.Repository, client messaging.Client, consent bool) *Runtime {
	ctx, cancel := context.WithCancel(context.Background())
	r := &Runtime{Repo: repo, Client: client, ctx: ctx, cancel: cancel, consent: consent, wake: make(chan struct{}, 1), done: make(chan struct{}), state: Connection{Status: "Offline"}}
	go r.ingest()
	return r
}

func (r *Runtime) Snapshot() Connection  { r.mu.Lock(); defer r.mu.Unlock(); return r.state }
func (r *Runtime) Wake() <-chan struct{} { return r.wake }
func (r *Runtime) AllowConnection()      { r.mu.Lock(); r.consent = true; r.mu.Unlock() }
func (r *Runtime) notify() {
	select {
	case r.wake <- struct{}{}:
	default:
	}
}

func (r *Runtime) storageFailure(message string) error {
	r.mu.Lock()
	r.state.Error = message
	r.state.StoreFailed = true
	r.mu.Unlock()
	r.notify()
	return errors.New(message)
}

func (r *Runtime) Connect() error {
	r.mu.Lock()
	if !r.consent || r.closing {
		r.mu.Unlock()
		return errors.New("connection requires accepted consent and an active application")
	}
	if r.connecting || r.state.Online {
		r.mu.Unlock()
		return nil
	}
	r.ops.Add(1)
	defer r.ops.Done()
	r.connecting = true
	r.state.Status = "Connecting"
	if !r.state.StoreFailed {
		r.state.Error = ""
	}
	r.mu.Unlock()
	r.notify()
	err := r.Client.Connect(r.ctx)
	if err != nil {
		r.mu.Lock()
		r.connecting = false
		r.state.Status = "Connection failed"
		r.mu.Unlock()
		r.notify()
	}
	return err
}

func (r *Runtime) ingest() {
	defer close(r.done)
	for event := range r.Client.Events() {
		if event.Type == messaging.Incoming && (event.Message != nil || event.Status != nil) {
			var err error
			if event.Message != nil {
				err = r.Repo.ApplyEvent(r.ctx, *event.Message)
			} else {
				err = r.Repo.SaveStatus(r.ctx, *event.Status)
			}
			if err != nil {
				r.mu.Lock()
				r.state.Error = "Could not save incoming data. Stop and check local storage before continuing."
				r.state.StoreFailed = true
				r.mu.Unlock()
			}
		}
		r.mu.Lock()
		if event.AccountID != "" {
			r.state.AccountID = event.AccountID
		}
		switch event.Type {
		case messaging.QR:
			r.state.QR = event.Code
			r.state.Status = "Scan with WhatsApp → Linked devices"
		case messaging.Connecting:
			r.state.Status = "Connecting"
		case messaging.Connected:
			r.state.Online = true
			r.connecting = false
			r.state.QR = ""
			r.state.Status = "Connected"
			if !r.state.StoreFailed {
				r.state.Error = ""
			}
		case messaging.Disconnected:
			r.state.Online = false
			r.connecting = false
			r.state.QR = ""
			r.state.Status = "Disconnected; reconnecting when possible"
		case messaging.LoggedOut:
			r.state.Online = false
			r.connecting = false
			r.state.QR = ""
			r.state.Status = "Device unlinked. Restart to pair again."
		case messaging.PairingExpired:
			r.connecting = false
			r.state.QR = ""
			r.state.Status = "Pairing expired. Press r to retry."
		case messaging.Failure:
			r.connecting = false
			r.state.QR = ""
			r.state.Error = "WhatsApp operation failed; retry or restart after checking connectivity."
		}
		r.mu.Unlock()
		r.notify()
	}
}

func (r *Runtime) Send(ctx context.Context, chat, text, path string) error {
	return r.send(ctx, chat, text, path, nil)
}

func (r *Runtime) SendReply(ctx context.Context, chat, text string, target history.Message) error {
	return r.send(ctx, chat, text, "", &target)
}

func (r *Runtime) React(ctx context.Context, chat string, target history.Message, emoji string) error {
	r.mu.Lock()
	if r.closing || !r.consent {
		r.mu.Unlock()
		return errors.New("application is closing or consent is missing")
	}
	r.ops.Add(1)
	r.mu.Unlock()
	defer r.ops.Done()
	state := r.Snapshot()
	if !state.Online || state.AccountID == "" || state.StoreFailed {
		return errors.New("reaction unavailable: connect first and resolve any storage errors")
	}
	client, ok := r.Client.(messaging.ReactionClient)
	if !ok {
		return errors.New("reactions are unavailable in this transport")
	}
	event, err := client.SendReaction(ctx, chat, target, emoji, r.Client.NewMessageID())
	if err != nil {
		return err
	}
	if err := r.Repo.ApplyEvent(ctx, event); err != nil {
		return r.storageFailure("reaction was sent, but local history could not be updated")
	}
	r.notify()
	return nil
}

func (r *Runtime) send(ctx context.Context, chat, text, path string, reply *history.Message) error {
	r.mu.Lock()
	if r.closing || !r.consent {
		r.mu.Unlock()
		return errors.New("application is closing or consent is missing")
	}
	r.ops.Add(1)
	r.mu.Unlock()
	defer r.ops.Done()
	state := r.Snapshot()
	if !state.Online || state.AccountID == "" || state.StoreFailed {
		return errors.New("send unavailable: connect first and resolve any storage errors")
	}
	if reply != nil {
		if _, ok := r.Client.(messaging.ReplyClient); !ok {
			return errors.New("reply sending is unavailable in this transport")
		}
	}
	id := r.Client.NewMessageID()
	pending := history.IncomingEvent{Kind: history.MessageEvent, AccountID: state.AccountID, ChatID: chat, MessageID: id, EventID: "outgoing:" + id, Sender: "me", Body: text, Status: "pending", Timestamp: time.Now().UTC(), FromMe: true}
	if reply != nil {
		pending.ReplyToID, pending.ReplySender, pending.ReplyBody = reply.ID, reply.Sender, reply.Body
	}
	if err := r.Repo.ApplyEvent(ctx, pending); err != nil {
		return errors.New("could not save outgoing message; nothing sent")
	}
	r.notify()
	var result history.IncomingEvent
	var err error
	if reply != nil {
		result, err = r.Client.(messaging.ReplyClient).SendReply(ctx, chat, text, id, *reply)
	} else if path == "" {
		result, err = r.Client.SendText(ctx, chat, text, id)
	} else {
		result, err = r.Client.SendFile(ctx, chat, path, id)
	}
	status := "sent"
	if err != nil {
		status = "unknown"
	}
	// Persist the outcome even when the caller cancelled the network operation.
	persistCtx, cancel := context.WithTimeout(r.ctx, 5*time.Second)
	defer cancel()
	if err == nil {
		result.Status = status
		if saveErr := r.Repo.ApplyEvent(persistCtx, result); saveErr != nil {
			return errors.New("sent, but failed to save the server result; do not resend blindly")
		}
	}
	pending.Kind = history.DeliveryEvent
	pending.EventID = "outcome:" + id
	pending.Status = status
	pending.Timestamp = time.Now().UTC()
	if saveErr := r.Repo.ApplyEvent(persistCtx, pending); saveErr != nil {
		return errors.New("could not save delivery outcome; check WhatsApp before retrying")
	}
	r.notify()
	if err != nil {
		return errors.New("delivery not confirmed; check WhatsApp before retrying to avoid a duplicate")
	}
	return nil
}

func (r *Runtime) PostStatus(ctx context.Context, text string) error {
	r.mu.Lock()
	if r.closing || !r.consent {
		r.mu.Unlock()
		return errors.New("application is closing or consent is missing")
	}
	r.ops.Add(1)
	r.mu.Unlock()
	defer r.ops.Done()
	state := r.Snapshot()
	if !state.Online || state.AccountID == "" || state.StoreFailed {
		return errors.New("status posting unavailable: connect first and resolve any storage errors")
	}
	client, ok := r.Client.(messaging.StatusClient)
	if !ok {
		return errors.New("status posting is unavailable in this transport")
	}
	id := r.Client.NewMessageID()
	status, err := client.SendStatus(ctx, text, id)
	if err != nil {
		return err
	}
	if err := r.Repo.SaveStatus(ctx, status); err != nil {
		return r.storageFailure("status was sent, but its local history could not be saved")
	}
	r.notify()
	return nil
}

func (r *Runtime) Logout(ctx context.Context) error {
	r.mu.Lock()
	if r.closing || !r.consent {
		r.mu.Unlock()
		return errors.New("logout requires accepted consent and an active application")
	}
	r.ops.Add(1)
	r.mu.Unlock()
	defer r.ops.Done()
	return r.Client.Logout(ctx)
}

func (r *Runtime) Close() error {
	r.closeOnce.Do(func() {
		r.mu.Lock()
		r.closing = true
		r.mu.Unlock()
		// Events continue draining while the adapter waits for admitted producers.
		r.closeErr = r.Client.Close()
		<-r.done
		r.ops.Wait()
		r.cancel()
	})
	return r.closeErr
}

// Avatar fetches a contact's profile picture bytes. It never blocks the UI:
// callers invoke it from a command. nil bytes mean "no artwork" (use initials).
func (r *Runtime) Avatar(ctx context.Context, chat string) ([]byte, error) {
	provider, ok := r.Client.(messaging.AvatarProvider)
	state := r.Snapshot()
	r.mu.Lock()
	allowed := r.consent && !r.closing
	r.mu.Unlock()
	if !ok || !allowed || !state.Online {
		return nil, nil
	}
	return provider.ProfilePicture(ctx, chat)
}

// CanFetchAvatars reports whether the transport can supply profile pictures now.
func (r *Runtime) CanFetchAvatars() bool {
	if _, ok := r.Client.(messaging.AvatarProvider); !ok {
		return false
	}
	return r.Snapshot().Online
}
