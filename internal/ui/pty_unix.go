//go:build darwin || linux

package ui

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/creack/pty"
	"github.com/hinshun/vt10x"
)

// ptyTerm is one embedded terminal: a subprocess on a PTY, its output fed into a
// terminal emulator so sshu can draw it inside a panel. This is the approach kbu
// and filu settled on, with one difference that matters — sshu runs MANY of these
// at once and they outlive being looked at, so a ptyTerm keeps reading whether or
// not its session is the one on screen.
//
// It is used through a pointer: the read goroutine and the value-copied Bubble
// Tea model must share the same handles and mutex.
type ptyTerm struct {
	ptmx *os.File
	term vt10x.Terminal
	cmd  *exec.Cmd
	done *atomic.Bool
	mu   *sync.Mutex

	// exitErr is what cmd.Wait returned, read only after done flips.
	exitErr atomic.Pointer[error]

	// spoke flips at the first byte out of the subprocess. It is the difference
	// between "this terminal is empty" and "nothing has happened yet", which
	// look identical on screen and are not remotely the same thing.
	spoke atomic.Bool

	// scrollback is the history vt10x does not keep. Its grid is a fixed
	// rectangle and a row leaving the top is cleared, not stored — so the only
	// moment a line can be preserved is on the way IN, before the emulator has
	// a chance to drop it. Guarded by mu: the read goroutine appends, the
	// update loop reads.
	//
	// Lines are stored RAW, escape sequences and all, because the point is to
	// show the line again exactly as it arrived — a scrollback that strips the
	// colour out of what you scrolled back to see is answering a different
	// question.
	scrollback  []string
	pendingLine *strings.Builder
	// pendingCR remembers a trailing \r: CRLF ends a line, a lone \r is a
	// progress bar redrawing itself in place. They cannot be told apart until
	// the next byte arrives.
	pendingCR bool
	// scrollOff is how many lines back from live the panel is showing. 0 is
	// live — the only state the emulator itself knows anything about.
	scrollOff int
}

// maxScrollback caps the history a single session keeps. Sessions run whether
// or not anything is looking at them (§11.x), so this is per-session memory
// that accrues in the background — the cap is what stops a chatty remote from
// growing without bound for as long as sshu is open.
const maxScrollback = 10000

// startPty launches cmd on a PTY sized cols×rows.
func startPty(cmd *exec.Cmd, cols, rows int) (*ptyTerm, error) {
	cols, rows = max(cols, 1), max(rows, 1)
	p := &ptyTerm{
		term:        vt10x.New(vt10x.WithSize(cols, rows)),
		cmd:         cmd,
		done:        &atomic.Bool{},
		mu:          &sync.Mutex{},
		pendingLine: &strings.Builder{},
	}
	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)})
	if err != nil {
		return nil, err
	}
	p.ptmx = ptmx
	registerProc(cmd) // so no exit path can leave the child running
	go p.readLoop()
	return p, nil
}

// readLoop copies PTY output into the emulator until the pipe closes, then reaps
// the subprocess. The handles are copied out under the mutex so a concurrent
// stop() cannot nil them mid-loop.
func (p *ptyTerm) readLoop() {
	p.mu.Lock()
	ptmx, cmd, done := p.ptmx, p.cmd, p.done
	p.mu.Unlock()
	if ptmx == nil || cmd == nil {
		return
	}
	buf := make([]byte, 4096)
	for {
		n, err := ptmx.Read(buf)
		if n > 0 {
			p.mu.Lock()
			if p.term != nil {
				_, _ = p.term.Write(buf[:n])
				p.capture(buf[:n])
			}
			p.mu.Unlock()
			p.spoke.Store(true)
		}
		if err != nil {
			break
		}
	}
	err := cmd.Wait()
	deregisterProc(cmd)
	p.exitErr.Store(&err)
	done.Store(true)
}

// ---------------------------------------------------------------- scrollback

