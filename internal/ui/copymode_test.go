package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/hinshun/vt10x"

	"github.com/vulcanshen/sshu/internal/store"
)

// ------------------------------------------------------------- the real path
//
// These go through the app, so the wiring is under test and not just the
// arithmetic: unbind Alt+v, stop freezing the cell, or drop the clipboard
// command and they go red. The unit tests below build a copyState by hand and
// would all still pass with the mode unreachable.

// captureClipboard swaps the real clipboard for a string, so a test can assert
// on what was copied without owning the developer's actual clipboard — which it
// would otherwise be pasting into for the rest of the day.
func captureClipboard(t *testing.T) *string {
	t.Helper()
	var got string
	old := putClipboard
	putClipboard = func(s string) error { got = s; return nil }
	t.Cleanup(func() { putClipboard = old })
	return &got
}

// pressCmd is pressA for the one key whose whole point is the command it
// returns. The clipboard write happens in a tea.Cmd; a test that dropped the
// Cmd would be asserting on a copy that never ran.
func pressCmd(t *testing.T, m AppModel, key string) (AppModel, tea.Msg) {
	t.Helper()
	next, cmd := m.Update(keyMsg(key))
	m = settle(next.(AppModel))
	if cmd == nil {
		return m, nil
	}
	return m, cmd()
}

func TestAltVOpensSelectionModeAndTheSameChordLeavesIt(t *testing.T) {
	m := openOne(t)
	if m.ssh.copy.on {
		t.Fatal("a pty starts talking to the remote, not selecting from it")
	}
	m = pressA(m, "alt+v")
	if !m.ssh.copy.on {
		t.Fatal("alt+v should open selection mode on the focused cell")
	}
	m = pressA(m, "alt+v")
	if m.ssh.copy.on {
		t.Error("the same chord must close it again")
	}
}

// Alt+Esc peels one layer at a time everywhere in this tab, and the mode is a
// layer: it comes off before the pty focus does.
func TestAltEscLeavesSelectionModeBeforeThePty(t *testing.T) {
	m := pressA(openOne(t), "alt+v", "alt+esc")
	if m.ssh.copy.on {
		t.Fatal("alt+esc should have taken the mode off")
	}
	if m.ssh.focus != panelPty {
		t.Error("and only the mode — the keyboard stays in the cell")
	}
}

// The mode owns the panel. A key that leaked through to the remote from a
// screen that has stopped following it is a command run somewhere its author
// cannot see, which is the accident §11.19 already refuses for scrollback.
func TestSelectionModeSwallowsKeysInsteadOfSendingThem(t *testing.T) {
	m := openOne(t) // the stand-in echoes whatever it is sent
	s := m.ssh.sessions[0]

	m = pressA(m, "alt+v")
	m = pressA(m, "z")           // means nothing in the mode, and must not travel
	m = pressA(m, "w", "b", "0") // mean something in the mode, and must not either
	m = pressA(m, "alt+v")
	m = typeText(m, "Q") // this one is the remote's

	// The echo is FIFO: once Q has come back, a z sent before it would already
	// be on screen.
	waitFor(t, "the remote to echo the key it was actually sent", func() bool {
		return strings.Contains(strings.Join(s.pty.screenLines(), "\n"), "Q")
	})
	if got := strings.Join(s.pty.screenLines(), "\n"); strings.ContainsAny(got, "z0wb") {
		t.Errorf("a key pressed in selection mode reached the remote: %q", got)
	}
}

func TestSelectionModePaintsTheCellYellow(t *testing.T) {
	withColour(t)
	m := openOne(t)
	if strings.Contains(m.View(), ansiOf(t, selectColor)) {
		t.Fatal("nothing wears the selection colour until the mode is open")
	}
	m = pressA(m, "alt+v")
	if !strings.Contains(m.View(), ansiOf(t, selectColor)) {
		t.Error("the frame is the only thing that can say the panel changed meaning")
	}
}

