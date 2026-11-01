package app

import (
	"strconv"

	"whatscli/internal/ui"
)

type hit struct {
	r  ui.Rect
	id string
}

// scene couples the canvas with hit regions. With a nil canvas only layout and
// hit regions are produced, so mouse handling uses exactly what is drawn.
type scene struct {
	c    *ui.Canvas
	hits []hit
	off  bool
	m    *Model
}

func (s *scene) add(r ui.Rect, id string) {
	if !s.off {
		s.hits = append(s.hits, hit{r, id})
	}
}
func (s *scene) is(id string) bool { return s.m.hover == id }

func (m Model) View() string {
	if m.width < 30 || m.height < 10 {
		return "WhatsCLI\nEnlarge terminal to at least 30×10.\nCtrl+C quits."
	}
	c := ui.NewCanvas(m.width, m.height, ui.BG)
	s := &scene{c: c, m: &m}
	m.draw(s)
	return c.String()
}

// hitAt returns the topmost interactive region under a cell.
func (m Model) hitAt(x, y int) string {
	s := &scene{m: &m}
	m.draw(s)
	for i := len(s.hits) - 1; i >= 0; i-- {
		if s.hits[i].r.Contains(x, y) {
			return s.hits[i].id
		}
	}
	return ""
}

func (m Model) isOverlay() bool {
	switch m.active {
	case settingsScreen, helpScreen, commandScreen, revisionsScreen, attachmentScreen, confirmFileScreen,
		logoutScreen, newChatScreen, explorerScreen, emojiScreen, menuScreen, statusesScreen, statusComposeScreen, statusConfirmScreen, updateScreen:
		return true
	}
	return false
}

func (m Model) baseScreen() screen {
	if n := len(m.trail); n > 0 {
		switch m.trail[n-1] {
		case welcomeScreen, consentScreen, pairingScreen:
			return m.trail[n-1]
		}
	}
	return inboxScreen
}

func (m Model) draw(s *scene) {
	switch m.active {
	case welcomeScreen:
		s.welcome()
	case consentScreen:
		s.consent()
	case pairingScreen:
		s.pairing()
	default:
		overlay := m.isOverlay()
		prev := s.off
		s.off = s.off || overlay
		switch base := m.baseScreen(); base {
		case welcomeScreen:
			s.welcome()
		case consentScreen:
			s.consent()
		case pairingScreen:
			s.pairing()
		default:
			s.workspace()
		}
		s.off = prev
		if overlay {
			s.overlay()
		}
	}
	if m.notice != "" && (m.active == welcomeScreen || m.active == consentScreen || m.active == pairingScreen || m.isOverlay()) {
		s.toast()
	}
}

func (s *scene) toast() {
	m := s.m
	text := ui.SingleLine(m.notice)
	w := min(m.width-4, ui.StringWidth(text)+4)
	r := ui.R((m.width-w)/2, m.height-3, w, 1)
	bg := ui.Hex(0x0B3A28)
	if m.noticeErr {
		bg = ui.Hex(0x4A1A1A)
	}
	s.c.Fill(r, bg)
	fg := ui.Text
	s.c.Text(r.X+2, r.Y, ui.Truncate(text, w-4, "…"), fg, bg, false)
}

// ---- shared widgets ----

func (s *scene) center(r ui.Rect, text string, fg, bg ui.Color, bold bool) {
	w := ui.StringWidth(text)
	s.c.Text(r.X+max(0, (r.W-w)/2), r.Y+r.H/2, ui.Truncate(text, r.W, "…"), fg, bg, bold)
}

func (s *scene) primary(r ui.Rect, label, id string, enabled bool) {
	bg, fg := ui.Accent, ui.AccentDark
	switch {
	case !enabled:
		bg, fg = ui.Hex(0x0E3B2A), ui.Dim
	case s.is(id):
		bg = ui.AccentHi
	}
	s.c.Fill(r, bg)
	s.center(r, label, fg, bg, true)
	s.add(r, id)
}

func (s *scene) outline(r ui.Rect, label, id string, focused bool) {
	border, fg := ui.Gray, ui.Text
	if s.is(id) || focused {
		border, fg = ui.Accent, ui.Accent
	}
	s.c.Box(r, border, ui.Panel)
	s.center(r, label, fg, ui.Panel, true)
	s.add(r, id)
}

