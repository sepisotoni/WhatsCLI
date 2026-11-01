package app

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func (m *Model) mouse(msg tea.MouseMsg) tea.Cmd {
	if m.consentDragging {
		if msg.Action == tea.MouseActionRelease {
			m.consentDragging = false
			return nil
		}
		if msg.Action == tea.MouseActionMotion {
			m.scrollConsentTo(msg.Y)
			return nil
		}
	}
	// Terminals report motion for every pixel-step; only a new cell can change
	// what is hovered, so skip the hit test otherwise.
	if msg.Action == tea.MouseActionMotion && msg.X == m.lastMouseX && msg.Y == m.lastMouseY && m.hover != "" {
		return nil
	}
	m.lastMouseX, m.lastMouseY = msg.X, msg.Y
	id := m.hitAt(msg.X, msg.Y)
	m.hover = id
	switch {
	case msg.Button == tea.MouseButtonWheelUp:
		return m.wheel(msg, -1)
	case msg.Button == tea.MouseButtonWheelDown:
		return m.wheel(msg, 1)
	case msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft:
		double := id != "" && id == m.lastClickID && time.Since(m.lastClickAt) < 450*time.Millisecond
		m.lastClickID, m.lastClickAt = id, time.Now()
		return m.activate(id, double)
	}
	return nil
}

func (m *Model) wheel(msg tea.MouseMsg, delta int) tea.Cmd {
	switch m.active {
	case explorerScreen:
		if m.explorer != nil {
			m.explorer.cursor += delta * 3
			m.explorer.clamp(m.explorerRows())
		}
	case helpScreen, revisionsScreen, confirmFileScreen, consentScreen:
		m.scroll = max(0, m.scroll+delta*3)
	case settingsScreen:
		m.settingsIndex = max(0, min(settingCount-1, m.settingsIndex+delta))
	case commandScreen:
		m.commandIndex = max(0, min(max(0, len(m.filteredCommands())-1), m.commandIndex+delta))
	case inboxScreen, chatScreen:
		g := m.geo()
		if g.list.W > 0 && msg.X < g.list.Right() {
			n := len(m.visibleChats())
			m.listTop = max(0, min(max(0, n-m.listCapacity()), m.listTop+delta*2))
		} else if delta < 0 && m.messageIndex+1 < len(m.messages) {
			m.messageIndex++
			m.fitMessages()
		} else if delta > 0 && m.messageIndex > 0 {
			m.messageIndex--
			m.fitMessages()
		}
	}
	return nil
}

func (m *Model) scrollConsentTo(y int) {
	W, H := m.width, m.height
	pw, ph := min(W-2, 92), min(H-2, 40)
	py := (H - ph) / 2
	iw := pw - 6
	bottom := 4
	if ph >= 30 {
		bottom = 11
	} else if ph < 12 {
		bottom = 2
	}
	viewY, viewH := py+5, ph-5-bottom
	content := consentHeight(iw)
	maxScroll := max(0, content-viewH)
	if maxScroll == 0 || viewH < 2 {
		m.scroll = 0
		return
	}
	m.scroll = max(0, min(maxScroll, (y-viewY)*maxScroll/(viewH-1)))
}

func idNum(id, prefix string) (int, bool) {
	if !strings.HasPrefix(id, prefix) {
		return 0, false
	}
	n, err := strconv.Atoi(strings.TrimPrefix(id, prefix))
	return n, err == nil
}

func (m *Model) pressKey(name string) tea.Cmd {
	switch name {
	case "esc":
		return m.key(tea.KeyMsg{Type: tea.KeyEsc})
	case "enter":
		return m.key(tea.KeyMsg{Type: tea.KeyEnter})
	}
	return m.key(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(name)})
}

