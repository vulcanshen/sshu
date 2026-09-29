package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// tdp v0.1.18–v0.1.19: focus is told by the line as well as the colour (L5), a
// mode names itself at the top right of its frame (K11), and a hint that does
// not fit drops whole items (D3).

// Focus is a double line, everything else rounded, the same width either way,
// so switching focus never moves a cell (tdp L5).
func TestFocusIsADoubleLine(t *testing.T) {
	body := []string{strings.Repeat(" ", 20)}
	on := strings.Split(ansi.Strip(panelChrome(20, body, "title", true)), "\n")
	off := strings.Split(ansi.Strip(panelChrome(20, body, "title", false)), "\n")
	if !strings.HasPrefix(on[0], "╔") || !strings.HasSuffix(on[0], "╗") ||
		!strings.HasPrefix(on[1], "║") || on[2] != "╚"+strings.Repeat("═", 20)+"╝" {
		t.Errorf("the focused frame should be double:\n%s", strings.Join(on, "\n"))
	}
	if !strings.HasPrefix(off[0], "╭") || !strings.HasPrefix(off[1], "│") || !strings.HasPrefix(off[2], "╰") {
		t.Errorf("an unfocused frame should be rounded:\n%s", strings.Join(off, "\n"))
	}
	for i := range on {
		if dispW(on[i]) != dispW(off[i]) {
			t.Errorf("row %d changes width with focus: %d vs %d", i, dispW(on[i]), dispW(off[i]))
		}
	}
	// The echo of a list cursor is not the keyboard: rounded.
	if echo := ansi.Strip(panelChromeTone(20, body, "t", toneEcho)); !strings.HasPrefix(echo, "╭") {
		t.Error("an echo is not focus and should stay rounded")
	}
}

// Selection mode repaints the frame Yellow and keeps the focus line, and names
// itself at the right of the top border (tdp K11, L5, D2); leaving restores both.
func TestSelectionModeNamesItselfOnItsFrame(t *testing.T) {
	withColour(t)
	m := twoOnGrid(t)
	waitFor(t, "the remote to answer", func() bool { return m.inPty() })
	m = pressA(m, "alt+v")
	if !m.ssh.copy.on {
		t.Fatal("setup: alt+v should start selection mode")
	}
	grid := m.ssh.gridView()
	top := strings.Split(grid, "\n")[0]
	plain := ansi.Strip(top)
	// One word set between two junctions of the double line (tdp v0.1.20).
	if !strings.Contains(plain, "╔") || !strings.Contains(plain, "╡Select╞═╗") {
		t.Errorf("the cell in the mode should be double-lined and labelled at the top right: %q", plain)
	}
	yellow := sgr(ansiOf(t, selectColor))
	if !strings.Contains(top, yellow+"╡") {
		t.Errorf("the junctions should be in the frame colour: %q", top)
	}
	if bold := lipgloss.NewStyle().Foreground(selectColor).Bold(true).Render("Select"); !strings.Contains(top, bold) {
		t.Errorf("the name should be bold in the mode colour: %q", top)
	}
	for i, r := range strings.Split(grid, "\n") {
		if w := dispW(r); w != dispW(top) {
			t.Errorf("row %d is %d wide, the top %d", i, w, dispW(top))
		}
	}

	m = pressA(m, "alt+v")
	grid = ansi.Strip(m.ssh.gridView())
	if strings.Contains(grid, "Select") {
		t.Error("leaving the mode should take its name off")
	}
	if !strings.Contains(grid, "╔") {
		t.Error("the cell still holds the keyboard, so it stays double-lined")
	}
}

