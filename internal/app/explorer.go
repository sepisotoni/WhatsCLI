package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"whatscli/internal/history"
	"whatscli/internal/messaging"
	"whatscli/internal/ui"
)

type dirEntry struct {
	name, path string
	dir, link  bool
	size       int64
	mod        time.Time
}

type explorer struct {
	dir       string
	all       []dirEntry
	cursor    int
	top       int
	selected  map[string]bool
	order     []string
	hidden    bool
	filter    string
	filtering bool
	sortBy    int // 0 name, 1 size, 2 modified
	typing    bool
	pathInput string
	err       string
	loading   bool
	seq       int
	truncated bool
}

type dirMsg struct {
	seq       int
	dir       string
	entries   []dirEntry
	truncated bool
	err       error
}

type uploadItem struct {
	path, name string
	size       int64
	folder     bool
	files      int
	skipped    int
}

type planMsg struct {
	items []uploadItem
	err   error
}

type uploadMsg struct {
	index int
	err   error
}

const maxDirEntries = 20000

func (m *Model) openExplorer() tea.Cmd {
	if m.chatID == "" {
		m.setNotice("Open a chat first, then choose files to send to it.", true)
		return nil
	}
	if m.sending || m.uploading {
		m.setNotice("Wait for the current send to finish.", true)
		return nil
	}
	if !m.settings.Accepted() {
		return nil
	}
	dir := m.lastDir
	if dir == "" {
		dir, _ = os.Getwd()
	}
	if dir == "" {
		dir, _ = os.UserHomeDir()
	}
	if dir == "" {
		dir = "/"
	}
	m.explorer = &explorer{selected: map[string]bool{}}
	m.active = explorerScreen
	m.composing = false
	return m.explorerGo(dir)
}

func (m *Model) explorerGo(dir string) tea.Cmd {
	e := m.explorer
	e.seq++
	e.loading, e.err = true, ""
	seq := e.seq
	return func() tea.Msg {
		abs, err := filepath.Abs(dir)
		if err != nil {
			return dirMsg{seq: seq, dir: dir, err: err}
		}
		entries, err := os.ReadDir(abs)
		if err != nil {
			return dirMsg{seq: seq, dir: abs, err: err}
		}
		out := make([]dirEntry, 0, min(len(entries), maxDirEntries))
		truncated := false
		for _, entry := range entries {
			if len(out) >= maxDirEntries {
				truncated = true
				break
			}
			info, err := entry.Info()
			if err != nil {
				continue
			}
			de := dirEntry{name: entry.Name(), path: filepath.Join(abs, entry.Name()), size: info.Size(), mod: info.ModTime()}
			if info.Mode()&os.ModeSymlink != 0 {
				de.link = true
				if target, err := os.Stat(de.path); err == nil {
					de.dir = target.IsDir()
				}
			} else {
				de.dir = info.IsDir()
			}
			out = append(out, de)
		}
		return dirMsg{seq: seq, dir: abs, entries: out, truncated: truncated}
	}
}

func (m *Model) explorerLoaded(msg dirMsg) tea.Cmd {
	e := m.explorer
	if e == nil || msg.seq != e.seq {
		return nil
	}
	e.loading = false
	if msg.err != nil {
		e.err = "Cannot open that folder."
		return nil
	}
	e.dir, e.all, e.truncated = msg.dir, msg.entries, msg.truncated
	e.cursor, e.top, e.filter, e.filtering = 0, 0, "", false
	m.lastDir = msg.dir
	return nil
}

// view returns the visible, sorted entries; ".." is always first when it exists.
func (e *explorer) view() []dirEntry {
	var out []dirEntry
	if filepath.Dir(e.dir) != e.dir {
		out = append(out, dirEntry{name: "..", path: filepath.Dir(e.dir), dir: true})
	}
	q := strings.ToLower(e.filter)
	var rest []dirEntry
	for _, d := range e.all {
		if !e.hidden && strings.HasPrefix(d.name, ".") {
			continue
		}
		if q != "" && !strings.Contains(strings.ToLower(d.name), q) {
			continue
		}
		rest = append(rest, d)
	}
	sort.SliceStable(rest, func(i, j int) bool {
		a, b := rest[i], rest[j]
		if a.dir != b.dir {
			return a.dir
		}
		switch e.sortBy {
		case 1:
			if a.size != b.size {
				return a.size > b.size
			}
		case 2:
			if !a.mod.Equal(b.mod) {
				return a.mod.After(b.mod)
			}
		}
		return strings.ToLower(a.name) < strings.ToLower(b.name)
	})
	return append(out, rest...)
}

func (e *explorer) toggle(d dirEntry) string {
	if d.name == ".." {
		return ""
	}
	if d.link {
		return "Symlinks are never uploaded; choose the real file or folder."
	}
	if e.selected[d.path] {
		delete(e.selected, d.path)
		for i, p := range e.order {
			if p == d.path {
				e.order = append(e.order[:i], e.order[i+1:]...)
				break
			}
		}
		return ""
	}
	if len(e.order) >= 200 {
		return "Select at most 200 items per upload."
	}
	e.selected[d.path] = true
	e.order = append(e.order, d.path)
	return ""
}

