package ui

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/vulcanshen/sshu/internal/store"
)

// fakeSSH points sshBinary at a script instead of the real ssh. The script
// ignores the ssh arguments, so the tests exercise the session machinery without
// opening a connection to anything.
func fakeSSH(t *testing.T, body string) {
	t.Helper()
	p := filepath.Join(t.TempDir(), "fake-ssh")
	if err := os.WriteFile(p, []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	old := sshBinary
	sshBinary = p
	t.Cleanup(func() { sshBinary = old })
}

// aliveSSH stays up until its PTY is closed, standing in for a connected host.
func aliveSSH(t *testing.T) { fakeSSH(t, "exec cat") }

func sshApp(t *testing.T, hosts []store.Host) AppModel {
	t.Helper()
	m := New(hosts, nil)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	return settle(next.(AppModel))
}

// openOne connects to the first sample host and lands in the PTY.
func openOne(t *testing.T) AppModel {
	t.Helper()
	aliveSSH(t)
	m := pressA(sshApp(t, sample()), "enter", "enter") // connect confirm -> accept
	if m.tab != tabSSH {
		t.Fatalf("connecting should land on tab [3], got %d", m.tab)
	}
	if len(m.ssh.sessions) != 1 {
		t.Fatalf("expected one live session, got %d", len(m.ssh.sessions))
	}
	t.Cleanup(func() { m.ssh.stopAll() })
	return m
}

// waitFor polls until cond holds, so a test never depends on a fixed sleep.
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

// ------------------------------------------------------------------- frame

func TestSSHTabPreservesFrame(t *testing.T) {
	aliveSSH(t)
	// 55/54/53 straddle the narrow threshold, where the layout changes shape.
	for _, sz := range [][2]int{{100, 30}, {92, 24}, {78, 24}, {60, 30}, {55, 30},
		{54, 30}, {53, 30}, {54, 16}, {40, 12}, {24, 9}} {
		w, h := sz[0], sz[1]
		m := New(sample(), nil)
		next, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: h})
		m = pressA(settle(next.(AppModel)), "enter", "enter")

		for _, focus := range []sshPanel{panelSessions, panelPty} {
			m.ssh.focus = focus
			lines := strings.Split(m.View(), "\n")
			if len(lines) != h {
				t.Errorf("%dx%d focus=%d: %d lines, want %d", w, h, focus, len(lines), h)
				continue
			}
			for i, l := range lines {
				if lw := dispW(l); lw != w {
					t.Errorf("%dx%d focus=%d line %d: width %d, want %d\n%q",
						w, h, focus, i, lw, w, l)
				}
			}
		}
		m.ssh.stopAll()
	}
}

// Below the narrow threshold the columns cannot both be useful, so the PTY takes
// the screen and the lists are reached with Alt+Esc.
func TestNarrowDropsTheLists(t *testing.T) {
	m := sshApp(t, sample())
	m.ssh.w, m.ssh.h = sshNarrowW-1, 20
	if !m.ssh.narrow() {
		t.Fatalf("%d columns should be narrow", sshNarrowW-1)
	}
	_, _, rightW, _ := m.ssh.panes()
	if rightW != sshNarrowW-1 {
		t.Errorf("the PTY should take the whole width, got %d", rightW)
	}
}

// ------------------------------------------------------------------ the PTY

// A live session that has said NOTHING yet is not an empty terminal, it is a
// wait — and the two look identical: an empty bordered box.
//
// ssh prints nothing while it waits for a TCP connection, so against an address
// that never answers that box stays blank for as long as the OS takes to give
// up, which can run past a minute. `cat` stands in here for exactly that
// property: it says nothing until it is spoken to.
func TestAConnectingSessionSaysSo(t *testing.T) {
	m := openOne(t)
	view := ansi.Strip(m.View())

	if !strings.Contains(view, "connecting to") {
		t.Errorf("a session that has not answered yet says nothing:\n%s", view)
	}
	// And it names WHICH host: "connecting" on its own does not tell you whether
	// you picked the one you meant.
	h := sample()[0]
	if !strings.Contains(view, h.User+"@"+h.Host) {
		t.Errorf("the wait does not name the host:\n%s", view)
	}
}

// The moment the far end says anything, its terminal takes the panel back.
func TestTheTerminalTakesOverOnTheFirstByte(t *testing.T) {
	fakeSSH(t, "printf 'a word from the remote\\n'; exec cat")
	m := pressA(sshApp(t, sample()), "enter", "enter")
	t.Cleanup(func() { m.ssh.stopAll() })

	waitFor(t, "the remote to say something", func() bool {
		s := m.ssh.currentSession()
		return s != nil && s.pty.hasSpoken()
	})

	view := ansi.Strip(m.View())
	if strings.Contains(view, "connecting to") {
		t.Errorf("still waiting after the remote spoke:\n%s", view)
	}
	if !strings.Contains(view, "a word from the remote") {
		t.Errorf("the terminal is not showing what arrived:\n%s", view)
	}
}

