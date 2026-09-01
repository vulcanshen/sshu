//go:build darwin || linux

package ui

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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
}

// startPty launches cmd on a PTY sized cols×rows.
func startPty(cmd *exec.Cmd, cols, rows int) (*ptyTerm, error) {
	cols, rows = max(cols, 1), max(rows, 1)
	p := &ptyTerm{
		term: vt10x.New(vt10x.WithSize(cols, rows)),
		cmd:  cmd,
		done: &atomic.Bool{},
		mu:   &sync.Mutex{},
	}
	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)})
	if err != nil {
		return nil, err
	}
	p.ptmx = ptmx
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
			}
			p.mu.Unlock()
			p.spoke.Store(true)
		}
		if err != nil {
			break
		}
	}
	err := cmd.Wait()
	p.exitErr.Store(&err)
	done.Store(true)
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

// lastWords is the last non-blank line on the grid.
//
// It exists because ssh's failures are printed and then thrown away: the reaper
// drops the emulator when a session ends, and exitReason can only say "exited
// 255" — while the line ssh actually wrote says "Connection refused" or
// "Permission denied" or which host key changed. That line is the whole of what
// a person needs, and it was being discarded a tick after it arrived.
func (p *ptyTerm) lastWords() string {
	if p == nil || p.term == nil {
		return ""
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	cols, rows := p.term.Size()
	for y := rows - 1; y >= 0; y-- {
		var b strings.Builder
		for x := range cols {
			if c := p.term.Cell(x, y).Char; c != 0 {
				b.WriteRune(c)
			}
		}
		if line := strings.TrimSpace(b.String()); line != "" {
			return line
		}
	}
	return ""
}

// write forwards a keystroke to the subprocess as the bytes a real terminal
// would have sent.
func (p *ptyTerm) write(msg tea.KeyMsg) {
	if p == nil || p.ptmx == nil {
		return
	}
	p.mu.Lock()
	appCursor := p.term != nil && p.term.Mode()&vt10x.ModeAppCursor != 0
	p.mu.Unlock()
	if raw := ptyKeyBytes(msg, appCursor); len(raw) > 0 {
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
	cols, rows := p.term.Size()
	cursorX, cursorY := -1, -1
	if p.term.CursorVisible() {
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
	p.mu.Unlock()

	for len(out) < h {
		out = append(out, blank)
	}
	return out
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
