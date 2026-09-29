package ui

import (
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	overlay "github.com/rmhubbert/bubbletea-overlay"
	"github.com/vulcanshen/sshu/internal/store"
)

// tdp v0.1.18–v0.1.20 D6: some Nerd Fonts made for CJK draw an icon two cells
// wide — the cursor moves two — while lipgloss counts one, and every border
// beside an icon goes crooked. Every width in internal/ui goes through
// width.go, and the screen tests run once more with two-cell icons.

// withWideIcons makes an icon two cells for the rest of the test.
func withWideIcons(t *testing.T) {
	t.Helper()
	old := iconCells
	iconCells = 2
	t.Cleanup(func() { iconCells = old })
}

// wideIcon is an ordinary Nerd Font icon: one cell, or two on a CJK icon font.
const wideIcon = ""

// Every screen test that measures the frame, again with two-cell icons: the
// hosts table's auth glyphs, file transfer's file glyphs, the session list,
// every popup's title glyph, the splash's pixels.
func TestTheFramesHoldWithTwoCellIcons(t *testing.T) {
	withWideIcons(t)
	for name, test := range map[string]func(*testing.T){
		"ssh tab":            TestSSHTabPreservesFrame,
		"grid layouts":       TestGridPreservesFrameAcrossLayouts,
		"file transfer":      TestSFTPTabPreservesFrame,
		"manage":             TestPrefTabPreservesFrame,
		"popups":             TestPopupPreservesFrame,
		"popup widths":       TestEveryPopupIsTheSameWidth,
		"empty states":       TestEmptyStatesPreserveFrame,
		"selection mode":     TestSelectionModePreservesTheFrame,
		"editor":             TestEditorPopupPreservesFrame,
		"identity picker":    TestPickerFrameHolds,
		"dialing":            TestDialingPreservesFrame,
		"search":             TestSearchPreservesFrame,
		"full screen":        TestFullScreenPreservesTheFrame,
		"viewer":             TestViewerPreservesFrame,
		"wide remote output": TestWideRemoteOutputCannotBreakTheFrame,
	} {
		t.Run(name, test)
	}
}

// The measure itself: an icon counts iconCells, a powerline cap stays one, and
// nothing changes on a normal font.
func TestAnIconIsAsWideAsTheFontDrawsIt(t *testing.T) {
	s := "a" + wideIcon + "b" + capLeft
	if got := dispW(s); got != 4 {
		t.Fatalf("one-cell icons: %d, want 4", got)
	}
	withWideIcons(t)
	if got := dispW(s); got != 5 {
		t.Errorf("two-cell icon: %d, want 5 (the cap stays one)", got)
	}
	if got := dispW("\U000F0A9E"); got != 2 {
		t.Errorf("a PUA-A icon (the loading glyphs) is an icon too: %d", got)
	}
	for w := 0; w <= 6; w++ {
		if got := dispW(clipANSI("\x1b[1m"+wideIcon+wideIcon+"xy\x1b[0m", w)); got > w {
			t.Errorf("clipANSI to %d is %d wide", w, got)
		}
	}
	if got := dispW(padRight(wideIcon+" privatekey", credAuthW())); got != credAuthW() ||
		!strings.Contains(padRight(wideIcon+" privatekey", credAuthW()), "privatekey") {
		t.Error("the credentials Auth column should hold a two-cell glyph and privatekey whole")
	}
}

// compositeDisp lays a popup over the screen by display width. The four edges:
// an icon in the popup, an icon in what it covers, and an icon the popup's left
// or right edge cuts in half — which becomes a space rather than a shifted row.
func TestAPopupOverIconsKeepsEveryRowItsWidth(t *testing.T) {
	withWideIcons(t)
	bg := strings.Join([]string{
		strings.Repeat("-", 20),
		"ab" + wideIcon + strings.Repeat("-", 16),   // the icon sits under the left edge
		strings.Repeat("-", 14) + wideIcon + "----", // under the right edge
		wideIcon + strings.Repeat("-", 18),
	}, "\n")
	fg := strings.Join([]string{"[" + wideIcon + "   ]", "[      ]", "[      ]"}, "\n")
	out := compositeDisp(fg, bg, overlay.Center, overlay.Bottom, 0, 0)
	for i, l := range strings.Split(out, "\n") {
		if dispW(l) != 20 {
			t.Errorf("row %d is %d wide, not 20: %q", i, dispW(l), l)
		}
	}
	if !strings.Contains(out, "["+wideIcon) {
		t.Error("the popup's own icon should be drawn")
	}
}