func (s *scene) pill(x, y int, text string, fg, bg ui.Color) int {
	s.c.Fill(ui.R(x, y, ui.StringWidth(text)+2, 1), bg)
	s.c.Text(x+1, y, text, fg, bg, true)
	return ui.StringWidth(text) + 2
}

// ---- welcome ----

func (s *scene) welcome() {
	m, c := s.m, s.c
	W, H := m.width, m.height
	frame := ui.R(0, 0, W, H)
	c.Fill(frame, ui.BG)
	cardW := min(78, W-8)
	if W < 52 {
		cardW = W - 2
	}
	cardH := min(24, H-4)
	if H < 22 {
		cardH = H - 2
	}
	card := ui.R((W-cardW)/2, (H-cardH)/2, cardW, cardH)
	c.Box(ui.R(card.X-1, card.Y-1, card.W+2, card.H+2), ui.Mix(ui.Accent, ui.Panel, .65), ui.BG)
	c.Box(card, ui.BorderHi, ui.Panel)

	micro := card.H < 11
	compact := micro || card.H < 20 || card.W < 54
	cx := card.X + card.W/2
	iconY := card.Y + 3
	titleY := card.Y + 9
	buttonY := card.Y + 16
	buttonH := 3
	if micro {
		titleY = card.Y + 1
		buttonY = card.Bottom() - 2
		buttonH = 1
	} else if compact {
		iconY = card.Y + 1
		titleY = card.Y + 6
		buttonY = card.Bottom() - 2
		buttonH = 1
	}
	if !micro {
		s.chatIcon(cx-5, iconY)
	}
	title := "Welcome to WhatsCLI"
	tx := cx - ui.StringWidth(title)/2
	c.Text(tx, titleY, "Welcome to ", ui.Hex(0xCDF7E3), ui.Panel, true)
	c.Text(tx+11, titleY, "WhatsCLI", ui.Accent, ui.Panel, true)
	sub := "Your conversations. Your terminal."
	subY := titleY + 2
	if micro {
		sub = ""
	} else if compact {
		sub = "Your conversations. Your terminal."
		if card.W < ui.StringWidth(sub)+4 {
			sub = "Your terminal. Your chats."
		}
		subY = titleY + 1
	} else {
		c.HLine(card.X+8, titleY+4, card.W-16, '─', ui.Border)
	}
	if sub != "" {
		c.Text(cx-ui.StringWidth(sub)/2, subY, ui.Truncate(sub, card.W-8, "…"), ui.Dim, ui.Panel, false)
	}

	gap, buttonW := 4, min(24, (card.W-12)/2)
	if compact {
		gap, buttonW = 2, (card.W-6)/2
	}
	groupW := buttonW*2 + gap
	left := ui.R(cx-groupW/2, buttonY, buttonW, buttonH)
	right := ui.R(left.Right()+gap, buttonY, buttonW, buttonH)
	s.outline(left, "Exit", "welcome:exit", m.welcomeFocus == 0)
	continueLabel := "Continue  →"
	if compact {
		continueLabel = "Continue"
	}
	s.primary(right, continueLabel, "welcome:start", true)
	c.Box(right, ui.Accent, ui.Color(0))
	if !compact {
		hint := "Enter/Space · select    ↑↓/Tab · choose    q · quit"
		c.Text(cx-ui.StringWidth(hint)/2, card.Bottom()-2, ui.Truncate(hint, card.W-6, "…"), ui.Faint, ui.Panel, false)
	}
}

// ---- disclaimer & consent ----

var consentItems = []struct{ title, body string }{
	{"Independent and unofficial", "WhatsCLI is an independent, unofficial client. It is not created, sponsored, or endorsed by WhatsApp or Meta. The WhatsApp name and brand belong to their respective owners."},
	{"Your account, your responsibility", "Connecting your account is at your own risk. We are not responsible for any limitations, suspensions, or other consequences that may result from using an unofficial client."},
	{"Local data storage", "Your device credentials and cached chat data are stored locally on this computer. The data is not encrypted by WhatsCLI. Please take care to keep your device secure. Cached data may remain on your device until you manually delete it."},
	{"Early software", "WhatsCLI is in early development. Features may be incomplete and may change. Use a test account first and do not rely on it for critical communication."},
}

