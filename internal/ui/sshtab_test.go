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

// aliveSSH stands in for a CONNECTED host: it says something and then stays up
// until its PTY is closed. The greeting is not decoration — a real ssh session
// prints a prompt the moment it is through, and that first byte is what tells
// sshu the wait is over (ptyTerm.spoke). A stand-in that never speaks is a
// stand-in for a connection still being made, which is silentSSH below.
func aliveSSH(t *testing.T) { fakeSSH(t, "printf '$ '; exec cat") }

// silentSSH stands in for a connection that has not been made yet: it holds the
// PTY open and says nothing, which is exactly what ssh does while it waits for
// TCP on an address that never answers.
func silentSSH(t *testing.T) { fakeSSH(t, "exec cat") }

func sshApp(t *testing.T, hosts []store.Host) AppModel {
	t.Helper()
	m := New(hosts, nil, store.DefaultConfig())
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
	// Wait for the greeting, because "connected" now means the far end answered
	// rather than "a process started". A test that skipped this would be holding
	// a session sshu still considers to be connecting — which is a real state,
	// just not this one.
	waitFor(t, "the stand-in to answer", func() bool {
		return m.ssh.sessions[0].pty.hasSpoken()
	})
	return m
}

// waitFor polls until cond holds, so a test never depends on a fixed sleep.
// The ceiling is generous because a loaded CI runner can take seconds just to
// reap a killed child under -race; a passing condition returns immediately, so
// only the failure case ever pays it.
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
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
		{54, 30}, {53, 30}, {54, 16}, {40, 12}, {34, 9}} {
		w, h := sz[0], sz[1]
		m := New(sample(), nil, store.DefaultConfig())
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
	_, rightW := m.ssh.panes()
	if rightW != sshNarrowW-1 {
		t.Errorf("the grid should take the whole width, got %d", rightW)
	}
}

// ------------------------------------------------------------------ the PTY

// A connection that fails leaves TWO marks: the panel that was watching it says
// what happened, and the log keeps it. The toast is the third, and it is the one
// that disappears — which is why the other two exist.
func TestAFailedConnectionIsSaidAndKept(t *testing.T) {
	fakeSSH(t, "printf 'ssh: connect to host db.internal.corp port 22: Connection refused\\n' >&2; exit 255")
	m := pressA(sshApp(t, sample()), "enter", "enter")
	s := m.ssh.sessions[0]
	waitFor(t, "ssh to give up", func() bool { return s.pty.exited() })

	next, _ := m.Update(sshTickMsg{})
	m = settle(next.(AppModel))

	// What ssh SAID, not what its exit code implied.
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "Connection refused") {
		t.Errorf("[5] does not say why it failed:\n%s", view)
	}
	if strings.Contains(view, "Nothing on the grid") {
		t.Errorf("[5] went blank instead of reporting:\n%s", view)
	}

	// The footer stops being quiet about it.
	if !strings.Contains(view, "1 unread error") {
		t.Errorf("the footer does not say there is something to read:\n%s", view)
	}

	// And the log kept it, in the words the remote used — as the newest entry,
	// after the "connecting" event that opened the attempt.
	last := m.log.entries[len(m.log.entries)-1]
	if last.level != logError {
		t.Fatalf("the newest entry should be the failure, got level %d: %q", last.level, last.msg)
	}
	if !strings.Contains(last.msg, "Connection refused") {
		t.Errorf("the entry lost the reason: %q", last.msg)
	}
	// Reading it happens where the log lives now: preference → logs. The line
	// is WRAPPED onto the panel, not truncated — the word that says why is at
	// the end, which is exactly what a cut tail would eat.
	m = pressA(m, "M", "1", "j", "j") // to the nav, then hosts → credentials → logs
	if m.pref.item != prefLogs {
		t.Fatalf("expected the logs section, got %d", m.pref.item)
	}
	logView := ansi.Strip(m.View())
	if !strings.Contains(logView, "refused") {
		t.Errorf("the rendered log dropped the tail of the reason:\n%s", logView)
	}
	// Having it on screen is reading it.
	if m.log.unreadErrors() != 0 {
		t.Errorf("%d errors still unread with the log on screen", m.log.unreadErrors())
	}
}

