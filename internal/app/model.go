package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"whatscli/internal/config"
	"whatscli/internal/history"
	"whatscli/internal/messaging"
	"whatscli/internal/ui"
)

type screen int

const (
	welcomeScreen screen = iota
	consentScreen
	pairingScreen
	inboxScreen
	chatScreen
	settingsScreen
	helpScreen
	commandScreen
	revisionsScreen
	attachmentScreen
	confirmFileScreen
	logoutScreen
	newChatScreen
)

type refreshMsg struct{}
type operationMsg struct{ err error }
type savedMsg struct {
	settings config.Settings
	err      error
	consent  bool
}
type chatsMsg struct {
	before  *history.Cursor
	account string
	chats   []history.Conversation
	err     error
}
type messagesMsg struct {
	before        *history.Cursor
	account, chat string
	messages      []history.Message
	err           error
}
type revisionsMsg struct {
	account, chat, message string
	revisions              []history.Revision
	err                    error
}
type fileMsg struct {
	path string
	file messaging.Attachment
	err  error
}
type sentMsg struct {
	account, chat string
	err           error
}
type createdChatMsg struct {
	account, chat, title string
	err                  error
}

type Model struct {
	repo                               history.Repository
	runtime                            *Runtime
	ctx                                context.Context
	settings                           config.Settings
	settingsPath                       string
	active                             screen
	trail                              []screen
	width, height                      int
	consentAccepted                    bool
	account                            string
	connection                         Connection
	chatBefore, messageBefore          *history.Cursor
	conversations                      []history.Conversation
	messages                           []history.Message
	revisions                          []history.Revision
	chatIndex, messageIndex            int
	chatID, chatTitle, revisionMessage string
	filter                             string
	commandIndex                       int
	composer                           string
	composing                          bool
	path                               string
	file                               messaging.Attachment
	sending, saving                    bool
	sendCancel                         context.CancelFunc
	notice                             string
	scroll                             int
	fullscreen                         bool
	search, newChat, hover             string
	searching, demo                    bool
	frame                              int
	pulseRunning                       bool
}

var commands = []string{"conversations", "new chat", "meta ai", "connect", "attach", "help", "settings", "logout", "quit"}

func New(repo history.Repository) Model {
	return Model{repo: repo, ctx: context.Background(), account: "local", width: 80, height: 24, active: welcomeScreen, pulseRunning: true}
}

func NewWithRuntime(runtime *Runtime, settings config.Settings, settingsPath string) Model {
	m := New(runtime.Repo)
	m.runtime, m.settings, m.settingsPath, m.ctx = runtime, settings, settingsPath, runtime.ctx
	m.consentAccepted = settings.Accepted()
	if settings.Accepted() {
		m.active = inboxScreen
	}
	return m
}

func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{pulse()}
	if m.runtime != nil {
		cmds = append(cmds, m.wait())
	}
	if m.settings.Accepted() {
		cmds = append(cmds, m.loadChats(nil))
		if !m.demo {
			cmds = append(cmds, m.connect())
		}
	}
	return tea.Batch(cmds...)
}

func (m Model) wait() tea.Cmd {
	if m.runtime == nil {
		return nil
	}
	return func() tea.Msg {
		select {
		case <-m.runtime.Wake():
			timer := time.NewTimer(75 * time.Millisecond)
			defer timer.Stop()
			select {
			case <-timer.C:
				return refreshMsg{}
			case <-m.runtime.done:
				return nil
			}
		case <-m.runtime.done:
			return nil
		}
	}
}

func (m Model) connect() tea.Cmd {
	if m.runtime == nil || !m.settings.Accepted() {
		return nil
	}
	return func() tea.Msg { return operationMsg{m.runtime.Connect()} }
}

func (m Model) loadChats(before *history.Cursor) tea.Cmd {
	if m.repo == nil || !m.settings.Accepted() {
		return nil
	}
	return func() tea.Msg {
		chats, err := m.repo.Conversations(m.ctx, m.account, before, 50)
		return chatsMsg{before, m.account, chats, err}
	}
}