// capture files the bytes just written to the emulator into the history the
// emulator does not keep. mu is held by the caller, and term has ALREADY been
// written — the modes it reports here are the ones this chunk left it in.
//
// Nothing is captured while the alt screen is up. A full-screen program repaints
// its whole window on every keystroke, so capturing there would push thousands
// of frames of vim through the buffer and flush the shell history that is
// actually worth scrolling back to. Real terminals freeze their scrollback for
// the same reason.
func (p *ptyTerm) capture(buf []byte) {
	if p.term.Mode()&vt10x.ModeAltScreen != 0 {
		// Whatever half-line was pending belongs to the screen being left.
		p.pendingLine.Reset()
		p.pendingCR = false
		return
	}
	// A terminal forgets its history only when asked to, and \x1b[3J is the ask.
	// \x1b[2J is NOT: it erases the SCREEN, and scrolling back past a `clear` to
	// what was there before is exactly what a scrollback is for. `clear` sends
	// both, which is why it drops the history here and in the user's own terminal
	// alike — matching what their terminal does is the point.
	//
	// An erase drops what came BEFORE it, and only that. `clear && ls` arrives as
	// one chunk, so returning here would throw the ls away too and leave the panel
	// correctly reporting a history it never got the chance to keep.
	if end := eraseEnd(buf); end >= 0 {
		p.scrollback = nil
		p.scrollOff = 0
		p.pendingLine.Reset()
		p.pendingCR = false
		buf = buf[end:]
	}
	for _, b := range buf {
		if p.pendingCR {
			p.pendingCR = false
			if b == '\n' {
				p.commitLine()
				continue
			}
			// A lone \r: the line is being redrawn over itself. Keep only the
			// latest version, which is what the screen ends up showing.
			p.pendingLine.Reset()
		}
		switch b {
		case '\r':
			p.pendingCR = true
		case '\n':
			p.commitLine()
		default:
			p.pendingLine.WriteByte(b)
		}
	}
}

// eraseEnd finds where the last scrollback erase in buf ends, or -1 if there is
// none. The LAST one is what counts: everything before it has already been
// asked away, and only the bytes after it are still history.
func eraseEnd(buf []byte) int {
	end := -1
	for _, seq := range [][]byte{[]byte("\x1b[3J"), []byte("\x1bc")} {
		if i := bytes.LastIndex(buf, seq); i >= 0 && i+len(seq) > end {
			end = i + len(seq)
		}
	}
	return end
}

// commitLine moves the pending line into the ring. Blank-once-stripped lines are
// dropped: a shell prompt repaint is a dozen escape sequences and no glyphs, and
// filing those would fill the history with rows that render as nothing. The RAW
// line is what gets stored — the strip only decides whether it is worth keeping.
func (p *ptyTerm) commitLine() {
	raw := p.pendingLine.String()
	p.pendingLine.Reset()
	if strings.TrimSpace(ansi.Strip(raw)) == "" {
		return
	}
	p.scrollback = append(p.scrollback, raw)
	if n := len(p.scrollback) - maxScrollback; n > 0 {
		p.scrollback = p.scrollback[n:]
	}
}

