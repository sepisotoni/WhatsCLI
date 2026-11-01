package app

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"whatscli/internal/config"
	"whatscli/internal/history"
	"whatscli/internal/messaging"
	"whatscli/internal/ui"
	"whatscli/internal/updater"
	"whatscli/internal/webpair"
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
	attachmentScreen // typed host path
	confirmFileScreen
	logoutScreen
	newChatScreen
	explorerScreen
	emojiScreen
	menuScreen
	statusesScreen
	statusComposeScreen
	statusConfirmScreen
	updateScreen
)

type listMode int

const (
	modeAll listMode = iota
	modeContacts
	modeUnread
	modeArchive
)

const (
	avLoading = 1
	avDone    = 2
)

type avatarEntry struct {
	photo *ui.Photo
	state int
}

// Messages produced by asynchronous commands.
type pulseMsg time.Time
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
	replyID       string
	err           error
}
type createdChatMsg struct {
	account, chat, title string
	err                  error
}
type statusesMsg struct {
	account string
	items   []history.StatusUpdate
	err     error
}
type statusPostedMsg struct{ err error }
type reactionMsg struct {
	account, chat string
	err           error
}
type updateCheckMsg struct {
	version updater.Version
	err     error
}
type updateRunMsg struct {
	output string
	err    error
}
type unreadMsg struct {
	account string
	counts  map[string]int
}
type avatarMsg struct {
	key   string
	photo *ui.Photo
}
type webMsg struct {
	srv *webpair.Server
	err error
}

type Model struct {
	repo         history.Repository
	runtime      *Runtime
	ctx          context.Context
	settings     config.Settings
	settingsPath string
	state        config.UIState
	statePath    string
	stateDir     string
	stateDirty   bool
	stateSaved   time.Time

	active screen
	trail  []screen

	width, height                      int
	consentAccepted                    bool
	account                            string
	connection                         Connection
	chatBefore, messageBefore          *history.Cursor
	conversations                      []history.Conversation
	messages                           []history.Message
	revisions                          []history.Revision
	replyTo                            *history.Message
	reactionTarget                     *history.Message
	unread                             map[string]int
	chatIndex, messageIndex            int
	listTop, msgTop                    int
	msgFocus                           bool
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
	noticeErr                          bool
	scroll                             int
	fullscreen                         bool
	search, newChat, hover             string
	searching, demo                    bool
	frame                              int
	now                                time.Time
	listMode                           listMode
	welcomeFocus                       int
	settingsIndex, menuIndex           int
	emojiIndex, slashIndex             int
	avatars                            map[string]*avatarEntry
	pairingSince                       time.Time
	helpDelay                          time.Duration
	web                                *webpair.Server
	webNote                            string
	explorer                           *explorer
	lastDir                            string
	queue                              []uploadItem
	queueDone                          int
	uploading                          bool
	uploadErr                          string
	lastClickID                        string
	lastMouseX, lastMouseY             int
	lastClickAt                        time.Time
	lastUnread                         int
	demoSubtitle                       map[string]string
	statuses                           []history.StatusUpdate
	statusIndex                        int
	statusDraft                        string
	updateVersion                      updater.Version
	updateNotice                       string
	updateChecking, updateRunning      bool
	onboardingPending                  bool
}

var commands = []string{"conversations", "new chat", "statuses", "files", "meta ai", "connect", "help", "settings", "update", "logout", "quit"}

func New(repo history.Repository) Model {
	return Model{
		repo: repo, ctx: context.Background(), account: "local", width: 80, height: 24, active: welcomeScreen,
		unread: map[string]int{}, avatars: map[string]*avatarEntry{},
		state:        config.UIState{Read: map[string]time.Time{}, Archived: map[string]bool{}},
		welcomeFocus: 1, helpDelay: 2 * time.Minute, now: time.Now(),
	}
}

func NewWithRuntime(runtime *Runtime, settings config.Settings, settingsPath string) Model {
	m := New(runtime.Repo)
	m.runtime, m.settings, m.settingsPath, m.ctx = runtime, settings, settingsPath, runtime.ctx
	m.consentAccepted = settings.Accepted()
	m.onboardingPending = true
	return m
}

