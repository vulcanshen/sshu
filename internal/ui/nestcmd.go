package ui

import (
	"bytes"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"
)

// The other direction (§11.45). Discovery lets the outermost SEE the chain;
// this lets it ACT on a layer it can see, without the user walking in.
//
// Addressing is a hop count, the user's own proposal: the command carries an
// int, every layer decrements it, and the layer that receives 0 performs it.
// No layer needs a global view — it only has to subtract one and pass it on,
// which is why this reaches any depth without anybody knowing the depth.
//
// It rides the INPUT side, which is why it needs a filter rather than a tee:
// the parent writes keystrokes into the PTY, and Bubble Tea would decode this
// sequence into some KeyMsg of its own. sshu hands Bubble Tea a wrapped reader
// instead (tea.WithInput), pulls its own sequences out, and passes the rest
// through untouched.
const nestCmdOSC = "7181"

// nestCmdMsg is one command that arrived on stdin, already addressed to this
// layer or in need of forwarding.
type nestCmdMsg struct {
	Hop  int    // 0 means "you"; anything else is forwarded one hop shorter
	Verb string // nestVerbLock / nestVerbRelease
}

const (
	nestVerbLock    = "lock"
	nestVerbRelease = "release"
)

// maxNestHop refuses an address that could only be a mistake or a loop. Sixteen
// is the same ceiling OpenSSH puts on nested Includes, and for the same reason:
// past a certain depth a number is not a chain, it is a bug.
const maxNestHop = 16

// nestCmdEncode builds a command for a layer `hop` hops further in. The parent
// of the target sends hop 0.
func nestCmdEncode(hop int, verb string) string {
	return "\x1b]" + nestCmdOSC + ";" + nestVersion + ";" +
		strconv.Itoa(hop) + ";" + verb + "\x1b\\"
}

// nestCmdParse reads a payload with the prefix already off.
func nestCmdParse(payload string) (nestCmdMsg, bool) {
	f := strings.Split(payload, ";")
	if len(f) != 3 || f[0] != nestVersion {
		return nestCmdMsg{}, false
	}
	hop, err := strconv.Atoi(f[1])
	if err != nil || hop < 0 || hop > maxNestHop {
		return nestCmdMsg{}, false
	}
	switch f[2] {
	case nestVerbLock, nestVerbRelease:
		return nestCmdMsg{Hop: hop, Verb: f[2]}, true
	}
	return nestCmdMsg{}, false
}

// nestCmdFilter wraps the real stdin. Everything that is not one of our command
// sequences reaches Bubble Tea byte for byte and on the same read — a filter
// that held keystrokes back waiting for a sequence that never came would make
// every keypress feel broken.
type nestCmdFilter struct {
	src io.Reader
	// tty is the real stdin when there is one. It exists so this wrapper still
	// satisfies term.File: Bubble Tea only puts the terminal into raw mode for
	// an input it can take an Fd from (bubbletea/tty_unix.go), so a filter that
	// was only an io.Reader would leave the whole app line-buffered — every key
	// waiting on Enter. Nothing in a unit test catches that; the app simply
	// stops working.
	tty     *os.File
	buf     []byte
	carry   []byte
	pending []byte
	cmds    chan nestCmdMsg
	// err is held back until everything already read has been handed on. A
	// reader that returned the error first would drop whatever was still in
	// the carry — bytes the user typed, lost to a feature they were not using.
	err  error
	once sync.Once
}

// NestInput wraps stdin so a parent sshu's commands can be taken out before
// Bubble Tea decodes the bytes as keystrokes, and returns a pump that feeds
// them to the running program. Everything else passes through untouched.
func NestInput(src io.Reader) (io.Reader, func(send func(m any))) {
	f := newNestCmdFilter(src)
	return f, func(send func(m any)) {
		for c := range f.commands() {
			send(c)
		}
	}
}

func newNestCmdFilter(src io.Reader) *nestCmdFilter {
	f := &nestCmdFilter{
		src:  src,
		buf:  make([]byte, 4096),
		cmds: make(chan nestCmdMsg, 16),
	}
	f.tty, _ = src.(*os.File)
	return f
}

