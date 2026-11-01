package app

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"whatscli/internal/history"
	"whatscli/internal/messaging"
	"whatscli/internal/ui"
)

// All hit testing uses these same cell dimensions as the renderer.
func (m Model) geometry() (rail, sidebar, main, height int) {
	rail, sidebar, height = 7, min(38, max(28, m.width/4)), m.height-2
	return rail, sidebar, m.width - rail - sidebar, height
}

func fit(text string, width, height int) string {
	lines := strings.Split(text, "\n")
	if len(lines) > height {
		lines = lines[:height]
	}
	for i, line := range lines {
		lines[i] = ansi.Truncate(line, max(1, width), "…")
	}
	for len(lines) < height {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}

func (m Model) workspaceView() string {
	rail, sidebar, main, height := m.geometry()
	left := []string{"", ui.Brand(" >_"), "", ui.Button(" C ", m.active == inboxScreen), "", ui.Button(" + ", m.hover == "new"), "", ui.Button(" AI", m.hover == "ai")}
	for len(left) < height-5 {
		left = append(left, "")
	}
	left = append(left, ui.Button(" S ", m.hover == "settings"))
	nav := ui.Frame(fit(strings.Join(left, "\n"), rail-2, height-2), rail, height, false)
	list := ui.Frame(m.sidebarView(sidebar-2, height-2), sidebar, height, m.active == inboxScreen)
	conversation := ui.Frame(m.conversationView(main-2, height-2), main, height, m.active == chatScreen)
	footer := " Tab · panes   ↑↓ · navigate   Enter · open   n · new chat   Ctrl+F · search   / · commands"
	if m.active == chatScreen {
		footer = " i · compose   a · attach   Enter · versions   p · older   r · latest   Tab · chats   / · commands"
	}
	if m.composing {
		footer = " Enter · send   Ctrl+J · newline   Ctrl+U · clear   Esc · stop typing   Ctrl+C · quit"
	}
	if m.searching {
		footer = " Type · filter loaded chats   ↑↓ · select   Enter · open   Esc · clear search"
	}
	status := m.connection.Status
	if status == "" {
		status = "Offline · local history"
	}
	if m.demo {
		status = "DEMO · synthetic messages · no WhatsApp connection"
	}
	if m.notice != "" {
		status = m.notice
	}
	if m.connection.Error != "" {
		status = m.connection.Error
	}
	if m.sending && !m.settings.ReducedMotion {
		status = []string{"◐", "◓", "◑", "◒"}[m.frame] + " " + status
	}
	if m.demo && !strings.HasPrefix(status, "DEMO") {
		status = "DEMO · " + status
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, nav, list, conversation) + "\n" +
		ui.Muted(ansi.Truncate(footer, m.width, "…")) + "\n" +
		ui.Brand(" >_ WhatsCLI  ") + ui.Muted(ansi.Truncate(ui.SingleLine(status), max(1, m.width-14), "…"))
}

func (m Model) welcomeView() string {
	width := min(76, m.width-6)
	body := "\n" + ui.Brand("╭──────╮\n│  >_  │\n╰──┬───╯") + "\n\n" +
		ui.Label("Welcome to ") + ui.Brand("WhatsCLI") + "\n\n" +
		ui.Muted("Your conversations. Your terminal.") + "\n\n" +
		ui.Muted(strings.Repeat("─", min(40, width-8))) + "\n\n" +
		ui.Button(" Get started → ", true) + "   " + ui.Button(" q · Exit ", false) +
		"\n\n" + ui.Muted("Enter to continue · / commands") + "\n"
	panel := ui.Frame(lipgloss.PlaceHorizontal(width-2, lipgloss.Center, body), width, min(23, m.height-2), true)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, panel)
}

func (m Model) visibleChats() []int {
	var indices []int
	for i, c := range m.conversations {
		if strings.Contains(strings.ToLower(c.Title+" "+c.ID), strings.ToLower(m.search)) {
			indices = append(indices, i)
		}
	}
	return indices
}

func (m Model) chatWindow(height int) ([]int, int) {
	indices := m.visibleChats()
	selected := 0
	for i, index := range indices {
		if index == m.chatIndex {
			selected = i
		}
	}
	count := max(1, (height-6)/3)
	start := max(0, selected-count+1)
	return indices[start:min(len(indices), start+count)], start
}

