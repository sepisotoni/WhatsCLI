package app

import (
	"fmt"
	"os"
	"strings"
	"time"

	"whatscli/internal/ui"
)

func (s *scene) modal(title string, w, h int, dim bool) ui.Rect {
	m, c := s.m, s.c
	W, H := m.width, m.height
	if dim {
		c.Dim(ui.R(0, 0, W, H), ui.BG, .62)
	}
	s.add(ui.R(0, 0, W, H), "modal:backdrop")
	w, h = min(w, W-2), min(h, H-3)
	r := ui.R((W-w)/2, max(1, (H-h)/2), w, h)
	s.add(r, "modal:body")
	c.Box(ui.R(r.X-1, r.Y-1, r.W+2, r.H+2), ui.Mix(ui.Accent, ui.BG, .8), 0)
	c.Box(r, ui.BorderHi, ui.Panel)
	c.Text(r.X+2, r.Y, " "+title+" ", ui.Accent, ui.Panel, true)
	c.Text(r.Right()-8, r.Y, " esc ", ui.Dim, ui.Panel, false)
	s.add(ui.R(r.Right()-8, r.Y, 5, 1), "modal:close")
	return r
}

func (s *scene) overlay() {
	switch s.m.active {
	case commandScreen:
		s.paletteView()
	case settingsScreen:
		s.settingsView()
	case helpScreen:
		s.helpView()
	case revisionsScreen:
		s.revisionsView()
	case logoutScreen:
		s.logoutView()
	case newChatScreen:
		s.newChatView()
	case attachmentScreen:
		s.attachView()
	case confirmFileScreen:
		s.confirmView()
	case explorerScreen:
		s.explorerView()
	case menuScreen:
		s.menuView()
	case emojiScreen:
		s.emojiView()
	}
}

func (s *scene) inputBox(r ui.Rect, prompt, text string) {
	c := s.c
	c.Box(r, ui.Accent, ui.Panel)
	c.Text(r.X+2, r.Y+1, prompt, ui.Accent, ui.Panel, true)
	px := r.X + 2 + ui.StringWidth(prompt) + 1
	shown := ui.TailFit(text, r.W-(px-r.X)-3)
	w := c.Text(px, r.Y+1, shown, ui.Text, ui.Panel, false)
	if s.m.settings.ReducedMotion || s.m.frame%2 == 0 {
		c.Fill(ui.R(px+w, r.Y+1, 1, 1), ui.Accent)
	}
}

func (s *scene) paletteView() {
	m, c := s.m, s.c
	matches := m.filteredCommands()
	n := max(1, len(matches))
	r := s.modal("Commands", 58, 8+n, true)
	s.inputBox(ui.R(r.X+2, r.Y+2, r.W-4, 3), "/", m.filter)
	if len(matches) == 0 {
		c.Text(r.X+4, r.Y+6, "No matching commands", ui.Dim, ui.Panel, false)
		return
	}
	for i, cmd := range matches {
		y := r.Y + 6 + i
		bg := ui.Panel
		id := fmt.Sprintf("palette:%d", i)
		if i == m.commandIndex || s.is(id) {
			bg = ui.PanelHi
		}
		c.Fill(ui.R(r.X+2, y, r.W-4, 1), bg)
		marker := " "
		if i == m.commandIndex {
			marker = "▸"
		}
		c.Text(r.X+3, y, marker, ui.Accent, bg, true)
		c.Text(r.X+5, y, cmd, ui.Text, bg, i == m.commandIndex)
		s.add(ui.R(r.X+2, y, r.W-4, 1), id)
	}
	c.Text(r.X+3, r.Bottom()-2, "↑↓ choose · Enter run · Esc close", ui.Faint, ui.Panel, false)
}

func (s *scene) toggle(x, y int, on bool, bg ui.Color) {
	if on {
		s.c.Text(x, y, "  ON  ", ui.AccentDark, ui.Accent, true)
	} else {
		s.c.Text(x, y, " OFF  ", ui.Dim, ui.Mix(ui.Panel, ui.Gray, .25), false)
	}
}

