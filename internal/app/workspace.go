package app

import (
	"fmt"
	"strings"
	"time"

	"whatscli/internal/history"
	"whatscli/internal/messaging"
	"whatscli/internal/ui"
)

type geo struct{ rail, list, main, foot ui.Rect }

func (m Model) chatPaneActive() bool {
	if m.active == chatScreen || m.composing {
		return true
	}
	if n := len(m.trail); n > 0 {
		return m.trail[n-1] == chatScreen
	}
	return m.isOverlay() && m.chatID != "" && m.active != settingsScreen && m.active != helpScreen && m.active != commandScreen && m.active != newChatScreen
}

func (m Model) geo() geo {
	W, H := m.width, m.height
	ph := H - 1
	g := geo{foot: ui.R(0, H-1, W, 1)}
	switch {
	case W >= 100:
		lw := min(44, max(32, W*30/100))
		if W >= 144 {
			lw = min(44, max(32, W*26/100))
			ph = H - 2
			g.rail = ui.R(1, 1, 11, ph)
			g.list = ui.R(13, 1, lw, ph)
			g.main = ui.R(13+lw, 1, W-14-lw, ph)
			g.foot = ui.Rect{}
		} else {
			g.rail = ui.R(0, 0, 11, ph)
			g.list = ui.R(12, 0, lw, ph)
			g.main = ui.R(13+lw, 0, W-13-lw, ph)
		}
	case W >= 72:
		g.list = ui.R(0, 0, 30, ph)
		g.main = ui.R(31, 0, W-31, ph)
	default:
		if m.chatPaneActive() && m.chatID != "" {
			g.main = ui.R(0, 0, W, ph)
		} else {
			g.list = ui.R(0, 0, W, ph)
		}
	}
	return g
}

const (
	itemPitch = 5
	listHead  = 7
)

func (s *scene) scrollbar(x, y, height, total, visible, offset int) {
	if total <= visible || height < 2 {
		return
	}
	thumb := max(1, height*visible/total)
	thumb = min(height, thumb)
	top := y + (height-thumb)*min(max(offset, 0), max(0, total-visible))/max(1, total-visible)
	for row := y; row < y+height; row++ {
		glyph, fg := '│', ui.Border
		if row >= top && row < top+thumb {
			glyph, fg = '█', ui.Accent
		}
		s.c.Put(x, row, glyph, fg, ui.Panel, false)
	}
}

// listCapacity is the number of chat rows that fit in the list viewport.
func (m Model) listCapacity() int {
	h := m.geo().list.H
	if h == 0 {
		h = m.height - 1
	}
	head := listHead
	if m.width >= 144 {
		head += 2
	}
	return max(1, (h-head-4)/itemPitch)
}

func (s *scene) workspace() {
	m := s.m
	g := m.geo()
	if g.rail.W > 0 {
		s.rail(g.rail)
	}
	if g.list.W > 0 {
		s.list(g.list)
	}
	if g.main.W > 0 {
		s.mainPane(g.main, g.list.W == 0)
	}
	if g.foot.W > 0 && g.foot.H > 0 {
		s.footer(g.foot)
	}
}

// ---- navigation rail ----

func (s *scene) railButton(r ui.Rect, glyph, id string, active bool) {
	c := s.c
	border, fill, fg := ui.Border, ui.Panel, ui.Text
	if active {
		border, fill, fg = ui.BorderHi, ui.PanelHi, ui.Accent
	}
	if s.is(id) {
		border, fill = ui.Accent, ui.PanelHi
	}
	if active || id == "rail:new" {
		ui.DrawCircle(c, r.X, r.Y, ui.Accent, ui.Panel)
		fg, fill = ui.AccentDark, ui.Accent
	} else if s.is(id) {
		c.Box(r, border, fill)
	}
	if id == "rail:new" {
		s.center(r, "+", fg, fill, true)
	} else {
		s.center(r, glyph, fg, fill, true)
	}
	s.add(r, id)
}

