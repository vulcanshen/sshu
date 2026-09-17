package ui

import (
	"strings"
	"unicode"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// Selection mode is sshu's copy-mode: Alt+v freezes the focused cell, puts a
// cursor on the frozen text, and hands the sweep to the system clipboard.
//
// It exists because the terminal's OWN selection is the one thing a grid of
// panels breaks — a drag runs along a physical screen line, so it collects the
// border and the neighbouring cell's output on the way past. The usual fix is
// mouse tracking, and sshu will not pay for it: enabling the mouse takes native
// selection away from the WHOLE app, including the lists and popups where it
// still works fine (§5, §11.33). A keyboard mode costs nothing outside itself.
//
// The buffer is FROZEN on entry. The remote keeps talking — the session never
// stops reading — but the panel stops following it, because a page that reflows
// under a half-made selection is not a page anyone can select from.

type selKind int

const (
	selNone selKind = iota // moving the cursor; nothing marked yet
	selChar                // v: anchor cell to cursor cell, like vim's visual
	selLine                // V: whole lines, anchor row to cursor row
)

// copyState is one panel's selection mode. Coordinates are indices into lines
// and DISPLAY columns within them — never byte offsets, because every line in
// here may carry the colour it arrived with.
type copyState struct {
	on     bool
	sessID int // whose panel: the mode cannot outlive its session
	// lines is the frozen buffer, each already exactly w cells wide.
	lines []string
	w, h  int
	top   int // first visible line
	// row/col is the cursor; ancR/ancC the point v or V anchored it at.
	row, col   int
	ancR, ancC int
	sel        selKind
}

// selStyle paints the selection. It replaces the text's own colour rather than
// reversing it: reverse video composes differently with every palette a remote
// might be using, and "what is selected" has to be one unmistakable thing. The
// yellow is the border's yellow — the frame and the mark say the same word.
var selStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(baseHex)).Background(selectColor)

// start freezes the panel and puts the cursor on the newest line that says
// anything. That lands "copy what just printed" on three keys — Alt+v, V, y —
// which is the reason the mode gets opened most of the time.
func (c *copyState) start(s *session, w, h int) {
	if s == nil || s.pty == nil || w <= 0 || h <= 0 {
		return
	}
	lines := s.pty.copySnapshot(w, h)
	if len(lines) == 0 {
		lines = []string{strings.Repeat(" ", w)}
	}
	*c = copyState{on: true, sessID: s.id, lines: lines, w: w, h: h}
	c.top = max(0, len(lines)-h)
	c.row, c.col = c.newest(), 0
}

func (c *copyState) stop() { *c = copyState{} }

// newest is the last line in view with anything on it. A shell that has just
// printed its prompt leaves blank rows below it, and starting the cursor down
// there would open the mode pointing at nothing.
func (c copyState) newest() int {
	for r := len(c.lines) - 1; r > c.top; r-- {
		if strings.TrimSpace(ansi.Strip(c.lines[r])) != "" {
			return r
		}
	}
	return c.top
}

// key runs one keystroke. yanked says y was pressed and the mode has finished:
// it is a separate flag from the text because selecting a blank line is a
// legitimate thing to do, and "copied nothing" still has to report itself
// rather than look like a key that did not register.
//
// Everything else is swallowed whether or not it means something here. The mode
// owns the panel: a stray letter reaching the remote from a screen that stopped
// following it is exactly the accident §11.19 already refuses for scrollback.
func (c *copyState) key(k string) (text string, yanked bool) {
	switch k {
	case "j", "down":
		c.moveRow(1)
	case "k", "up":
		c.moveRow(-1)
	case "d":
		c.moveRow(c.half())
	case "u":
		c.moveRow(-c.half())
	case "h", "left":
		c.col = max(0, c.col-1)
	case "l", "right":
		c.col = min(c.w-1, c.col+1)
	case "w":
		c.wordStart()
	case "e":
		c.wordEnd()
	case "b":
		c.wordBack()
	case "0":
		c.col = 0
	case "$":
		c.col = c.lineEnd(c.row)
	case "v":
		c.mark(selChar)
	case "V":
		c.mark(selLine)
	case "y":
		text := c.text()
		c.stop()
		return text, true
	case "esc":
		// Two stages, innermost first: drop the selection, then the mode. Same
		// shape as Esc everywhere else in sshu (§4.3).
		if c.sel != selNone {
			c.sel = selNone
			return "", false
		}
		c.stop()
	}
	return "", false
}

// half is how far u and d go. Half a screen rather than a whole one, because a
// full page leaves nothing on screen to tell you where you landed — vim's
// Ctrl+u / Ctrl+d, for the same reason.
func (c copyState) half() int { return max(1, c.h/2) }

func (c *copyState) moveRow(n int) { c.moveTo(c.row+n, c.col) }