func (s *scene) settingsView() {
	m, c := s.m, s.c
	tall := m.height >= 30
	rowH := 1
	if tall {
		rowH = 2
	}
	r := s.modal("Settings", 74, 6+settingCount*rowH+3, true)
	type row struct {
		label, desc string
		on, action  bool
		verb        string
	}
	rows := []row{
		{"Mouse support", "Click, hover and wheel scrolling", m.settings.Mouse, false, ""},
		{"Reduced motion", "Stop spinners and the blinking caret", m.settings.ReducedMotion, false, ""},
		{"Profile photos", "Contact photos; off = initials, nothing fetched", m.settings.Photos(), false, ""},
		{"Chat wallpaper", "Faint doodle pattern behind chats", m.settings.Wallpaper(), false, ""},
		{"12-hour clock", "Show 3:04 PM instead of 15:04", m.settings.Clock12h, false, ""},
		{"Terminal bell", "Ring when a new unread message arrives", m.settings.Bell, false, ""},
		{"Link / reconnect device", "Show the pairing QR and reconnect", false, true, "Open"},
		{"Unlink this device", "Sign out; local history is kept", false, true, "Unlink"},
		{"Send files", "Open the file browser for the current chat", false, true, "Browse"},
	}
	for i, rw := range rows {
		y := r.Y + 2 + i*rowH
		id := fmt.Sprintf("set:%d", i)
		bg := ui.Panel
		if i == m.settingsIndex || s.is(id) {
			bg = ui.PanelHi
		}
		c.Fill(ui.R(r.X+2, y, r.W-4, rowH), bg)
		marker := " "
		if i == m.settingsIndex {
			marker = "▸"
		}
		c.Text(r.X+3, y, marker, ui.Accent, bg, true)
		c.Text(r.X+5, y, rw.label, ui.Text, bg, true)
		if tall {
			c.Text(r.X+5, y+1, ui.Truncate(rw.desc, r.W-22, "…"), ui.Dim, bg, false)
		}
		if rw.action {
			label := " " + rw.verb + " › "
			c.Text(r.Right()-3-ui.StringWidth(label), y, label, ui.Accent, bg, true)
		} else {
			s.toggle(r.Right()-9, y, rw.on, bg)
		}
		s.add(ui.R(r.X+2, y, r.W-4, rowH), id)
	}
	dir := m.stateDir
	if dir == "" {
		dir = "(temporary)"
	}
	if m.demo {
		dir = "(demo: temporary, removed on exit)"
	}
	c.HLine(r.X+2, r.Bottom()-3, r.W-4, '─', ui.Border)
	c.Text(r.X+3, r.Bottom()-2, ui.Truncate("WhatsCLI 0.3.0 · data: "+dir+" · ↑↓ · Enter/Space toggle · m mouse · t motion", r.W-6, "…"), ui.Faint, ui.Panel, false)
}

var helpRows = []struct{ k, d string }{
	{"", "NAVIGATION"},
	{"Tab", "Switch between chat list and conversation"},
	{"↑ ↓  PgUp PgDn", "Move through chats or messages"},
	{"Enter", "Open chat · show retained versions of a message"},
	{"Ctrl+F", "Search chats (current page)"},
	{"e", "Archive / unarchive the selected chat"},
	{"n", "New chat from an international phone number"},
	{"s  ?  q", "Settings · this help · quit"},
	{"", "MESSAGE BOX"},
	{"i  or click", "Start typing · Enter sends · Ctrl+J newline · Ctrl+U clear"},
	{"/", "Slash commands: /help /files /new /meta /search /archive /emoji …"},
	{"//text", "Send a message that starts with a literal slash"},
	{"a  or  +  ↥", "Open the file browser (files and folders)"},
	{"", "FILE BROWSER"},
	{"↑↓ Enter Space", "Move · open folder · select"},
	{"a  x  .  s  /", "Select files · clear · hidden · sort · filter"},
	{"Ctrl+L", "Type a path · u uploads the selection"},
	{"", "OTHER"},
	{"Ctrl+P", "Command palette"},
	{"Ctrl+C", "Quit (cancels any send)"},
	{"", "Calls, status and stickers are not supported. Folders upload as .zip."},
}