func (s *scene) rail(r ui.Rect) {
	m, c := s.m, s.c
	c.Box(r, ui.Border, ui.Panel)
	bx := r.X + 1
	y := r.Y + 1
	total := 0
	for _, n := range m.unread {
		total += n
	}
	buttons := []struct {
		glyph, id string
		active    bool
	}{
		{"◉", "rail:chats", m.listMode == modeAll || m.listMode == modeArchive},
		{"+", "rail:new", false},
		{"✎", "rail:compose", false},
		{"♙", "rail:contacts", m.listMode == modeContacts},
		{"◌", "rail:statuses", false},
		{"⇩", "rail:files", false},
	}
	for _, b := range buttons {
		if y+4 > r.Bottom()-4 {
			break
		}
		btn := ui.R(bx+1, y, 8, 4)
		s.railButton(btn, b.glyph, b.id, b.active)
		if b.id == "rail:chats" && total > 0 {
			label := fmt.Sprint(total)
			if total > 99 {
				label = "99+"
			}
			bw := ui.StringWidth(label) + 2
			c.Fill(ui.R(btn.Right()-bw, y-1, bw, 1), ui.Accent)
			c.Text(btn.Right()-bw+1, y-1, label, ui.AccentDark, ui.Accent, true)
		}
		y += 5
	}
	s.railButton(ui.R(bx+1, r.Bottom()-5, 8, 4), "⚙", "rail:settings", m.active == settingsScreen)
}

// ---- chat list ----

func (m Model) listTitle() string {
	switch m.listMode {
	case modeContacts:
		return "Contacts"
	case modeUnread:
		return "Unread"
	case modeArchive:
		return "Archive"
	}
	return "Chats"
}

func (m Model) clock(t time.Time) string {
	t = t.Local()
	if m.settings.Clock12h {
		return t.Format("3:04 PM")
	}
	return t.Format("15:04")
}

func (m Model) stamp(t time.Time) string {
	now := time.Now()
	if !m.now.IsZero() {
		now = m.now
	}
	t = t.Local()
	switch days := int(now.Local().Sub(t).Hours() / 24); {
	case t.Format("2006-01-02") == now.Local().Format("2006-01-02") || (t.After(now) && days <= 0):
		return m.clock(t)
	case days <= 1:
		return "Yesterday"
	case days < 7:
		return t.Format("Mon")
	}
	return t.Format("02/01/06")
}

func (m Model) senderLabel(jid string) string {
	for _, c := range m.conversations {
		if c.ID == jid && c.Title != "" {
			return ui.SingleLine(c.Title)
		}
	}
	if messaging.IsMetaAI(jid) {
		return "Meta AI"
	}
	name, _, _ := strings.Cut(jid, "@")
	return ui.SingleLine(name)
}

// previewParts splits a chat-list preview into a coloured prefix and text.
func (m Model) previewParts(c history.Conversation) (prefix string, pfg ui.Color, text string) {
	text = ui.SingleLine(c.LastBody)
	group := strings.HasSuffix(c.ID, "@g.us")
	switch {
	case strings.HasPrefix(text, "[document: "):
		text = "▤ " + strings.TrimSuffix(strings.TrimPrefix(text, "[document: "), "]")
	case strings.HasPrefix(text, "[image"):
		rest := strings.TrimSpace(text[strings.Index(text, "]")+1:])
		text = "▣ " + map[bool]string{true: rest, false: "Image"}[rest != ""]
	case strings.HasPrefix(text, "[video"):
		text = "▶ Video"
	case strings.HasPrefix(text, "[audio"):
		text = "♪ Voice message"
	case strings.HasPrefix(text, "[sticker"):
		text = "Sticker"
	}
	if c.LastBody == "" {
		return "", ui.Dim, "No messages yet"
	}
	if c.LastFromMe {
		switch c.LastStatus {
		case "read":
			prefix, pfg = "✓✓ ", ui.Accent
		case "delivered":
			prefix, pfg = "✓✓ ", ui.Dim
		case "pending":
			prefix, pfg = "… ", ui.Dim
		case "unknown", "failed":
			prefix, pfg = "! ", ui.Danger
		default:
			prefix, pfg = "✓ ", ui.Dim
		}
		if group {
			text = "You: " + text
		}
		return
	}
	if group && c.LastSender != "" {
		text = m.senderLabel(c.LastSender) + ": " + text
	}
	return "", ui.Dim, text
}

