package app

import (
	"context"
	"fmt"
	"time"

	"whatscli/internal/config"
	"whatscli/internal/history"
	"whatscli/internal/ui"
)

type demoChat struct {
	id, title, sender, body string
	at                      string // clock time today
	fromMe                  bool
	status                  string
	unread                  int
}

var demoChats = []demoChat{
	{"demo-umbrella@g.us", "Umbrella Devs", "Joshua", "All good on my side. UI looks clean now.", "22:14", false, "", 3},
	{"27820000001@s.whatsapp.net", "Joshua", "me", "Lemme test it", "21:47", true, "read", 0},
	{"demo-merensky@g.us", "Merensky 9K", "Thabo", "[document: Notes.pdf]", "19:32", false, "", 5},
	{"27820000002@s.whatsapp.net", "Mom", "me", "Are you home?", "18:09", true, "read", 0},
	{"demo-tritech@g.us", "TRITECH Team", "Lerato", "- Project update tomorrow", "16:21", false, "", 0},
	{"27820000003@s.whatsapp.net", "Qhawe (Secondary)", "Qhawe", "[image; not downloaded]", "14:08", false, "", 0},
	{"27820000004@s.whatsapp.net", "Naledi", "Naledi", "See you at the standup", "12:30", false, "", 1},
	{"demo-design@g.us", "Design Review", "Sipho", "Mockups are in the shared folder", "11:02", false, "", 1},
	{"27820000005@s.whatsapp.net", "Dad", "Dad", "Call me when you can", "09:45", false, "", 2},
	{"27820000006@s.whatsapp.net", "Zanele", "me", "Thanks!", "08:20", true, "delivered", 0},
	{"demo-family@g.us", "Family", "Gogo", "Sunday lunch at 1", "07:10", false, "", 0},
	{"27820000007@s.whatsapp.net", "Mpho", "Mpho", "Sent you the invoice", "06:30", false, "", 0},
}

func demoTime(clock string) time.Time {
	now := time.Now()
	t, _ := time.ParseInLocation("15:04", clock, time.Local)
	return time.Date(now.Year(), now.Month(), now.Day(), t.Hour(), t.Minute(), 0, 0, time.Local).UTC()
}

// SeedDemo is only called for -demo's disposable database, never user history.
func SeedDemo(ctx context.Context, repo history.Repository) error {
	for i, ch := range demoChats {
		if i == 0 {
			continue // the Umbrella Devs thread below supplies its messages
		}
		err := repo.ApplyEvent(ctx, history.IncomingEvent{
			Kind: history.MessageEvent, AccountID: "local", ChatID: ch.id, Title: ch.title,
			MessageID: fmt.Sprintf("demo-%d", i), Sender: ch.sender, Body: ch.body,
			Timestamp: demoTime(ch.at), FromMe: ch.fromMe, Status: ch.status,
		})
		if err != nil {
			return err
		}
	}
	if err := repo.SaveConversation(ctx, history.Conversation{AccountID: "local", ID: demoChats[0].id, Title: demoChats[0].title, UpdatedAt: demoTime(demoChats[0].at)}); err != nil {
		return err
	}
	thread := []struct {
		sender, body, at string
		me               bool
	}{
		{"Joshua", "Pushed the fix for the QR flow.", "21:58", false},
		{"Lethabo", "Nice, build is green now ✅", "22:02", false},
		{"You", "Bet, I'll test it now.", "22:07", true},
		{"Joshua", "All good on my side. UI looks clean now.", "22:14", false},
		{"You", "Perfect 👌", "22:14", true},
	}
	for i, t := range thread {
		status := ""
		if t.me {
			status = "read"
		}
		if err := repo.ApplyEvent(ctx, history.IncomingEvent{
			Kind: history.MessageEvent, AccountID: "local", ChatID: demoChats[0].id, Title: demoChats[0].title,
			MessageID: fmt.Sprintf("demo-thread-%d", i), Sender: t.sender, Body: t.body,
			Timestamp: demoTime(t.at).Add(time.Duration(i) * time.Second), FromMe: t.me, Status: status,
		}); err != nil {
			return err
		}
	}
	return nil
}

// demoPhoto is a synthetic profile picture so the photo path is exercised
// without any network access.
func demoPhoto() *ui.Photo {
	var px [6][8]ui.Color
	for y := 0; y < 6; y++ {
		for x := 0; x < 8; x++ {
			v := uint32(0x1A + y*7 + x*2)
			px[y][x] = ui.Hex(v<<16 | (v+6)<<8 | (v + 14))
		}
	}
	for x := 2; x < 6; x++ {
		px[3][x] = ui.Hex(0xB9C3CC)
		px[4][x] = ui.Hex(0x090B0E)
	}
	return ui.PhotoFromColors(px)
}

func DemoModel(runtime *Runtime, settings config.Settings, settingsPath string) Model {
	m := NewWithRuntime(runtime, settings, settingsPath)
	m.demo = true
	m.demoSubtitle = map[string]string{demoChats[0].id: "6 members"}
	m.active, m.chatID, m.chatTitle = chatScreen, demoChats[0].id, demoChats[0].title
	m.conversations, _ = m.repo.Conversations(m.ctx, m.account, nil, 50)
	m.messages, _ = m.repo.Messages(m.ctx, m.account, m.chatID, nil, 50)
	for _, ch := range demoChats {
		if ch.unread > 0 {
			m.unread[ch.id] = ch.unread
		}
	}
	m.avatars[m.avatarKey(demoChats[1].id)] = &avatarEntry{photo: demoPhoto(), state: avDone}
	m.lastUnread = 12
	m.fitList()
	return m
}
