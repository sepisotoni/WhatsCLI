package app

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"whatscli/internal/config"
	"whatscli/internal/history"
)

func press(m Model, key tea.KeyMsg) (Model, tea.Cmd) {
	next, cmd := m.Update(key)
	return next.(Model), cmd
}
func runeKey(s string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }

func TestConsentMustSaveBeforePairing(t *testing.T) {
	m := New(nil)
	m, _ = press(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.active != consentScreen {
		t.Fatal("welcome did not open consent")
	}
	m, cmd := press(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.active != consentScreen || cmd != nil {
		t.Fatal("continued without consent")
	}
	m, _ = press(m, tea.KeyMsg{Type: tea.KeySpace})
	m, cmd = press(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.active != consentScreen || cmd == nil {
		t.Fatal("pairing must wait for saved consent")
	}
	next, _ := m.Update(cmd())
	m = next.(Model)
	if !m.settings.Accepted() || m.active != pairingScreen {
		t.Fatal("saved consent did not enable pairing")
	}
}

func TestWelcomeMatchesReferenceAndOpensAgreement(t *testing.T) {
	m := New(nil)
	m.width, m.height = 100, 30
	view := plain(m)
	for _, text := range []string{"Welcome to WhatsCLI", ">_", "Your conversations. Your terminal.", "Continue", "Exit"} {
		if !strings.Contains(view, text) {
			t.Errorf("welcome screen missing %q", text)
		}
	}
	if strings.Contains(view, "✉") {
		t.Fatal("welcome wallpaper still contains envelope glyphs")
	}
	var exit, proceed *hit
	s := &scene{m: &m}
	m.draw(s)
	for i := range s.hits {
		switch s.hits[i].id {
		case "welcome:exit":
			exit = &s.hits[i]
		case "welcome:start":
			proceed = &s.hits[i]
		}
	}
	if exit == nil || proceed == nil || exit.r.W != proceed.r.W || exit.r.H != proceed.r.H || exit.r.X >= proceed.r.X {
		t.Fatalf("welcome buttons must be equal-sized with Exit on the left: exit=%+v continue=%+v", exit, proceed)
	}
	if strings.Contains(strings.Split(view, "\n")[0], "╭") || strings.Contains(view, "Get started") {
		t.Fatal("welcome should use the compact centered-card design")
	}
	m, _ = press(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.active != consentScreen {
		t.Fatal("Continue did not open the agreement screen")
	}
}

func TestWelcomeMouseButtonsRemainInteractive(t *testing.T) {
	m := New(nil)
	m.width, m.height = 100, 30
	if !m.settings.Mouse {
		t.Fatal("mouse input should be enabled for first-run onboarding")
	}
	var proceed *hit
	s := &scene{m: &m}
	m.draw(s)
	for i := range s.hits {
		if s.hits[i].id == "welcome:start" {
			proceed = &s.hits[i]
			break
		}
	}
	if proceed == nil {
		t.Fatal("welcome continue button has no mouse target")
	}
	next, _ := m.Update(tea.MouseMsg{
		X:      proceed.r.X + proceed.r.W/2,
		Y:      proceed.r.Y + proceed.r.H/2,
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonLeft,
	})
	m = next.(Model)
	if m.active != consentScreen {
		t.Fatal("clicking Continue did not open agreement")
	}
}

func TestWelcomeSupportsArrowAndSpaceNavigation(t *testing.T) {
	m := New(nil)
	m.width, m.height = 100, 30
	m, _ = press(m, tea.KeyMsg{Type: tea.KeySpace})
	if m.active != consentScreen {
		t.Fatal("Space did not activate the focused Continue button")
	}

	m = New(nil)
	m, _ = press(m, tea.KeyMsg{Type: tea.KeyDown})
	if m.welcomeFocus != 0 {
		t.Fatal("Down arrow did not move focus to Exit")
	}
	m, cmd := press(m, tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil || m.active != welcomeScreen {
		t.Fatal("Enter did not activate the focused Exit button")
	}
}

func TestPairingBackReturnsToConsentAndUnpairedWorkspaceStaysHidden(t *testing.T) {
	m := New(nil)
	m.active = pairingScreen
	m.settings.ConsentVersion = config.ConsentVersion
	if !strings.Contains(plain(m), "STEP 02 / 02") {
		t.Fatalf("pairing screen does not show step 2 of 2:\n%s", plain(m))
	}
	m, _ = press(m, runeKey("i"))
	if m.active != pairingScreen {
		t.Fatal("unpaired keyboard shortcut opened the WhatsCLI workspace")
	}
	var back *hit
	s := &scene{m: &m}
	m.draw(s)
	for i := range s.hits {
		if s.hits[i].id == "pair:back" {
			back = &s.hits[i]
			break
		}
	}
	if back == nil {
		t.Fatal("pairing screen is missing its Back button")
	}
	next, _ := m.Update(tea.MouseMsg{
		X:      back.r.X + back.r.W/2,
		Y:      back.r.Y + back.r.H/2,
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonLeft,
	})
	m = next.(Model)
	if m.active != consentScreen {
		t.Fatal("pairing Back did not return to agreement")
	}
}

func TestConsentScrollbarCanBeDragged(t *testing.T) {
	m := New(nil)
	m.active, m.width, m.height = consentScreen, 60, 16
	s := &scene{m: &m}
	m.draw(s)
	var bar *hit
	for i := range s.hits {
		if s.hits[i].id == "consent:scrollbar" {
			bar = &s.hits[i]
			break
		}
	}
	if bar == nil {
		t.Fatal("short agreement screen is missing its scrollbar")
	}
	next, _ := m.Update(tea.MouseMsg{
		X:      bar.r.X + bar.r.W/2,
		Y:      bar.r.Bottom() - 1,
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonLeft,
	})
	m = next.(Model)
	if m.scroll == 0 || !m.consentDragging {
		t.Fatal("scrollbar press did not begin dragging")
	}
	next, _ = m.Update(tea.MouseMsg{Y: bar.r.Y, Action: tea.MouseActionMotion})
	m = next.(Model)
	if m.scroll != 0 {
		t.Fatal("dragging scrollbar to the top did not reset scroll")
	}
	next, _ = m.Update(tea.MouseMsg{Action: tea.MouseActionRelease})
	m = next.(Model)
	if m.consentDragging {
		t.Fatal("scrollbar drag did not stop on release")
	}
}

func TestReturningUserFollowsWelcomeAgreementPairingFlow(t *testing.T) {
	r, client := testRuntime(t, true)
	m := NewWithRuntime(r, config.Settings{ConsentVersion: config.ConsentVersion}, "")
	if m.active != welcomeScreen || !m.onboardingPending {
		t.Fatal("returning user did not start at welcome")
	}
	_ = m.Init()
	if client.connects.Load() != 0 {
		t.Fatal("runtime connected before the user completed onboarding")
	}
	if m.connect() != nil {
		t.Fatal("connection command was available before fresh agreement acceptance")
	}
	for _, command := range m.filteredCommands() {
		if command == "connect" || command == "conversations" || command == "update" {
			t.Fatalf("onboarding command available before agreement: %s", command)
		}
	}
	m, _ = press(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.active != consentScreen || m.consentAccepted {
		t.Fatal("returning agreement must require a fresh manual acceptance")
	}
	m, cmd := press(m, tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil || m.consentAccepted {
		t.Fatal("agreement continued without a fresh acceptance")
	}
	m, _ = press(m, tea.KeyMsg{Type: tea.KeySpace})
	if !m.consentAccepted {
		t.Fatal("Space did not manually accept the agreement")
	}
	m, cmd = press(m, tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("agreement confirmation did not continue")
	}
	next, _ := m.Update(cmd())
	m = next.(Model)
	if m.active != pairingScreen || m.onboardingPending {
		t.Fatal("agreement did not advance to device pairing")
	}
}

func TestPaletteCannotBypassConsentAndRestoresScreen(t *testing.T) {
	m := New(nil)
	m, _ = press(m, runeKey("/"))
	for _, command := range m.filteredCommands() {
		if command == "connect" || command == "conversations" {
			t.Fatal("pre-consent command enabled", command)
		}
	}
	m, _ = press(m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.active != welcomeScreen {
		t.Fatal("palette did not restore originating screen")
	}
}

func TestPaletteFiltersUnicodeAndRunsSettings(t *testing.T) {
	m := New(nil)
	m.settings.ConsentVersion = config.ConsentVersion
	m.active = inboxScreen
	m, _ = press(m, runeKey("/"))
	m, _ = press(m, runeKey("😀"))
	m, _ = press(m, tea.KeyMsg{Type: tea.KeyBackspace})
	if m.filter != "" || !utf8.ValidString(m.filter) {
		t.Fatal("backspace corrupted Unicode")
	}
	m, _ = press(m, runeKey("settings"))
	m, _ = press(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.active != settingsScreen {
		t.Fatal("command did not run")
	}
	m, _ = press(m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.active != inboxScreen {
		t.Fatal("settings return screen lost")
	}
}

func TestUpdateCommandIsOnDemandAndDisabledInDemo(t *testing.T) {
	m := demoFixture(t)
	m.composer, m.composing = "/update", true
	cmd := m.runSlash()
	if cmd != nil || m.active != updateScreen || m.updateNotice != "Update checks are disabled in demo mode." {
		t.Fatalf("unexpected demo update behavior: active=%v notice=%q cmd=%v", m.active, m.updateNotice, cmd != nil)
	}
	m, _ = press(m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.active != chatScreen && m.active != inboxScreen {
		t.Fatalf("update overlay did not close: %v", m.active)
	}
}

func TestUpdateCannotSkipAgreement(t *testing.T) {
	m := New(nil)
	m.active = welcomeScreen
	if cmd := m.openUpdate(); cmd != nil {
		t.Fatal("update started before agreement")
	}
	if m.active != welcomeScreen || !m.noticeErr {
		t.Fatal("update action bypassed the welcome/agreement flow")
	}
}

func TestComposerDoesNotTreatTextAsShortcuts(t *testing.T) {
	m := New(nil)
	m.active = chatScreen
	m.composing = true
	for _, text := range []string{"q", "/", "s", "😀"} {
		var cmd tea.Cmd
		m, cmd = press(m, runeKey(text))
		if cmd != nil {
			t.Fatal("typing executed a shortcut")
		}
	}
	if m.composer != "q/s😀" || !m.composing {
		t.Fatal("composer lost input", m.composer)
	}
	m, _ = press(m, tea.KeyMsg{Type: tea.KeyBackspace})
	if m.composer != "q/s" {
		t.Fatal("emoji deletion failed")
	}
	m, cmd := press(m, tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd != nil || m.notice == "" {
		t.Fatal("first Ctrl+C must request confirmation rather than quit")
	}
	m.now = time.Now()
	m.ctrlCNoticeUntil = m.now.Add(time.Second)
	next, _ := m.Update(pulseMsg(m.ctrlCNoticeUntil))
	m = next.(Model)
	if m.notice != "" {
		t.Fatal("Ctrl+C prompt did not disappear after three seconds")
	}
	m.ctrlCAt = time.Now()
	m, cmd = press(m, tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Fatal("second Ctrl+C must quit")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("not a quit command")
	}
}

func TestSpaceKeyIsTextInComposerAndFilePath(t *testing.T) {
	space := tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}}
	m := New(nil)
	m.active, m.composing, m.composer = chatScreen, true, "hello"
	m, _ = press(m, space)
	m, _ = press(m, runeKey("world"))
	if m.composer != "hello world" {
		t.Fatal("composer dropped space key", m.composer)
	}
	m.active, m.composing, m.path = attachmentScreen, false, "/host/file"
	m, _ = press(m, space)
	m, _ = press(m, runeKey("name.txt"))
	if m.path != "/host/file name.txt" {
		t.Fatal("file input dropped space key", m.path)
	}
}

func TestPagesIgnoreStaleResults(t *testing.T) {
	m := New(nil)
	m.account, m.chatID = "account", "chat"
	cursor := &history.Cursor{Time: time.Unix(10, 0), ID: "boundary"}
	m.chatBefore, m.messageBefore = cursor, cursor
	m.conversations = []history.Conversation{{ID: "old-chat"}}
	m.messages = []history.Message{{ID: "old-message"}}
	for _, msg := range []tea.Msg{
		chatsMsg{account: m.account, chats: []history.Conversation{{ID: "stale-chat"}}},
		messagesMsg{account: m.account, chat: m.chatID, messages: []history.Message{{ID: "stale-message"}}},
		messagesMsg{before: cursor, account: m.account, chat: "another-chat"},
	} {
		next, _ := m.Update(msg)
		m = next.(Model)
	}
	if m.conversations[0].ID != "old-chat" || m.messages[0].ID != "old-message" {
		t.Fatal("stale response overwrote the selected page")
	}
	copy := &history.Cursor{Time: cursor.Time.In(time.FixedZone("fixture", 3600)), ID: cursor.ID}
	next, _ := m.Update(messagesMsg{before: copy, account: m.account, chat: m.chatID, messages: []history.Message{{ID: "fresh"}}})
	if next.(Model).messages[0].ID != "fresh" {
		t.Fatal("equivalent cursor rejected")
	}
	m.active = chatScreen
	m, _ = press(m, runeKey("r"))
	if m.messageBefore != nil {
		t.Fatal("newest messages did not reset cursor")
	}
	m.active = inboxScreen
	m, _ = press(m, runeKey("r"))
	if m.chatBefore != nil {
		t.Fatal("newest conversations did not reset cursor")
	}
}

func TestLiveRefreshKeepsOlderPage(t *testing.T) {
	r, _ := testRuntime(t, true)
	m := NewWithRuntime(r, config.Settings{ConsentVersion: config.ConsentVersion}, "")
	m.chatID = "chat"
	cursor := &history.Cursor{Time: time.Unix(10, 0), ID: "boundary"}
	m.chatBefore, m.messageBefore = cursor, cursor
	for i := int64(1); i <= 20; i++ {
		e := history.IncomingEvent{Kind: history.MessageEvent, AccountID: "local", ChatID: "chat", MessageID: time.Unix(i, 0).Format(time.RFC3339), Sender: "peer", Body: "fixture", Timestamp: time.Unix(i, 0)}
		if err := r.Repo.ApplyEvent(context.Background(), e); err != nil {
			t.Fatal(err)
		}
	}
	next, _ := m.Update(refreshMsg{})
	m = next.(Model)
	if !sameCursor(m.chatBefore, cursor) || !sameCursor(m.messageBefore, cursor) {
		t.Fatal("live refresh jumped to newest page")
	}
	result := m.loadMessages(m.messageBefore)().(messagesMsg)
	if result.err != nil || len(result.messages) == 0 || !sameCursor(result.before, cursor) {
		t.Fatal("older page query failed", result.err)
	}
	for _, message := range result.messages {
		if message.CreatedAt.After(cursor.Time) {
			t.Fatal("newer message leaked into older page")
		}
	}
	next, _ = m.Update(sentMsg{account: m.account, chat: m.chatID})
	if !sameCursor(next.(Model).messageBefore, cursor) {
		t.Fatal("send outcome reset older page")
	}
}

func TestNestedOverlaysRestoreTheirOrigin(t *testing.T) {
	m := New(nil)
	m.active = inboxScreen
	m, _ = press(m, runeKey("s"))
	m, _ = press(m, runeKey("/"))
	m, _ = press(m, runeKey("help"))
	m, _ = press(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.active != helpScreen {
		t.Fatal("help command failed")
	}
	m, _ = press(m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.active != settingsScreen {
		t.Fatal("help did not restore settings")
	}
	m, _ = press(m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.active != inboxScreen {
		t.Fatal("settings trapped navigation")
	}
}

func TestDraftCannotMoveToAnotherRecipient(t *testing.T) {
	m := New(nil)
	m.active, m.chatID, m.composer = inboxScreen, "first", "private draft"
	m.conversations = []history.Conversation{{ID: "second", Title: "Second"}}
	m, _ = press(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.chatID != "first" || m.composer != "private draft" {
		t.Fatal("draft moved to another recipient")
	}
	m.active, m.composing = chatScreen, true
	m, _ = press(m, tea.KeyMsg{Type: tea.KeyCtrlU})
	m, _ = press(m, tea.KeyMsg{Type: tea.KeyEsc})
	m, _ = press(m, tea.KeyMsg{Type: tea.KeyEsc})
	m, _ = press(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.chatID != "second" || m.composer != "" {
		t.Fatal("could not change chats after clearing draft")
	}
}

func TestFailedConsentSaveDoesNotAllowConnection(t *testing.T) {
	r, client := testRuntime(t, false)
	m := NewWithRuntime(r, config.Settings{}, "")
	m.active = consentScreen
	next, _ := m.Update(savedMsg{settings: config.Settings{ConsentVersion: config.ConsentVersion}, consent: true, err: errors.New("fixture")})
	if next.(Model).settings.Accepted() || r.Connect() == nil || client.connects.Load() != 0 {
		t.Fatal("failed settings save enabled network access")
	}
}

func TestSmallTerminalAndScrollableConsent(t *testing.T) {
	m := New(nil)
	m.active = consentScreen
	next, _ := m.Update(tea.WindowSizeMsg{Width: 60, Height: 14})
	m = next.(Model)
	view := m.View()
	if len(strings.Split(view, "\n")) > 14 {
		t.Fatal("view exceeds terminal height")
	}
	m, _ = press(m, tea.KeyMsg{Type: tea.KeyPgDown})
	if m.scroll == 0 {
		t.Fatal("content cannot scroll")
	}
	next, _ = m.Update(tea.WindowSizeMsg{Width: 20, Height: 8})
	m = next.(Model)
	if !strings.Contains(m.View(), "Enlarge") {
		t.Fatal("missing minimum-size guidance")
	}
}
