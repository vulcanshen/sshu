package ui

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/vulcanshen/sshu/internal/remote"
	"github.com/vulcanshen/sshu/internal/store"
)

// sftpFixture builds a small local tree and points both sides at it, so the
// whole tab can be exercised without a network.
func sftpFixture(t *testing.T, w, h int) AppModel {
	t.Helper()
	root := t.TempDir()
	deep := filepath.Join(root, "Documents", "sideproj", "app")
	if err := os.MkdirAll(filepath.Join(deep, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"deploy.sh", "README.md", "main.go"} {
		os.WriteFile(filepath.Join(deep, f), []byte("x"), 0o644)
	}
	os.MkdirAll(filepath.Join(root, "backups", "2026-08"), 0o755)
	os.WriteFile(filepath.Join(root, "backups", "dump.sql.gz"), make([]byte, 4200000), 0o644)

	m := New(sample(), nil, store.DefaultConfig())
	next, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	m = settle(next.(AppModel))
	m.tab = tabFT

	for _, sd := range []side{sideLeft, sideRight} {
		m.sftp.sides[sd].connect(remote.Local())
		m.sftp.sides[sd].home = root
	}
	m.sftp.sides[sideLeft].open(deep)
	m.sftp.sides[sideRight].open(filepath.Join(root, "backups"))
	return m
}

// The frame invariant again, for four panels this time.
func TestSFTPTabPreservesFrame(t *testing.T) {
	for _, sz := range [][2]int{{120, 40}, {100, 26}, {80, 20}, {73, 16}, {72, 16}, {71, 16}, {50, 12}, {34, 9}} {
		w, h := sz[0], sz[1]
		m := sftpFixture(t, w, h)
		m.sftp.cur().toggleMark()

		for _, focus := range []sftpPanel{panelLeftFiles, panelLeftMarks, panelRightFiles, panelRightMarks} {
			m.sftp.focus = focus
			lines := strings.Split(m.View(), "\n")
			if len(lines) != h {
				t.Errorf("%dx%d focus=%d: %d lines, want %d", w, h, focus, len(lines), h)
				continue
			}
			for i, l := range lines {
				if lw := dispW(l); lw != w {
					t.Errorf("%dx%d focus=%d line %d: width %d, want %d\n%q",
						w, h, focus, i, lw, w, l)
				}
			}
		}
	}
}

// Enter descends, Esc comes back up — filu's two keys. Esc keeps its app-wide
// meaning of "close the float first" ahead of that.
func TestSFTPEnterAndEsc(t *testing.T) {
	m := sftpFixture(t, 100, 26)
	m.sftp.focus = panelLeftFiles
	start := m.sftp.sides[sideLeft].cwd

	m = pressA(m, "enter") // the cursor is on "assets"
	if got := m.sftp.sides[sideLeft].cwd; got == start {
		t.Fatalf("Enter should have descended, still at %q", got)
	}
	m = pressA(m, "esc")
	if got := m.sftp.sides[sideLeft].cwd; got != start {
		t.Errorf("Esc should have returned to %q, got %q", start, got)
	}

	// With a float up, Esc closes the float and leaves the directory alone.
	m = pressA(m, " ")
	m = pressA(m, "esc")
	if m.spaceMenu.isActive() {
		t.Error("Esc should have closed the menu")
	}
	if got := m.sftp.sides[sideLeft].cwd; got != start {
		t.Errorf("Esc on a float must not also change directory, got %q", got)
	}
}

// `a` appends — and still toggles, so marking the wrong thing is undone by
// pressing it again rather than by a trip to the marks panel. There, `c`
// clears the one under the cursor, pairing with `C` the way t/T and x/X pair.
func TestSFTPMarkToggles(t *testing.T) {
	m := sftpFixture(t, 100, 26)
	m.sftp.focus = panelLeftFiles

	m = pressA(m, "a")
	if n := len(m.sftp.sides[sideLeft].marks); n != 1 {
		t.Fatalf("a should mark one item, got %d", n)
	}
	m = pressA(m, "a")
	if n := len(m.sftp.sides[sideLeft].marks); n != 0 {
		t.Errorf("a second a should take the mark off, got %d", n)
	}
	m = pressA(m, "m")
	if n := len(m.sftp.sides[sideLeft].marks); n != 0 {
		t.Errorf("m is not a key any more, it must not mark (%d)", n)
	}

	// `c` on the marks panel drops the one under its cursor; `u` stays
	// half-page-up.
	m = pressA(m, "a", "j", "a")
	if n := len(m.sftp.sides[sideLeft].marks); n != 2 {
		t.Fatalf("expected two marks, got %d", n)
	}
	m.sftp.focus = panelLeftMarks
	m = pressA(m, "u")
	if n := len(m.sftp.sides[sideLeft].marks); n != 2 {
		t.Errorf("u is half-page-up, it must not unmark (%d left)", n)
	}
	m = pressA(m, "c")
	if n := len(m.sftp.sides[sideLeft].marks); n != 1 {
		t.Errorf("c should drop one mark, got %d", n)
	}
}

// Marks belong to their side and die with the host: a path is only meaningful
// against the filesystem it came from.
func TestSFTPMarksAreClearedOnHostSwitch(t *testing.T) {
	m := sftpFixture(t, 100, 26)
	m.sftp.focus = panelLeftFiles
	m = pressA(m, "a")
	if len(m.sftp.sides[sideLeft].marks) != 1 {
		t.Fatal("setup: expected a mark")
	}
	if len(m.sftp.sides[sideRight].marks) != 0 {
		t.Error("marking one side must not touch the other")
	}

	m.sftp.sides[sideLeft].connect(remote.Local())
	if n := len(m.sftp.sides[sideLeft].marks); n != 0 {
		t.Errorf("switching host should clear that side, got %d marks", n)
	}
}

// Tab walks [4] [5] [6] [7] and then leaves the tab.
func TestSFTPTabWalksAllFourPanels(t *testing.T) {
	m := sftpFixture(t, 100, 26)
	m.sftp.focus = panelLeftFiles
	for _, want := range []sftpPanel{panelLeftMarks, panelRightFiles, panelRightMarks} {
		m = pressA(m, "tab")
		if m.sftp.focus != want {
			t.Fatalf("tab should reach panel %d, got %d", want, m.sftp.focus)
		}
	}
	// Past the last panel it wraps, staying in this tab: changing tab is 1/2/3.
	m = pressA(m, "tab")
	if m.tab != tabFT || m.sftp.focus != panelLeftFiles {
		t.Errorf("tab should wrap to [1], got tab=%d focus=%d", m.tab, m.sftp.focus)
	}
	m = pressA(m, "shift+tab")
	if m.tab != tabFT || m.sftp.focus != panelRightMarks {
		t.Errorf("shift+tab should wrap back to [4], got tab=%d focus=%d",
			m.tab, m.sftp.focus)
	}
}

func TestSFTPDigitsFocusPanels(t *testing.T) {
	for _, tc := range []struct {
		key  string
		want sftpPanel
	}{{"1", panelLeftFiles}, {"2", panelLeftMarks},
		{"3", panelRightFiles}, {"4", panelRightMarks}} {
		m := pressA(sftpFixture(t, 100, 26), tc.key)
		if m.sftp.focus != tc.want {
			t.Errorf("%s should focus panel %d, got %d", tc.key, tc.want, m.sftp.focus)
		}
	}
}

// hasKey is an exact-case membership test. Substring matching would let "T"
// answer for "t" and hide exactly the collision these tests exist to catch.
func hasKey(keys []string, want string) bool {
	for _, k := range keys {
		if k == want {
			return true
		}
	}
	return false
}

// Every panel can start a transfer — that is the point of the tab. The Space
// menu lists exactly what the focused panel can do, and nothing it cannot.
func TestSFTPMenuOffersTransferEverywhere(t *testing.T) {
	m := sftpFixture(t, 100, 26)
	// Both sides get a mark, so the marks panels have a row for the row actions
	// to be about — with nothing marked they correctly offer none of them.
	for _, p := range []sftpPanel{panelLeftFiles, panelRightFiles} {
		m.sftp.focus = p
		m = pressA(m, "a")
	}
	for _, p := range []sftpPanel{panelLeftFiles, panelLeftMarks,
		panelRightFiles, panelRightMarks} {
		m.sftp.focus = p
		var keys []string
		for _, it := range m.sftpMenuItems() {
			if it.key != "" {
				keys = append(keys, it.key)
			}
		}
		joined := strings.Join(keys, ",")
		for _, want := range []string{"t", "T", keySelectHost, "r"} {
			if !hasKey(keys, want) {
				t.Errorf("panel %d: menu is missing %q (have %s)", p, want, joined)
			}
		}
		// The mark actions split by panel: `a` appends on a files row (and
		// its hint owns up to the toggle), `c` clears on a marks row.
		var aLabel, aHint, cLabel string
		for _, it := range m.sftpMenuItems() {
			if it.key == "a" {
				aLabel, aHint = it.label, it.hint
			}
			if it.key == "c" {
				cLabel = it.label
			}
		}
		if p.isMarks() && (cLabel != "Clear mark" || aLabel != "") {
			t.Errorf("panel %d: want c=Clear mark and no a, got a=%q c=%q", p, aLabel, cLabel)
		}
		if !p.isMarks() && (aLabel != "Append to marks" || cLabel != "") {
			t.Errorf("panel %d: want a=Append to marks and no c, got a=%q c=%q", p, aLabel, cLabel)
		}
		if !p.isMarks() && aHint != "again takes it off" {
			t.Errorf("panel %d: the hint must own up to the toggle, got %q", p, aHint)
		}
	}
}

func TestSFTPHostPickerOffersLocalFirst(t *testing.T) {
	m := pressA(sftpFixture(t, 100, 26), "S")
	if !m.hostPicker.isActive() {
		t.Fatal("s should open the host picker")
	}
	first := ""
	for _, it := range m.hostPicker.items {
		if it.key != "" {
			first = it.label
			break
		}
	}
	if first != remote.LocalLabel {
		t.Errorf("local should be the first choice, got %q", first)
	}
	if len(m.hostPicker.items) < 1+len(sample()) {
		t.Errorf("the picker should also list the saved hosts, got %d rows",
			len(m.hostPicker.items))
	}
}

// A transfer needs somewhere to land. Saying so beats a silent no-op.
func TestSFTPTransferNeedsTheOtherSide(t *testing.T) {
	m := sftpFixture(t, 100, 26)
	m.sftp.sides[sideRight].fs = nil
	m.sftp.focus = panelLeftFiles
	m = pressA(m, "t")
	if !m.toast.isActive() || m.toast.kind != toastError {
		t.Error("transferring with no destination should say so")
	}
}

// The breadcrumb never overflows its slot at any width, and shortens rather
// than being cut off.
func TestCrumbFitsItsWidth(t *testing.T) {
	withColour(t)
	for _, p := range []string{
		"~", "/", "~/app",
		"~/Documents/sideproj/app",
		"/var/lib/postgresql/16/main/pg_wal/archive_status",
	} {
		for w := 4; w <= 60; w++ {
			got := renderCrumb(p, w)
			if lw := dispW(got); lw > w {
				t.Fatalf("renderCrumb(%q, %d) is %d cells wide\n%q", p, w, lw, got)
			}
		}
	}
}

// "/" narrows the listing in place. Letters type into the query, arrows still
// move, and Esc drops the filter before it drops the directory.
func TestSFTPFilter(t *testing.T) {
	m := sftpFixture(t, 100, 26)
	m.sftp.focus = panelLeftFiles
	all := m.sftp.sides[sideLeft].rowCount()
	cwd := m.sftp.sides[sideLeft].cwd

	m = pressA(m, "/")
	if !m.sftp.sides[sideLeft].filtering {
		t.Fatal("/ should start a filter")
	}

	// "a" (in "ma") is a letter here, not the Append action.
	m = typeText(m, "ma")
	if n := len(m.sftp.sides[sideLeft].marks); n != 0 {
		t.Errorf("typing into the query must not mark anything, got %d marks", n)
	}
	if q := m.sftp.sides[sideLeft].query; q != "ma" {
		t.Errorf("query is %q, want \"ma\"", q)
	}
	if n := m.sftp.sides[sideLeft].rowCount(); n == 0 || n >= all {
		t.Errorf("the listing should have narrowed, %d of %d", n, all)
	}

	// Backspace unwinds; Esc drops the filter without leaving the directory.
	m = pressA(m, "backspace")
	if q := m.sftp.sides[sideLeft].query; q != "m" {
		t.Errorf("backspace should shorten the query, got %q", q)
	}
	m = pressA(m, "esc")
	if m.sftp.sides[sideLeft].filtering {
		t.Error("Esc should have dropped the filter")
	}
	if got := m.sftp.sides[sideLeft].cwd; got != cwd {
		t.Errorf("Esc on a filter must not also leave the directory, got %q", got)
	}
	if n := m.sftp.sides[sideLeft].rowCount(); n != all {
		t.Errorf("clearing the filter should restore all %d rows, got %d", all, n)
	}
}

// A filter belongs to the directory it was typed in.
func TestSFTPFilterClearsOnDirectoryChange(t *testing.T) {
	m := sftpFixture(t, 100, 26)
	m.sftp.focus = panelLeftFiles
	m = pressA(m, "/")
	m = typeText(m, "a")
	if !m.sftp.sides[sideLeft].filtering {
		t.Fatal("setup: expected a filter")
	}
	m = pressA(m, "enter") // descend into the first match
	if m.sftp.sides[sideLeft].filtering {
		t.Error("changing directory should drop the filter")
	}
}

// The crumb is plain text: lavender segments, dim slashes, no chip backgrounds
// to compete with the panel capsule directly above it.
func TestCrumbIsPlainTextNotChips(t *testing.T) {
	withColour(t)
	got := renderCrumb("~/Documents/app", 40)

	if !strings.Contains(got, ansiOf(t, editColor)) {
		t.Error("path segments should be lavender")
	}
	if !strings.Contains(got, ansiOf(t, dimColor)) {
		t.Error("separators should be dim")
	}
	for _, bg := range []lipgloss.Color{editColor, dimColor, focusColor} {
		if strings.Contains(got, ansiBgOf(t, bg)) {
			t.Errorf("the crumb must not paint backgrounds (%s)", bg)
		}
	}
	if strings.Contains(got, capLeft) || strings.Contains(got, capRight) {
		t.Error("the crumb must not use capsule caps")
	}

	// An absolute root is the leading slash itself, not a doubled one.
	if abs := stripANSI(renderCrumb("/var/log", 40)); strings.Contains(abs, "//") {
		t.Errorf("absolute root doubled the slash: %q", abs)
	}
}

// waitJob polls until the job leaves the running state.
func waitJob(t *testing.T, j *transferJob) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if j.status() != xferRunning {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("transfer never finished (%d%%)", j.percent())
}

// t sends the item under the cursor to the other side's current directory —
// from any panel, which is the whole point of the tab.
func TestSFTPTransferCursorItem(t *testing.T) {
	m := sftpFixture(t, 100, 26)
	m.sftp.focus = panelLeftFiles
	m.sftp.sides[sideLeft].cursor = 1 // a file, not the "assets" directory
	e, _ := m.sftp.sides[sideLeft].cursorEntry()
	dstDir := m.sftp.sides[sideRight].cwd

	m = pressA(m, "t")
	if len(m.transfers.jobs) != 1 {
		t.Fatalf("t should start one transfer, got %d", len(m.transfers.jobs))
	}
	waitJob(t, m.transfers.jobs[0])
	if got := m.transfers.jobs[0].status(); got != xferDone {
		t.Fatalf("transfer state %d: %s", got, m.transfers.jobs[0].err())
	}
	if !remote.Exists(m.sftp.sides[sideRight].fs, remote.Join(dstDir, e.Name)) {
		t.Errorf("%s did not arrive in %s", e.Name, dstDir)
	}
}

// a sends everything this side has marked, directories included.
func TestSFTPTransferAllMarks(t *testing.T) {
	m := sftpFixture(t, 100, 26)
	m.sftp.focus = panelLeftFiles
	m = pressA(m, "a", "j", "a") // "assets" (a directory) and the first file
	if len(m.sftp.sides[sideLeft].marks) != 2 {
		t.Fatalf("setup: expected two marks, got %d", len(m.sftp.sides[sideLeft].marks))
	}
	dstDir := m.sftp.sides[sideRight].cwd

	m = pressA(m, "T")
	if len(m.transfers.jobs) != 1 {
		t.Fatalf("T should start one transfer, got %d", len(m.transfers.jobs))
	}
	waitJob(t, m.transfers.jobs[0])
	if got := m.transfers.jobs[0].status(); got != xferDone {
		t.Fatalf("transfer state %d: %s", got, m.transfers.jobs[0].err())
	}
	if !remote.Exists(m.sftp.sides[sideRight].fs, remote.Join(dstDir, "assets")) {
		t.Error("the marked directory did not arrive")
	}
}

// An overwrite is asked about once, before anything is written — and cancelling
// writes nothing.
func TestSFTPOverwriteAsksFirst(t *testing.T) {
	m := sftpFixture(t, 100, 26)
	m.sftp.focus = panelLeftFiles
	m.sftp.sides[sideLeft].cursor = 1
	e, _ := m.sftp.sides[sideLeft].cursorEntry()

	// Put a file of the same name at the destination.
	dst := m.sftp.sides[sideRight]
	w, err := dst.fs.Create(remote.Join(dst.cwd, e.Name), 0o644)
	if err != nil {
		t.Fatal(err)
	}
	w.Write([]byte("original"))
	w.Close()

	m = pressA(m, "t")
	if !m.confirm.isActive() || m.confirm.action != confirmTransfer {
		t.Fatal("an overwrite should be confirmed first")
	}
	if len(m.transfers.jobs) != 0 {
		t.Fatal("nothing should start before the answer")
	}
	joined := strings.Join(m.confirm.lines, " ")
	if !strings.Contains(joined, "1 will overwrite") {
		t.Errorf("the count should be in the question: %q", joined)
	}

	m = pressA(m, "esc")
	if len(m.transfers.jobs) != 0 {
		t.Fatal("cancelling must not start the transfer")
	}
	got, _ := readAll(dst.fs, remote.Join(dst.cwd, e.Name))
	if got != "original" {
		t.Errorf("the destination was touched: %q", got)
	}

	// Accepting goes ahead.
	m = pressA(m, "t", "enter")
	if len(m.transfers.jobs) != 1 {
		t.Fatalf("accepting should start the transfer, got %d jobs", len(m.transfers.jobs))
	}
	waitJob(t, m.transfers.jobs[0])
	if got, _ := readAll(dst.fs, remote.Join(dst.cwd, e.Name)); got == "original" {
		t.Error("the file was not overwritten after accepting")
	}
}

func readAll(f remote.FS, p string) (string, error) {
	r, err := f.Open(p)
	if err != nil {
		return "", err
	}
	defer r.Close()
	b, err := io.ReadAll(r)
	return string(b), err
}

// Copying a directory into itself is refused up front rather than walked into.
func TestSFTPRefusesSelfCopy(t *testing.T) {
	m := sftpFixture(t, 100, 26)
	m.sftp.focus = panelLeftFiles
	// Point the other side inside this one, then send this directory.
	src := m.sftp.sides[sideLeft].cwd
	m.sftp.sides[sideRight].open(remote.Join(src, "assets"))
	m.sftp.sides[sideLeft].cursor = 0 // "assets"

	m = pressA(m, "t")
	if len(m.transfers.jobs) != 0 {
		t.Error("a self-copy should not start")
	}
	if !m.toast.isActive() || m.toast.kind != toastError {
		t.Error("a refused transfer should say why")
	}
}

// While something is moving, the tab row carries it: progress is what you glance
// at, and the marks are still visible in [5] and [7].
func TestTransferSummaryTakesTheStatusSlot(t *testing.T) {
	m := sftpFixture(t, 100, 26)
	if got := m.sftp.status(m.transfers.summary()); got == "" {
		t.Fatal("the status slot should never be empty")
	}
	if got := m.sftp.status("↑ 2/5 · 40%"); got != "↑ 2/5 · 40%" {
		t.Errorf("a running transfer should own the slot, got %q", got)
	}
	m.sftp.focus = panelLeftFiles
	m = pressA(m, "a")
	if got := m.sftp.status(""); !strings.Contains(got, "mark") {
		t.Errorf("with nothing running the slot shows marks, got %q", got)
	}
}

// The host picker is the only way a side gets a filesystem, so it is worth
// driving end to end rather than only inspecting its rows.
func TestSFTPHostPickerConnectsLocal(t *testing.T) {
	m := sftpFixture(t, 100, 26)
	m.sftp.focus = panelLeftFiles
	m.sftp.sides[sideLeft].fs = nil
	m.sftp.sides[sideLeft].host = ""

	m = pressA(m, "S")
	if !m.hostPicker.isInteractive() {
		t.Fatal("the picker should be ready for keys")
	}
	m = pressA(m, "enter") // "local" is the first selectable row

	if m.hostPicker.isActive() {
		t.Error("choosing should close the picker")
	}
	if m.sftp.sides[sideLeft].fs == nil {
		t.Fatal("the side should have a filesystem now")
	}
	if got := m.sftp.sides[sideLeft].host; got != remote.LocalLabel {
		t.Errorf("host = %q, want %q", got, remote.LocalLabel)
	}
	if m.sftp.sides[sideLeft].cwd == "" {
		t.Error("connecting should land somewhere, not nowhere")
	}
}

// A transfer can be called off, and the popup is where that happens.
func TestTransferCanBeCancelled(t *testing.T) {
	m := sftpFixture(t, 100, 26)

	// A file big enough that the copy is still running when we ask it to stop.
	big := remote.Join(m.sftp.sides[sideLeft].cwd, "big.bin")
	w, err := m.sftp.sides[sideLeft].fs.Create(big, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	w.Write(make([]byte, 32*1024*1024))
	w.Close()
	m.sftp.sides[sideLeft].reload()

	m.sftp.focus = panelLeftFiles
	for i, e := range m.sftp.sides[sideLeft].entries {
		if e.Name == "big.bin" {
			m.sftp.sides[sideLeft].cursor = i
		}
	}
	m = pressA(m, "t")
	if len(m.transfers.jobs) != 1 {
		t.Fatalf("expected one job, got %d", len(m.transfers.jobs))
	}

	m = pressA(m, "P")
	if !m.transfersUI.isInteractive() {
		t.Fatal("the transfers popup should be ready for keys")
	}
	m = pressA(m, "c")

	waitJob(t, m.transfers.jobs[0])
	if got := m.transfers.jobs[0].status(); got != xferCancelled {
		t.Errorf("state = %d, want cancelled (err: %s)", got, m.transfers.jobs[0].err())
	}
	// And nothing half-written is left where the file would have been.
	if remote.Exists(m.sftp.sides[sideRight].fs,
		remote.Join(m.sftp.sides[sideRight].cwd, "big.bin")) {
		t.Error("a cancelled transfer left a partial file behind")
	}
}
