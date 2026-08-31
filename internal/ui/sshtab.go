package ui

import (
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/vulcanshen/sshu/internal/store"
)

// Tab [3] holds two panels, numbered [4] [5] — the digits continue past
// the tabs rather than restarting, so one number always means one surface and
// nothing collides with the tab hotkeys.
type sshPanel int

const (
	panelSessions sshPanel = iota // [4] live sessions
	panelPty                      // [5] the session on screen
)

// Geometry. The left column is fixed: a draggable split would put the panel
// width back on the content, and every resize would have to be re-derived
// (§1.2). Below sshNarrowW the columns cannot both be useful, so the PTY takes
// the screen and the lists are reached with Alt+Esc.
//
// 26 is close to the floor. The list is name-only, so the column is the name
// plus a 2-cell marker and a 4-cell ordinal slot — go much below this and the
// overhead starts to dominate and ordinary hostnames begin to wrap.
const (
	sshLeftW = 26
	// Derived, not a separate constant to forget: the split is worth keeping while
	// the PTY still has enough columns to be a terminal. Narrowing the left column
	// therefore also lets the split survive on narrower terminals.
	sshNarrowW    = sshLeftW + 28
	sshListGlyphW = 2 // fixed marker column: the glyph plus its trailing space
	// Both lists put an identifying detail at the right edge of the name line and
	// let the name wrap against it. Neither is ever truncated: a port you cannot
	// read does not identify a session, and a time you cannot read does not date
	// one. ":65535" and "15:04:05" are the widest each gets.
	sshPortW = 6
	sshTimeW = 8
)

// sshTick polls the PTYs: it both reaps finished sessions and refreshes the
// screen while a remote is drawing.
type sshTickMsg struct{}

const sshTickEvery = 50 * time.Millisecond

type sshModel struct {
	// applied* is the geometry last pushed to the PTY. Resizing is a SIGWINCH
	// and makes the remote redraw, so it is only done when something changed —
	// focus flips the layout on every Alt+Esc.
	appliedCols, appliedRows, appliedID int

	sessions []*session // live, oldest first
	history  []*session // ended, newest first
	nextID   int
	current  int // id shown in [5]; 0 = nothing
	focus    sshPanel

	curSess int
	topSess int
	w, h    int
}

func newSSHModel() sshModel { return sshModel{focus: panelSessions, nextID: 1} }

// ------------------------------------------------------------------ geometry

func (m sshModel) narrow() bool { return m.w < sshNarrowW }

// panes returns the three panel boxes' outer sizes for the current terminal.
//
// A focused PTY gets the whole tab. While the remote has the keyboard the lists
// are unreachable anyway — keeping them on screen spends a quarter of the width
// on something you cannot touch — so they fold away and come back on Alt+Esc.
// The left column is one panel now. It used to be split with a history list
// underneath, which could not be acted on and was usually empty — see
// sshhistory.go for where that went.
func (m sshModel) panes() (leftW, leftH, rightW, rightH int) {
	total := m.h
	if m.narrow() || m.focus == panelPty {
		return 0, 0, m.w, total
	}
	return sshLeftW, total, m.w - sshLeftW, total
}

// listRows is how many rows [4] shows, which is the page size u/d move by half
// of. The pty is not a list.
func (m sshModel) listRows() int {
	_, leftH, _, _ := m.panes()
	return max(1, leftH-2)
}

// ptyInner is the cell grid the remote is told it has.
func (m sshModel) ptyInner() (cols, rows int) {
	_, _, rightW, rightH := m.panes()
	return max(1, rightW-2), max(1, rightH-2)
}

func (m *sshModel) setSize(w, h int) {
	m.w, m.h = w, h
	// Only the displayed session is resized. A background session keeps the
	// geometry it was started with until it comes on screen, which is what a
	// terminal multiplexer does too — resizing an unwatched remote just makes it
	// redraw for nobody.
	cols, rows := m.ptyInner()
	s := m.currentSession()
	if s != nil && (cols != m.appliedCols || rows != m.appliedRows || s.id != m.appliedID) {
		s.pty.resize(cols, rows)
		m.appliedCols, m.appliedRows, m.appliedID = cols, rows, s.id
	}
	m.clampCursors()
}

// setFocus moves focus and re-applies the geometry, because focus changes the
// layout: entering [5] gives it the whole tab, leaving gives the width back.
func (m *sshModel) setFocus(p sshPanel) {
	m.focus = p
	m.setSize(m.w, m.h)
}

// ---------------------------------------------------------------- lifecycle

