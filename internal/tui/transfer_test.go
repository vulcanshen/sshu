package tui

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	terminu "github.com/vulcanshen/terminu-go"
)

// twoDirs is a launch directory with a/x.txt and an empty b/.
func twoDirs(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, d := range []string{"a", "b"} {
		if err := os.Mkdir(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "a", "x.txt"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)
	return root
}

// waitFor runs the program's commands until cond holds.
func (hs *harness) waitFor(cond func() bool, what string) {
	hs.t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			hs.t.Fatalf("%s never happened:\n%s", what, hs.text())
		}
		hs.send(xferTick{})
	}
}

// Both sides on this machine, one file sent from a/ to b/: one operation,
// whichever ends are local (the File transfer screen's whole idea).
func TestAFileIsSentToTheOtherSidesDirectory(t *testing.T) {
	root := twoDirs(t)
	hs := newHarness(t, 120, 36, testData(nil, nil))
	hs.keys("F", "H", "Enter") // [1] local
	hs.contains("[1] local")
	hs.keys("Enter") // into a/
	hs.keys("3", "H", "Enter", "j", "Enter")
	hs.keys("1")
	hs.contains("x.txt")
	hs.keys("t")
	hs.waitFor(func() bool { _, err := os.Stat(filepath.Join(root, "b", "x.txt")); return err == nil }, "the copy")
	hs.waitFor(func() bool { return hs.s.ft.running() == 0 && len(hs.s.d.Changes) == 1 }, "the transfer's ending")
}

// A connection ssh gave up on says the last line ssh printed, the reason, in
// the toast and the panel: a toast and a panel's middle are short text, where
// a \n is drawn as \n (tdp components.5). Everything ssh said,
// a banner included, is in the App Log. The host's name keeps its case.
func TestAFailedConnectionSaysTheLastLineSSHPrinted(t *testing.T) {
	hs := newHarness(t, 120, 36, testData(nil, nil))
	hs.keys("F")
	sd := hs.s.ft.sides[0]
	sd.dialing = "box"
	hs.send(dialedMsg{side: 0, gen: sd.dialGen, name: "box",
		err: errors.New("box: Welcome to box\nAuthorized use only\nPermission denied (publickey).")})
	hs.contains("Connect to box failed: Permission denied (publickey).")
	hs.contains("box: Permission denied (publickey).")
	hs.lacks(`\n`)
	hs.lacks("Box")
	hs.keys("Esc", "!")
	hs.contains("ERR  Connect to box failed: Permission denied (publickey). · Welcome to box Authorized use only")
}

// A directory that will not open says why in a toast that starts with a
// capital, as the App Log keeps it (tdp note.7).
func TestADirectoryThatWillNotOpenSaysWhy(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root opens every directory")
	}
	root := twoDirs(t)
	b := filepath.Join(root, "b")
	if err := os.Chmod(b, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(b, 0o755) })
	hs := newHarness(t, 120, 36, testData(nil, nil))
	hs.keys("F", "H", "Enter", "j", "Enter")
	hs.contains("Can't read /")
	hs.contains("permission denied")
	hs.lacks("open /")
}

// h|l cross to the same panel on the other side (tdp K12).
func TestHAndLCrossSides(t *testing.T) {
	twoDirs(t)
	hs := newHarness(t, 120, 36, testData(nil, nil))
	l, r := hs.s.ft.sides[0], hs.s.ft.sides[1]
	focus := func(want string, p *terminu.Panel) {
		t.Helper()
		if hs.s.a.Screen().Focused() != p {
			t.Fatalf("the focus is not on %s:\n%s", want, hs.text())
		}
	}
	hs.keys("F", "l")
	focus("the right files", r.filesP)
	hs.keys("2", "l")
	focus("the right marks", r.markedP)
	hs.keys("h")
	focus("the left marks", l.markedP)
}

// A mark is kept by path, across directories, and listed in the side's
// marks panel.
func TestMarksFollowThePathAcrossDirectories(t *testing.T) {
	twoDirs(t)
	hs := newHarness(t, 120, 36, testData(nil, nil))
	hs.keys("F", "H", "Enter", "Enter", "a")
	hs.contains("x.txt")
	if got := hs.s.ft.sides[0].marks; len(got) != 1 || filepath.Base(got[0]) != "x.txt" {
		t.Fatalf("marks %v", got)
	}
	hs.keys("Esc") // up to the launch directory
	hs.contains("a/x.txt")
	hs.keys("Enter")
	if m := hs.s.ft.sides[0].files.Marked(); len(m) != 1 {
		t.Fatalf("back in a/, the list marks %v", m)
	}
}

func TestEveryTransferRowIsAsWideAsTheScreen(t *testing.T) {
	twoDirs(t)
	defer terminu.SetIconWidth(1, 1)
	for _, iw := range [][2]int{{1, 1}, {2, 1}, {2, 2}} {
		for _, size := range [][2]int{{80, 40}, {71, 24}, {120, 40}, {60, 20}} {
			w, h := size[0], size[1]
			hs := newHarness(t, w, h, testData(nil, nil))
			terminu.SetIconWidth(iw[0], iw[1])
			hs.keys("F")
			hs.fullWidth(w, "no host")
			hs.keys("H", "Enter", "Enter")
			hs.fullWidth(w, "local")
			hs.lacks(`\x1b`) // the glyphs' colours are styles, not codes in a cell
			if w == 80 {
				hs.keys("Space")
				hs.fullWidth(w, "space menu")
				hs.keys("Esc", "/")
				hs.fullWidth(w, "search")
				hs.keys("Esc")
			}
		}
	}
}
