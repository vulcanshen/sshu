package ui

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
)

// drainFilter reads until EOF and returns what came through, plus the commands
// the filter pulled out on the way. The read buffer is deliberately smaller
// than a command, so the caller's buffer size cannot be what makes it work.
func drainFilter(t *testing.T, in string) (string, []nestCmdMsg) {
	t.Helper()
	f := newNestCmdFilter(strings.NewReader(in))
	var out bytes.Buffer
	buf := make([]byte, 7)
	for {
		n, err := f.Read(buf)
		out.Write(buf[:n])
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	var cmds []nestCmdMsg
	for c := range f.commands() {
		cmds = append(cmds, c)
	}
	return out.String(), cmds
}

// The property that matters most: this filter sits on every keystroke the user
// will ever type. Anything that is not a command has to come out byte for byte.
func TestOrdinaryInputPassesThroughUntouched(t *testing.T) {
	for _, in := range []string{
		"a",
		"hello world",
		"\x1b",               // a lone Esc
		"\x1b\r",             // Alt+Enter, the layer chord
		"\x1b[A",             // an arrow
		"\x1b]0;a title\x07", // an OSC that is not ours
		"\x1b]718",           // our prefix, unfinished and never completed
	} {
		got, cmds := drainFilter(t, in)
		if got != in {
			t.Errorf("input %q came out as %q", in, got)
		}
		if len(cmds) != 0 {
			t.Errorf("input %q produced commands %+v", in, cmds)
		}
	}
}

// A lone Esc must be handed on IMMEDIATELY, not held back waiting for bytes
// that may never come — it is the most-reached-for key in the app, and a
// filter that sat on it would break it for everyone not using nesting.
func TestALoneEscIsNotHeldBack(t *testing.T) {
	pr, pw := io.Pipe()
	f := newNestCmdFilter(pr)
	go func() { _, _ = pw.Write([]byte("\x1b")) }()

	done := make(chan string, 1)
	go func() {
		buf := make([]byte, 8)
		n, _ := f.Read(buf)
		done <- string(buf[:n])
	}()
	select {
	case got := <-done:
		if got != "\x1b" {
			t.Errorf("got %q, want a lone Esc", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Esc was held back — the keyboard would look frozen")
	}
	_ = pw.Close()
}

// A command comes out of the stream and the surrounding keystrokes do not.
func TestACommandIsLiftedOutOfTheStream(t *testing.T) {
	got, cmds := drainFilter(t, "ab"+nestCmdEncode(2, nestVerbLock)+"cd")
	if got != "abcd" {
		t.Errorf("keystrokes around the command came out as %q", got)
	}
	if len(cmds) != 1 || cmds[0].Hop != 2 || cmds[0].Verb != nestVerbLock {
		t.Fatalf("commands: %+v", cmds)
	}
}

// splitAt hands out the input in exactly two reads, breaking at k.
type splitAt struct {
	s    string
	k, i int
}

func (r *splitAt) Read(p []byte) (int, error) {
	if r.i >= len(r.s) {
		return 0, io.EOF
	}
	end := r.k
	if r.i >= r.k {
		end = len(r.s)
	}
	n := copy(p, r.s[r.i:end])
	r.i += n
	return n, nil
}

func filterSplit(t *testing.T, raw string, k int) (string, []nestCmdMsg) {
	t.Helper()
	f := newNestCmdFilter(&splitAt{s: raw, k: k})
	var out bytes.Buffer
	buf := make([]byte, 64)
	for {
		n, err := f.Read(buf)
		out.Write(buf[:n])
		if err != nil {
			break
		}
	}
	var cmds []nestCmdMsg
	for c := range f.commands() {
		cmds = append(cmds, c)
	}
	return out.String(), cmds
}

// A command survives a read boundary falling ANYWHERE inside it — except the
// one place the design gives up on, which the next test pins.
func TestACommandSplitAcrossReadsStillArrives(t *testing.T) {
	raw := "x" + nestCmdEncode(0, nestVerbRelease) + "y"
	const escBoundary = 2 // just after the command's leading ESC
	for k := 1; k < len(raw); k++ {
		if k == escBoundary {
			continue
		}
		out, cmds := filterSplit(t, raw, k)
		if out != "xy" {
			t.Errorf("split at %d: passthrough was %q, want %q", k, out, "xy")
		}
		if len(cmds) != 1 || cmds[0].Verb != nestVerbRelease {
			t.Errorf("split at %d: commands %+v", k, cmds)
		}
	}
}

// The one boundary that is given up: a read ending on the command's leading
// ESC. That ESC is released immediately so the Esc KEY works, which means the
// rest of the sequence arrives as ordinary bytes.
//
// The point of this test is that it DEGRADES rather than corrupts: every byte
// still comes through, in order, so the worst case is characters typed at the
// far end — never a mangled stream and never a command half-executed.
func TestTheOneGivenUpBoundaryDegradesToPassthrough(t *testing.T) {
	raw := "x" + nestCmdEncode(0, nestVerbRelease) + "y"
	out, cmds := filterSplit(t, raw, 2)
	if len(cmds) != 0 {
		t.Errorf("the command should not have been recognised here: %+v", cmds)
	}
	if out != raw {
		t.Errorf("every byte must still come through in order: got %q want %q", out, raw)
	}
}

// Nothing malformed may be executed. A verb this build does not know, a
// negative or absurd hop, a version from the future — all refused.
func TestMalformedCommandsAreNotExecuted(t *testing.T) {
	for _, payload := range []string{
		"1;0;destroy",   // a verb that is not ours
		"1;-1;lock",     // a hop that cannot exist
		"1;99;lock",     // past the ceiling
		"9;0;lock",      // a version from the future
		"1;0",           // too few fields
		"1;0;lock;more", // too many
		"1;x;lock",      // a hop that is not a number
	} {
		if _, ok := nestCmdParse(payload); ok {
			t.Errorf("payload %q was accepted and must not be", payload)
		}
	}
}

// The hop is the whole addressing scheme, so the round trip has to hold for
// every depth the ceiling allows.
func TestEveryReachableHopRoundTrips(t *testing.T) {
	for hop := 0; hop <= maxNestHop; hop++ {
		_, cmds := drainFilter(t, nestCmdEncode(hop, nestVerbLock))
		if len(cmds) != 1 || cmds[0].Hop != hop {
			t.Fatalf("hop %d: %+v", hop, cmds)
		}
	}
}

// The two channels use different numbers on purpose: a report must never be
// read as a command, or a remote's output could drive the layer above it.
func TestAReportIsNotACommand(t *testing.T) {
	report := nestEncode([]nestLayer{{Host: "h", Locked: true}})
	got, cmds := drainFilter(t, report)
	if len(cmds) != 0 {
		t.Errorf("a report was executed as a command: %+v", cmds)
	}
	if got != report {
		t.Errorf("a report must pass through untouched, got %q", got)
	}
}

// reportingSink is a stand-in remote that does both halves at once: it enters
// the alt screen and announces a chain (so the menu has an inner layer to
// address), then records everything written to it (so a command sent inward
// can be read back as bytes).
func reportingSink(t *testing.T, announce string) (AppModel, string) {
	t.Helper()
	sink := filepath.Join(t.TempDir(), "stdin")
	fakeSSH(t, `stty raw -echo 2>/dev/null; printf '\033[?1049h'; printf '`+
		announce+`'; exec cat > `+sink)
	m := pressA(sshApp(t, sample()), "enter", "enter")
	t.Cleanup(func() { m.ssh.stopAll() })
	waitFor(t, "the stand-in to report", func() bool {
		_, ok := m.ssh.sessions[0].pty.nestChain()
		return ok
	})
	return m, sink
}

// The whole point of the addressing: pick an inner layer in the OUTERMOST menu
// and the command leaves for it. Nothing is opened anywhere else and the user
// never walks in.
func TestChoosingAnInnerLayerSendsACommand(t *testing.T) {
	m, sink := reportingSink(t, `\033]7180;1;inner-host:0\033\\`)

	m = pressA(m, "alt+enter")
	if !m.lockMenu.isActive() {
		t.Fatal("alt+enter should open the lock menu")
	}
	v := ansi.Strip(m.lockMenu.view())
	// The row NAMES THE MACHINE that layer runs on — which is the host this
	// sshu connected to, not the host the inner one reported. (The reported
	// host names the layer BELOW it, one row further down.)
	if !strings.Contains(v, "inner layers") || !strings.Contains(v, sample()[0].Name) {
		t.Fatalf("the inner layer should be a row here:\n%s", v)
	}
	// Down past the two fixed rows onto the inner layer, then run it.
	m = pressA(m, "j", "j", "enter")

	// Hop 0: the layer immediately below this one. Unlocked, so it is a lock.
	want := nestCmdEncode(0, nestVerbLock)
	waitSink(t, sink, want)
	if m.lockMenu.isActive() {
		t.Error("committing a layer row should close the menu")
	}
	// And nothing was changed HERE — the row addressed somewhere else.
	if m.ssh.sessions[0].locked {
		t.Error("addressing an inner layer must not lock this one")
	}
}

// A row for a layer that is already locked sends the opposite verb: the row is
// a toggle named by the side it goes to (§11.30).
func TestALockedInnerLayerIsOfferedTheRelease(t *testing.T) {
	m, sink := reportingSink(t, `\033]7180;1;inner-host:1\033\\`)
	m = pressA(m, "alt+enter", "j", "j", "enter")
	waitSink(t, sink, nestCmdEncode(0, nestVerbRelease))
}

// Hop 0 is us. Anything else is passed one hop shorter, so a chain of any
// depth is reachable without anybody knowing the depth.
func TestACommandForUsIsAppliedAndOneForDeeperIsForwarded(t *testing.T) {
	m, sink := reportingSink(t, `\033]7180;1;inner-host:0\033\\`)

	// Addressed to this layer.
	mm, _ := m.applyNestCmd(nestCmdMsg{Hop: 0, Verb: nestVerbLock})
	m = mm.(AppModel)
	if !m.ssh.sessions[0].locked {
		t.Error("hop 0 should have locked this layer")
	}

	// Addressed deeper: forwarded, one shorter, and NOT applied here.
	mm, _ = m.applyNestCmd(nestCmdMsg{Hop: 3, Verb: nestVerbRelease})
	m = mm.(AppModel)
	if !m.ssh.sessions[0].locked {
		t.Error("a command for a deeper layer must not act here")
	}
	waitSink(t, sink, nestCmdEncode(2, nestVerbRelease))
}

// The subtle one. A locked cell passes KEYS through — but a command is not a
// key, and a lock that swallowed the addressing would make every layer under a
// locked one unreachable, which is exactly the state a user locks their way
// into.
func TestALockedLayerStillForwardsCommands(t *testing.T) {
	m, sink := reportingSink(t, `\033]7180;1;inner-host:0\033\\`)
	m.ssh.sessions[0].locked = true

	mm, _ := m.applyNestCmd(nestCmdMsg{Hop: 2, Verb: nestVerbLock})
	m = mm.(AppModel)
	waitSink(t, sink, nestCmdEncode(1, nestVerbLock))
}

// The regression that unit tests cannot see. Bubble Tea only puts the terminal
// into raw mode for an input it can take an Fd from — so a filter that was
// merely an io.Reader left the whole app line-buffered, every key waiting on
// Enter, and nothing in the suite noticed. It took running the binary.
//
// The compile-time assertion is the real guard: it is term.File's shape, and
// dropping any of the three methods stops the package building.
var _ interface {
	io.ReadWriteCloser
	Fd() uintptr
} = (*nestCmdFilter)(nil)

func TestTheFilterStillLooksLikeTheTerminalItWraps(t *testing.T) {
	f := newNestCmdFilter(os.Stdin)
	if f.Fd() != os.Stdin.Fd() {
		t.Errorf("Fd() = %d, want stdin's %d — raw mode would be skipped",
			f.Fd(), os.Stdin.Fd())
	}
	// And with no file behind it, an fd that cannot be a terminal, so the
	// answer for a pipe stays honest rather than claiming stdin's.
	g := newNestCmdFilter(strings.NewReader("x"))
	if g.Fd() == os.Stdin.Fd() {
		t.Error("a non-file reader must not claim to be stdin")
	}
}

// Once a layer can be ADDRESSED, the chord stops broadcasting to it: its state
// is a row in this menu, and a second menu on the far side would be a popup
// nobody asked for on a layer the user can already drive from here.
func TestTheChordStopsBroadcastingOnceALayerCanBeAddressed(t *testing.T) {
	m, sink := reportingSink(t, `\033]7180;1;inner-host:0\033\\`)
	before := sinkBytes(t, sink)

	m = pressA(m, "alt+enter")
	if !m.lockMenu.isActive() {
		t.Fatal("the menu should still open here")
	}
	// Give a forward every chance to show up before concluding it did not.
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if sinkBytes(t, sink) != before {
			t.Fatalf("the chord was forwarded to a layer that can be addressed: %q",
				strings.TrimPrefix(sinkBytes(t, sink), before))
		}
	}
}

// And it still broadcasts to a cell that said nothing — a plain shell, or a
// sshu too old to report. That is the only way into such a layer, so losing it
// would strand exactly the users an upgrade should not strand.
func TestTheChordStillBroadcastsToASilentCell(t *testing.T) {
	m, sink := lockApp(t)
	if _, reported := m.ssh.sessions[0].pty.nestChain(); reported {
		t.Fatal("setup: this stand-in must not report")
	}
	m = pressA(m, "alt+enter")
	waitSink(t, sink, "\x1b\r")
}
