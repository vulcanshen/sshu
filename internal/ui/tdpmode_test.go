package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// Selection mode is a mode in tdp's sense (K11): the core keys keep a meaning
// in it. Each test failed against the mode that swallowed them all.

func inSelection(t *testing.T) AppModel {
	t.Helper()
	m := pressA(openOne(t), "alt+v")
	if !m.ssh.copy.on {
		t.Fatal("setup: alt+v should open selection mode")
	}
	return m
}

// Space does nothing in a mode (tdp v0.1.10 K11): no menu, no key list, and
// the mode stays exactly where it was. (It used to open a runnable key list.)
func TestSpaceDoesNothingInSelectionMode(t *testing.T) {
	m := inSelection(t)
	row, col := m.ssh.copy.row, m.ssh.copy.col
	m = pressA(m, " ")
	if m.floatsOpen() != 0 || m.help.isActive() {
		t.Errorf("Space in selection mode should open nothing, %d floats open", m.floatsOpen())
	}
	if !m.ssh.copy.on || m.ssh.copy.row != row || m.ssh.copy.col != col {
		t.Error("Space should leave the mode and its cursor as they were")
	}
}

// The mode's keys are pressed, not picked: l moves the cursor at once.
func TestTheModesKeysArePressedDirectly(t *testing.T) {
	m := inSelection(t)
	col := m.ssh.copy.col
	m = pressA(m, "l")
	if m.ssh.copy.col != col+1 {
		t.Errorf("l should move the cursor right: col %d → %d", col, m.ssh.copy.col)
	}
	if !m.ssh.copy.on || m.floatsOpen() != 0 {
		t.Error("a mode key should run in place, with nothing opened")
	}
}

// ? is the mode's key reference, read-only: every key the mode has, and no
// mention of a Space list; ? closes it and the mode stays.
func TestQuestionMarkIsTheSelectionKeyReference(t *testing.T) {
	m := pressA(inSelection(t), "?")
	if !m.help.isActive() || m.help.title != "selection mode" {
		t.Fatalf("? should open the selection mode key reference, got %q", m.help.title)
	}
	keys := map[string]bool{}
	for _, e := range m.help.entries {
		keys[e.key] = true
		if e.key == "Space" {
			t.Error("the reference should not offer Space: it does nothing in a mode")
		}
	}
	for _, k := range []string{"h/j/k/l", "w/e/b", "0/$", "u/d", "v/V", "y", "Esc", "Alt-v"} {
		if !keys[k] {
			t.Errorf("the reference is missing %q", k)
		}
	}
	m = pressA(m, "?")
	if m.help.isActive() || !m.ssh.copy.on {
		t.Error("? should close the reference and leave the mode running")
	}
}

// The footer leads with ? and does not list Space (tdp K11, M1).
func TestTheSelectionFooterLeadsWithTheKeyReference(t *testing.T) {
	pairs := copyLegendPairs()
	if pairs[0][0] != "?" {
		t.Errorf("the selection footer should lead with ?, got %q", pairs[0][0])
	}
	for _, p := range pairs {
		if p[0] == "Space" {
			t.Error("Space does nothing in a mode, so the footer should not list it")
		}
	}
}

// q and Ctrl+C are the leaving flow in here too (tdp K9); saying no comes back
// to the mode, and a second Ctrl+C leaves.
func TestQuitWorksFromSelectionMode(t *testing.T) {
	m := pressA(inSelection(t), "q")
	if !m.quitAsk.isActive() {
		t.Fatal("q with a live session should ask")
	}
	m = pressA(m, "esc")
	if m.quitAsk.isActive() || !m.ssh.copy.on {
		t.Error("Esc should come back to the mode")
	}

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	m = settle(next.(AppModel))
	if !m.quitAsk.isActive() {
		t.Fatal("Ctrl+C should ask, like q")
	}
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Fatal("a second Ctrl+C should leave")
	}
	if _, isQuit := cmd().(tea.QuitMsg); !isQuit {
		t.Error("a second Ctrl+C should leave at once")
	}
}

// Tab is paused in the mode, but it answers (tdp K11).
func TestTabInSelectionModeSaysHowToLeave(t *testing.T) {
	m := pressA(inSelection(t), "tab")
	if !m.toast.isActive() {
		t.Error("Tab in selection mode should say to leave the mode first")
	}
	if !m.ssh.copy.on {
		t.Error("the mode should still be on")
	}
}