// currentSession is only ever a LIVE session. A finished one is released the
// moment it is reaped, so [5] falls back to its empty state instead of freezing
// on a dead screen that still looks like a prompt.
func (m sshModel) currentSession() *session {
	for _, s := range m.sessions {
		if s.id == m.current {
			return s
		}
	}
	return nil
}

func (m sshModel) liveCount() int { return len(m.sessions) }

// connect starts a session and puts it on screen. A failure to even launch ssh
// lands in history with the reason, rather than vanishing.
func (m *sshModel) connect(h store.Host) (*session, error) {
	cols, rows := m.ptyInner()
	s := &session{id: m.nextID, host: h, started: time.Now(), state: sessLive}
	m.nextID++

	p, err := startPty(buildSSHCmd(h, selfPath()), cols, rows)
	if err != nil {
		s.state, s.ended, s.reason = sessEnded, time.Now(), "failed to start: "+err.Error()
		m.history = append([]*session{s}, m.history...)
		return s, err
	}
	s.pty = p
	m.sessions = append(m.sessions, s)
	m.renumber()
	m.current = s.id
	m.curSess = len(m.sessions) - 1
	m.clampCursors()
	return s, nil
}

// renumber assigns #N to hosts that have more than one live session, so the
// name-only list can still tell them apart.
func (m *sshModel) renumber() {
	count := map[string]int{}
	for _, s := range m.sessions {
		count[s.host.Name]++
	}
	seen := map[string]int{}
	for _, s := range m.sessions {
		if count[s.host.Name] < 2 {
			s.ordinal = 0
			continue
		}
		seen[s.host.Name]++
		s.ordinal = seen[s.host.Name]
	}
}

// reap moves finished sessions into history. Returns true if anything changed,
// so the caller knows to keep ticking or not.
// reap moves finished sessions to history and RETURNS them, because a session
// ending used to be completely silent: it left [4], [5] switched away, and the
// only trace was a panel that no longer exists. The caller says so.
func (m *sshModel) reap() []*session {
	var live, ended []*session
	for _, s := range m.sessions {
		if !s.pty.exited() {
			live = append(live, s)
			continue
		}
		s.state, s.ended, s.reason = sessEnded, time.Now(), s.pty.exitReason()
		s.ordinal = 0
		s.ok = s.reason == "exited 0"
		// The screen is not kept, so neither is the emulator behind it: a whole
		// terminal grid per past session, for something nothing renders.
		s.pty.stop()
		s.pty = nil
		if s.id == m.current {
			m.current = 0
		}
		m.history = append([]*session{s}, m.history...)
		ended = append(ended, s)
	}
	if len(ended) == 0 {
		return nil
	}
	m.sessions = live
	m.renumber()
	if len(m.history) > sshHistoryCap {
		m.history = m.history[:sshHistoryCap]
	}
	// A session that dies while focused would otherwise trap the keyboard in a
	// PTY nobody is driving.
	if m.focus == panelPty && m.currentSession() == nil {
		m.setFocus(panelSessions)
	}
	m.clampCursors()
	return ended
}

// sshHistoryCap bounds the in-memory history. Entries are metadata only once
// reaped, so this is about a list staying readable, not about memory.
const sshHistoryCap = 200

// stopAll kills every subprocess. Called on quit.
func (m *sshModel) stopAll() {
	for _, s := range m.sessions {
		s.pty.stop()
	}
}

func (m *sshModel) clampCursors() {
	m.curSess = clamp(m.curSess, 0, max(0, len(m.sessions)-1))
	m.topSess = clamp(m.topSess, 0, max(0, len(m.sessions)-1))
}

func clamp(v, lo, hi int) int { return min(max(v, lo), hi) }

// ------------------------------------------------------------------- keys

// tick keeps the screen in step with the remote while anything is running.
func (m sshModel) tick() tea.Cmd {
	if len(m.sessions) == 0 {
		return nil
	}
	return tea.Tick(sshTickEvery, func(time.Time) tea.Msg { return sshTickMsg{} })
}

// cycleFocus has one place to go: [4]. Tab stays inside this tab and [5] is not
// in the cycle — it hands the keyboard to a remote, so tabbing into it would
// swallow the very key that got you there and leave no way forward. Entering the
// PTY is a deliberate act (Enter on a session, the 5 key, or l) and leaving it
// is Alt+Esc.
//
// So Tab is a no-op here unless you are in the PTY, where it is a way out that
// costs nothing to offer.
func (m *sshModel) cycleFocus(back bool) { m.setFocus(panelSessions) }

