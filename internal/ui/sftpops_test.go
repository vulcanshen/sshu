package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/vulcanshen/sshu/internal/remote"
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
	m = pressA(m, "a") // mark it, so the mark has to follow

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
	m = pressA(m, "a", "j", "a") // assets/ (a directory) and deploy.sh
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
	m = pressA(m, "a", "j", "a")
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

// The local side opens where sshu was launched, not at $HOME. `cd ~/release &&
// sshu` should already be looking at the release — a home directory full of
// dotfiles is a place you then have to navigate OUT of.
//
// home is still the real home, because that is what folds the crumb back to ~.
func TestTheLocalSideOpensWhereSshuWasLaunched(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	if wd == home {
		t.Skip("the test is running from $HOME, so the two are indistinguishable")
	}

	m := sftpFixture(t, 100, 26)
	m.sftp.sides[sideLeft].connect(remote.Local())
	s := m.sftp.sides[sideLeft]

	if s.cwd != filepath.ToSlash(wd) {
		t.Errorf("the local side opened at %q, want the launch directory %q", s.cwd, wd)
	}
	if s.home != filepath.ToSlash(home) {
		t.Errorf("home is %q, want the real home %q — the crumb folds on it", s.home, home)
	}
}

// One box makes both kinds, and the trailing slash is the whole of the
// difference — the same way a shell has written directories forever.
//
// Either way the cursor lands on what was just made: making something is almost
// always the first half of doing something with it.
func TestAddMakesAFileOrADirectory(t *testing.T) {
	for _, tc := range []struct {
		typed string
		name  string
		isDir bool
	}{
		{"releases/", "releases", true},
		{"notes.md", "notes.md", false},
	} {
		m := sftpFixture(t, 100, 26)
		m.sftp.focus = panelLeftFiles
		cwd := m.sftp.sides[sideLeft].cwd

		m = pressA(m, "A")
		if !m.input.isActive() {
			t.Fatal("A should ask for a name")
		}
		if m.input.value != "" {
			t.Errorf("the box should start empty, got %q", m.input.value)
		}
		m = typeText(m, tc.typed)
		m = pressA(m, "enter")

		info, err := os.Stat(filepath.Join(cwd, tc.name))
		if err != nil {
			t.Fatalf("%q made nothing: %v", tc.typed, err)
		}
		if info.IsDir() != tc.isDir {
			t.Errorf("%q made isDir=%v, want %v", tc.typed, info.IsDir(), tc.isDir)
		}
		if !tc.isDir && info.Size() != 0 {
			t.Errorf("%q made a file of %d bytes, want an empty one", tc.typed, info.Size())
		}
		e, ok := m.sftp.cur().cursorEntry()
		if !ok || e.Name != tc.name {
			t.Errorf("%q left the cursor on %q", tc.typed, e.Name)
		}
	}
}

// The rule lives in one character at the end of the line, so the box says which
// side of it you are on WHILE you type. A label that only described the rule
// would leave you finding out by pressing Enter.
func TestAddSaysWhichKindItWillMake(t *testing.T) {
	m := sftpFixture(t, 100, 26)
	m.sftp.focus = panelLeftFiles
	m = pressA(m, "A")

	// Empty: the box has not been told anything yet, so it promises nothing.
	if got := ansi.Strip(m.input.view()); strings.Contains(got, "create file") ||
		strings.Contains(got, "create directory") {
		t.Errorf("an empty box already claims a kind:\n%s", got)
	}
	// And it names the two answers where the typing happens.
	if got := ansi.Strip(m.input.view()); !strings.Contains(got, "name/ for a directory") {
		t.Errorf("the empty box does not say how to ask for a directory:\n%s", got)
	}

	file := ansi.Strip(typeText(m, "logs").input.view())
	if !strings.Contains(file, "create file") {
		t.Errorf("a plain name does not promise a file:\n%s", file)
	}
	dir := ansi.Strip(typeText(m, "logs/").input.view())
	if !strings.Contains(dir, "create directory") {
		t.Errorf("a trailing slash does not promise a directory:\n%s", dir)
	}
}