func (s *scene) helpView() {
	m, c := s.m, s.c
	r := s.modal("Help", 78, m.height-4, true)
	view := ui.R(r.X+2, r.Y+2, r.W-4, r.H-4)
	maxScroll := max(0, len(helpRows)-view.H)
	off := min(m.scroll, maxScroll)
	for i := 0; i < view.H && off+i < len(helpRows); i++ {
		row := helpRows[off+i]
		y := view.Y + i
		if row.k == "" {
			c.Text(view.X, y, row.d, ui.Accent, ui.Panel, true)
			continue
		}
		c.Text(view.X+1, y, row.k, ui.Text, ui.Panel, true)
		c.Text(view.X+22, y, ui.Truncate(row.d, view.W-22, "…"), ui.Dim, ui.Panel, false)
	}
	foot := "↑↓ scroll · Esc close"
	if maxScroll > off {
		foot = "▼ more · " + foot
	}
	c.Text(r.X+3, r.Bottom()-2, foot, ui.Faint, ui.Panel, false)
}

func (s *scene) revisionsView() {
	m, c := s.m, s.c
	r := s.modal("Message versions", 74, m.height-4, true)
	var lines []string
	lines = append(lines, "Most recently observed first; not source-time order.", "")
	for _, rev := range m.revisions {
		lines = append(lines, fmt.Sprintf("Revision %d · %s", rev.Number, rev.EditedAt.Local().Format(time.RFC822)))
		lines = append(lines, ui.Wrap(ui.SafeText(rev.Body), r.W-6)...)
		lines = append(lines, "")
	}
	if len(m.revisions) == 0 {
		lines = append(lines, "No text version available; an original may never have been received.")
	}
	view := ui.R(r.X+2, r.Y+2, r.W-4, r.H-4)
	off := min(m.scroll, max(0, len(lines)-view.H))
	for i := 0; i < view.H && off+i < len(lines); i++ {
		line := lines[off+i]
		fg := ui.Text
		if strings.HasPrefix(line, "Revision ") {
			fg = ui.Accent
		} else if i+off < 2 {
			fg = ui.Dim
		}
		c.Text(view.X, view.Y+i, line, fg, ui.Panel, strings.HasPrefix(line, "Revision "))
	}
	c.Text(r.X+3, r.Bottom()-2, "↑↓ scroll · p older versions · r latest · Esc back", ui.Faint, ui.Panel, false)
}

func (s *scene) logoutView() {
	c := s.c
	r := s.modal("Unlink this device", 58, 11, true)
	c.Text(r.X+3, r.Y+2, "Unlink this WhatsCLI session from WhatsApp?", ui.Text, ui.Panel, true)
	c.Text(r.X+3, r.Y+3, "You will need to pair again to use it.", ui.Dim, ui.Panel, false)
	c.Text(r.X+3, r.Y+4, "Locally retained message history is NOT deleted.", ui.Dim, ui.Panel, false)
	s.outline(ui.R(r.X+3, r.Bottom()-4, 14, 3), "Cancel", "logout:cancel", false)
	s.primary(ui.R(r.Right()-3-18, r.Bottom()-4, 18, 3), "Unlink  ✓", "logout:confirm", true)
}

func (s *scene) newChatView() {
	m, c := s.m, s.c
	r := s.modal("New conversation", 66, 15, true)
	c.Text(r.X+3, r.Y+2, "Start a direct chat", ui.Text, ui.Panel, true)
	c.Text(r.X+3, r.Y+3, "International phone number with country code, e.g. +27 82 123 4567", ui.Dim, ui.Panel, false)
	s.inputBox(ui.R(r.X+3, r.Y+5, r.W-6, 3), "+", strings.TrimPrefix(m.newChat, "+"))
	c.Text(r.X+3, r.Y+9, "Enter creates a local conversation; it sends nothing.", ui.Dim, ui.Panel, false)
	c.Text(r.X+3, r.Y+10, "The recipient must have WhatsApp. Meta AI: /meta in any chat.", ui.Dim, ui.Panel, false)
	s.primary(ui.R(r.Right()-3-18, r.Bottom()-4, 18, 3), "Open chat  →", "newchat:go", true)
	s.outline(ui.R(r.X+3, r.Bottom()-4, 14, 3), "Cancel", "modal:close", false)
}

