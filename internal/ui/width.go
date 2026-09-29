package ui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
	overlay "github.com/rmhubbert/bubbletea-overlay"
)

// Everything drawn into a fixed slot goes through these. The card is a
// fixed-width box (tdp L2), so a field that miscounts its own width does not
// merely look off — it pushes the right border out and breaks the frame.
//
// Rule: measure and pad PLAIN text, then apply the style. Measuring skips ANSI,
// but padding a styled string means the pad lands inside the styled span and
// picks up its background.
//
// And one width for everything (tdp D6): some Nerd Fonts made for CJK draw an
// icon two cells wide — the cursor moves two — while lipgloss and x/ansi count
// one, and every border beside an icon goes crooked. The width is probed at
// startup (DetectIconWidth), and every measurement, cut, pad, join and overlay
// below counts an icon as iconCells. Nothing in internal/ui measures a width any
// other way. The port follows filu's width.go, the family reference.

// iconCells is how many cells the terminal moves the cursor for a Nerd Font
// icon. 1 on a normal font; DetectIconWidth or SSHU_ICON_WIDTH sets 2. At 1
// every function here measures exactly as x/ansi does.
var iconCells = 1

// iconFixed is set when SSHU_ICON_WIDTH chose the width: then neither the probe
// nor the layer above this one changes it.
var iconFixed bool

// IconCells reports the detected icon width.
func IconCells() int { return iconCells }

// isWideIcon reports whether r is a Nerd Font icon that a CJK icon font draws
// two cells wide. The powerline caps (U+E0A0–E0D7, the capsule ends) live in
// the Private Use Area too but stay one cell, so they are left out.
func isWideIcon(r rune) bool {
	if r >= 0xe0a0 && r <= 0xe0d7 {
		return false
	}
	// BMP Private Use Area, and supplementary PUA-A (Material Design icons).
	return (r >= 0xe000 && r <= 0xf8ff) || (r >= 0xf0000 && r <= 0xffffd)
}

// iconCount is how many wide icons s holds, ANSI stripped; 0 without looking
// when icons are one cell.
func iconCount(s string) int {
	if iconCells == 1 {
		return 0
	}
	n := 0
	for _, r := range ansi.Strip(s) {
		if isWideIcon(r) {
			n++
		}
	}
	return n
}

// dispW is the display width of s — of its widest line, when it has several.
func dispW(s string) int {
	if !strings.Contains(s, "\n") {
		return ansi.StringWidth(s) + iconCount(s)*(iconCells-1)
	}
	return blockWidth(strings.Split(s, "\n"))
}

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

// truncateHead cuts from the FRONT, keeping the tail. For a path that is the
// only useful direction: `~/.ssh/config.d/work` and `~/.ssh/config.d/team`
// differ at the end, and cutting there would leave two rows reading alike.
func truncateHead(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if dispW(s) <= w {
		return s
	}
	if w == 1 {
		return "…"
	}
	r := []rune(s)
	used, i := 0, len(r)
	for ; i > 0; i-- {
		rw := dispW(string(r[i-1]))
		if used+rw > w-1 { // leave one cell for the ellipsis
			break
		}
		used += rw
	}
	return "…" + strings.Repeat(" ", w-1-used) + string(r[i:])
}