// WithState attaches the persisted unread/archive state and the state directory.
func (m Model) WithState(path, dir string) Model {
	m.statePath, m.stateDir = path, dir
	if path != "" {
		if state, err := config.LoadState(path); err == nil {
			m.state = state
		}
	}
	if m.state.Read == nil {
		m.state.Read = map[string]time.Time{}
	}
	if m.state.Archived == nil {
		m.state.Archived = map[string]bool{}
	}
	return m
}

// FlushState synchronously stores pending read marks; call after the program exits.
func (m Model) FlushState() {
	if m.statePath != "" && m.stateDirty {
		_ = config.SaveState(m.statePath, m.state)
	}
}

func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{pulse()}
	if m.runtime != nil {
		cmds = append(cmds, m.wait())
	}
	return tea.Batch(cmds...)
}

func pulse() tea.Cmd {
	return tea.Tick(400*time.Millisecond, func(t time.Time) tea.Msg { return pulseMsg(t) })
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
	if m.runtime == nil || !m.settings.Accepted() || m.demo {
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

func (m Model) loadStatuses() tea.Cmd {
	if m.repo == nil || !m.settings.Accepted() {
		return nil
	}
	account, repo, ctx := m.account, m.repo, m.ctx
	return func() tea.Msg {
		items, err := repo.Statuses(ctx, account, time.Now().Add(-24*time.Hour), 50)
		return statusesMsg{account: account, items: items, err: err}
	}
}

func (m *Model) postStatus() tea.Cmd {
	if strings.TrimSpace(m.statusDraft) == "" || m.sending {
		return nil
	}
	draft := strings.TrimSpace(m.statusDraft)
	m.sending = true
	if m.demo {
		account, repo := m.account, m.repo
		return func() tea.Msg {
			return statusPostedMsg{err: repo.SaveStatus(m.ctx, history.StatusUpdate{
				AccountID: account, ID: fmt.Sprintf("demo-status-%d", time.Now().UnixNano()),
				Sender: "me", Name: "You", Body: draft, CreatedAt: time.Now().UTC(),
			})}
		}
	}
	if m.runtime == nil || !m.connection.Online {
		m.sending = false
		m.setNotice("Not connected; status was not posted.", true)
		return nil
	}
	ctx, cancel := context.WithTimeout(m.ctx, 2*time.Minute)
	return func() tea.Msg {
		defer cancel()
		return statusPostedMsg{err: m.runtime.PostStatus(ctx, draft)}
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

type unreadSource interface {
	IncomingSince(context.Context, string, time.Time) ([]history.Stamp, error)
}

func (m Model) key2(chat string) string { return m.account + "|" + chat }

func (m Model) loadUnread() tea.Cmd {
	src, ok := m.repo.(unreadSource)
	if !ok || m.repo == nil || m.demo || !m.settings.Accepted() || len(m.state.Read) == 0 {
		return nil
	}
	marks := make(map[string]time.Time, len(m.state.Read))
	oldest := time.Now().UTC()
	prefix := m.account + "|"
	for k, v := range m.state.Read {
		if strings.HasPrefix(k, prefix) {
			marks[strings.TrimPrefix(k, prefix)] = v
			if v.Before(oldest) {
				oldest = v
			}
		}
	}
	account, ctx := m.account, m.ctx
	return func() tea.Msg {
		stamps, err := src.IncomingSince(ctx, account, oldest)
		if err != nil {
			return nil
		}
		counts := map[string]int{}
		for _, s := range stamps {
			if mark, ok := marks[s.ChatID]; ok && s.At.After(mark) {
				counts[s.ChatID]++
			}
		}
		return unreadMsg{account, counts}
	}
}

func (m *Model) markRead(chat string) {
	if chat == "" {
		return
	}
	at := time.Now().UTC()
	if m.chatID == chat && len(m.messages) > 0 && m.messages[0].CreatedAt.After(at) {
		at = m.messages[0].CreatedAt
	}
	m.state.Read[m.key2(chat)] = at
	delete(m.unread, chat)
	m.stateDirty = true
}

func (m Model) saveStateCmd() tea.Cmd {
	if m.statePath == "" || !m.stateDirty {
		return nil
	}
	state := config.UIState{Read: map[string]time.Time{}, Archived: map[string]bool{}}
	for k, v := range m.state.Read {
		state.Read[k] = v
	}
	for k, v := range m.state.Archived {
		state.Archived[k] = v
	}
	path := m.statePath
	return func() tea.Msg { _ = config.SaveState(path, state); return nil }
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

func (m Model) avatarKey(chat string) string { return m.account + "|" + chat }

// fetchAvatars starts up to n concurrent profile-picture lookups. Failures and
// missing pictures resolve to initials; nothing blocks drawing.
func (m *Model) fetchAvatars(n int) tea.Cmd {
	if m.runtime == nil || m.demo || !m.settings.Photos() || !m.settings.Accepted() || !m.runtime.CanFetchAvatars() {
		return nil
	}
	loading := 0
	for _, e := range m.avatars {
		if e.state == avLoading {
			loading++
		}
	}
	var cmds []tea.Cmd
	for _, c := range m.conversations {
		if loading >= n {
			break
		}
		key := m.avatarKey(c.ID)
		if _, ok := m.avatars[key]; ok {
			continue
		}
		m.avatars[key] = &avatarEntry{state: avLoading}
		loading++
		runtime, ctx, chat := m.runtime, m.ctx, c.ID
		cmds = append(cmds, func() tea.Msg {
			data, err := runtime.Avatar(ctx, chat)
			if err != nil || len(data) == 0 {
				return avatarMsg{key, nil}
			}
			photo, err := ui.DecodePhoto(data)
			if err != nil {
				return avatarMsg{key, nil}
			}
			return avatarMsg{key, photo}
		})
	}
	return tea.Batch(cmds...)
}

func (m Model) photoFor(chat string) *ui.Photo {
	if !m.settings.Photos() {
		return nil
	}
	if e := m.avatars[m.avatarKey(chat)]; e != nil {
		return e.photo
	}
	return nil
}

func (m *Model) setNotice(text string, isErr bool) { m.notice, m.noticeErr = text, isErr }

func (m *Model) trackPairing() {
	if m.active == pairingScreen && !m.connection.Online {
		if m.pairingSince.IsZero() {
			m.pairingSince = m.now
		}
		return
	}
	m.pairingSince = time.Time{}
	if m.connection.Online && m.web != nil {
		m.web.Stop()
		m.web, m.webNote = nil, ""
	}
}

func (m Model) helpAvailable() bool {
	return m.active == pairingScreen && !m.pairingSince.IsZero() && m.now.Sub(m.pairingSince) >= m.helpDelay
}

func (m Model) startWeb() tea.Cmd {
	if m.runtime == nil || m.web != nil {
		return nil
	}
	runtime, ctx := m.runtime, m.ctx
	return func() tea.Msg {
		srv, err := webpair.Start(ctx, func() string { return runtime.Snapshot().QR }, 15*time.Minute)
		return webMsg{srv, err}
	}
}

func bell() tea.Cmd {
	return func() tea.Msg { _, _ = os.Stderr.WriteString("\a"); return nil }
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case pulseMsg:
		m.now = time.Time(msg)
		if !m.settings.ReducedMotion {
			m.frame = (m.frame + 1) % 10
		}
		m.trackPairing()
		cmds := []tea.Cmd{pulse()}
		if m.stateDirty && m.now.Sub(m.stateSaved) > time.Second {
			cmds = append(cmds, m.saveStateCmd())
			m.stateDirty, m.stateSaved = false, m.now
			m.stateDirty = false
		}
		return m, tea.Batch(cmds...)
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.hover = ""
		m.fitList()
		m.fitMessages()
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
			m.messages, m.conversations, m.revisions = nil, nil, nil
			m.chatIndex, m.messageIndex, m.listTop, m.msgTop = 0, 0, 0, 0
			m.composer, m.composing = "", false
			m.trail = nil
			m.unread = map[string]int{}
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
		m.trackPairing()
		return m, tea.Batch(m.wait(), m.loadChats(m.chatBefore), m.loadMessages(m.messageBefore), m.loadUnread(), m.fetchAvatars(4), m.loadStatuses())
	case operationMsg:
		if msg.err != nil {
			m.setNotice("Operation failed. Check connectivity, then retry; details are not logged.", true)
		}
	case webMsg:
		if msg.err != nil {
			m.webNote = "Could not open a local port."
			return m, nil
		}
		m.web = msg.srv
		host, _ := os.Hostname()
		m.webNote = fmt.Sprintf("%s  ·  over SSH: ssh -L %d:127.0.0.1:%d %s@%s", msg.srv.URL, msg.srv.Port, msg.srv.Port, os.Getenv("USER"), host)
	case savedMsg:
		m.saving = false
		if msg.err != nil {
			m.setNotice("Could not save private settings; connection remains disabled.", true)
			return m, nil
		}
		photosTurnedOn := !m.settings.Photos() && msg.settings.Photos()
		m.settings = msg.settings
		if photosTurnedOn {
			m.avatars = map[string]*avatarEntry{}
		}
		var mouse tea.Cmd = tea.DisableMouse
		if m.settings.Mouse {
			mouse = tea.EnableMouseAllMotion
		}
		if msg.consent {
			m.onboardingPending = false
			if m.runtime != nil {
				m.runtime.AllowConnection()
			}
			m.active = pairingScreen
			return m, tea.Batch(mouse, m.connect(), m.loadChats(nil))
		}
		return m, tea.Batch(mouse, m.fetchAvatars(4))
	case avatarMsg:
		if e := m.avatars[msg.key]; e != nil {
			e.photo, e.state = msg.photo, avDone
		}
		return m, m.fetchAvatars(4)
	case chatsMsg:
		if msg.account != m.account || !sameCursor(msg.before, m.chatBefore) {
			return m, nil
		}
		if msg.err != nil {
			m.setNotice("Could not load conversations.", true)
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
			// A chat with no read mark is treated as read up to its latest
			// message (so synced history is not flagged), unless that message
			// arrived moments ago, in which case it is genuinely unread.
			if _, ok := m.state.Read[m.key2(c.ID)]; !ok {
				mark := c.UpdatedAt
				if time.Since(c.UpdatedAt) < 2*time.Minute {
					mark = time.Unix(0, 0).UTC()
				}
				m.state.Read[m.key2(c.ID)] = mark
				m.stateDirty = true
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
		m.fitList()
		return m, tea.Batch(m.loadUnread(), m.fetchAvatars(4))
	case unreadMsg:
		if msg.account != m.account {
			return m, nil
		}
		if m.chatID != "" && (m.active == chatScreen) {
			delete(msg.counts, m.chatID)
		}
		m.unread = msg.counts
		total := 0
		for _, n := range msg.counts {
			total += n
		}
		ring := m.settings.Bell && total > m.lastUnread
		m.lastUnread = total
		if ring {
			return m, bell()
		}
	case statusesMsg:
		if msg.account != m.account {
			return m, nil
		}
		if msg.err != nil {
			m.setNotice("Could not load recent status updates.", true)
			return m, nil
		}
		m.statuses = msg.items
		m.statusIndex = min(m.statusIndex, max(0, len(m.statuses)-1))
	case statusPostedMsg:
		m.sending = false
		if msg.err != nil {
			m.setNotice(msg.err.Error(), true)
			return m, nil
		}
		m.statusDraft = ""
		m.active = statusesScreen
		m.setNotice("Status posted to your WhatsApp audience.", false)
		return m, m.loadStatuses()
	case updateCheckMsg:
		m.updateChecking = false
		if msg.err != nil {
			m.updateNotice = msg.err.Error()
		} else {
			m.updateVersion = msg.version
			if !msg.version.Available {
				m.updateNotice = "WhatsCLI is up to date."
			} else {
				m.updateNotice = "An update is available."
			}
		}
		return m, nil
	case updateRunMsg:
		m.updateRunning = false
		if msg.err != nil {
			m.updateNotice = msg.err.Error()
		} else {
			m.updateNotice = strings.TrimSpace(msg.output) + " Restart WhatsCLI to use the installed version."
			m.updateVersion = updater.Version{}
		}
		return m, nil
	case reactionMsg:
		m.sending = false
		if msg.err != nil {
			m.setNotice(msg.err.Error(), true)
		} else {
			m.active = chatScreen
			m.reactionTarget = nil
			m.setNotice("Reacted", false)
		}
		if msg.account == m.account && msg.chat == m.chatID {
			return m, m.loadMessages(m.messageBefore)
		}
		return m, nil
	case createdChatMsg:
		if msg.account != m.account {
			return m, nil
		}
		if msg.err != nil {
			m.setNotice("Could not create the conversation locally; nothing was sent.", true)
			return m, nil
		}
		if m.sending || m.composer != "" {
			m.setNotice("Conversation saved. Clear the current draft before switching recipients.", false)
			return m, m.loadChats(nil)
		}
		m.chatID, m.chatTitle, m.messages = msg.chat, msg.title, nil
		m.messageIndex, m.messageBefore, m.msgTop = 0, nil, 0
		m.replyTo, m.reactionTarget = nil, nil
		m.active, m.trail, m.newChat = chatScreen, nil, ""
		m.search, m.searching = "", false
		m.markRead(msg.chat)
		return m, tea.Batch(m.loadChats(nil), m.loadMessages(nil))
	case messagesMsg:
		if msg.account != m.account || msg.chat != m.chatID || !sameCursor(msg.before, m.messageBefore) {
			return m, nil
		}
		if msg.err != nil {
			m.setNotice("Could not load messages.", true)
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
		m.msgTop = min(m.msgTop, max(0, len(m.messages)-1))
		m.fitMessages()
		if m.active == chatScreen || m.composing {
			m.markRead(m.chatID)
		}
	case revisionsMsg:
		if msg.account != m.account || msg.chat != m.chatID || msg.message != m.revisionMessage {
			return m, nil
		}
		if msg.err != nil {
			m.setNotice("Could not load revisions.", true)
			return m, nil
		}
		m.revisions = msg.revisions
		m.scroll = 0
	case fileMsg:
		if m.active != attachmentScreen || msg.path != m.path {
			return m, nil
		}
		if msg.err != nil {
			m.setNotice("Choose a readable regular file (maximum 64 MiB, no symlinks).", true)
		} else {
			m.file = msg.file
			m.queue = []uploadItem{{path: msg.path, name: msg.file.Name, size: msg.file.Size}}
			m.queueDone, m.uploadErr = 0, ""
			m.active = confirmFileScreen
			m.setNotice("", false)
		}
	case dirMsg:
		return m, m.explorerLoaded(msg)
	case planMsg:
		return m, m.planReady(msg)
	case uploadMsg:
		return m, m.uploadDone(msg)
	case sentMsg:
		m.sending = false
		m.sendCancel = nil
		if msg.err != nil {
			m.setNotice(msg.err.Error(), true)
		} else {
			m.setNotice("Sent", false)
			if m.demo {
				m.setNotice("Demo message saved locally; nothing transmitted.", false)
			}
			if msg.account == m.account && msg.chat == m.chatID {
				m.composer = ""
				m.path = ""
				m.composing = false
				if m.replyTo != nil && m.replyTo.ID == msg.replyID {
					m.replyTo = nil
				}
			}
		}
		return m, m.loadMessages(m.messageBefore)
	case tea.MouseMsg:
		if !m.settings.Mouse {
			return m, nil
		}
		return m, m.mouse(msg)
	case tea.KeyMsg:
		cmd := m.key(msg)
		return m, tea.Batch(cmd, m.saveStateCmdIfIdle())
	}
	return m, nil
}

// saveStateCmdIfIdle saves read marks promptly after navigation, throttled.
func (m *Model) saveStateCmdIfIdle() tea.Cmd {
	if m.stateDirty && time.Since(m.stateSaved) > time.Second {
		cmd := m.saveStateCmd()
		m.stateDirty, m.stateSaved = false, time.Now()
		return cmd
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

func (m *Model) send(text, path string) tea.Cmd {
	return m.sendWithReply(text, path, nil)
}

func (m *Model) sendWithReply(text, path string, reply *history.Message) tea.Cmd {
	if m.demo {
		if path != "" {
			m.setNotice("Demo mode does not read or send files.", true)
			return nil
		}
		account, chat, title, repo, ctx := m.account, m.chatID, m.chatTitle, m.repo, m.ctx
		at := time.Now().UTC()
		if len(m.messages) > 0 && !m.messages[0].CreatedAt.Before(at) {
			at = m.messages[0].CreatedAt.Add(time.Minute) // demo data is dated later today
		}
		m.sending = true
		m.setNotice("Saving demo message locally; nothing is transmitted.", false)
		return func() tea.Msg {
			err := repo.ApplyEvent(ctx, history.IncomingEvent{
				Kind: history.MessageEvent, AccountID: account, ChatID: chat, Title: title,
				MessageID: fmt.Sprintf("demo-send-%d", time.Now().UnixNano()), Sender: "You",
				Body: text, Timestamp: at, FromMe: true, Status: "sent",
				ReplyToID: replyID(reply), ReplySender: replySender(reply), ReplyBody: replyBody(reply),
			})
			return sentMsg{account: account, chat: chat, replyID: replyID(reply), err: err}
		}
	}
	if m.runtime == nil || !m.connection.Online {
		m.setNotice("Not connected; nothing sent.", true)
		return nil
	}
	ctx, cancel := context.WithTimeout(m.ctx, 2*time.Minute)
	m.sendCancel = cancel
	m.sending = true
	m.setNotice("Sending… delivery will be confirmed or marked unknown.", false)
	chat := m.chatID
	runtime := m.runtime
	account := m.account
	target := reply
	return func() tea.Msg {
		defer cancel()
		var err error
		if target != nil {
			err = runtime.SendReply(ctx, chat, text, *target)
		} else {
			err = runtime.Send(ctx, chat, text, path)
		}
		return sentMsg{account: account, chat: chat, replyID: replyID(target), err: err}
	}
}

func (m *Model) react(emoji string) tea.Cmd {
	if m.reactionTarget == nil || m.sending {
		return nil
	}
	target := *m.reactionTarget
	account, chat := m.account, m.chatID
	if m.demo {
		m.sending = true
		repo, ctx := m.repo, m.ctx
		return func() tea.Msg {
			event := history.IncomingEvent{
				Kind: history.ReactionEvent, AccountID: account, ChatID: chat,
				MessageID: fmt.Sprintf("demo-reaction-%d", time.Now().UnixNano()),
				EventID:   fmt.Sprintf("demo-reaction:%s:%d", target.ID, time.Now().UnixNano()),
				Sender:    "You", Timestamp: time.Now().UTC(), FromMe: true,
				ReactionToID: target.ID, Reaction: emoji,
			}
			return reactionMsg{account: account, chat: chat, err: repo.ApplyEvent(ctx, event)}
		}
	}
	if m.runtime == nil || !m.connection.Online {
		m.setNotice("Not connected; reaction was not sent.", true)
		return nil
	}
	m.sending = true
	m.setNotice("Sending reaction…", false)
	ctx, cancel := context.WithTimeout(m.ctx, 2*time.Minute)
	runtime := m.runtime
	return func() tea.Msg {
		defer cancel()
		return reactionMsg{account: account, chat: chat, err: runtime.React(ctx, chat, target, emoji)}
	}
}

func replyID(message *history.Message) string {
	if message == nil {
		return ""
	}
	return message.ID
}

func replySender(message *history.Message) string {
	if message == nil {
		return ""
	}
	return message.Sender
}

func replyBody(message *history.Message) string {
	if message == nil {
		return ""
	}
	return message.Body
}
