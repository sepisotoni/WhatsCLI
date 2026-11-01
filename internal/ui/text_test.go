package ui

import (
	"strings"
	"testing"
)

func TestSafeTextRemovesTerminalCommands(t *testing.T) {
	input := "Hello\x1b[2J\x1b]52;c;c2VjcmV0\a\r\b世界😀\nnext\u202e"
	output := SafeText(input)
	if strings.ContainsAny(output, "\x1b\a\r\b\u202e") || strings.Contains(output, "c2VjcmV0") {
		t.Fatalf("unsafe output %q", output)
	}
	if !strings.Contains(output, "世界😀\nnext") {
		t.Fatalf("lost normal Unicode %q", output)
	}
}

func TestQRDoesNotExposePayloadAsText(t *testing.T) {
	payload := "2@" + strings.Repeat("synthetic-pairing-fixture-", 7) + ",A,B,C"
	q, err := NewQR(payload)
	if err != nil {
		t.Fatal(err)
	}
	cols, rows := q.Size()
	canvas := NewCanvas(cols, rows, BG)
	q.Draw(canvas, 0, 0)
	if strings.Contains(canvas.Plain(), "synthetic") {
		t.Fatal("raw pairing material displayed")
	}
	// A WhatsApp-sized code must fit comfortably in a 130x42 terminal.
	if cols > 70 || rows > 36 {
		t.Fatalf("QR too large: %dx%d", cols, rows)
	}
}

func TestWrapAndTruncateHandleWideRunes(t *testing.T) {
	for _, line := range Wrap("日本語のテキスト and a verylongwordthatmustbreakacrosslines 😀😀😀", 10) {
		if StringWidth(line) > 10 {
			t.Fatalf("line too wide: %q", line)
		}
	}
	if got := Truncate("日本語日本語", 7, "…"); StringWidth(got) > 7 {
		t.Fatalf("truncate overflow: %q", got)
	}
}

func TestCanvasFillsEveryCellAndSerializesTruecolor(t *testing.T) {
	SetProfile(TrueColor)
	c := NewCanvas(6, 2, Panel)
	c.Text(0, 0, "日a", Text, 0, true)
	out := c.String()
	if !strings.Contains(out, "48;2;4;21;15") || !strings.Contains(out, "38;2;234;246;239") {
		t.Fatalf("missing truecolor fills: %q", out)
	}
	if got := strings.Count(c.Plain(), "\n"); got != 1 {
		t.Fatal("wrong height")
	}
	SetProfile(Color256)
	defer SetProfile(TrueColor)
	if !strings.Contains(c.String(), "48;5;") {
		t.Fatal("256-colour fallback missing")
	}
}

func TestInitialsAndPhoto(t *testing.T) {
	for in, want := range map[string]string{"Umbrella Devs": "UD", "joshua": "J", "27821234567": "#", "  ": "#", "Qhawe (Secondary)": "QS"} {
		if got := Initials(in); got != want {
			t.Fatalf("Initials(%q)=%q want %q", in, got, want)
		}
	}
	if _, err := DecodePhoto([]byte("not an image")); err == nil {
		t.Fatal("decoded garbage")
	}
}

func TestTailFitKeepsCaretSideAndIsLinear(t *testing.T) {
	if got := TailFit("hello world", 5); got != "world" {
		t.Fatal(got)
	}
	if got := TailFit("日本語", 5); StringWidth(got) > 5 || got != "本語" {
		t.Fatal(got)
	}
	long := strings.Repeat("x", 4096)
	if len(TailFit(long, 40)) != 40 {
		t.Fatal("wrong tail length")
	}
}