func (s *scene) list(r ui.Rect) {
	m, c := s.m, s.c
	focus := (m.active == inboxScreen || (m.active != chatScreen && !m.composing)) && !s.off
	border := ui.Border
	if focus {
		border = ui.Mix(ui.Accent, ui.Panel, .35)
	}
	c.Box(r, border, ui.Panel)
	title := m.listTitle()
	c.Text(r.X+3, r.Y+1, title, ui.Text, ui.Panel, true)
	count := len(m.visibleChats())
	if m.listMode == modeAll {
		count = 0
		for _, conv := range m.conversations {
			if !m.isArchived(conv.ID) {
				count++
			}
		}
	}
	s.pill(r.X+3+ui.StringWidth(title)+1, r.Y+1, fmt.Sprint(count), ui.Accent, ui.PanelHi)
	sr := ui.R(r.X+2, r.Y+4, r.W-4, 3)
	sb := ui.Border
	if m.searching || s.is("search") {
		sb = ui.Accent
	}
	c.Box(sr, sb, ui.Panel)
	c.Text(sr.X+2, sr.Y+1, "⌕", ui.Dim, ui.Panel, false)
	query := "Search chats…"
	qfg := ui.Dim
	if m.search != "" || m.searching {
		query, qfg = ui.TailFit(ui.SingleLine(m.search), sr.W-8), ui.Text
		if m.searching {
			query += "▏"
		}
	}
	c.Text(sr.X+4, sr.Y+1, ui.Truncate(query, sr.W-6, "…"), qfg, ui.Panel, false)
	s.add(sr, "search")

	indices := m.visibleChats()
	vis := m.listCapacity()
	top := min(m.listTop, max(0, len(indices)-vis))
	head := listHead
	if m.width >= 144 {
		head += 2
	}
	y0 := r.Y + head
	for slot := 0; slot < vis && top+slot < len(indices); slot++ {
		idx := indices[top+slot]
		y := y0 + slot*itemPitch
		s.chatRow(ui.R(r.X+1, y, r.W-2, 3), idx, idx == m.chatIndex)
	}
	ay := r.Bottom() - 5
	trackY := y0
	trackH := max(0, ay-trackY-2)
	if trackH > 0 {
		s.scrollbar(r.Right()-2, trackY, trackH, len(indices), vis, top)
	}
	if len(indices) == 0 {
		msg := "No chats here yet."
		hint := "n · start a conversation"
		if m.listMode == modeUnread {
			msg, hint = "Nothing unread.", "You are all caught up."
		}
		if m.search != "" {
			msg, hint = "No matching chats.", "Esc clears the search."
		}
		c.Text(r.X+4, y0+1, msg, ui.Dim, ui.Panel, false)
		c.Text(r.X+4, y0+3, hint, ui.Faint, ui.Panel, false)
	}
	// Archive row (or back-to-chats while viewing the archive).
	c.HLine(r.X+2, ay-1, r.W-4, '─', ui.Mix(ui.Border, ui.Panel, .5))
	rect := ui.R(r.X+1, ay, r.W-2, 4)
	if s.is("archive") {
		c.Fill(ui.R(rect.X, ay, rect.W, 3), ui.PanelHi)
	}
	bg := ui.Panel
	if s.is("archive") {
		bg = ui.PanelHi
	}
	icon := ui.R(r.X+3, ay, 8, 3)
	c.Fill(icon, bg)
	c.Text(icon.X+3, icon.Y+1, "▣", ui.Accent, bg, true)
	label := "Archive"
	glyph := "›"
	if m.listMode == modeArchive {
		label, glyph = "Back to chats", "‹"
	}
	c.Text(r.X+13, ay+1, label, ui.Text, bg, true)
	c.Text(r.Right()-4, ay+1, glyph, ui.Text, bg, true)
	s.add(rect, "archive")
}

func (s *scene) chatRow(r ui.Rect, idx int, selected bool) {
	m, c := s.m, s.c
	conv := m.conversations[idx]
	id := fmt.Sprintf("chat:%d", idx)
	hover := s.is(id)
	bg := ui.Panel
	switch {
	case selected:
		bg = ui.PanelHi
		c.Box(ui.R(r.X, r.Y-1, r.W, 5), ui.BorderHi, ui.PanelHi)
	case hover:
		bg = ui.PanelHi
		c.Fill(ui.R(r.X, r.Y, r.W, 3), bg)
	default:
		c.HLine(r.X+9, r.Y+3, r.W-10, '─', ui.Mix(ui.Border, ui.Panel, .55))
	}
	name := ui.SingleLine(conv.Title)
	if name == "" {
		name = conv.ID
	}
	if strings.HasSuffix(conv.ID, "@g.us") {
		ui.DrawGroupAvatar(c, r.X+2, r.Y, bg)
	} else {
		ui.DrawAvatar(c, r.X+2, r.Y, name, m.photoFor(conv.ID), bg)
	}
	tx := r.X + 10
	tw := r.W - 12
	stamp := m.stamp(conv.UpdatedAt)
	unread := m.unread[conv.ID]
	stampFG := ui.Dim
	if unread > 0 {
		stampFG = ui.Accent
	}
	sw := ui.StringWidth(stamp)
	c.Text(r.Right()-2-sw, r.Y, stamp, stampFG, bg, false)
	nameFG := ui.Text
	c.Text(tx, r.Y, ui.Truncate(name, max(1, tw-sw-1), "…"), nameFG, bg, true)
	badgeW := 0
	if unread > 0 {
		label := fmt.Sprint(unread)
		if unread > 99 {
			label = "99+"
		}
		badgeW = ui.StringWidth(label) + 2
		c.Fill(ui.R(r.Right()-2-badgeW, r.Y+1, badgeW, 1), ui.Accent)
		c.Text(r.Right()-1-badgeW, r.Y+1, label, ui.AccentDark, ui.Accent, true)
	}
	prefix, pfg, text := m.previewParts(conv)
	avail := tw - badgeW - 1
	px := tx
	if prefix != "" {
		px += c.Text(px, r.Y+1, prefix, pfg, bg, true)
		avail -= ui.StringWidth(prefix)
	}
	c.Text(px, r.Y+1, ui.Truncate(text, max(1, avail), "…"), ui.Dim, bg, false)
	s.add(ui.R(r.X, r.Y-1, r.W, 5), id)
}