func TestYPutsTheSelectionOnTheClipboardAndSaysSo(t *testing.T) {
	clip := captureClipboard(t)
	m := openOne(t)

	m = pressA(m, "alt+v", "V")
	m, msg := pressCmd(t, m, "y")

	done, ok := msg.(clipboardDoneMsg)
	if !ok {
		t.Fatalf("y should return a clipboard command, got %T", msg)
	}
	if done.err != nil {
		t.Fatalf("clipboard write failed: %v", done.err)
	}
	if *clip != "$" {
		t.Errorf("clipboard = %q, want the line the stand-in printed", *clip)
	}
	if m.ssh.copy.on {
		t.Error("y finishes the mode — it is the reason the mode was opened")
	}
	next, _ := m.Update(msg)
	if got := ansi.Strip(settle(next.(AppModel)).View()); !strings.Contains(got, "copied 1 line") {
		t.Error("a copy that reports nothing looks the same as one that failed")
	}
}

// Inside a pty the footer is the ONLY live disclosure: a bare `?` belongs to
// the remote, so a key that is not on this row cannot be found at all.
func TestThePtyFooterOffersTheWayIn(t *testing.T) {
	m := openOne(t)
	if !strings.Contains(m.footer(), "alt+v") {
		t.Errorf("the pty row must advertise the way into selection mode: %q", m.footer())
	}
	// And it survives the squeeze: keyLegend drops from the end, so alt+v has
	// to sit ahead of the cell chord rather than after it.
	// 40 columns fits exactly two pairs, so whichever comes last is the one
	// that goes — which is the whole reason the order was chosen.
	narrow := m
	narrow.w = 40
	narrow.ssh.setSize(40, 30)
	if !strings.Contains(narrow.footer(), "alt+v") {
		t.Errorf("alt+v was the first thing dropped on a narrow footer: %q", narrow.footer())
	}
}

func TestTheFooterSwitchesToTheSelectionKeys(t *testing.T) {
	m := openOne(t)
	if strings.Contains(m.footer(), "v/V") {
		t.Fatal("the pty row must not advertise keys that belong to another mode")
	}
	m = pressA(m, "alt+v")
	got := m.footer()
	for _, want := range []string{"y", "v/V", "hjkl", "w/e/b", "0/$", "u/d", "alt+v"} {
		if !strings.Contains(got, want) {
			t.Errorf("the selection row should disclose %q: %q", want, got)
		}
	}
	if strings.Contains(got, "alt+esc") {
		t.Error("the pty's own keys are gone while the mode is up; the row must not claim them")
	}
}

// Seven pairs are one more than an 80-column row holds, and keyLegend drops
// from the end. The way out has to be the pair that stays, so it sits ahead
// of the motions; the row gives up u/d instead, which j and k can stand in
// for.
func TestTheWayOutSurvivesAnEightyColumnSelectionRow(t *testing.T) {
	m := openOne(t)
	m.w = 80
	m.ssh.setSize(80, 30)
	m = pressA(m, "alt+v")
	got := m.footer()
	if strings.Contains(got, "u/d") {
		t.Fatalf("80 columns held the whole row, so the test proves nothing: %q", got)
	}
	for _, want := range []string{"y", "v/V", "alt+v", "hjkl", "w/e/b", "0/$"} {
		if !strings.Contains(got, want) {
			t.Errorf("the cramped selection row dropped %q: %q", want, got)
		}
	}
}

// The mode draws its own body into the cell, so it can shear the frame in a way
// the live path cannot. Same check the ssh tab already runs, with the mode up:
// every line exactly w cells, at the sizes where the layout changes shape.
func TestSelectionModePreservesTheFrame(t *testing.T) {
	withColour(t)
	aliveSSH(t)
	for _, sz := range [][2]int{{100, 30}, {78, 24}, {60, 30}, {54, 30}, {53, 30}, {40, 12}} {
		w, h := sz[0], sz[1]
		m := New(sample(), nil, store.DefaultConfig())
		next, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: h})
		m = pressA(settle(next.(AppModel)), "enter", "enter")
		waitFor(t, "the stand-in to answer", func() bool {
			return m.ssh.sessions[0].pty.hasSpoken()
		})

		m = pressA(m, "alt+v")
		if !m.ssh.copy.on {
			t.Errorf("%dx%d: the mode did not open, so the frame proves nothing", w, h)
			m.ssh.stopAll()
			continue
		}
		// A selection reaching past the right edge is the case most likely to
		// push a line wide.
		m = pressA(m, "v", "l", "l", "l", "j")

		for i, l := range strings.Split(m.View(), "\n") {
			if lw := dispW(l); lw != w {
				t.Errorf("%dx%d line %d: width %d, want %d\n%q", w, h, i, lw, w, l)
			}
		}
		m.ssh.stopAll()
	}
}

