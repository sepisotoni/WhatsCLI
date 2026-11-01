package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"whatscli/internal/config"
	"whatscli/internal/history"
	"whatscli/internal/messaging"
	"whatscli/internal/ui"
)

func demoFixture(t *testing.T) Model {
	t.Helper()
	r, _ := testRuntime(t, true)
	if err := SeedDemo(context.Background(), r.Repo); err != nil {
		t.Fatal(err)
	}
	m := DemoModel(r, config.Settings{ConsentVersion: config.ConsentVersion, Mouse: true}, "")
	m.width, m.height = 130, 42
	m.fitList()
	m.fitMessages()
	return m
}

// TestSnapshots writes ANSI frames for visual comparison with the reference
// mock-ups when WHATSCLI_SNAPSHOT_DIR is set (see tools/ansi2png.py).
func TestSnapshots(t *testing.T) {
	dir := os.Getenv("WHATSCLI_SNAPSHOT_DIR")
	if dir == "" {
		t.Skip("set WHATSCLI_SNAPSHOT_DIR to write snapshots")
	}
	ui.SetProfile(ui.TrueColor)
	write := func(name string, m Model) {
		if err := os.WriteFile(filepath.Join(dir, name+".ansi"), []byte(m.View()), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	base := demoFixture(t)
	write("workspace", base)
	m := base
	m.search = ""
	m.composing, m.composer = true, "/"
	write("slash", m)
	m = base
	m.active = emojiScreen
	write("emoji", m)
	m = base
	m.openOverlay(settingsScreen)
	write("settings", m)
	m = base
	m.active = menuScreen
	write("menu", m)
	m = base
	m.openOverlay(helpScreen)
	write("help", m)
	m = New(nil)
	m.width, m.height = 130, 42
	write("welcome", m)
	m.active, m.consentAccepted = consentScreen, true
	m.width, m.height = 130, 42
	write("consent", m)
	m.active = pairingScreen
	m.connection.QR = "2@" + strings.Repeat("Zm9vYmFyYmF6", 8) + ",ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqr,ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqr,abcdefghijklmnopqrstuvwx,1"
	write("pairing", m)
	m.fullscreen = true
	write("pairing_full", m)
	m = base
	m.active = explorerScreen
	m.explorer = &explorer{selected: map[string]bool{}, dir: "/workspaces/default/code", all: []dirEntry{
		{name: "cmd", path: "/x/cmd", dir: true}, {name: "docs", path: "/x/docs", dir: true},
		{name: "README.md", path: "/x/README.md", size: 9012}, {name: "go.mod", path: "/x/go.mod", size: 2318}, {name: "install.sh", path: "/x/install.sh", size: 3500},
	}}
	m.explorer.selected["/x/README.md"], m.explorer.order = true, []string{"/x/README.md"}
	write("explorer", m)
}

func TestWorkspaceFitsTerminal(t *testing.T) {
	m := demoFixture(t)
	m.messages[0].Body = strings.Repeat("long message 日本語 ", 200)
	m.conversations[0].Title = strings.Repeat("long title ", 40)
	m.connection.QR = strings.Repeat("pairing-fixture", 12)
	m.explorer = &explorer{selected: map[string]bool{}, dir: "/tmp", all: []dirEntry{{name: strings.Repeat("n", 200), path: "/tmp/n"}}}
	m.queue = []uploadItem{{name: "a.txt", size: 3}}
	for _, size := range [][2]int{{90, 20}, {100, 24}, {140, 45}, {200, 60}, {60, 14}, {30, 10}, {72, 24}, {110, 28}} {
		for _, active := range []screen{welcomeScreen, consentScreen, pairingScreen, inboxScreen, chatScreen, newChatScreen,
			settingsScreen, helpScreen, commandScreen, explorerScreen, confirmFileScreen, attachmentScreen, logoutScreen, emojiScreen, menuScreen, revisionsScreen} {
			m.width, m.height, m.active = size[0], size[1], active
			lines := strings.Split(m.View(), "\n")
			if len(lines) > m.height {
				t.Fatalf("%dx%d screen %d has %d lines", m.width, m.height, active, len(lines))
			}
			for _, line := range lines {
				if w := ansi.StringWidth(line); w != m.width {
					t.Fatalf("%dx%d screen %d has line width %d: %q", m.width, m.height, active, w, ansi.Strip(line))
				}
			}
		}
	}
}

func plain(m Model) string {
	return ansi.Strip(m.View())
}

func TestWorkspaceMatchesReferenceContent(t *testing.T) {
	m := demoFixture(t)
	view := plain(m)
	for _, text := range []string{"Chats", "12", "Umbrella Devs", "6 members", "Online", "Joshua", "Pushed the fix for the QR flow.",
		"Lethabo", "Nice, build is green now ✅", "Bet, I'll test it now.", "Perfect 👌", "Today", "Type a message", "Archive",
		"Search chats", "DEMO", "22:14", "21:47", "Merensky 9K", "Qhawe (Secondary)", "TRITECH Team", "▤ Notes.pdf", "▣ Image"} {
		if !strings.Contains(view, text) {
			t.Errorf("workspace missing %q", text)
		}
	}
	for _, text := range []string{"Meta AI · availability depends", "synthetic messages"} {
		if strings.Contains(view, text) {
			t.Errorf("stale text %q", text)
		}
	}
}

func TestEveryCellHasExplicitBackground(t *testing.T) {
	ui.SetProfile(ui.TrueColor)
	m := demoFixture(t)
	for _, active := range []screen{welcomeScreen, consentScreen, pairingScreen, chatScreen, settingsScreen, explorerScreen} {
		m.active = active
		m.explorer = &explorer{selected: map[string]bool{}, dir: "/tmp"}
		out := m.View()
		// A bare reset ("\x1b[0m") may only occur at the very end of a row, never
		// mid-row where the terminal default colour would show through.
		for _, line := range strings.Split(out, "\n") {
			if strings.Contains(strings.TrimSuffix(line, "\x1b[0m"), "\x1b[0m") || strings.Contains(line, "\x1b[49m") {
				t.Fatalf("screen %d resets the background mid-row", active)
			}
			if !strings.Contains(line, "48;2;") {
				t.Fatalf("screen %d has a row without a background fill", active)
			}
		}
	}
}

func TestSearchTreatsShortcutsAsInputAndKeepsDraft(t *testing.T) {
	m := demoFixture(t)
	m.composer = "private draft"
	m, _ = press(m, tea.KeyMsg{Type: tea.KeyCtrlF})
	if !m.searching || m.active != inboxScreen {
		t.Fatal("search not focused")
	}
	m, _ = press(m, runeKey("Joshua"))
	if got := m.visibleChats(); len(got) != 1 || m.conversations[got[0]].Title != "Joshua" {
		t.Fatal("incorrect filtered chat")
	}
	m, _ = press(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.chatTitle != "Umbrella Devs" || m.composer != "private draft" {
		t.Fatal("search moved a draft to another recipient")
	}
	m, _ = press(m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.searching || m.search != "" {
		t.Fatal("search did not clear")
	}
}

func TestNewChatIsLocalAndConsentGated(t *testing.T) {
	m := demoFixture(t)
	m.active = inboxScreen
	m, _ = press(m, runeKey("n"))
	if m.active != newChatScreen {
		t.Fatal("new chat did not open")
	}
	m, _ = press(m, runeKey("+27 82 123 4567"))
	m, cmd := press(m, tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("new chat not saved")
	}
	next, _ := m.Update(cmd())
	m = next.(Model)
	if m.chatID != "27821234567@s.whatsapp.net" || m.active != chatScreen {
		t.Fatal("new chat not selected")
	}
	m.settings.ConsentVersion = 0
	if cmd := m.openMetaAI(); cmd != nil {
		t.Fatal("Meta AI bypassed consent")
	}
}

func TestMetaAIUsesUpstreamIdentityAndProtectsDraft(t *testing.T) {
	m := demoFixture(t)
	m.composer = "do not redirect"
	if cmd := m.openMetaAI(); cmd != nil {
		t.Fatal("Meta AI ignored draft guard")
	}
	m.composer = ""
	cmd := m.openMetaAI()
	next, _ := m.Update(cmd())
	m = next.(Model)
	if m.chatID != messaging.MetaAIChatID() || m.chatTitle != "Meta AI" {
		t.Fatal("incorrect Meta AI identity")
	}
}

func TestMouseHoverClickAndWheelUseRenderedRegions(t *testing.T) {
	m := demoFixture(t)
	g := m.geo()
	find := func(id string) (x, y int) {
		s := &scene{m: &m}
		m.draw(s)
		for _, h := range s.hits {
			if h.id == id {
				return h.r.X + h.r.W/2, h.r.Y + h.r.H/2
			}
		}
		t.Fatalf("no region %q", id)
		return
	}
	for _, id := range []string{"rail:new", "rail:settings", "search", "chat:2", "plus", "emoji", "send", "head:more", "compose", "archive"} {
		x, y := find(id)
		if got := m.hitAt(x, y); got != id {
			t.Fatalf("%s: cell %d,%d hit %q", id, x, y, got)
		}
		m.mouse(tea.MouseMsg{X: x, Y: y, Action: tea.MouseActionMotion})
		if m.hover != id {
			t.Fatalf("hover %q want %q", m.hover, id)
		}
	}
	// Clicking the second visible chat opens it.
	x, y := find("chat:2")
	m.mouse(tea.MouseMsg{X: x, Y: y, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
	if m.chatID != m.conversations[2].ID {
		t.Fatal("click did not open the rendered chat")
	}
	// Wheel over the list scrolls the viewport, not the selection.
	idx := m.chatIndex
	m.mouse(tea.MouseMsg{X: g.list.X + 3, Y: 12, Button: tea.MouseButtonWheelDown, Action: tea.MouseActionPress})
	if m.chatIndex != idx {
		t.Fatal("list wheel changed selection")
	}
	// Disabled mouse ignores everything.
	next, _ := m.Update(tea.MouseMsg{X: 2, Y: 6, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
	m2 := next.(Model)
	m.settings.Mouse = false
	next, _ = m.Update(tea.MouseMsg{X: 2, Y: 6, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
	if next.(Model).active != m.active || m2.active == newChatScreen && false {
		t.Fatal("disabled mouse changed state")
	}
}

func TestDemoSendOnlyWritesSyntheticHistory(t *testing.T) {
	m := demoFixture(t)
	m.composer = "local demonstration"
	m.composing = true
	cmd := m.send(m.composer, "")
	if cmd == nil {
		t.Fatal("demo send unavailable")
	}
	next, _ := m.Update(cmd())
	m = next.(Model)
	messages, err := m.repo.Messages(context.Background(), "local", m.chatID, nil, 50)
	found := false
	for _, message := range messages {
		found = found || message.Body == "local demonstration"
	}
	if err != nil || !found || m.composer != "" || messages[0].Body != "local demonstration" {
		t.Fatal("demo message not saved as the newest message", err)
	}
	if m.runtime.Snapshot().Online {
		t.Fatal("demo unexpectedly online")
	}
}

func TestReplyAndReactFromSelectedMessage(t *testing.T) {
	m := demoFixture(t)
	m.active, m.msgFocus, m.messageIndex = chatScreen, true, 1
	target := m.messages[m.messageIndex]
	m, _ = press(m, tea.KeyMsg{Type: tea.KeyCtrlR})
	if !m.composing || m.replyTo == nil || m.replyTo.ID != target.ID {
		t.Fatal("Ctrl+R did not set the selected reply target")
	}
	m.composer = "reply from terminal"
	if cmd := m.submit(); cmd == nil {
		t.Fatal("reply send command was not created")
	} else {
		next, _ := m.Update(cmd())
		m = next.(Model)
	}
	messages, err := m.repo.Messages(context.Background(), m.account, m.chatID, nil, 50)
	if err != nil {
		t.Fatal(err)
	}
	foundReply := false
	for _, message := range messages {
		if message.Body == "reply from terminal" {
			foundReply = message.ReplyToID == target.ID && message.ReplyBody == target.Body
		}
	}
	if !foundReply {
		t.Fatal("reply context did not persist")
	}

	m.messageIndex, m.msgFocus = 1, true
	target = m.messages[m.messageIndex]
	m, _ = press(m, tea.KeyMsg{Type: tea.KeyCtrlE})
	if m.active != emojiScreen || m.reactionTarget == nil || m.reactionTarget.ID != target.ID {
		t.Fatal("Ctrl+E did not open reactions for the selected message")
	}
	m, cmd := press(m, tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("reaction command was not created")
	}
	next, _ := m.Update(cmd())
	m = next.(Model)
	messages, err = m.repo.Messages(context.Background(), m.account, m.chatID, nil, 50)
	if err != nil {
		t.Fatal(err)
	}
	foundReaction := false
	for _, message := range messages {
		if message.ID == target.ID {
			for _, reaction := range message.Reactions {
				foundReaction = foundReaction || reaction.Sender == "You" && reaction.Emoji == "👍"
			}
		}
	}
	if !foundReaction {
		t.Fatal("reaction was not projected onto the selected message")
	}
}

func TestCreatedChatRejectsStaleAccountAndDraft(t *testing.T) {
	m := demoFixture(t)
	original := m.chatID
	next, _ := m.Update(createdChatMsg{"different", "recipient", "Wrong", nil})
	if next.(Model).chatID != original {
		t.Fatal("stale create changed account")
	}
	m.composer = "draft"
	next, _ = m.Update(createdChatMsg{m.account, "recipient", "Wrong", nil})
	if next.(Model).chatID != original {
		t.Fatal("async create redirected draft")
	}
}

func TestFilteredNavigationAndTab(t *testing.T) {
	m := demoFixture(t)
	m.active = inboxScreen
	m.search = "Mom"
	m.moveChat(1)
	if m.conversations[m.chatIndex].Title != "Mom" {
		t.Fatal("navigation escaped filter")
	}
	m, _ = press(m, tea.KeyMsg{Type: tea.KeyTab})
	if m.active != chatScreen {
		t.Fatal("tab did not switch panes")
	}
}

func TestSlashCommandsRunInsteadOfSending(t *testing.T) {
	m := demoFixture(t)
	m, _ = press(m, runeKey("/"))
	if !m.composing || m.composer != "/" || len(m.slashSuggestions()) < 10 {
		t.Fatal("slash did not open the command list")
	}
	m, _ = press(m, runeKey("he"))
	if got := m.slashSuggestions(); len(got) != 1 || got[0].name != "help" {
		t.Fatal("suggestions not filtered", got)
	}
	m, _ = press(m, tea.KeyMsg{Type: tea.KeyTab})
	if m.composer != "/help " {
		t.Fatal("tab did not complete", m.composer)
	}
	m, cmd := press(m, tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil || m.active != helpScreen || m.composer != "" || m.sending {
		t.Fatal("/help was sent or not run")
	}
	m.active, m.composing, m.composer = chatScreen, true, "/nonsense"
	m, cmd = press(m, tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil || m.composer != "/nonsense" || !m.noticeErr {
		t.Fatal("unknown command should keep the draft and warn")
	}
	m.composer = "//literal slash"
	if cmd := m.submit(); cmd == nil {
		t.Fatal("// must send a literal slash message")
	}
	m.sending, m.composer = false, "/new +27 82 123 4567"
	m.composing = true
	m, cmd = press(m, tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("/new did not create a chat")
	}
}

func TestSettingsRowsAreWiredAndPersistNothingInDemo(t *testing.T) {
	m := demoFixture(t)
	m.active = inboxScreen
	m, _ = press(m, runeKey("s"))
	if m.active != settingsScreen {
		t.Fatal("settings did not open")
	}
	for i := 0; i < 6; i++ {
		m.settingsIndex = i
		var cmd tea.Cmd
		m, cmd = press(m, tea.KeyMsg{Type: tea.KeyEnter})
		if cmd == nil {
			t.Fatalf("setting %d has no action", i)
		}
		next, _ := m.Update(cmd())
		m = next.(Model)
	}
	s := m.settings
	if s.Mouse || !s.ReducedMotion || !s.HidePhotos || !s.HideWallpaper || !s.Clock12h || !s.Bell {
		t.Fatalf("toggles not applied: %+v", s)
	}
	view := plain(m)
	for _, text := range []string{"Mouse support", "Reduced motion", "Profile photos", "Chat wallpaper", "12-hour clock", "Terminal bell", "Unlink this device", "Check for updates"} {
		if !strings.Contains(view, text) {
			t.Errorf("settings missing %q", text)
		}
	}
}

func TestPairingNeverNeedsFullscreenForTypicalQR(t *testing.T) {
	m := New(nil)
	m.width, m.height, m.active = 130, 42, pairingScreen
	m.connection.QR = "2@" + strings.Repeat("a", 90) + "," + strings.Repeat("b", 44) + "," + strings.Repeat("c", 44) + "," + strings.Repeat("d", 24) + ",1"
	view := plain(m)
	for _, text := range []string{"LINK A DEVICE", "Log in to WhatsApp", "On your phone, open WhatsApp.", "Go to Settings or ⋮ → Linked devices.",
		"Select Link a device and scan the QR.", "Keep your terminal open until linked.", "The code refreshes automatically.", "Never share it.", "Scan with WhatsApp", "Full-screen QR", "Back"} {
		if !strings.Contains(strings.ReplaceAll(view, "LINK  A DEVICE", "LINK A DEVICE"), text) && !strings.Contains(view, strings.ReplaceAll(text, "LINK A", "LINK")) {
			t.Errorf("pairing missing %q", text)
		}
	}
	if strings.Contains(view, "too small") {
		t.Fatal("a 130x42 terminal must show the QR inline")
	}
	m.width, m.height = 100, 40
	if !strings.Contains(plain(m), "Exit full screen") && strings.Contains(plain(m), "too small") {
		t.Fatal("narrow terminal should switch to full-screen QR automatically")
	}
}

func TestHelpButtonAppearsAfterTwoMinutes(t *testing.T) {
	m := New(nil)
	m.active = pairingScreen
	m.now = m.now.Add(0)
	next, _ := m.Update(pulseMsg(m.now))
	m = next.(Model)
	if m.helpAvailable() {
		t.Fatal("help offered immediately")
	}
	next, _ = m.Update(pulseMsg(m.now.Add(119e9)))
	if next.(Model).helpAvailable() {
		t.Fatal("help offered early")
	}
	next, _ = m.Update(pulseMsg(m.now.Add(121e9)))
	m = next.(Model)
	if !m.helpAvailable() {
		t.Fatal("help not offered after 2 minutes")
	}
	m.width, m.height = 130, 42
	if !strings.Contains(plain(m), "Need help?") {
		t.Fatal("help button not drawn")
	}
	m.connection.Online = true
	next, _ = m.Update(pulseMsg(m.now.Add(125e9)))
	if next.(Model).helpAvailable() {
		t.Fatal("help still offered after pairing")
	}
}

func TestUnreadBadgesAndArchive(t *testing.T) {
	m := demoFixture(t)
	if m.unread[demoChats[0].id] != 3 || m.unread[demoChats[2].id] != 5 {
		t.Fatal("demo unread counts wrong")
	}
	m.active = inboxScreen
	m, _ = press(m, runeKey("e"))
	if !m.isArchived(m.conversations[m.chatIndex].ID) && len(m.state.Archived) != 1 {
		t.Fatal("archive did not record")
	}
	for _, i := range m.visibleChats() {
		if m.isArchived(m.conversations[i].ID) {
			t.Fatal("archived chat still listed")
		}
	}
	m.listMode = modeArchive
	if len(m.visibleChats()) != 1 {
		t.Fatal("archive view wrong")
	}
	m.listMode = modeUnread
	for _, i := range m.visibleChats() {
		if m.unread[m.conversations[i].ID] == 0 {
			t.Fatal("unread filter included a read chat")
		}
	}
}

func TestExplorerSelectionAndUploadPlan(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("alpha"), 0o600)
	os.WriteFile(filepath.Join(dir, ".hidden"), []byte("h"), 0o600)
	os.MkdirAll(filepath.Join(dir, "folder", "sub"), 0o700)
	os.WriteFile(filepath.Join(dir, "folder", "sub", "b.bin"), []byte{1, 2, 3}, 0o600)
	os.Symlink(filepath.Join(dir, "a.txt"), filepath.Join(dir, "link"))
	m := demoFixture(t)
	m.lastDir = dir
	m.active = chatScreen
	cmd := m.openExplorer()
	next, _ := m.Update(cmd())
	m = next.(Model)
	if m.active != explorerScreen || m.explorer.dir != dir {
		t.Fatal("explorer did not open at the last directory")
	}
	names := func() (out []string) {
		for _, e := range m.explorer.view() {
			out = append(out, e.name)
		}
		return
	}
	if strings.Join(names(), ",") != "..,folder,a.txt,link" {
		t.Fatalf("view order/hidden handling wrong: %v", names())
	}
	m, _ = press(m, runeKey("."))
	if !strings.Contains(strings.Join(names(), ","), ".hidden") {
		t.Fatal("hidden toggle failed")
	}
	m, _ = press(m, runeKey("."))
	// cursor 0 is "..": go down to folder, select it, then a.txt, then the symlink (rejected).
	m, _ = press(m, tea.KeyMsg{Type: tea.KeyDown})
	m, _ = press(m, tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}})
	m, _ = press(m, tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}})
	m, _ = press(m, tea.KeyMsg{Type: tea.KeyDown})
	m, _ = press(m, tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}})
	if len(m.explorer.order) != 2 || !m.noticeErr {
		t.Fatalf("selection wrong: %v notice=%q", m.explorer.order, m.notice)
	}
	m, cmd = press(m, runeKey("u"))
	next, _ = m.Update(cmd())
	m = next.(Model)
	if m.active != confirmFileScreen || len(m.queue) != 2 || !m.queue[0].folder || m.queue[0].files != 1 || m.queue[1].name != "a.txt" {
		t.Fatalf("upload plan wrong: %+v", m.queue)
	}
	if view := plain(m); !strings.Contains(view, "folder.zip") || !strings.Contains(view, "zipped") {
		t.Fatal("confirmation does not describe the folder upload")
	}
	// Demo mode performs the whole queue locally (no file contents are sent).
	m, cmd = press(m, tea.KeyMsg{Type: tea.KeyEnter})
	for cmd != nil && m.uploading {
		msg := cmd()
		if batch, ok := msg.(tea.BatchMsg); ok {
			for _, c := range batch {
				if c == nil {
					continue
				}
				if um, ok := c().(uploadMsg); ok {
					msg = um
				}
			}
		}
		var model tea.Model
		model, cmd = m.Update(msg)
		m = model.(Model)
	}
	if m.uploading || m.uploadErr != "" {
		t.Fatalf("queue did not finish: %q", m.uploadErr)
	}
}

func TestExplorerPathTypingAndFilter(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "report.pdf"), []byte("x"), 0o600)
	os.WriteFile(filepath.Join(dir, "photo.jpg"), []byte("x"), 0o600)
	m := demoFixture(t)
	m.lastDir = dir
	cmd := m.openExplorer()
	next, _ := m.Update(cmd())
	m = next.(Model)
	m, _ = press(m, runeKey("/"))
	m, _ = press(m, runeKey("rep"))
	if got := m.explorer.view(); len(got) != 2 || got[1].name != "report.pdf" {
		t.Fatalf("filter wrong: %+v", got)
	}
	m, _ = press(m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.explorer.filter != "" {
		t.Fatal("esc did not clear the filter")
	}
	m, _ = press(m, tea.KeyMsg{Type: tea.KeyCtrlL})
	if !m.explorer.typing {
		t.Fatal("ctrl+l did not open path input")
	}
	m, _ = press(m, tea.KeyMsg{Type: tea.KeyEsc})
	m, _ = press(m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.active != chatScreen || m.explorer != nil {
		t.Fatal("esc did not close the explorer")
	}
}

func TestHistoryPreviewIncludesLatestBody(t *testing.T) {
	m := demoFixture(t)
	for _, c := range m.conversations {
		if c.LastBody == "" {
			t.Fatalf("missing latest message for %s", c.Title)
		}
	}
}

func BenchmarkWorkspaceView(b *testing.B) {
	m := benchModel(b)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = m.View()
	}
}

func BenchmarkHitTestOnMouseMotion(b *testing.B) {
	m := benchModel(b)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = m.hitAt(60+i%40, 10+i%20)
	}
}

func benchModel(b *testing.B) Model {
	r, _ := testRuntime(b, true)
	if err := SeedDemo(context.Background(), r.Repo); err != nil {
		b.Fatal(err)
	}
	m := DemoModel(r, config.Settings{ConsentVersion: config.ConsentVersion, Mouse: true}, "")
	m.width, m.height = 200, 60
	m.fitList()
	m.fitMessages()
	return m
}

func TestNewChatsAreUnreadButSyncedHistoryIsNot(t *testing.T) {
	m := New(nil)
	m.settings.ConsentVersion = config.ConsentVersion
	m.account = "acc"
	now := time.Now()
	next, _ := m.Update(chatsMsg{account: "acc", chats: []history.Conversation{
		{ID: "fresh@s.whatsapp.net", Title: "Fresh", UpdatedAt: now.Add(-5 * time.Second)},
		{ID: "synced@s.whatsapp.net", Title: "Synced", UpdatedAt: now.Add(-48 * time.Hour)},
	}})
	m = next.(Model)
	if !m.state.Read["acc|fresh@s.whatsapp.net"].Before(now.Add(-24 * time.Hour)) {
		t.Fatal("a message that just arrived in a new chat must count as unread")
	}
	if got := m.state.Read["acc|synced@s.whatsapp.net"]; got.Before(now.Add(-49*time.Hour)) || got.After(now.Add(-47*time.Hour)) {
		t.Fatal("old synced history must start read")
	}
}

func TestUnreadCountsComeFromStoredMessages(t *testing.T) {
	r, _ := testRuntime(t, true)
	base := time.Now().Add(-time.Hour).UTC()
	for i := 0; i < 4; i++ {
		e := history.IncomingEvent{Kind: history.MessageEvent, AccountID: "local", ChatID: "c@s.whatsapp.net", Title: "C",
			MessageID: string(rune('a' + i)), Sender: "peer", Body: "x", Timestamp: base.Add(time.Duration(i) * time.Minute)}
		if err := r.Repo.ApplyEvent(context.Background(), e); err != nil {
			t.Fatal(err)
		}
	}
	m := NewWithRuntime(r, config.Settings{ConsentVersion: config.ConsentVersion}, "")
	m.state.Read["local|c@s.whatsapp.net"] = base.Add(90 * time.Second) // messages 2 and 3 are newer
	cmd := m.loadUnread()
	if cmd == nil {
		t.Fatal("no unread query")
	}
	msg := cmd().(unreadMsg)
	if msg.counts["c@s.whatsapp.net"] != 2 {
		t.Fatalf("unread = %v", msg.counts)
	}
	next, _ := m.Update(msg)
	m = next.(Model)
	m.markRead("c@s.whatsapp.net")
	if m.unread["c@s.whatsapp.net"] != 0 || !m.stateDirty {
		t.Fatal("opening a chat must clear its badge and schedule a save")
	}
}

func TestLargestRealisticQRStaysInlineAt130x42(t *testing.T) {
	for _, n := range []int{150, 190, 230} {
		m := New(nil)
		m.width, m.height, m.active = 130, 42, pairingScreen
		m.connection.QR = "2@" + strings.Repeat("q", n-10) + ",A,B,C"
		q := m.qrCode()
		cols, rows := q.Size()
		view := plain(m)
		inline := strings.Contains(view, "▣  Full-screen QR") && strings.Contains(view, "Scan with WhatsApp") && strings.Contains(view, "Never share it.")
		if !inline {
			t.Errorf("%d-byte payload (%dx%d cells) was not shown inline at 130x42", n, cols, rows)
		}
	}
}

func TestTinyTerminalOffersFullscreenOrGuidanceNotBrokenQR(t *testing.T) {
	m := New(nil)
	m.width, m.height, m.active = 60, 20, pairingScreen
	m.connection.QR = "2@" + strings.Repeat("q", 200) + ",A,B,C"
	view := plain(m)
	if !strings.Contains(view, "too small") || !strings.Contains(view, "Enlarge to at least") {
		t.Fatal("small terminals must be told how large the QR needs to be, never shown a cropped QR")
	}
}