// Selection mode marks by display columns and cuts by the same ruler, so on a
// row with an icon the mark covers what the cursor is on.
func TestSelectionCutsByTheSameRulerItMeasures(t *testing.T) {
	withWideIcons(t)
	line := "a" + wideIcon + "bc"
	if got := ansi.Strip(dispCut(line, 3, 4)); got != "b" {
		t.Errorf("column 3 on %q should be b, got %q", line, got)
	}
	cs := lineChars(line)
	if len(cs) != 4 || cs[2].col != 3 {
		t.Fatalf("lineChars should put b at column 3: %+v", cs)
	}
	withColour(t)
	if got := markRow(line, 3, 4, 5); !strings.Contains(got, selStyle.Render("b")) {
		t.Errorf("the mark at column 3 should be on b: %q", got)
	}
	c := copyState{lines: []string{line}, w: 5, h: 1, sel: selChar, ancC: 3, col: 3}
	if got := c.text(); got != "b" {
		t.Errorf("copying column 3 should give b, got %q", got)
	}
}

// The splash's pixels are two cells either way: a one-cell glyph gets a space,
// a two-cell glyph none — and every line of it is the same width.
func TestTheSplashKeepsItsShapeWithTwoCellIcons(t *testing.T) {
	withWideIcons(t)
	m := newSplashModel()
	m.show()
	m.revealedCount = len(m.pixelOrder)
	m.identityVisible, m.versionVisible, m.taglineVisible, m.hintVisible = true, true, true, true
	for i, l := range strings.Split(m.render(80, 40), "\n") {
		if dispW(l) != 80 {
			t.Errorf("splash row %d is %d wide, not 80", i, dispW(l))
		}
	}
	// Unpadded, each logo row is exactly two cells a pixel.
	want := len(logoPixels[0]) * 2
	for i, l := range strings.Split(m.render(0, 0), "\n") {
		if strings.Contains(l, pixelGlyph) && dispW(l) != want {
			t.Errorf("logo row %d is %d wide, not %d", i, dispW(l), want)
		}
	}
}

// A toast carries an icon in its title and sits over rows that may carry more.
func TestAToastOverIconsKeepsEveryRowItsWidth(t *testing.T) {
	withWideIcons(t)
	// Enough hosts that auth glyphs sit in the rows the toast covers.
	var hosts []store.Host
	for i := range 8 {
		for _, h := range sample() {
			h.Name += string(rune(0x61 + i))
			hosts = append(hosts, h)
		}
	}
	m := sized(hosts, 90, 20)
	m.toast.show("Copied", toastInfo)
	m = settle(m)
	if !m.toast.isActive() {
		t.Fatal("setup: the toast should be up")
	}
	for i, l := range strings.Split(m.View(), "\n") {
		if dispW(l) != 90 {
			t.Errorf("row %d is %d wide, not 90: %q", i, dispW(l), l)
		}
	}
}

// A remote that prints an icon: the emulator counts one cell, the terminal
// draws two, and the cell is clipped to what the terminal draws.
func TestARemoteIconCannotBreakTheCell(t *testing.T) {
	withWideIcons(t)
	fakeSSH(t, `printf "\357\201\273 folder \357\201\273 again\n"; exec cat`)
	m := New(sample(), nil, store.DefaultConfig())
	next, _ := m.Update(tea.WindowSizeMsg{Width: 60, Height: 16})
	m = pressA(settle(next.(AppModel)), "enter", "enter")
	t.Cleanup(func() { m.ssh.stopAll() })
	s := m.ssh.currentSession()
	waitFor(t, "the icon line", func() bool {
		return strings.Contains(strings.Join(s.pty.render(58, 14), ""), "folder")
	})
	for i, l := range strings.Split(m.View(), "\n") {
		if dispW(l) != 60 {
			t.Fatalf("row %d is %d wide, not 60: %q", i, dispW(l), l)
		}
	}
}

// SSHU__ICON_WIDTH overrides the probe, which a nested sshu needs: its probe is
// answered by the outer sshu's emulator, which counts an icon as one.
func TestTheIconWidthCanBeSetByHand(t *testing.T) {
	old, oldFixed := iconCells, iconFixed
	t.Cleanup(func() { iconCells, iconFixed = old, oldFixed })
	t.Setenv("SSHU__ICON_WIDTH", "2")
	DetectIconWidth()
	if iconCells != 2 {
		t.Errorf("SSHU__ICON_WIDTH=2 should set two cells, got %d", iconCells)
	}
	// Set by hand, it also outranks what the layer above says.
	sized(sample(), 80, 24).applyNestCmd(nestCmdMsg{Hop: 0, Verb: nestVerbIcon1})
	if iconCells != 2 {
		t.Error("a width set by hand must outrank the layer above")
	}
	iconCells, iconFixed = 1, false
	// The old name is not read any more (tdp v0.1.21 D6).
	t.Setenv("SSHU__ICON_WIDTH", "")
	t.Setenv("SSHU_ICON_WIDTH", "2")
	DetectIconWidth()
	if iconCells != 1 || iconFixed {
		t.Error("SSHU_ICON_WIDTH is the old name and must not be read")
	}
	t.Setenv("SSHU__ICON_WIDTH", "7")
	DetectIconWidth() // out of range, and not a terminal: stays at the default
	if iconCells != 1 {
		t.Errorf("a bad value should be ignored, got %d", iconCells)
	}
}

