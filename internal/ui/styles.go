package ui

import "github.com/charmbracelet/lipgloss"

var (
	Green      = lipgloss.Color("#00EF9B")
	Background = lipgloss.Color("#00110C")
	Surface    = lipgloss.Color("#071D16")
	Incoming   = lipgloss.Color("#172B25")
	Outgoing   = lipgloss.Color("#00432B")
	Text       = lipgloss.Color("#E4F4EC")
	Dim        = lipgloss.Color("#7F9F90")
	Line       = lipgloss.Color("#15513A")
	brandStyle = lipgloss.NewStyle().Bold(true).Foreground(Green)
	mutedStyle = lipgloss.NewStyle().Foreground(Dim)
	panelStyle = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(Green).Padding(1, 2)
)

func Brand(text string) string { return brandStyle.Render(text) }
func Muted(text string) string { return mutedStyle.Render(text) }

// Frame dimensions include the border, making adjacent terminal panes predictable.
func Frame(body string, width, height int, focused bool) string {
	color := Line
	if focused {
		color = Green
	}
	return lipgloss.NewStyle().Foreground(Text).Background(Background).
		Border(lipgloss.RoundedBorder()).BorderForeground(color).
		Width(max(1, width-2)).Height(max(1, height-2)).
		MaxWidth(width).MaxHeight(height).Render(body)
}

func Label(text string) string {
	return lipgloss.NewStyle().Foreground(Text).Bold(true).Render(text)
}

func Button(text string, active bool) string {
	style := lipgloss.NewStyle().Padding(0, 1).Foreground(Green).Background(Surface)
	if active {
		style = style.Foreground(Background).Background(Green).Bold(true)
	}
	return style.Render(text)
}

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
