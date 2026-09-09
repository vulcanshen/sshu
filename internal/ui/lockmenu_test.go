package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// sinkSSH stands in for a remote that RECORDS its stdin: the greeting marks it
// connected, and everything sshu writes into the pty lands in a file the test
// can read back. That is the only honest way to test forwarding — asserting on
// model state would prove a flag flipped, not that bytes left for the remote.
func sinkSSH(t *testing.T) string {
	t.Helper()
	sink := filepath.Join(t.TempDir(), "stdin")
	// raw, like the real ssh sets its pty: canonical mode would hold ESC ESC in
	// the line buffer forever and rewrite CR to NL (ICRNL), so nothing this
	// test wants to observe would ever reach the file as itself.
	fakeSSH(t, "stty raw -echo 2>/dev/null; printf '$ '; exec cat > "+sink)
	return sink
}

// lockApp is one connected session with the keyboard in the pty.
func lockApp(t *testing.T) (AppModel, string) {
	t.Helper()
	sink := sinkSSH(t)
	m := pressA(sshApp(t, sample()), "enter", "enter")
	t.Cleanup(func() { m.ssh.stopAll() })
	waitFor(t, "the stand-in to answer", func() bool {
		return m.ssh.sessions[0].pty.hasSpoken()
	})
	if !m.inPty() {
		t.Fatal("setup: the keyboard should be in the pty")
	}
	return m, sink
}

func sinkBytes(t *testing.T, sink string) string {
	t.Helper()
	raw, err := os.ReadFile(sink)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	return string(raw)
}

// waitSink polls until the sink holds want — the write crosses a real pty, so
// it is not synchronous with the keystroke.
func waitSink(t *testing.T, sink, want string) {
	t.Helper()
	waitFor(t, "bytes to reach the remote", func() bool {
		return strings.Contains(sinkBytes(t, sink), want)
	})
}

// Alt+Enter does BOTH halves, in order: forwards itself (so a nested sshu one
// layer in opens its own menu) and opens this layer's menu. Making either half
// conditional breaks some depth — that was the whole §11.43 argument.
func TestAltEnterOpensTheLockMenuAndForwardsItself(t *testing.T) {
	m, sink := lockApp(t)
	m = pressA(m, "alt+enter")
	if !m.lockMenu.isActive() {
		t.Fatal("alt+enter should open the lock menu")
	}
	// The forwarded chord is ESC CR — exactly what a terminal sends for
	// Alt+Enter, so the inner sshu decodes the same key the user pressed.
	waitSink(t, sink, "\x1b\r")

	// And the menu names the two states, cursor on the one that can run.
	v := ansi.Strip(m.lockMenu.view())
	// Bracketed, because the menu disclosed the letters — [L]ock / [R]elease.
	if !strings.Contains(v, "[L]ock PTY") || !strings.Contains(v, "[R]elease PTY") {
		t.Errorf("both rows belong on the menu:\n%s", v)
	}
	if m.lockMenu.cursor != 0 {
		t.Errorf("unlocked: the cursor should start on Lock, got %d", m.lockMenu.cursor)
	}
}

// Locked, the cell is a transparent pipe: every chord this layer would have
// taken goes to the remote instead, byte-for-byte.
func TestALockedCellPassesEveryChordThrough(t *testing.T) {
	m, sink := lockApp(t)
	m = pressA(m, "alt+enter", "enter") // menu → Lock
	if !m.ssh.sessions[0].locked {
		t.Fatal("Enter on Lock should lock the session")
	}

	// Alt+Esc: normally "leave the pty". Locked, it must arrive as ESC ESC.
	m = pressA(m, "alt+esc")
	if !m.inPty() {
		t.Error("alt+esc must not take the keyboard back from a locked cell")
	}
	waitSink(t, sink, "\x1b\x1b")

	// Alt+Z: normally zoom. Locked, it travels.
	m = pressA(m, "alt+z")
	if m.ssh.zoomed {
		t.Error("alt+z must not zoom a locked cell")
	}
	waitSink(t, sink, "\x1bz")

	// Alt+v: normally selection mode. Locked, it travels.
	m = pressA(m, "alt+v")
	if m.ssh.copy.on {
		t.Error("alt+v must not open selection mode on a locked cell")
	}
	waitSink(t, sink, "\x1bv")
}

// The one exception: Alt+Enter still opens the menu — it is the key that
// releases the lock, so it is the one key the lock cannot swallow.
func TestAltEnterStillAnswersOnALockedCell(t *testing.T) {
	m, _ := lockApp(t)
	m = pressA(m, "alt+enter", "enter") // lock
	m = pressA(m, "alt+enter")
	if !m.lockMenu.isActive() {
		t.Fatal("alt+enter is the lock's only exception and must still answer")
	}
	// Cursor on Release now — each layer's menu displays its own state.
	if m.lockMenu.cursor != 1 {
		t.Errorf("locked: the cursor should start on Release, got %d", m.lockMenu.cursor)
	}
	m = pressA(m, "enter")
	if m.ssh.sessions[0].locked {
		t.Error("Enter on Release should unlock")
	}
	if m.lockMenu.isActive() {
		t.Error("committing should close the menu")
	}
}