// Alt+Esc is the only way out of a focused PTY — every other key is the
// remote's.
func TestAltEscLeavesThePty(t *testing.T) {
	m := openOne(t)
	if m.ssh.focus != panelPty {
		t.Fatalf("connecting should land in the PTY, got focus=%d", m.ssh.focus)
	}
	if !m.inPty() {
		t.Fatal("inPty should be true with a live focused session")
	}

	// A plain Esc belongs to the remote and must not close anything.
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEscape})
	if next.(AppModel).ssh.focus != panelPty {
		t.Error("a bare Esc must go to the remote, not unfocus the PTY")
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEscape, Alt: true})
	m = next.(AppModel)
	if m.ssh.focus != panelSessions {
		t.Fatalf("Alt+Esc should return to [4], got focus=%d", m.ssh.focus)
	}
	if m.inPty() {
		t.Error("focus left the PTY; inPty should be false")
	}
}

// Everything typed in the PTY reaches the far end. `cat` echoes it back, so the
// emulator grid is proof it made the round trip.
func TestKeysReachTheRemote(t *testing.T) {
	m := openOne(t)
	m = typeText(m, "hello")

	s := m.ssh.currentSession()
	waitFor(t, "the remote to echo", func() bool {
		return strings.Contains(strings.Join(s.pty.render(80, 24), ""), "hello")
	})
}

// While the remote holds the keyboard the footer must advertise the way out and
// nothing else — every other entry would be a lie.
func TestFooterInPtyAdvertisesTheWayOut(t *testing.T) {
	m := openOne(t)
	foot := m.footer()
	if !strings.Contains(foot, "alt+esc") {
		t.Error("the footer must disclose alt+esc while the PTY has focus")
	}
	for _, gone := range []string{"space", "help", "quit"} {
		if strings.Contains(foot, gone) {
			t.Errorf("%q reaches the remote, so the footer must not offer it", gone)
		}
	}

	m.ssh.setFocus(panelSessions)
	if !strings.Contains(m.footer(), "space") {
		t.Error("the normal footer should return once the PTY is unfocused")
	}
}

// --------------------------------------------------------------- navigation

// A digit addresses a panel of the CURRENT tab, and is inert where no such
// panel is on screen. The rule reads off the screen: the numbers you can see are
// the numbers you can press.
func TestDigitsAddressPanelsOfTheCurrentTab(t *testing.T) {
	for _, tc := range []struct {
		key  string
		want sshPanel
	}{{"4", panelSessions}, {"5", panelPty}} {
		m := pressA(sshApp(t, sample()), "3", tc.key)
		if m.ssh.focus != tc.want {
			t.Errorf("%s in tab [3] should focus panel %d, got %d", tc.key, tc.want, m.ssh.focus)
		}
	}

	// From tab [1] the same digits do nothing at all: no tab change, no focus
	// change, no surprise.
	for _, k := range []string{"4", "5", "6", "7"} {
		m := sshApp(t, sample())
		before := m.ssh.focus
		m = pressA(m, k)
		if m.tab != tabHosts {
			t.Errorf("%s from tab [1] must not switch tabs, got %d", k, m.tab)
		}
		if m.ssh.focus != before {
			t.Errorf("%s from tab [1] must not move focus elsewhere", k)
		}
	}
}

// Tab must never walk into the PTY: it would swallow the key that got you there.
// With [6] gone there is one list left, so Tab has nowhere to go — but it must
// still be a way OUT of the pty, which costs nothing to offer.
func TestTabNeverEntersThePty(t *testing.T) {
	m := pressA(sshApp(t, sample()), "3")
	for i := 0; i < 6; i++ {
		m = pressA(m, "tab")
		if m.tab != tabSSH {
			t.Fatal("Tab left the tab")
		}
		if m.ssh.focus == panelPty {
			t.Fatal("Tab reached the PTY — from there Tab belongs to the remote")
		}
	}
	m.ssh.setFocus(panelPty)
	m = pressA(m, "tab")
	if m.ssh.focus != panelSessions {
		t.Errorf("Tab out of the pty landed on %d", m.ssh.focus)
	}
}

// ------------------------------------------------------------------ sessions

// Moving the list cursor must NOT change what [5] shows — switching redraws the
// remote, and browsing the list should not do that.
func TestCursorDoesNotSwitchTheSession(t *testing.T) {
	aliveSSH(t)
	m := pressA(sshApp(t, sample()), "enter", "enter")
	m = pressA(m, "esc") // out of the PTY, onto [4]
	m.ssh.setFocus(panelSessions)
	m = pressA(m, "1", "l", "enter", "enter") // connect to a second host
	t.Cleanup(func() { m.ssh.stopAll() })

	if len(m.ssh.sessions) != 2 {
		t.Fatalf("expected two sessions, got %d", len(m.ssh.sessions))
	}
	shown := m.ssh.current
	m.ssh.setFocus(panelSessions)
	m = pressA(m, "k") // move the cursor off the shown session

	if m.ssh.current != shown {
		t.Error("moving the cursor must not change which session is displayed")
	}
	if m.ssh.curSess == 1 {
		t.Fatal("the cursor did not move; the rest of this test proves nothing")
	}
}