func consentHeight(iw int) int {
	total := 0
	for i, it := range consentItems {
		lines := len(ui.Wrap(it.body, iw-7))
		total += max(3, 1+lines)
		if i < len(consentItems)-1 {
			total++
		}
	}
	return total
}

func (s *scene) consent() {
	m, c := s.m, s.c
	W, H := m.width, m.height
	if m.settings.Wallpaper() {
		ui.Doodle(c, ui.R(0, 0, W, H), ui.Mix(ui.Ink, ui.BG, .4))
	}
	pw, ph := min(W-2, 92), min(H-2, 40)
	px, py := (W-pw)/2, (H-ph)/2
	if pw+2 <= W && ph+2 <= H {
		c.Box(ui.R(px-1, py-1, pw+2, ph+2), ui.Border, 0)
	}
	c.Box(ui.R(px, py, pw, ph), ui.BorderHi, ui.Panel)
	ix, iw := px+3, pw-6
	roomy := ph >= 30
	c.Text(ix, py+1, "BEFORE YOU CONNECT", ui.Dim, ui.Panel, false)
	step := "STEP 01 / 02"
	c.Text(ix+iw-ui.StringWidth(step), py+1, step, ui.Accent, ui.Panel, true)
	c.Text(ix, py+2, "Disclaimer", ui.Accent, ui.Panel, true)
	c.Text(ix+11, py+2, "& consent", ui.Text, ui.Panel, true)
	c.Text(ix+iw-11, py+2, "▬▬▬▬▬", ui.Accent, ui.Panel, true)
	c.Text(ix+iw-5, py+2, "▬▬▬▬▬", ui.Faint, ui.Panel, true)
	c.Text(ix, py+3, ui.Truncate("A few important things to know about WhatsCLI before you continue.", iw, "…"), ui.Dim, ui.Panel, false)
	c.HLine(ix, py+4, iw, '─', ui.Border)

	var bottom int
	if roomy {
		bottom = 11
	} else {
		bottom = 4
	}
	view := ui.R(ix, py+5, iw, ph-5-bottom)
	content := consentHeight(iw)
	maxScroll := max(0, content-view.H)
	off := min(m.scroll, maxScroll)
	restore := c.Clip(view)
	y := view.Y - off
	for i, it := range consentItems {
		lines := ui.Wrap(it.body, iw-7)
		h := max(3, 1+len(lines))
		c.Box(ui.R(ix, y, 5, 3), ui.Border, ui.PanelHi)
		c.Text(ix+1, y+1, "0"+string(rune('1'+i)), ui.Accent, ui.PanelHi, true)
		c.Text(ix+7, y, it.title, ui.Accent, ui.Panel, true)
		for j, line := range lines {
			c.Text(ix+7, y+1+j, line, ui.Body, ui.Panel, false)
		}
		y += h
		if i < len(consentItems)-1 {
			c.HLine(ix+7, y, iw-7, '─', ui.Mix(ui.Border, ui.Panel, .5))
			y++
		}
	}
	c.Restore(restore)
	if maxScroll > off {
		c.Text(ix+iw-1, view.Bottom()-1, "▼", ui.Accent, ui.Panel, true)
	}
	if off > 0 {
		c.Text(ix+iw-1, view.Y, "▲", ui.Accent, ui.Panel, true)
	}
	by := py + ph - 2
	boxY := by
	if roomy {
		by = py + ph - 4
		boxY = by - 4
		ty := boxY - 2
		c.HLine(ix, ty-1, iw, '─', ui.Border)
		c.Text(ix, ty, "▤", ui.Accent, ui.Panel, false)
		c.Text(ix+3, ty, "Read more ·", ui.Dim, ui.Panel, false)
		c.Text(ix+15, ty, "WhatsApp Terms of Service ↗", ui.Accent, ui.Panel, true)
		s.add(ui.R(ix, ty, 44, 1), "consent:terms")
		cb := ui.R(ix, boxY, iw, 3)
		fill := ui.Hex(0x061C14)
		bd := ui.Border
		if s.is("consent:check") {
			bd = ui.Accent
		}
		c.Box(cb, bd, fill)
		s.check(cb.X+2, cb.Y+1, m.consentAccepted, fill)
		agreeText := "I understand that this is an unofficial application and accept the risks above."
		if ui.StringWidth(agreeText) > iw-9 {
			agreeText = "I understand and accept the risks above."
		}
		c.Text(cb.X+7, cb.Y+1, ui.Truncate(agreeText, iw-9, "…"), ui.Text, fill, false)
		s.add(cb, "consent:check")
		s.buttons(ix, by, iw, 3)
	} else {
		cy := py + ph - 3
		s.check(ix, cy, m.consentAccepted, ui.Panel)
		c.Text(ix+4, cy, ui.Truncate("I understand and accept the risks above.", iw-4, "…"), ui.Text, ui.Panel, false)
		s.add(ui.R(ix, cy, iw, 1), "consent:check")
		s.buttons(ix, py+ph-2, iw, 1)
	}
	hint := "Space · accept   Enter · agree & continue   ↑↓ · scroll   t · terms   Esc · back"
	if H > ph+2 {
		c.Text((W-ui.StringWidth(hint))/2, py+ph+1, ui.Truncate(hint, W-2, "…"), ui.Faint, ui.BG, false)
	}
}

