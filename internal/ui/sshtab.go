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
	sshNarrowW   = sshLeftW + 28
	sshListLeadW = 1 // one cell of breathing room before the name
	// Both lists put an identifying detail at the right edge of the name line and
	// let the name wrap against it. Neither is ever truncated: a port you cannot
	// read does not identify a session, and a time you cannot read does not date
	// one. ":65535" and "15:04:05" are the widest each gets.
	sshPortW = 5 // "65535"; the port is never truncated
)

// sshTick polls the PTYs: it both reaps finished sessions and refreshes the
// screen while a remote is drawing.
type sshTickMsg struct{}

const sshTickEvery = 50 * time.Millisecond

type sshModel struct {
	// spinAt is the connecting spinner's frame. It counts ticks rather than
	// reading the clock, so the animation does not depend on when a frame is
	// drawn — the same as the sftp tab's.
	spinAt int

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
	// `l` used to cross into [5] as well. It was redundant: Enter on a session
	// already shows it AND focuses it (openSession), so `l` was a second key for
	// something one key already did — and every key that hands the keyboard to a
	// remote is one more way to end up somewhere you need Alt+Esc to leave.
	// Entering [5] is Enter, or the `5` that jumps to any panel.
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
	rows := fitLines(m.listBody(m.sessions, m.curSess, m.topSess, innerW, innerH), innerW, innerH)
	return panelChrome(innerW, rows, m.panelTitle(panelSessions), m.focus == panelSessions)
}

// ptyPanel draws the session on screen, or says why there is nothing to draw.
//
// A live session that has not said ANYTHING yet gets the connecting body rather
// than its own blank grid. Those two look identical — an empty bordered box —
// and they are not the same thing at all: ssh prints nothing while it waits for
// a TCP connection, and against an address that never answers that wait is the
// operating system's, which can run past a minute. What the user saw was a tab
// that did nothing, with no way to tell a slow host from a broken app.
//
// This is the same complaint the sftp dial spinner was built for, and this tab
// did not get one because "the PTY shows whatever the remote sends" — which is
// exactly wrong when the remote has not sent anything.
func (m sshModel) ptyPanel(w, h int) string {
	innerW, innerH := w-2, h-2
	s := m.currentSession()

	rows := m.ptyEmpty(innerW, innerH)
	switch {
	case s == nil:
	case s.state == sessLive && !s.pty.hasSpoken():
		rows = m.connectingBody(s, innerW, innerH)
	default:
		rows = s.pty.render(innerW, innerH)
	}
	return panelChrome(innerW, fitLines(rows, innerW, innerH),
		m.panelTitle(panelPty), m.focus == panelPty)
}

// connectingBody is the waiting state: it MOVES, because "is this thing stuck"
// is the question being asked and a still frame is what stuck looks like.
func (m sshModel) connectingBody(s *session, innerW, innerH int) []string {
	dim := lipgloss.NewStyle().Foreground(dimColor)
	hand := lipgloss.NewStyle().Foreground(handColor)

	spin := spinnerFrames[(m.spinAt/spinnerEvery)%len(spinnerFrames)]
	who := s.host.User + "@" + s.host.Host
	elapsed := ""
	if waited := int(time.Since(s.started).Seconds()); waited >= 2 {
		// Only once it is worth mentioning: a counter starting from zero on
		// every connection makes a fast one look slow.
		elapsed = "  " + itoa(waited) + "s"
	}

	plain := spin + " connecting to " + who + elapsed
	line := centerLine(innerW, plain,
		hand.Render(spin)+dim.Render(" connecting to ")+hand.Render(who)+dim.Render(elapsed))

	blank := spaces(innerW)
	out := make([]string, 0, max(0, innerH))
	for i := 0; i < max(0, (innerH-1)/2); i++ {
		out = append(out, blank)
	}
	return append(out, line)
}

func (m sshModel) ptyEmpty(innerW, innerH int) []string {
	return emptyBody(innerW, innerH, "No session on screen",
		emptyHint("Select a session in [4], or open one from [1]", "[4]", "[1]"))
}

// listBody lays out [4]. Each entry is a block, because a long address wraps.
func (m sshModel) listBody(items []*session, cursor, top, innerW, innerH int) []string {
	if len(items) == 0 {
		return emptyBody(innerW, innerH, "No sessions",
			emptyHint("Connect from [1] hosts", "[1]"))
	}

	out := make([]string, 0, max(0, innerH))
	for i := top; i < len(items) && len(out) < innerH; i++ {
		out = append(out, m.listItem(items[i], i == cursor, innerW)...)
	}
	return out
}

// listItem lays out one row of [4]:
//
//	<space><user>@<host>…<port>
//
// The row says what the connection IS rather than what it is called: two saved
// hosts can share a machine and a name is a label somebody chose, but
// `deploy@10.0.3.14` is the thing ssh actually did. The port sits at the right
// edge of the name line, which is where the empty space was, and never truncates.
//
// Colour is TWO independent channels:
//
//	foreground  ->  green when this is the session [5] is showing
//	background  ->  the cursor bar
//
// Because they are different channels there is no contention and no special
// case — which is what removed both the old inverse (a green BACKGROUND when the
// cursor landed on the on-screen row) and the terminal glyph that used to carry
// the same signal a second time.
//
// Where the two do meet, the bar wins and that one row's green is not visible.
// That is the trade, and it is a cheap one: the cursor is on the row, and [5]'s
// own title beside it is naming that session.
func (m sshModel) listItem(s *session, isCursor bool, innerW int) []string {
	nameFG := textColor
	if s.id == m.current {
		nameFG = liveColor
	}

	body := lipgloss.NewStyle().Foreground(nameFG)
	dim := lipgloss.NewStyle().Foreground(dimColor)
	if isCursor {
		bar := lipgloss.NewStyle().Foreground(lipgloss.Color(baseHex)).Background(handColor)
		body, dim = bar, bar
	}

	// tailCell is the gap AND the slot together — one number, so the gap cannot be
	// counted twice (subtracted from the name and again inside the slot), which is
	// exactly how this row once came out a cell short.
	tailCell := 0
	if innerW >= sshListLeadW+sshPortW+2 {
		tailCell = sshPortW + 1
	}

	// The ordinal rides along with the address instead of owning a slot: it is
	// the only thing separating two sessions to the same host, and neither the
	// address nor the port can do that job.
	label := s.host.User + "@" + s.host.Host
	if tag := s.ordinalTag(); tag != "" {
		label += " " + tag
	}
	nameW := max(1, innerW-sshListLeadW-tailCell)
	lines := wrapText(label, nameW)

	lead := strings.Repeat(" ", sshListLeadW)
	out := make([]string, 0, len(lines))
	for i, l := range lines {
		tail := strings.Repeat(" ", tailCell)
		if tailCell > 0 && i == len(lines)-1 {
			tail = padLeft(strconv.Itoa(s.host.Port), tailCell)
		}
		out = append(out, body.Render(lead+padRight(l, nameW))+dim.Render(tail))
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
