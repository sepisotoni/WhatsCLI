package ui

import (
	"strings"
	"unicode"

	"github.com/charmbracelet/x/ansi"
)

// SafeText strips terminal commands, C0/C1 controls and bidi overrides while
// preserving ordinary Unicode (including emoji and joiners) in stored messages.
func SafeText(text string) string {
	return strings.Map(func(r rune) rune {
		if r == '\n' {
			return r
		}
		if r == '\t' {
			return ' '
		}
		if unicode.IsControl(r) || r >= 0x202a && r <= 0x202e || r >= 0x2066 && r <= 0x2069 {
			return -1
		}
		return r
	}, ansi.Strip(text))
}

func SingleLine(text string) string { return strings.ReplaceAll(SafeText(text), "\n", " ") }