// Enter on [4] switches outright. Switching opens no connection and closes none
// — the session you leave keeps running — so there is nothing to confirm, on the
// current row or any other.
func TestEnterOnSessionNeverAsks(t *testing.T) {
	aliveSSH(t)
	m := pressA(sshApp(t, sample()), "enter", "enter") // first session
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEscape, Alt: true})
	m = settle(next.(AppModel))
	m = pressA(m, "1", "j", "enter", "enter") // a second session, to a second host
	t.Cleanup(func() { m.ssh.stopAll() })
	if len(m.ssh.sessions) != 2 {
		t.Fatalf("expected two sessions, got %d", len(m.ssh.sessions))
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEscape, Alt: true})
	m = settle(next.(AppModel))
	second := m.ssh.current

	// The row already showing: Enter just goes in.
	m = pressA(m, "enter")
	if m.confirm.isActive() {
		t.Fatal("Enter on the current session must not open a dialog")
	}
	if m.ssh.focus != panelPty {
		t.Errorf("Enter should take focus into the PTY, got %d", m.ssh.focus)
	}

	// A different row: Enter switches, still without asking.
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEscape, Alt: true})
	m = settle(next.(AppModel))
	m = pressA(m, "k")
	m = pressA(m, "enter")
	if m.confirm.isActive() {
		t.Fatal("Enter on another session must not open a dialog either")
	}
	if m.ssh.current == second {
		t.Error("Enter should have switched which session [5] shows")
	}
	if m.ssh.focus != panelPty {
		t.Errorf("Enter should land in the PTY, got %d", m.ssh.focus)
	}
	if len(m.ssh.sessions) != 2 {
		t.Error("switching must not close the session being left")
	}
}

// A dead session moves to history carrying the reason it ended.
func TestExitedSessionMovesToHistory(t *testing.T) {
	fakeSSH(t, "exit 7")
	m := pressA(sshApp(t, sample()), "enter", "enter")
	s := m.ssh.sessions[0]

	waitFor(t, "the subprocess to exit", func() bool { return s.pty.exited() })
	if len(m.ssh.reap()) != 1 {
		t.Fatal("reap should have moved the finished session")
	}
	if len(m.ssh.sessions) != 0 || len(m.ssh.history) != 1 {
		t.Fatalf("live=%d history=%d, want 0 and 1", len(m.ssh.sessions), len(m.ssh.history))
	}
	if got := m.ssh.history[0].reason; got != "exited 7" {
		t.Errorf("reason %q, want %q", got, "exited 7")
	}
	if m.ssh.focus == panelPty {
		t.Error("focus must not stay in a PTY nobody is driving")
	}
	// ssh's own 255 means the connection failed, which is worth naming apart.
	if m.ssh.history[0].state != sessEnded {
		t.Error("the session should be marked ended")
	}
}

// #N appears only when a host has more than one live session — the list is
// name-only, so that tag is the only thing telling them apart.
func TestOrdinalOnlyWhenDuplicated(t *testing.T) {
	aliveSSH(t)
	m := sshApp(t, sample())
	m.ssh.setSize(100, 28)
	if _, err := m.ssh.connect(sample()[0]); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { m.ssh.stopAll() })

	if tag := m.ssh.sessions[0].ordinalTag(); tag != "" {
		t.Errorf("a lone session needs no ordinal, got %q", tag)
	}
	if _, err := m.ssh.connect(sample()[0]); err != nil {
		t.Fatal(err)
	}
	if a, b := m.ssh.sessions[0].ordinalTag(), m.ssh.sessions[1].ordinalTag(); a != "#1" || b != "#2" {
		t.Errorf("two sessions to one host should be #1/#2, got %q/%q", a, b)
	}

	// A different host is not a duplicate.
	if _, err := m.ssh.connect(sample()[1]); err != nil {
		t.Fatal(err)
	}
	if tag := m.ssh.sessions[2].ordinalTag(); tag != "" {
		t.Errorf("a different host needs no ordinal, got %q", tag)
	}
}

// Quitting kills every live session, so it asks first — but only when there is
// something to lose.
func TestQuitWarnsOnlyWithLiveSessions(t *testing.T) {
	m := sshApp(t, sample())
	if _, cmd := m.Update(keyMsg("q")); cmd == nil {
		t.Error("with nothing running, q should quit outright")
	}

	m = openOne(t)
	m.ssh.setFocus(panelSessions)
	m = pressA(m, "q")
	if !m.confirm.isActive() || m.confirm.action != confirmQuit {
		t.Fatal("q with a live session should ask first")
	}
	if !strings.Contains(strings.Join(m.confirm.lines, " "), "1 live session") {
		t.Errorf("the warning should count the sessions, got %v", m.confirm.lines)
	}
}