func (m Model) loadMessages(before *history.Cursor) tea.Cmd {
	if m.repo == nil || m.chatID == "" || !m.settings.Accepted() {
		return nil
	}
	return func() tea.Msg {
		messages, err := m.repo.Messages(m.ctx, m.account, m.chatID, before, 50)
		return messagesMsg{before, m.account, m.chatID, messages, err}
	}
}

func (m Model) loadRevisions(before int) tea.Cmd {
	if m.repo == nil || m.revisionMessage == "" {
		return nil
	}
	return func() tea.Msg {
		revisions, err := m.repo.Revisions(m.ctx, m.account, m.chatID, m.revisionMessage, before, 20)
		return revisionsMsg{m.account, m.chatID, m.revisionMessage, revisions, err}
	}
}

func (m Model) save(settings config.Settings, consent bool) tea.Cmd {
	return func() tea.Msg {
		var err error
		if m.settingsPath != "" {
			err = config.Save(m.settingsPath, settings)
		}
		return savedMsg{settings, err, consent}
	}
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case pulseMsg:
		m.frame = (m.frame + 1) % 4
		if !m.settings.ReducedMotion {
			return m, pulse()
		}
		m.pulseRunning = false
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.hover = ""
	case refreshMsg:
		if m.runtime == nil {
			return m, nil
		}
		previous := m.account
		previousQR := m.connection.QR
		m.connection = m.runtime.Snapshot()
		if m.connection.AccountID != "" {
			m.account = m.connection.AccountID
		}
		if previous != m.account {
			m.chatID = ""
			m.chatBefore, m.messageBefore = nil, nil
			m.messages = nil
			m.conversations = nil
			m.chatIndex = 0
			m.messageIndex = 0
			m.composer = ""
			m.composing = false
			m.revisions = nil
			m.trail = nil
			if m.active != pairingScreen {
				m.active = inboxScreen
			}
		}
		if m.connection.QR != "" && previousQR == "" && m.settings.Accepted() && !m.composing {
			m.active = pairingScreen
		}
		if m.connection.Online && m.active == pairingScreen {
			m.active = inboxScreen
			m.fullscreen = false
		}
		return m, tea.Batch(m.wait(), m.loadChats(m.chatBefore), m.loadMessages(m.messageBefore))
	case operationMsg:
		if msg.err != nil {
			m.notice = "Operation failed. Check connectivity, then retry; details are not logged."
		}
	case savedMsg:
		m.saving = false
		if msg.err != nil {
			m.notice = "Could not save private settings; connection remains disabled."
			return m, nil
		}
		m.settings = msg.settings
		var mouse tea.Cmd = tea.DisableMouse
		if m.settings.Mouse {
			mouse = tea.EnableMouseAllMotion
		}
		if msg.consent {
			if m.runtime != nil {
				m.runtime.AllowConnection()
			}
			m.active = pairingScreen
			return m, tea.Batch(mouse, m.connect(), m.loadChats(nil))
		}
		if !m.settings.ReducedMotion && !m.pulseRunning {
			m.pulseRunning = true
			return m, tea.Batch(mouse, pulse())
		}
		return m, mouse
	case chatsMsg:
		if msg.account != m.account || !sameCursor(msg.before, m.chatBefore) {
			return m, nil
		}
		if msg.err != nil {
			m.notice = "Could not load conversations."
			return m, nil
		}
		selected := ""
		if m.chatIndex < len(m.conversations) {
			selected = m.conversations[m.chatIndex].ID
		}
		m.conversations = msg.chats
		m.chatIndex = 0
		for i, c := range m.conversations {
			if c.ID == selected {
				m.chatIndex = i
			}
		}
		if indices := m.visibleChats(); len(indices) > 0 {
			found := false
			for _, i := range indices {
				found = found || i == m.chatIndex
			}
			if !found {
				m.chatIndex = indices[0]
			}
		}
	case createdChatMsg:
		if msg.account != m.account {
			return m, nil
		}
		if msg.err != nil {
			m.notice = "Could not create the conversation locally; nothing was sent."
			return m, nil
		}
		if m.sending || m.composer != "" {
			m.notice = "Conversation saved. Clear the current draft before switching recipients."
			return m, m.loadChats(nil)
		}
		m.chatID, m.chatTitle, m.messages = msg.chat, msg.title, nil
		m.messageIndex, m.messageBefore = 0, nil
		m.active, m.trail, m.newChat = chatScreen, nil, ""
		m.search, m.searching = "", false
		return m, tea.Batch(m.loadChats(nil), m.loadMessages(nil))
	case messagesMsg:
		if msg.account != m.account || msg.chat != m.chatID || !sameCursor(msg.before, m.messageBefore) {
			return m, nil
		}
		if msg.err != nil {
			m.notice = "Could not load messages."
			return m, nil
		}
		selected := ""
		if m.messageIndex < len(m.messages) {
			selected = m.messages[m.messageIndex].ID
		}
		m.messages = msg.messages
		m.messageIndex = 0
		for i, message := range m.messages {
			if message.ID == selected {
				m.messageIndex = i
			}
		}
	case revisionsMsg:
		if msg.account != m.account || msg.chat != m.chatID || msg.message != m.revisionMessage {
			return m, nil
		}
		if msg.err != nil {
			m.notice = "Could not load revisions."
			return m, nil
		}
		m.revisions = msg.revisions
		m.scroll = 0
	case fileMsg:
		if m.active != attachmentScreen || msg.path != m.path {
			return m, nil
		}
		if msg.err != nil {
			m.notice = "Choose a readable regular file (maximum 10 MiB, no symlinks)."
		} else {
			m.file = msg.file
			m.active = confirmFileScreen
			m.notice = ""
		}
	case sentMsg:
		m.sending = false
		m.sendCancel = nil
		if msg.err != nil {
			m.notice = msg.err.Error()
		} else {
			m.notice = "Sent"
			if m.demo {
				m.notice = "Demo message saved locally; nothing transmitted."
			}
			if msg.account == m.account && msg.chat == m.chatID {
				m.composer = ""
				m.path = ""
				m.composing = false
			}
		}
		return m, m.loadMessages(m.messageBefore)
	case tea.MouseMsg:
		if !m.settings.Mouse {
			return m, nil
		}
		if m.width >= 90 && m.height >= 20 && (m.active == inboxScreen || m.active == chatScreen) {
			return m, m.mouse(msg)
		}
		if m.composing || m.active == commandScreen || m.active == newChatScreen || m.active == attachmentScreen {
			return m, nil
		}
		if msg.Button == tea.MouseButtonWheelUp {
			cmd := m.key(tea.KeyMsg{Type: tea.KeyUp})
			return m, cmd
		}
		if msg.Button == tea.MouseButtonWheelDown {
			cmd := m.key(tea.KeyMsg{Type: tea.KeyDown})
			return m, cmd
		}
	case tea.KeyMsg:
		cmd := m.key(msg)
		return m, cmd
	}
	return m, nil
}