// --------------------------------------------------------------- the snapshot

// The scrollback is filled on the way IN, so it already holds the lines that
// are still on screen. Appending the screen to it would show every visible line
// twice; the screen replaces the tail instead.
func TestCopySnapshotLaysTheScreenOverTheHistoryTail(t *testing.T) {
	p := newTestTerm(40, 10)
	for i := range 20 {
		p.feed("line-" + itoa(i) + "\r\n")
	}
	got := p.copySnapshot(40, 10)

	if len(got) != 20 {
		t.Fatalf("snapshot = %d lines, want the 20 said (10 of history + the 10 on screen)", len(got))
	}
	n := 0
	for _, l := range got {
		if strings.Contains(ansi.Strip(l), "line-19") {
			n++
		}
	}
	if n != 1 {
		t.Errorf("the newest line appears %d times — the screen must REPLACE the tail, not follow it", n)
	}
	if !strings.Contains(ansi.Strip(got[0]), "line-0") {
		t.Errorf("the snapshot should start at the oldest line held, got %q", ansi.Strip(got[0]))
	}
}

// Nothing is captured while a full-screen program owns the terminal, so there
// is no history to put above the grid — and vim's own window is exactly what
// somebody in there is trying to copy out of.
func TestCopySnapshotInTheAltScreenIsTheGridAlone(t *testing.T) {
	p := newTestTerm(40, 10)
	p.feed("before the editor\r\n")
	p.feed("\x1b[?1049h")
	p.feed("INSIDE VIM\r\n")

	got := p.copySnapshot(40, 10)
	if len(got) != 10 {
		t.Fatalf("snapshot = %d lines, want exactly the grid", len(got))
	}
	joined := ansi.Strip(strings.Join(got, "\n"))
	if !strings.Contains(joined, "INSIDE VIM") {
		t.Error("the alt screen's own content is the whole point of selecting in there")
	}
	if strings.Contains(joined, "before the editor") {
		t.Error("the history is frozen during the alt screen and must not be spliced in")
	}
	if p.term.Mode()&vt10x.ModeAltScreen == 0 {
		t.Fatal("the stand-in never entered the alt screen — the test proved nothing")
	}
}

// The frozen page carries the mode's OWN cursor. Leaving the remote's on it
// too would put a second cursor on screen pointing at where typing would go —
// which is the one thing that cannot happen in there.
func TestTheFrozenPageDoesNotKeepTheRemoteCursor(t *testing.T) {
	withColour(t)
	p := newTestTerm(40, 4)
	p.feed("prompt$ ")

	// The live view draws it, so the absence below is the snapshot's doing and
	// not the emulator having no cursor to draw.
	if !strings.Contains(p.render(40, 4)[0], "\x1b[7m") {
		t.Fatal("the live cell should be drawing the remote's cursor")
	}
	if got := p.copySnapshot(40, 4)[0]; strings.Contains(got, "\x1b[7m") {
		t.Errorf("the frozen page kept the remote's cursor: %q", got)
	}
}

// ------------------------------------------------------------ the mode itself

// testCopy is a frozen page with no session behind it, for the arithmetic.
func testCopy(lines []string, w, h int) copyState {
	padded := make([]string, len(lines))
	for i, l := range lines {
		padded[i] = padRight(l, w)
	}
	c := copyState{on: true, lines: padded, w: w, h: h}
	c.top = max(0, len(padded)-h)
	c.row = c.newest()
	return c
}

func yank(c copyState, keys ...string) string {
	for _, k := range keys {
		if text, yanked := c.key(k); yanked {
			return text
		}
	}
	return ""
}

