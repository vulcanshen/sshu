package ui

import (
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/vulcanshen/sshu/internal/store"
)

// The ssh tab is three surfaces: the sessions list [1], the layout strip [2],
// and a GRID of live terminals. Any number of sessions can put a cell on the
// grid at once — Tab on the list toggles a session's cell, Enter shows one
// and hands it the keyboard, Alt+1..9 jump between cells, and Alt+Esc brings
// the keyboard (and the side column) back.
type sshPanel int

const (
	panelSessions sshPanel = iota // [1] live sessions
	panelLayout                   // [2] how the grid is arranged
	panelPty                      // the keyboard is inside a grid cell
)

// layoutMode is the grid's arrangement. Horizontal and vertical are the two
// degenerate grids (one row / one column); custom is a fixed columns × rows.
type layoutMode int

const (
	layoutHorizontal layoutMode = iota
	layoutVertical
	layoutCustom
	layoutModeCount
)

func (l layoutMode) label() string {
	switch l {
	case layoutVertical:
		return "vertical"
	case layoutCustom:
		return "custom"
	}
	return "horizontal"
}

// Geometry. The left column is fixed: a draggable split would put the panel
// width back on the content, and every resize would have to be re-derived
// (§1.2). Below sshNarrowW the columns cannot both be useful, so the grid
// takes the screen and the list is reached with Alt+Esc.
const (
	sshLeftW     = 26
	sshNarrowW   = sshLeftW + 28
	sshListLeadW = 1
	// layoutRows is the layout strip's fixed height — border, one option row
	// per mode, border. It lives at the BOTTOM OF THE LEFT COLUMN, so the
	// right side is nothing but terminals.
	layoutRows = 5
)

// sshTick polls the PTYs: it both reaps finished sessions and refreshes the
// screen while a remote is drawing.
type sshTickMsg struct{}

const sshTickEvery = 50 * time.Millisecond

type sshModel struct {
	// timeout is one connection attempt's budget, from config.yaml.
	timeout time.Duration

	// failed is a session that ended badly while its cell was the last thing
	// on the grid. The grid keeps saying what happened instead of going blank
	// — a failure that erases itself is a failure you cannot read.
	failed *session

	// spinAt is the connecting spinner's frame, shared by every connecting
	// cell. It counts ticks rather than reading the clock.
	spinAt int

	sessions []*session // live, oldest first
	nextID   int

	// shown is the grid, in display order: session ids, first toggled first.
	// Alt+1 is shown[0]. A session not in here keeps running off screen.
	shown []int
	// focusPty is the cell holding the keyboard — an index into shown, only
	// meaningful while focus == panelPty.
	focusPty int

	layout       layoutMode
	gridC, gridR int // custom's columns × rows

	focus   sshPanel
	curSess int
	topSess int
	w, h    int
}

func newSSHModel() sshModel {
	return sshModel{focus: panelSessions, nextID: 1, gridC: 2, gridR: 2}
}

// ------------------------------------------------------------------ geometry

func (m sshModel) narrow() bool { return m.w < sshNarrowW }

// panes is the outer split. The side column folds away while the keyboard is
// inside the grid — the list is unreachable then anyway, and a quarter of the
// width should not be spent on something you cannot touch. Alt+Esc brings it
// back.
func (m sshModel) panes() (leftW, rightW int) {
	if m.narrow() || m.focus == panelPty {
		return 0, m.w
	}
	return sshLeftW, m.w - sshLeftW
}

// stripVisible: the layout strip yields when the left column is too short to
// hold it AND a usable sessions list.
func (m sshModel) stripVisible() bool { return m.h >= layoutRows+5 }

// gridArea is the box the terminal grid gets: the whole right column. The
// layout strip lives at the bottom of the LEFT column, so the right side is
// nothing but terminals.
func (m sshModel) gridArea() (w, h int) {
	_, rightW := m.panes()
	return rightW, max(1, m.h)
}