func (m *Model) key(key tea.KeyMsg) tea.Cmd {
	k := key.String()
	if k == "ctrl+c" {
		if m.sendCancel != nil {
			m.sendCancel()
		}
		return tea.Quit
	}
	if k == "ctrl+x" && m.sendCancel != nil {
		m.sendCancel()
		m.notice = "Cancelling send; delivery may be uncertain."
		return nil
	}
	if m.active == newChatScreen {
		return m.newChatKey(key)
	}
	if m.active == commandScreen {
		return m.palette(key)
	}
	if m.active == attachmentScreen {
		return m.attachment(key)
	}
	if m.composing {
		return m.compose(key)
	}
	if m.searching {
		return m.searchKey(key)
	}
	if (m.active == inboxScreen || m.active == chatScreen) && m.settings.Accepted() {
		switch k {
		case "tab":
			if m.active == chatScreen {
				m.active = inboxScreen
			} else if m.chatID != "" {
				m.active = chatScreen
			}
			return nil
		case "ctrl+f":
			m.active, m.searching = inboxScreen, true
			return nil
		case "n":
			m.newChat = ""
			m.openOverlay(newChatScreen)
			return nil
		case "?":
			m.openOverlay(helpScreen)
			return nil
		}
	}
	if k == "q" {
		return tea.Quit
	}
	if k == "/" || k == "ctrl+p" {
		m.openOverlay(commandScreen)
		m.filter = ""
		m.commandIndex = 0
		return nil
	}
	if k == "esc" {
		m.notice = ""
		m.scroll = 0
		switch m.active {
		case consentScreen:
			if !m.saving {
				m.active = welcomeScreen
			}
		case chatScreen:
			m.active = inboxScreen
		case revisionsScreen, confirmFileScreen, logoutScreen:
			m.active = chatScreen
			if m.chatID == "" {
				m.active = inboxScreen
			}
		case settingsScreen, helpScreen:
			m.leaveOverlay()
		case pairingScreen:
			m.fullscreen = false // Use q to cancel pairing and shut down the adapter.
		}
		return nil
	}
	switch m.active {
	case welcomeScreen:
		if k == "enter" {
			m.active = consentScreen
			m.scroll = 0
		}
	case consentScreen:
		if k == " " && !m.saving {
			m.consentAccepted = !m.consentAccepted
		}
		if k == "enter" && m.consentAccepted && !m.saving {
			m.saving = true
			settings := m.settings
			settings.ConsentVersion = config.ConsentVersion
			return m.save(settings, true)
		}
		m.scrollKey(k)
	case pairingScreen:
		if k == "r" {
			return m.connect()
		}
		if k == "f" {
			m.fullscreen = !m.fullscreen
		}
		if k == "i" && m.settings.Accepted() {
			m.active = inboxScreen
		}
	case inboxScreen:
		switch k {
		case "up":
			m.moveChat(-1)
		case "down":
			m.moveChat(1)
		case "enter":
			return m.openChat()
		case "p":
			if len(m.conversations) > 0 {
				c := m.conversations[len(m.conversations)-1]
				m.chatBefore = &history.Cursor{Time: c.UpdatedAt, ID: c.ID}
				return m.loadChats(m.chatBefore)
			}
		case "r":
			m.chatBefore = nil
			return m.loadChats(nil)
		case "s":
			m.openOverlay(settingsScreen)
		}
	case chatScreen:
		switch k {
		case "up":
			if m.messageIndex+1 < len(m.messages) {
				m.messageIndex++
			}
		case "down":
			if m.messageIndex > 0 {
				m.messageIndex--
			}
		case "i":
			if !m.sending {
				m.composing = true
			}
		case "enter":
			if len(m.messages) > 0 {
				m.revisionMessage = m.messages[m.messageIndex].ID
				m.revisions = nil
				m.active = revisionsScreen
				m.scroll = 0
				return m.loadRevisions(0)
			}
		case "p":
			if len(m.messages) > 0 {
				last := m.messages[len(m.messages)-1]
				m.messageBefore = &history.Cursor{Time: last.CreatedAt, ID: last.ID}
				return m.loadMessages(m.messageBefore)
			}
		case "r":
			m.messageBefore = nil
			return m.loadMessages(nil)
		case "a":
			if !m.sending {
				m.active = attachmentScreen
				m.path = ""
				m.notice = ""
			}
		case "s":
			m.openOverlay(settingsScreen)
		}
	case settingsScreen:
		if (k == "m" || k == "t") && !m.saving {
			settings := m.settings
			if k == "m" {
				settings.Mouse = !settings.Mouse
			} else {
				settings.ReducedMotion = !settings.ReducedMotion
			}
			m.saving = true
			return m.save(settings, false)
		}
		m.scrollKey(k)
	case revisionsScreen:
		if k == "p" && len(m.revisions) > 0 {
			return m.loadRevisions(m.revisions[len(m.revisions)-1].Number)
		}
		if k == "r" {
			return m.loadRevisions(0)
		}
		m.scrollKey(k)
	case helpScreen:
		m.scrollKey(k)
	case confirmFileScreen:
		if k == "enter" && !m.sending {
			m.active = chatScreen
			return m.send("[document] "+m.file.Name, m.path)
		}
	case logoutScreen:
		if k == "enter" && m.runtime != nil && m.settings.Accepted() {
			m.active = inboxScreen
			return func() tea.Msg {
				ctx, cancel := context.WithTimeout(m.ctx, 20*time.Second)
				defer cancel()
				return operationMsg{m.runtime.Logout(ctx)}
			}
		}
	}
	return nil
}

