package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// The cwd is plain text: lavender segments joined by dim slashes, no spaces, so
// the row reads as the literal path it is.
//
// No powerline chips — and the reason is the panel titles. Those wear capsules
// now, and a chip-backed breadcrumb sitting one row underneath competes with
// them for the same visual weight; two strips of filled shapes stacked together
// read as one piece of chrome rather than as a title and a location. filu landed
// on exactly this and says so in its own crumbRow.
//
// Lavender says "you are here". It is the same colour as the form row under
// edit, which is one meaning at two scales — the field you are changing, the
// directory you are in — rather than two meanings sharing a band.
func renderCrumb(p string, w int) string {
	if w <= 0 || p == "" {
		return ""
	}
	segs := crumbSegments(fitPath(p, w))
	if len(segs) == 0 {
		return ""
	}

	sep := lipgloss.NewStyle().Foreground(dimColor).Render("/")
	text := lipgloss.NewStyle().Foreground(editColor)

	parts := make([]string, 0, len(segs))
	for _, seg := range segs {
		parts = append(parts, text.Render(seg))
	}
	// An absolute path's "/" root IS the leading separator; rendering it as a
	// segment as well would show "//".
	if segs[0] == "/" {
		return sep + strings.Join(parts[1:], sep)
	}
	return strings.Join(parts, sep)
}

// crumbSegments splits a path into its parts, keeping a leading "/" as its own
// segment so an absolute path still shows where it starts.
func crumbSegments(p string) []string {
	if p == "/" {
		return []string{"/"}
	}
	root := ""
	if strings.HasPrefix(p, "/") {
		root, p = "/", strings.TrimPrefix(p, "/")
	}
	var out []string
	if root != "" {
		out = append(out, root)
	}
	for _, seg := range strings.Split(p, "/") {
		if seg != "" {
			out = append(out, seg)
		}
	}
	return out
}

// crumbWidth is the rendered cell width. Plain text with single-cell separators,
// so it is the shortened path's own width.
func crumbWidth(p string) int { return dispW(p) }
