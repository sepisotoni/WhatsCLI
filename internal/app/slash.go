package app

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"whatscli/internal/history"
	"whatscli/internal/messaging"
)

type slashCmd struct{ name, usage, desc string }

var slashCommands = []slashCmd{
	{"help", "/help", "Show keyboard and command help"},
	{"files", "/files", "Open the file browser to upload files or folders"},
	{"new", "/new <number>", "Open a chat with an international phone number"},
	{"meta", "/meta", "Open the Meta AI chat"},
	{"search", "/search <text>", "Filter the chat list"},
	{"archive", "/archive", "Archive or unarchive this chat"},
	{"versions", "/versions", "Show retained versions of the selected message"},
	{"older", "/older", "Load an older page of messages"},
	{"latest", "/latest", "Jump to the newest messages"},
	{"emoji", "/emoji", "Open the emoji picker"},
	{"settings", "/settings", "Open settings"},
	{"connect", "/connect", "Link or reconnect this device"},
	{"qr", "/qr", "Show the pairing QR full screen"},
	{"web", "/web", "Open the local web page for QR pairing"},
	{"mouse", "/mouse", "Toggle mouse support"},
	{"clear", "/clear", "Clear the draft"},
	{"logout", "/logout", "Unlink this device"},
	{"quit", "/quit", "Quit WhatsCLI"},
}

var slashAliases = map[string]string{"upload": "files", "attach": "files", "ai": "meta", "find": "search", "?": "help", "exit": "quit", "q": "quit", "unarchive": "archive"}

// slashSuggestions lists commands matching the typed prefix; it is empty once
// arguments are being typed or the text is not a command.
func (m Model) slashSuggestions() []slashCmd {
	if !m.composing || !strings.HasPrefix(m.composer, "/") || strings.HasPrefix(m.composer, "//") || strings.ContainsAny(m.composer, " \n") {
		return nil
	}
	prefix := strings.ToLower(m.composer[1:])
	var out []slashCmd
	for _, c := range slashCommands {
		if strings.HasPrefix(c.name, prefix) {
			out = append(out, c)
		}
	}
	return out
}

// runSlash executes a "/command" typed in the message box. Nothing is sent to
// WhatsApp; use "//text" to send a message that starts with a slash.
func (m *Model) runSlash() tea.Cmd {
	fields := strings.Fields(m.composer[1:])
	if len(fields) == 0 {
		return nil
	}
	name := strings.ToLower(fields[0])
	if alias, ok := slashAliases[name]; ok {
		name = alias
	}
	args := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(m.composer[1:]), fields[0]))
	known := false
	for _, c := range slashCommands {
		known = known || c.name == name
	}
	if !known {
		m.setNotice("Unknown command /"+fields[0]+". Type / to list commands, or // to send a literal slash.", true)
		return nil
	}
	finish := func() { m.composer, m.composing = "", false }
	switch name {
	case "help":
		finish()
		m.openOverlay(helpScreen)
	case "files":
		finish()
		return m.openExplorer()
	case "new":
		jid, err := messaging.PhoneRecipient(args)
		if err != nil {
			m.setNotice(err.Error(), true)
			return nil
		}
		finish()
		return m.createChat(jid, args)
	case "meta":
		finish()
		return m.openMetaAI()
	case "search":
		finish()
		m.active, m.searching, m.search = inboxScreen, true, args
		m.fitList()
	case "archive":
		finish()
		m.toggleArchive()
	case "versions":
		finish()
		return m.showVersions()
	case "older":
		finish()
		if len(m.messages) > 0 {
			return m.runCommandOlder()
		}
	case "latest":
		finish()
		m.messageBefore, m.msgTop, m.messageIndex = nil, 0, 0
		return m.loadMessages(nil)
	case "emoji":
		m.composer = ""
		m.active = emojiScreen
	case "settings":
		finish()
		m.openOverlay(settingsScreen)
	case "connect":
		finish()
		m.active = pairingScreen
		return m.connect()
	case "qr":
		finish()
		m.active, m.fullscreen = pairingScreen, true
	case "web":
		finish()
		m.active = pairingScreen
		return m.startWeb()
	case "mouse":
		finish()
		return m.activateSetting(0)
	case "clear":
		finish()
	case "logout":
		finish()
		m.active = logoutScreen
	case "quit":
		return tea.Quit
	}
	return nil
}

func (m *Model) runCommandOlder() tea.Cmd {
	last := m.messages[len(m.messages)-1]
	m.messageBefore = &history.Cursor{Time: last.CreatedAt, ID: last.ID}
	return m.loadMessages(m.messageBefore)
}
