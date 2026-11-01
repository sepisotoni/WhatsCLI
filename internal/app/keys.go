package app

import (
	"context"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"whatscli/internal/config"
	"whatscli/internal/history"
	"whatscli/internal/messaging"
	"whatscli/internal/ui"
)

const termsURL = "https://www.whatsapp.com/legal/terms-of-service"

func isText(key tea.KeyMsg) bool { return key.Type == tea.KeyRunes || key.Type == tea.KeySpace }

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
		m.setNotice("Cancelling send; delivery may be uncertain.", false)
		return nil
	}
	switch m.active {
	case newChatScreen:
		return m.newChatKey(key)
	case commandScreen:
		return m.palette(key)
	case attachmentScreen:
		return m.attachment(key)
	case explorerScreen:
		return m.explorerKey(key)
	case emojiScreen:
		return m.emojiKey(key)
	case menuScreen:
		return m.menuKey(key)
	case statusesScreen:
		return m.statusesKey(key)
	case statusComposeScreen:
		return m.statusComposeKey(key)
	case statusConfirmScreen:
		return m.statusConfirmKey(key)
	case updateScreen:
		return m.updateKey(key)
	}
	if m.composing {
		return m.compose(key)
	}
	if m.searching {
		return m.searchKey(key)
	}
	if k == "q" {
		return tea.Quit
	}
	if k == "ctrl+p" || (k == "/" && m.active != chatScreen) {
		m.openOverlay(commandScreen)
		m.filter = ""
		m.commandIndex = 0
		return nil
	}
	if k == "/" && m.active == chatScreen && m.settings.Accepted() && !m.sending {
		m.composing = true
		if m.composer == "" {
			m.composer = "/"
		}
		return nil
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
	if k == "esc" {
		m.setNotice("", false)
		m.scroll = 0
		switch m.active {
		case consentScreen:
			if !m.saving {
				m.active = welcomeScreen
			}
		case chatScreen:
			m.active = inboxScreen
		case confirmFileScreen:
			m.active = chatScreen
			if m.explorer != nil {
				m.active = explorerScreen
			} else if m.chatID == "" {
				m.active = inboxScreen
			}
		case revisionsScreen, logoutScreen:
			m.active = chatScreen
			if m.chatID == "" {
				m.active = inboxScreen
			}
		case settingsScreen, helpScreen, updateScreen:
			m.leaveOverlay()
		case pairingScreen:
			if m.fullscreen {
				m.fullscreen = false
			} else if m.settings.Accepted() {
				m.active = inboxScreen
			}
		}
		return nil
	}
	switch m.active {
	case welcomeScreen:
		switch k {
		case "tab", "left", "right", "up", "down", "shift+tab":
			m.welcomeFocus = 1 - m.welcomeFocus
		case "enter", " ":
			if m.welcomeFocus == 0 {
				return tea.Quit
			}
			m.active = consentScreen
			m.scroll = 0
		}
	case consentScreen:
		switch k {
		case " ":
			if !m.saving {
				m.consentAccepted = !m.consentAccepted
			}
		case "enter":
			return m.agree()
		case "t":
			m.setNotice("WhatsApp Terms of Service: "+termsURL, false)
		}
		m.scrollKey(k)
	case pairingScreen:
		switch k {
		case "r":
			return m.connect()
		case "f":
			m.fullscreen = !m.fullscreen
		case "i", "b":
			if m.settings.Accepted() {
				m.active = inboxScreen
			}
		case "h":
			return m.startWeb()
		}
	case inboxScreen:
		switch k {
		case "up":
			m.moveChat(-1)
		case "down":
			m.moveChat(1)
		case "pgup":
			m.moveChat(-5)
		case "pgdown":
			m.moveChat(5)
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
		case "e":
			m.toggleArchive()
		}
	case chatScreen:
		if k == "ctrl+r" && m.msgFocus && m.messageIndex >= 0 && m.messageIndex < len(m.messages) && !m.sending {
			target := m.messages[m.messageIndex]
			m.replyTo = &target
			m.composing, m.msgFocus = true, false
			return nil
		}
		if k == "ctrl+e" && m.msgFocus && m.messageIndex >= 0 && m.messageIndex < len(m.messages) && !m.sending {
			target := m.messages[m.messageIndex]
			m.reactionTarget = &target
			m.emojiIndex = 10
			m.active = emojiScreen
			return nil
		}
		switch k {
		case "up":
			m.msgFocus = true
			if m.messageIndex+1 < len(m.messages) {
				m.messageIndex++
				m.fitMessages()
			} else if len(m.messages) >= 50 {
				last := m.messages[len(m.messages)-1]
				m.messageBefore = &history.Cursor{Time: last.CreatedAt, ID: last.ID}
				return m.loadMessages(m.messageBefore)
			}
		case "down":
			m.msgFocus = true
			if m.messageIndex > 0 {
				m.messageIndex--
				m.fitMessages()
			}
		case "i":
			if !m.sending {
				m.composing, m.msgFocus = true, false
			}
		case "enter":
			return m.showVersions()
		case "p":
			if len(m.messages) > 0 {
				last := m.messages[len(m.messages)-1]
				m.messageBefore = &history.Cursor{Time: last.CreatedAt, ID: last.ID}
				return m.loadMessages(m.messageBefore)
			}
		case "r":
			m.messageBefore = nil
			m.msgTop, m.messageIndex = 0, 0
			return m.loadMessages(nil)
		case "a":
			return m.openExplorer()
		case "e":
			m.toggleArchive()
		case "s":
			m.openOverlay(settingsScreen)
		}
	case settingsScreen:
		switch k {
		case "up":
			m.settingsIndex = max(0, m.settingsIndex-1)
		case "down":
			m.settingsIndex = min(settingCount-1, m.settingsIndex+1)
		case "enter", " ":
			return m.activateSetting(m.settingsIndex)
		case "m":
			return m.activateSetting(0)
		case "t":
			return m.activateSetting(1)
		}
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
	case statusesScreen, statusComposeScreen, statusConfirmScreen:
		return nil
	case confirmFileScreen:
		if k == "enter" && !m.sending && !m.uploading {
			return m.startUpload()
		}
	case logoutScreen:
		if k == "enter" {
			return m.logout()
		}
	}
	return nil
}