// ---------------------------------------------------------------- the menu

// §A.1 again: every action of the focused panel must be in its Space menu.
func TestSSHMenuListsFocusedPanelActions(t *testing.T) {
	m := openOne(t)
	for _, focus := range []sshPanel{panelSessions} {
		m.ssh.focus = focus
		items := m.sshMenuItems()
		for _, a := range sshActions {
			if a.panel != focus {
				continue
			}
			found := false
			for _, it := range items {
				if it.key == a.key && it.label == a.label {
					found = true
				}
			}
			if !found {
				t.Errorf("focus=%d: %q is not in the Space menu", focus, a.label)
			}
		}
		// An action belonging to the other panel must not leak in.
		for _, it := range items {
			for _, a := range sshActions {
				if a.panel != focus && a.label == it.label {
					t.Errorf("focus=%d: %q belongs to the other panel", focus, it.label)
				}
			}
		}
	}
}

// Space must answer even where the remote owns the keyboard — by saying so.
func TestSSHMenuInPtySaysWhoHasTheKeyboard(t *testing.T) {
	m := openOne(t)
	items := m.sshMenuItems()
	joined := ""
	for _, it := range items {
		joined += it.label + " "
	}
	if !strings.Contains(joined, "alt+esc") {
		t.Errorf("the PTY's menu should point at the way out, got %q", joined)
	}
}

// ------------------------------------------------------------------ wrapping

func TestWrapText(t *testing.T) {
	got := wrapText("db-replica-tokyo-ap-northeast-1", 20)
	if len(got) < 2 {
		t.Fatalf("a 31-char name should wrap at 20, got %v", got)
	}
	for _, l := range got {
		if dispW(l) > 20 {
			t.Errorf("line %q exceeds the width", l)
		}
	}
	if strings.Join(got, "") != "db-replica-tokyo-ap-northeast-1" {
		t.Errorf("wrapping lost characters: %v", got)
	}
	// It should break after a separator rather than mid-token.
	if !strings.HasSuffix(got[0], "-") {
		t.Errorf("expected a break after a dash, got %q", got[0])
	}
	if l := wrapText("short", 20); len(l) != 1 || l[0] != "short" {
		t.Errorf("a name that fits should not wrap, got %v", l)
	}
}

// ---------------------------------------------------------------- ssh command

func TestBuildSSHCmdArgs(t *testing.T) {
	h := store.Host{Name: "a", Host: "h.example.com", Port: 2222, User: "root",
		Auth: store.AuthPrivateKey, IdentityFile: "~/.ssh/id_ed25519"}
	got := strings.Join(buildSSHCmd(h, "").Args[1:], " ")
	for _, want := range []string{"-p 2222", "-i ", "IdentitiesOnly=yes", "root@h.example.com"} {
		if !strings.Contains(got, want) {
			t.Errorf("args %q missing %q", got, want)
		}
	}
	if strings.Contains(got, "~") {
		t.Error("the identity path should be expanded before ssh sees it")
	}
}

// A password host wires ssh's askpass back at sshu — and the password itself
// never enters the child's environment.
func TestPasswordHostUsesAskpassNotTheEnvironment(t *testing.T) {
	h := store.Host{Name: "a", Host: "h", Port: 22, User: "u",
		Auth: store.AuthPassword, Password: "s3cr3t"}
	env := strings.Join(sshEnv(h, "/usr/local/bin/sshu"), "\n")

	for _, want := range []string{"SSH_ASKPASS=/usr/local/bin/sshu",
		"SSH_ASKPASS_REQUIRE=force", askpassHostEnv + "=a"} {
		if !strings.Contains(env, want) {
			t.Errorf("env missing %q", want)
		}
	}
	if strings.Contains(env, "s3cr3t") {
		t.Error("the password must not be copied into the child's environment")
	}

	// A key host has no password to ask for.
	k := store.Host{Name: "b", Host: "h", Port: 22, User: "u", Auth: store.AuthPrivateKey}
	if strings.Contains(strings.Join(sshEnv(k, "/bin/sshu"), "\n"), "SSH_ASKPASS=") {
		t.Error("a privatekey host should not wire an askpass helper")
	}
}

