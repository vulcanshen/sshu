package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// twoOnGrid is two live sessions, both with a cell on the grid, keyboard on
// the second cell (the newest connect takes it).
func twoOnGrid(t *testing.T) AppModel {
	t.Helper()
	aliveSSH(t)
	m := sshApp(t, sample())
	for _, h := range sample()[:2] {
		if _, err := m.ssh.connect(h); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() { m.ssh.stopAll() })
	m.tab = tabSSH
	m.ssh.setFocus(panelPty)
	return m
}

// Tab on the list is the display toggle: off, on, and never a duplicate cell.
func TestTabTogglesASessionsCell(t *testing.T) {
	m := twoOnGrid(t)
	m.ssh.setFocus(panelSessions)
	m.ssh.curSess = 0
	id := m.ssh.sessions[0].id

	m = pressA(m, "tab")
	if m.ssh.isShown(id) || len(m.ssh.shown) != 1 {
		t.Fatalf("Tab should take the cell off the grid, shown=%v", m.ssh.shown)
	}
	m = pressA(m, "tab")
	if !m.ssh.isShown(id) || len(m.ssh.shown) != 2 {
		t.Fatalf("Tab should put it back — once, shown=%v", m.ssh.shown)
	}
	m = pressA(m, "tab", "tab")
	if len(m.ssh.shown) != 2 {
		t.Fatalf("toggling must never duplicate a cell, shown=%v", m.ssh.shown)
	}
}

// Alt+N moves the keyboard between cells; a chord with no cell behind it does
// nothing rather than something surprising.
func TestAltDigitsSwitchCells(t *testing.T) {
	m := twoOnGrid(t)
	first, second := m.ssh.shown[0], m.ssh.shown[1]

	m = pressA(m, "alt+1")
	if cur := m.ssh.currentSession(); cur == nil || cur.id != first {
		t.Fatalf("alt+1 should focus the first cell")
	}
	m = pressA(m, "alt+2")
	if cur := m.ssh.currentSession(); cur == nil || cur.id != second {
		t.Fatalf("alt+2 should focus the second cell")
	}
	m = pressA(m, "alt+9")
	if cur := m.ssh.currentSession(); cur == nil || cur.id != second {
		t.Fatal("alt+9 has no cell behind it and must change nothing")
	}
	if m.ssh.focus != panelPty {
		t.Fatal("the keyboard should still be in the grid")
	}
}

// The grid divides the width EXACTLY, whatever the layout — the frame
// invariant does not tolerate a stray column — and each cell's remote is told
// its own size.
func TestGridPreservesFrameAcrossLayouts(t *testing.T) {
	m := twoOnGrid(t)
	for _, mode := range []layoutMode{layoutHorizontal, layoutVertical, layoutCustom} {
		m.ssh.layout = mode
		m.ssh.applyGeometry()
		for _, sz := range [][2]int{{100, 30}, {81, 17}, {57, 11}} {
			next, _ := m.Update(tea.WindowSizeMsg{Width: sz[0], Height: sz[1]})
			m = next.(AppModel)
			got := m.View()
			lines := strings.Split(got, "\n")
			if len(lines) != sz[1] {
				t.Fatalf("layout %d %dx%d: %d lines, want %d", mode, sz[0], sz[1], len(lines), sz[1])
			}
			for i, l := range lines {
				if lw := lipgloss.Width(l); lw != sz[0] {
					t.Errorf("layout %d %dx%d line %d: width %d\n%q", mode, sz[0], sz[1], i, lw, l)
				}
			}
		}
	}

	// Horizontal at 100 wide: the two cells share the width, so each remote
	// was told roughly half — never the whole tab.
	m.ssh.layout = layoutHorizontal
	next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = next.(AppModel)
	for _, s := range m.ssh.shownSessions() {
		if s.appliedCols >= 90 {
			t.Errorf("a shared grid must not tell a cell it owns the width: %d", s.appliedCols)
		}
	}
}