// While that toast is up, the first Esc closes it and nothing else: the mode
// and the selection half made are still there (tdp v0.1.14 K11). The second Esc
// is the mode's own again.
func TestTheFirstEscClosesTheToastTabRaised(t *testing.T) {
	m := pressA(inSelection(t), "v", "tab")
	if !m.toast.anim.owns() {
		t.Fatal("setup: Tab should have raised the toast")
	}
	m = pressA(m, "esc")
	if m.toast.anim.owns() {
		t.Error("the first Esc should close the toast")
	}
	if !m.ssh.copy.on || m.ssh.copy.sel != selChar {
		t.Errorf("the first Esc should leave the mode and its selection alone: on=%v sel=%d",
			m.ssh.copy.on, m.ssh.copy.sel)
	}
	m = pressA(m, "esc")
	if m.ssh.copy.sel != selNone {
		t.Error("the second Esc should drop the selection")
	}
}

// The same on a panel: a toast on screen is what Esc closes first, before the
// search it would clear or the directory it would leave (tdp F1, K4).
func TestEscClosesAToastBeforeTheHostsSearch(t *testing.T) {
	m := sized(sample(), 100, 24)
	m = pressA(m, "/")
	m = typeText(m, "prod")
	m.toast.show("Copied", toastInfo)
	m = settle(m)
	m = pressA(m, "esc")
	if m.toast.anim.owns() {
		t.Error("Esc should close the toast")
	}
	if !m.hosts.filtering {
		t.Fatal("the search should outlast the Esc that closed the toast")
	}
	m = pressA(m, "esc")
	if m.hosts.filtering {
		t.Error("the next Esc should leave the search")
	}
}

func TestEscClosesAToastBeforeLeavingTheDirectory(t *testing.T) {
	m := sftpFixture(t, 100, 26)
	m.sftp.focus = panelLeftFiles
	m = pressA(m, "R")
	if !m.toast.anim.owns() {
		t.Fatal("setup: R should say the directory was refreshed")
	}
	cwd := m.sftp.cur().cwd
	m = pressA(m, "esc")
	if m.toast.anim.owns() {
		t.Error("Esc should close the toast")
	}
	if got := m.sftp.cur().cwd; got != cwd {
		t.Errorf("the Esc that closed the toast went up a directory: %q → %q", cwd, got)
	}
	m = pressA(m, "esc")
	if m.sftp.cur().cwd == cwd {
		t.Error("the next Esc should go up a directory")
	}
}

// Inside a pty a bare Esc is the remote's (tdp K10): a toast waits for its timer.
func TestEscInAPtyStillReachesTheRemoteUnderAToast(t *testing.T) {
	fakeSSH(t, `printf '$ '; exec cat -v`)
	m := pressA(sshApp(t, sample()), "enter", "enter")
	t.Cleanup(func() { m.ssh.stopAll() })
	s := m.ssh.sessions[0]
	waitFor(t, "the stand-in to answer", func() bool { return s.pty.hasSpoken() })
	m.toast.show("Copied", toastInfo)
	m = settle(m)
	m = pressA(m, "esc")
	waitFor(t, "Esc to arrive at the remote", func() bool {
		return strings.Contains(strings.Join(s.pty.render(80, 24), ""), "^[")
	})
	if !m.toast.anim.owns() {
		t.Error("in a pty the toast should wait for its timer, not take Esc")
	}
}

// gg and G go to the first and last line of the frozen page, scrollback and
// all, as in vim (tdp v0.1.21 D5); a lone g waits for the next key, and a
// selection stretches with them.
func TestGgAndGReachEitherEndOfThePage(t *testing.T) {
	lines := make([]string, 50)
	for i := range lines {
		lines[i] = "line"
	}
	c := copyState{on: true, lines: lines, w: 20, h: 10, row: 20, col: 2, top: 15}
	c.key("G")
	if c.row != 49 || c.top != 40 {
		t.Errorf("G should land on the last line in view: row %d top %d", c.row, c.top)
	}
	c.key("g")
	if c.row != 49 {
		t.Error("a lone g should wait")
	}
	c.key("g")
	if c.row != 0 || c.top != 0 || c.col != 2 {
		t.Errorf("gg should land on the first line: row %d top %d col %d", c.row, c.top, c.col)
	}
	c.key("g")
	c.key("j")
	if c.row != 1 || c.gPending {
		t.Errorf("g then j is just j: row %d", c.row)
	}
	c.key("v")
	c.key("G")
	if r0, _, r1, _ := c.span(); r0 != 1 || r1 != 49 {
		t.Errorf("the selection should stretch to the bottom: %d..%d", r0, r1)
	}
	found := false
	for _, e := range copyModeHelp() {
		found = found || e.key == "gg/G"
	}
	if !found {
		t.Error("? should list gg/G")
	}
}
