package app

import (
	"context"
	"fmt"
	"time"

	"whatscli/internal/config"
	"whatscli/internal/history"
)

// SeedDemo is only called for -demo's disposable database, never user history.
func SeedDemo(ctx context.Context, repo history.Repository) error {
	now := time.Now().Truncate(time.Minute)
	chats := []struct{ id, title, body string }{
		{"demo-umbrella@g.us", "Umbrella Devs", "All good on my side. UI looks clean now."},
		{"27820000001@s.whatsapp.net", "Joshua", "Lemme test it"},
		{"demo-merensky@g.us", "Merensky 9K", "[document: Notes.pdf]"},
		{"27820000002@s.whatsapp.net", "Mom", "Are you home?"},
		{"demo-tritech@g.us", "TRITECH Team", "Project update tomorrow"},
		{"27820000003@s.whatsapp.net", "Qhawe (Secondary)", "[image; not downloaded]"},
	}
	for i, chat := range chats {
		err := repo.ApplyEvent(ctx, history.IncomingEvent{
			Kind: history.MessageEvent, AccountID: "local", ChatID: chat.id, Title: chat.title,
			MessageID: fmt.Sprintf("demo-%d", i), Sender: chat.title, Body: chat.body,
			Timestamp: now.Add(-time.Duration(i+1) * time.Hour),
		})
		if err != nil {
			return err
		}
	}
	messages := []struct {
		sender, body string
		me           bool
	}{
		{"Joshua", "Pushed the fix for the QR flow.", false},
		{"Lethabo", "Nice, build is green now.", false},
		{"You", "Bet, I'll test it now.", true},
		{"Joshua", "All good on my side. UI looks clean now.", false},
		{"You", "Perfect. Keyboard-first, as it should be.", true},
	}
	for i, message := range messages {
		status := ""
		if message.me {
			status = "read"
		}
		if err := repo.ApplyEvent(ctx, history.IncomingEvent{
			Kind: history.MessageEvent, AccountID: "local", ChatID: chats[0].id, Title: chats[0].title,
			MessageID: fmt.Sprintf("demo-thread-%d", i), Sender: message.sender, Body: message.body,
			Timestamp: now.Add(-time.Duration(len(messages)-i) * time.Minute), FromMe: message.me, Status: status,
		}); err != nil {
			return err
		}
	}
	return nil
}

func DemoModel(runtime *Runtime, settings config.Settings, settingsPath string) Model {
	m := NewWithRuntime(runtime, settings, settingsPath)
	m.demo = true
	m.active, m.chatID, m.chatTitle = chatScreen, "demo-umbrella@g.us", "Umbrella Devs"
	m.conversations, _ = m.repo.Conversations(m.ctx, m.account, nil, 50)
	m.messages, _ = m.repo.Messages(m.ctx, m.account, m.chatID, nil, 50)
	return m
}