// The same three refusals as a rename, because they are the same three ways to
// mean something other than what you typed. Only the LAST slash is the type
// marker; one anywhere else still makes it a path.
func TestAddRefusesBadNames(t *testing.T) {
	for _, tc := range []struct{ name, typed string }{
		{"an existing name", "assets"},
		{"an existing file", "main.go"},
		{"a path", "a/b"},
		{"a path ending in a slash", "a/b/"},
		{"a slash on its own", "/"},
		{"nothing at all", "   "},
	} {
		m := sftpFixture(t, 100, 26)
		m.sftp.focus = panelLeftFiles
		cwd := m.sftp.sides[sideLeft].cwd
		before, err := os.ReadDir(cwd)
		if err != nil {
			t.Fatal(err)
		}
		// An existing FILE is the dangerous one: Create truncates, so a refusal
		// that only counted entries would call an emptied file a success.
		existing, _ := os.ReadFile(filepath.Join(cwd, "main.go"))

		m = pressA(m, "A")
		m = typeText(m, tc.typed)
		m = pressA(m, "enter")

		if now, _ := os.ReadFile(filepath.Join(cwd, "main.go")); string(now) != string(existing) {
			t.Errorf("%s: an existing file was emptied", tc.name)
		}

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
		if strings.TrimSpace(tc.typed) != "" && (!m.toast.isActive() || m.toast.kind != toastError) {
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
		{"A", "create", "rename"},
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

// [D]isconnect is [S]elect host answered the other way, so it hands the side
// back the state it had before a host was picked — no filesystem, no listing,
// no marks. A mark is a path on a filesystem; keeping one across a disconnect
// would leave the side holding a paper address for a house that is gone.
func TestDisconnectReturnsTheSideToNoHost(t *testing.T) {
	m := sftpFixture(t, 100, 26)
	m.sftp.focus = panelLeftFiles
	m = pressA(m, "a", "j", "a") // two marks, so the reset has something to lose
	if len(m.sftp.sides[sideLeft].marks) == 0 {
		t.Fatal("setup: nothing marked")
	}
	gen := m.sftp.sides[sideLeft].dialGen

	m = pressA(m, "D")
	s := m.sftp.sides[sideLeft]
	if s.fs != nil || s.host != "" || s.cwd != "" {
		t.Errorf("the side should hold no host: fs=%v host=%q cwd=%q", s.fs != nil, s.host, s.cwd)
	}
	if len(s.entries) != 0 || len(s.marks) != 0 {
		t.Errorf("listing and marks belonged to the host: %d entries, %d marks",
			len(s.entries), len(s.marks))
	}
	if s.markedSet == nil {
		t.Error("the mark set must be usable again without a nil map panic")
	}
	// A dial already in flight must not be able to land on the side afterwards.
	if s.dialGen <= gen {
		t.Errorf("dialGen should have moved past %d, got %d", gen, s.dialGen)
	}
	// The panel says what it now is, and the other side is untouched.
	if !strings.Contains(ansi.Strip(m.View()), "select a host") {
		t.Error("the panel should be back to the no-host prompt")
	}
	if m.sftp.sides[sideRight].fs == nil {
		t.Error("disconnecting one side must not touch the other")
	}
}

// The menu and the hotkey have to agree (§4.2), and both have to disappear
// when there is no host — nothing to disconnect from.
func TestDisconnectOnlyExistsWhereThereIsAHost(t *testing.T) {
	m := sftpFixture(t, 100, 26)
	m.sftp.focus = panelLeftFiles
	m = pressA(m, "D")

	for _, it := range m.sftpMenuItems() {
		if it.key == "D" {
			t.Error("a side with no host is offering to disconnect")
		}
	}
	// Pressing it anyway falls through to the list keys and changes nothing.
	if before := m.sftp.sides[sideLeft]; pressA(m, "D").sftp.sides[sideLeft].host != before.host {
		t.Error("D did something on a side that has no host")
	}

	// It is a files-panel action: the marks panel shows the marks, not the host.
	m.sftp.focus = panelLeftMarks
	m2 := sftpFixture(t, 100, 26)
	m2.sftp.focus = panelLeftMarks
	for _, it := range m2.sftpMenuItems() {
		if it.key == "D" {
			t.Error("Disconnect should not be offered on a marks panel")
		}
	}
}

// The watch loop re-lists on a two-second poll, and only when the directory's
// own timestamp moved. [R]efresh is the key for the doubt that leaves: it
// re-reads NOW, without waiting and without asking the timestamp's permission.
func TestRefreshRereadsTheDirectoryWithoutTheWatch(t *testing.T) {
	m := sftpFixture(t, 100, 26)
	m.sftp.focus = panelLeftFiles
	before := len(m.sftp.sides[sideLeft].entries)

	// Something appears behind sshu's back. No tick is delivered, so nothing
	// but the key itself can bring it in.
	made := filepath.Join(m.sftp.sides[sideLeft].cwd, "appeared.txt")
	if err := os.WriteFile(made, []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := len(m.sftp.sides[sideLeft].entries); got != before {
		t.Fatalf("setup: the listing changed on its own (%d → %d)", before, got)
	}

	m = pressA(m, "R")
	if got := len(m.sftp.sides[sideLeft].entries); got != before+1 {
		t.Errorf("R should have re-read the directory: %d entries, want %d", got, before+1)
	}
	if !strings.Contains(ansi.Strip(m.View()), "Refreshed") {
		t.Errorf("a refresh that changes nothing must still say it happened:\n%s",
			ansi.Strip(m.View()))
	}
	// The other side is not this side.
	if m.sftp.sides[sideRight].cwd == m.sftp.sides[sideLeft].cwd {
		t.Fatal("setup: the two sides should be looking at different directories")
	}
}

// A refresh must not move the cursor off what it was on — you press it to see
// what changed, not to lose your place.
func TestRefreshKeepsTheCursorWhereItWas(t *testing.T) {
	m := sftpFixture(t, 100, 26)
	m.sftp.focus = panelLeftFiles
	m.sftp.sides[sideLeft].cursor = 2
	want, ok := m.sftp.sides[sideLeft].cursorEntry()
	if !ok {
		t.Fatal("setup: no row at the cursor")
	}
	// Something sorts in ahead of it, so holding the INDEX would not be enough.
	if err := os.WriteFile(filepath.Join(m.sftp.sides[sideLeft].cwd, "AAA.txt"),
		[]byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	m = pressA(m, "R")
	got, ok := m.sftp.sides[sideLeft].cursorEntry()
	if !ok || got.Name != want.Name {
		t.Errorf("the cursor landed on %q, want %q", got.Name, want.Name)
	}
}

// Reading is the one thing that stays safe while bytes move, and mid-transfer
// is exactly when a listing is worth looking at again — so R is not frozen the
// way [S] and [D] are.
func TestRefreshIsNotFrozenByARunningTransfer(t *testing.T) {
	m := busy(sftpFixture(t, 100, 26))
	m.sftp.focus = panelLeftFiles
	for _, it := range m.sftpMenuItems() {
		if it.key == "R" && it.disabled {
			t.Error("Refresh should stay live while a transfer runs")
		}
	}
	before := len(m.sftp.sides[sideLeft].entries)
	if err := os.WriteFile(filepath.Join(m.sftp.sides[sideLeft].cwd, "mid.txt"),
		[]byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := pressA(m, "R"); len(got.sftp.sides[sideLeft].entries) != before+1 {
		t.Error("R did not re-read while a transfer was running")
	}
}

// R belongs to the directory, and the marks panel is not showing one — the
// same reason [D]isconnect is a files-panel action.
func TestRefreshIsAFilesPanelAction(t *testing.T) {
	m := sftpFixture(t, 100, 26)
	m.sftp.focus = panelLeftMarks
	for _, it := range m.sftpMenuItems() {
		if it.key == "R" {
			t.Error("Refresh should not be offered on a marks panel")
		}
	}
}

// A directory that has gone away says so instead of leaving a stale listing
// that looks current because it was just refreshed.
func TestRefreshReportsAFailure(t *testing.T) {
	m := sftpFixture(t, 100, 26)
	m.sftp.focus = panelLeftFiles
	gone := filepath.Join(t.TempDir(), "not-there")
	m.sftp.sides[sideLeft].cwd = gone

	m = pressA(m, "R")
	if m.sftp.sides[sideLeft].err == "" {
		t.Error("a refresh that could not read must record why")
	}
	if strings.Contains(ansi.Strip(m.View()), "Refreshed") {
		t.Error("a failed refresh must not report success")
	}
}
