package ui

import (
	"strings"
	"testing"
)

func TestSafeTextRemovesTerminalCommands(t *testing.T) {
	input := "Hello\x1b[2J\x1b]52;c;c2VjcmV0\a\r\b世界😀\nnext‮"
	output := SafeText(input)
	if strings.ContainsAny(output, "\x1b\a\r\b‮") || strings.Contains(output, "c2VjcmV0") {
		t.Fatalf("unsafe output %q", output)
	}
	if !strings.Contains(output, "世界😀\nnext") {
		t.Fatalf("lost normal Unicode %q", output)
	}
}

func TestQRDoesNotExposePayloadAsText(t *testing.T) {
	payload := "secret-pairing-fixture-not-real"
	if strings.Contains(QR(payload, 100, 100), payload) {
		t.Fatal("raw pairing material displayed")
	}
	if !strings.Contains(QR(payload, 10, 5), "too small") {
		t.Fatal("QR should never be cropped")
	}
}
