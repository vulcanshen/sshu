package ui

import (
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/vulcanshen/sshu/internal/store"
)

// Panel [1] is a table, not a grid of cards. Cards read better one at a time,
// but a host list is something you scan and compare down a column — and a card
// costs six rows where a row costs one, so a real list of hosts stopped fitting
// on the screen.
//
// Columns: Name, User, Host, Port, Auth.
const (
	// Port is fixed at the width of the largest port there is, so the column
	// neither grows nor shrinks with what happens to be in the list.
	colPortW = 5
	// Auth here is NOT the same column as the credentials list's, even though
	// both are headed "Auth" and both once shared this number. That one shows a
	// method and nothing else, so twelve cells is exactly enough forever. This
	// one also has to say WHICH credential — a name the user chose, and the only
	// cell in this table whose content nobody can bound — so it gets room past
	// the longest method name instead of being sized by it.
	colAuthW  = 16
	credAuthW = 12 // glyph + space + "privatekey", and never anything else
	colGap    = 2

	// Minimums below which a column stops carrying information and is dropped
	// instead of being shaved to nothing.
	minNameW = 8
	minUserW = 6
	minHostW = 10
)

// tableCols is the width given to each column at this panel width, and which
// columns survive. Columns are dropped from the least load-bearing end: Auth
// first, then Port, then User — the name is the last thing standing, because a
// row you cannot name is not a row.
type tableCols struct {
	name, user, host int
	port, auth       bool
}

func computeCols(w int) tableCols {
	c := tableCols{port: true, auth: true}
	avail := w - 2 // one cell of padding each side

	for {
		fixed := 0
		gaps := 2 // name|user|host
		if c.port {
			fixed += colPortW
			gaps++
		}
		if c.auth {
			fixed += colAuthW
			gaps++
		}
		free := avail - fixed - gaps*colGap

		if free >= minNameW+minUserW+minHostW {
			// Share out by weight, and the name really does get the most: it is
			// what the user picked the host by, and the one column they scan
			// rather than read.
			c.name = max(minNameW, free*45/100)
			c.host = max(minHostW, free*35/100)
			c.user = max(minUserW, free-c.name-c.host)
			// Weights can overshoot once the minimums bite, and the row must
			// still come to exactly free — a column over budget pushes the
			// panel's right border out of line. Give back from the host first,
			// which degrades most gracefully (a truncated domain still reads),
			// and only then from the name.
			if over := c.name + c.user + c.host - free; over > 0 {
				give := min(over, c.host-minHostW)
				c.host -= give
				if over -= give; over > 0 {
					c.name = max(minNameW, c.name-over)
				}
			}
			return c
		}
		switch {
		case c.auth:
			c.auth = false
		case c.port:
			c.port = false
		default:
			// Nothing left to drop: give everything to the name.
			c.name, c.user, c.host = max(1, avail), 0, 0
			return c
		}
	}
}

// tableHeader names the columns. Dim, because it is a label and never the thing
// being read.
func tableHeader(c tableCols, w int) string {
	dim := lipgloss.NewStyle().Foreground(dimColor)
	return dim.Render(padRight(" "+tableCells(c, "Name", "User", "Host", "Port", "Auth", "").plain(), w))
}

// rowCells is one row laid out at the current column widths: every cell padded
// to its column, and empty string for a column that was dropped.
//
// The cells come back apart rather than as a finished line because a data row
// now colours each column on its own, while the header and the selected row
// still want one tone across the whole thing. All three go through this same
// layout, so they cannot drift.
type rowCells struct {
	name, user, host, port, auth string
}

// tableCells lays out one row at the current column widths.
func tableCells(c tableCols, name, user, host, port, auth, authGlyph string) rowCells {
	r := rowCells{name: padRight(name, c.name)}
	if c.user > 0 {
		r.user = padRight(user, c.user)
	}
	if c.host > 0 {
		r.host = padRight(host, c.host)
	}
	if c.port {
		// Left, like every other column. Right-aligning numbers is the habit
		// from columns you add up; nobody adds up ports, and the alignment made
		// the one fixed-width column in the table look like the one that moved.
		r.port = padRight(port, colPortW)
	}
	if c.auth {
		cell := auth
		if authGlyph != "" {
			cell = authGlyph + " " + auth
		}
		r.auth = padRight(cell, colAuthW)
	}
	return r
}

// plain joins the cells at the column gap with no styling — what the header and
// the selected row both need, since neither colours per column.
func (r rowCells) plain() string {
	gap := strings.Repeat(" ", colGap)
	out := r.name
	for _, cell := range []string{r.user, r.host, r.port, r.auth} {
		if cell == "" {
			continue
		}
		out += gap + cell
	}
	return out
}