// ---- conversation pane ----

func (m Model) subtitle() string {
	if sub, ok := m.demoSubtitle[m.chatID]; ok {
		return sub
	}
	switch {
	case messaging.IsMetaAI(m.chatID):
		return "Meta AI · availability depends on your account"
	case strings.HasSuffix(m.chatID, "@g.us"):
		return "Group chat"
	}
	return "Direct chat"
}

func (s *scene) mainPane(r ui.Rect, compact bool) {
	m, c := s.m, s.c
	if m.chatID == "" {
		c.Box(r, ui.Border, ui.Panel)
		if m.settings.Wallpaper() {
			ui.Doodle(c, r.Inset(1), ui.Ink)
		}
		cx, cy := r.X+r.W/2, r.Y+r.H/2
		box := ui.R(cx-6, cy-6, 13, 5)
		c.Box(box, ui.Accent, ui.Panel)
		c.Text(cx-1, cy-4, ">_", ui.Accent, ui.Panel, true)
		c.Put(cx-4, cy-1, '╲', ui.Accent, ui.Panel, true)
		lines := []struct {
			t   string
			fg  ui.Color
			bld bool
		}{{"Your conversations. Your terminal.", ui.Text, true}, {"Select a chat to get started.", ui.Dim, false}, {"", 0, false},
			{"n new chat   /  commands   ?  help", ui.Faint, false}}
		for i, l := range lines {
			c.Text(cx-ui.StringWidth(l.t)/2, cy+1+i, l.t, l.fg, ui.Panel, l.bld)
		}
		return
	}
	ch := 5
	if r.H >= 30 {
		ch = 6
	}
	if r.H < 24 && m.replyTo == nil {
		ch = 3
	}
	head := ui.R(r.X, r.Y, r.W, 5)
	comp := ui.R(r.X, r.Bottom()-ch, r.W, ch)
	body := ui.R(r.X, r.Y+5, r.W, r.H-5-ch)
	s.header(head, compact)
	s.body(body)
	s.composerBar(comp)
	s.popups(comp)
}

func (s *scene) header(r ui.Rect, compact bool) {
	m, c := s.m, s.c
	c.Box(r, ui.Border, ui.Panel)
	x := r.X + 2
	if compact {
		br := ui.R(x, r.Y+1, 5, 3)
		c.Box(br, ui.Border, ui.Panel)
		s.center(br, "‹", ui.Accent, ui.Panel, true)
		s.add(br, "head:back")
		x += 6
	}
	if strings.HasSuffix(m.chatID, "@g.us") {
		ui.DrawGroupAvatar(c, x, r.Y+1, ui.Panel)
	} else {
		ui.DrawAvatar(c, x, r.Y+1, m.chatTitle, m.photoFor(m.chatID), ui.Panel)
	}
	btnW := 8
	bx := r.Right() - 2 - 3*btnW - 2
	for i, b := range []struct{ glyph, id string }{{"▶", "head:video"}, {"✆", "head:call"}, {"⋮", "head:more"}} {
		br := ui.R(bx+i*(btnW+1), r.Y+1, btnW, 3)
		border := ui.Border
		if s.is(b.id) {
			border = ui.Accent
		}
		c.Box(br, border, ui.Panel)
		s.center(br, b.glyph, ui.Accent, ui.Panel, true)
		s.add(br, b.id)
	}
	tw := bx - (x + 9) - 1
	c.Text(x+9, r.Y+1, ui.Truncate(ui.SingleLine(m.chatTitle), tw, "…"), ui.Text, ui.Panel, true)
	sub := m.subtitle()
	status := ""
	if m.demo {
		status = "Online"
	} else if m.connection.Online {
		status = "Connected"
	} else {
		status = "Offline"
	}
	if sub != "" && tw > 0 {
		c.Text(x+7, r.Y+2, ui.Truncate(sub, tw, "…"), ui.Dim, ui.Panel, false)
		if w := ui.StringWidth(sub); w+3+ui.StringWidth(status) < tw {
			c.Text(x+7+w, r.Y+2, " · ", ui.Dim, ui.Panel, false)
			fg := ui.Accent
			if status == "Offline" {
				fg = ui.Faint
			}
			c.Text(x+10+w, r.Y+2, status, fg, ui.Panel, true)
		}
	}
}

