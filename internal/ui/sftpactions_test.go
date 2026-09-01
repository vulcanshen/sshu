package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/vulcanshen/sshu/internal/remote"
	"github.com/vulcanshen/sshu/internal/store"
)

// `t` and `T` are two actions, not two spellings of one: the item under the
// cursor, or every mark on this side. Case is significant here and nowhere else,
// which is why the test sends BOTH and checks what actually arrived.
func TestTransferCursorAndTransferAllAreDifferentKeys(t *testing.T) {
	// deploy.sh and main.go marked; the cursor left on README.md, which is
	// deliberately not one of them.
	setUp := func() (AppModel, string) {
		t.Helper()
		m := sftpFixture(t, 100, 26)
		m.sftp.focus = panelLeftFiles
		m = pressA(m, "j", "m", "j", "m", "j")
		if n := len(m.sftp.sides[sideLeft].marks); n != 2 {
			t.Fatalf("setup: expected two marks, got %d", n)
		}
		if e, _ := m.sftp.cur().cursorEntry(); e.Name != "README.md" {
			t.Fatalf("setup: cursor is on %q, want README.md", e.Name)
		}
		return m, m.sftp.sides[sideRight].cwd
	}

	m, dst := setUp()
	m = pressA(m, "t")
	if n := len(m.transfers.jobs); n != 1 {
		t.Fatalf("t should start one transfer, got %d", n)
	}
	waitJob(t, m.transfers.jobs[0])
	fs := m.sftp.sides[sideRight].fs
	if !remote.Exists(fs, remote.Join(dst, "README.md")) {
		t.Error("t must send the item under the cursor")
	}
	if remote.Exists(fs, remote.Join(dst, "deploy.sh")) {
		t.Error("t sent a marked item — that is T's job")
	}

	m, dst = setUp()
	m = pressA(m, "T")
	if n := len(m.transfers.jobs); n != 1 {
		t.Fatalf("T should start one transfer, got %d", n)
	}
	waitJob(t, m.transfers.jobs[0])
	fs = m.sftp.sides[sideRight].fs
	for _, name := range []string{"deploy.sh", "main.go"} {
		if !remote.Exists(fs, remote.Join(dst, name)) {
			t.Errorf("T must send every mark, %s is missing", name)
		}
	}
	if remote.Exists(fs, remote.Join(dst, "README.md")) {
		t.Error("T sent the cursor item — that is t's job")
	}
}

// A side with no host has nothing to mark, send or reset. Offering those rows
// anyway teaches that the menu does not mean what it says (§A.1).
func TestASideWithNoHostOnlyOffersSelectHost(t *testing.T) {
	m := New(sample(), nil, store.DefaultConfig())
	next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 26})
	m = settle(next.(AppModel))
	m.tab = tabFT
	m.sftp.focus = panelLeftFiles

	items := m.sftpMenuItems()
	if len(items) != 1 || items[0].key != keySelectHost {
		var got []string
		for _, it := range items {
			got = append(got, it.key)
		}
		t.Fatalf("menu offers %v, want only %q", got, keySelectHost)
	}

	// The letters must agree with the menu, or one of them is lying.
	if after := pressA(m, "/"); after.sftp.sides[sideLeft].filtering {
		t.Error("with no host there is nothing to search")
	}
	if after := pressA(m, "t"); len(after.transfers.jobs) != 0 {
		t.Error("with no host there is nothing to transfer")
	}
	if after := pressA(m, "S"); !after.hostPicker.isActive() {
		t.Error("S must still open the host picker")
	}
}

