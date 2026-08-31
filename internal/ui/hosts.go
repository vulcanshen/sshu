package ui

import (
	"fmt"
	"strings"

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
}

// headerRows is the column-name row the list is offset by.
const headerRows = 1

func (m *hostsModel) setSize(w, h int) {
	m.w, m.h = w, h
	m.ensureVisible()
}

// visibleRows is how many host rows fit under the header.
func (m hostsModel) visibleRows() int { return max(1, m.h-2-headerRows) }

// ensureVisible scrolls the viewport so the cursor is on screen.
func (m *hostsModel) ensureVisible() {
	if len(m.hosts) == 0 || m.w == 0 {
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
	if len(m.hosts) == 0 {
		return
	}
	m.cursor = moveCursor(m.cursor, len(m.hosts), k, m.visibleRows())
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
	if len(m.hosts) == 0 {
		body = m.emptyBody(innerW, innerH)
	}
	// A capsule here repeats the tab above it, which is mild redundancy — but
	// every other panel below the rule wears one, and a single bare box in the
	// set reads as an unfinished frame rather than as a deliberate exception.
	return panelChrome(innerW, fitLines(body, innerW, innerH), "hosts", true)
}

func (m hostsModel) tableBody(innerW, innerH int) []string {
	c := computeCols(innerW)
	out := make([]string, 0, max(0, innerH))
	out = append(out, tableHeader(c, innerW))
	for i := m.top; i < len(m.hosts) && len(out) < innerH; i++ {
		out = append(out, renderHostRow(m.hosts[i], c, i == m.cursor, innerW))
	}
	return out
}

// emptyBody is the first-run state. It MUST disclose both [A] and Space: a new
// user facing an empty panel with no visible way forward is where
// discoverability dies (§1.5).
func (m hostsModel) emptyBody(innerW, innerH int) []string {
	dim := lipgloss.NewStyle().Foreground(dimColor)
	key := lipgloss.NewStyle().Foreground(handColor)

	titlePlain := "No hosts yet"
	title := centerLine(innerW, titlePlain, dim.Render(titlePlain))
	hintPlain := "Press [A] to add a host, or Space to see what you can do here"
	hint := centerLine(innerW, hintPlain,
		dim.Render("Press ")+key.Render("[A]")+dim.Render(" to add a host, or ")+
			key.Render("Space")+dim.Render(" to see what you can do here"))

	blank := strings.Repeat(" ", innerW)
	out := make([]string, 0, max(0, innerH))
	for i := 0; i < max(0, (innerH-3)/2); i++ {
		out = append(out, blank)
	}
	return append(out, title, blank, hint)
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