func (m *Model) statusesKey(key tea.KeyMsg) tea.Cmd {
	switch key.String() {
	case "esc":
		m.leaveOverlay()
	case "up":
		m.statusIndex = max(0, m.statusIndex-1)
	case "down":
		m.statusIndex = min(max(0, len(m.statuses)-1), m.statusIndex+1)
	case "n":
		m.statusDraft = ""
		m.active = statusComposeScreen
	}
	return nil
}

func (m *Model) statusComposeKey(key tea.KeyMsg) tea.Cmd {
	switch key.String() {
	case "esc":
		m.active = statusesScreen
	case "backspace":
		m.statusDraft = dropRune(m.statusDraft)
	case "ctrl+u":
		m.statusDraft = ""
	case "enter":
		if strings.TrimSpace(m.statusDraft) == "" {
			m.setNotice("Type a status before previewing it.", true)
		} else {
			m.active = statusConfirmScreen
		}
	default:
		if isText(key) && len([]rune(m.statusDraft)) < 700 {
			m.statusDraft += ui.SingleLine(string(key.Runes))
		}
	}
	return nil
}

func (m *Model) statusConfirmKey(key tea.KeyMsg) tea.Cmd {
	switch key.String() {
	case "esc":
		m.active = statusComposeScreen
	case "enter":
		return m.postStatus()
	}
	return nil
}

func (m *Model) agree() tea.Cmd {
	if !m.consentAccepted || m.saving {
		if !m.consentAccepted {
			m.setNotice("Tick the box to accept the risks before continuing.", true)
		}
		return nil
	}
	m.saving = true
	settings := m.settings
	settings.ConsentVersion = config.ConsentVersion
	return m.save(settings, true)
}

func (m *Model) logout() tea.Cmd {
	if m.runtime == nil || !m.settings.Accepted() {
		return nil
	}
	m.active = inboxScreen
	runtime, ctx := m.runtime, m.ctx
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
		return operationMsg{runtime.Logout(ctx)}
	}
}