func (s *scene) check(x, y int, on bool, bg ui.Color) {
	if on {
		s.c.Text(x, y, " ✓ ", ui.AccentDark, ui.Accent, true)
		return
	}
	s.c.Text(x, y, "[ ]", ui.Dim, bg, false)
}

func (s *scene) buttons(x, y, w, h int) {
	back := ui.R(x, y, 16, h)
	agree := ui.R(x+w-26, y, 26, h)
	if h == 1 {
		s.c.Fill(back, ui.PanelHi)
		s.center(back, "← Back", ui.Text, ui.PanelHi, true)
		s.add(back, "consent:back")
		s.primary(agree, "Agree & continue →", "consent:agree", s.m.consentAccepted)
		return
	}
	s.outline(back, "←  Back", "consent:back", false)
	s.primary(agree, "Agree & continue  →", "consent:agree", s.m.consentAccepted)
}

// ---- link a device ----

func (m Model) qrCode() *ui.QRCode {
	if m.connection.QR == "" {
		return nil
	}
	q, err := ui.NewQR(m.connection.QR)
	if err != nil {
		return nil
	}
	return q
}

var spinner = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

func (s *scene) pairing() {
	m, c := s.m, s.c
	W, H := m.width, m.height
	frame := ui.R(0, 0, W, H)
	c.Box(frame, ui.Border, ui.Panel)
	q := m.qrCode()
	var qw, qh int
	if q != nil {
		qw, qh = q.Size()
	}
	barY := H - 5
	if H < 24 {
		barY = H - 3
	}
	content := ui.R(1, 1, W-2, barY-1)
	lw := 46
	if W < 120 {
		lw = W * 40 / 100
	}
	rightX := lw + 6
	rightW := W - rightX - 2
	// The QR beside the instructions needs the frame (+glow) around it.
	fitsSplit := q != nil && qw+4 <= rightW && qh+2 <= content.H
	fitsFull := q != nil && qw+2 <= W-2 && qh+4 <= H-3
	full := q != nil && (m.fullscreen || !fitsSplit) && fitsFull

	// bottom bar
	c.HLine(1, barY, W-2, '─', ui.Border)
	if full {
		s.qrFull(q, qw, qh)
	} else if q != nil && !fitsSplit {
		msg := "Terminal too small to show the QR."
		c.Text((W-ui.StringWidth(msg))/2, H/2-1, msg, ui.Danger, ui.Panel, true)
		need := "Enlarge to at least " + itoa(qw+2) + "×" + itoa(qh+6) + ", or press h for web pairing."
		c.Text(max(1, (W-ui.StringWidth(need))/2), H/2+1, ui.Truncate(need, W-2, "…"), ui.Dim, ui.Panel, false)
	} else {
		s.pairLeft(content, lw)
		right := ui.R(rightX, 1, rightW, barY-1)
		inner := right
		// The panel border is dropped only when the QR needs those two rows.
		if q == nil || (qw+6 <= rightW && qh+4 <= right.H) {
			c.Box(right, ui.Border, ui.Panel)
			inner = right.Inset(1)
		}
		if m.settings.Wallpaper() {
			ui.Doodle(c, inner, ui.Ink)
		}
		s.pairRight(inner, q, qw, qh)
	}
	bh := 3
	by := barY + 1
	if H < 24 {
		bh = 1
		by = barY + 1
	}
	back := ui.R(3, by, 14, bh)
	if bh == 1 {
		c.Fill(back, ui.PanelHi)
		s.center(back, "← Back", ui.Text, ui.PanelHi, true)
		s.add(back, "pair:back")
	} else {
		s.outline(back, "←  Back", "pair:back", false)
	}
	x := back.Right() + 2
	if m.helpAvailable() || m.web != nil {
		label := "Need help?  Open web QR"
		wr := ui.R(x, by, ui.StringWidth(label)+6, bh)
		if m.web != nil {
			label = "Web QR is open"
		}
		if bh == 1 {
			c.Fill(wr, ui.PanelHi)
			s.center(wr, label, ui.Accent, ui.PanelHi, true)
			s.add(wr, "pair:help")
		} else {
			s.outline(wr, label, "pair:help", false)
		}
	}
	label := "▣  Full-screen QR"
	if full && m.fullscreen {
		label = "▣  Exit full screen"
	}
	fr := ui.R(W-3-24, by, 24, bh)
	s.primary(fr, label, "pair:full", q != nil)
}