func (s *scene) attachView() {
	m, c := s.m, s.c
	r := s.modal("Send a file by path", 78, 12, true)
	c.Text(r.X+3, r.Y+2, "Path on this computer (not your SSH client):", ui.Text, ui.Panel, true)
	s.inputBox(ui.R(r.X+3, r.Y+4, r.W-6, 3), "›", m.path)
	c.Text(r.X+3, r.Y+8, "Spaces are fine; do not add shell quotes. Maximum 64 MiB.", ui.Dim, ui.Panel, false)
	c.Text(r.X+3, r.Y+9, "Enter previews · Esc cancels · use the file browser (+) for folders.", ui.Faint, ui.Panel, false)
}

func (s *scene) confirmView() {
	m, c := s.m, s.c
	n := len(m.queue)
	r := s.modal("Confirm upload", 78, min(m.height-4, 13+n), true)
	c.Text(r.X+3, r.Y+2, "Send to:", ui.Dim, ui.Panel, false)
	c.Text(r.X+12, r.Y+2, ui.Truncate(ui.SingleLine(m.chatTitle), r.W-16, "…"), ui.Accent, ui.Panel, true)
	view := ui.R(r.X+3, r.Y+4, r.W-6, r.H-11)
	off := min(m.scroll, max(0, n-view.H))
	var total int64
	for _, it := range m.queue {
		total += it.size
	}
	for i := 0; i < view.H && off+i < n; i++ {
		it := m.queue[off+i]
		y := view.Y + i
		icon, detail := "▤", humanSize(it.size)
		if it.folder {
			icon, detail = "▸", fmt.Sprintf("%d files · %s → zipped", it.files, humanSize(it.size))
		}
		c.Text(view.X, y, icon, ui.Accent, ui.Panel, true)
		nameW := view.W - ui.StringWidth(detail) - 4
		c.Text(view.X+2, y, ui.Truncate(ui.SingleLine(it.name), max(4, nameW), "…"), ui.Text, ui.Panel, false)
		c.Text(view.Right()-ui.StringWidth(detail), y, detail, ui.Dim, ui.Panel, false)
	}
	c.HLine(r.X+3, r.Bottom()-7, r.W-6, '─', ui.Border)
	summary := fmt.Sprintf("%d item(s) · %s · sent one by one as documents", n, humanSize(total))
	c.Text(r.X+3, r.Bottom()-6, ui.Truncate(summary, r.W-6, "…"), ui.Dim, ui.Panel, false)
	c.Text(r.X+3, r.Bottom()-5, "Folders are zipped (symlinks skipped). Stops on the first failure.", ui.Faint, ui.Panel, false)
	s.outline(ui.R(r.X+3, r.Bottom()-4, 14, 3), "Back", "confirm:cancel", false)
	s.primary(ui.R(r.Right()-3-22, r.Bottom()-4, 22, 3), fmt.Sprintf("Send %d  →", n), "confirm:send", !m.uploading)
}

func (s *scene) menuView() {
	m, c := s.m, s.c
	g := m.geo()
	mx := g.main.Right() - 2 - 30
	if g.main.W == 0 {
		mx = m.width - 32
	}
	r := ui.R(max(1, mx), g.main.Y+4, 30, len(menuItems)+2)
	s.add(ui.R(0, 0, m.width, m.height), "modal:backdrop")
	s.add(r, "modal:body")
	c.Box(r, ui.Accent, ui.Panel)
	for i, item := range menuItems {
		id := fmt.Sprintf("menu:%d", i)
		bg := ui.Panel
		if i == m.menuIndex || s.is(id) {
			bg = ui.PanelHi
		}
		c.Fill(ui.R(r.X+1, r.Y+1+i, r.W-2, 1), bg)
		c.Text(r.X+3, r.Y+1+i, item, ui.Text, bg, i == m.menuIndex)
		s.add(ui.R(r.X+1, r.Y+1+i, r.W-2, 1), id)
	}
}

