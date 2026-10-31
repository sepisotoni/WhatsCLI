package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	qrcode "github.com/skip2/go-qrcode"
)

// QR uses explicit black-on-white cells, including the encoder's quiet zone.
// It never scales or truncates the matrix; a too-small terminal gets guidance.
func QR(code string, width, height int) string {
	qr, err := qrcode.New(code, qrcode.Medium)
	if err != nil {
		return "Could not render pairing code. Retry linking."
	}
	bits := qr.Bitmap()
	if len(bits) > width || (len(bits)+1)/2 > height {
		return "Terminal too small for QR. Enlarge it or press f for fullscreen."
	}
	var out strings.Builder
	for y := 0; y < len(bits); y += 2 {
		for x := range bits[y] {
			top := bits[y][x]
			bottom := y+1 < len(bits) && bits[y+1][x]
			switch {
			case top && bottom:
				out.WriteRune('█')
			case top:
				out.WriteRune('▀')
			case bottom:
				out.WriteRune('▄')
			default:
				out.WriteByte(' ')
			}
		}
		if y+2 < len(bits) {
			out.WriteByte('\n')
		}
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color("#000000")).Background(lipgloss.Color("#FFFFFF")).Render(out.String())
}
