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
	// Port and Auth are fixed: "65535" and the longer of the two auth names.
	colPortW = 5
	colAuthW = 12 // glyph + space + "privatekey"
	colGap   = 2

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
			// Share out by weight; the name gets the most because it is what the
			// user picked the host by.
			c.name = max(minNameW, free*35/100)
			c.host = max(minHostW, free*40/100)
			c.user = max(minUserW, free-c.name-c.host)
			// Weights can overshoot after the minimums bite; trim the host column,
			// which degrades most gracefully (a truncated domain still reads).
			if over := c.name + c.user + c.host - free; over > 0 {
				c.host = max(minHostW, c.host-over)
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
	return dim.Render(padRight(" "+tableRowText(c, "Name", "User", "Host", "Port", "Auth", ""), w))
}

// tableRowText lays out one row's cells at the current column widths. Both the
// header and the data rows go through it, so they cannot drift apart.
func tableRowText(c tableCols, name, user, host, port, auth, authGlyph string) string {
	gap := strings.Repeat(" ", colGap)
	out := padRight(name, c.name)
	if c.user > 0 {
		out += gap + padRight(user, c.user)
	}
	if c.host > 0 {
		out += gap + padRight(host, c.host)
	}
	if c.port {
		out += gap + padLeft(port, colPortW)
	}
	if c.auth {
		cell := auth
		if authGlyph != "" {
			cell = authGlyph + " " + auth
		}
		out += gap + padRight(cell, colAuthW)
	}
	return out
}

// renderHostRow draws one host. The cursor is a filled bar — the same cursor
// form as every other list in the app, which a table can finally use because a
// row is one line tall (a six-row card could not, §2.3).
func renderHostRow(h store.Host, c tableCols, selected bool, w int) string {
	authGlyph, authText := glyphLock, string(store.AuthPassword)
	if h.Auth == store.AuthPrivateKey {
		authGlyph, authText = glyphKey, string(store.AuthPrivateKey)
	}
	plain := " " + tableRowText(c, h.Name, h.User, h.Host,
		strconv.Itoa(h.Port), authText, authGlyph)

	if selected {
		bar := lipgloss.NewStyle().Foreground(lipgloss.Color(baseHex)).Background(rowSelColor)
		return bar.Render(padRight(plain, w))
	}

	// Unselected: the name stays readable so the list can be scanned, the detail
	// columns recede. Same lit-versus-unlit reading the cards had.
	txt := lipgloss.NewStyle().Foreground(textColor)
	dim := lipgloss.NewStyle().Foreground(dimColor)
	rest := tableRowText(tableCols{user: c.user, host: c.host, port: c.port, auth: c.auth},
		"", h.User, h.Host, strconv.Itoa(h.Port), authText, authGlyph)
	styled := " " + txt.Render(padRight(h.Name, c.name)) + dim.Render(rest)
	return styled + strings.Repeat(" ", max(0, w-1-c.name-dispW(rest)))
}