func (m *Model) showVersions() tea.Cmd {
	if len(m.messages) == 0 {
		return nil
	}
	m.revisionMessage = m.messages[m.messageIndex].ID
	m.revisions = nil
	m.active = revisionsScreen
	m.scroll = 0
	return m.loadRevisions(0)
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

func (m *Model) toggleArchive() {
	if m.chatID == "" && m.active == chatScreen {
		return
	}
	if len(m.conversations) == 0 || m.chatIndex >= len(m.conversations) {
		return
	}
	id := m.conversations[m.chatIndex].ID
	if m.active == chatScreen && m.chatID != "" {
		id = m.chatID
	}
	key := m.key2(id)
	if m.state.Archived[key] {
		delete(m.state.Archived, key)
		m.setNotice("Chat unarchived.", false)
	} else {
		m.state.Archived[key] = true
		m.setNotice("Chat archived. Open Archive from the chat list to see it.", false)
		if id == m.chatID {
			m.chatID, m.chatTitle, m.messages = "", "", nil
			m.active = inboxScreen
		}
	}
	m.stateDirty = true
	if indices := m.visibleChats(); len(indices) > 0 {
		m.chatIndex = indices[min(len(indices)-1, 0)]
	}
	m.fitList()
}

func (m Model) filteredCommands() []string {
	var matches []string
	for _, command := range commands {
		if (!m.settings.Accepted() || m.onboardingPending) && command != "help" && command != "settings" && command != "quit" {
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
		return m.runCommand(matches[min(m.commandIndex, len(matches)-1)])
	default:
		if isText(key) && len([]rune(m.filter)) < 80 {
			m.filter += ui.SingleLine(string(key.Runes))
			m.commandIndex = 0
		}
	}
	return nil
}

func (m *Model) runCommand(command string) tea.Cmd {
	m.leaveOverlay()
	m.scroll = 0
	switch command {
	case "quit":
		return tea.Quit
	case "help":
		m.openOverlay(helpScreen)
	case "settings":
		m.openOverlay(settingsScreen)
	case "statuses":
		m.openOverlay(statusesScreen)
		return m.loadStatuses()
	case "conversations":
		m.active = inboxScreen
		m.trail = nil
		m.listMode = modeAll
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
	case "files":
		return m.openExplorer()
	case "update":
		return m.openUpdate()
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
		if isText(key) && len(m.path) < 4096 {
			m.path += ui.SingleLine(string(key.Runes))
		}
	}
	return nil
}

func (m *Model) newChatKey(key tea.KeyMsg) tea.Cmd {
	switch key.String() {
	case "esc":
		m.newChat = ""
		m.leaveOverlay()
	case "backspace":
		m.newChat = dropRune(m.newChat)
	case "enter":
		jid, err := messaging.PhoneRecipient(m.newChat)
		if err != nil {
			m.setNotice(err.Error(), true)
			return nil
		}
		return m.createChat(jid, strings.TrimSpace(m.newChat))
	default:
		if isText(key) && len([]rune(m.newChat)) < 40 {
			m.newChat += ui.SingleLine(string(key.Runes))
		}
	}
	return nil
}

func (m *Model) createChat(chat, title string) tea.Cmd {
	if !m.settings.Accepted() || m.repo == nil {
		return nil
	}
	if m.sending || m.composer != "" {
		m.setNotice("Finish the send or clear the current draft (Ctrl+U in composer) before changing chats.", true)
		return nil
	}
	account, repo, ctx := m.account, m.repo, m.ctx
	return func() tea.Msg {
		err := repo.SaveConversation(ctx, history.Conversation{AccountID: account, ID: chat, Title: title, UpdatedAt: time.Now().UTC()})
		return createdChatMsg{account, chat, title, err}
	}
}

func (m *Model) openMetaAI() tea.Cmd {
	for _, chat := range m.conversations {
		if messaging.IsMetaAI(chat.ID) {
			return m.createChat(chat.ID, "Meta AI")
		}
	}
	return m.createChat(messaging.MetaAIChatID(), "Meta AI")
}

func (m *Model) searchKey(key tea.KeyMsg) tea.Cmd {
	switch key.String() {
	case "esc":
		m.searching, m.search = false, ""
	case "enter":
		return m.openChat()
	case "up":
		m.moveChat(-1)
	case "down":
		m.moveChat(1)
	case "backspace":
		m.search = dropRune(m.search)
	default:
		if isText(key) && len([]rune(m.search)) < 80 {
			m.search += ui.SingleLine(string(key.Runes))
		}
	}
	if indices := m.visibleChats(); len(indices) > 0 {
		found := false
		for _, index := range indices {
			found = found || index == m.chatIndex
		}
		if !found {
			m.chatIndex = indices[0]
		}
	}
	m.fitList()
	return nil
}

// ---- chat list helpers ----

func (m Model) isArchived(id string) bool { return m.state.Archived[m.key2(id)] }

func (m Model) visibleChats() []int {
	q := strings.ToLower(m.search)
	var indices []int
	for i, c := range m.conversations {
		if (m.listMode == modeArchive) != m.isArchived(c.ID) {
			continue
		}
		if m.listMode == modeContacts && strings.HasSuffix(c.ID, "@g.us") {
			continue
		}
		if m.listMode == modeUnread && m.unread[c.ID] == 0 {
			continue
		}
		if q != "" && !strings.Contains(strings.ToLower(c.Title+" "+c.ID), q) {
			continue
		}
		indices = append(indices, i)
	}
	return indices
}

func (m *Model) moveChat(delta int) {
	indices := m.visibleChats()
	if len(indices) == 0 {
		return
	}
	pos := 0
	for i, index := range indices {
		if index == m.chatIndex {
			pos = i
		}
	}
	m.chatIndex = indices[min(len(indices)-1, max(0, pos+delta))]
	m.fitList()
}

// fitList keeps the selected chat inside the list viewport.
func (m *Model) fitList() {
	indices := m.visibleChats()
	vis := m.listCapacity()
	pos := 0
	for i, index := range indices {
		if index == m.chatIndex {
			pos = i
		}
	}
	if pos < m.listTop {
		m.listTop = pos
	}
	if pos >= m.listTop+vis {
		m.listTop = pos - vis + 1
	}
	m.listTop = max(0, min(m.listTop, max(0, len(indices)-vis)))
}

func (m *Model) openChat() tea.Cmd {
	indices := m.visibleChats()
	if len(indices) == 0 || m.chatIndex >= len(m.conversations) {
		return nil
	}
	c := m.conversations[m.chatIndex]
	if c.ID != m.chatID && (m.sending || m.composer != "") {
		m.setNotice("Finish the send or clear the current draft (Ctrl+U in composer) before changing chats.", true)
		return nil
	}
	m.searching = false
	m.active = chatScreen
	m.markRead(c.ID)
	if m.chatID == c.ID {
		return nil
	}
	m.chatID, m.chatTitle, m.messages = c.ID, c.Title, nil
	m.messageBefore, m.messageIndex, m.msgTop, m.msgFocus = nil, 0, 0, false
	m.replyTo, m.reactionTarget = nil, nil
	return m.loadMessages(nil)
}

// ---- composer ----

func (m *Model) compose(key tea.KeyMsg) tea.Cmd {
	slash := m.slashSuggestions()
	switch key.String() {
	case "ctrl+u":
		if !m.sending {
			m.composer = ""
		}
	case "esc":
		m.composing = false
		if m.composer == "" {
			m.replyTo = nil
		}
	case "tab":
		if len(slash) > 0 {
			m.composer = "/" + slash[min(m.slashIndex, len(slash)-1)].name + " "
		}
	case "up":
		if len(slash) > 0 {
			m.slashIndex = max(0, m.slashIndex-1)
		}
	case "down":
		if len(slash) > 0 {
			m.slashIndex = min(len(slash)-1, m.slashIndex+1)
		}
	case "enter":
		return m.submit()
	case "backspace":
		if !m.sending {
			m.composer = dropRune(m.composer)
			m.slashIndex = 0
		}
	case "ctrl+j":
		if !m.sending && len([]rune(m.composer)) < 4096 {
			m.composer += "\n"
		}
	default:
		if !m.sending && isText(key) {
			text := m.composer + ui.SafeText(string(key.Runes))
			if len([]rune(text)) <= 4096 {
				m.composer = text
				m.slashIndex = 0
			}
		}
	}
	return nil
}

// ---- settings ----

const settingCount = 10

func (m *Model) activateSetting(i int) tea.Cmd {
	settings := m.settings
	switch i {
	case 0:
		settings.Mouse = !settings.Mouse
		settings.MouseConfigured = true
	case 1:
		settings.ReducedMotion = !settings.ReducedMotion
	case 2:
		settings.HidePhotos = !settings.HidePhotos
	case 3:
		settings.HideWallpaper = !settings.HideWallpaper
	case 4:
		settings.Clock12h = !settings.Clock12h
	case 5:
		settings.Bell = !settings.Bell
	case 6: // link / reconnect
		m.trail = nil
		m.active = pairingScreen
		return m.connect()
	case 7: // unlink
		m.active = logoutScreen
		return nil
	case 8: // open the file browser
		m.leaveOverlay()
		return m.openExplorer()
	case 9:
		return m.openUpdate()
	default:
		return nil
	}
	if m.saving {
		return nil
	}
	m.saving = true
	return m.save(settings, false)
}

// ---- menus / emoji ----

var menuItems = []string{"Message versions", "Send files…", "Archive / unarchive chat", "Search chats", "Settings", "Close chat"}

func (m *Model) menuKey(key tea.KeyMsg) tea.Cmd {
	switch key.String() {
	case "esc":
		m.active = chatScreen
	case "up":
		m.menuIndex = max(0, m.menuIndex-1)
	case "down":
		m.menuIndex = min(len(menuItems)-1, m.menuIndex+1)
	case "enter":
		return m.runMenu(m.menuIndex)
	}
	return nil
}

func (m *Model) runMenu(i int) tea.Cmd {
	m.active = chatScreen
	switch i {
	case 0:
		return m.showVersions()
	case 1:
		return m.openExplorer()
	case 2:
		m.toggleArchive()
	case 3:
		m.active, m.searching = inboxScreen, true
	case 4:
		m.openOverlay(settingsScreen)
	case 5:
		m.active = inboxScreen
	}
	return nil
}

var emojis = []string{"😀", "😂", "🙂", "😉", "😍", "😎", "🤔", "😢", "😮", "😡", "👍", "👎", "👌", "🙏", "👏", "🔥",
	"❤", "✅", "❌", "⭐", "🎉", "💯", "🚀", "👀", "🙌", "💪", "🤝", "😅", "🥳", "😴", "🤷", "👋"}

const emojiCols = 8

func (m *Model) emojiKey(key tea.KeyMsg) tea.Cmd {
	switch key.String() {
	case "esc":
		m.active = chatScreen
		if m.reactionTarget != nil {
			m.reactionTarget = nil
			m.composing = false
		} else {
			m.composing = true
		}
	case "left":
		m.emojiIndex = max(0, m.emojiIndex-1)
	case "right":
		m.emojiIndex = min(len(emojis)-1, m.emojiIndex+1)
	case "up":
		m.emojiIndex = max(0, m.emojiIndex-emojiCols)
	case "down":
		m.emojiIndex = min(len(emojis)-1, m.emojiIndex+emojiCols)
	case "enter", " ":
		return m.insertEmoji(m.emojiIndex)
	}
	return nil
}

func (m *Model) insertEmoji(i int) tea.Cmd {
	if i < 0 || i >= len(emojis) {
		return nil
	}
	if m.reactionTarget != nil {
		return m.react(emojis[i])
	}
	if text := m.composer + emojis[i]; len([]rune(text)) <= 4096 && !m.sending {
		m.composer = text
	}
	m.active, m.composing = chatScreen, true
	return nil
}

// submit runs a slash command or sends the draft (shared by Enter and the send button).
func (m *Model) submit() tea.Cmd {
	if m.sending || strings.TrimSpace(m.composer) == "" {
		return nil
	}
	if strings.HasPrefix(m.composer, "/") && !strings.HasPrefix(m.composer, "//") {
		return m.runSlash()
	}
	text := m.composer
	if strings.HasPrefix(text, "//") {
		text = text[1:] // "//" sends a message that begins with a literal slash
	}
	if m.replyTo != nil {
		return m.sendWithReply(text, "", m.replyTo)
	}
	return m.send(text, "")
}