// The timeout reaches ssh as its OWN option, which is what lets ssh produce the
// message. sshu killing the process would have produced a corpse with no
// explanation attached.
func TestTheTimeoutIsHandedToSSH(t *testing.T) {
	m := New(sample(), nil, store.Config{ConnectTimeout: 3})
	if got := m.ssh.timeoutSecs(); got != 3 {
		t.Errorf("the model carries %ds, want 3", got)
	}
	args := strings.Join(buildSSHCmd(sample()[0], "", m.ssh.timeoutSecs()).Args, " ")
	if !strings.Contains(args, "ConnectTimeout=3") {
		t.Errorf("ssh was not told: %s", args)
	}
}

// ssh's own ConnectTimeout covers the TCP connect. It does NOT cover a host
// that completes the connection and then says nothing — and from the outside
// that is indistinguishable from a connection still being made, so it would
// spin until the user gave up on the app rather than on the host.
func TestAStalledConnectionIsGivenUpOn(t *testing.T) {
	silentSSH(t)
	m := New(sample(), nil, store.Config{ConnectTimeout: 1})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = settle(next.(AppModel))
	m = pressA(m, "enter", "enter")
	s := m.ssh.sessions[0]
	t.Cleanup(func() { m.ssh.stopAll() })

	// Before the deadline it is still trying — the sweep must not be trigger
	// happy, because a slow link is not a broken one.
	m.ssh.sweepStalled()
	if s.timedOut {
		t.Fatal("gave up before the budget was spent")
	}

	// ssh gets its own timeout plus the grace before sshu reaches for the plug.
	s.started = time.Now().Add(-(1*time.Second + stallGrace + time.Second))
	m.ssh.sweepStalled()
	if !s.timedOut {
		t.Fatal("a connection that never answered was never given up on")
	}

	waitFor(t, "the stopped session to be reaped", func() bool { return s.pty.exited() })
	ended := m.ssh.reap()
	if len(ended) != 1 {
		t.Fatalf("%d sessions retired", len(ended))
	}
	// And it says the budget it actually spent, not an exit code nobody can read.
	if !strings.Contains(ended[0].reason, "no answer after 1s") {
		t.Errorf("reason is %q", ended[0].reason)
	}
	if ended[0].ok {
		t.Error("a timeout is not a clean exit")
	}
}

// A failure is often not one line. A host key mismatch is a banner, a
// fingerprint and an offending known_hosts line — and the LAST of them is only
// "Host key verification failed", which is the one line that tells you nothing
// you did not already know. The fingerprint is the part you need and it is in
// the middle, so the log keeps the whole screen.
func TestTheLogKeepsTheWholeFailureNotJustItsLastLine(t *testing.T) {
	fakeSSH(t, `cat >&2 <<'EOF'
@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@
@  WARNING: REMOTE HOST IDENTIFICATION HAS CHANGED!  @
@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@
IT IS POSSIBLE THAT SOMEONE IS DOING SOMETHING NASTY!
The fingerprint for the ED25519 key sent by the remote host is
SHA256:uNiVeRsAlLyUnIqUeFiNgErPrInTvAlUe1234567890.
Offending ECDSA key in /Users/you/.ssh/known_hosts:42
Host key verification failed.
EOF
exit 255`)
	m := pressA(sshApp(t, sample()), "enter", "enter")
	s := m.ssh.sessions[0]
	waitFor(t, "ssh to give up", func() bool { return s.pty.exited() })

	next, _ := m.Update(sshTickMsg{})
	m = settle(next.(AppModel))

	// The headline is the last line, because that is what a toast can hold.
	if !strings.Contains(s.reason, "Host key verification failed") {
		t.Errorf("headline is %q", s.reason)
	}
	// The log has the parts a headline had no room for. The failure is ONE
	// entry — the newest, after the "connecting" event.
	body := m.log.entries[len(m.log.entries)-1].msg
	for _, want := range []string{
		"REMOTE HOST IDENTIFICATION HAS CHANGED",
		"SHA256:uNiVeRsAlLyUnIqUeFiNgErPrInT",
		"known_hosts:42",
		"Host key verification failed",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the log lost %q:\n%s", want, body)
		}
	}

	// And it is one ENTRY, not eight — one thing happened.
	if n := strings.Count(body, "\n"); n < 5 {
		t.Errorf("the entry has only %d line breaks, want the whole screen", n)
	}
}

