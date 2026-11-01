package app

import (
	"context"
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"whatscli/internal/config"
	"whatscli/internal/history"
	"whatscli/internal/messaging"
)

func demoFixture(t *testing.T) Model {
	t.Helper()
	r, _ := testRuntime(t, true)
	if err := SeedDemo(context.Background(), r.Repo); err != nil {
		t.Fatal(err)
	}
	return DemoModel(r, config.Settings{ConsentVersion: config.ConsentVersion, Mouse: true}, "")
}

func TestWorkspaceFitsTerminal(t *testing.T) {
	m := demoFixture(t)
	m.messages[0].Body = strings.Repeat("long message 日本語 ", 200)
	m.conversations[0].Title = strings.Repeat("long title ", 40)
	for _, size := range [][2]int{{90, 20}, {100, 24}, {140, 45}, {200, 60}, {60, 14}, {30, 10}} {
		for _, active := range []screen{welcomeScreen, consentScreen, pairingScreen, inboxScreen, chatScreen, newChatScreen, settingsScreen, helpScreen} {
			m.width, m.height, m.active = size[0], size[1], active
			view := m.View()
			if lines := strings.Split(view, "\n"); len(lines) > m.height {
				t.Fatalf("%dx%d screen %d has %d lines", m.width, m.height, active, len(lines))
			} else {
				for _, line := range lines {
					if w := ansi.StringWidth(line); w > m.width {
						t.Fatalf("%dx%d screen %d has line width %d: %q", m.width, m.height, active, w, ansi.Strip(line))
					}
				}
			}
		}
	}
}

func TestWorkspaceSnapshot(t *testing.T) {
	m := demoFixture(t)
	m.width, m.height = 130, 42
	view := ansi.Strip(m.View())
	for _, text := range []string{"Chats", "Umbrella Devs", "Joshua", "Type a message", "DEMO"} {
		if !strings.Contains(view, text) {
			t.Fatalf("workspace missing %q", text)
		}
	}
	t.Log("\n" + view)
}

