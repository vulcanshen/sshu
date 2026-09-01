package ui

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/vulcanshen/sshu/internal/remote"
	"github.com/vulcanshen/sshu/internal/store"
)

// errTestDial stands in for whatever a real dial failure would be.
var errTestDial = errors.New("connection refused")

// dialing puts a side in the connecting state without opening a socket.
func dialing(t *testing.T, w, h int) AppModel {
	t.Helper()
	m := sftpFixture(t, w, h)
	m.sftp.focus = panelLeftFiles
	m.sftp.sides[sideLeft].fs = nil
	m.sftp.sides[sideLeft].entries = nil
	m.sftp.startDial(sideLeft, store.Host{Name: "prod-web-01", Host: "10.0.3.14", Port: 22})
	return m
}

// While a dial is in flight the panel says so. It used to show the no-host
// prompt — "Press [S] to select a host" — for the whole fifteen seconds, which
// reads as "your keypress did nothing".
func TestDialingPanelSaysWhatItIsDoing(t *testing.T) {
	m := dialing(t, 100, 26)
	view := ansi.Strip(m.View())

	if !strings.Contains(view, "connecting to prod-web-01") {
		t.Errorf("the panel should say what it is waiting for:\n%s", view)
	}
	if strings.Contains(view, "Press [S]") {
		t.Error("that is the no-host prompt — this side is connecting, not idle")
	}
}

// And it MOVES. The complaint this answers is "is it stuck", and a frame that
// never changes is what stuck looks like.
func TestDialingSpinnerTurns(t *testing.T) {
	m := dialing(t, 100, 26)

	// The frame is the rune immediately before " connecting to". The line's first
	// character is the panel BORDER, and its first byte is the 0xE2 every braille
	// frame starts with — either one makes a turning spinner look frozen.
	frame := func(m AppModel) string {
		for _, line := range strings.Split(ansi.Strip(m.View()), "\n") {
			i := strings.Index(line, " connecting to")
			if i < 0 {
				continue
			}
			rs := []rune(line[:i])
			return string(rs[len(rs)-1])
		}
		return ""
	}

	seen := map[string]bool{}
	for i := 0; i < len(spinnerFrames); i++ {
		if f := frame(m); f != "" {
			seen[f] = true
		}
		next, cmd := m.Update(dialTickMsg{})
		m = next.(AppModel)
		if cmd == nil {
			t.Fatal("the tick should keep itself going while a dial is in flight")
		}
	}
	if len(seen) < 2 {
		t.Errorf("the spinner did not move: saw %d distinct frames", len(seen))
	}

	// Every frame is one cell, or the panel would breathe as it turns.
	for _, f := range spinnerFrames {
		if dispW(f) != 1 {
			t.Errorf("spinner frame %q is %d cells", f, dispW(f))
		}
	}
}

// The tick stops when nothing is connecting — an idle sshu repaints for nothing.
func TestDialTickStopsWhenNothingIsDialing(t *testing.T) {
	m := dialing(t, 100, 26)
	if m.sftp.dialTick() == nil {
		t.Fatal("the tick should run while a dial is in flight")
	}

	next, _ := m.Update(sftpConnectedMsg{
		sd: sideLeft, gen: m.sftp.sides[sideLeft].dialGen, fs: remote.Local(),
	})
	m = next.(AppModel)

	if m.sftp.sides[sideLeft].dialing != "" {
		t.Error("landing should clear the connecting state")
	}
	if m.sftp.dialTick() != nil {
		t.Error("the tick should stop once nothing is connecting")
	}
}

// Pick a host, change your mind, pick another: the first one can still land
// afterwards, and must not put you on the host you rejected.
func TestASupersededDialCannotLand(t *testing.T) {
	m := dialing(t, 100, 26)
	stale := m.sftp.sides[sideLeft].dialGen

	m.sftp.startDial(sideLeft, store.Host{Name: "db-replica", Host: "db.internal", Port: 22})
	if m.sftp.sides[sideLeft].dialGen == stale {
		t.Fatal("setup: the second dial should be a new generation")
	}

	next, _ := m.Update(sftpConnectedMsg{sd: sideLeft, gen: stale, fs: remote.Local()})
	m = next.(AppModel)

	if m.sftp.sides[sideLeft].fs != nil {
		t.Error("the answer to a question nobody is asking any more was installed")
	}
	if got := m.sftp.sides[sideLeft].dialing; got != "db-replica" {
		t.Errorf("still connecting to %q, want db-replica", got)
	}
}

// A failed dial leaves the side with no host rather than with a name and no
// connection, and says why.
func TestAFailedDialClearsTheHost(t *testing.T) {
	m := dialing(t, 100, 26)
	next, _ := m.Update(sftpConnectedMsg{
		sd:  sideLeft,
		gen: m.sftp.sides[sideLeft].dialGen,
		err: errTestDial,
	})
	m = settle(next.(AppModel))

	s := m.sftp.sides[sideLeft]
	if s.dialing != "" || s.host != "" || s.fs != nil {
		t.Errorf("a failed dial left state behind: dialing=%q host=%q", s.dialing, s.host)
	}
	if !m.toast.isActive() || m.toast.kind != toastError {
		t.Error("a failed dial should say so")
	}
}

// The elapsed counter only appears once the wait is worth mentioning — starting
// it at 0 on every connection makes a fast one look slow.
func TestDialElapsedAppearsOnlyAfterAWhile(t *testing.T) {
	m := dialing(t, 100, 26)
	if strings.Contains(ansi.Strip(m.View()), "0s") {
		t.Error("a dial that just started should not be counting yet")
	}

	m.sftp.sides[sideLeft].dialSince = time.Now().Add(-7 * time.Second)
	if !strings.Contains(ansi.Strip(m.View()), "7s") {
		t.Error("a long wait should say how long it has been")
	}
}

// The frame invariant, in the state that has its own body.
func TestDialingPreservesFrame(t *testing.T) {
	for _, sz := range [][2]int{{120, 40}, {100, 26}, {80, 20}, {72, 16}, {50, 12}, {34, 9}} {
		w, h := sz[0], sz[1]
		m := dialing(t, w, h)
		for i, l := range strings.Split(m.View(), "\n") {
			if lw := dispW(l); lw != w {
				t.Errorf("%dx%d line %d is %d cells, want %d: %q", w, h, i, lw, w, l)
				break
			}
		}
	}
}