// A finished session must not leave its last screen sitting in [5] looking like
// a live prompt. [5] goes back to its empty state and the emulator is released.
func TestEndedSessionLeavesThePanel(t *testing.T) {
	fakeSSH(t, "exit 0")
	m := pressA(sshApp(t, sample()), "enter", "enter")
	s := m.ssh.sessions[0]
	if m.ssh.current != s.id {
		t.Fatal("the new session should be the one on screen")
	}

	waitFor(t, "the subprocess to exit", func() bool { return s.pty.exited() })
	m.ssh.reap()

	if m.ssh.current != 0 {
		t.Errorf("[5] should show nothing once its session ended, current=%d", m.ssh.current)
	}
	if m.ssh.currentSession() != nil {
		t.Error("currentSession must only ever be a live session")
	}
	if s.pty != nil {
		t.Error("the emulator should be released — nothing renders it any more")
	}

	m.ssh.setFocus(panelSessions)
	if !strings.Contains(m.ssh.view(), "Select a session") {
		t.Error("[5] should be back to its empty state")
	}
}

// Focusing the PTY folds the lists away and gives it the whole tab; leaving
// brings them back. The remote is resized both ways, or it paints to the wrong
// geometry.
func TestFocusedPtyTakesTheWholeTab(t *testing.T) {
	m := openOne(t)
	full, _ := m.ssh.ptyInner()
	leftW, _, rightW, _ := m.ssh.panes()
	if leftW != 0 || rightW != m.ssh.w {
		t.Fatalf("a focused PTY should own the tab: left=%d right=%d w=%d", leftW, rightW, m.ssh.w)
	}

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEscape, Alt: true})
	m = next.(AppModel)
	split, _ := m.ssh.ptyInner()
	leftW, _, _, _ = m.ssh.panes()
	if leftW != sshLeftW {
		t.Fatalf("leaving the PTY should bring the lists back, left=%d", leftW)
	}
	if split >= full {
		t.Errorf("the PTY should be narrower once the lists are back: %d -> %d", full, split)
	}
	if m.ssh.appliedCols != split {
		t.Errorf("the remote was not resized on the way out: applied=%d want %d",
			m.ssh.appliedCols, split)
	}
}

// The four colour cases of a [4] row, written out because the fourth is the one
// that is easy to get wrong: a cursor bar over the on-screen session must not
// hide the fact that it IS the on-screen session.
func TestSessionRowColourCases(t *testing.T) {
	withColour(t)
	m := sshApp(t, sample())
	m.ssh.setSize(100, 28)

	shown := &session{id: 1, host: sample()[0], state: sessLive}
	other := &session{id: 2, host: sample()[1], state: sessLive}
	m.ssh.sessions = []*session{shown, other}
	m.ssh.current = 1

	green, hand := ansiOf(t, liveColor), ansiOf(t, handColor)
	greenBG, handBG := ansiBgOf(t, liveColor), ansiBgOf(t, handColor)

	// Foreground says on-screen.
	row := strings.Join(m.ssh.listItem(shown, false, 24), "\n")
	if !strings.Contains(row, green) {
		t.Error("the on-screen session should be green")
	}
	if strings.Contains(row, greenBG) {
		t.Error("green belongs in the foreground, not behind the row")
	}

	// Background says cursor — the same bar on every row, including that one.
	row = strings.Join(m.ssh.listItem(other, true, 24), "\n")
	if !strings.Contains(row, handBG) {
		t.Error("the cursor should be a filled bar")
	}
	row = strings.Join(m.ssh.listItem(shown, true, 24), "\n")
	if !strings.Contains(row, handBG) {
		t.Error("the cursor over the on-screen session is the same bar")
	}
	if strings.Contains(row, greenBG) {
		t.Error("there is no inverse case any more — green never becomes a background")
	}

	// And an ordinary row is neither.
	row = strings.Join(m.ssh.listItem(other, false, 24), "\n")
	for name, seq := range map[string]string{"green": green, "cursor": hand} {
		if strings.Contains(row, seq) {
			t.Errorf("an ordinary row should carry no %s", name)
		}
	}
}

// The row says what the connection IS, not what it is called.
func TestSessionRowShowsUserAtHost(t *testing.T) {
	m := sshApp(t, sample())
	m.ssh.setSize(100, 28)
	h := sample()[0]
	s := &session{id: 1, host: h, state: sessLive}
	m.ssh.sessions = []*session{s}

	row := ansi.Strip(strings.Join(m.ssh.listItem(s, false, 40), "\n"))
	if want := h.User + "@" + h.Host; !strings.Contains(row, want) {
		t.Errorf("row is %q, want it to carry %q", row, want)
	}
	if strings.Contains(row, h.Name) {
		t.Errorf("the row should not fall back to the saved name: %q", row)
	}
	if !strings.Contains(row, strconv.Itoa(h.Port)) {
		t.Errorf("the port is missing: %q", row)
	}
}

