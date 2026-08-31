package ui

import (
	"testing"
)

// Every list with a cursor is a ring: off the bottom is the top. It matters most
// on short lists, where the alternative is holding k and watching nothing move.
func TestCursorsWrapEverywhere(t *testing.T) {
	// [2] file list
	s, _ := atRoot(sftpFixture(t, 100, 26))
	s.sftp.focus = panelLeftFiles
	n := s.sftp.sides[sideLeft].rowCount()
	if n < 2 {
		t.Fatal("setup: the fixture needs more than one row")
	}
	s = pressA(s, "k")
	if got := s.sftp.sides[sideLeft].cursor; got != n-1 {
		t.Errorf("file list: k at the top went to %d, want %d", got, n-1)
	}
	s = pressA(s, "j")
	if got := s.sftp.sides[sideLeft].cursor; got != 0 {
		t.Errorf("file list: j at the bottom went to %d, want 0", got)
	}

	// [2] marks panel
	s.sftp.focus = panelLeftFiles
	s = pressA(s, "m", "j", "m")
	s.sftp.focus = panelLeftMarks
	marks := len(s.sftp.sides[sideLeft].marks)
	s = pressA(s, "k")
	if got := s.sftp.sides[sideLeft].markCur; got != marks-1 {
		t.Errorf("marks: k at the top went to %d, want %d", got, marks-1)
	}
	s = pressA(s, "j")
	if got := s.sftp.sides[sideLeft].markCur; got != 0 {
		t.Errorf("marks: j at the bottom went to %d, want 0", got)
	}
}

// The Space menu wraps too, and steps over its own headers and rules on the way
// round rather than landing on one.
func TestSpaceMenuWrapsPastItsHeaders(t *testing.T) {
	m := pressA(appWith(sample(), nil), " ")
	if !m.spaceMenu.isActive() {
		t.Fatal("setup: the menu should be open")
	}
	first := m.spaceMenu.cursor

	m = pressA(m, "k") // off the top
	last := m.spaceMenu.cursor
	if last == first {
		t.Fatal("k did not move")
	}
	if it := m.spaceMenu.items[last]; it.header || it.separator {
		t.Errorf("wrapping landed on a %q label", it.label)
	}
	// It is the LAST selectable row, not merely a different one.
	for i := last + 1; i < len(m.spaceMenu.items); i++ {
		if it := m.spaceMenu.items[i]; !it.header && !it.separator {
			t.Errorf("k should wrap to the last row, stopped at %d", last)
			break
		}
	}

	m = pressA(m, "j") // and back round
	if m.spaceMenu.cursor != first {
		t.Errorf("j off the bottom went to %d, want %d", m.spaceMenu.cursor, first)
	}
}

// A menu of nothing but labels must not spin forever looking for a row to land
// on. (There is no such menu today; the loop has to be safe anyway.)
func TestMenuStepTerminatesWithNothingSelectable(t *testing.T) {
	m := spaceMenu{items: []menuItem{
		{label: "a", header: true}, {separator: true}, {label: "b", header: true},
	}}
	done := make(chan struct{})
	go func() { m.step(1); m.step(-1); close(done) }()
	<-done
}

// The Transfers list wraps too — it is a cursor over jobs like any other.
func TestTransfersPopupWraps(t *testing.T) {
	var p transfersPopup
	p.anim.phase = animOpen

	p.update(keyMsg("k"), 3)
	if p.cursor != 2 {
		t.Errorf("k at the top went to %d, want 2", p.cursor)
	}
	p.update(keyMsg("j"), 3)
	if p.cursor != 0 {
		t.Errorf("j at the bottom went to %d, want 0", p.cursor)
	}
	// And an empty list has nowhere to go rather than dividing by zero.
	p.cursor = 0
	p.update(keyMsg("j"), 0)
	if p.cursor != 0 {
		t.Errorf("an empty list moved to %d", p.cursor)
	}
}

// A viewport has no cursor, so it does not wrap — a scroll that jumps back to
// the top when it reaches the bottom reads as a glitch.
func TestViewportsDoNotWrap(t *testing.T) {
	if got := moveScroll(0, 5, "k", 4); got != 0 {
		t.Errorf("scrolling up from the top gave %d, want 0", got)
	}
	if got := moveScroll(5, 5, "j", 4); got != 5 {
		t.Errorf("scrolling down from the end gave %d, want 5", got)
	}
	if got := moveScroll(0, 0, "j", 4); got != 0 {
		t.Errorf("a viewport with nothing below it moved to %d", got)
	}
}