// portStyle is the one colour on an unselected row.
//
// 22 is the answer nobody needs to check, so it recedes. Anything else is a
// deliberate choice somebody made, and in a list of twenty hosts the two that
// are not 22 are exactly what a glance should land on — so the exception gets
// the colour and the rule does not. Peach and not red: nothing is wrong here,
// it is just not the usual answer (§11.48).
func portStyle(port int) lipgloss.Style {
	// 0 is an sshconfig host leaving the port to the file: nothing here to
	// flag either, the answer is just somewhere else.
	if port == store.DefaultPort || port == 0 {
		return lipgloss.NewStyle().Foreground(dimColor)
	}
	return lipgloss.NewStyle().Foreground(peachColor)
}

// portUnset is the port cell of a host that lets ssh decide.
const portUnset = "—"

// tagNone stands in when a host has no tags. A placeholder rather than a blank
// line: the second line is part of the entry's shape, and leaving it empty made
// a tagged host look like it had grown something rather than filled something
// in.
const tagNone = "—"

// tagLineText is a host's second line: its tags, or the placeholder. Indented
// past the first column so it reads as belonging to the row above rather than
// as a row of its own, and truncated with the same ellipsis as every other
// overlong cell in the app.
func tagLineText(tags []string, w int) string {
	body := glyphTag + " " + tagNone
	if len(tags) > 0 {
		body = glyphTag + " " + strings.Join(tags, " ")
	}
	return "  " + truncate(body, max(0, w-2))
}

// hostRowLines is how many screen lines one host occupies. ALWAYS two, tags or
// not.
//
// A variable row height would turn every scroll calculation into an
// accumulation — visibleRows, ensureVisible and the half-page jump all count
// entries today — and would make the list shift under the cursor as it moved
// between tagged and untagged hosts. The empty-ish second line is not waste
// either: it separates the entries, which a dense one-line table never did.
const hostRowLines = 2

// renderHostRow draws one host as its two lines. The cursor is a filled bar
// across both — the same cursor form as every other list in the app.
func renderHostRow(h store.Host, user string, c tableCols, selected bool, w int) []string {
	authGlyph, authText := glyphLock, string(store.AuthPassword)
	switch h.Auth {
	case store.AuthPrivateKey:
		authGlyph, authText = glyphKey, string(store.AuthPrivateKey)
	case store.AuthCredential:
		// The NAME is the information — which credential, not just that one is
		// in play. The glyph carries the kind.
		authGlyph, authText = glyphCred, truncate(h.Credential, colAuthW-2)
	case store.AuthSSHConfig:
		// The file's glyph, because the file is what answers for this host.
		authGlyph, authText = glyphFileCog, string(store.AuthSSHConfig)
	}
	port := strconv.Itoa(h.Port)
	if h.Port == 0 {
		port = portUnset // ssh decides — a dash, not a 0 nobody will dial
	}
	cells := tableCells(c, h.Name, user, h.Host, port, authText, authGlyph)
	tagText := tagLineText(h.Tags, w)

	if selected {
		// One bar over both lines, and NOTHING keeps its colour. The per-column
		// tones below answer "what kind of value is this"; the bar answers "you
		// are here". A row cannot carry both without the second one winning, so
		// the selected row drops the first entirely.
		bar := lipgloss.NewStyle().Foreground(lipgloss.Color(baseHex)).Background(rowSelColor)
		return []string{
			bar.Render(padRight(" "+cells.plain(), w)),
			bar.Render(padRight(tagText, w)),
		}
	}

	// Unselected. Name, user and host share one tone because they are one
	// thing — which machine is this — and splitting them into a brightness
	// ranking only asserted that host outranks user, which is not true. The
	// single exception is the port, and only when it is not 22 (portStyle).
	txt := lipgloss.NewStyle().Foreground(textColor)
	dim := lipgloss.NewStyle().Foreground(dimColor)
	gap := strings.Repeat(" ", colGap)

	line := " " + txt.Render(cells.name)
	for _, cell := range []string{cells.user, cells.host} {
		if cell == "" {
			continue
		}
		line += gap + txt.Render(cell)
	}
	if cells.port != "" {
		line += gap + portStyle(h.Port).Render(cells.port)
	}
	if cells.auth != "" {
		// Auth stays dim behind its glyph. Colour here would compete with the
		// port for the one signal an unselected row is allowed to raise, and
		// every row has an auth — a colour that marks every row marks none.
		line += gap + dim.Render(cells.auth)
	}
	line += strings.Repeat(" ", max(0, w-dispW(" "+cells.plain())))

	return []string{line, dim.Render(tagText) + strings.Repeat(" ", max(0, w-dispW(tagText)))}
}