// The port is the one thing in a [4] row that must never be cut: the name wraps
// against it, however long the name is.
func TestSessionRowAlwaysShowsThePort(t *testing.T) {
	m := sshApp(t, sample())
	m.ssh.setSize(100, 28)

	// The ADDRESS is what wraps now, so that is what has to be long.
	long := store.Host{Name: "db", Host: "db-replica-tokyo.ap-northeast-1.internal",
		Port: 2222, User: "postgres", Auth: store.AuthPassword}
	s := &session{id: 1, host: long, state: sessLive}
	m.ssh.sessions = []*session{s}

	for _, innerW := range []int{24, 20, 16, 12} {
		lines := m.ssh.listItem(s, false, innerW)
		joined := strings.Join(lines, "\n")
		if !strings.Contains(joined, "2222") {
			t.Errorf("innerW=%d: the port was truncated away\n%s", innerW, joined)
		}
		if len(lines) < 2 {
			t.Errorf("innerW=%d: a 49-char address should have wrapped", innerW)
		}
		for i, l := range lines {
			if dispW(l) != innerW {
				t.Errorf("innerW=%d line %d: width %d", innerW, i, dispW(l))
			}
		}
	}
}

// Quitting closes every live session, so it must ask — including along the path
// a user actually takes: connect (which lands in the PTY), Alt+Esc, then q.
func TestQuitFromSessionsAsksAndThenStops(t *testing.T) {
	m := openOne(t)
	s := m.ssh.sessions[0]

	// q inside the PTY belongs to the remote, not to sshu.
	next, _ := m.Update(keyMsg("q"))
	if next.(AppModel).confirm.isActive() {
		t.Error("q inside the PTY should go to the remote, not open a dialog")
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEscape, Alt: true})
	m = settle(next.(AppModel))
	m = pressA(m, "q")
	if !m.confirm.isActive() || m.confirm.action != confirmQuit {
		t.Fatal("q with a live session must ask before closing it")
	}

	// Cancelling leaves everything running.
	m = pressA(m, "esc")
	if s.pty.exited() {
		t.Fatal("cancelling the quit must not close the session")
	}

	m = pressA(m, "q")
	next, cmd := m.Update(keyMsg("enter"))
	m = next.(AppModel)
	if cmd == nil {
		t.Fatal("confirming should quit")
	}
	waitFor(t, "the session to be killed", func() bool { return s.pty.exited() })
}

// Which way the seam points. Both orders draw the same glyph at the same width,
// so the rendered row cannot tell them apart — but one of them cuts the notch
// out of the wrong tab.
func TestTheSeamBelongsToTheTabOnItsLeft(t *testing.T) {
	lit, unlit := focusColor, lipgloss.Color(baseHex)

	glyph, fg, bg := divider(lit, unlit)
	if glyph != dividerHard {
		t.Error("a lit-to-unlit seam needs the filled arrow")
	}
	if fg != lit || bg != unlit {
		t.Errorf("seam is fg=%v bg=%v, want the LEFT fill as ink", fg, bg)
	}

	glyph, fg, bg = divider(unlit, lit)
	if glyph != dividerHard {
		t.Error("an unlit-to-lit seam needs the filled arrow too")
	}
	if fg != unlit || bg != lit {
		t.Errorf("seam is fg=%v bg=%v, want the LEFT fill as ink", fg, bg)
	}

	if glyph, _, _ = divider(unlit, unlit); glyph != dividerSoft {
		t.Error("no colour change means the outlined arrow, not the filled one")
	}
}

// The tab row is ONE strip: two round caps for the whole thing, and an arrow
// between neighbours. WHICH arrow says where the lit segment is — a filled one
// carries a colour change, an outlined one only draws a line where the fill is
// the same on both sides. So the counts pin down the shape and the lit position
// together.
func TestTheTabRowIsOneStripWithOneLitSegment(t *testing.T) {
	for _, tc := range []struct {
		key         string
		solid, thin int
	}{
		{"1", 1, 1}, // lit|unlit, unlit|unlit
		{"2", 2, 0}, // unlit|lit, lit|unlit
		{"3", 1, 1}, // unlit|unlit, unlit|lit
	} {
		m := pressA(sized(sample(), 100, 26), tc.key)
		row := strings.Split(m.View(), "\n")[0]

		if got := strings.Count(row, capLeft); got != 1 {
			t.Errorf("%s: %d opening caps, want one strip", tc.key, got)
		}
		if got := strings.Count(row, capRight); got != 1 {
			t.Errorf("%s: %d closing caps, want one strip", tc.key, got)
		}
		if got := strings.Count(row, dividerHard); got != tc.solid {
			t.Errorf("%s: %d filled arrows, want %d", tc.key, got, tc.solid)
		}
		if got := strings.Count(row, dividerSoft); got != tc.thin {
			t.Errorf("%s: %d outlined arrows, want %d", tc.key, got, tc.thin)
		}
	}
}