// A long entry is scrollable to its end: the viewport counts rendered ROWS, and
// one entry is not one row.
func TestTheLogScrollsThroughALongEntry(t *testing.T) {
	m := sshApp(t, sample())
	long := make([]string, 30)
	for i := range long {
		long[i] = "line " + itoa(i)
	}
	m.log.errorf("something went wrong", long...)

	const w, h = 80, 12
	rows := len(m.log.allRows(w))
	if rows < 30 {
		t.Fatalf("%d rendered rows, want the whole entry", rows)
	}
	for range 40 {
		m.log.scrollKey("j", w, h)
	}
	if m.log.top == 0 {
		t.Error("j never scrolled")
	}
	if m.log.top > rows-h {
		t.Errorf("scrolled past the end: top=%d rows=%d", m.log.top, rows)
	}
	// The last line is reachable.
	body := ansi.Strip(strings.Join(m.log.body(w, h), "\n"))
	if !strings.Contains(body, "line 29") {
		t.Errorf("the end of the entry is unreachable:\n%s", body)
	}
}

// Keys typed at a connection that has not answered are NOT sent. ssh is not
// reading its stdin while it waits, so they would be delivered to the remote
// shell whenever it finally arrives — a `q` meant for sshu, run minutes later on
// somebody else's machine.
func TestKeysAreNotSentToAConnectionThatHasNotAnswered(t *testing.T) {
	silentSSH(t)
	m := pressA(sshApp(t, sample()), "enter", "enter")
	t.Cleanup(func() { m.ssh.stopAll() })

	if m.inPty() {
		t.Fatal("a connection that has not answered is not a remote to type at")
	}
	if !m.ptyFocused() {
		t.Fatal("the panel should still hold the keyboard")
	}
	// And q does not quit sshu either: the panel has the keyboard, so it eats it.
	next, cmd := m.Update(keyMsg("q"))
	if cmd != nil {
		t.Error("q while connecting should do nothing at all")
	}
	if next.(AppModel).confirm.isActive() {
		t.Error("q while connecting should not raise the quit confirm")
	}
	// Alt+Esc still works, because being stuck needs a way out.
	next, _ = m.Update(keyMsg("alt+esc"))
	if next.(AppModel).ssh.focus == panelPty {
		t.Error("alt+esc must leave a connecting pty")
	}
}

// A live session that has said NOTHING yet is not an empty terminal, it is a
// wait — and the two look identical: an empty bordered box.
//
// ssh prints nothing while it waits for a TCP connection, so against an address
// that never answers that box stays blank for as long as the OS takes to give
// up, which can run past a minute. `cat` stands in here for exactly that
// property: it says nothing until it is spoken to.
func TestAConnectingSessionSaysSo(t *testing.T) {
	silentSSH(t)
	m := pressA(sshApp(t, sample()), "enter", "enter")
	t.Cleanup(func() { m.ssh.stopAll() })
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

// Ctrl+C is the most reflexive key a shell has. It used to quit sshu from inside
// a session — so killing a runaway command on the far end killed every other
// session too, without even the confirmation `q` asks for.
func TestCtrlCInsideAPtyInterruptsTheRemoteNotSshu(t *testing.T) {
	// A bare `cat` would DIE of the SIGINT the PTY raises from \x03 — the
	// line discipline turns the byte into a signal for the foreground group,
	// which is exactly what a real remote shell survives and a stand-in has to
	// as well. trap makes the disposition SIG_IGN, and a child inherits that.
	// -v so the byte can be seen arriving (the terminal echoes it as ^C).
	fakeSSH(t, `trap "" INT; printf '$ '; cat -v`)
	m := pressA(sshApp(t, sample()), "enter", "enter")
	t.Cleanup(func() { m.ssh.stopAll() })
	s := m.ssh.sessions[0]
	waitFor(t, "the stand-in to answer", func() bool { return s.pty.hasSpoken() })

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	m = next.(AppModel)
	if cmd != nil {
		t.Error("Ctrl+C at a remote must not raise a command — least of all tea.Quit")
	}
	waitFor(t, "the interrupt to arrive at the remote", func() bool {
		return strings.Contains(strings.Join(s.pty.render(80, 24), ""), "^C")
	})
	if s.pty.exited() {
		t.Error("the session must survive its own interrupt")
	}
	if len(m.ssh.sessions) != 1 {
		t.Errorf("sessions = %d, want the one that was there", len(m.ssh.sessions))
	}
}

// And it is still the emergency exit everywhere else, including on the tab that
// owns the sessions — one Alt+Esc away from the remote.
func TestCtrlCStillQuitsOnceTheKeyboardIsBack(t *testing.T) {
	m := openOne(t)
	m = pressA(m, "alt+esc")
	if m.inPty() {
		t.Fatal("alt+esc should have taken the keyboard back")
	}
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Fatal("Ctrl+C outside a pty is the emergency exit")
	}
	if _, isQuit := cmd().(tea.QuitMsg); !isQuit {
		t.Error("Ctrl+C should quit, without asking")
	}
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
	}{{"1", panelSessions}, {"2", panelLayout}} {
		m := pressA(sshApp(t, sample()), "S", tc.key)
		if m.ssh.focus != tc.want {
			t.Errorf("%s in the ssh tab should focus panel %d, got %d", tc.key, tc.want, m.ssh.focus)
		}
	}

	// On the preference tab the same digits do nothing at all: no tab change,
	// no focus change, no surprise.
	for _, k := range []string{"1", "2", "3", "4"} {
		m := sshApp(t, sample())
		before := m.ssh.focus
		m = pressA(m, k)
		if m.tab != tabPref {
			t.Errorf("%s from the preference tab must not switch tabs, got %d", k, m.tab)
		}
		if m.ssh.focus != before {
			t.Errorf("%s from the preference tab must not move focus elsewhere", k)
		}
	}
}

