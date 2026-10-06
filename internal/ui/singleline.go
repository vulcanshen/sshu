package ui

import (
	"strings"
	"unicode"

	"github.com/charmbracelet/lipgloss"
)

// A single-line value — a form field, the input box, ssh's password question,
// a search — has no use for a line break or a tab, and both arrive by paste:
// one KeyRunes holds a whole bracketed paste, line breaks, tabs and escapes
// included. Every single-line value in the terminu family keeps and draws them
// the same way (the input survey, 2026-10-06): they stay in the value, drawn as
// a Red \n / \t, and a value that gets used is refused while it holds one.

// singleLine is what a single-line value keeps of text that arrives as runes —
// typed, pasted or prefilled. A line break or a tab stays, \r\n as one \n, so
// the value is what the user gave and Backspace takes a break whole; every
// other control character (C0, DEL, C1) is dropped, so an ESC in a remote file
// name never reaches the terminal.
func singleLine(s string) string {
	return strings.Map(func(r rune) rune {
		if r != '\n' && r != '\r' && r != '\t' && unicode.IsControl(r) {
			return -1
		}
		return r
	}, strings.ReplaceAll(s, "\r\n", "\n"))
}

// hasBreak reports whether a value holds a line break or a tab — what a value
// that gets used cannot be submitted with. It is asked of the value as typed,
// before any trim: a trim would drop one at either end unseen and send the rest.
func hasBreak(s string) bool { return strings.ContainsAny(s, "\n\r\t") }

// breakErr is the refusal, naming the value it is about.
func breakErr(what string) string { return what + " cannot have line breaks or tabs" }

// valueUnit is one rune of a value as it is drawn. A line break is drawn as \n
// and a tab as \t: two cells, never cut, and marked, so they read apart from a
// \ and an n typed by hand.
type valueUnit struct {
	s   string
	esc bool
}

func valueUnits(v string) []valueUnit {
	units := make([]valueUnit, 0, len(v))
	for _, r := range v {
		switch r {
		case '\n', '\r':
			units = append(units, valueUnit{`\n`, true})
		case '\t':
			units = append(units, valueUnit{`\t`, true})
		default:
			units = append(units, valueUnit{s: string(r)})
		}
	}
	return units
}

func unitsW(units []valueUnit) int {
	w := 0
	for _, u := range units {
		w += dispW(u.s)
	}
	return w
}

// cutUnits is v in at most w cells, cut at the end the way truncate cuts: the
// last cell is "…", and a unit that would cross it is left out whole, its cells
// kept as spaces. tail is the spaces and the "…", or "" when v fits.
func cutUnits(v string, w int) (units []valueUnit, tail string) {
	units = valueUnits(v)
	if unitsW(units) <= w {
		return units, ""
	}
	if w <= 0 {
		return nil, ""
	}
	used, n := 0, 0
	for n < len(units) && used+dispW(units[n].s) <= w-1 {
		used += dispW(units[n].s)
		n++
	}
	return units[:n], strings.Repeat(" ", w-1-used) + "…"
}

// drawUnits renders units with each \n and \t in Red and everything else in
// style, one style span per run rather than one per rune.
func drawUnits(units []valueUnit, style lipgloss.Style) string {
	mark := lipgloss.NewStyle().Foreground(warnColor)
	var b, run strings.Builder
	flush := func() {
		if run.Len() > 0 {
			b.WriteString(style.Render(run.String()))
			run.Reset()
		}
	}
	for _, u := range units {
		if u.esc {
			flush()
			b.WriteString(mark.Render(u.s))
			continue
		}
		run.WriteString(u.s)
	}
	flush()
	return b.String()
}

// valueText draws v in at most w cells, cut at the end like truncate, with each
// line break and tab a Red \n / \t; style colours the rest.
func valueText(v string, w int, style lipgloss.Style) string {
	units, tail := cutUnits(v, w)
	if tail == "" {
		return drawUnits(units, style)
	}
	return drawUnits(units, style) + style.Render(tail)
}

// valuePlain is valueText unstyled, for a row drawn in one colour — a search
// row nobody is typing into is grey whole, its \n and \t included, coloured
// once.
func valuePlain(v string, w int) string {
	units, tail := cutUnits(v, w)
	var b strings.Builder
	for _, u := range units {
		b.WriteString(u.s)
	}
	return b.String() + tail
}