// gridDims is how many columns and rows the grid uses for n cells. Custom
// respects its stated shape and grows ROWS when overfilled — a cell must
// never silently not exist.
func (m sshModel) gridDims(n int) (cols, rows int) {
	if n <= 0 {
		return 0, 0
	}
	switch m.layout {
	case layoutVertical:
		return 1, n
	case layoutCustom:
		c := clamp(m.gridC, 1, 9)
		r := clamp(m.gridR, 1, 9)
		return c, max(r, (n+c-1)/c)
	default:
		return n, 1
	}
}

// splitEven divides total cells among parts, spreading the remainder from the
// left so the widths always sum EXACTLY to total — the frame invariant does
// not tolerate a stray column.
func splitEven(total, parts int) []int {
	out := make([]int, max(0, parts))
	for i := range out {
		out[i] = total / parts
		if i < total%parts {
			out[i]++
		}
	}
	return out
}

// sessionsH is the sessions panel's share of the left column — what the
// layout strip below it does not take.
func (m sshModel) sessionsH() int {
	if !m.stripVisible() {
		return m.h
	}
	return m.h - layoutRows
}

// listRows is how many rows [1] shows, which is the page size u/d move by
// half of.
func (m sshModel) listRows() int { return max(1, m.sessionsH()-2) }

func (m *sshModel) setSize(w, h int) {
	m.w, m.h = w, h
	m.applyGeometry()
	m.clampCursors()
}

// setFocus moves focus and re-applies the geometry, because focus changes the
// layout: the side column folds while the grid holds the keyboard.
func (m *sshModel) setFocus(p sshPanel) {
	m.focus = p
	m.applyGeometry()
}

// applyGeometry pushes each displayed session's cell size to its PTY. A
// resize is a SIGWINCH and makes the remote redraw, so it only reaches a
// session whose numbers actually changed — appliedCols/Rows on the session
// remember what it was last told.
func (m *sshModel) applyGeometry() {
	shown := m.shownSessions()
	n := len(shown)
	if n == 0 {
		return
	}
	gw, gh := m.gridArea()
	cols, rows := m.gridDims(n)
	ws, hs := splitEven(gw, cols), splitEven(gh, rows)
	for i, s := range shown {
		cellCols, cellRows := max(1, ws[i%cols]-2), max(1, hs[i/cols]-2)
		if s.pty != nil && (cellCols != s.appliedCols || cellRows != s.appliedRows) {
			s.pty.resize(cellCols, cellRows)
			s.appliedCols, s.appliedRows = cellCols, cellRows
		}
	}
}

// ---------------------------------------------------------------- lifecycle

func (m sshModel) byID(id int) *session {
	for _, s := range m.sessions {
		if s.id == id {
			return s
		}
	}
	return nil
}

func (m sshModel) isShown(id int) bool {
	for _, sid := range m.shown {
		if sid == id {
			return true
		}
	}
	return false
}

// shownSessions resolves the grid to its live sessions, in display order.
func (m sshModel) shownSessions() []*session {
	out := make([]*session, 0, len(m.shown))
	for _, id := range m.shown {
		if s := m.byID(id); s != nil {
			out = append(out, s)
		}
	}
	return out
}

// currentSession is the session whose PTY holds the keyboard, which only
// means something while focus is inside the grid.
func (m sshModel) currentSession() *session {
	if m.focus != panelPty || m.focusPty < 0 || m.focusPty >= len(m.shown) {
		return nil
	}
	return m.byID(m.shown[m.focusPty])
}

func (m sshModel) liveCount() int { return len(m.sessions) }

// connect starts a session, puts its cell on the grid and points focusPty at
// it (the caller decides whether to enter). A failure to even launch ssh
// lands in failed with the reason, rather than vanishing.
func (m *sshModel) connect(h store.Host) (*session, error) {
	m.failed = nil
	s := &session{id: m.nextID, host: h, started: time.Now(), state: sessLive}
	m.nextID++

	// A provisional size; applyGeometry corrects it to the real cell the
	// moment the session joins the grid.
	p, err := startPty(buildSSHCmd(h, selfPath(), m.timeoutSecs()), 80, 24)
	if err != nil {
		s.state, s.ended, s.reason = sessEnded, time.Now(), "failed to start: "+err.Error()
		m.failed = s
		return s, err
	}
	s.pty, s.appliedCols, s.appliedRows = p, 80, 24
	m.sessions = append(m.sessions, s)
	m.renumber()
	m.shown = append(m.shown, s.id)
	m.focusPty = len(m.shown) - 1
	m.curSess = len(m.sessions) - 1
	m.applyGeometry()
	m.clampCursors()
	return s, nil
}

