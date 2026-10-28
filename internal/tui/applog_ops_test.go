package tui

import (
	"os"
	"testing"
	"time"
)

// logged opens the App Log, finds entry among its newest, and closes it.
func (hs *harness) logged(entry string) {
	hs.t.Helper()
	hs.keys("!")
	hs.contains("INFO " + entry)
	hs.keys("Esc")
}

// An operation that only looks, or changes only what is on the screen, says
// so in the App Log: what the user was looking at before an error is part
// of it (tdp note.7). Manage's are the viewers; going into a section only
// moves the focus to what the cursor showed already, and is not.
func TestManagesLooksAreInTheAppLog(t *testing.T) {
	hs := newHarness(t, 160, 40, testData(sampleHosts(), sampleCreds()))
	hs.keys("1", "j", "Enter", "!")
	hs.contains("APP LOG")
	hs.lacks("Opened Credentials")
	hs.keys("Esc", "Enter")
	hs.contains("CREDENTIAL")
	hs.keys("Esc")
	hs.logged("Viewed credential ops")
}

// File transfer's: a side on a host, a directory, a mark, a file read.
func TestFileTransfersLooksAreInTheAppLog(t *testing.T) {
	twoDirs(t)
	// Home is where it starts: the paths are written as ~/…, as the
	// location row writes them.
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", wd)
	hs := newHarness(t, 160, 40, testData(nil, nil))
	hs.keys("F", "H", "Enter")
	hs.logged("Opened local on the left side")
	hs.keys("Enter")
	hs.logged("Opened ~/a on local")
	hs.keys("a")
	hs.logged("Marked ~/a/x.txt on local")
	hs.keys("v")
	hs.contains("FILE")
	hs.keys("Esc")
	hs.logged("Viewed ~/a/x.txt on local")
	hs.keys("a")
	hs.logged("Unmarked ~/a/x.txt on local")
}

// The SSH screen's: a session opened, the keyboard put in it, its cell
// hidden and shown again, the grid arranged.
func TestSSHOperationsAreInTheAppLog(t *testing.T) {
	fakeSSH(t, "echo welcome; exec cat")
	hs := newHarness(t, 160, 40, testData(sampleHosts(), sampleCreds()))
	hs.keys("2", "Enter", "Enter")
	hs.settle(func() bool { return hs.shows("welcome") }, 2*time.Second)
	x := hs.s.sh.sessions[0]
	label := hs.s.sh.label(x)
	hs.keys("Alt-Esc", "Enter")
	hs.logged("Opened session " + label)
	hs.keys("H")
	hs.logged("Hid " + label)
	hs.keys("H")
	hs.logged("Showed " + label)
	hs.keys("Enter", "Alt-Esc", "Enter")
	hs.logged("Typing into " + label)
	hs.keys("2", "Enter", "j", "Enter")
	hs.logged("Set Arrange to vertical")
	hs.s.sh.closeAll()
}