// moveTo puts the cursor on a cell and scrolls the page just far enough to
// keep it in view.
func (c *copyState) moveTo(row, col int) {
	c.row = clamp(row, 0, len(c.lines)-1)
	c.col = clamp(col, 0, c.w-1)
	c.top = clamp(c.top, c.row-c.h+1, c.row)
	c.top = clamp(c.top, 0, max(0, len(c.lines)-c.h))
}

// A motion that walks WORDS has to walk characters, not columns. h and l can
// step a column at a time because a column is always somewhere; "the next
// word" is a question about what is written, and a wide rune is one thing
// written across two columns. So a line is read out as characters first,
// each with the columns it covers, and w, e and $ land on one of those — never
// on the second half of a glyph.
type char struct {
	col, end int // display columns: the first one it covers, and the one after
	r        rune
}

// lineChars is one frozen line as characters, the panel's padding left off.
// A row with nothing on it comes back empty, which is what makes it vim's
// blank line below. Trailing blanks the remote itself printed are
// indistinguishable from the padding and go the same way — a $ stops where
// the visible text does.
func lineChars(line string) []char {
	plain := plainText(line)
	out := make([]char, 0, len(plain))
	col := 0
	for _, r := range plain {
		end := col + dispW(string(r))
		out = append(out, char{col, end, r})
		col = end
	}
	return out
}

// charAt is the index of the character under column col, or len(cs) when the
// column is past the last one — out in the padding.
func charAt(cs []char, col int) int {
	for i, ch := range cs {
		if col < ch.end {
			return i
		}
	}
	return len(cs)
}

// wordClass is vim's: a blank, a run of punctuation, or a run of keyword
// characters — letters, digits and underscore. w and e stop where the class
// changes, so `foo.bar` is three words and `foo_bar` one, the same places vim
// stops. The vocabulary is borrowed, so its rule comes with it.
func wordClass(r rune) int {
	switch {
	case unicode.IsSpace(r):
		return 0
	case r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r):
		return 2
	}
	return 1
}

// classAt is the class of character i, and a blank past the end of the line:
// the padding, and the line break itself, separate words the way a space
// does.
func classAt(cs []char, i int) int {
	if i < 0 || i >= len(cs) {
		return 0
	}
	return wordClass(cs[i].r)
}

// wordStart is w: past the word under the cursor, past the blanks after it,
// onto the first character of the next word — on this line or a later one. A
// row with nothing on it counts as a word, so w stops there; that is vim's
// rule, and the only way w can ever land on a blank line.
func (c *copyState) wordStart() {
	row := c.row
	cs := lineChars(c.lines[row])
	i := charAt(cs, c.col)
	if cls := classAt(cs, i); cls != 0 {
		for classAt(cs, i) == cls {
			i++
		}
	}
	for {
		for i < len(cs) && classAt(cs, i) == 0 {
			i++
		}
		if i < len(cs) {
			c.moveTo(row, cs[i].col)
			return
		}
		row++
		if row >= len(c.lines) {
			c.moveToEnd()
			return
		}
		cs, i = lineChars(c.lines[row]), 0
		if len(cs) == 0 {
			c.moveTo(row, 0)
			return
		}
	}
}

// wordEnd is e: the last character of the word under the cursor, or — from
// there, or from a blank — of the next word, on this line or a later one.
// Unlike w it does not stop on a blank line: e looks for the end of
// something, and a line with nothing on it has no end to give.
func (c *copyState) wordEnd() {
	row := c.row
	cs := lineChars(c.lines[row])
	i := charAt(cs, c.col)
	cls := classAt(cs, i)
	// One step on, and then either still inside the same word, or looking
	// for the next one.
	i++
	if cls == 0 || classAt(cs, i) != cls {
		for classAt(cs, i) == 0 {
			if i < len(cs) {
				i++
				continue
			}
			row++
			if row >= len(c.lines) {
				c.moveToEnd()
				return
			}
			cs, i = lineChars(c.lines[row]), 0
		}
	}
	cls = classAt(cs, i)
	for classAt(cs, i+1) == cls {
		i++
	}
	c.moveTo(row, cs[i].col)
}

// wordBack is b: back over the blanks before the cursor, then to the first
// character of the word they follow — on this line or an earlier one. It is
// w's mirror, and a blank line stops it the way one stops w: the empty line
// is a word to both. The line break counts as a blank, the same as the
// padding does, so b from the head of a row lands on the last word above.
func (c *copyState) wordBack() {
	row := c.row
	cs := lineChars(c.lines[row])
	i := charAt(cs, c.col)
	// back is one character backwards, crossing to the line break of the row
	// above; false at the head of the page.
	back := func() bool {
		if i > 0 {
			i--
			return true
		}
		if row == 0 {
			return false
		}
		row--
		cs = lineChars(c.lines[row])
		i = len(cs)
		return true
	}
	if !back() {
		return
	}
	for classAt(cs, i) == 0 {
		if len(cs) == 0 || !back() {
			c.moveTo(row, 0)
			return
		}
	}
	cls := classAt(cs, i)
	for i > 0 && classAt(cs, i-1) == cls {
		i--
	}
	c.moveTo(row, cs[i].col)
}

