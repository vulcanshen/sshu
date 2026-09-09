package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/vulcanshen/sshu/internal/store"
)

// cellFrames counts the terminal frames actually drawn on the grid.
func cellFrames(grid string) int {
	return strings.Count(ansi.Strip(grid), "\u256d")
}

// ------------------------------------------------------------------- the cycle

// Alt+Z has three stops and returns to where it started: the focused cell takes
// the grid, then the chrome comes off too, then the grid is back.
func TestAltZWalksThreeStages(t *testing.T) {
	m := twoOnGrid(t)
	if got := cellFrames(m.ssh.gridView()); got != 2 {
		t.Fatalf("setup: expected two cells on the grid, got %d", got)
	}

	m = pressA(m, "alt+z")
	if m.ssh.zoomAt() != zoomGrid {
		t.Fatalf("the first press fills the grid, stage=%d", m.ssh.zoomAt())
	}
	if got := cellFrames(m.ssh.gridView()); got != 1 {
		t.Errorf("a grid zoom draws one framed cell, got %d", got)
	}

	m = pressA(m, "alt+z")
	if m.ssh.zoomAt() != zoomFull {
		t.Fatalf("the second press goes full screen, stage=%d", m.ssh.zoomAt())
	}
	if got := cellFrames(m.ssh.gridView()); got != 0 {
		t.Errorf("full screen draws no frame at all, got %d", got)
	}

	m = pressA(m, "alt+z")
	if m.ssh.zoomAt() != zoomOff {
		t.Fatalf("the third press closes the cycle, stage=%d", m.ssh.zoomAt())
	}
	if got := cellFrames(m.ssh.gridView()); got != 2 {
		t.Errorf("and the grid is back, got %d cells", got)
	}
}

// One cell has no grid stage to visit — it would look exactly like no zoom at
// all, and a cycle with an invisible stop reads as a key that did nothing the
// first time it was pressed.
func TestOneCellSkipsTheGridStage(t *testing.T) {
	m := openOne(t)
	if len(m.ssh.shown) != 1 {
		t.Fatalf("setup: expected one cell, got %d", len(m.ssh.shown))
	}
	m = pressA(m, "alt+z")
	if m.ssh.zoomAt() != zoomFull {
		t.Fatalf("one cell goes straight to full screen, stage=%d", m.ssh.zoomAt())
	}
	m = pressA(m, "alt+z")
	if m.ssh.zoomAt() != zoomOff {
		t.Errorf("and straight back, stage=%d", m.ssh.zoomAt())
	}
}

// The chord stopped being the remote's on a grid of one. It used to travel
// there, on the grounds that a key which visibly does nothing reads as broken;
// full screen always does something, and forwarding it now would zoom an INNER
// sshu when this one was asked to.
func TestAnUnlockedCellNeverSeesAltZ(t *testing.T) {
	sink := sinkSSH(t)
	m := pressA(sshApp(t, sample()), "enter", "enter")
	t.Cleanup(func() { m.ssh.stopAll() })
	waitFor(t, "the stand-in to answer", func() bool {
		return m.ssh.sessions[0].pty.hasSpoken()
	})

	// A plain key AFTER it, so absence is proved by something arriving rather
	// than by waiting and hoping.
	m = pressA(m, "alt+z", "x")
	waitSink(t, sink, "x")
	if strings.Contains(sinkBytes(t, sink), "\x1bz") {
		t.Error("alt+z reached the remote; it belongs to this layer now")
	}
	if m.ssh.zoomAt() != zoomFull {
		t.Errorf("...and it should have zoomed instead, stage=%d", m.ssh.zoomAt())
	}
}

// ------------------------------------------------------------- the whole display

// Full screen means the whole display: the tab strip, the rule and the footer
// are not shrunk, they are not drawn. Those are the rows a nested sshu was
// paying for at every layer.
func TestFullScreenReplacesTheFrame(t *testing.T) {
	m := twoOnGrid(t)
	waitFor(t, "the remote to answer", func() bool { return m.inPty() })
	if !strings.Contains(ansi.Strip(m.View()), "alt+esc") {
		t.Fatal("setup: the footer should be on screen")
	}

	m = pressA(m, "alt+z", "alt+z")
	v := ansi.Strip(m.View())
	if strings.Contains(v, "alt+esc") {
		t.Errorf("the footer must not be drawn in full screen:\n%s", v)
	}
	if n := len(strings.Split(m.View(), "\n")); n != 30 {
		t.Errorf("the display is still 30 rows, got %d", n)
	}
}