// The Space menu belongs to a PANEL, not to a tab: in a split tab "what can I do
// here" depends on which panel you are standing in, and a title naming the tab
// cannot tell [4] from [6].
func TestMenuTitleNamesTheFocusedPanel(t *testing.T) {
	m := sftpFixture(t, 100, 26)
	for _, tc := range []struct {
		p    sftpPanel
		want string
	}{
		{panelLeftFiles, "[1] local"},
		{panelLeftMarks, "[2] Marked files"},
		{panelRightFiles, "[3] local"},
		{panelRightMarks, "[4] Marked files"},
	} {
		m.sftp.focus = tc.p
		if got := m.menuTitle(); got != tc.want {
			t.Errorf("focus %d: menu title %q, want %q", tc.p, got, tc.want)
		}
		// And it is the same string the panel's own capsule shows, because it
		// comes from the same function.
		if view := ansi.Strip(m.View()); !strings.Contains(view, tc.want) {
			t.Errorf("focus %d: %q is not on screen", tc.p, tc.want)
		}
	}
}

// Tab [3] gets the same treatment: the menu says which of [4]/[5]/[6] it is for.
func TestSSHMenuTitleNamesTheFocusedPanel(t *testing.T) {
	m := appWith(sample(), nil)
	m.tab = tabSSH
	for _, tc := range []struct {
		p    sshPanel
		want string
	}{
		{panelSessions, "[1] sessions"},
		{panelLayout, "[2] layout"},
	} {
		m.ssh.setFocus(tc.p)
		if got := m.menuTitle(); got != tc.want {
			t.Errorf("focus %d: menu title %q, want %q", tc.p, got, tc.want)
		}
	}
}

// The prompt is the search glyph, not a literal "/". Echoing the key that opened
// the search makes a query CONTAINING a slash unreadable — /tmp would render as
// //tmp and there is no way to tell which slash is yours.
func TestSearchPromptIsAGlyphNotASlash(t *testing.T) {
	m, _ := atRoot(sftpFixture(t, 100, 26))
	m = pressA(m, "/")
	m = typeText(m, "/tmp")

	row := ansi.Strip(searchRow(m.sftp.sides[sideLeft], 40))
	if !strings.Contains(row, glyphSearch) {
		t.Errorf("no search glyph in the prompt: %q", row)
	}
	if strings.Contains(row, "//tmp") {
		t.Errorf("the prompt slash collided with the query: %q", row)
	}
	if !strings.Contains(row, "/tmp") {
		t.Errorf("the query itself did not survive: %q", row)
	}
}

// Committing an action from the Space menu hands the keyboard back at once. The
// close animation is a visual, not a modal state — while it ran, the next
// keystroke was swallowed by a popup already on its way out, so Esc had to be
// pressed twice to leave the search.
func TestAClosingPopupDoesNotEatTheNextKey(t *testing.T) {
	m, _ := atRoot(sftpFixture(t, 100, 26))
	cwd := m.sftp.sides[sideLeft].cwd

	m = pressA(m, " ")
	next, _ := m.Update(keyMsg("/")) // commit Search, then do NOT settle
	m = next.(AppModel)
	if !m.sftp.sides[sideLeft].filtering {
		t.Fatal("setup: Search should have started")
	}
	if !m.spaceMenu.isActive() {
		t.Fatal("setup: the menu should still be animating out")
	}

	next, _ = m.Update(keyMsg("esc"))
	m = next.(AppModel)
	if m.sftp.sides[sideLeft].filtering {
		t.Error("Esc was eaten by the closing menu instead of leaving the search")
	}
	if got := m.sftp.sides[sideLeft].cwd; got != cwd {
		t.Errorf("Esc left the directory rather than the search: %q", got)
	}
}

// Esc leaves the search before it leaves the directory, whichever way the search
// was started.
func TestEscLeavesTheSearchBeforeTheDirectory(t *testing.T) {
	for _, how := range []string{"hotkey", "menu"} {
		m, _ := atRoot(sftpFixture(t, 100, 26))
		cwd := m.sftp.sides[sideLeft].cwd
		if how == "menu" {
			m = pressA(m, " ")
		}
		m = pressA(m, "/")
		m = typeText(m, "dep")

		m = pressA(m, "esc")
		if s := m.sftp.sides[sideLeft]; s.filtering {
			t.Errorf("%s: Esc did not leave the search", how)
		}
		if got := m.sftp.sides[sideLeft].cwd; got != cwd {
			t.Errorf("%s: Esc also left the directory: %q", how, got)
		}
		// A second Esc is the one that goes up.
		m = pressA(m, "esc")
		if got := m.sftp.sides[sideLeft].cwd; got == cwd {
			t.Errorf("%s: a second Esc should go up, still at %q", how, got)
		}
	}
}