// timeoutSecs is the budget in whole seconds, defaulted here so a zero value
// model (tests that build sshModel directly) still behaves.
func (m sshModel) timeoutSecs() int {
	if m.timeout <= 0 {
		return store.DefaultConnectTimeout
	}
	return int(m.timeout / time.Second)
}

// sweepStalled stops sessions that have said NOTHING for longer than the budget.
//
// ssh's own -o ConnectTimeout covers the TCP connect and covers it better than
// this can, because ssh prints a real message when it fires. What it does not
// cover is a host that completes the connection and then goes quiet. The grace
// is what keeps the two from racing: ssh gets to fire its own timeout, with
// its own words, before sshu reaches for the plug.
func (m *sshModel) sweepStalled() {
	if len(m.sessions) == 0 {
		return
	}
	budget := m.timeout
	if budget <= 0 {
		budget = store.DefaultConnectTimeout * time.Second
	}
	deadline := budget + stallGrace
	for _, s := range m.sessions {
		if s.pty.hasSpoken() || s.pty.exited() || time.Since(s.started) < deadline {
			continue
		}
		s.timedOut = true
		s.pty.stop() // reap notices on the next tick
	}
}

// stallGrace is how long ssh's own timeout gets before sshu stops waiting for
// it to act.
const stallGrace = 5 * time.Second

// renumber assigns #N to hosts that have more than one live session, so the
// list and the cell titles can still tell them apart.
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

// reap moves finished sessions out and RETURNS them, because a session ending
// used to be completely silent. Its cell leaves the grid; the keyboard must
// never silently land in another remote, so if the FOCUSED cell died, focus
// falls back to the list.
func (m *sshModel) reap() []*session {
	focusID := -1
	if m.focus == panelPty && m.focusPty >= 0 && m.focusPty < len(m.shown) {
		focusID = m.shown[m.focusPty]
	}

	var live, ended []*session
	var lastBadShown *session
	for _, s := range m.sessions {
		if !s.pty.exited() {
			live = append(live, s)
			continue
		}
		s.state, s.ended, s.reason = sessEnded, time.Now(), s.pty.exitReason()
		s.ordinal = 0
		s.ok = s.reason == "exited 0"
		if s.timedOut {
			// It was killed, so its exit code says nothing worth repeating.
			s.reason = "no answer after " + itoa(m.timeoutSecs()) + "s"
			s.ok = false
		}
		// What ssh SAID beats what its exit code implies: "exited 255" is the
		// same for a refused connection, a wrong key and a changed host key,
		// and the line on the screen tells them apart. Read it before the
		// emulator goes.
		if !s.ok {
			if w := s.pty.lastWords(); w != "" {
				s.reason = w
			}
			s.detail = s.pty.screenLines()
			if m.isShown(s.id) {
				lastBadShown = s
			}
		}
		// The screen is not kept, so neither is the emulator behind it.
		s.pty.stop()
		s.pty = nil
		m.dropShown(s.id)
		ended = append(ended, s)
	}
	if len(ended) == 0 {
		return nil
	}
	m.sessions = live
	m.renumber()

	// A failure holds the grid only when it left the grid EMPTY — with other
	// cells still live, the toast and the log carry it and the grid reflows.
	if len(m.shown) == 0 && lastBadShown != nil {
		m.failed = lastBadShown
	}
	if m.focus == panelPty {
		if idx := indexOfID(m.shown, focusID); idx >= 0 {
			m.focusPty = idx
		} else {
			m.setFocus(panelSessions)
		}
	}
	m.applyGeometry()
	m.clampCursors()
	return ended
}

func (m *sshModel) dropShown(id int) {
	for i, sid := range m.shown {
		if sid == id {
			m.shown = append(m.shown[:i], m.shown[i+1:]...)
			if m.focusPty >= len(m.shown) {
				m.focusPty = max(0, len(m.shown)-1)
			}
			return
		}
	}
}