func TestCapitalVTakesWholeLinesAndVTakesCells(t *testing.T) {
	lines := []string{"alpha bravo", "charlie delta"}

	c := testCopy(lines, 20, 4)
	c.row = 0
	if got := yank(c, "V", "j", "y"); got != "alpha bravo\ncharlie delta" {
		t.Errorf("V yanked %q, want both whole lines", got)
	}

	c = testCopy(lines, 20, 4)
	c.row, c.col = 0, 6
	if got := yank(c, "v", "l", "l", "l", "l", "y"); got != "bravo" {
		t.Errorf("v yanked %q, want the cells swept", got)
	}
}

// Changing your mind about the shape must not cost you the start point.
func TestSwitchingShapeKeepsTheAnchor(t *testing.T) {
	c := testCopy([]string{"one", "two", "three"}, 10, 4)
	c.row, c.col = 0, 0
	c.key("v")
	c.key("j")
	c.key("V")
	if c.ancR != 0 {
		t.Fatalf("anchor moved to row %d when the shape changed", c.ancR)
	}
	if got, _ := c.key("y"); got != "one\ntwo" {
		t.Errorf("yanked %q, want the two lines the anchor still spans", got)
	}
}

// Two stages, innermost first — the same shape Esc has everywhere in sshu.
func TestEscDropsTheSelectionThenTheMode(t *testing.T) {
	c := testCopy([]string{"one", "two"}, 10, 4)
	c.key("V")
	c.key("esc")
	if !c.on {
		t.Fatal("the first Esc drops the selection, not the mode")
	}
	if c.sel != selNone {
		t.Fatal("the selection should be gone")
	}
	c.key("esc")
	if c.on {
		t.Error("the second Esc leaves the mode")
	}
}

// A y that did nothing would be a dead key, and "copy this line" is the only
// thing it could sensibly mean.
func TestYWithNothingMarkedTakesTheCursorLine(t *testing.T) {
	c := testCopy([]string{"first", "second"}, 10, 4)
	c.row = 1
	if got := yank(c, "y"); got != "second" {
		t.Errorf("yanked %q, want the line under the cursor", got)
	}
}

// The colour is why a line is worth looking at; escape sequences are not what
// anyone means to paste into an editor.
func TestCopiedTextIsPlainAndUnpadded(t *testing.T) {
	c := testCopy([]string{"\x1b[31mred alert\x1b[0m"}, 40, 4)
	got := yank(c, "y")
	if got != "red alert" {
		t.Errorf("yanked %q, want the text with neither styling nor the panel's padding", got)
	}
}

func TestTheCursorScrollsTheFrozenPage(t *testing.T) {
	lines := make([]string, 30)
	for i := range lines {
		lines[i] = "line-" + itoa(i)
	}
	c := testCopy(lines, 20, 5)
	top := c.top
	for range 10 {
		c.key("k")
	}
	if c.top >= top {
		t.Errorf("top stayed at %d — moving past the edge has to scroll the page", c.top)
	}
	if c.row < c.top || c.row >= c.top+c.h {
		t.Errorf("cursor at row %d is outside the window [%d,%d)", c.row, c.top, c.top+c.h)
	}
}

// Half a screen rather than a whole one: a full page leaves nothing on screen
// to tell you where you landed.
func TestUAndDMoveHalfAScreen(t *testing.T) {
	lines := make([]string, 40)
	for i := range lines {
		lines[i] = "line-" + itoa(i)
	}
	c := testCopy(lines, 20, 10)
	start := c.row
	c.key("u")
	if got := start - c.row; got != 5 {
		t.Errorf("u moved %d rows, want half of the 10-row cell", got)
	}
	c.key("d")
	if c.row != start {
		t.Errorf("d moved back to %d, want %d", c.row, start)
	}
}

func TestOnlyTheSelectedCellsAreMarked(t *testing.T) {
	withColour(t)
	c := testCopy([]string{"abcdefgh"}, 8, 1)
	c.row, c.col = 0, 2
	c.key("v")
	c.key("l") // cells 2..3

	line := c.visible()[0]
	mark := ansiBgOf(t, selectColor)
	if !strings.Contains(line, mark) {
		t.Fatalf("nothing is marked: %q", line)
	}
	// Everything before the anchor is outside the mark, so the first marked
	// cell must be the anchor's.
	before := strings.Index(line, mark)
	if got := dispW(ansi.Strip(line[:before])); got != 2 {
		t.Errorf("the mark starts %d cells in, want 2", got)
	}
	if got := ansi.Strip(line); got != "abcdefgh" {
		t.Errorf("marking changed the text to %q", got)
	}
}