func TestParseCPRColumn(t *testing.T) {
	for in, want := range map[string]int{"\x1b[1;2R": 2, "\x1b[1;3R": 3, "\x1b[24;80R": 80} {
		if col, ok := parseCPRColumn([]byte(in)); !ok || col != want {
			t.Errorf("%q: (%d, %v), want %d", in, col, ok, want)
		}
	}
	for _, bad := range []string{"\x1b[1;R", "\x1b[1;2", "garbage"} {
		if _, ok := parseCPRColumn([]byte(bad)); ok {
			t.Errorf("%q should not parse", bad)
		}
	}
}

// A nested sshu cannot find the icon width itself: its probe is answered by
// this layer's emulator, which counts an icon as one. So this layer tells it,
// on the command channel (§11.45), once it sees a sshu reporting from a cell —
// and only once while that sshu runs.
func TestAnInnerSshuIsToldTheIconWidth(t *testing.T) {
	withWideIcons(t)
	m, sink := reportingSink(t, `\033]7180;1;inner-host:0\033\\`)
	for range 3 {
		next, _ := m.Update(sshTickMsg{})
		m = next.(AppModel)
	}
	waitSink(t, sink, nestCmdEncode(0, nestVerbIcon2))
	if n := strings.Count(sinkBytes(t, sink), nestCmdEncode(0, nestVerbIcon2)); n != 1 {
		t.Errorf("told %d times; once per sshu is enough", n)
	}

	// Learning a different width from the layer above, it tells the one below.
	next, _ := m.applyNestCmd(nestCmdMsg{Hop: 0, Verb: nestVerbIcon1})
	m = next.(AppModel)
	if iconCells != 1 {
		t.Fatalf("the command should set this layer's width, got %d", iconCells)
	}
	next, _ = m.Update(sshTickMsg{})
	m = next.(AppModel)
	waitSink(t, sink, nestCmdEncode(0, nestVerbIcon1))
}

// A cell with no sshu in it is told nothing: the bytes would land in a shell.
func TestAPlainCellIsNotToldTheIconWidth(t *testing.T) {
	withWideIcons(t)
	sink := filepath.Join(t.TempDir(), "stdin")
	fakeSSH(t, `stty raw -echo 2>/dev/null; printf '$ '; exec cat > `+sink)
	m := pressA(sshApp(t, sample()), "enter", "enter")
	t.Cleanup(func() { m.ssh.stopAll() })
	waitFor(t, "the stand-in to answer", func() bool { return m.ssh.sessions[0].pty.hasSpoken() })
	for range 3 {
		next, _ := m.Update(sshTickMsg{})
		m = next.(AppModel)
	}
	m = pressA(m, "x") // something that does reach the sink, to know it is read
	waitSink(t, sink, "x")
	if strings.Contains(sinkBytes(t, sink), nestCmdOSC) {
		t.Error("a plain shell must not be sent a command")
	}
}

// The inner side: the command sets the width with no session open, and a width
// set by hand with SSHU__ICON_WIDTH stays.
func TestTheIconWidthCommandIsTakenUnlessSetByHand(t *testing.T) {
	old, oldFixed := iconCells, iconFixed
	t.Cleanup(func() { iconCells, iconFixed = old, oldFixed })
	iconCells, iconFixed = 1, false
	m := sized(sample(), 80, 24)
	next, _ := m.applyNestCmd(nestCmdMsg{Hop: 0, Verb: nestVerbIcon2})
	m = next.(AppModel)
	if iconCells != 2 {
		t.Errorf("icon2 should set two cells, got %d", iconCells)
	}
	iconFixed = true
	m.applyNestCmd(nestCmdMsg{Hop: 0, Verb: nestVerbIcon1})
	if iconCells != 2 {
		t.Error("a width set by hand must not be overridden from outside")
	}
	for _, v := range []string{nestVerbIcon1, nestVerbIcon2} {
		if got, ok := nestCmdParse("1;0;" + v); !ok || got.Verb != v {
			t.Errorf("%s should parse", v)
		}
	}
}

