package ui

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/vulcanshen/sshu/internal/store"
)

// hostsModel is tab [1]: a table over hosts.yaml. It is the only panel in its
// tab, so it is always focused — there is no unfocused state to render.
//
// It started as a grid of cards. Cards read beautifully one at a time, but a
// host list is scanned and compared down a column, and six rows per host meant
// a real list stopped fitting on screen. The table keeps the same fields
// (Name, User, Host, Port, Auth) at one row each.
type hostsModel struct {
	hosts  []store.Host
	cursor int
	top    int // first visible host
	w, h   int // panel outer size

	// `/` narrows the table. matches indexes into hosts, so everything
	// downstream keeps working against the real entry — a filtered row is the
	// same host, reached by a different index.
	filtering bool
	query     string
	matches   []int
}

// headerRows is the column-name row the list is offset by.
const headerRows = 1

func (m *hostsModel) setSize(w, h int) {
	m.w, m.h = w, h
	m.ensureVisible()
}

// visibleRows is how many host rows fit under the header.
func (m hostsModel) visibleRows() int { return max(1, m.h-2-headerRows) }

// rowCount is how many rows the table is showing.
func (m hostsModel) rowCount() int {
	if m.filtering {
		return len(m.matches)
	}
	return len(m.hosts)
}

// rowAt maps a row on screen back to the host behind it.
func (m hostsModel) rowAt(i int) (store.Host, bool) {
	if i < 0 || i >= m.rowCount() {
		return store.Host{}, false
	}
	if m.filtering {
		return m.hosts[m.matches[i]], true
	}
	return m.hosts[i], true
}

// startFilter opens the query. The cursor goes to the top, because after the
// list is rebuilt wherever it pointed is not where anyone is looking.
func (m *hostsModel) startFilter() {
	m.filtering, m.query = true, ""
	m.cursor, m.top = 0, 0
	m.refilter()
}

// clearFilter drops the query and LANDS THE CURSOR ON THE SAME HOST. Searching
// for something and then losing it on the way out is worse than not searching:
// on this tab the letter actions are typed into the query (§4.5), so
// "search → Esc → act" is the whole point of the feature.
func (m *hostsModel) clearFilter() {
	at := 0
	if i, ok := m.cursorIndex(); ok {
		at = i
	}
	m.filtering, m.query, m.matches = false, "", nil
	m.cursor = clamp(at, 0, max(0, m.rowCount()-1))
	m.ensureVisible()
}

// cursorIndex is where the cursor points in the UNFILTERED list.
func (m hostsModel) cursorIndex() (int, bool) {
	if !m.filtering {
		return m.cursor, m.cursor < len(m.hosts)
	}
	if m.cursor < 0 || m.cursor >= len(m.matches) {
		return 0, false
	}
	return m.matches[m.cursor], true
}

// refilter reruns the match.
//
// The haystack is the row's identifying fields joined — name, user, host, port —
// and NOT the auth method: "password" and "privatekey" are two words shared by
// most of the table, so matching them would drag in rows that have nothing to do
// with what was typed. Joining rather than testing each field separately is what
// lets a query span columns, so "prod 22" finds prod-web-01 on port 22.
func (m *hostsModel) refilter() {
	type hit struct{ i, score int }
	var hits []hit
	for i, h := range m.hosts {
		if s, ok := fuzzyScore(hostHaystack(h), m.query); ok {
			hits = append(hits, hit{i, s})
		}
	}
	// Best first. A subsequence over a joined haystack is permissive — "prod"
	// is also somewhere inside db-replica-tokyo-ap-northeast-1 — so ranking is
	// what keeps that from mattering: the one you meant is row 0.
	//
	// Tab [2] deliberately does not rank, and that is not an inconsistency:
	// there the results arrive while the cursor is live in the same list, so
	// re-sorting each batch would move the row out from under the user's hand.
	// Here the list is complete on every keystroke and the cursor is already
	// being sent back to the top.
	sort.SliceStable(hits, func(a, b int) bool { return hits[a].score > hits[b].score })

	m.matches = m.matches[:0]
	for _, h := range hits {
		m.matches = append(m.matches, h.i)
	}
	m.cursor = clamp(m.cursor, 0, max(0, len(m.matches)-1))
	m.ensureVisible()
}

func hostHaystack(h store.Host) string {
	return h.Name + " " + h.User + " " + h.Host + " " + strconv.Itoa(h.Port)
}

// filterKey edits the query. Letters type and arrows move — the same split the
// picker, the form and tab [2] make, so there is no mode to learn here either
// (§4.5). It reports whether it consumed the key.
func (m *hostsModel) filterKey(msg tea.KeyMsg) bool {
	if !m.filtering {
		return false
	}
	switch msg.Type {
	case tea.KeyRunes:
		m.query += string(msg.Runes)
	case tea.KeySpace:
		m.query += " "
	case tea.KeyBackspace:
		if r := []rune(m.query); len(r) > 0 {
			m.query = string(r[:len(r)-1])
		} else {
			m.clearFilter()
			return true
		}
	default:
		return false // arrows, Enter and Esc belong to the panel
	}
	m.cursor, m.top = 0, 0
	m.refilter()
	return true
}