// h/l cross between the halves, keeping the row: the two sides are mirror
// images, so [5] goes to [7] and not to [6]. Tab is for "the next panel"; h/l
// are for "the other host", which on a 1:1 split is what you usually mean.
func TestHLCrossesSides(t *testing.T) {
	m := sftpFixture(t, 100, 26)
	for _, tc := range []struct {
		from sftpPanel
		key  string
		want sftpPanel
	}{
		{panelLeftFiles, "l", panelRightFiles},
		{panelRightFiles, "h", panelLeftFiles},
		{panelLeftMarks, "l", panelRightMarks},
		{panelRightMarks, "h", panelLeftMarks},
		// Already there: it stays put rather than wrapping round.
		{panelLeftFiles, "h", panelLeftFiles},
		{panelRightMarks, "l", panelRightMarks},
		// The arrows are synonyms.
		{panelLeftFiles, "right", panelRightFiles},
		{panelRightFiles, "left", panelLeftFiles},
	} {
		m.sftp.focus = tc.from
		m = pressA(m, tc.key)
		if m.sftp.focus != tc.want {
			t.Errorf("%q from panel %d landed on %d, want %d",
				tc.key, tc.from, m.sftp.focus, tc.want)
		}
	}
}

// While a search is being typed, h and l are letters.
func TestHLAreLettersWhileSearching(t *testing.T) {
	m, _ := atRoot(sftpFixture(t, 100, 26))
	m = pressA(m, "/")
	m = typeText(m, "hl")

	if got := m.sftp.sides[sideLeft].query; got != "hl" {
		t.Errorf("query is %q, want \"hl\"", got)
	}
	if m.sftp.focus != panelLeftFiles {
		t.Errorf("typing into the query moved the focus to panel %d", m.sftp.focus)
	}
}

// Leaving takes the sftp connections with it. An ssh connection left open
// because the process exited is the server's problem to time out.
func TestQuitClosesTheSftpConnections(t *testing.T) {
	m := sftpFixture(t, 100, 26)
	if m.sftp.sides[sideLeft].fs == nil || m.sftp.sides[sideRight].fs == nil {
		t.Fatal("setup: both sides should be connected")
	}

	m = pressA(m, "q")
	if m.confirm.isActive() {
		t.Fatal("nothing is running, so quitting should not ask")
	}
	for _, sd := range []side{sideLeft, sideRight} {
		if m.sftp.sides[sd].fs != nil {
			t.Errorf("side %d was left connected", sd)
		}
	}
}

// Ctrl+C is the emergency exit and it tears down the same three things.
func TestForceQuitClosesTheSftpConnections(t *testing.T) {
	m := sftpFixture(t, 100, 26)
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	m = next.(AppModel)
	for _, sd := range []side{sideLeft, sideRight} {
		if m.sftp.sides[sd].fs != nil {
			t.Errorf("side %d was left connected", sd)
		}
	}
}

// A half-copied file is as much "something to lose" as an idle shell, so it gets
// the same warning rather than being dropped silently.
func TestQuitAsksAboutARunningTransfer(t *testing.T) {
	m := sftpFixture(t, 100, 26)
	m.sftp.focus = panelLeftFiles
	m.transfers.jobs = append(m.transfers.jobs, &transferJob{
		id: 1, label: "test", files: 1, cancel: func() {},
	})

	m = pressA(m, "q")
	if !m.confirm.isActive() || m.confirm.action != confirmQuit {
		t.Fatal("a running transfer should raise the quit confirmation")
	}
	if lines := m.quitCost(); len(lines) != 1 ||
		!strings.Contains(lines[0], "transfer") {
		t.Errorf("the confirmation should name the transfer, got %v", lines)
	}
}