// tdp v0.1.21 D6: a box wider or taller than the screen — drawn at the old size
// in the frame a resize lands in — starts at 0 and is cut at the screen's edge;
// it never panics. A box the size of the screen covers it exactly. (filu's
// TestD6CompositeDispOversized, ported with the fix.)
func TestAnOverlayLargerThanTheScreenIsCutNotAPanic(t *testing.T) {
	bg := strings.TrimSuffix(strings.Repeat(strings.Repeat(".", 10)+"\n", 5), "\n") // 10 × 5
	wide := strings.Repeat("W", 14) + "\n" + strings.Repeat("W", 14)
	tall := strings.TrimSuffix(strings.Repeat("T\n", 8), "\n")
	both := strings.TrimSuffix(strings.Repeat("ABCDEFGHIJKLMN\n", 8), "\n")
	for name, fg := range map[string]string{"wider": wide, "taller": tall, "wider and taller": both} {
		for _, pos := range [][2]overlay.Position{{overlay.Center, overlay.Center}, {overlay.Center, overlay.Bottom}, {overlay.Left, overlay.Top}} {
			rows := strings.Split(compositeDisp(fg, bg, pos[0], pos[1], 0, -2), "\n")
			if len(rows) != 5 {
				t.Errorf("a %s box made %d rows, the screen has 5", name, len(rows))
			}
			for r, row := range rows {
				if dispW(row) != 10 {
					t.Errorf("a %s box: row %d is %d wide, the screen 10: %q", name, r, dispW(row), row)
				}
			}
		}
	}
	rows := strings.Split(compositeDisp(both, bg, overlay.Center, overlay.Center, 0, 0), "\n")
	if rows[0] != "ABCDEFGHIJ" || rows[4] != "ABCDEFGHIJ" {
		t.Errorf("a box larger both ways should show its top-left, cut: %q", rows)
	}
	same := strings.TrimSuffix(strings.Repeat(strings.Repeat("#", 10)+"\n", 5), "\n")
	if got := compositeDisp(same, bg, overlay.Left, overlay.Top, 0, 0); got != same {
		t.Errorf("a box the size of the screen should cover it exactly: %q", got)
	}
}

// tdp v0.1.22 D6: SSHU__ICON_WIDTH, then TERMINU__ICON_WIDTH — what a family app
// running sshu in its PTY hands down — then the probe. The family variable is
// not a hand setting: the layer above may still correct it over the channel.
func TestTheIconWidthIsReadInTheFamilyOrder(t *testing.T) {
	old, oldFixed := iconCells, iconFixed
	t.Cleanup(func() { iconCells, iconFixed = old, oldFixed })
	reset := func(own, family string) {
		iconCells, iconFixed = 1, false
		t.Setenv("SSHU__ICON_WIDTH", own)
		t.Setenv("TERMINU__ICON_WIDTH", family)
		DetectIconWidth()
	}
	reset("1", "2")
	if iconCells != 1 || !iconFixed {
		t.Errorf("SSHU__ICON_WIDTH should win: %d fixed=%v", iconCells, iconFixed)
	}
	reset("", "2")
	if iconCells != 2 || iconFixed {
		t.Errorf("TERMINU__ICON_WIDTH should be used, and not as a hand setting: %d fixed=%v", iconCells, iconFixed)
	}
	reset("9", "x")
	if iconCells != 1 || iconFixed {
		t.Errorf("values other than 1 or 2 are as if unset: %d fixed=%v", iconCells, iconFixed)
	}
}

// Every PTY sshu opens hands its own width down as TERMINU__ICON_WIDTH, once,
// replacing what sshu itself was started with.
func TestAPtyChildIsToldTheIconWidth(t *testing.T) {
	old := iconCells
	t.Cleanup(func() { iconCells = old })
	t.Setenv("TERMINU__ICON_WIDTH", "7") // inherited from whatever started sshu
	for _, n := range []int{1, 2} {
		iconCells = n
		want := "TERMINU__ICON_WIDTH=" + strconv.Itoa(n)
		for name, env := range map[string][]string{
			"ssh cell": sshEnv(sample()[0], ""),
			"editor":   editorEnv(),
		} {
			got := 0
			for _, v := range env {
				if strings.HasPrefix(v, "TERMINU__ICON_WIDTH=") {
					got++
					if v != want {
						t.Errorf("%s at %d: %q", name, n, v)
					}
				}
			}
			if got != 1 {
				t.Errorf("%s at %d: TERMINU__ICON_WIDTH appears %d times", name, n, got)
			}
		}
	}
}