func itoa(n int) string {
	return strconv.Itoa(n)
}

func (s *scene) pairLeft(content ui.Rect, lw int) {
	m, c := s.m, s.c
	x := 4
	y := 2
	c.Box(ui.R(x, y, 7, 3), ui.Border, ui.PanelHi)
	c.Text(x+3, y+1, "∞", ui.Accent, ui.PanelHi, true)
	c.Text(x+9, y, "LINK", ui.Text, ui.Panel, true)
	c.Text(x+14, y, "A DEVICE", ui.Accent, ui.Panel, true)
	c.Text(x+9, y+1, "Log in to WhatsApp", ui.Dim, ui.Panel, false)
	steps := []string{
		"On your phone, open WhatsApp.",
		"Go to Settings or ⋮ → Linked devices.",
		"Select Link a device and scan the QR.",
		"Keep your terminal open until linked.",
	}
	roomy := content.H >= 24
	pitch := 2
	if roomy {
		pitch = 4
	}
	sy := y + 5
	for i, text := range steps {
		if roomy {
			c.Box(ui.R(x, sy, 7, 3), ui.Border, ui.PanelHi)
			c.Text(x+3, sy+1, string(rune('1'+i)), ui.Accent, ui.PanelHi, true)
			c.Text(x+9, sy+1, ui.Truncate(text, lw-9, "…"), ui.Text, ui.Panel, false)
		} else {
			c.Text(x, sy, string(rune('1'+i)), ui.Accent, ui.Panel, true)
			c.Text(x+3, sy, ui.Truncate(text, lw-3, "…"), ui.Text, ui.Panel, false)
		}
		sy += pitch
	}
	if roomy && sy+4 <= content.Bottom() {
		info := ui.R(x, sy+1, lw-2, 4)
		c.Box(info, ui.Border, ui.Hex(0x061C14))
		c.Text(info.X+3, info.Y+1, "(i)", ui.Accent, ui.Hex(0x061C14), false)
		c.Text(info.X+9, info.Y+1, "The code refreshes automatically.", ui.Dim, ui.Hex(0x061C14), false)
		c.Text(info.X+9, info.Y+2, "Never share it.", ui.Dim, ui.Hex(0x061C14), false)
		sy += 5
	}
	if m.webNote != "" && sy+3 < content.Bottom() {
		note := ui.Wrap("Web QR: "+m.webNote, lw-2)
		for i, line := range note {
			if sy+1+i >= content.Bottom() {
				break
			}
			c.Text(x, sy+1+i, line, ui.Accent, ui.Panel, false)
		}
	}
}

