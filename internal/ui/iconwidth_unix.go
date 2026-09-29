//go:build darwin || linux

package ui

import (
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/x/term"
	"golang.org/x/sys/unix"
)

// DetectIconWidth asks the terminal how many cells it moves the cursor for a
// Nerd Font icon and sets iconCells (tdp D6). What counts is where the cursor
// lands, not how wide the glyph looks: a font whose icon spills into the next
// cell but moves the cursor one is one cell. It prints an icon at column 1 and
// asks for the cursor position (CPR). Any failure — not a tty, no reply, a
// timeout — leaves the default of 1. SSHU__ICON_WIDTH (1 or 2) overrides the
// probe, for a terminal that answers wrongly, and for a nested sshu: its probe
// is answered by the outer sshu's emulator, which counts an icon as one.
// Call once, before Bubble Tea starts reading stdin.
func DetectIconWidth() {
	if v := os.Getenv("SSHU__ICON_WIDTH"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 1 && n <= 2 {
			iconCells, iconFixed = n, true
			return
		}
	}
	in, out := os.Stdin, os.Stdout
	if !term.IsTerminal(in.Fd()) || !term.IsTerminal(out.Fd()) {
		return
	}
	state, err := term.MakeRaw(in.Fd())
	if err != nil {
		return
	}
	defer func() { _ = term.Restore(in.Fd(), state) }()

	icon := string(rune(0xf07b)) // nf-fa-folder, an ordinary icon
	if _, err := out.WriteString("\r" + icon + "\x1b[6n"); err != nil {
		return
	}
	col, ok := readCPRColumn(int(in.Fd()))
	_, _ = out.WriteString("\r\x1b[2K") // wipe the probe before the screen is taken
	if ok && col >= 2 {
		iconCells = min(col-1, 2) // the cursor started at column 1
	}
}

// readCPRColumn reads a reply "\x1b[<row>;<col>R" within a short deadline and
// returns col. Poll guards against a terminal that never answers: a missing
// reply must not hang startup.
func readCPRColumn(fd int) (int, bool) {
	deadline := time.Now().Add(200 * time.Millisecond)
	var buf []byte
	one := make([]byte, 1)
	for {
		ms := int(time.Until(deadline).Milliseconds())
		if ms <= 0 {
			return 0, false
		}
		n, err := unix.Poll([]unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}, ms)
		if err == unix.EINTR {
			continue
		}
		if err != nil || n == 0 {
			return 0, false
		}
		if _, err := unix.Read(fd, one); err != nil {
			return 0, false
		}
		buf = append(buf, one[0])
		if one[0] == 'R' {
			return parseCPRColumn(buf)
		}
		if len(buf) > 32 {
			return 0, false
		}
	}
}

// parseCPRColumn pulls col out of "…[<row>;<col>R".
func parseCPRColumn(buf []byte) (int, bool) {
	s := string(buf)
	open := strings.IndexByte(s, '[')
	semi := strings.IndexByte(s, ';')
	end := strings.IndexByte(s, 'R')
	if open < 0 || semi < open || end < semi {
		return 0, false
	}
	col, err := strconv.Atoi(s[semi+1 : end])
	if err != nil {
		return 0, false
	}
	return col, true
}
