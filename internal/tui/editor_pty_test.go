// This test runs the editor in a PTY: see ssh_pty_test.go for why it stays
// out of -race (TODO(lib SS10)).

//go:build !race

package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	terminu "github.com/vulcanshen/terminu-go"

	"github.com/vulcanshen/sshu/internal/store"
)

// Edit file on a Host block's form opens the file in $EDITOR on the Host's
// line, and the form comes back with what the file says after (tdp S70: the
// other options are the editor's).
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
	hs.keys("j", "j", "2", "E")
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
		hs.keys("j", "j", "2", "E", "Tab")
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