// The remote has to be told, or it keeps painting to the old geometry — and at
// the last stage the geometry is the display, chrome included.
func TestEachStageResizesTheRemote(t *testing.T) {
	m := twoOnGrid(t)
	s := m.ssh.shownSessions()[m.ssh.focusPty]
	before := s.appliedCols

	m = pressA(m, "alt+z")
	gw, gh := m.ssh.gridArea()
	if s.appliedCols != gw-2 || s.appliedRows != gh-2 {
		t.Errorf("grid zoom: %dx%d, want %dx%d",
			s.appliedCols, s.appliedRows, gw-2, gh-2)
	}

	m = pressA(m, "alt+z")
	gw, gh = m.ssh.gridArea()
	if want := m.ssh.h + chromeRows; gh != want {
		t.Errorf("full screen takes the chrome rows back: %d, want %d", gh, want)
	}
	if s.appliedCols != gw || s.appliedRows != gh {
		t.Errorf("full screen: %dx%d, want %dx%d — the border is gone too",
			s.appliedCols, s.appliedRows, gw, gh)
	}

	m = pressA(m, "alt+z")
	if s.appliedCols != before {
		t.Errorf("closing the cycle puts it back: %d, want %d", s.appliedCols, before)
	}
}

// The overlays cover cells. They must never change how many there are.
func TestFullScreenPreservesTheFrame(t *testing.T) {
	aliveSSH(t)
	for _, sz := range [][2]int{{100, 30}, {80, 24}, {60, 12}, {40, 9}} {
		w, h := sz[0], sz[1]
		m := New(sample(), nil, store.DefaultConfig())
		next, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: h})
		m = pressA(settle(next.(AppModel)), "enter", "enter")
		waitFor(t, "the stand-in to answer", func() bool {
			return m.ssh.sessions[0].pty.hasSpoken()
		})
		m = pressA(m, "alt+z")
		if m.ssh.zoomAt() != zoomFull {
			t.Fatalf("%dx%d: expected full screen, stage=%d", w, h, m.ssh.zoomAt())
		}
		// Both overlays: the badge, and then the selection legend over it.
		for _, sel := range []bool{false, true} {
			if sel {
				m = pressA(m, "alt+v")
				if !m.ssh.copy.on {
					t.Fatalf("%dx%d: setup: alt+v should have opened selection mode", w, h)
				}
			}
			lines := strings.Split(m.View(), "\n")
			if len(lines) != h {
				t.Errorf("%dx%d sel=%v: %d lines, want %d", w, h, sel, len(lines), h)
				continue
			}
			for i, l := range lines {
				if lw := dispW(l); lw != w {
					t.Errorf("%dx%d sel=%v line %d: width %d, want %d\n%q",
						w, h, sel, i, lw, w, l)
				}
			}
		}
		m.ssh.stopAll()
	}
}

// ------------------------------------------------------------------ the way back

// Alt+Esc walks the cycle backwards, one stage per press, and hands the
// keyboard back only once there are no stages left.
func TestAltEscWalksBackOneStageAtATime(t *testing.T) {
	m := twoOnGrid(t)
	m = pressA(m, "alt+z", "alt+z")
	if m.ssh.zoomAt() != zoomFull {
		t.Fatalf("setup: expected full screen, stage=%d", m.ssh.zoomAt())
	}

	m = pressA(m, "alt+esc")
	if m.ssh.zoomAt() != zoomGrid {
		t.Errorf("the first alt+esc drops to the grid zoom, stage=%d", m.ssh.zoomAt())
	}
	if m.ssh.focus != panelPty {
		t.Errorf("...and the keyboard stays in the cell, focus=%d", m.ssh.focus)
	}
	if got := cellFrames(m.ssh.gridView()); got != 1 {
		t.Errorf("the frame is back, on one cell: got %d", got)
	}

	m = pressA(m, "alt+esc")
	if m.ssh.zoomAt() != zoomOff {
		t.Errorf("the second leaves the zoom, stage=%d", m.ssh.zoomAt())
	}
	if m.ssh.focus != panelPty {
		t.Errorf("...still in the cell, focus=%d", m.ssh.focus)
	}

	m = pressA(m, "alt+esc")
	if m.ssh.focus != panelSessions {
		t.Errorf("only the third hands the keyboard back, focus=%d", m.ssh.focus)
	}
}