func (e *explorer) clamp(rows int) {
	n := len(e.view())
	e.cursor = max(0, min(e.cursor, n-1))
	if e.cursor < e.top {
		e.top = e.cursor
	}
	if e.cursor >= e.top+rows {
		e.top = e.cursor - rows + 1
	}
	e.top = max(0, min(e.top, max(0, n-rows)))
}

// explorerChrome is the number of modal rows that are not file rows.
const explorerChrome = 12

func (m *Model) explorerRows() int { return max(3, m.height-4-explorerChrome) }

func (m *Model) explorerKey(key tea.KeyMsg) tea.Cmd {
	e := m.explorer
	if e == nil {
		m.active = chatScreen
		return nil
	}
	k := key.String()
	rows := m.explorerRows()
	entries := e.view()
	cur := func() (dirEntry, bool) {
		if e.cursor >= 0 && e.cursor < len(entries) {
			return entries[e.cursor], true
		}
		return dirEntry{}, false
	}
	if e.typing {
		switch k {
		case "esc":
			e.typing = false
		case "backspace":
			e.pathInput = dropRune(e.pathInput)
		case "enter":
			e.typing = false
			p := strings.TrimSpace(e.pathInput)
			if strings.HasPrefix(p, "~") {
				if home, err := os.UserHomeDir(); err == nil {
					p = filepath.Join(home, strings.TrimPrefix(p, "~"))
				}
			}
			if info, err := os.Stat(p); err == nil && info.IsDir() {
				return m.explorerGo(p)
			} else if err == nil {
				dir := filepath.Dir(p)
				m.setNotice("", false)
				e.pendingSelect(p)
				return m.explorerGo(dir)
			}
			m.setNotice("That path does not exist on this host.", true)
		default:
			if isText(key) && len(e.pathInput) < 4096 {
				e.pathInput += ui.SingleLine(string(key.Runes))
			}
		}
		return nil
	}
	if e.filtering {
		switch k {
		case "esc":
			e.filter, e.filtering, e.cursor = "", false, 0
		case "enter":
			e.filtering = false
		case "up":
			e.cursor--
		case "down":
			e.cursor++
		case "backspace":
			e.filter, e.cursor = dropRune(e.filter), 0
		default:
			if isText(key) && len([]rune(e.filter)) < 60 {
				e.filter, e.cursor = e.filter+ui.SingleLine(string(key.Runes)), 0
			}
		}
		e.clamp(rows)
		return nil
	}
	switch k {
	case "esc":
		if e.filter != "" {
			e.filter = ""
			e.clamp(rows)
			return nil
		}
		m.active, m.explorer = chatScreen, nil
	case "up", "k":
		e.cursor--
	case "down", "j":
		e.cursor++
	case "pgup":
		e.cursor -= rows
	case "pgdown":
		e.cursor += rows
	case "home":
		e.cursor = 0
	case "end":
		e.cursor = len(entries) - 1
	case "enter", "right", "l":
		if d, ok := cur(); ok {
			if d.dir {
				return m.explorerGo(d.path)
			}
			if k == "enter" {
				m.setNotice(e.toggle(d), true)
			}
		}
	case "left", "h", "backspace":
		return m.explorerGo(filepath.Dir(e.dir))
	case " ":
		if d, ok := cur(); ok {
			m.setNotice(e.toggle(d), true)
			e.cursor++
		}
	case "a":
		for _, d := range e.view() {
			if !d.dir && !d.link && !e.selected[d.path] {
				e.toggle(d)
			}
		}
	case "x":
		e.selected, e.order = map[string]bool{}, nil
	case ".":
		e.hidden = !e.hidden
	case "/":
		e.filtering = true
	case "ctrl+l", "g":
		e.typing, e.pathInput = true, e.dir+string(filepath.Separator)
	case "~":
		if home, err := os.UserHomeDir(); err == nil {
			return m.explorerGo(home)
		}
	case "s":
		e.sortBy = (e.sortBy + 1) % 3
	case "u", "ctrl+s":
		return m.planUpload()
	}
	e.clamp(rows)
	return nil
}

// pendingSelect marks a typed file path for selection once its folder loads.
func (e *explorer) pendingSelect(p string) {
	if !e.selected[p] {
		e.selected[p] = true
		e.order = append(e.order, p)
	}
}