// ---- transcript ----

type bubble struct {
	name         string
	quote        string
	reactions    string
	lines        []string
	meta         string
	tick         string
	tickFG       ui.Color
	inline       bool
	w, h         int
	own, deleted bool
}

type titem struct {
	day   bool
	label string
	idx   int
	h     int
}

func tickFor(status string) (string, ui.Color) {
	switch status {
	case "read":
		return "✓✓", ui.Accent
	case "delivered":
		return "✓✓", ui.Dim
	case "pending":
		return "…", ui.Dim
	case "unknown", "failed":
		return "!", ui.Danger
	}
	return "✓", ui.Dim
}

func (m Model) buildBubble(msg history.Message, iw int) bubble {
	b := bubble{own: msg.FromMe}
	group := strings.HasSuffix(m.chatID, "@g.us")
	if !msg.FromMe && group {
		b.name = m.senderLabel(msg.Sender)
	}
	body := ui.SafeText(msg.Body)
	if msg.Deleted {
		body = "[deleted · retained locally]\n" + body
	}
	maxText := max(12, min(iw*58/100, 72))
	if msg.ReplyToID != "" {
		sender := "Quoted message"
		if msg.ReplySender != "" {
			sender = m.quotedSender(msg.ReplySender)
		}
		body := ui.SingleLine(msg.ReplyBody)
		if body == "" {
			body = "Message"
		}
		b.quote = ui.Truncate(sender+": "+body, maxText, "…")
	}
	if len(msg.Reactions) > 0 {
		counts := make(map[string]int, len(msg.Reactions))
		var order []string
		for _, reaction := range msg.Reactions {
			if _, exists := counts[reaction.Emoji]; !exists {
				order = append(order, reaction.Emoji)
			}
			counts[reaction.Emoji]++
		}
		var parts []string
		for _, emoji := range order {
			part := emoji
			if counts[emoji] > 1 {
				part += fmt.Sprintf(" %d", counts[emoji])
			}
			parts = append(parts, part)
		}
		b.reactions = ui.Truncate(strings.Join(parts, "  "), maxText, "…")
	}
	b.lines = ui.Wrap(body, maxText)
	b.meta = m.clock(msg.CreatedAt)
	if msg.Edited {
		b.meta = "edited " + b.meta
	}
	meta := b.meta
	if msg.FromMe {
		b.tick, b.tickFG = tickFor(msg.Status)
		meta += " " + b.tick
	}
	mw := ui.StringWidth(meta)
	content := ui.StringWidth(b.name)
	content = max(content, ui.StringWidth(b.quote))
	content = max(content, ui.StringWidth(b.reactions))
	for _, l := range b.lines {
		content = max(content, ui.StringWidth(l))
	}
	last := ui.StringWidth(b.lines[len(b.lines)-1])
	b.inline = last+2+mw <= max(maxText, content)
	if b.inline {
		content = max(content, last+2+mw)
	} else {
		content = max(content, mw)
	}
	b.w = content + 8
	rows := len(b.lines)
	if b.quote != "" {
		rows++
	}
	if b.name != "" {
		rows++
	}
	if !b.inline {
		rows++
	}
	if b.reactions != "" {
		rows++
	}
	b.h = rows + 2
	return b
}

func (m Model) quotedSender(sender string) string {
	if sender == m.account || sender == "me" {
		return "You"
	}
	return m.senderLabel(sender)
}

func (m Model) dayLabel(t time.Time) string {
	now := time.Now()
	if !m.now.IsZero() {
		now = m.now
	}
	if t.Local().Format("2006-01-02") == now.Local().Format("2006-01-02") {
		return "Today"
	}
	if t.Local().Format("2006-01-02") == now.Local().AddDate(0, 0, -1).Format("2006-01-02") {
		return "Yesterday"
	}
	return t.Local().Format("2 January 2006")
}