func indexOfID(ids []int, id int) int {
	for i, v := range ids {
		if v == id {
			return i
		}
	}
	return -1
}

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

// toggleShown flips one session's cell in and out of the grid.
func (m *sshModel) toggleShown(id int) {
	m.failed = nil
	if m.isShown(id) {
		m.dropShown(id)
	} else {
		m.shown = append(m.shown, id)
	}
	m.applyGeometry()
}

// toggleCursorShown is Tab on [1]: flip the cursor session's cell. Tab's
// panel-cycling job is meaningless on this tab — the grid is not somewhere
// Tab may wander (it would swallow the key) — so the key was free for the
// thing the list actually does all day.
func (m *sshModel) toggleCursorShown() {
	if m.focus != panelSessions || m.curSess >= len(m.sessions) {
		return
	}
	m.toggleShown(m.sessions[m.curSess].id)
}

// showAndFocus puts a session's cell on the grid (if it is not already there)
// and hands it the keyboard. The side column folds on the way in.
func (m *sshModel) showAndFocus(id int) {
	m.failed = nil
	if !m.isShown(id) {
		m.shown = append(m.shown, id)
	}
	if idx := indexOfID(m.shown, id); idx >= 0 {
		m.focusPty = idx
	}
	m.setFocus(panelPty)
}

// moveCell steers the keyboard one cell in a direction — Alt+arrows. Edges
// CLAMP rather than wrap: a spatial move that teleports to the far side is
// the one thing a spatial move must never do (same rule as u/d in lists).
// The bottom row of a ragged grid simply has no cell past its end.
func (m *sshModel) moveCell(dx, dy int) {
	n := len(m.shown)
	if m.focus != panelPty || n == 0 {
		return
	}
	cols, _ := m.gridDims(n)
	r, c := m.focusPty/cols+dy, m.focusPty%cols+dx
	if r < 0 || c < 0 || c >= cols {
		return
	}
	i := r*cols + c
	if i < 0 || i >= n {
		return
	}
	m.failed = nil
	m.focusPty = i
}

// layoutKey drives the [2] strip: j/k (the options stack vertically now —
// h/l still answer) walk the three modes and apply immediately. It reports
// whether the caller should ask for custom's shape — Enter on custom is the
// request to change it.
func (m *sshModel) layoutKey(k string) (askDims bool) {
	switch k {
	case "k", "up", "h", "left":
		m.layout = (m.layout + layoutModeCount - 1) % layoutModeCount
	case "j", "down", "l", "right":
		m.layout = (m.layout + 1) % layoutModeCount
	case "enter":
		return m.layout == layoutCustom
	default:
		return false
	}
	m.applyGeometry()
	return false
}

func (m *sshModel) handleListKey(k string) {
	switch m.focus {
	case panelSessions:
		m.curSess = moveCursor(m.curSess, len(m.sessions), k, m.listRows())
		m.clampCursors()
	}
}

// status fills the capsule row's right-hand slot.
func (m sshModel) status() string {
	if len(m.sessions) == 0 {
		return "no sessions"
	}
	out := plural(len(m.sessions), "live session")
	if n := len(m.shown); n > 0 {
		out += " · " + itoa(n) + " on grid"
	}
	return out
}

// endedBadlyToast is the immediate half of the news. The log keeps the detail;
// this only has to be enough to make somebody open it.
func endedBadlyToast(bad []*session) string {
	switch len(bad) {
	case 0:
		return ""
	case 1:
		return bad[0].host.Name + " · " + bad[0].reason
	default:
		return plural(len(bad), "session") + " ended badly · see [Alt+p] logs"
	}
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
	leftW, _ := m.panes()
	right := m.gridView()
	if leftW <= 0 {
		return right
	}
	left := m.sessionsPanel(leftW, m.sessionsH())
	if m.stripVisible() {
		left = joinVertical(left, m.layoutPanel(leftW))
	}
	return joinHorizontal(left, right)
}