// With nothing marked the cursor is still a cell you can see. A mode whose
// cursor is invisible is a mode nobody can aim.
func TestTheCursorIsVisibleBeforeAnythingIsMarked(t *testing.T) {
	withColour(t)
	c := testCopy([]string{"abcdefgh"}, 8, 1)
	if !strings.Contains(c.visible()[0], ansiBgOf(t, selectColor)) {
		t.Error("the cursor cell must be drawn even with no selection")
	}
}

// --------------------------------------------------------- word motions
//
// w, e, b, 0 and $ are vim's, class rule included: a word is a run of keyword
// characters or a run of punctuation, so `foo.bar` is three stops, not one.

// motion presses keys from a starting cell and says where the cursor ended.
func motion(lines []string, row, col int, keys ...string) (int, int) {
	c := testCopy(lines, 20, 4)
	c.row, c.col = row, col
	for _, k := range keys {
		c.key(k)
	}
	return c.row, c.col
}

func TestWStopsWhereVimWould(t *testing.T) {
	line := []string{"foo.bar baz_qux  end"}
	for _, tc := range []struct{ from, want int }{
		{0, 3},  // foo → the dot, a word of its own
		{3, 4},  // the dot → bar
		{4, 8},  // bar → baz_qux, one word: _ is a keyword character
		{8, 17}, // over two blanks
	} {
		if _, got := motion(line, 0, tc.from, "w"); got != tc.want {
			t.Errorf("w from %d landed on %d, want %d", tc.from, got, tc.want)
		}
	}
}

func TestWCrossesLinesAndStopsOnABlankOne(t *testing.T) {
	lines := []string{"one two", "", "  three"}
	if r, col := motion(lines, 0, 4, "w"); r != 1 || col != 0 {
		t.Errorf("w from the last word landed on %d:%d, want 1:0 — a blank line is a word", r, col)
	}
	if r, col := motion(lines, 1, 0, "w"); r != 2 || col != 2 {
		t.Errorf("w from the blank line landed on %d:%d, want 2:2, past the indent", r, col)
	}
}

// Where vim leaves you: on the last word of the page, w goes to its end, and
// from there it goes nowhere. It never moves backwards.
func TestWOnTheLastWordGoesToItsEndAndNoFurther(t *testing.T) {
	line := []string{"foo bar"}
	if _, got := motion(line, 0, 4, "w"); got != 6 {
		t.Errorf("w on the last word landed on %d, want 6, its last character", got)
	}
	if _, got := motion(line, 0, 6, "w"); got != 6 {
		t.Errorf("w on the last character moved to %d; there is nowhere to go", got)
	}
	if _, got := motion(line, 0, 12, "w"); got != 12 {
		t.Errorf("w from out in the padding moved to %d; a motion never goes backwards", got)
	}
}

func TestEGoesToTheEndOfThisWordThenTheNext(t *testing.T) {
	line := []string{"foo.bar  baz"}
	for _, tc := range []struct{ from, want int }{
		{0, 2},   // inside foo → its end
		{2, 3},   // the end of foo → the dot
		{3, 6},   // → the end of bar
		{6, 11},  // over the blanks → the end of baz
		{11, 11}, // nothing ahead
	} {
		if _, got := motion(line, 0, tc.from, "e"); got != tc.want {
			t.Errorf("e from %d landed on %d, want %d", tc.from, got, tc.want)
		}
	}
}

func TestBGoesToTheStartOfThisWordThenThePrevious(t *testing.T) {
	line := []string{"foo.bar  baz"}
	for _, tc := range []struct{ from, want int }{
		{11, 9}, // inside baz → its start
		{9, 4},  // the start of baz → over the blanks → the start of bar
		{4, 3},  // → the dot
		{3, 0},  // → foo
		{1, 0},  // inside foo → its start
		{0, 0},  // nothing behind
	} {
		if _, got := motion(line, 0, tc.from, "b"); got != tc.want {
			t.Errorf("b from %d landed on %d, want %d", tc.from, got, tc.want)
		}
	}
	if _, got := motion([]string{"   foo"}, 0, 3, "b"); got != 0 {
		t.Errorf("b with only blanks behind landed on %d, want 0, the head of the page", got)
	}
}

