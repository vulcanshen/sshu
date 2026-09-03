package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// cellFrames counts the terminal frames actually drawn on the grid.
func cellFrames(grid string) int {
	return strings.Count(ansi.Strip(grid), "╭")
}

// Alt+Enter gives the focused cell the whole grid. Nothing else is drawn —
// which is the entire point, and why the state needs no marker of its own.
func TestAltEnterFillsTheGridWithOneCell(t *testing.T) {
	m := twoOnGrid(t)
	if got := cellFrames(m.ssh.gridView()); got != 2 {
		t.Fatalf("setup: expected two cells on the grid, got %d", got)
	}

	m = pressA(m, "alt+enter")
	if !m.ssh.zoomed {
		t.Fatal("alt+enter should have zoomed")
	}
	if got := cellFrames(m.ssh.gridView()); got != 1 {
		t.Errorf("a zoomed grid draws one cell, got %d", got)
	}

	// And the cell drawn is the one that had the keyboard.
	view := ansi.Strip(m.ssh.gridView())
	want := m.ssh.shownSessions()[m.ssh.focusPty].host.Host
	if !strings.Contains(view, want) {
		t.Errorf("the zoomed cell should be the focused one (%s):\n%s", want, view)
	}
}

// The remote has to be told, or it keeps painting to the old geometry.
func TestZoomingResizesTheRemote(t *testing.T) {
	m := twoOnGrid(t)
	s := m.ssh.shownSessions()[m.ssh.focusPty]
	before := s.appliedCols

	m = pressA(m, "alt+enter")
	if s.appliedCols <= before {
		t.Errorf("the zoomed cell should have been widened: %d -> %d", before, s.appliedCols)
	}
	gw, _ := m.ssh.gridArea()
	if s.appliedCols != gw-2 {
		t.Errorf("the zoomed cell should span the grid: %d, want %d", s.appliedCols, gw-2)
	}

	m = pressA(m, "alt+enter")
	if s.appliedCols != before {
		t.Errorf("unzooming should put it back: %d, want %d", s.appliedCols, before)
	}
}

// Alt+Esc peels one layer at a time. Pressing a way-out key once and landing
// two levels away is how it stops being predictable.
func TestAltEscLeavesTheZoomBeforeThePty(t *testing.T) {
	m := twoOnGrid(t)
	m = pressA(m, "alt+enter")
	if !m.ssh.zoomed {
		t.Fatal("setup: expected a zoom")
	}

	m = pressA(m, "alt+esc")
	if m.ssh.zoomed {
		t.Error("the first alt+esc should leave the zoom")
	}
	if m.ssh.focus != panelPty {
		t.Errorf("...but the keyboard stays in the cell, focus=%d", m.ssh.focus)
	}
	if got := cellFrames(m.ssh.gridView()); got != 2 {
		t.Errorf("the grid should be back, got %d cells", got)
	}

	m = pressA(m, "alt+esc")
	if m.ssh.focus != panelSessions {
		t.Errorf("the second alt+esc should hand the keyboard back, focus=%d", m.ssh.focus)
	}
}

// A zoom only exists while a cell has the keyboard, so any other way out of the
// pty clears it too — otherwise coming back would land in a zoom nobody asked
// for a second time.
func TestLeavingTheGridClearsTheZoom(t *testing.T) {
	m := twoOnGrid(t)
	m = pressA(m, "alt+enter")
	m.ssh.setFocus(panelSessions)
	if m.ssh.zoomed {
		t.Error("leaving the grid should clear the zoom")
	}
	if got := cellFrames(m.ssh.gridView()); got != 2 {
		t.Errorf("the grid should be whole again, got %d cells", got)
	}
}

// One cell already fills the grid. A key that visibly does nothing reads as
// broken, so it is not taken here — it goes to the remote like any other chord.
func TestAltEnterIsTheRemotesWithOneCell(t *testing.T) {
	m := openOne(t)
	if len(m.ssh.shown) != 1 {
		t.Fatalf("setup: expected one cell, got %d", len(m.ssh.shown))
	}
	if m.ssh.canZoom() {
		t.Error("a single cell has nothing to zoom")
	}

	m = pressA(m, "alt+enter")
	if m.ssh.zoomed {
		t.Error("alt+enter must not zoom a grid of one")
	}
	if m.ssh.focus != panelPty {
		t.Error("and it must not disturb the focus either")
	}
}

// It really is forwarded, not swallowed: ESC then CR arrive at the far end.
func TestAnUnusedAltEnterReachesTheRemote(t *testing.T) {
	fakeSSH(t, `printf '$ '; exec cat -v`)
	m := pressA(sshApp(t, sample()), "enter", "enter")
	t.Cleanup(func() { m.ssh.stopAll() })
	s := m.ssh.sessions[0]
	waitFor(t, "the stand-in to answer", func() bool { return s.pty.hasSpoken() })

	m = pressA(m, "alt+enter")
	waitFor(t, "the chord to arrive at the remote", func() bool {
		return strings.Contains(strings.Join(s.pty.render(80, 24), ""), "^[")
	})
}

// Steering keeps the zoom: you zoomed to read one terminal closely, and the
// next terminal is one you want to read just as closely.
func TestSteeringInsideAZoomKeepsIt(t *testing.T) {
	m := twoOnGrid(t)
	m = pressA(m, "alt+enter")
	was := m.ssh.focusPty

	m = pressA(m, "alt+left")
	if !m.ssh.zoomed {
		t.Error("moving between cells should not drop the zoom")
	}
	if m.ssh.focusPty == was {
		t.Fatal("setup: alt+left should have moved to the other cell")
	}
	if got := cellFrames(m.ssh.gridView()); got != 1 {
		t.Errorf("still one cell after steering, got %d", got)
	}
	// The cell that just took the keyboard is the one filling the screen.
	s := m.ssh.shownSessions()[m.ssh.focusPty]
	if !strings.Contains(ansi.Strip(m.ssh.gridView()), s.host.Host) {
		t.Error("the newly focused cell should be the one on screen")
	}
	gw, _ := m.ssh.gridArea()
	if s.appliedCols != gw-2 {
		t.Errorf("and it should have been resized to fill it: %d, want %d", s.appliedCols, gw-2)
	}
}

// ------------------------------------------------------------------ footer

// The footer names the layer alt+esc would take off if pressed right now, and
// offers the zoom only where it would do something.
func TestTheFooterTracksTheZoom(t *testing.T) {
	m := twoOnGrid(t)
	waitFor(t, "the remote to answer", func() bool { return m.inPty() })

	foot := m.footer()
	if !strings.Contains(foot, "leave pty") {
		t.Errorf("unzoomed, alt+esc leaves the pty: %q", foot)
	}
	if !strings.Contains(foot, "alt+enter") || !strings.Contains(foot, "zoom") {
		t.Errorf("with two cells the zoom should be disclosed: %q", foot)
	}

	m = pressA(m, "alt+enter")
	foot = m.footer()
	if !strings.Contains(foot, "leave zoom") {
		t.Errorf("zoomed, alt+esc leaves the zoom: %q", foot)
	}
	if !strings.Contains(foot, "unzoom") {
		t.Errorf("and the chord that got you here should say what it does now: %q", foot)
	}

	// One cell: nothing to zoom, nothing offered.
	one := openOne(t)
	if strings.Contains(one.footer(), "alt+enter") {
		t.Errorf("a grid of one must not advertise a zoom: %q", one.footer())
	}
}