// Tab must never walk focus into the grid: it would swallow the key that got
// you there. On this tab it is the display toggle instead, and with no
// sessions it does nothing at all.
func TestTabNeverEntersThePty(t *testing.T) {
	m := pressA(sshApp(t, sample()), "S")
	for i := 0; i < 6; i++ {
		m = pressA(m, "tab")
		if m.tab != tabSSH {
			t.Fatal("Tab left the tab")
		}
		if m.ssh.focus == panelPty {
			t.Fatal("Tab reached the PTY — from there Tab belongs to the remote")
		}
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
	m = pressA(m, "M", "l", "enter", "enter") // connect to a second host
	t.Cleanup(func() { m.ssh.stopAll() })

	if len(m.ssh.sessions) != 2 {
		t.Fatalf("expected two sessions, got %d", len(m.ssh.sessions))
	}
	shown := append([]int(nil), m.ssh.shown...)
	m.ssh.setFocus(panelSessions)
	m = pressA(m, "k") // move the cursor off the shown session

	if len(m.ssh.shown) != len(shown) || m.ssh.shown[0] != shown[0] {
		t.Error("moving the cursor must not change the grid")
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
	m = pressA(m, "M", "j", "enter", "enter") // a second session, to a second host
	t.Cleanup(func() { m.ssh.stopAll() })
	if len(m.ssh.sessions) != 2 {
		t.Fatalf("expected two sessions, got %d", len(m.ssh.sessions))
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEscape, Alt: true})
	m = settle(next.(AppModel))

	// The row already on the grid: Enter just goes in.
	m = pressA(m, "enter")
	if m.confirm.isActive() {
		t.Fatal("Enter on a session must not open a dialog")
	}
	if m.ssh.focus != panelPty {
		t.Errorf("Enter should take focus into the PTY, got %d", m.ssh.focus)
	}
	was := m.ssh.currentSession().id

	// A different row: Enter moves the keyboard, still without asking.
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEscape, Alt: true})
	m = settle(next.(AppModel))
	m = pressA(m, "k")
	m = pressA(m, "enter")
	if m.confirm.isActive() {
		t.Fatal("Enter on another session must not open a dialog either")
	}
	if m.ssh.currentSession().id == was {
		t.Error("Enter should have moved the keyboard to the other session")
	}
	if m.ssh.focus != panelPty {
		t.Errorf("Enter should land in the PTY, got %d", m.ssh.focus)
	}
	if len(m.ssh.sessions) != 2 {
		t.Error("switching must not close the session being left")
	}
}