// panelTitle is what panel p's capsule says, and what the Space menu calls
// the surface it was opened on — one function for both.
func (m sshModel) panelTitle(p sshPanel) string {
	switch p {
	case panelSessions:
		return "[1] sessions"
	case panelLayout:
		return "[2] layout"
	}
	if s := m.currentSession(); s != nil {
		t := s.host.Name
		if tag := s.ordinalTag(); tag != "" {
			t += " " + tag
		}
		return t
	}
	return "ssh"
}

func (m sshModel) sessionsPanel(w, h int) string {
	innerW, innerH := w-2, h-2
	rows := fitLines(m.listBody(m.sessions, m.curSess, m.topSess, innerW, innerH), innerW, innerH)
	return panelChrome(innerW, rows, m.panelTitle(panelSessions), m.focus == panelSessions)
}

// layoutPanel is the [2] strip at the bottom of the left column: one radio
// row per mode, stacked — a 24-cell column cannot seat three choices side by
// side, and a list reads naturally under a list.
func (m sshModel) layoutPanel(w int) string {
	innerW := w - 2
	dim := lipgloss.NewStyle().Foreground(dimColor)
	on := lipgloss.NewStyle().Foreground(textColor)
	if m.focus == panelLayout {
		on = lipgloss.NewStyle().Foreground(editColor).Bold(true)
	}

	rows := make([]string, 0, int(layoutModeCount))
	for l := layoutMode(0); l < layoutModeCount; l++ {
		glyph := glyphRadioOff
		if l == m.layout {
			glyph = glyphRadioOn
		}
		label := l.label()
		if l == layoutCustom {
			// Rows × columns — the same order the prompt asks in.
			label += " " + itoa(clamp(m.gridR, 1, 9)) + "×" + itoa(clamp(m.gridC, 1, 9))
		}
		text := truncate(" "+glyph+" "+label, max(0, innerW))
		if l == m.layout {
			rows = append(rows, on.Render(text))
		} else {
			rows = append(rows, dim.Render(text))
		}
	}
	return panelChrome(innerW, fitLines(rows, innerW, layoutRows-2),
		m.panelTitle(panelLayout), m.focus == panelLayout)
}

// gridView is the terminal grid — or, with nothing on it, one panel that says
// what to do about that (and what just went wrong, if something did).
func (m sshModel) gridView() string {
	gw, gh := m.gridArea()
	shown := m.shownSessions()
	if len(shown) == 0 {
		innerW, innerH := gw-2, gh-2
		body := m.gridEmpty(innerW, innerH)
		if m.failed != nil {
			body = m.failedBody(m.failed, innerW, innerH)
		}
		return panelChrome(innerW, fitLines(body, innerW, innerH), "", false)
	}

	cols, rows := m.gridDims(len(shown))
	ws, hs := splitEven(gw, cols), splitEven(gh, rows)
	out := make([]string, 0, rows)
	for r := 0; r < rows; r++ {
		cells := make([]string, 0, cols)
		for c := 0; c < cols; c++ {
			i := r*cols + c
			if i < len(shown) {
				cells = append(cells, m.cellView(shown[i], i, ws[c], hs[r]))
			} else {
				cells = append(cells, blankBlock(ws[c], hs[r]))
			}
		}
		out = append(out, joinHorizontal(cells...))
	}
	return joinVertical(out...)
}

// cellView is one grid cell: a bordered terminal wearing the chord that
// focuses it.
func (m sshModel) cellView(s *session, i, w, h int) string {
	innerW, innerH := w-2, h-2
	var rows []string
	if s.state == sessLive && !s.pty.hasSpoken() {
		rows = m.connectingBody(s, innerW, innerH)
	} else {
		rows = s.pty.render(innerW, innerH)
	}
	focused := m.focus == panelPty && i == m.focusPty
	return panelChrome(innerW, fitLines(rows, innerW, innerH),
		m.cellTitle(s, i, innerW), focused)
}

// cellTitle names the cell by what it is. It used to lead with an [Alt][N]
// chord; the chord is spatial now (Alt+arrows), so there is no number to
// disclose.
func (m sshModel) cellTitle(s *session, i, innerW int) string {
	t := s.host.User + "@" + s.host.Host
	if tag := s.ordinalTag(); tag != "" {
		t += " " + tag
	}
	return truncate(t, max(0, innerW-2))
}

