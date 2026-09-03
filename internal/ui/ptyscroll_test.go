package ui

import (
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/hinshun/vt10x"
)

// ------------------------------------------------------------ the real path
//
// These drive a stand-in ssh through the actual read loop, so the wiring is
// under test and not just the shape of it: delete the capture() call in
// readLoop and they go red. The unit tests below can only reach capture()
// directly, which means they would all still pass with the engine unplugged.

// chattySSH prints 200 uniquely-named lines and then holds the PTY open — far
// more than any cell can show at once, which is the whole precondition for
// there being a history to scroll to.
func chattySSH(t *testing.T) {
	t.Helper()
	fakeSSH(t, `i=1; while [ $i -le 200 ]; do printf 'line-%03d\n' $i; i=$((i+1)); done; exec cat`)
}

// openChatty connects to the noisy stand-in and waits until it has said more
// than fits on screen.
func openChatty(t *testing.T) AppModel {
	t.Helper()
	chattySSH(t)
	m := pressA(sshApp(t, sample()), "enter", "enter")
	if len(m.ssh.sessions) != 1 {
		t.Fatalf("expected one live session, got %d", len(m.ssh.sessions))
	}
	t.Cleanup(func() { m.ssh.stopAll() })
	waitFor(t, "the stand-in to overflow the cell", func() bool {
		return m.ssh.sessions[0].pty.scrollable()
	})
	return m
}

func screen(m AppModel) string { return ansi.Strip(m.View()) }

func TestPgUpReachesWhatScrolledOffTheGrid(t *testing.T) {
	m := openChatty(t)

	// The precondition: the oldest line is gone from the emulator. vt10x clears
	// the rows that leave the top, so without a scrollback it is unrecoverable.
	if live := screen(m); strings.Contains(live, "line-001") {
		t.Fatal("the first line should already be off the grid — the cell is not that tall")
	}

	// Enough pages to reach the far end of anything 200 lines long, then the
	// clamp holds it there.
	for range 30 {
		m = pressA(m, "pgup")
	}
	if got := screen(m); !strings.Contains(got, "line-001") {
		t.Error("paging back to the start should reach the first line the remote said")
	}

	// And back down returns to the present.
	for range 30 {
		m = pressA(m, "pgdown")
	}
	if got := screen(m); !strings.Contains(got, "line-200") {
		t.Error("paging forward should land back on live output")
	}
}

func TestTypingReturnsToLiveOutput(t *testing.T) {
	m := openChatty(t)
	for range 30 {
		m = pressA(m, "pgup")
	}
	if s := m.ssh.currentSession(); s.pty.scrolledBy() == 0 {
		t.Fatal("pgup should have moved the view back")
	}

	m = typeText(m, "x")
	if s := m.ssh.currentSession(); s.pty.scrolledBy() != 0 {
		t.Error("typing talks to the remote, so it must snap the view back to live")
	}
	if got := screen(m); strings.Contains(got, "line-001") {
		t.Error("the history should be gone from the screen once typing resumes")
	}
}

// The marker is the difference between "showing the past" and "the remote has
// gone quiet", which are the same still picture.
func TestScrolledCellSaysSoInItsTitle(t *testing.T) {
	m := openChatty(t)
	if strings.Contains(screen(m), glyphHistory) {
		t.Fatal("a live cell must not claim to be showing history")
	}
	m = pressA(m, "pgup")
	if !strings.Contains(screen(m), glyphHistory) {
		t.Error("a scrolled cell must say so in its title")
	}
	// And the distance, because "somewhere back there" is not an answer.
	n := m.ssh.currentSession().pty.scrolledBy()
	if n == 0 {
		t.Fatal("pgup moved nothing")
	}
	if !strings.Contains(screen(m), glyphHistory+" "+itoa(n)) {
		t.Errorf("the title should name the distance back, %d lines", n)
	}
}

// A key offered where it cannot move anything reads as broken the first time it
// is pressed, so the footer waits until there is history to page through.
func TestFooterOffersHistoryOnlyOnceThereIsSome(t *testing.T) {
	quiet := openOne(t) // says "$ " and nothing more
	if strings.Contains(quiet.footer(), "pgup") {
		t.Error("nothing has scrolled off yet — the footer must not offer to page back")
	}

	m := openChatty(t)
	if !strings.Contains(m.footer(), "pgup") {
		t.Error("with more said than fits, the footer must disclose the scrollback keys")
	}
	if !strings.Contains(m.footer(), "alt+esc") {
		t.Error("the way out must survive the addition")
	}
}