func TestSearchTreatsShortcutsAsInputAndKeepsDraft(t *testing.T) {
	m := demoFixture(t)
	m.composer = "private draft"
	m, _ = press(m, tea.KeyMsg{Type: tea.KeyCtrlF})
	if !m.searching || m.active != inboxScreen {
		t.Fatal("search not focused")
	}
	m, _ = press(m, runeKey("Joshua"))
	if got := m.visibleChats(); len(got) != 1 || m.conversations[got[0]].Title != "Joshua" {
		t.Fatal("incorrect filtered chat")
	}
	m, _ = press(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.chatTitle != "Umbrella Devs" || m.composer != "private draft" {
		t.Fatal("search moved a draft to another recipient")
	}
	m, _ = press(m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.searching || m.search != "" {
		t.Fatal("search did not clear")
	}
}

func TestNewChatIsLocalAndConsentGated(t *testing.T) {
	m := demoFixture(t)
	m, _ = press(m, runeKey("n"))
	if m.active != newChatScreen {
		t.Fatal("new chat did not open")
	}
	m, _ = press(m, runeKey("+27 82 123 4567"))
	m, cmd := press(m, tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("new chat not saved")
	}
	next, _ := m.Update(cmd())
	m = next.(Model)
	if m.chatID != "27821234567@s.whatsapp.net" || m.active != chatScreen {
		t.Fatal("new chat not selected")
	}
	m.settings.ConsentVersion = 0
	if cmd := m.openMetaAI(); cmd != nil {
		t.Fatal("Meta AI bypassed consent")
	}
}

func TestMetaAIUsesUpstreamIdentityAndProtectsDraft(t *testing.T) {
	m := demoFixture(t)
	m.composer = "do not redirect"
	if cmd := m.openMetaAI(); cmd != nil {
		t.Fatal("Meta AI ignored draft guard")
	}
	m.composer = ""
	cmd := m.openMetaAI()
	next, _ := m.Update(cmd())
	m = next.(Model)
	if m.chatID != messaging.MetaAIChatID() || m.chatTitle != "Meta AI" {
		t.Fatal("incorrect Meta AI identity")
	}
}

func TestMouseTargetsMatchRenderedCells(t *testing.T) {
	m := demoFixture(t)
	m.width, m.height = 130, 42
	rail, sidebar, _, height := m.geometry()
	for _, target := range []struct {
		x, y int
		want string
	}{{2, 6, "new"}, {2, 8, "ai"}, {2, height - 4, "settings"}, {rail + 2, 4, "search"}, {rail + sidebar + 8, height - 3, "compose"}} {
		m.mouse(tea.MouseMsg{X: target.x, Y: target.y, Action: tea.MouseActionMotion})
		if m.hover != target.want {
			t.Fatalf("cell %d,%d hit %q, want %q", target.x, target.y, m.hover, target.want)
		}
	}
	indices, _ := m.chatWindow(height - 2)
	m.mouse(tea.MouseMsg{X: rail + 3, Y: 10, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
	if m.chatID != m.conversations[indices[1]].ID {
		t.Fatal("click did not open second rendered chat")
	}
	m.settings.Mouse = false
	m.hover = ""
	m.mouse(tea.MouseMsg{X: 2, Y: 6, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
	if m.hover != "" || m.active == newChatScreen {
		t.Fatal("disabled mouse changed state")
	}
}

func TestDemoSendOnlyWritesSyntheticHistory(t *testing.T) {
	m := demoFixture(t)
	m.composer = "local demonstration"
	m.composing = true
	cmd := m.send(m.composer, "")
	if cmd == nil {
		t.Fatal("demo send unavailable")
	}
	next, _ := m.Update(cmd())
	m = next.(Model)
	messages, err := m.repo.Messages(context.Background(), "local", m.chatID, nil, 50)
	if err != nil || messages[0].Body != "local demonstration" || m.composer != "" {
		t.Fatal("demo message not saved", err)
	}
	if m.runtime.Snapshot().Online {
		t.Fatal("demo unexpectedly online")
	}
}

func TestCreatedChatRejectsStaleAccountAndDraft(t *testing.T) {
	m := demoFixture(t)
	original := m.chatID
	next, _ := m.Update(createdChatMsg{"different", "recipient", "Wrong", nil})
	if next.(Model).chatID != original {
		t.Fatal("stale create changed account")
	}
	m.composer = "draft"
	next, _ = m.Update(createdChatMsg{m.account, "recipient", "Wrong", nil})
	if next.(Model).chatID != original {
		t.Fatal("async create redirected draft")
	}
}

func TestFilteredNavigationAndTab(t *testing.T) {
	m := demoFixture(t)
	m.active = inboxScreen
	m.search = "Mom"
	m.moveChat(1)
	if m.conversations[m.chatIndex].Title != "Mom" {
		t.Fatal("navigation escaped filter")
	}
	m, _ = press(m, tea.KeyMsg{Type: tea.KeyTab})
	if m.active != chatScreen {
		t.Fatal("tab did not switch panes")
	}
}

func TestHistoryPreviewIncludesLatestBody(t *testing.T) {
	m := demoFixture(t)
	for _, c := range m.conversations {
		if c.LastBody == "" {
			t.Fatalf("missing latest message for %s", c.Title)
		}
	}
	e := history.IncomingEvent{Kind: history.DeleteEvent, AccountID: m.account, ChatID: m.chatID, MessageID: m.messages[0].ID, Timestamp: m.messages[0].CreatedAt}
	if err := m.repo.ApplyEvent(context.Background(), e); err != nil {
		t.Fatal(err)
	}
	chats, err := m.repo.Conversations(context.Background(), m.account, nil, 50)
	if err != nil || chats[0].LastBody != "[deleted; retained]" {
		t.Fatal(fmt.Sprint("deleted preview incorrect: ", chats, err))
	}
}