// b is w's mirror: the line break is a blank to cross, and a blank line is a
// word to stop on.
func TestBCrossesLinesAndStopsOnABlankOne(t *testing.T) {
	lines := []string{"one two", "", "  three"}
	if r, col := motion(lines, 2, 2, "b"); r != 1 || col != 0 {
		t.Errorf("b from three landed on %d:%d, want the blank line 1:0", r, col)
	}
	if r, col := motion(lines, 1, 0, "b"); r != 0 || col != 4 {
		t.Errorf("b from the blank line landed on %d:%d, want 0:4, the start of two", r, col)
	}
}

// e looks for the end of something, and a blank line has none to give — so
// unlike w it passes over one.
func TestEPassesOverABlankLine(t *testing.T) {
	if r, col := motion([]string{"foo", "", "bar"}, 0, 2, "e"); r != 2 || col != 2 {
		t.Errorf("e landed on %d:%d, want 2:2, the end of bar", r, col)
	}
}

func TestZeroAndDollarAreTheEndsOfTheText(t *testing.T) {
	lines := []string{"  hello world", ""}
	if _, got := motion(lines, 0, 5, "$"); got != 12 {
		t.Errorf("$ landed on %d, want 12 — the last character, not the padding", got)
	}
	if _, got := motion(lines, 0, 5, "0"); got != 0 {
		t.Errorf("0 landed on %d, want column 0", got)
	}
	if _, got := motion(lines, 1, 5, "$"); got != 0 {
		t.Errorf("$ on a blank line landed on %d, want 0", got)
	}
}

// A wide rune is one character across two columns. The motions walk
// characters, so none of them can stop in the second column of one — where
// the cursor would be marking half a glyph.
func TestMotionsLandOnCharactersNotColumns(t *testing.T) {
	line := []string{"日本語 ab"} // 日 0-1, 本 2-3, 語 4-5, blank 6, a 7, b 8
	if _, got := motion(line, 0, 0, "e"); got != 4 {
		t.Errorf("e landed on %d, want 4, the first column of 語", got)
	}
	if _, got := motion(line, 0, 1, "w"); got != 7 {
		t.Errorf("w from inside 日 landed on %d, want 7", got)
	}
	if _, got := motion([]string{"ab 日本"}, 0, 0, "$"); got != 5 {
		t.Errorf("$ landed on %d, want 5, where 本 starts", got)
	}
	if _, got := motion(line, 0, 5, "b"); got != 0 {
		t.Errorf("b from the second column of 語 landed on %d, want 0, the start of 日本語", got)
	}
}

// A motion that leaves the window scrolls the page, the same as j and k do.
func TestWordMotionsScrollThePage(t *testing.T) {
	lines := make([]string, 30)
	for i := range lines {
		lines[i] = "line-" + itoa(i)
	}
	c := testCopy(lines, 20, 5)
	c.top, c.row, c.col = 0, 4, 5 // the last visible row, on its last word
	c.key("w")
	if c.row != 5 {
		t.Fatalf("w landed on row %d, want 5, the first word below the window", c.row)
	}
	if c.row < c.top || c.row >= c.top+c.h {
		t.Errorf("cursor at row %d is outside the window [%d,%d)", c.row, c.top, c.top+c.h)
	}
	c.top, c.row, c.col = 10, 10, 0 // the first visible row, on its first word
	c.key("b")
	if c.row != 9 {
		t.Fatalf("b landed on row %d, want 9, the last word above the window", c.row)
	}
	if c.row < c.top || c.row >= c.top+c.h {
		t.Errorf("cursor at row %d is outside the window [%d,%d)", c.row, c.top, c.top+c.h)
	}
}

func TestAMissingClipboardToolSaysWhatToInstall(t *testing.T) {
	got := clipboardFailure(errNoClipboard)
	if !strings.Contains(got, "copy failed") {
		t.Errorf("message = %q, want it to say the copy did not happen", got)
	}
	if strings.Contains(got, "no clipboard tool found") {
		t.Errorf("message = %q — the raw error names no remedy", got)
	}
}