// --------------------------------------------------------------- alt screen

// altScreenSSH switches to the alt screen and echoes what it is sent with the
// control bytes made visible, so a forwarded key can be seen arriving.
func altScreenSSH(t *testing.T) {
	t.Helper()
	fakeSSH(t, `printf '\033[?1049hFULL-SCREEN\r\n'; exec cat -v`)
}

// A full-screen program pages with PgUp itself. Taking the key from it would
// break paging in vim, less and htop — every place the key matters most.
func TestPgUpBelongsToAFullScreenProgram(t *testing.T) {
	altScreenSSH(t)
	m := pressA(sshApp(t, sample()), "enter", "enter")
	t.Cleanup(func() { m.ssh.stopAll() })
	s := m.ssh.sessions[0]
	waitFor(t, "the stand-in to take the alt screen", func() bool { return s.pty.altScreen() })

	m = pressA(m, "pgup")
	waitFor(t, "pgup to arrive at the remote", func() bool {
		return strings.Contains(strings.Join(s.pty.render(80, 24), ""), "^[[5~")
	})
}

// ------------------------------------------------------------- the capture
//
// Byte-level rules that a stand-in cannot produce on demand.

func newTestTerm(cols, rows int) *ptyTerm {
	return &ptyTerm{
		term:        vt10x.New(vt10x.WithSize(cols, rows)),
		done:        &atomic.Bool{},
		mu:          &sync.Mutex{},
		pendingLine: &strings.Builder{},
	}
}

// feed is the two lines readLoop runs on every chunk it reads.
func (p *ptyTerm) feed(s string) {
	p.mu.Lock()
	_, _ = p.term.Write([]byte(s))
	p.capture([]byte(s))
	p.mu.Unlock()
}

func (p *ptyTerm) history() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.scrollback...)
}

// A remote sends CRLF, so \r has to wait for the byte after it before it can be
// read as anything. Treating it as an end of line on sight files every line
// twice; treating it as a redraw on sight files nothing at all.
func TestCRLFEndsALineAndALoneCRRedrawsIt(t *testing.T) {
	p := newTestTerm(40, 10)
	p.feed("first\r\n")
	p.feed("10%\r50%\r100% done\r\n")

	got := p.history()
	want := []string{"first", "100% done"}
	if len(got) != len(want) {
		t.Fatalf("history = %q, want %q", got, want)
	}
	for i := range want {
		if ansi.Strip(got[i]) != want[i] {
			t.Errorf("line %d = %q, want %q", i, ansi.Strip(got[i]), want[i])
		}
	}
}

// Colour is the reason to scroll back to a line at all — an error is red where
// it happened and has to still be red where it is read.
func TestHistoryKeepsTheStylingButNotThePureNoise(t *testing.T) {
	p := newTestTerm(40, 10)
	p.feed("\x1b[31mred alert\x1b[0m\r\n")
	p.feed("\x1b[K\x1b[1G\r\n") // a prompt repaint: control bytes, no glyphs
	p.feed("after\r\n")

	got := p.history()
	if len(got) != 2 {
		t.Fatalf("a line that renders as nothing should not be filed: %q", got)
	}
	if !strings.Contains(got[0], "\x1b[31m") {
		t.Errorf("the raw line should keep its colour: %q", got[0])
	}
}

// A full-screen program repaints its whole window on every keystroke. Capturing
// that would push thousands of frames of vim through the ring and flush the
// shell history the buffer exists to hold.
func TestAltScreenOutputStaysOutOfTheHistory(t *testing.T) {
	p := newTestTerm(40, 10)
	p.feed("before\r\n")
	p.feed("\x1b[?1049h")
	for range 50 {
		p.feed("a whole screen of vim\r\n")
	}
	p.feed("\x1b[?1049l")
	p.feed("after\r\n")

	got := p.history()
	for _, l := range got {
		if strings.Contains(l, "vim") {
			t.Fatalf("alt-screen output reached the history: %q", got)
		}
	}
	if len(got) != 2 {
		t.Errorf("history = %q, want the two lines from outside the alt screen", got)
	}
}

