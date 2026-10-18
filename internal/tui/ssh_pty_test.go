// These tests run a program in a PTY, and terminu-go v0.1.13 resizes a PTY
// on the drawing goroutine while its reader closes it when the program ends,
// with nothing ordering the two: the race detector reports it on every
// session that is drawn and then ends. They run in `go test` (and make
// check-lib); TODO(lib SS10): back under -race once the lib orders them.

//go:build !race

package tui

import (
	"slices"
	"strings"
	"testing"
	"time"

	terminu "github.com/vulcanshen/terminu-go"

	"github.com/vulcanshen/sshu/internal/store"
)

// Connecting from the hosts table lands on the SSH screen with the keyboard
// in the new cell (tdp T1: a long session clears its source); Alt-Esc
// asks before it takes the keyboard back (tdp K10).
func TestConnectPutsTheKeyboardInTheNewCell(t *testing.T) {
	fakeSSH(t, "echo welcome; exec cat")
	hs := newHarness(t, 120, 30, testData(sampleHosts(), sampleCreds()))
	hs.keys("2", "Enter", "Enter")
	if !hs.settle(func() bool { return hs.shows("welcome") }, 3*time.Second) {
		t.Fatalf("the session's output never showed:\n%s", hs.text())
	}
	if hs.s.a.Screen() != hs.s.sh.screen {
		t.Fatal("connecting did not go to the SSH screen")
	}
	hs.contains("Alt-Esc:leave")
	hs.keys("h", "i", "Enter")
	if !hs.settle(func() bool { return strings.Count(hs.text(), "hi") >= 2 }, 3*time.Second) {
		t.Fatalf("what was typed did not reach the session:\n%s", hs.text())
	}
	hs.keys("Alt-Esc")
	hs.contains("Leave")
	hs.keys("Esc")
	hs.contains("Alt-Esc:leave") // Esc goes back into the PTY
	hs.keys("Alt-Esc", "Enter")
	hs.contains("Space:menu")
	hs.s.sh.closeAll()
}

// A cell hidden and shown again goes back to its place: [3] stays the first
// session, and Alt-→ from it reaches the second (the order on screen).
func TestAShownCellKeepsItsPlace(t *testing.T) {
	fakeSSH(t, "exec cat")
	hs := newHarness(t, 140, 30, testData(sampleHosts(), sampleCreds()))
	hs.keys("2", "Enter", "Enter")
	hs.settle(func() bool { return hs.s.sh.liveCount() == 1 }, 2*time.Second)
	hs.keys("Alt-Esc", "Enter", "M", "Enter", "Enter")
	hs.settle(func() bool { return hs.s.sh.liveCount() == 2 }, 2*time.Second)
	hs.keys("Alt-Esc", "Enter", "1", "gg", "H")
	first := hs.s.sh.sessions[0]
	if slices.Contains(hs.s.sh.screen.Panels, first.panel) {
		t.Fatal("the hidden cell is still a panel: Tab and the digits could land on it")
	}
	hs.keys("H")
	if ps := hs.s.sh.screen.Panels; len(ps) != 4 || ps[2] != first.panel {
		t.Fatalf("shown again, the first session's cell is not [3] (%d panels)", len(ps))
	}
	hs.keys("Enter", "Alt-→")
	if hs.s.a.Screen().Focused() != hs.s.sh.sessions[1].panel {
		t.Fatal("Alt-→ from [3] did not reach [4]")
	}
	hs.s.sh.closeAll()
}

// A session ssh gave up on says what ssh said, in a toast and in Errors.
func TestAFailedSessionSaysWhatSSHSaid(t *testing.T) {
	fakeSSH(t, `echo "ssh: connect to host db.internal port 2222: Connection refused" > "$LOG"; exit 255`)
	hs := newHarness(t, 120, 30, testData(sampleHosts(), sampleCreds()))
	hs.keys("2", "j", "Enter", "Enter")
	if !hs.settle(func() bool { return len(hs.s.d.Errors) == 1 }, 3*time.Second) {
		t.Fatalf("no error was recorded:\n%s", hs.text())
	}
	if e := hs.s.d.Errors[0]; !strings.Contains(e.Cause, "Connection refused") || e.Host != "db-replica" {
		t.Fatalf("recorded %+v", e)
	}
	if c := hs.s.d.Conns; len(c) != 1 || c[0].Result != store.ResultFail {
		t.Fatalf("connections %+v", c)
	}
	hs.contains("Connection refused")
}

// Every SSH screen row is as wide as the terminal, cells and all (tdp L4).
func TestEverySSHRowIsAsWideAsTheScreen(t *testing.T) {
	fakeSSH(t, "printf '日本語 \\U0000F120 wide\\n'; exec cat")
	defer terminu.SetIconWidth(1, 1)
	for _, iw := range [][2]int{{1, 1}, {2, 1}, {2, 2}} {
		for _, size := range [][2]int{{80, 40}, {57, 24}, {160, 50}, {40, 12}} {
			w, h := size[0], size[1]
			hs := newHarness(t, w, h, testData(sampleHosts(), sampleCreds()))
			terminu.SetIconWidth(iw[0], iw[1])
			hs.keys("S")
			hs.fullWidth(w, "no sessions")
			hs.keys("M", "2", "Enter", "Enter")
			hs.settle(func() bool { return hs.shows("wide") }, 2*time.Second)
			hs.fullWidth(w, "one cell, inside")
			hs.keys("Alt-Esc", "Enter")
			hs.fullWidth(w, "one cell, outside")
			hs.keys("1", "j")
			hs.s.sh.closeAll()
		}
	}
}
