package ui

import (
	"time"

	"github.com/vulcanshen/sshu/internal/remote"
)

// Tab [2] is two independent sides, each a filesystem browser over its own
// marks. It has nothing to do with tab [3]: an sftp side is its own connection,
// and however many ssh sessions are open changes nothing here.
//
// Panels are [4] [5] [6] [7] — left files, left marks, right files, right marks
// — and each is its own focus, because doing anything to a mark needs a cursor
// on it.
type sftpPanel int

const (
	panelLeftFiles sftpPanel = iota
	panelLeftMarks
	panelRightFiles
	panelRightMarks
)

// side is which half a panel belongs to. The two halves are symmetric: whatever
// one can do, the other can do to it.
type side int

const (
	sideLeft side = iota
	sideRight
)

func (p sftpPanel) side() side {
	if p == panelLeftFiles || p == panelLeftMarks {
		return sideLeft
	}
	return sideRight
}

func (p sftpPanel) isMarks() bool { return p == panelLeftMarks || p == panelRightMarks }

// onSide is the panel in the SAME ROW on side sd. The two halves are mirror
// images, so crossing to "the other side's equivalent panel" is the only landing
// that changes which host you are looking at without also changing what you are
// looking at.
func (p sftpPanel) onSide(sd side) sftpPanel {
	switch {
	case p.isMarks() && sd == sideLeft:
		return panelLeftMarks
	case p.isMarks():
		return panelRightMarks
	case sd == sideLeft:
		return panelLeftFiles
	default:
		return panelRightFiles
	}
}

// sftpSideModel is one half: a filesystem, where it is looking, and what has
// been marked there.
type sftpSideModel struct {
	fs   remote.FS
	host string // panel title; "" until a host is picked
	cwd  string
	home string // this filesystem's home, for folding the cwd back to ~
	err  string

	entries []remote.Entry
	cursor  int
	top     int

	// `/` searches the whole subtree in place (see sftpsearch.go). found is
	// everything the walk has produced, named relative to cwd; matches indexes
	// into it. entries stays what it always was — the current directory — so
	// leaving the search restores the listing without re-listing anything.
	filtering bool
	query     string
	found     []remote.Entry
	matches   []int // indices into found
	scan      *searchScan
	scanning  bool
	capped    bool

	// marks are absolute paths on THIS side. They are cleared when the side
	// switches host, because a path is only meaningful against the filesystem it
	// came from.
	marks     []string
	markCur   int
	markTop   int
	markedSet map[string]bool

	// dialing is the host being connected to, "" when not. It is its own field
	// rather than a message in err, because the panel has to be able to tell
	// "there is no host" from "there is one, on its way" — those look nothing
	// alike to a user and used to look identical (see sftpdial.go).
	dialing   string
	dialSince time.Time
	dialGen   int

	// The directory's own timestamp, and whether a probe for it is in flight.
	// SFTP cannot push a change, so this is how a listing stays current without
	// re-reading it every couple of seconds (sftpwatch.go).
	watchMTime time.Time
	probing    bool
}

type sftpModel struct {
	sides [2]sftpSideModel
	focus sftpPanel
	w, h  int

	// onScreen gates the refresh loop: a tab nobody is looking at should not be
	// working the connection. watchGen retires a superseded loop.
	onScreen bool
	watchGen int

	// spinAt is the connecting spinner's frame. It counts ticks rather than
	// reading the clock, so the animation does not depend on when a frame is
	// drawn.
	spinAt int
}

func newSFTPModel() sftpModel {
	return sftpModel{
		sides: [2]sftpSideModel{
			{markedSet: map[string]bool{}},
			{markedSet: map[string]bool{}},
		},
	}
}

func (m *sftpModel) cur() *sftpSideModel   { return &m.sides[m.focus.side()] }
func (m *sftpModel) other() *sftpSideModel { return &m.sides[1-m.focus.side()] }

// ------------------------------------------------------------------ geometry

// sftpNarrowW is where a 1:1 split stops being two usable columns. Below it only
// the focused side is drawn, and Tab is how you reach the other one.
const sftpNarrowW = 72

func (m sftpModel) narrow() bool { return m.w < sftpNarrowW }

// panes returns each side's outer width and the files/marks split. The two sides
// are 1:1 by design — neither is the "main" one.
func (m sftpModel) panes() (sideW, filesH, marksH int) {
	sideW = m.w / 2
	if m.narrow() {
		sideW = m.w
	}
	filesH = max(3, m.h*2/3)
	marksH = m.h - filesH
	if marksH < 3 { // too short to split: the browser takes it all
		filesH, marksH = m.h, 0
	}
	return
}