func (m Model) sidebarView(width, height int) string {
	title := " " + ui.Label("Chats") + " " + ui.Brand(fmt.Sprint(len(m.conversations)))
	query := " Search chats…"
	if m.search != "" || m.searching {
		query = " / " + ui.SingleLine(m.search)
		if m.searching {
			query += "▏"
		}
	}
	lines := []string{"", title, "", ui.Muted(ansi.Truncate(query, width, "…")), ui.Muted(strings.Repeat("─", width)), ""}
	indices, _ := m.chatWindow(height)
	for _, i := range indices {
		c := m.conversations[i]
		name := ui.SingleLine(c.Title)
		if name == "" {
			name = c.ID
		}
		prefix := "  "
		if strings.HasSuffix(c.ID, "@g.us") {
			prefix = "◎ "
		}
		if i == m.chatIndex {
			prefix = "› "
		}
		stamp := c.UpdatedAt.Local().Format("15:04")
		row := prefix + ansi.Truncate(name, max(1, width-10), "…")
		row += strings.Repeat(" ", max(1, width-ansi.StringWidth(row)-len(stamp)-1)) + stamp
		preview := c.LastBody
		if preview == "" {
			preview = "No messages yet"
		}
		row2 := "  " + ansi.Truncate(ui.SingleLine(preview), width-3, "…")
		style := lipgloss.NewStyle().Width(width).Foreground(ui.Text)
		if i == m.chatIndex || m.hover == fmt.Sprintf("chat:%d", i) {
			style = style.Background(ui.Outgoing).Foreground(ui.Green)
		}
		lines = append(lines, style.Bold(i == m.chatIndex).Render(row), style.Render(row2), "")
	}
	if len(indices) == 0 {
		lines = append(lines, ui.Muted(" No matching chats."), "", ui.Muted(" n · start a conversation"))
	}
	return fit(strings.Join(lines, "\n"), width, height)
}

func (m Model) conversationView(width, height int) string {
	if m.chatID == "" {
		body := ui.Brand(">_") + "\n\n" + ui.Label("Your conversations.\nYour terminal.") +
			"\n\n" + ui.Muted("Select a chat to get started.\n\nn · new conversation\n/ · command palette\n\nIndependent. Keyboard-first. Local.")
		return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, body)
	}
	title := ui.Label(" " + ui.SingleLine(m.chatTitle))
	context := " direct conversation"
	if strings.HasSuffix(m.chatID, "@g.us") {
		context = " group conversation"
	}
	if strings.HasSuffix(m.chatID, "@bot") || m.chatTitle == "Meta AI" {
		context = " Meta AI · availability depends on your account"
	}
	header := title + "\n" + ui.Muted(ansi.Truncate(context, width, "…")) + "\n" + ui.Muted(strings.Repeat("─", width))
	bodyHeight := max(1, height-8)
	transcript := m.transcript(width, bodyHeight)
	draft := "Type a message…"
	if m.composer != "" {
		runes := []rune(ui.SingleLine(m.composer))
		// Keep the caret and most recent input visible, not the start of a long draft.
		for ansi.StringWidth(string(runes)) > max(1, width-15) && len(runes) > 0 {
			runes = runes[1:]
		}
		draft = string(runes)
	}
	if m.composing {
		draft += "▏"
	}
	prompt := ui.Muted(draft)
	if m.composing {
		prompt = ui.Label(draft)
	}
	sendLabel := " › "
	if m.sending {
		sendLabel = " … "
	}
	composer := ui.Muted(strings.Repeat("─", width)) + "\n\n" +
		ui.Button("+", m.hover == "attach") + " " + lipgloss.NewStyle().Width(max(1, width-10)).Render(ansi.Truncate(prompt, max(1, width-10), "…")) +
		" " + ui.Button(sendLabel, m.composing || m.hover == "send") + "\n"
	return fit(header+"\n"+transcript+"\n"+composer, width, height)
}