// ensureVisible scrolls the viewport so the cursor is on screen.
func (m *hostsModel) ensureVisible() {
	if m.rowCount() == 0 || m.w == 0 {
		m.top = 0
		return
	}
	vis := m.visibleRows()
	if m.cursor < m.top {
		m.top = m.cursor
	}
	if m.cursor >= m.top+vis {
		m.top = m.cursor - vis + 1
	}
	m.top = max(0, m.top)
}

// handleKey runs the panel's navigation. A table has no columns to move
// between, so h/l are unbound here — j/k walk the rows and u/d take half a page.
//
// It goes through the shared moveCursor rather than repeating the switch, so a
// key added to the vocabulary lands on every list at once (nav.go).
func (m *hostsModel) handleKey(k string) {
	if m.rowCount() == 0 {
		return
	}
	m.cursor = moveCursor(m.cursor, m.rowCount(), k, m.visibleRows())
	m.ensureVisible()
}

// status fills the right-hand slot of the tab row (§1.1).
func (m hostsModel) status() string {
	if len(m.hosts) == 0 {
		return "no hosts"
	}
	return fmt.Sprintf("%d/%d hosts", m.cursor+1, len(m.hosts))
}

func (m hostsModel) view() string {
	innerW, innerH := m.w-2, m.h-2
	body := m.tableBody(innerW, innerH)
	switch {
	case len(m.hosts) == 0:
		body = m.emptyState(innerW, innerH)
	case m.filtering && m.rowCount() == 0:
		// Not the first-run state: there ARE hosts, none of them match. Saying
		// "no hosts yet" here would be a different and wrong thing to say — and
		// the query row stays, because it is what you would edit next.
		body = append([]string{m.filterRow(innerW)},
			emptyBody(innerW, innerH-1, "No match", nil)...)
	}
	// No title. A panel title exists to tell panels APART, and this tab has one
	// panel — the capsule would have said "hosts" directly under a tab capsule
	// already reading "[1] hosts", which is a label answering a question nobody
	// could have.
	return panelChrome(innerW, fitLines(body, innerW, innerH), "", true)
}

func (m hostsModel) tableBody(innerW, innerH int) []string {
	c := computeCols(innerW)
	out := make([]string, 0, max(0, innerH))

	// While filtering the query takes the header's row rather than pushing the
	// table down: the two answer the same question ("what am I looking at"), and
	// sharing one slot keeps the row count fixed (§1.3).
	if m.filtering {
		out = append(out, m.filterRow(innerW))
	} else {
		out = append(out, tableHeader(c, innerW))
	}

	for i := m.top; i < m.rowCount() && len(out) < innerH; i++ {
		h, ok := m.rowAt(i)
		if !ok {
			break
		}
		out = append(out, renderHostRow(h, c, i == m.cursor, innerW))
	}
	return out
}

// filterRow is the query line: the search glyph, what has been typed, and how
// many of the hosts it leaves. The glyph rather than a literal "/" for the same
// reason tab [2] uses one — echoing the key that opened the search makes a query
// containing that character unreadable.
func (m hostsModel) filterRow(w int) string {
	hand := lipgloss.NewStyle().Foreground(handColor)
	cur := lipgloss.NewStyle().Foreground(lipgloss.Color(baseHex)).Background(handColor)
	dim := lipgloss.NewStyle().Foreground(dimColor)

	q := truncate(glyphSearch+" "+m.query, max(0, w-2))
	used := 2 + dispW(q)
	note := fmt.Sprintf("%d of %d", len(m.matches), len(m.hosts))
	if dispW(note)+2 > w-used {
		note = ""
	}
	row := " " + hand.Render(q) + cur.Render(" ") +
		strings.Repeat(" ", max(0, w-used-dispW(note))) + dim.Render(note)
	return clipANSI(row, w)
}

// emptyState is the first-run state. It MUST disclose both [A] and Space: a new
// user facing an empty panel with no visible way forward is where
// discoverability dies (§1.5).
func (m hostsModel) emptyState(innerW, innerH int) []string {
	return emptyBody(innerW, innerH, "No hosts yet",
		emptyHint("Press [A] to add a host, or Space to see what you can do here",
			"[A]", "Space"))
}

// centerLine centres styled within innerW, measuring plain (styled carries ANSI).
func centerLine(innerW int, plain, styled string) string {
	w := dispW(plain)
	if w >= innerW {
		return padRight(plain, innerW)
	}
	left := (innerW - w) / 2
	return strings.Repeat(" ", left) + styled + strings.Repeat(" ", innerW-left-w)
}

// fitLines forces body to exactly h lines, each at least w cells — the
// invariant panelChrome relies on to keep its right border straight. Padding only:
// a styled line is full of ANSI and blind truncation would cut an escape
// sequence in half, so over-long lines are the builders' job to prevent (and the
// width test's job to catch).
func fitLines(body []string, w, h int) []string {
	if len(body) > h {
		body = body[:max(0, h)]
	}
	out := make([]string, 0, max(0, h))
	for _, l := range body {
		out = append(out, l+strings.Repeat(" ", max(0, w-dispW(l))))
	}
	blank := strings.Repeat(" ", max(0, w))
	for len(out) < h {
		out = append(out, blank)
	}
	return out
}