func (m *sftpModel) setSize(w, h int) {
	m.w, m.h = w, h
	m.clamp()
}

func (m *sftpModel) clamp() {
	for i := range m.sides {
		s := &m.sides[i]
		s.cursor = clamp(s.cursor, 0, max(0, s.rowCount()-1))
		s.top = clamp(s.top, 0, max(0, s.rowCount()-1))
		s.markCur = clamp(s.markCur, 0, max(0, len(s.marks)-1))
		s.markTop = clamp(s.markTop, 0, max(0, len(s.marks)-1))
	}
}

// sftpCwdRows is the path line at the top of a files panel — filu shows the
// directory inside the panel, above the listing, not tucked into a border.
const sftpCwdRows = 1

// visibleRows is how many list rows fit. The files panel gives one to the path.
func (m sftpModel) visibleRows(marks bool) int {
	_, filesH, marksH := m.panes()
	if marks {
		return max(1, marksH-2)
	}
	return max(1, filesH-2-sftpCwdRows)
}

// ---------------------------------------------------------------- navigation

// cycleFocus walks [4] -> [5] -> [6] -> [7], wrapping. It stays inside this tab:
// Tab moves between the panels you can see, and changing tab is 1/2/3.
func (m *sftpModel) cycleFocus(back bool) {
	n := int(panelRightMarks) + 1
	at := int(m.focus)
	if back {
		at--
	} else {
		at++
	}
	m.focus = sftpPanel((at + n) % n)
}

func (m *sftpModel) handleListKey(k string) {
	// h/l cross between the halves. Tab walks all four panels in order, which is
	// the right key when you want the next thing; h/l are the right key when you
	// know which side you want — and on a 1:1 split, that is most of the time.
	switch k {
	case "h", "left":
		m.focus = m.focus.onSide(sideLeft)
		return
	case "l", "right":
		m.focus = m.focus.onSide(sideRight)
		return
	}

	s := m.cur()
	if m.focus.isMarks() {
		rows := m.visibleRows(true)
		s.markCur = moveCursor(s.markCur, len(s.marks), k, rows)
		s.markTop = scrollTo(s.markTop, s.markCur, rows)
		return
	}
	rows := m.visibleRows(false)
	s.cursor = moveCursor(s.cursor, s.rowCount(), k, rows)
	s.top = scrollTo(s.top, s.cursor, rows)
}

// scrollTo keeps cur inside a window of vis rows starting at top.
func scrollTo(top, cur, vis int) int {
	if cur < top {
		return cur
	}
	if cur >= top+vis {
		return cur - vis + 1
	}
	return max(0, top)
}

// ------------------------------------------------------------- side actions

// connect points a side at a filesystem and lands it in that filesystem's home.
// Marks are dropped: they were paths on the previous host.
func (s *sftpSideModel) connect(fsys remote.FS) {
	// Stop any walk before the connection under it is closed.
	s.clearFilter()
	s.dialing = ""

	if s.fs != nil {
		s.fs.Close()
	}
	s.fs, s.host, s.err = fsys, fsys.Label(), ""
	s.marks, s.markedSet, s.markCur, s.markTop = nil, map[string]bool{}, 0, 0

	home, err := fsys.Home()
	if err != nil {
		home = "/"
	}
	s.home = home
	// home is kept for the ~ folding in the crumb; where the side OPENS is a
	// different question, and for this machine it is the launch directory.
	s.open(remote.StartDir(fsys, home))
}

// open lists dir and puts the cursor at the top. A failure leaves the previous
// listing on screen with the error beside it — an empty panel would look like an
// empty directory.
func (s *sftpSideModel) open(dir string) {
	if s.fs == nil {
		return
	}
	// The timestamp is taken BEFORE the listing on purpose — see dirMTime.
	mtime := dirMTime(s.fs, dir)
	entries, err := s.fs.List(dir)
	if err != nil {
		s.err = err.Error()
		return
	}
	s.cwd, s.entries, s.cursor, s.top, s.err = dir, entries, 0, 0, ""
	s.watchMTime = mtime
	s.clearFilter() // a filter belongs to the directory it was typed in
}