// Fd, Write and Close are what term.File asks for, delegated to the real
// stdin. With no file behind the reader, Fd is an fd that cannot be a terminal,
// so term.IsTerminal says no and Bubble Tea does what it would have done for a
// pipe — which is the honest answer for a test's strings.Reader.
func (f *nestCmdFilter) Fd() uintptr {
	if f.tty == nil {
		return ^uintptr(0)
	}
	return f.tty.Fd()
}

func (f *nestCmdFilter) Write(b []byte) (int, error) {
	if f.tty == nil {
		return 0, io.ErrClosedPipe
	}
	return f.tty.Write(b)
}

func (f *nestCmdFilter) Close() error {
	if f.tty == nil {
		return nil
	}
	return f.tty.Close()
}

// commands is where extracted commands come out. Buffered, and the send is
// non-blocking: a full channel drops the command rather than stalling the
// reader, because stalling the reader freezes the keyboard — and nobody would
// connect a dead keyboard to a feature they were not using.
func (f *nestCmdFilter) commands() <-chan nestCmdMsg { return f.cmds }

func (f *nestCmdFilter) Read(p []byte) (int, error) {
	for {
		if len(f.pending) > 0 {
			n := copy(p, f.pending)
			f.pending = f.pending[n:]
			return n, nil
		}
		if f.err != nil {
			f.once.Do(func() { close(f.cmds) })
			return 0, f.err
		}
		n, err := f.src.Read(f.buf)
		if n > 0 {
			var payloads []string
			var kept []byte
			var carry []byte
			payloads, carry, kept = nestCmdSplit(append(f.carry, f.buf[:n]...))
			// Copied, not aliased: carry points into the scratch slice that the
			// next read appends over.
			f.carry = append([]byte(nil), carry...)
			for _, pl := range payloads {
				if cmd, ok := nestCmdParse(pl); ok {
					select {
					case f.cmds <- cmd:
					default:
					}
				}
			}
			f.pending = append(f.pending, kept...)
			if len(f.pending) > 0 {
				continue
			}
		}
		if err != nil {
			// Flush what was being held for a sequence that will now never
			// finish: it was never ours, and it is the user's keystrokes.
			f.pending = append(f.pending, f.carry...)
			f.carry, f.err = nil, err
			continue
		}
	}
}

// partialPrefix is how many trailing bytes of buf could still be the beginning
// of prefix — the bytes worth waiting on.
//
// It never holds a lone ESC. A user pressing Esc sends exactly that one byte,
// and a filter that sat on it would swallow Esc until the NEXT key arrived —
// the most-reached-for key in the app, dead, for a feature the user is not
// using. Two bytes is where the ambiguity ends: nobody types ESC followed by
// "]", so from there it is ours to wait on.
//
// The cost is a command split by the kernel between its first and second byte,
// which would then reach Bubble Tea as keystrokes instead. A command is one
// short write into an otherwise idle PTY, so that split does not happen in
// practice — and if it did, the bytes are typed, not executed.
func partialPrefix(buf, prefix []byte) int {
	n := min(len(prefix)-1, len(buf))
	for k := n; k >= 2; k-- {
		if bytes.Equal(buf[len(buf)-k:], prefix[:k]) {
			return k
		}
	}
	return 0
}

// nestCmdSplit separates a read into the command payloads it carried, the
// partial sequence to hold for next time, and the ordinary bytes to pass on.
func nestCmdSplit(buf []byte) (payloads []string, carry, kept []byte) {
	prefix := []byte("\x1b]" + nestCmdOSC + ";")
	for {
		i := bytes.Index(buf, prefix)
		if i < 0 {
			// Hold back ONLY a tail that is genuinely the start of our prefix,
			// and only once it is unambiguous — see partialPrefix. Holding a
			// fixed number of bytes instead would stall every keystroke until
			// enough of them piled up, which is a frozen keyboard.
			h := partialPrefix(buf, prefix)
			return payloads, buf[len(buf)-h:], append(kept, buf[:len(buf)-h]...)
		}
		kept = append(kept, buf[:i]...)
		body := buf[i+len(prefix):]
		end, term := nestTerminator(body)
		if end < 0 {
			if len(body) > nestCarryMax {
				// Never terminated. Give up rather than grow without bound, and
				// let the bytes through as themselves — they were not ours.
				return payloads, nil, append(kept, buf[i:]...)
			}
			return payloads, buf[i:], kept
		}
		payloads = append(payloads, string(body[:end]))
		buf = body[end+term:]
	}
}