func (m *sshModel) handleListKey(k string) {
	// l crosses into [5], spatially: [4] and [6] are the left column and the pty
	// is the right one, the same "go right" h/l mean in tab [2].
	//
	// Tab deliberately does NOT do this (§4.4.1) and that is not a contradiction:
	// Tab would be swallowed by the remote, so tabbing in locks the very key that
	// got you there. `l` is not a way back OUT of anywhere, so lending it to the
	// pty costs nothing — and the way out is the Alt+Esc it always was.
	if k == "l" || k == "right" {
		m.setFocus(panelPty)
		return
	}

	switch m.focus {
	case panelSessions:
		m.curSess = moveCursor(m.curSess, len(m.sessions), k, m.listRows())
		m.clampCursors()
	}
}

// status fills the capsule row's right-hand slot.
func (m sshModel) status() string {
	if len(m.sessions) == 0 && len(m.history) == 0 {
		return "no sessions"
	}
	return itoa(len(m.sessions)) + " live · " + itoa(len(m.history)) + " past"
}

// plural renders a count with its noun, so a message reads as English rather
// than as "1 sessions".
func plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return itoa(n) + " " + noun + "s"
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

// -------------------------------------------------------------------- view

func (m sshModel) view() string {
	// panes() is the only place that decides the shape. Repeating the narrow test
	// here is how the two got out of step when focus became a second reason to
	// collapse the split, and a zero-width panel is a crash, not a cosmetic bug.
	leftW, leftH, rightW, rightH := m.panes()
	if leftW <= 0 {
		return m.ptyPanel(rightW, rightH)
	}
	return joinHorizontal(m.sessionsPanel(leftW, leftH), m.ptyPanel(rightW, rightH))
}

// panelTitle is what panel p's capsule says, and what the Space menu calls the
// surface it was opened on — one function for both (see sftpModel.panelTitle).
func (m sshModel) panelTitle(p sshPanel) string {
	switch p {
	case panelSessions:
		return "[4] sessions"
	}
	if s := m.currentSession(); s != nil {
		return "[5] " + s.host.Name
	}
	return "[5] ssh"
}

func (m sshModel) sessionsPanel(w, h int) string {
	innerW, innerH := w-2, h-2
	rows := fitLines(m.listBody(m.sessions, m.curSess, m.topSess, innerW, innerH, false), innerW, innerH)
	return panelChrome(innerW, rows, m.panelTitle(panelSessions), m.focus == panelSessions)
}

func (m sshModel) ptyPanel(w, h int) string {
	innerW, innerH := w-2, h-2
	s := m.currentSession()

	rows := m.ptyEmpty(innerW, innerH)
	if s != nil {
		rows = s.pty.render(innerW, innerH)
	}
	return panelChrome(innerW, fitLines(rows, innerW, innerH),
		m.panelTitle(panelPty), m.focus == panelPty)
}

func (m sshModel) ptyEmpty(innerW, innerH int) []string {
	dim := lipgloss.NewStyle().Foreground(dimColor)
	key := lipgloss.NewStyle().Foreground(handColor)
	plain := "Select a session in [4], or open one from [1]"
	line := centerLine(innerW, plain,
		dim.Render("Select a session in ")+key.Render("[4]")+
			dim.Render(", or open one from ")+key.Render("[1]"))

	blank := strings.Repeat(" ", innerW)
	out := make([]string, 0, innerH)
	for i := 0; i < max(0, (innerH-1)/2); i++ {
		out = append(out, blank)
	}
	return append(out, line)
}

// listBody lays out one of the two lists. Each entry is a block: the wrapped
// name, plus (history only) a dim line saying how it ended — history exists to
// answer that, so dropping it would leave a list of bare names.
func (m sshModel) listBody(items []*session, cursor, top, innerW, innerH int, withReason bool) []string {
	if len(items) == 0 {
		dim := lipgloss.NewStyle().Foreground(dimColor)
		empty := "none"
		if withReason {
			empty = "nothing ended yet"
		}
		return []string{dim.Render(padRight(" "+empty, innerW))}
	}

	out := make([]string, 0, max(0, innerH))
	for i := top; i < len(items) && len(out) < innerH; i++ {
		out = append(out, m.listItem(items[i], i == cursor, innerW, withReason)...)
	}
	return out
}