// reload re-lists the current directory in place, keeping the cursor. Used after
// a transfer lands: the arrival should show up without the user navigating away
// and back, but not by yanking them to the top of the listing.
func (s *sftpSideModel) reload() {
	if s.fs == nil {
		return
	}
	mtime := dirMTime(s.fs, s.cwd)
	entries, err := s.fs.List(s.cwd)
	if err != nil {
		s.err = err.Error()
		return
	}
	// A search is a snapshot of the moment it was run, so a re-list underneath
	// one leaves its results alone rather than half-refreshing them. Keeping the
	// cursor on the same entry is applyWatch's job either way.
	s.applyWatch(entries, mtime)
}

// rowCount is how many rows the files panel is showing.
func (s sftpSideModel) rowCount() int {
	if s.filtering {
		return len(s.matches)
	}
	return len(s.entries)
}

// rowAt is the entry drawn at row i.
//
// While searching, its Name is the path relative to cwd — the walk reaches below
// the current directory — so Join(cwd, Name) is still the absolute path and
// everything downstream (mark, transfer, Enter) keeps working without knowing a
// search happened. That is the whole reason the search draws in place.
func (s sftpSideModel) rowAt(i int) (remote.Entry, bool) {
	if s.filtering {
		if i < 0 || i >= len(s.matches) {
			return remote.Entry{}, false
		}
		return s.found[s.matches[i]], true
	}
	if i < 0 || i >= len(s.entries) {
		return remote.Entry{}, false
	}
	return s.entries[i], true
}

// enter descends into the entry under the cursor. Files are not opened: this is
// a transfer tab, not a viewer. A search result carries its relative path, so
// Enter on one lands in the directory it was found in, however deep that is.
func (s *sftpSideModel) enter() {
	e, ok := s.cursorEntry()
	if !ok || !e.IsDir {
		return
	}
	s.open(remote.Join(s.cwd, e.Name))
}

// up goes to the parent, and remembers nothing: the cursor lands at the top,
// which is where the eye already is after a directory changes.
func (s *sftpSideModel) up() {
	if s.fs == nil {
		return
	}
	if p := remote.Parent(s.cwd); p != s.cwd {
		s.open(p)
	}
}

func (s sftpSideModel) cursorEntry() (remote.Entry, bool) { return s.rowAt(s.cursor) }

// cursorPath is the absolute path under the files cursor.
func (s sftpSideModel) cursorPath() (string, bool) {
	e, ok := s.cursorEntry()
	if !ok {
		return "", false
	}
	return remote.Join(s.cwd, e.Name), true
}

// toggleMark is `m` in the files list. It toggles, which is what makes the
// common "I marked the wrong one" case a second press rather than a trip to
// another panel.
func (s *sftpSideModel) toggleMark() {
	p, ok := s.cursorPath()
	if !ok {
		return
	}
	if s.markedSet[p] {
		s.unmark(p)
		return
	}
	s.markedSet[p] = true
	s.marks = append(s.marks, p)
}

func (s *sftpSideModel) unmark(p string) {
	delete(s.markedSet, p)
	out := s.marks[:0]
	for _, m := range s.marks {
		if m != p {
			out = append(out, m)
		}
	}
	s.marks = out
	s.markCur = clamp(s.markCur, 0, max(0, len(s.marks)-1))
}

func (s *sftpSideModel) resetMarks() {
	s.marks, s.markedSet, s.markCur, s.markTop = nil, map[string]bool{}, 0, 0
}

// markCursorPath is the path under the marks cursor.
func (s sftpSideModel) markCursorPath() (string, bool) {
	if s.markCur >= len(s.marks) {
		return "", false
	}
	return s.marks[s.markCur], true
}

// closeAll tears both sides down: stop any walk, then close the connection
// under it. On the way out this is what keeps sshu from leaving two ssh
// connections for the server to time out on its own.
func (m *sftpModel) closeAll() {
	for i := range m.sides {
		s := &m.sides[i]
		s.clearFilter() // the walk holds the connection; stop it before closing
		if s.fs != nil {
			s.fs.Close()
			s.fs = nil
		}
	}
}

// status fills the capsule row's right-hand slot. A running transfer takes it:
// progress is the thing you glance at, and marks are still visible in [5]/[7].
func (m sftpModel) status(xfer string) string {
	if xfer != "" {
		return xfer
	}
	n := len(m.sides[0].marks) + len(m.sides[1].marks)
	if n == 0 {
		return "no marks"
	}
	return plural(n, "mark")
}