func (m *Model) scrollKey(key string) {
	switch key {
	case "up":
		if m.scroll > 0 {
			m.scroll--
		}
	case "down":
		m.scroll++
	case "pgdown":
		m.scroll += max(1, m.height-10)
	case "pgup":
		m.scroll = max(0, m.scroll-max(1, m.height-10))
	}
}

func (m Model) filteredCommands() []string {
	var matches []string
	for _, command := range commands {
		if !m.settings.Accepted() && command != "help" && command != "settings" && command != "quit" {
			continue
		}
		if strings.Contains(command, strings.ToLower(m.filter)) {
			matches = append(matches, command)
		}
	}
	return matches
}

func (m *Model) palette(key tea.KeyMsg) tea.Cmd {
	matches := m.filteredCommands()
	switch key.String() {
	case "esc":
		m.leaveOverlay()
	case "up":
		m.commandIndex = max(0, m.commandIndex-1)
	case "down":
		m.commandIndex = min(max(0, len(matches)-1), m.commandIndex+1)
	case "backspace":
		m.filter = dropRune(m.filter)
		m.commandIndex = 0
	case "enter":
		if len(matches) == 0 {
			return nil
		}
		command := matches[min(m.commandIndex, len(matches)-1)]
		m.leaveOverlay()
		m.scroll = 0
		switch command {
		case "quit":
			return tea.Quit
		case "help":
			m.openOverlay(helpScreen)
		case "settings":
			m.openOverlay(settingsScreen)
		case "conversations":
			m.active = inboxScreen
			m.trail = nil
			m.chatBefore = nil
			return m.loadChats(nil)
		case "connect":
			m.active = pairingScreen
			return m.connect()
		case "new chat":
			m.newChat = ""
			m.openOverlay(newChatScreen)
		case "meta ai":
			return m.openMetaAI()
		case "logout":
			m.active = logoutScreen
		case "attach":
			if m.chatID != "" && !m.sending {
				m.active = attachmentScreen
				m.path = ""
			} else {
				m.notice = "Select a chat first and wait for any pending send."
			}
		}
	default:
		if (key.Type == tea.KeyRunes || key.Type == tea.KeySpace) && len([]rune(m.filter)) < 80 {
			m.filter += ui.SingleLine(string(key.Runes))
			m.commandIndex = 0
		}
	}
	return nil
}