func (m Model) transcript(width, height int) string {
	if len(m.messages) == 0 {
		return fit("\n "+ui.Muted("No messages on this page.\n Press i to write the first message."), width, height)
	}
	date := m.messages[min(m.messageIndex, len(m.messages)-1)].CreatedAt.Local()
	label := date.Format("02 Jan 2006")
	if date.Format("2006-01-02") == time.Now().Format("2006-01-02") {
		label = "Today"
	}
	dateRow := lipgloss.PlaceHorizontal(width, lipgloss.Center, ui.Button(label, false))
	height = max(1, height-2)
	// Pages arrive newest-first. Include the selected message and fit as many
	// older messages as possible, then display them in chronological order.
	var bubbles []string
	used := 0
	start := max(0, min(m.messageIndex, len(m.messages)-1))
	for i := start; i < len(m.messages); i++ {
		bubble := m.bubble(m.messages[i], width, i == m.messageIndex, height)
		h := lipgloss.Height(bubble) + 1
		if used+h > height && len(bubbles) > 0 {
			break
		}
		bubbles = append([]string{bubble}, bubbles...)
		used += h
		if used >= height {
			break
		}
	}
	body := strings.Join(bubbles, "\n\n")
	lines := strings.Split(body, "\n")
	if len(lines) > height {
		body = strings.Join(lines[len(lines)-height:], "\n")
	}
	return dateRow + "\n\n" + fit(body, width, height)
}

func (m Model) bubble(message history.Message, width int, selected bool, maxHeight int) string {
	bubbleWidth := max(10, min(width-4, width*4/5))
	color, bg := ui.Line, ui.Incoming
	if message.FromMe {
		bg = ui.Outgoing
	}
	if selected && m.active == chatScreen && !m.composing {
		color = ui.Green
	}
	body := ui.SafeText(message.Body)
	if message.Deleted {
		body = "[deleted · retained locally]\n" + body
	}
	naturalWidth := 22
	for _, line := range strings.Split(body, "\n") {
		naturalWidth = max(naturalWidth, ansi.StringWidth(line)+4)
	}
	bubbleWidth = min(bubbleWidth, naturalWidth)
	sender := ui.SingleLine(message.Sender)
	if message.FromMe {
		sender = "You"
	} else if messaging.IsMetaAI(message.Sender) {
		sender = "Meta AI"
	} else {
		for _, chat := range m.conversations {
			if chat.ID == message.Sender && chat.Title != "" {
				sender = ui.SingleLine(chat.Title)
				break
			}
		}
		sender = strings.Split(sender, "@")[0]
	}
	wrapped := strings.Split(ansi.Hardwrap(body, bubbleWidth-4, true), "\n")
	if len(wrapped) > max(1, maxHeight-5) {
		wrapped = wrapped[:max(1, maxHeight-5)]
		wrapped[len(wrapped)-1] = ansi.Truncate(wrapped[len(wrapped)-1], bubbleWidth-5, "") + "…"
	}
	stamp := message.CreatedAt.Local().Format("15:04")
	if message.Edited {
		stamp += " · edited"
	}
	if message.FromMe {
		switch message.Status {
		case "read":
			stamp += " · ✓✓ read"
		case "delivered":
			stamp += " · ✓✓"
		case "sent":
			stamp += " · ✓"
		case "pending":
			stamp += " · pending"
		default:
			stamp += " · " + message.Status
		}
	}
	content := ui.Brand(ansi.Truncate(sender, bubbleWidth-4, "…")) + "\n" + strings.Join(wrapped, "\n") + "\n" + ui.Muted(stamp)
	bubble := lipgloss.NewStyle().Foreground(ui.Text).Background(bg).Border(lipgloss.RoundedBorder()).
		BorderForeground(color).Padding(0, 1).Width(bubbleWidth - 2).Render(content)
	if message.FromMe {
		return lipgloss.PlaceHorizontal(width, lipgloss.Right, bubble)
	}
	return lipgloss.NewStyle().PaddingLeft(1).Render(bubble)
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
}

