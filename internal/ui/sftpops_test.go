package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// Rename changes a name in place, and moves the mark with it — a mark is a path,
// so a renamed mark would otherwise point at something that is no longer there.
func TestRenameMovesTheFileAndItsMark(t *testing.T) {
	m := sftpFixture(t, 100, 26)
	m.sftp.focus = panelLeftFiles
	m = pressA(m, "j") // deploy.sh
	old, _ := m.sftp.cur().cursorPath()
	if filepath.Base(old) != "deploy.sh" {
		t.Fatalf("setup: cursor is on %q", old)
	}
	m = pressA(m, "m") // mark it, so the mark has to follow

	m = pressA(m, "r")
	if !m.input.isActive() {
		t.Fatal("r should ask for the new name")
	}
	if m.input.value != "deploy.sh" {
		t.Errorf("the box should start from the old name, got %q", m.input.value)
	}

	// Edit the name rather than retyping it.
	m = pressA(m, "backspace", "backspace", "backspace")
	m = typeText(m, "ment.sh")
	m = pressA(m, "enter")

	dir := filepath.Dir(old)
	if _, err := os.Stat(filepath.Join(dir, "deployment.sh")); err != nil {
		t.Fatalf("the renamed file is not there: %v", err)
	}
	if _, err := os.Stat(old); err == nil {
		t.Error("the old name is still there")
	}
	marks := m.sftp.sides[sideLeft].marks
	if len(marks) != 1 || filepath.Base(marks[0]) != "deployment.sh" {
		t.Errorf("the mark did not follow the rename: %v", marks)
	}
}

// Renaming onto an existing name is refused rather than silently overwriting.
// os.Rename would clobber it and SFTP's would refuse — one action must not
// depend on which end it runs on.
func TestRenameRefusesToClobber(t *testing.T) {
	m := sftpFixture(t, 100, 26)
	m.sftp.focus = panelLeftFiles
	m = pressA(m, "j") // deploy.sh
	old, _ := m.sftp.cur().cursorPath()

	m = pressA(m, "r")
	for range "deploy.sh" {
		m = pressA(m, "backspace")
	}
	m = typeText(m, "main.go")
	m = pressA(m, "enter")

	if _, err := os.Stat(old); err != nil {
		t.Error("the original was renamed away over an existing file")
	}
	body, err := os.ReadFile(filepath.Join(filepath.Dir(old), "main.go"))
	if err != nil || string(body) != "x" {
		t.Errorf("main.go was overwritten: %q, %v", body, err)
	}
	if !m.toast.isActive() {
		t.Error("a refused rename should say why")
	}
}

// A name with a separator in it is a move, not a rename, and this box does not
// do moves — there is a whole tab for that.
func TestRenameRefusesASlash(t *testing.T) {
	m := sftpFixture(t, 100, 26)
	m.sftp.focus = panelLeftFiles
	m = pressA(m, "j")
	old, _ := m.sftp.cur().cursorPath()

	m = pressA(m, "r")
	m = typeText(m, "/x")
	m = pressA(m, "enter")

	if _, err := os.Stat(old); err != nil {
		t.Error("the file moved somewhere it should not have")
	}
}

// Delete erases the marked paths, directories and all — and asks first.
func TestDeleteMarksErasesThemAfterConfirming(t *testing.T) {
	m := sftpFixture(t, 100, 26)
	m.sftp.focus = panelLeftFiles
	m = pressA(m, "m", "j", "m") // assets/ (a directory) and deploy.sh
	marks := append([]string(nil), m.sftp.sides[sideLeft].marks...)
	if len(marks) != 2 {
		t.Fatalf("setup: expected two marks, got %d", len(marks))
	}

	m = pressA(m, "X")
	if !m.confirm.isActive() || m.confirm.action != confirmDeleteMarks {
		t.Fatal("X must ask before erasing anything")
	}
	// Cancelling changes nothing.
	m = pressA(m, "esc")
	for _, p := range marks {
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("cancelling the delete still removed %s", p)
		}
	}

	m = pressA(m, "X", "enter")
	for _, p := range marks {
		if _, err := os.Stat(p); err == nil {
			t.Errorf("%s survived the delete", p)
		}
	}
	if n := len(m.sftp.sides[sideLeft].marks); n != 0 {
		t.Errorf("%d marks left pointing at deleted paths", n)
	}
}

