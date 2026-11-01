package ui

import (
	"strings"
	"testing"
)

func TestDoodleOmitsEnvelopeGlyph(t *testing.T) {
	canvas := NewCanvas(300, 100, BG)
	Doodle(canvas, R(0, 0, canvas.W, canvas.H), Ink)
	if strings.Contains(canvas.Plain(), "✉") {
		t.Fatal("wallpaper must not draw envelope glyphs")
	}
}
