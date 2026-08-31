package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/vulcanshen/sshu/internal/remote"
)

// Panel numbers as they appear in the border titles. A number the screen does
// not show is a number the keyboard does not answer to (§4.4), so these and the
// digit bindings are one list.
var sftpPanelNum = map[sftpPanel]string{
	panelLeftFiles: "[4]", panelLeftMarks: "[5]",
	panelRightFiles: "[6]", panelRightMarks: "[7]",
}

// panelTitle is what panel p's capsule says — and what the Space menu calls the
// surface it was opened on. One function for both, so the menu can never
// disagree with the frame the user is looking at: in a split tab "what can I do
// here" depends on WHICH panel, and a title naming the tab cannot tell [4] from
// [6].
//
// No glyph. Every panel title in the app is "[N] label" and nothing more.
func (m sftpModel) panelTitle(p sftpPanel) string {
	n := sftpPanelNum[p]
	if p.isMarks() {
		return n + " Marked files"
	}
	if h := m.sides[p.side()].host; h != "" {
		return n + " " + h
	}
	return n + " no host"
}

func (m sftpModel) view() string {
	sideW, filesH, marksH := m.panes()

	if m.narrow() {
		// One side at a time; Tab is how the other one is reached.
		return m.sideView(m.focus.side(), m.w, filesH, marksH)
	}
	left := m.sideView(sideLeft, sideW, filesH, marksH)
	right := m.sideView(sideRight, m.w-sideW, filesH, marksH)
	return joinHorizontal(left, right)
}

func (m sftpModel) sideView(sd side, w, filesH, marksH int) string {
	files := m.filesPanel(sd, w, filesH)
	if marksH == 0 {
		return files
	}
	return joinVertical(files, m.marksPanel(sd, w, marksH))
}

// filesPanel is [4] / [6]. Its title carries the host — that is what tells the
// two sides apart — and its bottom border carries the directory, which is the
// other thing you need to know before pressing anything.
func (m sftpModel) filesPanel(sd side, w, h int) string {
	s := m.sides[sd]
	innerW, innerH := w-2, h-2
	focused := m.focus.side() == sd && !m.focus.isMarks()

	panel := panelLeftFiles
	if sd == sideRight {
		panel = panelRightFiles
	}

	var rows []string
	switch {
	case s.fs == nil:
		rows = m.noHostBody(innerW, innerH)
	case s.err != "" && len(s.entries) == 0:
		rows = []string{lipgloss.NewStyle().Foreground(warnColor).
			Render(padRight("  "+s.err, innerW))}
	default:
		// The directory goes at the TOP of the panel, above the listing (filu's
		// placement). A border hint reads as chrome; this is content — it is the
		// answer to "where am I", which you check as often as the listing itself.
		//
		// While filtering, the same row carries the query: the two answer the same
		// question ("what am I looking at"), so they share the slot rather than
		// one pushing the other down and shifting every row below.
		var head string
		if s.filtering {
			head = searchRow(s, innerW)
		} else {
			head = renderCrumb(foldHomePath(s.cwd, s.home), innerW-1)
			head = " " + head + strings.Repeat(" ", max(0, innerW-1-dispW(head)))
		}
		rows = append([]string{head}, m.fileRows(s, innerW, innerH-sftpCwdRows)...)
	}
	return panelChrome(innerW, fitLines(rows, innerW, innerH), m.panelTitle(panel), focused)
}

func (m sftpModel) fileRows(s sftpSideModel, innerW, innerH int) []string {
	n := s.rowCount()
	if n == 0 {
		note := "  (empty)"
		switch {
		case s.filtering && s.scanning:
			// Nothing yet is not nothing at all while the walk is still going.
			note = "  searching…"
		case s.filtering:
			note = "  no match"
		}
		return []string{lipgloss.NewStyle().Foreground(dimColor).Render(padRight(note, innerW))}
	}
	out := make([]string, 0, max(0, innerH))
	for row := s.top; row < n && len(out) < innerH; row++ {
		e, ok := s.rowAt(row)
		if !ok {
			break
		}
		p := remote.Join(s.cwd, e.Name)
		out = append(out, renderFileRow(e, s.markedSet[p], row == s.cursor, innerW))
	}
	return out
}

// searchRow takes the path line's place while a search is running: the query on
// the left, and on the right how much of what has been seen is on screen. Both
// answer "what am I looking at", so they share the one slot rather than one
// pushing the other down and shifting every row below it.
//
// The query is what must survive a narrow panel; the count is dropped first.
func searchRow(s sftpSideModel, w int) string {
	hand := lipgloss.NewStyle().Foreground(handColor)
	cur := lipgloss.NewStyle().Foreground(lipgloss.Color(baseHex)).Background(handColor)
	dim := lipgloss.NewStyle().Foreground(dimColor)

	q := truncate(s.filterLabel(), max(0, w-2))
	used := 2 + dispW(q) // the leading space and the block cursor
	note := s.scanNote()
	if dispW(note)+2 > w-used {
		note = ""
	}
	row := " " + hand.Render(q) + cur.Render(" ") +
		strings.Repeat(" ", max(0, w-used-dispW(note))) + dim.Render(note)
	return clipANSI(row, w)
}