// titems lays the page out chronologically with a date pill per day.
func (m Model) titems(iw int) []titem {
	var items []titem
	prevDay := ""
	for i := len(m.messages) - 1; i >= 0; i-- {
		day := m.messages[i].CreatedAt.Local().Format("2006-01-02")
		if day != prevDay {
			items = append(items, titem{day: true, label: m.dayLabel(m.messages[i].CreatedAt), idx: i, h: 3})
			prevDay = day
		}
		items = append(items, titem{idx: i, h: m.buildBubble(m.messages[i], iw).h})
	}
	return items
}

// window picks the items shown above bottom message `bottom`, keeping `sel` visible.
func window(items []titem, area, bottom, sel int) (lo, hi int) {
	pos := func(idx int) int {
		for i, it := range items {
			if !it.day && it.idx == idx {
				return i
			}
		}
		return len(items) - 1
	}
	hi = pos(bottom)
	span := func(hi int) int {
		used, lo := 0, hi+1
		for i := hi; i >= 0; i-- {
			h := items[i].h
			if used > 0 {
				h++
			}
			if used+h > area {
				break
			}
			used += h
			lo = i
		}
		return lo
	}
	sp := pos(sel)
	if sp > hi {
		hi = sp
	}
	lo = span(hi)
	for sp < lo && hi > 0 {
		hi--
		for hi > 0 && items[hi].day {
			hi--
		}
		lo = span(hi)
	}
	return
}

func (m Model) bodyInner() ui.Rect {
	g := m.geo()
	r := g.main
	if r.W == 0 {
		return ui.Rect{}
	}
	ch := 5
	if r.H >= 30 {
		ch = 6
	}
	if r.H < 24 && m.replyTo == nil {
		ch = 3
	}
	return ui.R(r.X, r.Y+5, r.W, r.H-5-ch).Inset(1)
}

// fitMessages keeps the selected message inside the transcript viewport.
func (m *Model) fitMessages() {
	inner := m.bodyInner()
	if len(m.messages) == 0 || inner.W < 10 || inner.H < 3 {
		return
	}
	items := m.titems(inner.W)
	_, hi := window(items, inner.H, m.msgTop, m.messageIndex)
	if hi >= 0 && hi < len(items) && !items[hi].day {
		m.msgTop = items[hi].idx
	}
}

func (s *scene) body(r ui.Rect) {
	m, c := s.m, s.c
	c.Box(r, ui.Border, ui.Panel)
	inner := r.Inset(1)
	s.add(inner, "body")
	if m.settings.Wallpaper() {
		ui.Doodle(c, inner, ui.Ink)
	}
	if len(m.messages) == 0 {
		t1, t2 := "No messages yet", "Press i or click the box below to say hello."
		c.Text(inner.X+(inner.W-ui.StringWidth(t1))/2, inner.Y+inner.H/2-1, t1, ui.Text, ui.Panel, true)
		c.Text(inner.X+max(0, (inner.W-ui.StringWidth(t2))/2), inner.Y+inner.H/2+1, ui.Truncate(t2, inner.W, "…"), ui.Dim, ui.Panel, false)
		return
	}
	restore := c.Clip(inner)
	defer c.Restore(restore)
	items := m.titems(inner.W)
	lo, hi := window(items, inner.H, m.msgTop, m.messageIndex)
	totalHeight := 0
	for _, item := range items {
		totalHeight += item.h + 1
	}
	totalHeight = max(0, totalHeight-1)
	topAligned := lo == 0 && hi == len(items)-1 && totalHeight <= inner.H
	drawItem := func(it titem, y int) {
		if it.day {
			w := ui.StringWidth(it.label) + 6
			pr := ui.R(inner.X+(inner.W-w)/2, y, w, 3)
			c.Box(pr, ui.Border, ui.Hex(0x0A1F18))
			s.center(pr, it.label, ui.Text, ui.Hex(0x0A1F18), false)
		} else {
			msg := m.messages[it.idx]
			b := m.buildBubble(msg, inner.W)
			sel := m.msgFocus && it.idx == m.messageIndex && m.active == chatScreen && !m.composing
			s.drawBubble(inner, y, b, sel, s.is(fmt.Sprintf("msg:%d", it.idx)))
			s.add(ui.R(inner.X, y, inner.W, it.h), fmt.Sprintf("msg:%d", it.idx))
		}
	}
	if topAligned {
		y := inner.Y
		for i := lo; i <= hi; i++ {
			it := items[i]
			drawItem(it, y)
			y += it.h + 1
		}
	} else {
		y := inner.Bottom()
		for i := hi; i >= lo; i-- {
			it := items[i]
			y -= it.h
			drawItem(it, y)
			y--
		}
	}
	if lo > 0 {
		c.Text(inner.Right()-3, inner.Y, "▲", ui.Accent, 0, true)
	}
	s.scrollbar(inner.Right()-1, inner.Y, inner.H, len(items), hi-lo+1, lo)
}