func (s *scene) emojiView() {
	m, c := s.m, s.c
	g := m.geo()
	rows := (len(emojis) + emojiCols - 1) / emojiCols
	w, h := emojiCols*4+2, rows+2
	ch := 5
	if g.main.H < 24 {
		ch = 3
	}
	r := ui.R(max(1, g.main.Right()-w-2), g.main.Bottom()-ch-h, w, h)
	if g.main.W == 0 {
		r.X = m.width - w - 2
	}
	r.Y = max(1, r.Y)
	s.add(ui.R(0, 0, m.width, m.height), "modal:backdrop")
	s.add(r, "modal:body")
	c.Box(r, ui.Accent, ui.Panel)
	for i, e := range emojis {
		x, y := r.X+1+(i%emojiCols)*4, r.Y+1+i/emojiCols
		id := fmt.Sprintf("emoji:%d", i)
		bg := ui.Panel
		if i == m.emojiIndex || s.is(id) {
			bg = ui.PanelHi
		}
		c.Fill(ui.R(x, y, 4, 1), bg)
		c.Text(x+1, y, e, ui.Text, bg, false)
		s.add(ui.R(x, y, 4, 1), id)
	}
}

// ---- file explorer ----

func (s *scene) explorerView() {
	m, c := s.m, s.c
	e := m.explorer
	if e == nil {
		return
	}
	r := s.modal("Send files · "+ui.Truncate(ui.SingleLine(m.chatTitle), 30, "…"), min(m.width-6, 110), m.height-4, true)
	restore := c.Clip(r.Inset(0))
	defer c.Restore(restore)
	up := ui.R(r.X+2, r.Y+1, 7, 3)
	border := ui.Border
	if s.is("x:up") {
		border = ui.Accent
	}
	c.Box(up, border, ui.Panel)
	s.center(up, "↑", ui.Accent, ui.Panel, true)
	s.add(up, "x:up")
	pr := ui.R(up.Right()+1, r.Y+1, r.W-12, 3)
	if e.typing {
		s.inputBox(pr, "path ›", e.pathInput)
	} else {
		pb := ui.Border
		if s.is("x:path") {
			pb = ui.Accent
		}
		c.Box(pr, pb, ui.Panel)
		label := e.dir
		if e.loading {
			label += "   loading…"
		}
		if e.err != "" {
			label = e.err
		}
		c.Text(pr.X+2, pr.Y+1, tailPath(ui.SingleLine(label), pr.W-4), ui.Text, ui.Panel, false)
		s.add(pr, "x:path")
	}
	rows := max(3, r.H-explorerChrome)
	entries := e.view()
	e.clamp(rows)
	hy := r.Y + 4
	nameX := r.X + 10
	sizeX := r.Right() - 26
	modX := r.Right() - 14
	c.Text(nameX, hy, "NAME", ui.Faint, ui.Panel, true)
	c.Text(sizeX, hy, "   SIZE", ui.Faint, ui.Panel, true)
	c.Text(modX, hy, "MODIFIED", ui.Faint, ui.Panel, true)
	for i := 0; i < rows; i++ {
		idx := e.top + i
		if idx >= len(entries) {
			break
		}
		d := entries[idx]
		y := r.Y + 5 + i
		bg := ui.Panel
		rid := fmt.Sprintf("x:row:%d", idx)
		if idx == e.cursor || s.is(rid) {
			bg = ui.PanelHi
		}
		c.Fill(ui.R(r.X+1, y, r.W-2, 1), bg)
		if idx == e.cursor {
			c.Put(r.X+2, y, '▌', ui.Accent, bg, false)
		}
		if d.name != ".." {
			if e.selected[d.path] {
				c.Text(r.X+4, y, " ✓ ", ui.AccentDark, ui.Accent, true)
			} else if d.link {
				c.Text(r.X+4, y, " – ", ui.Faint, bg, false)
			} else {
				c.Text(r.X+4, y, "[ ]", ui.Dim, bg, false)
			}
		}
		fg := ui.Text
		icon, iconFG := "·", ui.Dim
		name := ui.SingleLine(d.name)
		if d.dir {
			icon, iconFG = "▸", ui.Accent
			if d.name != ".." {
				name += "/"
			}
		}
		if d.link {
			name += " →"
			fg = ui.Dim
		}
		if e.selected[d.path] {
			fg = ui.Accent
		}
		c.Text(r.X+8, y, icon, iconFG, bg, true)
		c.Text(nameX, y, ui.Truncate(name, max(4, sizeX-nameX-2), "…"), fg, bg, d.dir)
		if d.name != ".." {
			if !d.dir {
				sz := humanSize(d.size)
				c.Text(sizeX+9-ui.StringWidth(sz), y, sz, ui.Dim, bg, false)
			}
			c.Text(modX, y, d.mod.Local().Format("2006-01-02"), ui.Dim, bg, false)
		}
		s.add(ui.R(r.X+1, y, r.W-2, 1), rid)
		if d.name != ".." && !d.link {
			s.add(ui.R(r.X+4, y, 3, 1), fmt.Sprintf("x:chk:%d", idx))
		}
	}
	if len(entries) == 0 || (len(entries) == 1 && entries[0].name == "..") {
		c.Text(nameX, r.Y+6, "This folder is empty (or everything is hidden: press . to show hidden files).", ui.Dim, ui.Panel, false)
	}
	if e.truncated {
		c.Text(nameX, r.Y+5+rows-1, "Showing the first 20000 entries.", ui.Danger, ui.Panel, false)
	}
	hint := "↑↓ move · Enter open · Space select · a all files · x clear · . hidden · / filter · Ctrl+L path · s sort · u upload"
	if e.filtering {
		hint = "Filter: " + e.filter + "▏   (Enter keeps it · Esc clears)"
	} else if e.filter != "" {
		hint = "Filter: " + e.filter + "   (Esc clears)"
	}
	c.Text(r.X+3, r.Y+5+rows, ui.Truncate(hint, r.W-6, "…"), ui.Faint, ui.Panel, false)
	c.HLine(r.X+2, r.Bottom()-6, r.W-4, '─', ui.Border)
	files, dirs := 0, 0
	for _, p := range e.order {
		if info, err := os.Lstat(p); err == nil && info.IsDir() {
			dirs++
		} else {
			files++
		}
	}
	sortName := [...]string{"name", "size", "modified"}[e.sortBy]
	hidden := "off"
	if e.hidden {
		hidden = "on"
	}
	c.Text(r.X+3, r.Bottom()-5, fmt.Sprintf("%d selected · %d file(s), %d folder(s)", len(e.order), files, dirs), ui.Text, ui.Panel, true)
	toggles := []struct{ label, id string }{{" a · all files ", "x:all"}, {" x · clear ", "x:clear"}, {" . · hidden: " + hidden + " ", "x:hidden"}, {" s · sort: " + sortName + " ", "x:sort"}}
	tx := r.X + 3
	for _, t := range toggles {
		bg := ui.PanelHi
		if s.is(t.id) {
			bg = ui.Mix(ui.PanelHi, ui.Accent, .25)
		}
		c.Fill(ui.R(tx, r.Bottom()-3, ui.StringWidth(t.label), 1), bg)
		c.Text(tx, r.Bottom()-3, t.label, ui.Text, bg, false)
		s.add(ui.R(tx, r.Bottom()-3, ui.StringWidth(t.label), 1), t.id)
		tx += ui.StringWidth(t.label) + 1
	}
	s.outline(ui.R(r.Right()-3-22-15, r.Bottom()-4, 14, 3), "Cancel", "x:cancel", false)
	label := fmt.Sprintf("Upload %d  →", len(e.order))
	if len(e.order) == 0 {
		label = "Upload  →"
	}
	s.primary(ui.R(r.Right()-3-22, r.Bottom()-4, 22, 3), label, "x:upload", true)
}

// tailPath keeps the end of a long path visible, which is the useful part.
func tailPath(p string, w int) string {
	if ui.StringWidth(p) <= w {
		return p
	}
	return "…" + ui.TailFit(p, w-1)
}