func sameCursor(a, b *history.Cursor) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.ID == b.ID && a.Time.Equal(b.Time)
}

func (m *Model) openOverlay(next screen) {
	if m.active == next {
		return
	}
	m.trail = append(m.trail, m.active)
	m.active = next
	m.scroll = 0
}

func (m *Model) leaveOverlay() {
	m.active = inboxScreen
	if n := len(m.trail); n > 0 {
		m.active = m.trail[n-1]
		m.trail = m.trail[:n-1]
	}
	m.scroll = 0
}

func dropRune(s string) string {
	runes := []rune(s)
	if len(runes) > 0 {
		return string(runes[:len(runes)-1])
	}
	return s
}

func (m *Model) compose(key tea.KeyMsg) tea.Cmd {
	switch key.String() {
	case "ctrl+u":
		if !m.sending {
			m.composer = ""
		}
	case "esc":
		m.composing = false
	case "enter":
		if !m.sending && strings.TrimSpace(m.composer) != "" {
			return m.send(m.composer, "")
		}
	case "backspace":
		if !m.sending {
			m.composer = dropRune(m.composer)
		}
	case "ctrl+j":
		if !m.sending && len([]rune(m.composer)) < 4096 {
			m.composer += "\n"
		}
	default:
		if !m.sending && (key.Type == tea.KeyRunes || key.Type == tea.KeySpace) {
			text := m.composer + ui.SafeText(string(key.Runes))
			if len([]rune(text)) <= 4096 {
				m.composer = text
			}
		}
	}
	return nil
}