func (s *scene) drawBubble(inner ui.Rect, y int, b bubble, selected, hover bool) {
	c := s.c
	col := ui.BubbleIn
	if b.own {
		col = ui.BubbleOut
	}
	if selected {
		col = ui.Mix(col, ui.Accent, .16)
	} else if hover {
		col = ui.Mix(col, ui.Accent, .07)
	}
	x := inner.X + 2
	if b.own {
		x = inner.Right() - 2 - b.w
	}
	c.Fill(ui.R(x+1, y, b.w-2, 1), col)
	c.Fill(ui.R(x, y+1, b.w, b.h-2), col)
	c.Fill(ui.R(x+1, y+b.h-1, b.w-2, 1), col)
	if selected {
		for i := 1; i < b.h-1; i++ {
			c.Put(x-1, y+i, '▌', ui.Accent, 0, false)
		}
	}
	row := y + 1
	if b.name != "" {
		c.Text(x+2, row, b.name, ui.Accent, col, true)
		row++
	}
	if b.quote != "" {
		c.Put(x+1, row, '▌', ui.Accent, col, true)
		c.Text(x+2, row, b.quote, ui.Dim, col, false)
		row++
	}
	for i, line := range b.lines {
		c.Text(x+2, row, line, ui.Text, col, false)
		if i == len(b.lines)-1 && b.inline {
			s.meta(x, row, b, col)
		}
		row++
	}
	if !b.inline {
		s.meta(x, row, b, col)
		row++
	}
	if b.reactions != "" {
		c.Text(x+2, row, b.reactions, ui.Accent, col, true)
	}
}

func (s *scene) meta(x, row int, b bubble, col ui.Color) {
	c := s.c
	end := x + b.w - 2
	if b.tick != "" {
		end -= ui.StringWidth(b.tick)
		c.Text(end, row, b.tick, b.tickFG, col, true)
		end--
	}
	end -= ui.StringWidth(b.meta)
	c.Text(end, row, b.meta, ui.Dim, col, false)
}

// ---- composer ----

func (s *scene) composerBar(r ui.Rect) {
	m, c := s.m, s.c
	outer := r
	bordered := r.H >= 6
	if bordered {
		c.Box(r, ui.Border, ui.Panel)
		r = ui.R(r.X+1, r.Y+1, r.W-2, r.H-2)
	} else {
		r = ui.R(r.X+1, r.Y, r.W-2, r.H)
	}
	y := r.Y
	if m.replyTo != nil {
		y = outer.Y + 2
		quote := "Replying to " + m.quotedSender(m.replyTo.Sender) + ": " + ui.SingleLine(m.replyTo.Body)
		quoteY := outer.Y
		if bordered {
			quoteY++
		}
		c.Text(outer.X+2, quoteY, ui.Truncate(quote, outer.W-4, "…"), ui.Accent, ui.Panel, false)
	} else if outer.H >= 5 && !bordered {
		y++
	}
	controlH := min(4, r.Bottom()-y)
	if controlH < 3 {
		controlH = 3
	}
	plus := ui.R(r.X, y, 8, controlH)
	border := ui.Border
	if s.is("plus") {
		border = ui.Accent
	}
	c.Box(plus, border, ui.Panel)
	s.center(plus, "+", ui.Text, ui.Panel, true)
	s.add(plus, "plus")
	send := ui.R(r.Right()-8, y, 8, controlH)
	in := ui.R(plus.Right()+1, y, send.X-plus.Right()-2, controlH)
	ib := ui.Border
	if m.composing || s.is("compose") {
		ib = ui.Accent
	}
	c.Box(in, ib, ui.Panel)
	s.add(in, "compose")
	text, fg := "Type a message…", ui.Dim
	if m.composer != "" || m.composing {
		text, fg = strings.ReplaceAll(ui.SafeText(m.composer), "\n", "↵"), ui.Text
	}
	avail := in.W - 4 - 8
	shown := ui.TailFit(text, avail)
	if m.composer == "" && !m.composing {
		shown = ui.Truncate(shown, avail, "…")
	}
	w := c.Text(in.X+2, y+controlH/2, shown, fg, ui.Panel, false)
	if m.composing && (m.settings.ReducedMotion || m.frame%2 == 0) {
		c.Fill(ui.R(in.X+2+w, y+controlH/2, 1, 1), ui.Accent)
	}
	emoji := ui.R(in.Right()-8, y+controlH/2, 3, 1)
	for _, ic := range []struct {
		r     ui.Rect
		glyph string
		id    string
	}{{emoji, "☺", "emoji"}} {
		fgc := ui.Dim
		if s.is(ic.id) {
			fgc = ui.Accent
		}
		s.c.Text(ic.r.X+1, ic.r.Y, ic.glyph, fgc, ui.Panel, true)
		s.add(ic.r, ic.id)
	}
	c.Text(in.Right()-3, y+controlH/2, "⌁", ui.Dim, ui.Panel, false)
	bg := ui.Accent
	if s.is("send") {
		bg = ui.AccentHi
	}
	if m.sending {
		bg = ui.Mix(ui.Accent, ui.Panel, .5)
	}
	c.Fill(send, bg)
	glyph := "➤"
	if m.sending {
		glyph = "…"
		if !m.settings.ReducedMotion {
			glyph = spinner[m.frame%len(spinner)]
		}
	}
	s.center(send, glyph, ui.AccentDark, bg, true)
	s.add(send, "send")
}