// A terminal forgets its history when asked to, and \x1b[3J is the ask. \x1b[2J
// erases the SCREEN — scrolling back past a `clear` to what was there before is
// exactly what a scrollback is for.
func TestOnlyAnExplicitScrollbackEraseDropsTheHistory(t *testing.T) {
	for _, tc := range []struct {
		name string
		seq  string
		kept bool
	}{
		{"erase screen", "\x1b[2J", true},
		{"cursor home then erase below", "\x1b[H\x1b[J", true},
		{"erase scrollback", "\x1b[3J", false},
		{"full reset", "\x1bc", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := newTestTerm(40, 10)
			p.feed("worth keeping\r\n")
			p.feed(tc.seq)
			if kept := len(p.history()) > 0; kept != tc.kept {
				t.Errorf("%q: history kept = %v, want %v", tc.name, kept, tc.kept)
			}
		})
	}
}

// The ring is what stops a chatty remote growing without bound for as long as
// sshu is open — sessions keep reading whether or not anything is looking.
func TestHistoryIsCappedAtTheOldestEnd(t *testing.T) {
	p := newTestTerm(40, 10)
	for i := range maxScrollback + 100 {
		p.feed("line " + itoa(i) + "\r\n")
	}
	got := p.history()
	if len(got) != maxScrollback {
		t.Fatalf("history = %d lines, want the cap of %d", len(got), maxScrollback)
	}
	if ansi.Strip(got[0]) != "line 100" {
		t.Errorf("the oldest lines should be the ones dropped, kept from %q", got[0])
	}
}

// Nothing can be scrolled to until more has been said than fits: everything the
// buffer holds is already on display.
func TestAHistoryThatFitsOnScreenDoesNotScroll(t *testing.T) {
	p := newTestTerm(40, 10)
	for i := range 8 {
		p.feed("line " + itoa(i) + "\r\n")
	}
	if p.scrollable() {
		t.Error("8 lines in a 10-row cell is not a history to page through")
	}
	p.scrollPage(-1)
	if p.scrolledBy() != 0 {
		t.Error("paging back through nothing must not move the view")
	}

	for i := range 20 {
		p.feed("more " + itoa(i) + "\r\n")
	}
	if !p.scrollable() {
		t.Fatal("28 lines in a 10-row cell is a history")
	}
	p.scrollPage(-1)
	if p.scrolledBy() != 10 {
		t.Errorf("a page back is a screenful: %d, want 10", p.scrolledBy())
	}
	for range 20 {
		p.scrollPage(-1)
	}
	if got := p.scrolledBy(); got != 18 {
		t.Errorf("paging past the oldest line should stop there: %d, want 18", got)
	}
}

// A cell that GROWS while scrolled back has less history behind it than it did,
// and an offset that was legal when it was set no longer is. The clamp on the
// keystroke cannot catch that, because no key was pressed — so the frame clamps
// too, on its way to the screen.
//
// The emulator is resized directly here: resize() also SIGWINCHes the child,
// and there is no child in a unit test.
func TestAResizeReClampsAnOffsetItInvalidates(t *testing.T) {
	p := newTestTerm(40, 10)
	for i := range 30 {
		p.feed("line " + itoa(i) + "\r\n")
	}
	for range 5 {
		p.scrollPage(-1)
	}
	if p.scrolledBy() != 20 {
		t.Fatalf("expected to be at the oldest line, got %d", p.scrolledBy())
	}

	p.mu.Lock()
	p.term.Resize(40, 28) // the cell grew: only 2 lines remain behind it
	p.mu.Unlock()

	rows := p.render(40, 28)
	if len(rows) != 28 {
		t.Fatalf("render must fill the cell: %d rows", len(rows))
	}
	// Clamped to 2, the cell shows lines 0-27 — everything it has room for, held
	// two lines back from live. Unclamped it would still be 20 back, drawing
	// lines 0-9 over eighteen rows of blank.
	if got := strings.TrimSpace(ansi.Strip(rows[27])); got != "line 27" {
		t.Errorf("the grown cell should fill with history, got %q on its last row", got)
	}
	if got := p.scrolledBy(); got != 2 {
		t.Errorf("the offset should have been re-clamped to %d, got %d", 2, got)
	}
}