func (m *Model) attachment(key tea.KeyMsg) tea.Cmd {
	switch key.String() {
	case "esc":
		m.active = chatScreen
	case "backspace":
		m.path = dropRune(m.path)
	case "enter":
		path := m.path
		return func() tea.Msg { file, err := messaging.InspectAttachment(path); return fileMsg{path, file, err} }
	default:
		if (key.Type == tea.KeyRunes || key.Type == tea.KeySpace) && len(m.path) < 4096 {
			m.path += ui.SingleLine(string(key.Runes))
		}
	}
	return nil
}

func (m *Model) send(text, path string) tea.Cmd {
	if m.demo {
		if path != "" {
			m.notice = "Demo mode does not read or send files."
			return nil
		}
		account, chat, title, repo, ctx := m.account, m.chatID, m.chatTitle, m.repo, m.ctx
		m.sending = true
		m.notice = "Saving demo message locally; nothing is transmitted."
		return func() tea.Msg {
			err := repo.ApplyEvent(ctx, history.IncomingEvent{
				Kind: history.MessageEvent, AccountID: account, ChatID: chat, Title: title,
				MessageID: fmt.Sprintf("demo-send-%d", time.Now().UnixNano()), Sender: "You",
				Body: text, Timestamp: time.Now().UTC(), FromMe: true, Status: "sent",
			})
			return sentMsg{account, chat, err}
		}
	}
	if m.runtime == nil || !m.connection.Online {
		m.notice = "Not connected; nothing sent."
		return nil
	}
	ctx, cancel := context.WithTimeout(m.ctx, 2*time.Minute)
	m.sendCancel = cancel
	m.sending = true
	m.notice = "Sending… delivery will be confirmed or marked unknown."
	chat := m.chatID
	runtime := m.runtime
	account := m.account
	return func() tea.Msg {
		defer cancel()
		return sentMsg{account: account, chat: chat, err: runtime.Send(ctx, chat, text, path)}
	}
}

func (m Model) View() string {
	if m.width < 30 || m.height < 10 {
		return "WhatsCLI\nEnlarge terminal to at least 30×10.\nCtrl+C quits."
	}
	if m.active == pairingScreen && m.connection.QR != "" && m.fullscreen {
		return ui.QR(m.connection.QR, m.width, m.height-2) + "\nEsc · exit fullscreen    q · cancel and quit"
	}
	if m.active == pairingScreen && m.width >= 110 && m.height >= 26 {
		return m.pairingView()
	}
	if m.width >= 90 && m.height >= 20 && (m.active == inboxScreen || m.active == chatScreen) {
		return m.workspaceView()
	}
	width := m.width - 4
	height := m.height - 7
	title, body, footer := m.content(width, height)
	lines := strings.Split(ansi.Hardwrap(body, width, true), "\n")
	start := min(m.scroll, max(0, len(lines)-height))
	visible := strings.Join(lines[start:min(len(lines), start+height)], "\n")
	if len(lines) > height {
		footer = "↑/↓ · scroll   " + footer
	}
	status := m.connection.Status
	if status == "" {
		status = "Offline · local history"
	}
	if m.connection.Error != "" {
		status = m.connection.Error
	}
	if m.notice != "" {
		status = m.notice
	}
	if m.demo {
		status = "DEMO · synthetic conversations · no network"
	}
	if m.active == welcomeScreen && m.width >= 60 && m.height >= 20 {
		return m.welcomeView()
	}
	return ui.Frame(ui.Brand(" >_ WhatsCLI · "+ui.SingleLine(title))+"\n"+
		ui.Muted(" "+ansi.Truncate(ui.SingleLine(status), max(1, m.width-4), "…"))+"\n\n"+
		visible+"\n\n"+ui.Muted(ansi.Truncate(footer, m.width-2, "…")), m.width, m.height, true)
}