func (s *scene) pairRight(right ui.Rect, q *ui.QRCode, qw, qh int) {
	m, c := s.m, s.c
	if q == nil {
		status := m.connection.Status
		if status == "" {
			status = "Connecting…"
		}
		text := spinner[m.frame%len(spinner)] + "  " + status
		if m.settings.ReducedMotion {
			text = status
		}
		c.Text(right.X+max(0, (right.W-ui.StringWidth(text))/2), right.Y+right.H/2, ui.Truncate(text, right.W, "…"), ui.Accent, 0, true)
		hint := "Waiting for a pairing code · r retry"
		c.Text(right.X+max(0, (right.W-ui.StringWidth(hint))/2), right.Y+right.H/2+2, ui.Truncate(hint, right.W, "…"), ui.Dim, 0, false)
		return
	}
	pillRoom := right.H >= qh+2+4
	total := qh + 2
	if pillRoom {
		total += 4
	}
	fy := right.Y + max(0, (right.H-total)/2)
	fx := right.X + (right.W-(qw+4))/2
	frame := ui.R(fx, fy, qw+4, qh+2)
	glow := ui.R(fx-1, fy-1, qw+6, qh+4)
	if glow.X >= right.X && glow.Y >= right.Y && glow.Bottom() <= right.Bottom() && glow.Right() <= right.Right() {
		c.Box(glow, ui.Mix(ui.Accent, ui.Panel, .75), 0)
	}
	c.Box(frame, ui.Mix(ui.Accent, ui.Panel, .45), ui.Panel)
	q.Draw(c, fx+2, fy+1)
	for _, p := range []struct {
		x, y int
		r    rune
	}{{frame.X, frame.Y, '┏'}, {frame.Right() - 1, frame.Y, '┓'}, {frame.X, frame.Bottom() - 1, '┗'}, {frame.Right() - 1, frame.Bottom() - 1, '┛'}} {
		c.Put(p.x, p.y, p.r, ui.Accent, ui.Panel, true)
	}
	s.add(frame, "pair:full")
	if pillRoom {
		pw := 26
		pr := ui.R(right.X+(right.W-pw)/2, frame.Bottom()+1, pw, 3)
		c.Box(pr, ui.Border, ui.Hex(0x061C14))
		c.Text(pr.X+3, pr.Y+1, "▯", ui.Accent, ui.Hex(0x061C14), true)
		c.Text(pr.X+6, pr.Y+1, "Scan with WhatsApp", ui.Accent, ui.Hex(0x061C14), false)
	} else {
		label := " ▯ Scan with WhatsApp "
		c.Text(frame.X+(frame.W-ui.StringWidth(label))/2, frame.Bottom()-1, label, ui.Accent, ui.Panel, true)
	}
}

func (s *scene) qrFull(q *ui.QRCode, qw, qh int) {
	m, c := s.m, s.c
	W, H := m.width, m.height
	if m.settings.Wallpaper() {
		ui.Doodle(c, ui.R(1, 1, W-2, H-7), ui.Ink)
	}
	frame := ui.R((W-qw-4)/2, max(1, (H-6-qh-2)/2+1), qw+4, qh+2)
	c.Box(ui.R(frame.X-1, frame.Y-1, frame.W+2, frame.H+2), ui.Mix(ui.Accent, ui.Panel, .75), 0)
	c.Box(frame, ui.Mix(ui.Accent, ui.Panel, .45), ui.Panel)
	q.Draw(c, frame.X+2, frame.Y+1)
	for _, p := range []struct {
		x, y int
		r    rune
	}{{frame.X, frame.Y, '┏'}, {frame.Right() - 1, frame.Y, '┓'}, {frame.X, frame.Bottom() - 1, '┗'}, {frame.Right() - 1, frame.Bottom() - 1, '┛'}} {
		c.Put(p.x, p.y, p.r, ui.Accent, ui.Panel, true)
	}
	msg := "Scan with WhatsApp · Settings → Linked devices → Link a device · Esc exits full screen"
	if y := frame.Bottom() + 1; y < H-5 {
		c.Text(max(1, (W-ui.StringWidth(msg))/2), y, ui.Truncate(msg, W-2, "…"), ui.Dim, ui.Panel, false)
	}
	s.add(frame, "pair:full")
}

// chatIcon draws the thick speech-bubble terminal icon from the mock-ups.
func (s *scene) chatIcon(x, y int) {
	c := s.c
	icon := ui.R(x, y, 10, 5)
	c.Box(icon, ui.Accent, ui.Panel)
	c.Text(x+3, y+1, ">_", ui.Accent, ui.Panel, true)
}
