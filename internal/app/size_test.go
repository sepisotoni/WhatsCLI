package app

import (
	"strings"
	"testing"
)

// TestFrameSizeStaysReasonableForSSH guards the wire cost of one full redraw and
// of the per-tick change (caret/spinner), which is all a steady-state session sends.
func TestFrameSizeStaysReasonableForSSH(t *testing.T) {
	m := demoFixture(t)
	a := m.View()
	m.frame++
	m.composing = true
	b := m.View()
	changed := 0
	la, lb := strings.Split(a, "\n"), strings.Split(b, "\n")
	for i := range la {
		if la[i] != lb[i] {
			changed += len(lb[i])
		}
	}
	t.Logf("130x42 full frame: %d bytes; caret/spinner tick changes %d bytes", len(a), changed)
	if len(a) > 120_000 {
		t.Fatalf("full frame too large for slow SSH links: %d bytes", len(a))
	}
	if changed > 6_000 {
		t.Fatalf("a steady-state tick resends too much: %d bytes", changed)
	}
}