// blankBlock fills a custom grid's unused cell with canvas, not with an empty
// border — a frame around nothing reads as a broken terminal.
func blankBlock(w, h int) string {
	row := strings.Repeat(" ", max(0, w))
	rows := make([]string, 0, max(0, h))
	for i := 0; i < h; i++ {
		rows = append(rows, row)
	}
	return strings.Join(rows, "\n")
}

// connectingBody is the waiting state: it MOVES, because "is this thing
// stuck" is the question being asked and a still frame is what stuck looks
// like.
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

func (m sshModel) gridEmpty(innerW, innerH int) []string {
	return emptyBody(innerW, innerH, "Nothing on the grid",
		emptyHint("Tab in [1] toggles a session's cell — Enter shows one and takes the keyboard", "[1]"))
}

// failedBody is how a connection ends when it ends badly and nothing else is
// left on the grid: the host, what the far end actually said, and where the
// record is. It stays until something else takes the space.
func (m sshModel) failedBody(s *session, innerW, innerH int) []string {
	who := s.host.User + "@" + s.host.Host
	return emptyBody(innerW, innerH, who+" · "+s.reason,
		emptyHint("The detail is in [Alt+p] logs — or try another host", "[Alt+p]"))
}

// listBody lays out [1]. Each entry is a block, because a long address wraps.
func (m sshModel) listBody(items []*session, cursor, top, innerW, innerH int) []string {
	if len(items) == 0 {
		return emptyBody(innerW, innerH, "No sessions",
			emptyHint("Connect from [Alt+p] hosts", "[Alt+p]"))
	}

	out := make([]string, 0, max(0, innerH))
	for i := top; i < len(items) && len(out) < innerH; i++ {
		out = append(out, m.listItem(items[i], i == cursor, innerW)...)
	}
	return out
}

// listItem lays out one row of [1]:
//
//	<space><display glyph> <user>@<host>:<port>
//
// One string, the way ssh itself would spell it — the port rides the address
// instead of owning a right-aligned slot of its own. The leading glyph is
// the display column — a monitor when this session has a cell on the grid, a
// struck-through one when it does not. Two shapes rather than one shape in
// two colours, so the difference survives any palette.
//
// Colour is still two independent channels: green foreground = on the grid,
// background = the cursor bar. Where they meet the bar wins; the glyph is
// what still tells the state there.
func (m sshModel) listItem(s *session, isCursor bool, innerW int) []string {
	nameFG := textColor
	glyph, glyphFG := glyphMonitorOff, dimColor
	if m.isShown(s.id) {
		nameFG = liveColor
		glyph, glyphFG = glyphMonitor, liveColor
	}

	body := lipgloss.NewStyle().Foreground(nameFG)
	gStyle := lipgloss.NewStyle().Foreground(glyphFG)
	if isCursor {
		bar := lipgloss.NewStyle().Foreground(lipgloss.Color(baseHex)).Background(handColor)
		body, gStyle = bar, bar
	}

	const glyphCell = 2 // the glyph and its trailing space
	nameW := max(1, innerW-sshListLeadW-glyphCell)

	// The address wraps; the ":port" (and the #N tag) ride as ONE unbreakable
	// tail — a port split across lines ("…:2" / "222") is another number, not
	// a shortened one. If the tail cannot share the last line it takes its
	// own, intact.
	tail := ":" + strconv.Itoa(s.host.Port)
	if tag := s.ordinalTag(); tag != "" {
		tail += " " + tag
	}
	lines := wrapText(s.host.User+"@"+s.host.Host, nameW)
	if last := lines[len(lines)-1]; dispW(last)+dispW(tail) <= nameW {
		lines[len(lines)-1] = last + tail
	} else if dispW(tail) <= nameW {
		lines = append(lines, tail)
	} else {
		lines = append(lines, wrapText(tail, nameW)...)
	}

	lead := strings.Repeat(" ", sshListLeadW)
	out := make([]string, 0, len(lines))
	for i, l := range lines {
		g := glyph + " "
		if i > 0 {
			g = "  " // continuation lines indent under the name
		}
		out = append(out, gStyle.Render(lead+g)+body.Render(padRight(l, nameW)))
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