// It skips exactly the stage Alt+Z skipped, or the way back would visit a stop
// the way in never had.
func TestAltEscSkipsTheStageAltZSkipped(t *testing.T) {
	m := openOne(t)
	m = pressA(m, "alt+z")
	m = pressA(m, "alt+esc")
	if m.ssh.zoomAt() != zoomOff {
		t.Errorf("one cell has no grid stage to land on, stage=%d", m.ssh.zoomAt())
	}
	if m.ssh.focus != panelPty {
		t.Errorf("but the keyboard is still in the cell, focus=%d", m.ssh.focus)
	}
}

// A zoom only exists while a cell has the keyboard, so any other way out of the
// pty clears it too — otherwise coming back would land in a zoom nobody asked
// for a second time.
func TestLeavingTheGridClearsTheZoom(t *testing.T) {
	m := twoOnGrid(t)
	m = pressA(m, "alt+z", "alt+z")
	m.ssh.setFocus(panelSessions)
	if m.ssh.zoomAt() != zoomOff {
		t.Errorf("leaving the grid should clear the zoom, stage=%d", m.ssh.zoomAt())
	}
	if got := cellFrames(m.ssh.gridView()); got != 2 {
		t.Errorf("the grid should be whole again, got %d cells", got)
	}
}

// Steering keeps the stage: you zoomed to read one terminal closely, and the
// next terminal is one you want to read just as closely.
func TestSteeringInsideAZoomKeepsIt(t *testing.T) {
	m := twoOnGrid(t)
	m = pressA(m, "alt+z")
	was := m.ssh.focusPty

	m = pressA(m, "alt+left")
	if m.ssh.zoomAt() != zoomGrid {
		t.Errorf("moving between cells should not drop the zoom, stage=%d", m.ssh.zoomAt())
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
		t.Errorf("and it should have been resized to fill it: %d, want %d",
			s.appliedCols, gw-2)
	}
}

// A stage must never outlive the grid it was asked for. When the OTHER cell's
// session ends the keyboard stays where it is (multipty_test pins that), and a
// grid zoom with nothing left to give up is a zoom nobody can see — one that
// would still charge a press of the way-out key to leave.
func TestAGridZoomDoesNotOutliveItsGrid(t *testing.T) {
	m := twoOnGrid(t)
	waitFor(t, "the remote to answer", func() bool { return m.inPty() })
	m = pressA(m, "alt+z")
	if m.ssh.zoomAt() != zoomGrid {
		t.Fatalf("setup: expected the grid stage, stage=%d", m.ssh.zoomAt())
	}

	victim := m.ssh.byID(m.ssh.shown[0]) // the one WITHOUT the keyboard
	victim.pty.stop()
	waitFor(t, "the victim to exit", func() bool { return victim.pty.exited() })
	m.ssh.reap()
	if len(m.ssh.shown) != 1 || m.ssh.focus != panelPty {
		t.Fatalf("setup: expected one cell still holding the keyboard, shown=%v focus=%d",
			m.ssh.shown, m.ssh.focus)
	}

	if m.ssh.zoomAt() != zoomOff {
		t.Errorf("a grid zoom with one cell left is invisible, stage=%d", m.ssh.zoomAt())
	}
	if !strings.Contains(m.footer(), "leave pty") {
		t.Errorf("...so the row must not offer to leave it: %q", m.footer())
	}
	// One press of the way out, not two — the second would be spent on a
	// stage that was not on screen.
	m = pressA(m, "alt+esc")
	if m.ssh.focus != panelSessions {
		t.Errorf("alt+esc should hand the keyboard back, focus=%d", m.ssh.focus)
	}
}

// ------------------------------------------------------------------- the badge