// The name always shows, so the title gives way; only a frame too narrow for
// the name alone cuts it. Every row keeps the frame's width.
func TestTheModeNameOutlastsTheTitle(t *testing.T) {
	body := []string{strings.Repeat(" ", 24)}
	out := strings.Split(ansi.Strip(panelChromeMode(24, body, "a-very-long-host-name", toneSelect, "selection mode")), "\n")
	if !strings.Contains(out[0], "╡selection mode╞") {
		t.Errorf("the name must show between its junctions: %q", out[0])
	}
	// A rounded frame takes the single-line junctions.
	if r := ansi.Strip(panelChromeMode(24, body, "t", toneIdle, "Select")); !strings.Contains(r, "┤Select├─╮") {
		t.Errorf("a rounded frame should use ┤ ├: %q", strings.Split(r, "\n")[0])
	}
	if strings.Contains(out[0], "a-very-long-host-name") {
		t.Errorf("the title should give way: %q", out[0])
	}
	for i, r := range out {
		if dispW(r) != 26 {
			t.Errorf("row %d is %d wide, not 26", i, dispW(r))
		}
	}
	narrow := strings.Split(ansi.Strip(panelChromeMode(8, []string{"        "}, "t", toneSelect, "selection mode")), "\n")
	if dispW(narrow[0]) != 10 || !strings.Contains(narrow[0], "sel") {
		t.Errorf("a narrow frame cuts the name and keeps its width: %q", narrow[0])
	}
	// The cell cuts its title to the room the name leaves, rather than losing
	// it: a host name too long to fit beside the name still shows its start.
	m := twoOnGrid(t)
	waitFor(t, "the remote to answer", func() bool { return m.inPty() })
	m = pressA(m, "alt+v")
	m.ssh.currentSession().host.Host = "a" + strings.Repeat("b", 80)
	top := ansi.Strip(strings.Split(m.ssh.gridView(), "\n")[0])
	if !strings.Contains(top, "abbb") || !strings.Contains(top, "╡Select╞") {
		t.Errorf("both the cut title and the name should show: %q", top)
	}
}

// Full screen has no frame: the name goes where its top right would be, and
// the legend stays on the last row.
func TestFullScreenSelectionModeNamesItself(t *testing.T) {
	m := twoOnGrid(t)
	waitFor(t, "the remote to answer", func() bool { return m.inPty() })
	m = pressA(m, "alt+z", "alt+z", "alt+v")
	rows := strings.Split(m.View(), "\n")
	if got := strings.TrimRight(ansi.Strip(rows[0]), " "); !strings.HasSuffix(got, "Select") {
		t.Errorf("the name should sit at the top right: %q", got)
	}
	if last := ansi.Strip(rows[len(rows)-1]); !strings.Contains(last, "Alt-v:leave") {
		t.Errorf("the legend should stay on the last row: %q", last)
	}
	for i, r := range rows {
		if dispW(r) != m.w {
			t.Errorf("row %d is %d wide, not %d", i, dispW(r), m.w)
		}
	}
}

// A hint too wide for its border gives up whole items from the end, like the
// footer; it never stops in the middle of one (tdp D3).
func TestAHintDropsWholeItemsFromTheEnd(t *testing.T) {
	pairs := [][2]string{{"j/k", "move"}, {"Enter", "run"}, {"Esc", "close"}}
	for w, want := range map[int]string{
		40: " j/k:move Enter:run Esc:close ",
		29: " j/k:move Enter:run ",
		20: " j/k:move Enter:run ",
		19: " j/k:move ",
	} {
		if got := ansi.Strip(fitLegend(pairs, w)); got != want {
			t.Errorf("w=%d: %q, want %q", w, got, want)
		}
	}
	// Narrow enough that even the first item cannot fit: cut, but never wider.
	if got := fitLegend(pairs, 5); dispW(got) > 5 {
		t.Errorf("the last resort must still fit: %q", ansi.Strip(got))
	}

	// On a box: the Space menu's hint at the minimum width's inner 28 has room
	// for 27 — a cut would stop inside Esc:close.
	box := strings.Split(ansi.Strip(drawPopupBox(focusColor, " menu ", pairs, []string{"x"}, 28)), "\n")
	bottom := box[len(box)-1]
	if !strings.Contains(bottom, " j/k:move Enter:run ") || strings.Contains(bottom, "Esc") {
		t.Errorf("the hint should end on a whole item: %q", bottom)
	}
	if dispW(bottom) != dispW(box[0]) {
		t.Errorf("the bottom border changed width: %q", bottom)
	}
}
