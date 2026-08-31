package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// Everything drawn into a fixed slot goes through these. The card is a
// fixed-width box (§1.2), so a field that miscounts its own width does not
// merely look off — it pushes the right border out and breaks the frame.
//
// Rule: measure and pad PLAIN text, then apply the style. lipgloss.Width knows
// how to skip ANSI, but padding a styled string means the pad lands inside the
// styled span and picks up its background.

// dispW is the terminal cell width of s.
func dispW(s string) int { return lipgloss.Width(s) }

// truncate clips s to at most w cells, marking the cut with a single-cell "…".
// w <= 0 yields "". A string that already fits is returned untouched.
func truncate(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if dispW(s) <= w {
		return s
	}
	if w == 1 {
		return "…"
	}
	var b strings.Builder
	used := 0
	for _, r := range s {
		rw := dispW(string(r))
		if used+rw > w-1 { // leave one cell for the ellipsis
			break
		}
		b.WriteRune(r)
		used += rw
	}
	return b.String() + strings.Repeat(" ", w-1-used) + "…"
}

// clipANSI cuts a possibly-styled string to w cells without severing an escape
// sequence. truncate() is for plain text; using it on styled output would cut
// mid-ANSI and bleed the style into everything after it.
func clipANSI(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if dispW(s) <= w {
		return s
	}
	return ansi.Truncate(s, w, "")
}

// padRight fits s into exactly w cells, truncating or right-padding as needed.
func padRight(s string, w int) string {
	s = truncate(s, w)
	return s + strings.Repeat(" ", max(0, w-dispW(s)))
}

// padLeft fits s into exactly w cells, right-aligned.
func padLeft(s string, w int) string {
	s = truncate(s, w)
	return strings.Repeat(" ", max(0, w-dispW(s))) + s
}