func (m Model) content(width, height int) (string, string, string) {
	switch m.active {
	case welcomeScreen:
		return "Welcome", "Your conversations. Your terminal.\n\nAn independent WhatsApp client for your terminal.\nLocal retained history is not encrypted.", "Enter · get started    / · commands    q · quit"
	case consentScreen:
		box := "[ ]"
		if m.consentAccepted {
			box = "[x]"
		}
		body := "BEFORE YOU CONNECT                                  STEP 01 / 02\n\n" +
			ui.Brand("01  Independent and unofficial") + "\nNot created, sponsored, or endorsed by WhatsApp or Meta.\n\n" +
			ui.Brand("02  Your account, your responsibility") + "\nUnofficial clients may lead to account restrictions.\nUse a spare account; account safety is not guaranteed.\n\n" +
			ui.Brand("03  Local data storage") + "\nCredentials and history remain on this host, unencrypted.\nEdits and revoked messages received here are retained locally.\n\n" +
			ui.Brand("04  Early software") + "\nLive compatibility and complete history sync are not certified.\nDo not rely on this client for critical communication.\n\n" +
			ui.Brand(box) + " I understand and accept these risks."
		return "Disclaimer & consent", body, "Space · accept    Enter · save/continue    Esc · back"
	case pairingScreen:
		body := "Open WhatsApp → Linked devices → Link a device.\nNever share the pairing QR. Keep this terminal open.\n\n"
		if m.connection.QR != "" {
			body += ui.QR(m.connection.QR, width, height-4)
		} else {
			body += "Waiting for a pairing code or an existing session…"
		}
		return "Link a device", body, "f · fullscreen QR    r · retry    i · history    q · cancel/quit"
	case commandScreen:
		lines := []string{"Filter: /" + ui.SingleLine(m.filter), ""}
		for i, c := range m.filteredCommands() {
			prefix := "  "
			if i == m.commandIndex {
				prefix = "> "
			}
			lines = append(lines, prefix+c)
		}
		if len(lines) == 2 {
			lines = append(lines, "No matching commands")
		}
		return "Commands", strings.Join(lines, "\n"), "↑/↓ · select    Enter · run    Esc · return"
	case inboxScreen:
		if len(m.conversations) == 0 {
			return "Conversations", "No conversations received yet.\nUse /connect to link, or n to start a new chat.\nOlder chats depend on the history WhatsApp provides.", "n · new chat    Ctrl+F · search    s · settings    / · commands"
		}
		var lines []string
		indices := m.visibleChats()
		selected := 0
		for pos, i := range indices {
			if i == m.chatIndex {
				selected = pos
			}
		}
		start := max(0, selected-height+2)
		if m.searching || m.search != "" {
			lines = append(lines, "Search: "+ui.SingleLine(m.search)+"▏")
		}
		for pos := start; pos < min(len(indices), start+height-1); pos++ {
			i := indices[pos]
			c := m.conversations[i]
			prefix := "  "
			if i == m.chatIndex {
				prefix = "> "
			}
			lines = append(lines, ansi.Truncate(prefix+ui.SingleLine(c.Title), width, "…"))
		}
		if len(indices) == 0 {
			lines = append(lines, "No matching chats. Esc clears search.")
		}
		return "Conversations", strings.Join(lines, "\n"), "↑/↓ · select  Enter · chat  p · older  r · newest  s · settings"
	case chatScreen:
		var lines []string
		count := max(1, height-3)
		end := min(len(m.messages), max(count, m.messageIndex+1))
		for i := end - 1; i >= max(0, end-count); i-- {
			message := m.messages[i]
			prefix := "  "
			if i == m.messageIndex {
				prefix = "> "
			}
			marker := ""
			if message.Edited {
				marker += " [edited]"
			}
			if message.Deleted {
				marker += " [deleted locally retained]"
			}
			if message.FromMe {
				marker += " [" + message.Status + "]"
			}
			lines = append(lines, ansi.Truncate(prefix+message.CreatedAt.Local().Format("15:04")+" "+ui.SingleLine(message.Sender)+": "+ui.SingleLine(message.Body)+marker, width, "…"))
		}
		if len(lines) == 0 {
			lines = append(lines, "No messages on this page.")
		}
		footer := "i · compose  Enter · details  p · older  r · newest  a · file  Esc · chats"
		if m.composing {
			lines = append(lines, "", "Message: "+ansi.Truncate(ui.SingleLine(m.composer), max(1, width-9), "…"))
			footer = "Enter · send    Ctrl+J · newline    Esc · leave composer    Ctrl+C · quit"
		}
		return m.chatTitle, strings.Join(lines, "\n"), footer
	case revisionsScreen:
		lines := []string{"Retained versions (most recently observed first; not source-time order).", ""}
		for _, r := range m.revisions {
			lines = append(lines, fmt.Sprintf("Revision %d · %s", r.Number, r.EditedAt.Local().Format(time.RFC822)), ui.SafeText(r.Body), "")
		}
		if len(m.revisions) == 0 {
			lines = append(lines, "No text version available; an original may never have been received.")
		}
		return "Message history", strings.Join(lines, "\n"), "↑/↓ · scroll    p · older versions    r · latest    Esc · back"
	case settingsScreen:
		return "Settings", fmt.Sprintf("INTERACTION\n\nMouse support: %t  [m]\nClick chats, buttons and composer; hover highlights controls.\nKeyboard navigation always remains available.\n\nReduced motion: %t  [t]\n\nPRIVACY\n\nHistory and credentials are stored on this execution host.\nHistory is not encrypted. Edits/deletions are retained.\nQuitting preserves your login. /logout unlinks with confirmation\nbut retains local history.", m.settings.Mouse, m.settings.ReducedMotion), "m · mouse    t · reduced motion    Esc · back    / · commands"
	case helpScreen:
		return "Help", "KEYBOARD FIRST\n\n/ or Ctrl+P opens commands; type to filter, then arrows and Enter.\nTab switches panes. Ctrl+F searches loaded chat titles/numbers.\nn starts a direct chat using an international phone number.\nThe meta ai command opens Meta AI (account/region dependent).\ni composes; Enter sends; Ctrl+J adds a newline.\nEsc leaves input; Ctrl+U clears a draft while composing.\nEnter on a selected message shows retained versions.\np loads older pages; r returns to newest.\ns opens settings; m toggles optional mouse support there.\n\nFile paths refer to THIS host. Over SSH, copy laptop files\nwith SCP/SFTP first. Documents require confirmation (max 10 MiB).\nCalls and rich stickers are not implemented.\nCtrl+C cancels and quits. Nothing is sent automatically.", "↑/↓ · scroll    Esc · return"
	case newChatScreen:
		return "New conversation", "NEW DIRECT CHAT\n\nEnter an international phone number including country code.\nExample: +27 82 123 4567\n\n" + ui.Brand("> "+ui.SingleLine(m.newChat)+"▏") + "\n\nEnter creates a local conversation; it sends nothing.\nThe recipient must have a WhatsApp account.\n\nFor Meta AI, use the meta ai command in the palette.", "Enter · open chat    Esc · cancel"
	case attachmentScreen:
		return "Choose host-local file", "Path on this computer (not your SSH client):\n\n" + ui.SingleLine(m.path) + "\n\nMaximum 10 MiB. Spaces are supported; do not add shell quotes.", "Enter · preview    Esc · cancel    Ctrl+C · quit"
	case confirmFileScreen:
		return "Confirm attachment", fmt.Sprintf("Send to: %s\nFile: %s\nSize: %d bytes\n\nThe file will be sent as a document.\nIt is read from this host.", ui.SingleLine(m.chatTitle), ui.SingleLine(m.file.Name), m.file.Size), "Enter · send file    Esc · cancel"
	case logoutScreen:
		return "Unlink this device", "Unlink this WhatsCLI session from WhatsApp?\nThis requires pairing again.\nYour locally retained message history will NOT be deleted.", "Enter · unlink    Esc · cancel"
	}
	return "WhatsCLI", "", "Ctrl+C · quit"
}