// clipANSI cuts a possibly-styled string to w cells without severing an escape
// sequence. truncate() is for plain text; using it on styled output would cut
// mid-ANSI and bleed the style into everything after it.
//
// The first w measured cells are at least w display cells; each icon among them
// takes one more, so it steps back until they fit — an icon anywhere in the
// line, not only at its start.
func clipANSI(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if dispW(s) <= w {
		return s
	}
	for target := w; target > 0; target-- {
		if out := ansi.Truncate(s, target, ""); dispW(out) <= w {
			return out
		}
	}
	return ""
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

// dispCutLeft drops the first n display cells of s and returns the rest, ANSI
// styles kept. A wide icon or character cut in half becomes spaces, so the
// result is always dispW(s) − n wide.
func dispCutLeft(s string, n int) string {
	if n <= 0 {
		return s
	}
	total := dispW(s)
	if n >= total {
		return ""
	}
	// m measured cells hold at least n display cells; start where they would
	// if every icon so far were narrow, and step up past a wide one.
	m := max(n-iconCount(s)*(iconCells-1), 0)
	for dispW(ansi.Truncate(s, m, "")) < n {
		m++
	}
	rest := ansi.TruncateLeft(s, m, "")
	return strings.Repeat(" ", max(total-n-dispW(rest), 0)) + rest
}

// dispCut is display cells [a, b) of s, styles kept — ansi.Cut, counting an
// icon as the terminal draws it.
func dispCut(s string, a, b int) string {
	if b <= a {
		return ""
	}
	return dispCutLeft(clipANSI(s, b), a)
}

// compositeDisp draws fg over bg — overlay.Composite, but every width is the
// display width, so a wide icon in the popup or in what it covers cannot push
// a line past the screen (tdp D6, L4). Placement is overlay's: Left / Top at 0,
// Center at half the background less half the foreground, Right / Bottom
// flush, then moved by the offsets and kept on screen.
func compositeDisp(fg, bg string, xPos, yPos overlay.Position, xOff, yOff int) string {
	if fg == "" {
		return bg
	}
	if bg == "" {
		return fg
	}
	fgLines, bgLines := strings.Split(fg, "\n"), strings.Split(bg, "\n")
	fgW, bgW := blockWidth(fgLines), blockWidth(bgLines)
	fgH, bgH := len(fgLines), len(bgLines)
	if fgW >= bgW && fgH >= bgH {
		return fg
	}
	x := clampSpan(placeOffset(xPos, bgW, fgW)+xOff, bgW-fgW)
	y := clampSpan(placeOffset(yPos, bgH, fgH)+yOff, bgH-fgH)
	for i, line := range fgLines {
		if y+i >= bgH {
			break
		}
		row := bgLines[y+i]
		left := clipANSI(row, x)
		left += strings.Repeat(" ", x-dispW(left)) // a wide icon cut at x, or a short row
		right := dispCutLeft(row, x+dispW(line))
		bgLines[y+i] = left + line + right
	}
	return strings.Join(bgLines, "\n")
}

// centerDisp centres s in a w × h area by display width, the smaller half of
// the gap left and on top — lipgloss.Place(w, h, Center, Center, s). In a
// direction s already fills it is left as it is; h 0 centres across only.
func centerDisp(w, h int, s string) string {
	lines := strings.Split(s, "\n")
	width := blockWidth(lines)
	if w > width {
		for i, l := range lines {
			gap := w - dispW(l)
			lines[i] = strings.Repeat(" ", gap/2) + l + strings.Repeat(" ", gap-gap/2)
		}
		width = w
	}
	if gap := h - len(lines); gap > 0 {
		blank := strings.Repeat(" ", width)
		out := make([]string, 0, h)
		for range gap / 2 {
			out = append(out, blank)
		}
		out = append(out, lines...)
		for len(out) < h {
			out = append(out, blank)
		}
		lines = out
	}
	return strings.Join(lines, "\n")
}

// blockWidth is the display width of the widest line.
func blockWidth(lines []string) int {
	w := 0
	for _, l := range lines {
		w = max(w, dispW(l))
	}
	return w
}

// placeOffset is where a span of size fg starts in one of size bg.
func placeOffset(p overlay.Position, bg, fg int) int {
	switch p {
	case overlay.Center:
		return bg/2 - fg/2
	case overlay.Right, overlay.Bottom:
		return bg - fg
	}
	return 0
}

// clampSpan keeps v between 0 and hi (either way round, as overlay does).
func clampSpan(v, hi int) int {
	lo := 0
	if lo > hi {
		lo, hi = hi, lo
	}
	return min(max(v, lo), hi)
}

// joinHorizontal lays blocks side by side, top-aligned. Each block's lines are
// padded to that block's own display width, so a wide icon in one column never
// shoves the next one over (lipgloss.JoinHorizontal counts an icon as one).
func joinHorizontal(blocks ...string) string {
	rows := make([][]string, len(blocks))
	widths := make([]int, len(blocks))
	maxRows := 0
	for i, b := range blocks {
		rows[i] = strings.Split(b, "\n")
		widths[i] = blockWidth(rows[i])
		maxRows = max(maxRows, len(rows[i]))
	}
	out := make([]string, maxRows)
	for r := range maxRows {
		var line strings.Builder
		for i := range rows {
			cell := ""
			if r < len(rows[i]) {
				cell = clipANSI(rows[i][r], widths[i])
			}
			line.WriteString(cell + strings.Repeat(" ", widths[i]-dispW(cell)))
		}
		out[r] = line.String()
	}
	return strings.Join(out, "\n")
}

// joinVertical stacks blocks, left-aligned, every line padded to the widest.
func joinVertical(blocks ...string) string {
	var lines []string
	for _, b := range blocks {
		lines = append(lines, strings.Split(b, "\n")...)
	}
	w := blockWidth(lines)
	for i, l := range lines {
		lines[i] = l + strings.Repeat(" ", max(0, w-dispW(l)))
	}
	return strings.Join(lines, "\n")
}
