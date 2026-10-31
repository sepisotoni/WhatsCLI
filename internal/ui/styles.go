package ui

import "github.com/charmbracelet/lipgloss"

var (
	brandStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#25D366"))
	mutedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#8A9AA5"))
	panelStyle = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#25D366")).Padding(1, 2)
)

func Brand(text string) string { return brandStyle.Render(text) }
func Muted(text string) string { return mutedStyle.Render(text) }

func Panel(title, body string, width int) string {
	content := Brand(title) + "\n\n" + body
	if width < 24 {
		if width < 1 {
			width = 1
		}
		return lipgloss.NewStyle().Width(width).Render(content)
	}
	return panelStyle.Width(width - 6).Render(content)
}