// listItem lays out one row of [4] or [6].
//
//	[4]  <glyph><space><name…><port>
//	[6]  <space><space><name…><time>
//	     <space><space><reason>
//
// Both put an identifying detail at the right edge of the name line, which is
// where the empty space was. [6] shows the time it ENDED — that is the event the
// entry records — and no date: history lives in memory and dies with the
// process, so a date could only ever say today.
//
// Colour, in [4]:
//
//	cursor + on screen  ->  green BACKGROUND, uncoloured text  (inverse)
//	cursor              ->  handColor background, uncoloured text
//	on screen           ->  green foreground on glyph and name
//	otherwise           ->  plain text
//
// A row can only carry one background. Rather than let the cursor bar hide the
// fact that this is the session on screen, the two signals combine there: the
// bar takes the colour the foreground would have had.
//
// In [6] the colour belongs to the REASON only — "exited 0" is green, a failure
// is red — and the name stays plain. How a session ended is a property of the
// ending, not of the host, so painting the whole row would overstate it.
func (m sshModel) listItem(s *session, isCursor bool, innerW int, withReason bool) []string {
	onScreen := !withReason && s.id == m.current

	nameFG := textColor
	if onScreen {
		nameFG = liveColor
	}
	reasonFG := warnColor
	if s.ok {
		reasonFG = liveColor
	}

	body := lipgloss.NewStyle().Foreground(nameFG)
	reason := lipgloss.NewStyle().Foreground(reasonFG)
	dim := lipgloss.NewStyle().Foreground(dimColor)
	if isCursor {
		// The cursor is always a filled bar. It inverts only for the on-screen
		// session, where the bar would otherwise erase that signal outright; a
		// history row loses its reason colour under the bar, which is fine —
		// the text still says what happened.
		bg := handColor
		if onScreen {
			bg = liveColor
		}
		bar := lipgloss.NewStyle().Foreground(lipgloss.Color(baseHex)).Background(bg)
		body, reason, dim = bar, bar, bar
	}

	// The marker column is a fixed slot: the glyph appears and disappears without
	// shifting the name sideways (filu's pick-mark rule).
	marker, gutter := "  ", "  "
	// tailCell is the gap AND the slot together — one number, so the gap cannot be
	// counted twice (subtracted from the name and again inside the slot), which is
	// exactly how this row once came out a cell short.
	tailW := sshPortW
	if withReason {
		tailW = sshTimeW
	}
	tailCell := 0
	if innerW >= sshListGlyphW+tailW+2 {
		tailCell = tailW + 1
	}
	if onScreen {
		marker = glyphOnScreen + " "
	}

	// The ordinal rides along with the name instead of owning a slot: it is the
	// only thing separating two sessions to the same host, and the port cannot
	// do that job.
	label := s.host.Name
	if tag := s.ordinalTag(); tag != "" {
		label += " " + tag
	}
	nameW := max(1, innerW-sshListGlyphW-tailCell)
	lines := wrapText(label, nameW)

	tailText := ""
	if tailCell > 0 {
		tailText = ":" + strconv.Itoa(s.host.Port)
		if withReason {
			tailText = s.ended.Format("15:04:05")
		}
	}

	out := make([]string, 0, len(lines)+1)
	for i, l := range lines {
		lead := marker
		if i > 0 {
			lead = gutter
		}
		tail := strings.Repeat(" ", tailCell)
		if tailCell > 0 && i == len(lines)-1 {
			tail = padLeft(tailText, tailCell)
		}
		out = append(out, body.Render(lead+padRight(l, nameW))+dim.Render(tail))
	}

	if withReason && s.reason != "" {
		out = append(out, reason.Render(padRight(gutter+s.reason, innerW)))
	}
	return out
}

// wrapText breaks s to at most w cells per line, preferring a break just after
// a separator so a hostname splits at a dot or dash instead of mid-token.
func wrapText(s string, w int) []string {
	if w <= 0 {
		return []string{""}
	}
	var out []string
	rs := []rune(s)
	for len(rs) > 0 {
		if dispW(string(rs)) <= w {
			out = append(out, string(rs))
			break
		}
		cut := 0
		used := 0
		for i, r := range rs {
			rw := dispW(string(r))
			if used+rw > w {
				break
			}
			used += rw
			cut = i + 1
		}
		// Prefer a separator in the last third, so "db-replica-tokyo-ap-" breaks
		// after the dash rather than inside "ap".
		if best := lastSepBefore(rs[:cut], cut*2/3); best > 0 {
			cut = best
		}
		out = append(out, string(rs[:cut]))
		rs = rs[cut:]
	}
	if len(out) == 0 {
		out = []string{""}
	}
	return out
}

func lastSepBefore(rs []rune, floor int) int {
	for i := len(rs) - 1; i >= floor; i-- {
		if strings.ContainsRune("-._/", rs[i]) {
			return i + 1
		}
	}
	return 0
}
