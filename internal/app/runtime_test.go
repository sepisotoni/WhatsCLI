package app

import (
	"context"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"whatscli/internal/history"
	"whatscli/internal/messaging"
)

type fakeClient struct {
	events   chan messaging.Event
	connects atomic.Int32
	once     sync.Once
}

func newFakeClient() *fakeClient { return &fakeClient{events: make(chan messaging.Event, 64)} }
func (c *fakeClient) Connect(context.Context) error {
	c.connects.Add(1)
	c.events <- messaging.Event{Type: messaging.Connected, AccountID: "account"}
	return nil
}
func (c *fakeClient) Close() error { c.once.Do(func() { close(c.events) }); return nil }
func (c *fakeClient) Logout(context.Context) error {
	c.events <- messaging.Event{Type: messaging.LoggedOut, AccountID: "account"}
	return nil
}
func (c *fakeClient) Events() <-chan messaging.Event { return c.events }
func (c *fakeClient) NewMessageID() string           { return "outgoing-1" }
func (c *fakeClient) SendText(_ context.Context, chat, text, id string) (history.IncomingEvent, error) {
	e := history.IncomingEvent{Kind: history.MessageEvent, AccountID: "account", ChatID: chat, MessageID: id, EventID: "message:" + id, Sender: "me", Body: text, Timestamp: time.Now().UTC(), FromMe: true, Status: "sent"}
	c.events <- messaging.Event{Type: messaging.Incoming, AccountID: "account", Message: &e}
	return e, nil
}
func (c *fakeClient) SendFile(ctx context.Context, chat, path, id string) (history.IncomingEvent, error) {
	return c.SendText(ctx, chat, "[document]", id)
}

func testRuntime(t testing.TB, consent bool) (*Runtime, *fakeClient) {
	t.Helper()
	repo, err := history.Open(filepath.Join(t.TempDir(), "history.db"))
	if err != nil {
		t.Fatal(err)
	}
	client := newFakeClient()
	runtime := NewRuntime(repo, client, consent)
	t.Cleanup(func() { runtime.Close(); repo.Close() })
	return runtime, client
}

func waitOnline(t *testing.T, r *Runtime) {
	t.Helper()
	timer := time.NewTimer(3 * time.Second)
	defer timer.Stop()
	for !r.Snapshot().Online {
		select {
		case <-r.Wake():
		case <-timer.C:
			t.Fatal("connection did not become online")
		}
	}
}

func TestRuntimeConsentAndSendEcho(t *testing.T) {
	r, client := testRuntime(t, false)
	if err := r.Connect(); err == nil || client.connects.Load() != 0 {
		t.Fatal("connection bypassed consent")
	}
	r.AllowConnection()
	if err := r.Connect(); err != nil {
		t.Fatal(err)
	}
	waitOnline(t, r)
	if err := r.Send(context.Background(), "chat", "hello", ""); err != nil {
		t.Fatal(err)
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	messages, err := r.Repo.Messages(context.Background(), "account", "chat", nil, 20)
	if err != nil || len(messages) != 1 || messages[0].Status != "sent" {
		t.Fatal("pending/self echo reconciliation failed", messages, err)
	}
	revisions, err := r.Repo.Revisions(context.Background(), "account", "chat", "outgoing-1", 0, 20)
	if err != nil || len(revisions) != 1 {
		t.Fatal("self echo duplicated revision", revisions, err)
	}
}

func TestRuntimeDrainsBeforeClose(t *testing.T) {
	r, client := testRuntime(t, true)
	for i := 0; i < 30; i++ {
		e := history.IncomingEvent{Kind: history.MessageEvent, AccountID: "account", ChatID: "chat", MessageID: time.Unix(int64(i+1), 0).Format(time.RFC3339), Sender: "peer", Body: "fixture", Timestamp: time.Unix(int64(i+1), 0)}
		client.events <- messaging.Event{Type: messaging.Incoming, AccountID: "account", Message: &e}
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	messages, err := r.Repo.Messages(context.Background(), "account", "chat", nil, 50)
	if err != nil || len(messages) != 30 {
		t.Fatal("accepted events lost at shutdown", len(messages), err)
	}
	if err := r.Connect(); err == nil {
		t.Fatal("connected after close")
	}
}

func TestRuntimeLogoutRequiresConsentAndOpenRuntime(t *testing.T) {
	r, _ := testRuntime(t, false)
	if err := r.Logout(context.Background()); err == nil {
		t.Fatal("logout bypassed consent")
	}
	r.AllowConnection()
	if err := r.Logout(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	if err := r.Logout(context.Background()); err == nil {
		t.Fatal("logout admitted after close")
	}
}

func TestConnectionRecoveryClearsTransientErrorAndPairingMaterial(t *testing.T) {
	r, client := testRuntime(t, true)
	client.events <- messaging.Event{Type: messaging.QR, Code: "synthetic-pairing-fixture"}
	client.events <- messaging.Event{Type: messaging.Failure}
	client.events <- messaging.Event{Type: messaging.Connected, AccountID: "account"}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	state := r.Snapshot()
	if !state.Online || state.QR != "" || state.Error != "" {
		t.Fatal("connection recovery retained transient error or pairing material")
	}
}

func TestConnectionRecoveryDoesNotHideStorageFailure(t *testing.T) {
	r, client := testRuntime(t, true)
	bad := history.IncomingEvent{Kind: history.MessageEvent}
	client.events <- messaging.Event{Type: messaging.Incoming, Message: &bad}
	client.events <- messaging.Event{Type: messaging.Connected, AccountID: "account"}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	if !r.Snapshot().StoreFailed || r.Snapshot().Error == "" {
		t.Fatal("connection recovery hid storage failure")
	}
}

func TestIngestionFailureIsVisible(t *testing.T) {
	r, client := testRuntime(t, true)
	bad := history.IncomingEvent{Kind: history.MessageEvent}
	client.events <- messaging.Event{Type: messaging.Incoming, Message: &bad}
	r.Close()
	if !r.Snapshot().StoreFailed || r.Snapshot().Error == "" {
		t.Fatal("storage failure silently discarded")
	}
}