// planUpload inspects the selection (folders are counted) before confirmation.
func (m *Model) planUpload() tea.Cmd {
	e := m.explorer
	if len(e.order) == 0 {
		entries := e.view()
		if e.cursor < len(entries) && entries[e.cursor].name != ".." {
			if msg := e.toggle(entries[e.cursor]); msg != "" {
				m.setNotice(msg, true)
				return nil
			}
		}
	}
	if len(e.order) == 0 {
		m.setNotice("Select files or folders with Space first.", true)
		return nil
	}
	paths := append([]string(nil), e.order...)
	m.setNotice("Inspecting selection…", false)
	return func() tea.Msg {
		var items []uploadItem
		for _, p := range paths {
			info, err := os.Lstat(p)
			if err != nil {
				return planMsg{err: fmt.Errorf("%s is no longer readable", filepath.Base(p))}
			}
			switch {
			case info.IsDir():
				fi, err := messaging.InspectFolder(p)
				if err != nil {
					return planMsg{err: fmt.Errorf("%s: %v", filepath.Base(p), err)}
				}
				if fi.Files == 0 {
					return planMsg{err: fmt.Errorf("%s contains no uploadable files", filepath.Base(p))}
				}
				items = append(items, uploadItem{path: p, name: filepath.Base(p) + ".zip", size: fi.Bytes, folder: true, files: fi.Files, skipped: fi.Skipped})
			default:
				att, err := messaging.InspectAttachment(p)
				if err != nil {
					return planMsg{err: fmt.Errorf("%s: %v", filepath.Base(p), err)}
				}
				items = append(items, uploadItem{path: p, name: att.Name, size: att.Size})
			}
		}
		return planMsg{items: items}
	}
}

func (m *Model) planReady(msg planMsg) tea.Cmd {
	if m.active != explorerScreen {
		return nil
	}
	if msg.err != nil {
		m.setNotice(msg.err.Error(), true)
		return nil
	}
	m.queue, m.queueDone, m.uploadErr, m.scroll = msg.items, 0, "", 0
	m.active = confirmFileScreen
	m.setNotice("", false)
	return nil
}

func (m *Model) startUpload() tea.Cmd {
	if len(m.queue) == 0 {
		return nil
	}
	if !m.demo && (m.runtime == nil || !m.connection.Online) {
		m.setNotice("Not connected; nothing sent.", true)
		return nil
	}
	m.uploading, m.sending, m.queueDone, m.uploadErr = true, true, 0, ""
	m.active, m.explorer = chatScreen, nil
	return m.uploadStep()
}

func (m *Model) uploadStep() tea.Cmd {
	i := m.queueDone
	item := m.queue[i]
	m.setNotice(fmt.Sprintf("Uploading %d of %d · %s", i+1, len(m.queue), item.name), false)
	ctx, cancel := context.WithTimeout(m.ctx, 5*time.Minute)
	m.sendCancel = cancel
	runtime, chat, title, demo, repo, account := m.runtime, m.chatID, m.chatTitle, m.demo, m.repo, m.account
	at := time.Now().UTC()
	if len(m.messages) > 0 && !m.messages[0].CreatedAt.Before(at) {
		at = m.messages[0].CreatedAt.Add(time.Minute)
	}
	return func() tea.Msg {
		defer cancel()
		path := item.path
		if item.folder {
			dir, err := os.MkdirTemp("", "whatscli-upload-")
			if err != nil {
				return uploadMsg{i, fmt.Errorf("could not prepare the archive")}
			}
			defer os.RemoveAll(dir)
			archive, _, err := messaging.ZipFolder(item.path, dir)
			if err != nil {
				return uploadMsg{i, err}
			}
			path = archive
		}
		if demo {
			err := repo.ApplyEvent(ctx, history.IncomingEvent{
				Kind: history.MessageEvent, AccountID: account, ChatID: chat, Title: title,
				MessageID: fmt.Sprintf("demo-file-%d-%d", time.Now().UnixNano(), i), Sender: "You",
				Body: "[document: " + item.name + "]", Timestamp: at, FromMe: true, Status: "sent",
			})
			return uploadMsg{i, err}
		}
		return uploadMsg{i, runtime.Send(ctx, chat, "[document] "+item.name, path)}
	}
}

func (m *Model) uploadDone(msg uploadMsg) tea.Cmd {
	if !m.uploading || msg.index != m.queueDone {
		return nil
	}
	m.sendCancel = nil
	if msg.err != nil {
		m.uploading, m.sending = false, false
		m.uploadErr = msg.err.Error()
		m.setNotice(fmt.Sprintf("Stopped at %s (%d of %d sent): %s", m.queue[msg.index].name, msg.index, len(m.queue), msg.err.Error()), true)
		return m.loadMessages(m.messageBefore)
	}
	m.queueDone++
	if m.queueDone >= len(m.queue) {
		m.uploading, m.sending = false, false
		m.setNotice(fmt.Sprintf("Sent %d item(s) as documents.", len(m.queue)), false)
		m.queue = nil
		return m.loadMessages(m.messageBefore)
	}
	return tea.Batch(m.loadMessages(m.messageBefore), m.uploadStep())
}

func humanSize(n int64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(n)/(1<<10))
	}
	return fmt.Sprintf("%d B", n)
}