func (m *Model) activate(id string, double bool) tea.Cmd {
	switch id {
	case "":
		return nil
	case "modal:backdrop", "modal:close":
		return m.pressKey("esc")
	case "modal:body", "body":
		return nil
	case "welcome:start":
		m.active, m.scroll = consentScreen, 0
	case "welcome:exit":
		return tea.Quit
	case "consent:check":
		if !m.saving {
			m.consentAccepted = !m.consentAccepted
		}
	case "consent:agree":
		return m.agree()
	case "consent:back":
		m.active = welcomeScreen
	case "consent:terms":
		m.setNotice("WhatsApp Terms of Service: "+termsURL, false)
	case "pair:back":
		m.active, m.fullscreen, m.scroll = consentScreen, false, 0
	case "consent:scrollbar":
		m.consentDragging = true
		m.scrollConsentTo(m.lastMouseY)
	case "pair:full":
		m.fullscreen = !m.fullscreen
	case "pair:help":
		return m.startWeb()
	case "rail:chats":
		m.listMode, m.search, m.searching = modeAll, "", false
		m.active, m.composing = inboxScreen, false
		m.fitList()
	case "rail:contacts":
		m.listMode, m.active, m.composing = modeContacts, inboxScreen, false
		m.moveChat(0)
	case "rail:unread":
		m.listMode, m.active, m.composing = modeUnread, inboxScreen, false
		m.moveChat(0)
	case "rail:new":
		m.composing, m.newChat = false, ""
		m.openOverlay(newChatScreen)
	case "rail:settings":
		m.composing = false
		m.openOverlay(settingsScreen)
	case "search":
		m.active, m.searching, m.composing = inboxScreen, true, false
	case "archive":
		if m.listMode == modeArchive {
			m.listMode = modeAll
		} else {
			m.listMode = modeArchive
		}
		m.moveChat(0)
	case "head:back":
		m.active, m.composing = inboxScreen, false
	case "head:video", "head:call":
		m.setNotice("Calls are not supported by this client.", true)
	case "head:more":
		m.composing, m.menuIndex = false, 0
		m.active = menuScreen
	case "plus":
		return m.openExplorer()
	case "status:new":
		m.statusDraft = ""
		m.active = statusComposeScreen
	case "status:back":
		m.active = statusComposeScreen
	case "status:post":
		return m.postStatus()
	case "emoji":
		if !m.sending {
			m.composing = false
			m.active = emojiScreen
		}
	case "send":
		return m.submit()
	case "compose":
		m.active = chatScreen
		m.composing = !m.sending
	case "logout:cancel":
		return m.pressKey("esc")
	case "logout:confirm":
		return m.logout()
	case "update:check":
		if !m.updateChecking && !m.updateRunning {
			return m.openUpdate()
		}
	case "update:run":
		return m.runUpdate()
	case "newchat:go":
		return m.key(tea.KeyMsg{Type: tea.KeyEnter})
	case "confirm:cancel":
		return m.pressKey("esc")
	case "confirm:send":
		if !m.uploading {
			return m.startUpload()
		}
	case "x:up":
		if m.explorer != nil {
			return m.pressKey("h")
		}
	case "x:path":
		if m.explorer != nil {
			m.explorer.typing = true
			m.explorer.pathInput = m.explorer.dir + "/"
		}
	case "x:all":
		return m.pressKey("a")
	case "x:clear":
		return m.pressKey("x")
	case "x:hidden":
		return m.pressKey(".")
	case "x:sort":
		return m.pressKey("s")
	case "x:cancel":
		m.active, m.explorer = chatScreen, nil
	case "x:upload":
		return m.planUpload()
	}
	if n, ok := idNum(id, "chat:"); ok && n >= 0 && n < len(m.conversations) {
		old := m.chatIndex
		m.chatIndex = n
		cmd := m.openChat()
		if m.chatID != m.conversations[n].ID {
			m.chatIndex = old
		}
		return cmd
	}
	if n, ok := idNum(id, "msg:"); ok && n >= 0 && n < len(m.messages) {
		m.active, m.composing = chatScreen, false
		m.messageIndex, m.msgFocus = n, true
		m.fitMessages()
		if double {
			return m.showVersions()
		}
		return nil
	}
	if n, ok := idNum(id, "menu:"); ok {
		return m.runMenu(n)
	}
	if n, ok := idNum(id, "emoji:"); ok {
		return m.insertEmoji(n)
	}
	if n, ok := idNum(id, "set:"); ok {
		m.settingsIndex = n
		return m.activateSetting(n)
	}
	if n, ok := idNum(id, "palette:"); ok {
		matches := m.filteredCommands()
		if n >= 0 && n < len(matches) {
			return m.runCommand(matches[n])
		}
		return nil
	}
	if strings.HasPrefix(id, "slash:") {
		m.composer = "/" + strings.TrimPrefix(id, "slash:") + " "
		m.composing = true
		return nil
	}
	if e := m.explorer; e != nil && m.active == explorerScreen {
		entries := e.view()
		if n, ok := idNum(id, "x:chk:"); ok && n < len(entries) {
			e.cursor = n
			m.setNotice(e.toggle(entries[n]), true)
			return nil
		}
		if n, ok := idNum(id, "x:row:"); ok && n < len(entries) {
			e.cursor = n
			d := entries[n]
			if double || d.name == ".." {
				if d.dir {
					return m.explorerGo(d.path)
				}
				m.setNotice(e.toggle(d), true)
			}
			return nil
		}
	}
	_ = fmt.Sprint
	return nil
}
