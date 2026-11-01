package app

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"whatscli/internal/history"
	"whatscli/internal/messaging"
	"whatscli/internal/ui"
)

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
			m.notice = err.Error()
			return nil
		}
		return m.createChat(jid, strings.TrimSpace(m.newChat))
	default:
		if (key.Type == tea.KeyRunes || key.Type == tea.KeySpace) && len([]rune(m.newChat)) < 40 {
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
		m.notice = "Finish the send or clear the current draft (Ctrl+U in composer) before changing chats."
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