// popups draws the slash-command list and the emoji picker above the composer.
func (s *scene) popups(comp ui.Rect) {
	m, c := s.m, s.c
	if sug := m.slashSuggestions(); len(sug) > 0 {
		n := min(len(sug), 7)
		w := min(comp.W-4, 64)
		r := ui.R(comp.X+2, comp.Y-n-2, w, n+2)
		if r.Y < 1 {
			return
		}
		c.Box(r, ui.Accent, ui.Panel)
		sel := min(m.slashIndex, len(sug)-1)
		start := max(0, min(sel-n+1, len(sug)-n))
		for i := 0; i < n; i++ {
			cmd := sug[start+i]
			bg := ui.Panel
			id := "slash:" + cmd.name
			if start+i == sel || s.is(id) {
				bg = ui.PanelHi
			}
			c.Fill(ui.R(r.X+1, r.Y+1+i, w-2, 1), bg)
			c.Text(r.X+2, r.Y+1+i, cmd.usage, ui.Accent, bg, true)
			c.Text(r.X+2+22, r.Y+1+i, ui.Truncate(cmd.desc, w-26, "…"), ui.Dim, bg, false)
			s.add(ui.R(r.X+1, r.Y+1+i, w-2, 1), id)
		}
	}
}

// ---- footer ----

func (m Model) hints() string {
	switch {
	case m.composing && strings.HasPrefix(m.composer, "/"):
		return "Enter run · Tab complete · ↑↓ choose · // sends a literal slash · Esc close"
	case m.composing:
		return "Enter send · Ctrl+R reply target · Ctrl+J newline · Ctrl+U clear · Esc stop typing"
	case m.searching:
		return "Type to filter · ↑↓ select · Enter open · Esc clear"
	case m.active == chatScreen:
		return "↑↓ select · Ctrl+R reply · Ctrl+E react · i type · a files · /statuses · Tab chats · Esc back"
	}
	return "↑↓ select · Enter open · / commands · n new chat · Ctrl+F search · e archive · s settings · ? help · q quit"
}

func (s *scene) footer(r ui.Rect) {
	m, c := s.m, s.c
	right := "● Offline"
	rfg := ui.Dim
	switch {
	case m.demo:
		right, rfg = "DEMO · no network", ui.Accent
	case m.connection.Online:
		right, rfg = "● Connected", ui.Accent
	case m.connection.Status != "" && m.connection.Status != "Offline":
		right, rfg = "◌ "+m.connection.Status, ui.Dim
	}
	c.Fill(r, ui.BG)
	rw := ui.StringWidth(right)
	c.Text(r.Right()-rw-1, r.Y, right, rfg, ui.BG, true)
	left, lfg := m.hints(), ui.Faint
	if m.notice != "" && m.active == m.noticeScreen {
		left, lfg = m.notice, ui.Accent
		if m.noticeErr {
			lfg = ui.Danger
		}
	} else if m.connection.Error != "" {
		left, lfg = m.connection.Error, ui.Danger
	}
	c.Text(r.X+1, r.Y, ui.Truncate(ui.SingleLine(left), max(1, r.W-rw-4), "…"), lfg, ui.BG, false)
}