// Full screen has no chrome left to say what it is, so it says so in the
// corner, OVER the output — never in a row of its own, because a reserved row
// would cost one line per layer and put the compression straight back.
func TestTheBadgeNamesItselfInFullScreen(t *testing.T) {
	m := openOne(t)
	m = pressA(m, "alt+z")
	if v := ansi.Strip(m.View()); !strings.Contains(v, "sshu") {
		t.Errorf("full screen should carry a badge:\n%s", v)
	}
	if n := len(strings.Split(m.View(), "\n")); n != 30 {
		t.Errorf("and it must not cost a row: %d rows, want 30", n)
	}
	// Not before, either — the badge belongs to the state, not to the tab.
	m = pressA(m, "alt+z")
	if v := ansi.Strip(m.View()); strings.Contains(v, "sshu \u00d7") {
		t.Error("the badge should be gone once the chrome is back")
	}
}

// The count comes from the report, which travels inward-out (§11.44): a layer
// knows what is INSIDE it and nothing about what is outside. So the outermost
// is the only one that can count the stack — and it is also the one whose paint
// lands on top, which is why the number is right wherever it is read.
func TestTheBadgeCountsTheStack(t *testing.T) {
	m, _ := reportingSink(t, `\033]7180;1;inner-host:0\033\\`)
	m = pressA(m, "alt+z")
	if v := ansi.Strip(m.View()); !strings.Contains(v, "sshu \u00d72") {
		t.Errorf("two layers should be counted:\n%s", v)
	}
}

// A locked cell has nothing else left to say so: the title that carried the
// glyph and the footer that named the release are both off screen.
func TestTheBadgeCarriesTheLock(t *testing.T) {
	m := openOne(t)
	m = pressA(m, "alt+z")
	if strings.Contains(ansi.Strip(m.View()), glyphPtyLock) {
		t.Fatal("setup: an unlocked cell should not show the lock")
	}
	m.ssh.currentSession().locked = true
	if !strings.Contains(ansi.Strip(m.View()), glyphPtyLock) {
		t.Error("a locked full-screen cell must still say it is locked")
	}
}

// ------------------------------------------------------------- selection mode

// Selection mode replaces the meaning of every key under the user's fingers,
// and in full screen the footer that would have said so is not on screen. The
// legend moves onto the frozen page — which costs nothing, because it is
// frozen (§11.33).
func TestFullScreenKeepsSelectionModeDisclosed(t *testing.T) {
	m := openOne(t)
	m = pressA(m, "alt+z", "alt+v")
	if !m.ssh.copy.on {
		t.Fatal("setup: alt+v should have opened selection mode")
	}
	v := ansi.Strip(m.View())
	for _, want := range []string{"hjkl", "half page", "alt+v"} {
		if !strings.Contains(v, want) {
			t.Errorf("the selection keys should be disclosed (%q):\n%s", want, v)
		}
	}
	if n := len(strings.Split(m.View(), "\n")); n != 30 {
		t.Errorf("and still without costing a row: %d rows", n)
	}
}

// ------------------------------------------------------------------ the footer

// The row names the layer alt+esc would take off, and what the NEXT alt+z does
// — three stops need three labels, or the row is describing the key instead of
// the state.
func TestTheFooterTracksTheZoom(t *testing.T) {
	m := twoOnGrid(t)
	waitFor(t, "the remote to answer", func() bool { return m.inPty() })

	foot := m.footer()
	if !strings.Contains(foot, "leave pty") {
		t.Errorf("unzoomed, alt+esc leaves the pty: %q", foot)
	}
	if !strings.Contains(foot, "alt+z") || !strings.Contains(foot, "zoom") {
		t.Errorf("with two cells the grid stage is next: %q", foot)
	}

	m = pressA(m, "alt+z")
	foot = m.footer()
	if !strings.Contains(foot, "leave zoom") {
		t.Errorf("zoomed, alt+esc leaves the zoom: %q", foot)
	}
	if !strings.Contains(foot, "full screen") {
		t.Errorf("and the next press is the full one: %q", foot)
	}

	// One cell: the grid stage is skipped, so the row must not promise a zoom
	// that will not happen.
	one := openOne(t)
	waitFor(t, "the remote to answer", func() bool { return one.inPty() })
	if !strings.Contains(one.footer(), "full screen") {
		t.Errorf("a grid of one goes straight to full screen: %q", one.footer())
	}
}