// Panel titles wear capsules, and so does the tab row — which is only legible
// because a rule separates the two zones. Without it the two strips of capsules
// run together and read as one row of buttons, which is why the capsules came
// off these titles once and went back on only alongside the rule.
func TestPanelTitlesAreCapsulesUnderTheRule(t *testing.T) {
	got := panelChrome(30, []string{strings.Repeat(" ", 30)}, "[4] sessions", true)
	if !strings.Contains(got, "[4] sessions") {
		t.Fatal("the title should still be in the border")
	}
	for name, cap := range map[string]string{"left cap": capLeft, "right cap": capRight} {
		if !strings.Contains(got, cap) {
			t.Errorf("a panel title should use the capsule %s", name)
		}
	}

	// And the rule is there to hold the two zones apart.
	m := sized(sample(), 78, 24)
	lines := strings.Split(m.View(), "\n")
	if len(lines) < 2 {
		t.Fatal("no view")
	}
	rule := lines[1]
	if strings.TrimSpace(strings.ReplaceAll(stripANSI(rule), "─", "")) != "" {
		t.Errorf("row 1 should be the rule under the tabs, got %q", rule)
	}
	if dispW(rule) != 78 {
		t.Errorf("the rule should span the width, got %d", dispW(rule))
	}
}

// History is a view. Nothing in it can be selected, so nothing in it can be
// acted on — and j/k scroll it rather than moving a cursor that does not exist.
// It kept that character when it stopped being a panel.
func TestHistoryIsAViewNotAList(t *testing.T) {
	withColour(t)
	m := sshApp(t, sample())
	m.ssh.setSize(100, 28)
	m.historyUI.setSize(100, 12) // shorter than the list, so it can scroll
	for i := range 8 {
		m.ssh.history = append(m.ssh.history, &session{
			id: i + 1, host: sample()[i%len(sample())], state: sessEnded,
			reason: "exited 0", ok: true,
		})
	}

	// No row is ever painted as a cursor.
	m.historyUI.anim.phase = animOpen
	box := m.historyUI.view(m.ssh.history)
	for name, bg := range map[string]string{
		"cursor": ansiBgOf(t, handColor), "green": ansiBgOf(t, liveColor),
	} {
		if strings.Contains(box, bg) {
			t.Errorf("history must not paint a %s bar — it has no cursor", name)
		}
	}

	// j/k scroll the view, and it does not wrap.
	before := m.historyUI.top
	m.historyUI.update(keyMsg("j"), len(m.ssh.history))
	if m.historyUI.top != before+1 {
		t.Errorf("j should scroll, top=%d want %d", m.historyUI.top, before+1)
	}
	m.historyUI.update(keyMsg("k"), len(m.ssh.history))
	m.historyUI.update(keyMsg("k"), len(m.ssh.history))
	if m.historyUI.top != 0 {
		t.Errorf("k should scroll back and clamp, top=%d", m.historyUI.top)
	}

	// And it is reachable: [H] is a panel action on [4], not a lost feature.
	found := false
	for _, it := range m.sshMenuItems() {
		if it.key == "H" {
			found = true
		}
	}
	if !found {
		t.Error("the Space menu should offer History")
	}
}

// TestDumpSSH is not an assertion — run with -v to eyeball tab [3].
func TestDumpSSH(t *testing.T) {
	if !testing.Verbose() {
		t.Skip("run with -v to print tab [3]")
	}
	aliveSSH(t)
	m := New([]store.Host{
		{Name: "prod-web-01", Host: "10.0.3.14", Port: 22, User: "deploy", Auth: store.AuthPrivateKey},
		{Name: "db-replica-tokyo-ap-northeast-1", Host: "db.internal.corp", Port: 2222,
			User: "postgres", Auth: store.AuthPassword},
	}, nil)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 92, Height: 24})
	m = settle(next.(AppModel))
	m.ssh.setSize(92, 22)

	for _, h := range m.hosts.hosts {
		if _, err := m.ssh.connect(h); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := m.ssh.connect(m.hosts.hosts[0]); err != nil { // a duplicate, for #N
		t.Fatal(err)
	}
	defer m.ssh.stopAll()

	m.tab = tabSSH
	m.ssh.current = m.ssh.sessions[0].id
	m.ssh.setFocus(panelSessions)
	t.Logf("\n=== tab [3], focus [4] ===\n%s", m.View())

	m.ssh.setFocus(panelPty)
	t.Logf("\n=== focus [5] (footer changes) ===\n%s", m.View())
}