// Alt+Enter on its own open menu closes it and does NOT forward again — a
// second broadcast would stack a second menu on every inner layer.
func TestAltEnterOnItsOwnMenuClosesWithoutForwarding(t *testing.T) {
	m, sink := lockApp(t)
	m = pressA(m, "alt+enter")
	waitSink(t, sink, "\x1b\r")
	first := sinkBytes(t, sink)
	m = pressA(m, "alt+enter")
	if m.lockMenu.isActive() {
		t.Error("the entry chord should close its own float")
	}
	if got := sinkBytes(t, sink); got != first {
		t.Errorf("closing must not forward a second chord: %q -> %q", first, got)
	}
}

// Esc cancels the menu through the one shared resolver (§4.3), changing nothing.
func TestEscClosesTheLockMenuAndChangesNothing(t *testing.T) {
	m, _ := lockApp(t)
	m = pressA(m, "alt+enter", "esc")
	if m.lockMenu.isActive() {
		t.Error("Esc should close the lock menu")
	}
	if m.ssh.sessions[0].locked {
		t.Error("cancelling must not lock anything")
	}
}

// The disabled row answers rather than ignoring (§A.1): committing it says why
// nothing happened, and the state does not flip.
func TestTheDisabledRowAnswersInsteadOfFlipping(t *testing.T) {
	m, _ := lockApp(t)
	m = pressA(m, "alt+enter", "k", "enter") // wrap to Release, disabled while unlocked
	if m.ssh.sessions[0].locked {
		t.Error("a disabled Release must not lock")
	}
	if v := ansi.Strip(m.View()); !strings.Contains(v, "Not locked") {
		t.Errorf("the refusal should be said:\n%s", v)
	}
}

// The lock is per session: locking the cell that leads to one host says
// nothing about the cell that leads to another.
func TestTheLockIsPerSession(t *testing.T) {
	m, _ := lockApp(t)
	// BOTH sessions exist before anything is locked. Locking first and
	// connecting second would pass even if the lock flipped every session
	// alive at the time — there was only one (the fixth fixture-never-created-
	// the-situation catch of this branch of work).
	if _, err := m.ssh.connect(sample()[1]); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { m.ssh.stopAll() })
	waitFor(t, "the second stand-in to answer", func() bool {
		return m.ssh.sessions[1].pty.hasSpoken()
	})

	// The keyboard followed the connect onto session 1; lock THAT one.
	m = pressA(m, "alt+enter", "enter")
	if !m.ssh.sessions[1].locked {
		t.Fatal("Enter on Lock should lock the focused session")
	}
	if m.ssh.sessions[0].locked {
		t.Error("locking one cell must not lock its neighbour")
	}
}

// A locked footer says the one true thing. Every other entry — leave pty,
// zoom, select — would be a lie, and the pty row's whole rule is honesty.
func TestALockedFooterOffersOnlyTheRelease(t *testing.T) {
	m, _ := lockApp(t)
	m = pressA(m, "alt+enter", "enter")
	foot := ansi.Strip(m.footer())
	if !strings.Contains(foot, "alt+enter") || !strings.Contains(foot, "release") {
		t.Errorf("the footer must offer the way back: %q", foot)
	}
	for _, gone := range []string{"leave pty", "zoom", "select"} {
		if strings.Contains(foot, gone) {
			t.Errorf("%q is a lie on a locked cell: %q", gone, foot)
		}
	}
}

// And the unlocked footer discloses the lock chord — the pty swallows `?`, so
// the footer is the only live disclosure this key gets (§11.19).
func TestTheUnlockedFooterDisclosesTheLockChord(t *testing.T) {
	m, _ := lockApp(t)
	if foot := ansi.Strip(m.footer()); !strings.Contains(foot, "lock") {
		t.Errorf("the lock chord must be advertised: %q", foot)
	}
}

// The locked cell's title carries the lock glyph: a cell whose keys all pass
// through is the most important thing to know about it before typing.
func TestALockedCellWearsTheLockGlyph(t *testing.T) {
	m, _ := lockApp(t)
	m = pressA(m, "alt+enter", "enter")
	s := m.ssh.sessions[0]
	if title := m.ssh.cellTitle(s, 0, 40); !strings.Contains(title, glyphPtyLock) {
		t.Errorf("a locked cell must say so in its title: %q", title)
	}
	m = pressA(m, "alt+enter", "enter") // release
	if title := m.ssh.cellTitle(s, 0, 40); strings.Contains(title, glyphPtyLock) {
		t.Errorf("a released cell must drop the mark: %q", title)
	}
}

// Alt+Enter no longer zooms — the swap that freed the most reliable chord on
// the keyboard for the key that must never fail. (Alt+Z's zoom is pinned by
// sshzoom_test; this pins the vacancy.)
func TestAltEnterNoLongerZooms(t *testing.T) {
	m, _ := lockApp(t)
	if _, err := m.ssh.connect(sample()[1]); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { m.ssh.stopAll() })
	if !m.ssh.canZoom() {
		t.Fatal("setup: two cells should make zoom meaningful")
	}
	// alpha takes Alt+Enter before the zoom branch could — and off the pty,
	// canZoom is false anyway. (A mutation re-adding KeyEnter to the zoom
	// branch is therefore UNOBSERVABLE, not untested: the branch it revives
	// is unreachable from every focus. Checked, not assumed.)
	m = pressA(m, "alt+enter")
	if m.ssh.zoomed {
		t.Error("alt+enter must not zoom from the pty")
	}
	m = pressA(m, "esc") // the menu away; the keyboard is back in the pty
	m = pressA(m, "alt+z")
	if !m.ssh.zoomed {
		t.Error("alt+z is where zoom lives now")
	}
}