// Vertical stacks; custom respects its stated columns and grows rows rather
// than losing a cell.
func TestGridDims(t *testing.T) {
	m := newSSHModel()
	m.layout = layoutVertical
	if c, r := m.gridDims(3); c != 1 || r != 3 {
		t.Errorf("vertical: got %d×%d", c, r)
	}
	m.layout = layoutHorizontal
	if c, r := m.gridDims(3); c != 3 || r != 1 {
		t.Errorf("horizontal: got %d×%d", c, r)
	}
	m.layout = layoutCustom
	m.gridC, m.gridR = 2, 1
	if c, r := m.gridDims(3); c != 2 || r != 2 {
		t.Errorf("custom must grow rows for the overflow cell, got %d×%d", c, r)
	}
}

// The layout strip: 2 focuses it, h/l walk the modes and apply immediately,
// Enter on custom asks for the shape and the answer lands.
func TestLayoutStripDrivesTheGrid(t *testing.T) {
	m := twoOnGrid(t)
	m.ssh.setFocus(panelSessions)
	m = pressA(m, "2")
	if m.ssh.focus != panelLayout {
		t.Fatalf("2 should focus the layout strip, got %d", m.ssh.focus)
	}
	m = pressA(m, "l")
	if m.ssh.layout != layoutVertical {
		t.Fatalf("l should move to vertical, got %d", m.ssh.layout)
	}
	m = pressA(m, "l")
	if m.ssh.layout != layoutCustom {
		t.Fatalf("l again should reach custom, got %d", m.ssh.layout)
	}
	m = pressA(m, "enter")
	if !m.input.isActive() {
		t.Fatal("Enter on custom should ask for the shape")
	}
	// Replace the prefilled value wholesale.
	for range 8 {
		m = pressA(m, "backspace")
	}
	m = typeText(m, "3x2")
	m = pressA(m, "enter")
	if m.ssh.gridC != 3 || m.ssh.gridR != 2 {
		t.Fatalf("the answer should land, got %d×%d", m.ssh.gridC, m.ssh.gridR)
	}

	// Nonsense is refused and does not change the shape.
	m.ssh.setFocus(panelLayout)
	m = pressA(m, "enter")
	for range 8 {
		m = pressA(m, "backspace")
	}
	m = typeText(m, "0x40")
	m = pressA(m, "enter")
	if m.ssh.gridC != 3 || m.ssh.gridR != 2 {
		t.Fatalf("a refused shape must change nothing, got %d×%d", m.ssh.gridC, m.ssh.gridR)
	}
}

// When the FOCUSED cell's session dies, the keyboard falls back to the list —
// it must never silently land in another remote.
func TestDyingFocusedCellReturnsTheKeyboard(t *testing.T) {
	m := twoOnGrid(t)
	m = pressA(m, "alt+1")
	victim := m.ssh.currentSession()

	victim.pty.stop()
	waitFor(t, "the victim to exit", func() bool { return victim.pty.exited() })
	m.ssh.reap()

	if m.ssh.focus == panelPty {
		t.Fatal("the keyboard must not stay in the grid when its cell died")
	}
	if len(m.ssh.shown) != 1 {
		t.Fatalf("the dead cell should have left the grid, shown=%v", m.ssh.shown)
	}
}

// A NON-focused cell dying reflows the grid but leaves the keyboard where it
// is — in the cell the user was typing at.
func TestDyingOtherCellKeepsTheKeyboard(t *testing.T) {
	m := twoOnGrid(t)
	m = pressA(m, "alt+2")
	keeper := m.ssh.currentSession()
	victim := m.ssh.byID(m.ssh.shown[0])

	victim.pty.stop()
	waitFor(t, "the victim to exit", func() bool { return victim.pty.exited() })
	m.ssh.reap()

	if m.ssh.focus != panelPty {
		t.Fatal("the keyboard should stay in the grid")
	}
	if cur := m.ssh.currentSession(); cur == nil || cur.id != keeper.id {
		t.Fatal("the keyboard should still be on the same session")
	}
}