// marksPanel is [5] / [7]. It lists what has been marked on its own side, as
// paths relative to nothing — the full path, because a mark made three
// directories ago has to be recognisable from here.
func (m sftpModel) marksPanel(sd side, w, h int) string {
	s := m.sides[sd]
	innerW, innerH := w-2, h-2
	focused := m.focus.side() == sd && m.focus.isMarks()

	panel := panelLeftMarks
	if sd == sideRight {
		panel = panelRightMarks
	}

	var rows []string
	if len(s.marks) == 0 {
		rows = []string{lipgloss.NewStyle().Foreground(dimColor).
			Render(padRight("  (none)", innerW))}
	} else {
		rows = make([]string, 0, max(0, innerH))
		for i := s.markTop; i < len(s.marks) && len(rows) < innerH; i++ {
			rows = append(rows, renderMarkRow(s.marks[i], s.cwd, i == s.markCur, innerW))
		}
	}
	return panelChrome(innerW, fitLines(rows, innerW, innerH), m.panelTitle(panel), focused)
}

func (m sftpModel) noHostBody(innerW, innerH int) []string {
	dim := lipgloss.NewStyle().Foreground(dimColor)
	key := lipgloss.NewStyle().Foreground(handColor)
	plain := "Press [S] to select a host, or local"
	line := centerLine(innerW, plain,
		dim.Render("Press ")+key.Render("[S]")+dim.Render(" to select a host, or ")+
			key.Render("local"))

	blank := strings.Repeat(" ", innerW)
	out := make([]string, 0, max(0, innerH))
	for i := 0; i < max(0, (innerH-1)/2); i++ {
		out = append(out, blank)
	}
	return append(out, line)
}

// ------------------------------------------------------------------- rows

// sftpSizeW is the fixed right-hand slot for a size, so the name wraps against
// it rather than pushing it off (the same rule as the port in [4] of tab [3]).
const sftpSizeW = 8

// The row is built by MEASURING its fixed part rather than assuming each glyph
// is one cell. Nerd Font glyphs do not all measure the same — a folder icon and
// a file icon can disagree by a cell — and a row built on the assumption comes
// out a cell wide only for directories, which is exactly the kind of drift that
// shows up as a bent border rather than as an obvious bug.
func renderFileRow(e remote.Entry, marked, isCursor bool, w int) string {
	glyph, glyphC := glyphFile, dimColor
	if e.IsDir {
		glyph, glyphC = glyphDir, focusColor
	}
	mark := " "
	if marked {
		mark = glyphMark
	}
	size := humanSize(e.Size)
	if e.IsDir {
		size = "-"
	}

	lead := " " + mark + " " + glyph + " "
	nameW := max(1, w-dispW(lead)-1-sftpSizeW)
	// A search result's name is its path relative to the directory, so it is
	// shortened the way every other path in the app is: leading segments give way
	// first, the basename last. A plain name has no separators and comes out of
	// fitPath truncated, exactly as before.
	name := padRight(fitPath(e.Name, nameW), nameW)
	sizeCell := padLeft(size, sftpSizeW)

	if isCursor {
		bar := lipgloss.NewStyle().Foreground(lipgloss.Color(baseHex)).Background(rowSelColor)
		return bar.Render(lead + name + " " + sizeCell)
	}
	markStyle := lipgloss.NewStyle().Foreground(liveColor) // a mark is a footprint you left
	return " " + markStyle.Render(mark) + " " +
		lipgloss.NewStyle().Foreground(glyphC).Render(glyph) + " " +
		lipgloss.NewStyle().Foreground(textColor).Render(name) + " " +
		lipgloss.NewStyle().Foreground(dimColor).Render(sizeCell)
}

// renderMarkRow shows a mark by its path. The path is shortened the same way the
// cwd is, so a mark made three directories ago is still recognisable from here.
func renderMarkRow(p, cwd string, isCursor bool, w int) string {
	label := p
	if rel := strings.TrimPrefix(p, cwd+"/"); rel != p && cwd != "" {
		label = rel
	}
	lead := "  " + glyphMark + " "
	labelW := max(1, w-dispW(lead))
	label = padRight(fitPath(label, labelW), labelW)

	if isCursor {
		bar := lipgloss.NewStyle().Foreground(lipgloss.Color(baseHex)).Background(rowSelColor)
		return bar.Render(lead + label)
	}
	return "  " + lipgloss.NewStyle().Foreground(liveColor).Render(glyphMark) + " " +
		lipgloss.NewStyle().Foreground(textColor).Render(label)
}