// A dead session leaves the live list carrying the reason it ended, and that
// reason is what reaches the log.
func TestExitedSessionLeavesWithItsReason(t *testing.T) {
	fakeSSH(t, "exit 7")
	m := pressA(sshApp(t, sample()), "enter", "enter")
	s := m.ssh.sessions[0]

	waitFor(t, "the subprocess to exit", func() bool { return s.pty.exited() })
	ended := m.ssh.reap()
	if len(ended) != 1 {
		t.Fatal("reap should have retired the finished session")
	}
	if len(m.ssh.sessions) != 0 {
		t.Fatalf("%d sessions still live", len(m.ssh.sessions))
	}
	if got := ended[0].reason; got != "exited 7" {
		t.Errorf("reason %q, want %q", got, "exited 7")
	}
	if ended[0].state != sessEnded {
		t.Error("the session should be marked ended")
	}
	if m.ssh.focus == panelPty {
		t.Error("focus must not stay in a PTY nobody is driving")
	}
}

// Hide is on H, and Tab does the same thing. Both halves matter and neither
// implies the other: the menu row is marked [H]ide, so H has to work or the
// marking is a lie; and Tab is this tab's own convention (§4.4.1), so dropping
// it would break a key nothing in the menu was ever responsible for (§11.30).
func TestHideIsOnHAndAlsoOnTab(t *testing.T) {
	for _, key := range []string{"H", "tab"} {
		m := openOne(t)
		next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEscape, Alt: true})
		m = settle(next.(AppModel))
		t.Cleanup(func() { m.ssh.stopAll() })
		if len(m.ssh.shown) != 1 {
			t.Fatalf("%s: setup — a new session starts on the grid, shown=%v", key, m.ssh.shown)
		}
		if m = pressA(m, key); len(m.ssh.shown) != 0 {
			t.Errorf("%s should take the cell off the grid, shown=%v", key, m.ssh.shown)
		}
		if m = pressA(m, key); len(m.ssh.shown) != 1 {
			t.Errorf("%s should put it back, shown=%v", key, m.ssh.shown)
		}
	}

	// And the marking says H, not something else that happens to work.
	for _, a := range sshActions {
		if a.label == "Hide" && a.key != "H" {
			t.Errorf("the Hide row is marked %q — the bracket is the disclosure", a.key)
		}
	}
}