// A history row carries the time it ended, on the name line where the space was
// going unused. Time only: history dies with the process, so a date could only
// Duplicate opens a second session to the host already under the cursor — the
// point is not having to go back to [1] for it.
func TestDuplicateOpensASecondSessionToTheSameHost(t *testing.T) {
	m := openOne(t)
	first := m.ssh.sessions[0]
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEscape, Alt: true})
	m = settle(next.(AppModel))

	// Upper case: lower-case d is half-page-down on the session list.
	m = pressA(m, "d")
	if m.confirm.isActive() {
		t.Fatal("lower-case d is navigation, it must not open a confirm")
	}
	m = pressA(m, "D")
	if !m.confirm.isActive() || m.confirm.action != confirmDuplicate {
		t.Fatal("D should ask before opening another connection")
	}
	m = pressA(m, "enter")
	t.Cleanup(func() { m.ssh.stopAll() })

	if len(m.ssh.sessions) != 2 {
		t.Fatalf("expected two sessions, got %d", len(m.ssh.sessions))
	}
	if m.ssh.sessions[1].host.Name != first.host.Name {
		t.Errorf("the duplicate should target the same host, got %q", m.ssh.sessions[1].host.Name)
	}
	if m.ssh.sessions[1].id == first.id {
		t.Error("the duplicate should be a distinct session")
	}
	// Two sessions to one host: the ordinal is what tells them apart.
	if a, b := first.ordinalTag(), m.ssh.sessions[1].ordinalTag(); a == "" || b == "" || a == b {
		t.Errorf("expected distinct ordinals, got %q and %q", a, b)
	}
	if m.ssh.current != m.ssh.sessions[1].id {
		t.Error("the new session should be the one on screen")
	}
	if first.pty.exited() {
		t.Error("duplicating must not disturb the session it copied")
	}
}

// It duplicates what is ON SCREEN, not what hosts.yaml happens to say now.
func TestDuplicateUsesTheSessionHostNotTheHostsFile(t *testing.T) {
	m := openOne(t)
	first := m.ssh.sessions[0]
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEscape, Alt: true})
	m = settle(next.(AppModel))

	m.hosts.hosts = nil // the entry is gone from [1]
	m = pressA(m, "D", "enter")
	t.Cleanup(func() { m.ssh.stopAll() })

	if len(m.ssh.sessions) != 2 {
		t.Fatalf("a deleted hosts entry must not block duplicating, got %d sessions",
			len(m.ssh.sessions))
	}
	if m.ssh.sessions[1].host.Host != first.host.Host {
		t.Error("the duplicate should carry the session own connection details")
	}
}

// Close ends the session under the cursor, and asks first.
func TestCloseEndsTheSession(t *testing.T) {
	m := openOne(t)
	s := m.ssh.sessions[0]
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEscape, Alt: true})
	m = settle(next.(AppModel))

	m = pressA(m, "C")
	if !m.confirm.isActive() || m.confirm.action != confirmClose {
		t.Fatal("c should ask before killing a session")
	}
	m = pressA(m, "esc")
	if s.pty.exited() {
		t.Fatal("cancelling must leave the session running")
	}

	m = pressA(m, "C", "enter")
	waitFor(t, "the session to end", func() bool { return s.pty.exited() })
}

// Every row of the [4] Space menu must run [4] own action, not the action that
// happens to share its letter in another tab. This is the test that was missing
// when the menu dispatched every commit to tab [1]: [C]lose opened the new-host
// form and [D]uplicate reported "No host selected".
//
// It runs each action twice — once by hotkey, once through the menu — because
// the bug was a divergence between those two paths, not in either action.
func TestSSHMenuRowsRunTheirOwnActions(t *testing.T) {
	wantAction := map[string]confirmAction{
		"Close":     confirmClose,
		"Duplicate": confirmDuplicate,
	}
	// Open lands in the pty and History opens a popup; the rest ask first.
	opens := map[string]func(AppModel) bool{
		"Open":    func(m AppModel) bool { return m.ssh.focus == panelPty },
		"History": func(m AppModel) bool { return m.historyUI.isActive() },
	}
	for _, a := range sshActions {
		if a.panel != panelSessions {
			continue
		}
		for _, how := range []string{"hotkey", "menu"} {
			m := openOne(t)
			next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEscape, Alt: true})
			m = settle(next.(AppModel))
			if how == "menu" {
				m = pressA(m, " ")
				if !m.spaceMenu.isActive() {
					t.Fatal("Space did not open the menu")
				}
			}
			m = pressA(m, a.key)

			if want, ok := opens[a.label]; ok {
				if !want(m) {
					t.Errorf("%s/%s: did not open what it names", a.label, how)
				}
			} else if !m.confirm.isActive() {
				t.Errorf("%s/%s: expected a confirmation, got none", a.label, how)
			} else if m.confirm.action != wantAction[a.label] {
				t.Errorf("%s/%s: ran the wrong action (%d, want %d)",
					a.label, how, m.confirm.action, wantAction[a.label])
			}
			// And nothing belonging to tab [1] should have been touched.
			if m.form.isActive() {
				t.Errorf("%s/%s: opened the hosts form", a.label, how)
			}
			m.ssh.stopAll()
		}
	}
}

// stripANSI removes escape sequences so a test can look at the characters a row
// is actually made of.
func stripANSI(s string) string {
	var b strings.Builder
	in := false
	for _, r := range s {
		switch {
		case r == 0x1b:
			in = true
		case in && r == 'm':
			in = false
		case !in:
			b.WriteRune(r)
		}
	}
	return b.String()
}
