package ui

import (
	"bytes"
	"strings"
)

// A nested sshu announces itself to its parent the way a program has always
// told a terminal its title: an escape sequence riding in its ordinary output,
// invisible to anything that does not recognise it (§11.44).
//
// The carrier is a private OSC. Verified inert against tmux 3.7b and against
// sshu's own emulator (oscprobe_test) — a terminal that does not know the
// number consumes it to the terminator and prints nothing. APC was rejected
// (kitty's graphics protocol claims it), OSC 777 was rejected (rxvt's notify
// convention owns it), and DCS was rejected because tmux's allow-passthrough
// setting changes what happens to it.
//
// Nobody has to know how deep they are. Each sshu reports its OWN focused cell
// and appends what its child told it, so a parent assembles the whole chain
// from one report and the outermost counts the depths itself.
const nestOSC = "7180"

// nestVersion guards the format. A parent that sees a version it does not know
// ignores the report rather than guessing at fields — an old sshu inside a new
// one (or the reverse) then degrades to knowing nothing, which is exactly what
// today's behaviour already is.
const nestVersion = "1"

// nestLayer is one sshu in the chain: the machine its focused cell leads to,
// and whether that cell is passing every key through.
type nestLayer struct {
	Host   string
	Locked bool
}

// nestEncode builds the announcement a sshu appends to its own frames. An empty
// chain still produces a sequence: "I am sshu and I have nothing below me" is a
// different fact from silence, and the parent needs to tell them apart.
func nestEncode(layers []nestLayer) string {
	var b strings.Builder
	b.WriteString("\x1b]" + nestOSC + ";" + nestVersion)
	for _, l := range layers {
		b.WriteString(";" + nestField(l.Host) + ":")
		if l.Locked {
			b.WriteString("1")
		} else {
			b.WriteString("0")
		}
	}
	b.WriteString("\x1b\\")
	return b.String()
}

// nestField makes a host name safe to carry. The separators and every control
// byte come out — a name is display text here, so losing a character is better
// than a name that can forge a field or cut the sequence short.
func nestField(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == ';' || r == ':':
			// separators — drop
		case r < 0x20 || r == 0x7f:
			// control bytes — drop
		default:
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return "?"
	}
	return b.String()
}

// nestCarryMax bounds the partial sequence held between reads. A stream that
// opens our OSC and never terminates it must not grow the buffer without end,
// and 4KB is far past any legitimate chain.
const nestCarryMax = 4096

// nestScanner pulls announcements out of a PTY's output as it streams past. It
// is fed the same bytes the emulator gets, so a report can arrive split across
// reads — which is why it carries a remainder rather than scanning each read on
// its own.
type nestScanner struct {
	carry   []byte
	layers  []nestLayer
	present bool
}

// chain is the last report seen, and whether one has ever arrived. A cell with
// no report holds a plain shell (or a sshu too old to speak), and the caller
// must be able to tell that from a sshu reporting an empty chain.
func (s *nestScanner) chain() ([]nestLayer, bool) { return s.layers, s.present }

// reset forgets what a cell reported. The session it described is gone, and a
// stale chain would have the menu offering layers that no longer exist.
func (s *nestScanner) reset() {
	s.carry, s.layers, s.present = nil, nil, false
}

func (s *nestScanner) feed(b []byte) {
	prefix := []byte("\x1b]" + nestOSC + ";")
	s.carry = append(s.carry, b...)
	for {
		i := bytes.Index(s.carry, prefix)
		if i < 0 {
			// Keep only enough tail that a prefix split across this read and
			// the next can still be found.
			if n := len(prefix) - 1; len(s.carry) > n {
				s.carry = append(s.carry[:0], s.carry[len(s.carry)-n:]...)
			}
			return
		}
		body := s.carry[i+len(prefix):]
		end, term := nestTerminator(body)
		if end < 0 {
			// Incomplete — hold from the prefix and wait for more, unless the
			// hold has gone past anything plausible.
			s.carry = append(s.carry[:0], s.carry[i:]...)
			if len(s.carry) > nestCarryMax {
				s.carry = nil
			}
			return
		}
		s.parse(string(body[:end]))
		s.carry = append(s.carry[:0], body[end+term:]...)
	}
}

// nestTerminator finds ST (ESC backslash) or BEL, returning where the payload
// ends and how many bytes the terminator took. BEL is accepted because it is
// what older terminals emit and what plenty of software still writes.
func nestTerminator(b []byte) (end, term int) {
	for k := 0; k < len(b); k++ {
		switch {
		case b[k] == 0x07:
			return k, 1
		case b[k] == 0x1b && k+1 < len(b) && b[k+1] == '\\':
			return k, 2
		case b[k] == 0x1b:
			// An ESC that is not ST means the sequence was abandoned — a real
			// terminal would resynchronise here, and so does this.
			return -1, 0
		}
	}
	return -1, 0
}

// parse reads "<version>;<host>:<0|1>;..." — the prefix is already off.
func (s *nestScanner) parse(payload string) {
	fields := strings.Split(payload, ";")
	if len(fields) == 0 || fields[0] != nestVersion {
		return
	}
	layers := make([]nestLayer, 0, len(fields)-1)
	for _, f := range fields[1:] {
		at := strings.LastIndex(f, ":")
		if at < 0 {
			continue
		}
		layers = append(layers, nestLayer{
			Host:   f[:at],
			Locked: f[at+1:] == "1",
		})
	}
	s.layers, s.present = layers, true
}
