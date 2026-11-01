package app

import (
	"context"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"whatscli/internal/ui"
	"whatscli/internal/updater"
)

func (m *Model) openUpdate() tea.Cmd {
	if !m.settings.Accepted() || m.onboardingPending {
		m.setNotice("Accept the agreement before using the update checker.", true)
		return nil
	}
	m.openOverlay(updateScreen)
	m.updateVersion = updater.Version{}
	m.updateNotice = ""
	if m.demo {
		m.updateNotice = "Update checks are disabled in demo mode."
		return nil
	}
	m.updateChecking = true
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
		defer cancel()
		version, err := updater.Check(ctx, updater.BuildVersion)
		return updateCheckMsg{version: version, err: err}
	}
}

func (m *Model) updateKey(key tea.KeyMsg) tea.Cmd {
	switch key.String() {
	case "esc":
		m.updateChecking, m.updateRunning = false, false
		m.leaveOverlay()
	case "enter", "c":
		if !m.updateChecking && !m.updateRunning {
			return m.openUpdate()
		}
	case "u":
		return m.runUpdate()
	}
	return nil
}

func (m *Model) runUpdate() tea.Cmd {
	if m.demo {
		m.updateNotice = "Updates are disabled in demo mode."
		return nil
	}
	if m.updateChecking || m.updateRunning || !m.updateVersion.Available {
		return nil
	}
	if _, err := updater.ScriptPath(); err != nil {
		m.updateNotice = err.Error()
		return nil
	}
	m.updateRunning = true
	m.updateNotice = "Updating from GitHub…"
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		output, err := updater.Run(ctx)
		return updateRunMsg{output: output, err: err}
	}
}

func (m Model) updateAvailable() bool {
	return !m.demo && !m.updateRunning && !m.updateChecking &&
		m.updateVersion.Available
}

func cleanUpdateText(text string) string {
	return strings.TrimSpace(ui.SingleLine(text))
}