// moveToEnd is where w and e go with no word left ahead of them: the last
// character on the page, which is where vim leaves you — unless the cursor
// is already past it, in which case nowhere. Neither motion moves backwards.
func (c *copyState) moveToEnd() {
	for row := len(c.lines) - 1; row >= 0; row-- {
		cs := lineChars(c.lines[row])
		if len(cs) == 0 {
			continue
		}
		if last := cs[len(cs)-1].col; row > c.row || (row == c.row && last > c.col) {
			c.moveTo(row, last)
		}
		return
	}
}

// lineEnd is $: the last character on the row, or column 0 of a row with
// nothing on it. The padding does not count — the panel put it there, not
// the remote.
func (c copyState) lineEnd(row int) int {
	cs := lineChars(c.lines[row])
	if len(cs) == 0 {
		return 0
	}
	return cs[len(cs)-1].col
}

// mark starts, switches, or drops a selection. Pressing the same key again
// clears it; switching between v and V keeps the anchor where it was, so
// changing your mind about the shape does not cost you the start point.
func (c *copyState) mark(k selKind) {
	if c.sel == k {
		c.sel = selNone
		return
	}
	if c.sel == selNone {
		c.ancR, c.ancC = c.row, c.col
	}
	c.sel = k
}

// span puts the two ends in reading order, so everything downstream can assume
// the first one comes first.
func (c copyState) span() (r0, c0, r1, c1 int) {
	if c.sel == selNone {
		return c.row, c.col, c.row, c.col
	}
	r0, c0, r1, c1 = c.ancR, c.ancC, c.row, c.col
	if r1 < r0 || (r1 == r0 && c1 < c0) {
		r0, c0, r1, c1 = r1, c1, r0, c0
	}
	return r0, c0, r1, c1
}

// text is what y puts on the clipboard: the selection, or with nothing marked,
// the line the cursor is on. A y that did nothing would be a dead key, and
// "copy this line" is the only thing it could sensibly mean.
//
// What comes out is PLAIN: the colour is why the line is worth looking at, and
// escape sequences are not what anyone means to paste into an editor.
func (c copyState) text() string {
	r0, c0, r1, c1 := c.span()
	if c.sel != selChar {
		out := make([]string, 0, r1-r0+1)
		for r := r0; r <= r1; r++ {
			out = append(out, plainText(c.lines[r]))
		}
		return strings.Join(out, "\n")
	}
	if r0 == r1 {
		return plainText(ansi.Cut(c.lines[r0], c0, c1+1))
	}
	out := []string{plainText(ansi.Cut(c.lines[r0], c0, c.w))}
	for r := r0 + 1; r < r1; r++ {
		out = append(out, plainText(c.lines[r]))
	}
	return strings.Join(append(out, plainText(ansi.Cut(c.lines[r1], 0, c1+1))), "\n")
}

// plainText strips the styling and the padding a rendered line carries. The
// buffer is padded to the panel's width; pasting that trailing whitespace into
// an editor is nobody's intent.
func plainText(s string) string { return strings.TrimRight(ansi.Strip(s), " ") }

// visible is the h lines the panel draws while the mode is on, with the mark
// laid over them — or, with nothing marked, the single cell under the cursor,
// so the cursor is always somewhere you can see.
func (c copyState) visible() []string {
	r0, c0, r1, c1 := c.span()
	blank := strings.Repeat(" ", max(0, c.w))
	out := make([]string, 0, c.h)
	for i := range c.h {
		r := c.top + i
		if r < 0 || r >= len(c.lines) {
			out = append(out, blank)
			continue
		}
		a, b := c.rowSpan(r, r0, c0, r1, c1)
		out = append(out, markRow(c.lines[r], a, b, c.w))
	}
	return out
}

// rowSpan is the half-open column range marked on row r; b <= a means none.
func (c copyState) rowSpan(r, r0, c0, r1, c1 int) (a, b int) {
	if r < r0 || r > r1 {
		return 0, 0
	}
	switch {
	case c.sel == selNone:
		return c.col, c.col + 1
	case c.sel == selLine:
		return 0, c.w
	case r0 == r1:
		return c0, c1 + 1
	case r == r0:
		return c0, c.w
	case r == r1:
		return 0, c1 + 1
	}
	return 0, c.w
}

// markRow lays the mark over one line between two display columns, keeping the
// colour on either side of it. ansi.Cut carries the active styling into each
// piece, so the unmarked halves come out exactly as they went in.
func markRow(line string, a, b, w int) string {
	a, b = clamp(a, 0, w), clamp(b, 0, w)
	if b <= a {
		return line
	}
	return ansi.Cut(line, 0, a) +
		selStyle.Render(ansi.Strip(ansi.Cut(line, a, b))) +
		ansi.Cut(line, b, w)
}
