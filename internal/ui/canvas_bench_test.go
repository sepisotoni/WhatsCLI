package ui

import "testing"

func BenchmarkCanvasStringFullScreen(b *testing.B) {
	c := NewCanvas(200, 60, Panel)
	for y := 0; y < 60; y++ {
		c.Text(2, y, "Umbrella Devs 22:14 ✓✓ The quick brown fox jumps over the lazy dog", Text, 0, y%2 == 0)
	}
	Doodle(c, Rect{X: 0, Y: 0, W: 200, H: 60}, Ink)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = c.String()
	}
}

func BenchmarkWrap(b *testing.B) {
	text := "Your device credentials and cached chat data are stored locally on this computer. The data is not encrypted by WhatsCLI."
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = Wrap(text, 60)
	}
}