// Clear only forgets. It sits next to Delete in the menu, so the difference has
// to be real and it has to be tested.
func TestClearMarksLeavesTheFilesAlone(t *testing.T) {
	m := sftpFixture(t, 100, 26)
	m.sftp.focus = panelLeftFiles
	m = pressA(m, "m", "j", "m")
	marks := append([]string(nil), m.sftp.sides[sideLeft].marks...)

	m = pressA(m, "C")
	if n := len(m.sftp.sides[sideLeft].marks); n != 0 {
		t.Errorf("C should clear the marks, %d left", n)
	}
	for _, p := range marks {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("C deleted %s — it must only forget", p)
		}
	}
}

// Delete and Clear are one letter apart in effect, so the menu has to keep them
// distinguishable in words as well as in keys.
func TestDeleteAndClearReadDifferently(t *testing.T) {
	m := sftpFixture(t, 100, 26)
	m.sftp.focus = panelLeftMarks

	var del, clear menuItem
	for _, it := range m.sftpMenuItems() {
		switch it.key {
		case "X":
			del = it
		case "C":
			clear = it
		}
	}
	if del.label == "" || clear.label == "" {
		t.Fatal("both Delete all marks and Clear should be on a marks panel")
	}
	if del.hint == clear.hint {
		t.Error("the two hints must say what is different about them")
	}
	if !strings.Contains(clear.hint, "forget") {
		t.Errorf("Clear's hint should say it changes nothing: %q", clear.hint)
	}
}

// New directory makes one in the directory being browsed, and leaves the cursor
// on it — making a directory is almost always the first half of putting
// something in it.
func TestNewDirectoryLandsTheCursorOnIt(t *testing.T) {
	m := sftpFixture(t, 100, 26)
	m.sftp.focus = panelLeftFiles
	cwd := m.sftp.sides[sideLeft].cwd

	m = pressA(m, "N")
	if !m.input.isActive() {
		t.Fatal("N should ask for a name")
	}
	if m.input.value != "" {
		t.Errorf("the box should start empty, got %q", m.input.value)
	}
	m = typeText(m, "releases")
	m = pressA(m, "enter")

	info, err := os.Stat(filepath.Join(cwd, "releases"))
	if err != nil || !info.IsDir() {
		t.Fatalf("the directory was not created: %v", err)
	}
	e, ok := m.sftp.cur().cursorEntry()
	if !ok || e.Name != "releases" {
		t.Errorf("the cursor is on %q, want the new directory", e.Name)
	}
}

// The same three refusals as a rename, because they are the same three ways to
// mean something other than what you typed.
func TestNewDirectoryRefusesBadNames(t *testing.T) {
	for _, tc := range []struct{ name, typed string }{
		{"an existing name", "assets"},
		{"a path", "a/b"},
		{"nothing at all", "   "},
	} {
		m := sftpFixture(t, 100, 26)
		m.sftp.focus = panelLeftFiles
		cwd := m.sftp.sides[sideLeft].cwd
		before, err := os.ReadDir(cwd)
		if err != nil {
			t.Fatal(err)
		}

		m = pressA(m, "N")
		m = typeText(m, tc.typed)
		m = pressA(m, "enter")

		after, err := os.ReadDir(cwd)
		if err != nil {
			t.Fatal(err)
		}
		if len(after) != len(before) {
			t.Errorf("%s: created something anyway (%d -> %d entries)",
				tc.name, len(before), len(after))
		}
		if m.input.isActive() {
			t.Errorf("%s: the box should have closed", tc.name)
		}
		// Counting entries is not enough on its own: MkdirAll over an existing
		// directory is a no-op, so dropping the check would leave the count
		// unchanged and still claim success. The refusal is the thing.
		if tc.typed != "   " && (!m.toast.isActive() || m.toast.kind != toastError) {
			t.Errorf("%s: should have said no, toast=%q kind=%d",
				tc.name, m.toast.msg, m.toast.kind)
		}
	}
}

// The box is shared, so its Enter hint has to say which thing it is about to do.
func TestInputHintNamesItsAction(t *testing.T) {
	m := sftpFixture(t, 100, 26)
	m.sftp.focus = panelLeftFiles

	// The RENDERING, not the field — the field is only worth anything if it
	// reaches the hint line the user actually reads.
	for _, tc := range []struct{ key, want, other string }{
		{"N", "create", "rename"},
		{"r", "rename", "create"},
	} {
		got := ansi.Strip(pressA(m, tc.key).input.view())
		if !strings.Contains(got, tc.want) {
			t.Errorf("%s: the hint does not say %q", tc.key, tc.want)
		}
		if strings.Contains(got, tc.other) {
			t.Errorf("%s: the hint still says %q", tc.key, tc.other)
		}
	}
}