// Two sessions to one host draw two identical entries, and that is the accepted
// answer rather than an oversight. The #N that used to distinguish them keyed on
// the hosts.yaml NAME while the entry drew the ADDRESS, so it tagged the wrong
// pairs in both directions: two entries pointing at one box got no tag, and one
// entry edited between two connects got #1/#2 on two different machines
// (§11.32). What the list still promises is that there are two of them, in the
// order they were opened, which the cursor walks.
func TestTwoSessionsToOneHostAreTwoEntries(t *testing.T) {
	aliveSSH(t)
	m := sshApp(t, sample())
	m.ssh.setSize(100, 28)
	for range 2 {
		if _, err := m.ssh.connect(sample()[0]); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() { m.ssh.stopAll() })

	if len(m.ssh.sessions) != 2 {
		t.Fatalf("two connects should be two sessions, got %d", len(m.ssh.sessions))
	}
	innerW, _ := m.ssh.listInner()
	a := m.ssh.listItem(m.ssh.sessions[0], false, innerW)
	b := m.ssh.listItem(m.ssh.sessions[1], false, innerW)
	if len(a) != sshItemH || len(b) != sshItemH {
		t.Fatalf("each should be %d lines, got %d and %d", sshItemH, len(a), len(b))
	}
	for i := range a {
		if a[i] != b[i] {
			t.Errorf("line %d differs, so something is still tagging them:\n%q\n%q", i, a[i], b[i])
		}
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
	got := strings.Join(buildSSHCmd(h, "", 9).Args[1:], " ")
	for _, want := range []string{"-p 2222", "-o ConnectTimeout=9", "-i ",
		"IdentitiesOnly=yes", "root@h.example.com"} {
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
	if !m.ssh.isShown(s.id) {
		t.Fatal("the new session should have a cell on the grid")
	}

	waitFor(t, "the subprocess to exit", func() bool { return s.pty.exited() })
	m.ssh.reap()

	if len(m.ssh.shown) != 0 {
		t.Errorf("the ended session's cell should leave the grid, shown=%v", m.ssh.shown)
	}
	if m.ssh.currentSession() != nil {
		t.Error("currentSession must only ever be a live session")
	}
	if s.pty != nil {
		t.Error("the emulator should be released — nothing renders it any more")
	}

	m.ssh.setFocus(panelSessions)
	if !strings.Contains(m.ssh.view(), "Nothing on the grid") {
		t.Error("the grid should be back to its empty state")
	}
}

// Focusing the PTY folds the lists away and gives it the whole tab; leaving
// brings them back. The remote is resized both ways, or it paints to the wrong
// geometry.
func TestFocusedPtyTakesTheWholeTab(t *testing.T) {
	m := openOne(t)
	s := m.ssh.sessions[0]
	leftW, rightW := m.ssh.panes()
	if leftW != 0 || rightW != m.ssh.w {
		t.Fatalf("a focused PTY should own the tab: left=%d right=%d w=%d", leftW, rightW, m.ssh.w)
	}
	full := s.appliedCols

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEscape, Alt: true})
	m = next.(AppModel)
	leftW, _ = m.ssh.panes()
	if leftW != sshLeftW {
		t.Fatalf("leaving the PTY should bring the lists back, left=%d", leftW)
	}
	if s.appliedCols >= full {
		t.Errorf("the remote should be narrower once the lists are back: %d -> %d",
			full, s.appliedCols)
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
	m.ssh.shown = []int{1}

	green, hand := ansiOf(t, liveColor), ansiOf(t, handColor)
	greenBG, handBG := ansiBgOf(t, liveColor), ansiBgOf(t, handColor)

	// An entry is two lines and both wear whatever it is wearing — half a
	// highlighted entry reads as the cursor sitting between two things.
	joined := func(s *session, isCursor bool) string {
		return strings.Join(m.ssh.listItem(s, isCursor, 24), "\n")
	}
	each := func(s *session, isCursor bool) []string {
		return m.ssh.listItem(s, isCursor, 24)
	}

	// Foreground says on-screen.
	row := joined(shown, false)
	if !strings.Contains(row, green) {
		t.Error("the on-screen session should be green")
	}
	if strings.Contains(row, greenBG) {
		t.Error("green belongs in the foreground, not behind the row")
	}

	// Background says cursor — the same bar on every row, including that one.
	row = joined(other, true)
	if !strings.Contains(row, handBG) {
		t.Error("the cursor should be a filled bar")
	}
	for i, ln := range each(other, true) {
		if !strings.Contains(ln, handBG) {
			t.Errorf("line %d of the cursor entry is not on the bar: %q", i, ln)
		}
	}
	row = joined(shown, true)
	if !strings.Contains(row, handBG) {
		t.Error("the cursor over the on-screen session is the same bar")
	}
	if strings.Contains(row, greenBG) {
		t.Error("there is no inverse case any more — green never becomes a background")
	}

	// And an ordinary row is neither.
	row = joined(other, false)
	for name, seq := range map[string]string{"green": green, "cursor": hand} {
		if strings.Contains(row, seq) {
			t.Errorf("an ordinary row should carry no %s", name)
		}
	}
}

// An entry says both halves of what a session is: what it is called, and what
// the connection IS. It used to say only the second — one line could hold only
// one of them — so a list of sessions never showed a name the hosts table is
// entirely made of.
func TestSessionEntryShowsTheNameAndTheAddress(t *testing.T) {
	m := sshApp(t, sample())
	m.ssh.setSize(100, 28)
	h := sample()[0]
	s := &session{id: 1, host: h, state: sessLive}
	m.ssh.sessions = []*session{s}

	lines := m.ssh.listItem(s, false, 40)
	if len(lines) != sshItemH {
		t.Fatalf("an entry is %d lines, got %d", sshItemH, len(lines))
	}
	name, addr := ansi.Strip(lines[0]), ansi.Strip(lines[1])
	if !strings.Contains(name, h.Name) {
		t.Errorf("the first line is %q, want the name %q", name, h.Name)
	}
	if want := h.User + "@" + h.Host; !strings.Contains(addr, want) {
		t.Errorf("the second line is %q, want it to carry %q", addr, want)
	}
	if !strings.Contains(addr, strconv.Itoa(h.Port)) {
		t.Errorf("the port is missing: %q", addr)
	}
}

// The port is the one thing in a [1] entry that must never be cut: the ADDRESS
// shortens against it, however long the address is and however narrow the
// column gets (§11.32). The entry stays two lines of exactly innerW throughout —
// that constant height is what the scrolling divides by.
func TestSessionEntryAlwaysShowsThePort(t *testing.T) {
	m := sshApp(t, sample())
	m.ssh.setSize(100, 28)

	long := store.Host{Name: "db-replica-tokyo-ap-northeast-1",
		Host: "db-replica-tokyo.ap-northeast-1.internal",
		Port: 2222, User: "postgres", Auth: store.AuthPassword}
	s := &session{id: 1, host: long, state: sessLive}
	m.ssh.sessions = []*session{s}

	for _, innerW := range []int{24, 20, 16, 12} {
		lines := m.ssh.listItem(s, false, innerW)
		if len(lines) != sshItemH {
			t.Errorf("innerW=%d: %d lines, want %d", innerW, len(lines), sshItemH)
			continue
		}
		if !strings.Contains(ansi.Strip(lines[1]), "2222") {
			t.Errorf("innerW=%d: the port was truncated away\n%s", innerW, ansi.Strip(lines[1]))
		}
		for i, ln := range lines {
			if strings.Contains(ln, "\n") {
				t.Errorf("innerW=%d line %d: wrapped instead of shortening\n%s", innerW, i, ansi.Strip(ln))
			}
			if got := dispW(ln); got != innerW {
				t.Errorf("innerW=%d line %d: width %d", innerW, i, got)
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
// between neighbours. Exactly one segment is lit, so WHICH arrows are filled
// pins down which — a filled arrow carries a colour change, an outlined one
// only draws a line where the fill matches on both sides.
//
// There used to be a lit [Alt] lead in front, and the counts below were three
// seams rather than two. Bare letters spell no chord, so there is nothing for a
// second lit segment to be half of.
func TestTheTabRowIsOneStrip(t *testing.T) {
	for _, tc := range []struct {
		key         string
		solid, thin int
	}{
		{"M", 1, 1}, // M lit: M|F changes colour, F|S does not
		{"F", 2, 0}, // the lit tab is in the middle — both seams change
		{"S", 1, 1}, // F|S changes colour, M|F does not
	} {
		m := pressA(sized(sample(), 100, 26), tc.key)
		row := strings.Split(m.View(), "\n")[0]

		if strings.Contains(row, "[Alt]") {
			t.Errorf("%s: the strip must not still carry a chord lead", tc.key)
		}
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
	got := panelChrome(30, []string{strings.Repeat(" ", 30)}, "[1] sessions", true)
	if !strings.Contains(got, "[1] sessions") {
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

// The app log is a view. Nothing in it can be selected, so nothing in it can be
// acted on — j/k scroll it rather than moving a cursor that does not exist.
func TestTheAppLogIsAViewNotAList(t *testing.T) {
	withColour(t)
	m := sshApp(t, sample())
	for i := range 12 {
		m.log.errorf("prod-web-0" + itoa(i%9+1) + " · Connection refused")
	}

	// No row is ever painted as a cursor.
	box := strings.Join(m.log.body(96, 8), "\n") // shorter than the rows, so it scrolls
	for name, bg := range map[string]string{
		"cursor": ansiBgOf(t, handColor), "green": ansiBgOf(t, liveColor),
	} {
		if strings.Contains(box, bg) {
			t.Errorf("the log must not paint a %s bar — it has no cursor", name)
		}
	}

	// j/k scroll the view, and it does not wrap.
	before := m.log.top
	m.log.scrollKey("j", 96, 8)
	if m.log.top != before+1 {
		t.Errorf("j should scroll, top=%d want %d", m.log.top, before+1)
	}
	m.log.scrollKey("k", 96, 8)
	m.log.scrollKey("k", 96, 8)
	if m.log.top != 0 {
		t.Errorf("k should scroll back and clamp, top=%d", m.log.top)
	}
}

func TestDumpSSH(t *testing.T) {
	if !testing.Verbose() {
		t.Skip("run with -v to print tab [3]")
	}
	aliveSSH(t)
	m := New([]store.Host{
		{Name: "prod-web-01", Host: "10.0.3.14", Port: 22, User: "deploy", Auth: store.AuthPrivateKey},
		{Name: "db-replica-tokyo-ap-northeast-1", Host: "db.internal.corp", Port: 2222,
			User: "postgres", Auth: store.AuthPassword},
	}, nil, store.DefaultConfig())
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
	// They are distinct SESSIONS with distinct PTYs; what they are not is
	// distinguishable on the list, and that is settled rather than missed
	// (§11.32 — see TestTwoSessionsToOneHostAreTwoEntries).
	if m.ssh.sessions[1].pty == first.pty {
		t.Error("the duplicate should have a PTY of its own")
	}
	// The keyboard STAYS on [1] — the Enter that ran this was an Enter on a
	// confirmation, not on a session row (§11.23).
	if m.ssh.focus != panelSessions {
		t.Errorf("duplicating must leave the keyboard on [1], focus=%d", m.ssh.focus)
	}
	if m.ssh.curSess != 1 {
		t.Errorf("the cursor should be on the new session, curSess=%d", m.ssh.curSess)
	}
	if first.pty.exited() {
		t.Error("duplicating must not disturb the session it copied")
	}
}

// The new session is on the grid and echoing the cursor beside it, so the list
// alone is not the only thing that says it happened.
func TestADuplicateLandsOnTheListWithItsCellEchoing(t *testing.T) {
	m := openOne(t)
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEscape, Alt: true})
	m = settle(next.(AppModel))
	m = pressA(m, "D", "enter")
	t.Cleanup(func() { m.ssh.stopAll() })

	if len(m.ssh.shown) != 2 {
		t.Fatalf("the new session should be on the grid, shown=%d", len(m.ssh.shown))
	}
	newest := m.ssh.sessions[len(m.ssh.sessions)-1]
	if got := m.ssh.cellTone(newest, len(m.ssh.shown)-1); got != toneEcho {
		t.Errorf("the new cell should echo the cursor, tone=%d", got)
	}
	if m.inPty() {
		t.Error("nothing should have handed the keyboard to a remote")
	}
}

// Enter ON A ROW is still the one thing that means "take me in" — the whole
// point of the distinction.
func TestEnterOnASessionRowStillEntersThePty(t *testing.T) {
	m := openOne(t)
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEscape, Alt: true})
	m = settle(next.(AppModel))
	if m.ssh.focus != panelSessions {
		t.Fatal("setup: expected the keyboard on [1]")
	}

	m = pressA(m, "enter")
	if m.ssh.focus != panelPty {
		t.Errorf("Enter on a session row should hand it the keyboard, focus=%d", m.ssh.focus)
	}
}

// And connecting from the hosts table still lands in the remote: reaching a
// remote is what the key was pressed for there.
func TestConnectingFromTheHostsTableStillLandsInThePty(t *testing.T) {
	aliveSSH(t)
	m := pressA(sshApp(t, sample()), "enter", "enter")
	t.Cleanup(func() { m.ssh.stopAll() })
	if m.ssh.focus != panelPty {
		t.Errorf("connecting should land in the session, focus=%d", m.ssh.focus)
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

// pressable reports whether a row's key names a keystroke somebody can make.
// The core keys spell themselves out; a letter is a letter; anything longer is
// a menu-only action and cannot be typed.
func pressable(key string) bool {
	switch key {
	case "enter", "tab":
		return true
	}
	return len(key) == 1
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
		"Close":              confirmClose,
		"Duplicate":          confirmDuplicate,
		"Close all sessions": confirmCloseAll,
	}
	// Open lands in the pty and Hide takes the cell off the grid (openOne put
	// it on); the rest ask first.
	opens := map[string]func(AppModel) bool{
		"Open": func(m AppModel) bool { return m.ssh.focus == panelPty },
		"Hide": func(m AppModel) bool { return len(m.ssh.shown) == 0 },
	}
	for _, a := range sshActions {
		if a.panel != panelSessions {
			continue
		}
		for _, how := range []string{"hotkey", "menu"} {
			// A menu-only row has no keystroke to press — the menu is its
			// only path, on purpose (§11.26).
			if how == "hotkey" && !pressable(a.key) {
				continue
			}
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