func (m *Model) openChat() tea.Cmd {
	indices := m.visibleChats()
	if len(indices) == 0 || m.chatIndex >= len(m.conversations) {
		return nil
	}
	c := m.conversations[m.chatIndex]
	if c.ID != m.chatID && (m.sending || m.composer != "") {
		m.notice = "Finish the send or clear the current draft (Ctrl+U in composer) before changing chats."
		return nil
	}
	m.searching = false
	m.active = chatScreen
	if m.chatID == c.ID {
		return nil
	}
	m.chatID, m.chatTitle, m.messages = c.ID, c.Title, nil
	m.messageBefore, m.messageIndex = nil, 0
	return m.loadMessages(nil)
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
		if (key.Type == tea.KeyRunes || key.Type == tea.KeySpace) && len([]rune(m.search)) < 80 {
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
	return nil
}

func (m *Model) mouse(msg tea.MouseMsg) tea.Cmd {
	if !m.settings.Mouse || m.active != inboxScreen && m.active != chatScreen || m.width < 90 || m.height < 20 {
		return nil
	}
	rail, sidebar, _, height := m.geometry()
	target := ""
	if msg.X < rail {
		switch {
		case msg.Y == 4:
			target = "chats"
		case msg.Y == 6:
			target = "new"
		case msg.Y == 8:
			target = "ai"
		case msg.Y == height-4:
			target = "settings"
		}
	} else if msg.X < rail+sidebar {
		if msg.Y == 4 {
			target = "search"
		} else if msg.Y >= 7 {
			indices, _ := m.chatWindow(height - 2)
			row := (msg.Y - 7) / 3
			if row >= 0 && row < len(indices) {
				target = fmt.Sprintf("chat:%d", indices[row])
			}
		}
	} else if msg.Y >= height-4 && m.chatID != "" {
		target = "compose"
		if msg.X < rail+sidebar+5 {
			target = "attach"
		}
		if msg.X >= m.width-8 {
			target = "send"
		}
	}
	m.hover = target
	if msg.Button == tea.MouseButtonWheelUp || msg.Button == tea.MouseButtonWheelDown {
		delta := 1
		if msg.Button == tea.MouseButtonWheelUp {
			delta = -1
		}
		if msg.X < rail+sidebar {
			m.moveChat(delta)
		} else if !m.composing {
			m.messageIndex = min(max(0, len(m.messages)-1), max(0, m.messageIndex-delta))
		}
		return nil
	}
	if msg.Action != tea.MouseActionPress || msg.Button != tea.MouseButtonLeft {
		return nil
	}
	switch {
	case strings.HasPrefix(target, "chat:"):
		var index int
		fmt.Sscanf(target, "chat:%d", &index)
		old := m.chatIndex
		m.chatIndex = index
		cmd := m.openChat()
		if m.active != chatScreen || m.chatID != m.conversations[index].ID {
			m.chatIndex = old
		}
		return cmd
	case target == "chats":
		m.active, m.composing = inboxScreen, false
	case target == "search":
		m.active, m.searching, m.composing = inboxScreen, true, false
	case target == "settings":
		m.composing = false
		m.openOverlay(settingsScreen)
	case target == "new":
		m.composing = false
		m.openOverlay(newChatScreen)
		m.newChat = ""
	case target == "ai":
		return m.openMetaAI()
	case target == "compose":
		m.active, m.composing = chatScreen, !m.sending
	case target == "attach":
		if !m.sending {
			m.active, m.composing, m.path = attachmentScreen, false, ""
		}
	case target == "send":
		if !m.sending && strings.TrimSpace(m.composer) != "" {
			return m.send(m.composer, "")
		}
	}
	return nil
}

type pulseMsg time.Time

func pulse() tea.Cmd {
	return tea.Tick(350*time.Millisecond, func(t time.Time) tea.Msg { return pulseMsg(t) })
}

func (m Model) pairingView() string {
	leftWidth := min(44, m.width/3)
	rightWidth := m.width - leftWidth - 6
	instructions := ui.Brand("LINK A DEVICE") + "\n" + ui.Muted("Log in to WhatsApp") + "\n\n" +
		ui.Brand("01") + "  Open WhatsApp on your phone.\n\n" +
		ui.Brand("02") + "  Settings → Linked devices.\n\n" +
		ui.Brand("03") + "  Choose Link a device.\n\n" +
		ui.Brand("04") + "  Scan the QR and keep this\n    terminal open until linked.\n\n" +
		ui.Muted("The code refreshes automatically.\nNever share or log it.")
	qr := ui.Muted("Waiting for a pairing code…")
	if m.connection.QR != "" {
		qr = ui.QR(m.connection.QR, rightWidth, m.height-7)
	} else if !m.settings.ReducedMotion {
		qr = ui.Brand([]string{"◐", "◓", "◑", "◒"}[m.frame]) + " " + qr
	}
	bodyHeight := m.height - 6
	body := lipgloss.JoinHorizontal(lipgloss.Top,
		lipgloss.NewStyle().Width(leftWidth).Render(instructions),
		lipgloss.Place(rightWidth, bodyHeight, lipgloss.Center, lipgloss.Center, qr))
	footer := ui.Muted("f · fullscreen QR   r · retry   i · history   q · cancel and quit")
	if m.notice != "" {
		footer = ui.Muted(ansi.Truncate(ui.SingleLine(m.notice), m.width-4, "…"))
	}
	return ui.Frame(body+"\n"+footer, m.width, m.height, true)
}