// altScreen reports whether a full-screen program has the terminal. It is the
// one question that decides who PgUp belongs to: a program in the alt screen
// does its own paging and must keep the key.
func (p *ptyTerm) altScreen() bool {
	if p == nil || p.term == nil {
		return false
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.term.Mode()&vt10x.ModeAltScreen != 0
}

// maxScrollOff is how far back the panel can look: everything held, less the
// screenful that would be on display. Caller holds mu.
func (p *ptyTerm) maxScrollOff() int {
	_, rows := p.term.Size()
	return max(0, len(p.scrollback)-rows)
}

// scrollPage moves the view one screenful. dir -1 goes back in time, +1 comes
// forward; 0 is live. A history shorter than the screen cannot be scrolled at
// all — everything it holds is already on display.
func (p *ptyTerm) scrollPage(dir int) {
	if p == nil || p.term == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	_, rows := p.term.Size()
	p.scrollOff = clamp(p.scrollOff-dir*rows, 0, p.maxScrollOff())
}

// scrollable reports whether more has been said than currently fits on screen.
// The footer asks: a key offered for something that cannot move is a key that
// reads as broken the first time it is pressed.
func (p *ptyTerm) scrollable() bool {
	if p == nil || p.term == nil {
		return false
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.maxScrollOff() > 0
}

// scrolledBy is how many lines back the panel is showing, 0 when live. The cell
// title reads this: a panel showing history and a panel that has stopped
// updating look identical, and only one of them is a problem.
func (p *ptyTerm) scrolledBy() int {
	if p == nil || p.term == nil {
		return 0
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return min(p.scrollOff, p.maxScrollOff())
}

func (p *ptyTerm) exited() bool { return p != nil && p.done != nil && p.done.Load() }

// hasSpoken reports whether anything has come back yet.
func (p *ptyTerm) hasSpoken() bool { return p != nil && p.spoke.Load() }

// exitReason describes how the subprocess ended, for the history list. ssh's
// own exit code 255 is its "connection failed" signal, which is worth naming
// separately from a shell that simply exited non-zero.
func (p *ptyTerm) exitReason() string {
	if p == nil || !p.exited() {
		return ""
	}
	ep := p.exitErr.Load()
	if ep == nil || *ep == nil {
		return "exited 0"
	}
	var ee *exec.ExitError
	if ok := asExitError(*ep, &ee); ok {
		if code := ee.ExitCode(); code == 255 {
			return "disconnected"
		} else if code >= 0 {
			return fmt.Sprintf("exited %d", code)
		}
	}
	return "failed"
}

// screenLines is everything readable on the grid, blank rows at either end
// trimmed off.
//
// One line is not enough. A refused connection is one line, but a host key
// mismatch is fifteen — a banner, the fingerprint, the offending known_hosts
// line — and the LAST of them is only "Host key verification failed.", which is
// the one line that tells you nothing you did not already know. The fingerprint
// is the part you need, and it is in the middle.
func (p *ptyTerm) screenLines() []string {
	if p == nil || p.term == nil {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	cols, rows := p.term.Size()
	out := make([]string, 0, rows)
	for y := range rows {
		var b strings.Builder
		for x := range cols {
			if c := p.term.Cell(x, y).Char; c != 0 {
				b.WriteRune(c)
			}
		}
		out = append(out, strings.TrimRight(b.String(), " "))
	}
	// Trim the blank rows at each end; the ones in the middle are the remote's
	// own spacing and throwing them away would re-flow somebody else's message.
	for len(out) > 0 && strings.TrimSpace(out[0]) == "" {
		out = out[1:]
	}
	for len(out) > 0 && strings.TrimSpace(out[len(out)-1]) == "" {
		out = out[:len(out)-1]
	}
	return out
}

// lastWords is the single most useful line: the last non-blank one. It is the
// headline — what the toast says and what [5] leads with — while the whole
// screen goes to the log.
func (p *ptyTerm) lastWords() string {
	lines := p.screenLines()
	for i := len(lines) - 1; i >= 0; i-- {
		if l := strings.TrimSpace(lines[i]); l != "" {
			return l
		}
	}
	return ""
}

// write forwards a keystroke to the subprocess as the bytes a real terminal
// would have sent.
//
// Anything that actually reaches the remote also snaps the view back to live.
// Scrolling back is for reading; the moment you type you are talking to the far
// end again, and typing into a screen that is showing five minutes ago is how
// a command gets sent somewhere its author cannot see.
func (p *ptyTerm) write(msg tea.KeyMsg) {
	if p == nil || p.ptmx == nil {
		return
	}
	p.mu.Lock()
	appCursor := p.term != nil && p.term.Mode()&vt10x.ModeAppCursor != 0
	p.mu.Unlock()
	if raw := ptyKeyBytes(msg, appCursor); len(raw) > 0 {
		p.mu.Lock()
		p.scrollOff = 0
		p.mu.Unlock()
		_, _ = p.ptmx.Write(raw)
	}
}

// resize sends SIGWINCH and reshapes the emulator, so the remote redraws to the
// panel's new size. Every terminal resize and every layout change has to reach
// here or the remote paints to the wrong geometry.
func (p *ptyTerm) resize(cols, rows int) {
	if p == nil || p.ptmx == nil {
		return
	}
	cols, rows = max(cols, 1), max(rows, 1)
	_ = pty.Setsize(p.ptmx, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)})
	p.mu.Lock()
	if p.term != nil {
		p.term.Resize(cols, rows)
	}
	p.mu.Unlock()
}

// stop force-terminates the subprocess and releases the PTY. Idempotent.
func (p *ptyTerm) stop() {
	if p == nil {
		return
	}
	p.mu.Lock()
	ptmx, cmd := p.ptmx, p.cmd
	p.ptmx, p.cmd = nil, nil
	p.mu.Unlock()

	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
	if ptmx != nil {
		_ = ptmx.Close()
	}
}

// render draws the emulator grid as h lines of exactly w cells. The emulator is
// kept at the panel's size, so this is a straight copy — but it still clamps,
// because a resize and a frame can interleave.
//
// Scrolled back, it draws the stored lines instead. The emulator is untouched by
// that: it keeps receiving whatever the remote sends, so coming back to live
// shows the present rather than a replay of what was missed.
func (p *ptyTerm) render(w, h int) []string {
	blank := strings.Repeat(" ", max(w, 0))
	out := make([]string, 0, h)
	if p == nil || p.term == nil {
		for range h {
			out = append(out, blank)
		}
		return out
	}

	p.mu.Lock()
	// Re-clamp here as well as on the keystroke: a resize between the two
	// shrinks the ceiling under an offset that was legal when it was set.
	p.scrollOff = min(p.scrollOff, p.maxScrollOff())
	if p.scrollOff > 0 {
		end := len(p.scrollback) - p.scrollOff
		for _, l := range p.scrollback[max(0, end-h):end] {
			s := clipANSI(l, w)
			if pad := w - dispW(s); pad > 0 {
				s += strings.Repeat(" ", pad)
			}
			out = append(out, s)
		}
		p.mu.Unlock()
		for len(out) < h {
			out = append(out, blank)
		}
		return out
	}
	out = p.gridLines(w, h, true)
	p.mu.Unlock()
	return out
}

// gridLines draws the emulator grid as h lines of exactly w cells. The caller
// holds mu.
//
// withCursor draws the terminal's own cursor as a reversed cell. Selection mode
// passes false: it freezes a page and puts its OWN cursor on it, and two cursors
// on one screen is one too many — the remote's would be pointing at where
// typing would go, which is the one thing that cannot happen there.
func (p *ptyTerm) gridLines(w, h int, withCursor bool) []string {
	blank := strings.Repeat(" ", max(w, 0))
	out := make([]string, 0, h)
	cols, rows := p.term.Size()
	cursorX, cursorY := -1, -1
	if withCursor && p.term.CursorVisible() {
		c := p.term.Cursor()
		cursorX, cursorY = c.X, c.Y
	}
	for y := range min(rows, h) {
		var line strings.Builder
		for x := range min(cols, w) {
			line.WriteString(renderGlyph(p.term.Cell(x, y), x == cursorX && y == cursorY))
		}
		// Clip, then pad. The emulator counts one grid cell per rune, but a
		// terminal draws an emoji or a CJK glyph two cells wide — so a remote
		// prompt with an emoji in it renders WIDER than the grid says and shoves
		// the panel border off the right edge. Clipping costs the last column or
		// two of such a line; not clipping costs the whole frame.
		s := clipANSI(line.String(), w)
		if pad := w - dispW(s); pad > 0 {
			s += strings.Repeat(" ", pad)
		}
		out = append(out, s)
	}
	for len(out) < h {
		out = append(out, blank)
	}
	return out
}

// copySnapshot freezes what selection mode works on: the history, with the
// screenful the panel is actually showing pasted over its tail.
//
// The tail is REPLACED, not appended. The scrollback is filled on the way IN,
// so it already holds the lines that are still on screen — appending would show
// every visible line twice. Leaving the screen out instead would lose the parts
// that never reach the ring: the half-line a shell prompt ends on, and any line
// commitLine declined to file. Replacing is the same assumption PgUp has always
// made, that the screen is the last h entries; here it is finally written down.
//
// In the alt screen the grid is the whole buffer. Nothing is captured while a
// full-screen program owns the terminal (§11.19), so there is no history to put
// above it — and vim's own window is exactly what somebody in there wants to
// copy out of.
func (p *ptyTerm) copySnapshot(w, h int) []string {
	if p == nil || p.term == nil || w <= 0 || h <= 0 {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	screen := p.gridLines(w, h, false)
	if p.term.Mode()&vt10x.ModeAltScreen != 0 {
		return screen
	}
	out := make([]string, 0, len(p.scrollback)+h)
	for _, l := range p.scrollback[:max(0, len(p.scrollback)-h)] {
		s := clipANSI(l, w)
		if pad := w - dispW(s); pad > 0 {
			s += strings.Repeat(" ", pad)
		}
		out = append(out, s)
	}
	return append(out, screen...)
}

// vt10x attribute bit positions (fixed by iota order in vt10x/state.go).
const (
	vtAttrReverse   int16 = 1
	vtAttrUnderline int16 = 2
	vtAttrBold      int16 = 4
	vtAttrItalic    int16 = 16
	vtAttrAny             = vtAttrReverse | vtAttrUnderline | vtAttrBold | vtAttrItalic
)

// renderGlyph maps one emulator cell to a styled rune. A default-everything cell
// emits the raw rune — this is the hot path (one call per cell per frame), and a
// lipgloss allocation there is the difference between smooth and not.
func renderGlyph(g vt10x.Glyph, isCursor bool) string {
	ch := string(g.Char)
	if g.Char == 0 {
		ch = " "
	}
	defaultFG := g.FG == vt10x.DefaultFG
	defaultBG := g.BG == vt10x.DefaultBG
	hasAttrs := g.Mode&vtAttrAny != 0
	if !isCursor && defaultFG && defaultBG && !hasAttrs {
		return ch
	}
	style := lipgloss.NewStyle()
	if !defaultFG {
		if fg, ok := vtColorToLipgloss(g.FG); ok {
			style = style.Foreground(fg)
		}
	}
	if !defaultBG {
		if bg, ok := vtColorToLipgloss(g.BG); ok {
			style = style.Background(bg)
		}
	}
	if g.Mode&vtAttrBold != 0 {
		style = style.Bold(true)
	}
	if g.Mode&vtAttrUnderline != 0 {
		style = style.Underline(true)
	}
	if g.Mode&vtAttrItalic != 0 {
		style = style.Italic(true)
	}
	reverse := g.Mode&vtAttrReverse != 0
	if isCursor {
		reverse = !reverse
	}
	if reverse {
		style = style.Reverse(true)
	}
	return style.Render(ch)
}

// vtColorToLipgloss maps a vt10x colour: 0–255 palette index, 256+ true-colour
// RGB. Defaults return ok=false so the host's own default applies.
func vtColorToLipgloss(c vt10x.Color) (lipgloss.Color, bool) {
	if c == vt10x.DefaultFG || c == vt10x.DefaultBG || c == vt10x.DefaultCursor {
		return "", false
	}
	u := uint32(c)
	if u < 256 {
		return lipgloss.Color(fmt.Sprintf("%d", u)), true
	}
	return lipgloss.Color(fmt.Sprintf("#%02X%02X%02X", (u>>16)&0xFF, (u>>8)&0xFF, u&0xFF)), true
}

// ptyKeyBytes converts a Bubble Tea KeyMsg into the raw bytes a real terminal
// writes to a process's stdin. appCursor selects the DEC application-cursor
// sequences when the running program set DECCKM (vim's normal mode).
//
// The Alt prefix matters more than it looks: Bubble Tea reports ESC-then-key as
// Alt, so without re-encoding it here every Esc a user presses inside vim over
// ssh would arrive as a bare key and normal mode would never engage.
func ptyKeyBytes(msg tea.KeyMsg, appCursor bool) []byte {
	b := ptyKeyBytesPlain(msg, appCursor)
	if b == nil {
		return nil
	}
	if msg.Alt {
		return append([]byte{'\x1b'}, b...)
	}
	return b
}

func ptyKeyBytesPlain(msg tea.KeyMsg, appCursor bool) []byte {
	if msg.Type == tea.KeyRunes {
		return []byte(string(msg.Runes))
	}
	if appCursor {
		if b, ok := ptyKeyBytesAppCursorMap[msg.Type]; ok {
			return b
		}
	}
	if b, ok := ptyKeyBytesMap[msg.Type]; ok {
		return b
	}
	return nil
}

var ptyKeyBytesMap = map[tea.KeyType][]byte{
	tea.KeyEnter: {'\r'}, tea.KeyTab: {'\t'}, tea.KeyBackspace: {'\x7f'},
	tea.KeyDelete: {'\x1b', '[', '3', '~'}, tea.KeySpace: {' '}, tea.KeyEscape: {'\x1b'},
	tea.KeyUp: {'\x1b', '[', 'A'}, tea.KeyDown: {'\x1b', '[', 'B'},
	tea.KeyRight: {'\x1b', '[', 'C'}, tea.KeyLeft: {'\x1b', '[', 'D'},
	tea.KeyHome: {'\x1b', '[', 'H'}, tea.KeyEnd: {'\x1b', '[', 'F'},
	tea.KeyPgUp: {'\x1b', '[', '5', '~'}, tea.KeyPgDown: {'\x1b', '[', '6', '~'},
	tea.KeyShiftTab: {'\x1b', '[', 'Z'},
	tea.KeyCtrlA:    {'\x01'}, tea.KeyCtrlB: {'\x02'}, tea.KeyCtrlC: {'\x03'},
	tea.KeyCtrlD: {'\x04'}, tea.KeyCtrlE: {'\x05'}, tea.KeyCtrlF: {'\x06'},
	tea.KeyCtrlG: {'\x07'}, tea.KeyCtrlH: {'\x08'}, tea.KeyCtrlK: {'\x0b'},
	tea.KeyCtrlL: {'\x0c'}, tea.KeyCtrlN: {'\x0e'}, tea.KeyCtrlO: {'\x0f'},
	tea.KeyCtrlP: {'\x10'}, tea.KeyCtrlR: {'\x12'}, tea.KeyCtrlU: {'\x15'},
	tea.KeyCtrlV: {'\x16'}, tea.KeyCtrlW: {'\x17'}, tea.KeyCtrlX: {'\x18'},
	tea.KeyCtrlY: {'\x19'}, tea.KeyCtrlZ: {'\x1a'},
	tea.KeyCtrlLeft: {'\x1b', '[', '1', ';', '5', 'D'}, tea.KeyCtrlRight: {'\x1b', '[', '1', ';', '5', 'C'},
	tea.KeyShiftLeft: {'\x1b', '[', '1', ';', '2', 'D'}, tea.KeyShiftRight: {'\x1b', '[', '1', ';', '2', 'C'},
	tea.KeyF1: {'\x1b', 'O', 'P'}, tea.KeyF2: {'\x1b', 'O', 'Q'},
	tea.KeyF3: {'\x1b', 'O', 'R'}, tea.KeyF4: {'\x1b', 'O', 'S'},
	tea.KeyF5: {'\x1b', '[', '1', '5', '~'}, tea.KeyF6: {'\x1b', '[', '1', '7', '~'},
	tea.KeyF7: {'\x1b', '[', '1', '8', '~'}, tea.KeyF8: {'\x1b', '[', '1', '9', '~'},
	tea.KeyF9: {'\x1b', '[', '2', '0', '~'}, tea.KeyF10: {'\x1b', '[', '2', '1', '~'},
	tea.KeyF11: {'\x1b', '[', '2', '3', '~'}, tea.KeyF12: {'\x1b', '[', '2', '4', '~'},
}

var ptyKeyBytesAppCursorMap = map[tea.KeyType][]byte{
	tea.KeyUp: {'\x1b', 'O', 'A'}, tea.KeyDown: {'\x1b', 'O', 'B'},
	tea.KeyRight: {'\x1b', 'O', 'C'}, tea.KeyLeft: {'\x1b', 'O', 'D'},
}
