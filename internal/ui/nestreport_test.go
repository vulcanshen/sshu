package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// The round trip is the contract: what one sshu writes, its parent reads back
// as the same chain. Both halves live here, so a change to the format that
// breaks one and not the other cannot pass.
func TestAChainSurvivesTheRoundTrip(t *testing.T) {
	want := []nestLayer{{Host: "self-a", Locked: false}, {Host: "self-b", Locked: true}}
	var s nestScanner
	s.feed([]byte("noise before" + nestEncode(want) + "noise after"))

	got, ok := s.chain()
	if !ok {
		t.Fatal("a report was written and none was read")
	}
	if len(got) != len(want) {
		t.Fatalf("chain length %d, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("layer %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

// A report crosses a PTY in whatever sized reads the kernel feels like, so it
// arrives split — including through the middle of the escape sequence. Feeding
// it one byte at a time is the worst case and the honest one.
func TestAReportSplitAcrossEveryByteStillArrives(t *testing.T) {
	raw := "before" + nestEncode([]nestLayer{{Host: "self-a", Locked: true}}) + "after"
	var s nestScanner
	for i := 0; i < len(raw); i++ {
		s.feed([]byte{raw[i]})
	}
	got, ok := s.chain()
	if !ok {
		t.Fatal("a report split byte-by-byte was lost")
	}
	if len(got) != 1 || got[0].Host != "self-a" || !got[0].Locked {
		t.Errorf("got %+v", got)
	}
}

// Both terminators, because BEL is what plenty of software still writes.
func TestBothTerminatorsAreAccepted(t *testing.T) {
	for _, tc := range []struct{ name, raw string }{
		{"ST", "\x1b]7180;1;h:1\x1b\\"},
		{"BEL", "\x1b]7180;1;h:1\x07"},
	} {
		var s nestScanner
		s.feed([]byte(tc.raw))
		if got, ok := s.chain(); !ok || len(got) != 1 || !got[0].Locked {
			t.Errorf("%s: got %+v ok=%v", tc.name, got, ok)
		}
	}
}

// The latest report wins: this is a state broadcast repeated every frame, so a
// parent that kept the first one would show the state the far side was in when
// it connected and never move again.
func TestTheLatestReportWins(t *testing.T) {
	var s nestScanner
	s.feed([]byte(nestEncode([]nestLayer{{Host: "h", Locked: false}})))
	s.feed([]byte(nestEncode([]nestLayer{{Host: "h", Locked: true}})))
	if got, _ := s.chain(); len(got) != 1 || !got[0].Locked {
		t.Errorf("the second report should have replaced the first: %+v", got)
	}
}

// An empty chain is a REPORT, not silence: "I am sshu with nothing below me"
// has to be distinguishable from "there is a plain shell over there", because
// only one of them means the lock menu has a layer list to show.
func TestAnEmptyChainStillCountsAsAReport(t *testing.T) {
	var s nestScanner
	s.feed([]byte(nestEncode(nil)))
	got, ok := s.chain()
	if !ok {
		t.Error("an empty chain must still register as a report")
	}
	if len(got) != 0 {
		t.Errorf("chain should be empty, got %+v", got)
	}
}

// Nothing that is not ours may be read as ours — a shell printing a title, a
// different OSC, or a version this build does not know.
func TestForeignSequencesAreIgnored(t *testing.T) {
	for _, raw := range []string{
		"\x1b]0;a title\x07",       // a title
		"\x1b]777;something\x1b\\", // another OSC
		"\x1b]7180;9;h:1\x1b\\",    // a version from the future
		"\x1b]7180;1;h:1",          // never terminated
		// An ESC that is not ST, followed by a real terminator later on. The
		// trailing ST matters: without it the scan runs off the end and
		// returns nothing anyway, so the fixture would pass whether or not
		// the abandonment rule existed at all.
		"\x1b]7180;1;h:1\x1b[0m junk\x1b\\",
		"plain text with no escapes", //
	} {
		var s nestScanner
		s.feed([]byte(raw))
		if _, ok := s.chain(); ok {
			t.Errorf("%q was read as a report and must not be", raw)
		}
	}
}

// reset is what stops a chain outliving the thing it described — quitting the
// inner sshu leaves its last report sitting in the scanner otherwise.
func TestResetForgetsTheChain(t *testing.T) {
	var s nestScanner
	s.feed([]byte(nestEncode([]nestLayer{{Host: "h", Locked: true}})))
	if _, ok := s.chain(); !ok {
		t.Fatal("setup: expected a report")
	}
	s.reset()
	if got, ok := s.chain(); ok || len(got) != 0 {
		t.Errorf("after reset: got %+v ok=%v", got, ok)
	}
}

// A host name cannot be allowed to forge a field or cut the sequence short.
func TestAHostNameCannotForgeTheFormat(t *testing.T) {
	var s nestScanner
	s.feed([]byte(nestEncode([]nestLayer{
		{Host: "ev;il:1;fake:1", Locked: false},
	})))
	got, ok := s.chain()
	if !ok {
		t.Fatal("expected a report")
	}
	if len(got) != 1 {
		t.Fatalf("a name with separators created %d layers, want 1: %+v", len(got), got)
	}
	if strings.ContainsAny(got[0].Host, ";:") {
		t.Errorf("separators survived into the name: %q", got[0].Host)
	}
	if got[0].Locked {
		t.Error("the forged field must not have set the state")
	}
}

// An unterminated sequence must not grow the carry buffer without bound — a
// remote that prints our prefix and then streams forever is a memory leak
// otherwise, and it is the far side that chooses what to print.
func TestAnUnterminatedReportCannotGrowForever(t *testing.T) {
	var s nestScanner
	s.feed([]byte("\x1b]7180;1;"))
	for i := 0; i < 200; i++ {
		s.feed([]byte(strings.Repeat("x", 100)))
	}
	if len(s.carry) > nestCarryMax {
		t.Errorf("carry grew to %d, cap is %d", len(s.carry), nestCarryMax)
	}
}

// nestFake is a stand-in remote that behaves like a nested sshu: it enters the
// alt screen (which is what a Bubble Tea program does, and what tells a parent
// the reporter is still on screen) and then announces a chain.
func nestFake(t *testing.T, announce string) AppModel {
	t.Helper()
	// Waits on stdin before leaving the alt screen, so a test can decide WHEN
	// the reporter goes away instead of racing a sleep.
	fakeSSH(t, `printf '\033[?1049h'; printf '`+announce+`'; read x; printf '\033[?1049l'; sleep 30`)
	m := pressA(sshApp(t, sample()), "enter", "enter")
	t.Cleanup(func() { m.ssh.stopAll() })
	waitFor(t, "the stand-in to report", func() bool {
		_, ok := m.ssh.sessions[0].pty.nestChain()
		return ok
	})
	return m
}

// The wiring, end to end and through a real PTY: the reader has to SCAN the
// stream (not just draw it), this sshu has to put its OWN entry in front of
// what it heard, and the frame has to CARRY the result. Unit tests on the
// codec prove none of that — each of these three was a live mutation that
// survived until this test existed.
func TestTheWiringCarriesAChainUpOneLayer(t *testing.T) {
	// Sent as a literal ESC-backslash pair for /bin/sh's printf.
	m := nestFake(t, `\033]7180;1;inner-host:1\033\\`)

	// 1. the reader scanned it out of the stream
	inner, ok := m.ssh.sessions[0].pty.nestChain()
	if !ok || len(inner) != 1 || inner[0].Host != "inner-host" || !inner[0].Locked {
		t.Fatalf("the reader did not scan the report: %+v ok=%v", inner, ok)
	}

	// 2. this sshu prepends its own entry, so the outermost can count depths
	chain := m.nestChain()
	if len(chain) != 2 {
		t.Fatalf("chain should be this sshu plus the one below: %+v", chain)
	}
	if chain[0].Host != sample()[0].Name {
		t.Errorf("this sshu's own entry must come first, got %q", chain[0].Host)
	}
	if chain[1].Host != "inner-host" {
		t.Errorf("the inner entry must follow, got %q", chain[1].Host)
	}

	// 3. and the frame carries the assembled chain onward
	if !strings.Contains(m.View(), nestEncode(chain)) {
		t.Error("View() must carry the announcement, or nothing propagates")
	}
}

// Leaving the alt screen is the only signal that the reporter is gone —
// quitting the inner sshu back to its shell. Its chain must go with it rather
// than sitting there describing something that is not running.
func TestLeavingTheAltScreenDropsTheChain(t *testing.T) {
	m := nestFake(t, `\033]7180;1;inner-host:1\033\\`)
	// Release the remote's `read`, so it leaves the alt screen exactly as a
	// quitting TUI does — by PRINTING the sequence. (Writing it to the master
	// would send it as the child's input, where nothing would ever draw it.)
	m.ssh.sessions[0].pty.write(tea.KeyMsg{Type: tea.KeyEnter})
	waitFor(t, "the chain to be dropped", func() bool {
		_, ok := m.ssh.sessions[0].pty.nestChain()
		return !ok
	})
}

// And a stream that never contains our prefix must not accumulate either —
// that is every ordinary session, which is most of them.
func TestOrdinaryOutputDoesNotAccumulate(t *testing.T) {
	var s nestScanner
	for i := 0; i < 500; i++ {
		s.feed([]byte(strings.Repeat("ordinary output\n", 50)))
	}
	if len(s.carry) > 16 {
		t.Errorf("carry grew to %d on a stream with no report", len(s.carry))
	}
}
