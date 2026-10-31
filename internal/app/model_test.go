package app

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestWelcomeConsentAndInboxNavigation(t *testing.T) {
	model := New(nil)
	if model.active != welcomeScreen {
		t.Fatalf("initial screen = %v, want welcome", model.active)
	}
	model = model.updateKey(tea.KeyMsg{Type: tea.KeyEnter})
	if model.active != consentScreen {
		t.Fatalf("after welcome enter = %v, want consent", model.active)
	}
	model = model.updateKey(tea.KeyMsg{Type: tea.KeyEnter})
	if model.active != consentScreen {
		t.Fatal("consent screen continued without explicit acceptance")
	}
	model = model.updateKey(tea.KeyMsg{Type: tea.KeySpace})
	model = model.updateKey(tea.KeyMsg{Type: tea.KeyEnter})
	if model.active != pairingScreen {
		t.Fatalf("after accepting consent = %v, want pairing", model.active)
	}
	model = model.updateKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
	if model.active != inboxScreen {
		t.Fatalf("after i = %v, want inbox", model.active)
	}
	model = model.updateKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	if model.active != settingsScreen {
		t.Fatalf("after s = %v, want settings", model.active)
	}
	model = model.updateKey(tea.KeyMsg{Type: tea.KeyEsc})
	if model.active != inboxScreen {
		t.Fatalf("after escape = %v, want inbox", model.active)
	}
}

func TestCommandPaletteFiltersAndRunsSettings(t *testing.T) {
	model := New(nil)
	model.active = inboxScreen
	model = model.updateKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	model = model.updateKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	model = model.updateKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	matches := model.filteredCommands()
	if len(matches) != 1 || matches[0] != "settings" {
		t.Fatalf("filtered commands = %v, want [settings]", matches)
	}
	model = model.updateKey(tea.KeyMsg{Type: tea.KeyEnter})
	if model.active != settingsScreen {
		t.Fatalf("after command = %v, want settings", model.active)
	}
}

func TestQuitKeyReturnsQuitCommand(t *testing.T) {
	model := New(nil)
	_, command := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if command == nil {
		t.Fatal("q did not request application shutdown")
	}
}
