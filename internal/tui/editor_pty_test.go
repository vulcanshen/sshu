// These tests run a stand-in for $EDITOR in a PTY.

package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	terminu "github.com/vulcanshen/terminu-go"

	"github.com/vulcanshen/sshu/v3/internal/store"
)

// Edit file on a Host block's form opens the file in $EDITOR on the Host's
// line, and the form comes back with what the file says after (tdp
// textarea.1: the other options are the editor's).
func TestEditFileOpensTheEditorAndReadsTheFileAgain(t *testing.T) {
	dir := t.TempDir()
	// Under HOME, as ~/.ssh/config is: the file's path is then in ~ form.
	t.Setenv("HOME", dir)
	cfg := filepath.Join(dir, ".ssh", "config")
	if err := os.MkdirAll(filepath.Dir(cfg), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfg, []byte("# mine\nHost box\n  HostName 10.0.0.1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	args := filepath.Join(dir, "args")
	editor := filepath.Join(dir, "editor")
	script := "#!/bin/sh\necho \"$1\" > " + args + "\necho '  Compression yes' >> \"$2\"\n"
	if err := os.WriteFile(editor, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", editor)
	d := testData(sampleHosts(), sampleCreds())
	var err error
	if d.SSHConfig, err = store.LoadSSHConfigFrom(cfg); err != nil {
		t.Fatal(err)
	}
	hs := newHarness(t, 100, 40, d)
	hs.keys("1", "j", "j", "2", "E")
	hs.contains("EDIT: box")
	hs.contains("Others")
	hs.keys("Tab")
	if !hs.shows("Enter:edit file") {
		hs.keys("h")
	}
	hs.contains("Enter:edit file")
	hs.keys("Enter")
	if !hs.settle(func() bool { return hs.shows("Compression yes") }, 3*time.Second) {
		t.Fatalf("the form did not come back with the file's new line:\n%s", hs.text())
	}
	hs.contains("EDIT: box")
	got, _ := os.ReadFile(args)
	if strings.TrimSpace(string(got)) != "+2" {
		t.Errorf("the editor was started at %q, want the Host line, +2", got)
	}
}

// The terminal popup the editor runs in keeps every row of the screen as
// wide as the terminal, with icons and powerline symbols one cell or two
// (tdp E5, tdp E6; the rest of sshu's popups are in popups_test.go).
func TestTheEditorPopupKeepsTheScreenFullWidth(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	cfg := filepath.Join(dir, ".ssh", "config")
	if err := os.MkdirAll(filepath.Dir(cfg), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfg, []byte("Host box\n  HostName 10.0.0.1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	editor := filepath.Join(dir, "editor")
	if err := os.WriteFile(editor, []byte("#!/bin/sh\necho editing\nsleep 5\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", editor)
	defer terminu.SetIconWidth(1, 1)
	for _, iw := range [][2]int{{1, 1}, {2, 1}, {2, 2}} {
		d := testData(sampleHosts(), sampleCreds())
		var err error
		if d.SSHConfig, err = store.LoadSSHConfigFrom(cfg); err != nil {
			t.Fatal(err)
		}
		hs := newHarness(t, 80, 40, d)
		terminu.SetIconWidth(iw[0], iw[1])
		hs.keys("1", "j", "j", "2", "E", "Tab")
		if !hs.shows("Enter:edit file") {
			hs.keys("h")
		}
		hs.keys("Enter")
		if !hs.settle(func() bool { return hs.shows("editing") }, 3*time.Second) {
			t.Fatalf("the editor never showed:\n%s", hs.text())
		}
		hs.fullWidth(80, "terminal popup")
		hs.keys("Alt-Esc")
	}
	KillChildren()
}

// With changes in the form, Edit file asks to save them first, for the editor
// opens the file as it is on disk; Esc stays on the form, Enter saves and
// opens the editor over it.
func TestEditFileAsksToSaveTheFormFirst(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	cfg := filepath.Join(dir, ".ssh", "config")
	if err := os.MkdirAll(filepath.Dir(cfg), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfg, []byte("Host box\n  HostName 10.0.0.1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	editor := filepath.Join(dir, "editor")
	if err := os.WriteFile(editor, []byte("#!/bin/sh\necho '  Compression yes' >> \"$2\"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", editor)
	d := testData(sampleHosts(), sampleCreds())
	d.SaveSSHConfig = store.SaveSSHConfig
	var err error
	if d.SSHConfig, err = store.LoadSSHConfigFrom(cfg); err != nil {
		t.Fatal(err)
	}
	hs := newHarness(t, 100, 40, d)
	hs.keys("1", "j", "j", "2", "E", "j", "Enter") // HostName
	hs.keys("Backspace", "10.0.0.2", "Enter")      // the old value is taken whole by the first key
	hs.keys("Tab")
	if !hs.shows("Enter:edit file") {
		hs.keys("h")
	}
	hs.keys("Enter")
	hs.contains("Save the changes to Host box first?")
	hs.keys("Esc")
	hs.contains("EDIT: box")
	hs.keys("Enter", "Enter")
	if !hs.settle(func() bool { return hs.shows("Compression yes") }, 3*time.Second) {
		t.Fatalf("the form did not read the file again:\n%s", hs.text())
	}
	got, _ := os.ReadFile(cfg)
	if !strings.Contains(string(got), "HostName 10.0.0.2") || !strings.Contains(string(got), "Compression yes") {
		t.Errorf("the file is\n%s\nwant the saved HostName and the editor's line", got)
	}
}

// Whether an edit is saved is a question about the file, not the editor's
// exit code: vim's :cq writes the file and exits 1, and what it wrote is
// kept (design §7.3.4).
func TestAnEditorThatExitsNonZeroStillSavesWhatItWrote(t *testing.T) {
	root := twoDirs(t)
	editor := filepath.Join(t.TempDir(), "editor")
	if err := os.WriteFile(editor, []byte("#!/bin/sh\nprintf 'more\\n' >> \"$1\"\nexit 1\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", editor)
	hs := newHarness(t, 120, 36, testData(nil, nil))
	hs.keys("F", "H", "Enter", "Enter", "e")
	if !hs.settle(func() bool { return hs.shows("Saved x.txt") || hs.shows("Abandoned") }, 3*time.Second) {
		t.Fatalf("the edit never ended:\n%s", hs.text())
	}
	hs.contains("Saved x.txt")
	if b, _ := os.ReadFile(filepath.Join(root, "a", "x.txt")); string(b) != "hello\nmore\n" {
		t.Errorf("x.txt is %q", b)
	}
	if c := hs.s.d.Changes; len(c) != 1 || !strings.Contains(c[0].Action, "saved x.txt") {
		t.Errorf("changes: %+v", c)
	}
}

// Editing a file from the Space menu keeps the menu: the editor opens once
// the file is ready, and when it ends the menu is still there (tdp T1: an
// editor on the item keeps its source).
func TestEditingFromTheSpaceMenuKeepsIt(t *testing.T) {
	twoDirs(t)
	editor := filepath.Join(t.TempDir(), "editor")
	if err := os.WriteFile(editor, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", editor)
	hs := newHarness(t, 120, 36, testData(nil, nil))
	menu := "everything under here" // a row only the Space menu has
	hs.keys("F", "H", "Enter", "Enter", "Space", "e")
	if !hs.settle(func() bool { return hs.shows("No changes to x.txt") }, 3*time.Second) {
		t.Fatalf("the edit never ended:\n%s", hs.text())
	}
	hs.contains(menu)
}
