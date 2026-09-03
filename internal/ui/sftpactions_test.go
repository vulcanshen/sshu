package ui

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/vulcanshen/sshu/internal/remote"
	"github.com/vulcanshen/sshu/internal/store"
)

// `t` and `T` are two actions, not two spellings of one: the item under the
// cursor, or every mark on this side. Case is significant here and nowhere else,
// which is why the test sends BOTH and checks what actually arrived.
func TestTransferCursorAndTransferAllAreDifferentKeys(t *testing.T) {
	// deploy.sh and main.go marked; the cursor left on README.md, which is
	// deliberately not one of them.
	setUp := func() (AppModel, string) {
		t.Helper()
		m := sftpFixture(t, 100, 26)
		m.sftp.focus = panelLeftFiles
		m = pressA(m, "j", "a", "j", "a", "j")
		if n := len(m.sftp.sides[sideLeft].marks); n != 2 {
			t.Fatalf("setup: expected two marks, got %d", n)
		}
		if e, _ := m.sftp.cur().cursorEntry(); e.Name != "README.md" {
			t.Fatalf("setup: cursor is on %q, want README.md", e.Name)
		}
		return m, m.sftp.sides[sideRight].cwd
	}

	m, dst := setUp()
	m = pressA(m, "t")
	if n := len(m.transfers.jobs); n != 1 {
		t.Fatalf("t should start one transfer, got %d", n)
	}
	waitJob(t, m.transfers.jobs[0])
	fs := m.sftp.sides[sideRight].fs
	if !remote.Exists(fs, remote.Join(dst, "README.md")) {
		t.Error("t must send the item under the cursor")
	}
	if remote.Exists(fs, remote.Join(dst, "deploy.sh")) {
		t.Error("t sent a marked item — that is T's job")
	}

	m, dst = setUp()
	m = pressA(m, "T")
	if n := len(m.transfers.jobs); n != 1 {
		t.Fatalf("T should start one transfer, got %d", n)
	}
	waitJob(t, m.transfers.jobs[0])
	fs = m.sftp.sides[sideRight].fs
	for _, name := range []string{"deploy.sh", "main.go"} {
		if !remote.Exists(fs, remote.Join(dst, name)) {
			t.Errorf("T must send every mark, %s is missing", name)
		}
	}
	if remote.Exists(fs, remote.Join(dst, "README.md")) {
		t.Error("T sent the cursor item — that is t's job")
	}
}

// A side with no host has nothing to mark, send or reset. Offering those rows
// anyway teaches that the menu does not mean what it says (§A.1).
func TestASideWithNoHostOnlyOffersSelectHost(t *testing.T) {
	m := New(sample(), nil, store.DefaultConfig())
	next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 26})
	m = settle(next.(AppModel))
	m.tab = tabFT
	m.sftp.focus = panelLeftFiles

	items := m.sftpMenuItems()
	if len(items) != 1 || items[0].key != keySelectHost {
		var got []string
		for _, it := range items {
			got = append(got, it.key)
		}
		t.Fatalf("menu offers %v, want only %q", got, keySelectHost)
	}

	// The letters must agree with the menu, or one of them is lying.
	if after := pressA(m, "/"); after.sftp.sides[sideLeft].filtering {
		t.Error("with no host there is nothing to search")
	}
	if after := pressA(m, "t"); len(after.transfers.jobs) != 0 {
		t.Error("with no host there is nothing to transfer")
	}
	if after := pressA(m, "H"); !after.hostPicker.isActive() {
		t.Error("H must still open the host picker")
	}
}

// The Space menu belongs to a PANEL, not to a tab: in a split tab "what can I do
// here" depends on which panel you are standing in, and a title naming the tab
// cannot tell [4] from [6].
func TestMenuTitleNamesTheFocusedPanel(t *testing.T) {
	m := sftpFixture(t, 100, 26)
	for _, tc := range []struct {
		p    sftpPanel
		want string
	}{
		{panelLeftFiles, "[1] local"},
		{panelLeftMarks, "[2] Marks"},
		{panelRightFiles, "[3] local"},
		{panelRightMarks, "[4] Marks"},
	} {
		m.sftp.focus = tc.p
		if got := m.menuTitle(); got != tc.want {
			t.Errorf("focus %d: menu title %q, want %q", tc.p, got, tc.want)
		}
		// And it is the same string the panel's own capsule shows, because it
		// comes from the same function.
		if view := ansi.Strip(m.View()); !strings.Contains(view, tc.want) {
			t.Errorf("focus %d: %q is not on screen", tc.p, tc.want)
		}
	}
}

// Tab [3] gets the same treatment: the menu says which of [4]/[5]/[6] it is for.
func TestSSHMenuTitleNamesTheFocusedPanel(t *testing.T) {
	m := appWith(sample(), nil)
	m.tab = tabSSH
	for _, tc := range []struct {
		p    sshPanel
		want string
	}{
		{panelSessions, "[1] sessions"},
		{panelLayout, "[2] layout"},
	} {
		m.ssh.setFocus(tc.p)
		if got := m.menuTitle(); got != tc.want {
			t.Errorf("focus %d: menu title %q, want %q", tc.p, got, tc.want)
		}
	}
}

// The prompt is the search glyph, not a literal "/". Echoing the key that opened
// the search makes a query CONTAINING a slash unreadable — /tmp would render as
// //tmp and there is no way to tell which slash is yours.
func TestSearchPromptIsAGlyphNotASlash(t *testing.T) {
	m, _ := atRoot(sftpFixture(t, 100, 26))
	m = pressA(m, "/")
	m = typeText(m, "/tmp")

	row := ansi.Strip(searchRow(m.sftp.sides[sideLeft], 40))
	if !strings.Contains(row, glyphSearch) {
		t.Errorf("no search glyph in the prompt: %q", row)
	}
	if strings.Contains(row, "//tmp") {
		t.Errorf("the prompt slash collided with the query: %q", row)
	}
	if !strings.Contains(row, "/tmp") {
		t.Errorf("the query itself did not survive: %q", row)
	}
}

// Committing an action from the Space menu hands the keyboard back at once. The
// close animation is a visual, not a modal state — while it ran, the next
// keystroke was swallowed by a popup already on its way out, so Esc had to be
// pressed twice to leave the search.
func TestAClosingPopupDoesNotEatTheNextKey(t *testing.T) {
	m, _ := atRoot(sftpFixture(t, 100, 26))
	cwd := m.sftp.sides[sideLeft].cwd

	m = pressA(m, " ")
	next, _ := m.Update(keyMsg("/")) // commit Search, then do NOT settle
	m = next.(AppModel)
	if !m.sftp.sides[sideLeft].filtering {
		t.Fatal("setup: Search should have started")
	}
	if !m.spaceMenu.isActive() {
		t.Fatal("setup: the menu should still be animating out")
	}

	next, _ = m.Update(keyMsg("esc"))
	m = next.(AppModel)
	if m.sftp.sides[sideLeft].filtering {
		t.Error("Esc was eaten by the closing menu instead of leaving the search")
	}
	if got := m.sftp.sides[sideLeft].cwd; got != cwd {
		t.Errorf("Esc left the directory rather than the search: %q", got)
	}
}

// Esc leaves the search before it leaves the directory, whichever way the search
// was started.
func TestEscLeavesTheSearchBeforeTheDirectory(t *testing.T) {
	for _, how := range []string{"hotkey", "menu"} {
		m, _ := atRoot(sftpFixture(t, 100, 26))
		cwd := m.sftp.sides[sideLeft].cwd
		if how == "menu" {
			m = pressA(m, " ")
		}
		m = pressA(m, "/")
		m = typeText(m, "dep")

		m = pressA(m, "esc")
		if s := m.sftp.sides[sideLeft]; s.filtering {
			t.Errorf("%s: Esc did not leave the search", how)
		}
		if got := m.sftp.sides[sideLeft].cwd; got != cwd {
			t.Errorf("%s: Esc also left the directory: %q", how, got)
		}
		// A second Esc is the one that goes up.
		m = pressA(m, "esc")
		if got := m.sftp.sides[sideLeft].cwd; got == cwd {
			t.Errorf("%s: a second Esc should go up, still at %q", how, got)
		}
	}
}

// h/l cross between the halves, keeping the row: the two sides are mirror
// images, so [5] goes to [7] and not to [6]. Tab is for "the next panel"; h/l
// are for "the other host", which on a 1:1 split is what you usually mean.
func TestHLCrossesSides(t *testing.T) {
	m := sftpFixture(t, 100, 26)
	for _, tc := range []struct {
		from sftpPanel
		key  string
		want sftpPanel
	}{
		{panelLeftFiles, "l", panelRightFiles},
		{panelRightFiles, "h", panelLeftFiles},
		{panelLeftMarks, "l", panelRightMarks},
		{panelRightMarks, "h", panelLeftMarks},
		// Already there: it stays put rather than wrapping round.
		{panelLeftFiles, "h", panelLeftFiles},
		{panelRightMarks, "l", panelRightMarks},
		// The arrows are synonyms.
		{panelLeftFiles, "right", panelRightFiles},
		{panelRightFiles, "left", panelLeftFiles},
	} {
		m.sftp.focus = tc.from
		m = pressA(m, tc.key)
		if m.sftp.focus != tc.want {
			t.Errorf("%q from panel %d landed on %d, want %d",
				tc.key, tc.from, m.sftp.focus, tc.want)
		}
	}
}

// While a search is being typed, h and l are letters.
func TestHLAreLettersWhileSearching(t *testing.T) {
	m, _ := atRoot(sftpFixture(t, 100, 26))
	m = pressA(m, "/")
	m = typeText(m, "hl")

	if got := m.sftp.sides[sideLeft].query; got != "hl" {
		t.Errorf("query is %q, want \"hl\"", got)
	}
	if m.sftp.focus != panelLeftFiles {
		t.Errorf("typing into the query moved the focus to panel %d", m.sftp.focus)
	}
}

// Leaving takes the sftp connections with it. An ssh connection left open
// because the process exited is the server's problem to time out.
func TestQuitClosesTheSftpConnections(t *testing.T) {
	m := sftpFixture(t, 100, 26)
	if m.sftp.sides[sideLeft].fs == nil || m.sftp.sides[sideRight].fs == nil {
		t.Fatal("setup: both sides should be connected")
	}

	m = pressA(m, "q")
	if m.confirm.isActive() {
		t.Fatal("nothing is running, so quitting should not ask")
	}
	for _, sd := range []side{sideLeft, sideRight} {
		if m.sftp.sides[sd].fs != nil {
			t.Errorf("side %d was left connected", sd)
		}
	}
}

// Ctrl+C is the emergency exit and it tears down the same three things.
func TestForceQuitClosesTheSftpConnections(t *testing.T) {
	m := sftpFixture(t, 100, 26)
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	m = next.(AppModel)
	for _, sd := range []side{sideLeft, sideRight} {
		if m.sftp.sides[sd].fs != nil {
			t.Errorf("side %d was left connected", sd)
		}
	}
}

// A half-copied file is as much "something to lose" as an idle shell, so it gets
// the same warning rather than being dropped silently.
func TestQuitAsksAboutARunningTransfer(t *testing.T) {
	m := sftpFixture(t, 100, 26)
	m.sftp.focus = panelLeftFiles
	m.transfers.jobs = append(m.transfers.jobs, &transferJob{
		id: 1, label: "test", files: 1, cancel: func() {},
	})

	m = pressA(m, "q")
	if !m.confirm.isActive() || m.confirm.action != confirmQuit {
		t.Fatal("a running transfer should raise the quit confirmation")
	}
	if lines := m.quitCost(); len(lines) != 1 ||
		!strings.Contains(lines[0], "transfer") {
		t.Errorf("the confirmation should name the transfer, got %v", lines)
	}
}

// arrivingAt parks a job whose current item is p, the way a real one looks
// mid-copy. A real local copy is over before the next keystroke, so the state
// under test has to be held still.
func arrivingAt(m AppModel, p string) AppModel {
	j := runningJob(1, 50, 100)
	j.cur.Store(&p)
	m.transfers.jobs = append(m.transfers.jobs, j)
	return m
}

// A file still being written into is not a thing you can act on yet. Marking it
// would promise otherwise — and a mark is what [T] sends onward, so the promise
// would be kept by shipping a truncation nobody was told about.
func TestAFileStillArrivingCannotBeMarked(t *testing.T) {
	m := sftpFixture(t, 100, 26)
	m.sftp.focus = panelLeftFiles
	p, ok := m.sftpCursorPath()
	if !ok {
		t.Fatal("setup: no row under the cursor")
	}
	m = arrivingAt(m, p)

	after := pressA(m, "a")
	if len(after.sftp.sides[sideLeft].marks) != 0 {
		t.Errorf("marked a file that is still arriving: %v", after.sftp.sides[sideLeft].marks)
	}
	if !strings.Contains(ansi.Strip(after.View()), "Still arriving") {
		t.Errorf("the refusal has to say why:\n%s", ansi.Strip(after.View()))
	}

	// The same key on a row that is NOT arriving still marks, so the refusal is
	// about the file rather than about the transfer being on at all.
	m.sftp.sides[sideLeft].cursor++
	if got := pressA(m, "a"); len(got.sftp.sides[sideLeft].marks) != 1 {
		t.Error("a settled file must still be markable while another one arrives")
	}
}

// The mark cell carries the spinner while the row is being written into. It
// takes that cell rather than a column of its own: the cell is free by
// construction, and a row that grew a column mid-transfer would shift every
// name beside it.
func TestAnArrivingRowSpinsInTheMarkColumn(t *testing.T) {
	withColour(t)
	m := sftpFixture(t, 100, 26)
	m.sftp.focus = panelLeftFiles
	p, _ := m.sftpCursorPath()
	m = arrivingAt(m, p)
	m.sftp.sides[sideLeft].cursor = 999 // off this row, so the cursor bar is not what we read

	body := m.sftp.fileRows(m.sftp.sides[sideLeft], 60, 10, m.transfers.arrivals())
	if len(body) == 0 {
		t.Fatal("no rows rendered")
	}
	row := body[0]
	if !strings.Contains(row, spinnerFrames[0]) {
		t.Errorf("the arriving row should carry the spinner:\n%q", ansi.Strip(row))
	}
	if !strings.Contains(row, ansiOf(t, handColor)) {
		t.Errorf("the spinner is work happening, not a mark you left:\n%q", row)
	}

	// Every other row is untouched — and the frame is still exactly as wide.
	if strings.Contains(body[1], spinnerFrames[0]) {
		t.Error("a settled row must not spin")
	}

	// A file can be marked and THEN overwritten by a transfer. The arrival
	// wins the cell: "not all here yet" is the more urgent of the two facts,
	// and the mark it hides is one the user can no longer act on anyway.
	side := m.sftp.sides[sideLeft]
	side.markedSet = map[string]bool{p: true}
	side.marks = []string{p}
	row = m.sftp.fileRows(side, 60, 10, m.transfers.arrivals())[0]
	if !strings.Contains(row, spinnerFrames[0]) {
		t.Errorf("the arrival should take the cell from the mark:\n%q", ansi.Strip(row))
	}
	if strings.Contains(row, glyphMark) {
		t.Errorf("both cannot be in one cell:\n%q", ansi.Strip(row))
	}
	for i, r := range body {
		if got := dispW(ansi.Strip(r)); got != 60 {
			t.Errorf("row %d is %d cells wide, want 60", i, got)
		}
	}
}

// Transfer a directory and the row you can SEE is the directory — the file
// being written is three levels down inside it. So a directory the bytes are
// landing in counts as arriving too; otherwise the whole thing is invisible in
// the case people actually use marks for.
func TestADirectoryBeingFilledCountsAsArriving(t *testing.T) {
	a := arrivals{paths: []string{"/dst/blob/deep/f01.bin"}, frame: "x"}
	for _, p := range []string{"/dst/blob/deep/f01.bin", "/dst/blob/deep", "/dst/blob", "/dst"} {
		if !a.receiving(p) {
			t.Errorf("%q should read as receiving", p)
		}
	}
	for _, p := range []string{"/dst/blob/deep/f02.bin", "/dst/blobby", "/other", ""} {
		if a.receiving(p) {
			t.Errorf("%q is not being written into", p)
		}
	}
	// The separator is what makes it "inside", not a shared spelling. Without
	// it /dst/blob would swallow every sibling whose name merely starts the
	// same way.
	sibling := arrivals{paths: []string{"/dst/blobby/f01.bin"}, frame: "x"}
	if sibling.receiving("/dst/blob") {
		t.Error("/dst/blob is not the directory /dst/blobby is filling")
	}
	if !sibling.receiving("/dst/blobby") {
		t.Error("/dst/blobby is")
	}
	if (arrivals{}).receiving("/dst/blob") {
		t.Error("nothing is arriving when nothing is running")
	}
}

// The spinner has to stop AND the listing has to catch up, together, at the
// moment the job ends — not two seconds later when the watch poll notices.
func TestTheEndOfAJobStopsTheSpinnerAndRelists(t *testing.T) {
	m := sftpFixture(t, 100, 26)
	dst := m.sftp.sides[sideRight]
	landed := filepath.Join(dst.cwd, "arrived.txt")
	m = arrivingAt(m, landed)
	if !m.transfers.arrivals().receiving(landed) {
		t.Fatal("setup: the job should be reported as arriving")
	}
	before := len(m.sftp.sides[sideRight].entries)

	// The file finishes, and so does the job.
	if err := os.WriteFile(landed, []byte("here"), 0o644); err != nil {
		t.Fatal(err)
	}
	// cur is deliberately NOT cleared here. The copy goroutine stores the end
	// state before its deferred clear runs, so "ended but cur still set" is a
	// window that really happens — and a job that is over must stop reporting
	// an arrival on the state alone.
	m.transfers.jobs[0].state.Store(int32(xferDone))

	next, _ := m.Update(xferTickMsg{})
	m = next.(AppModel)

	if m.transfers.arrivals().receiving(landed) {
		t.Error("a finished job must stop reporting an arrival")
	}
	if got := len(m.sftp.sides[sideRight].entries); got != before+1 {
		t.Errorf("the destination should have been re-listed: %d entries, want %d",
			got, before+1)
	}
}

// A second job still running must not make the first one's ending re-list —
// and, more to the point, must not make a still-running job report an ending.
func TestOnlyAnActualEndingTriggersTheRelist(t *testing.T) {
	m := sftpFixture(t, 100, 26)
	m.transfers.jobs = append(m.transfers.jobs, runningJob(1, 50, 100), runningJob(2, 10, 100))
	if m.logFinishedTransfers() {
		t.Fatal("two running jobs, nothing has ended")
	}
	m.transfers.jobs[0].state.Store(int32(xferDone))
	if !m.logFinishedTransfers() {
		t.Error("one of them ended, in front of a job that is still going")
	}
	if m.logFinishedTransfers() {
		t.Error("an ending is reported once, not on every pass")
	}
}

// heldFS is the real filesystem with its reads held until the test lets go.
// It is how a copy is caught MID-FLIGHT without a sleep: every other method is
// the real one, so what runs is the real transfer path.
type heldFS struct {
	remote.FS
	release <-chan struct{}
}

func (f heldFS) Open(p string) (io.ReadCloser, error) {
	rc, err := f.FS.Open(p)
	if err != nil {
		return nil, err
	}
	return &heldReader{ReadCloser: rc, release: f.release}, nil
}

type heldReader struct {
	io.ReadCloser
	release <-chan struct{}
	waited  bool
}

func (r *heldReader) Read(p []byte) (int, error) {
	if !r.waited {
		r.waited = true
		<-r.release
	}
	return r.ReadCloser.Read(p)
}

// The engine has to actually publish what it is writing, and stop publishing
// when it stops writing. Everything else about arrivals is tested against a
// parked job, which would keep passing with the wiring cut out entirely.
func TestARunningJobPublishesTheFileItIsWriting(t *testing.T) {
	m := sftpFixture(t, 100, 26)
	src, dst := m.sftp.sides[sideLeft], m.sftp.sides[sideRight]
	e, ok := src.rowAt(1) // a file, not the directory
	if !ok || e.IsDir {
		t.Fatal("setup: wanted a file under row 1")
	}
	from := remote.Join(src.cwd, e.Name)
	want := remote.Join(dst.cwd, e.Name)

	items, total, err := remote.Plan(src.fs, []string{from}, dst.cwd)
	if err != nil {
		t.Fatal(err)
	}

	release := make(chan struct{})
	m.transfers.start(heldFS{FS: src.fs, release: release}, dst.fs, items, total, "one file")
	j := m.transfers.jobs[0]

	waitFor(t, "the job to name what it is writing", func() bool { return j.cur.Load() != nil })
	if got := *j.cur.Load(); got != want {
		t.Errorf("the job is writing %q, want %q", got, want)
	}
	if !m.transfers.arrivals().receiving(want) {
		t.Error("a running job's file must read as arriving")
	}

	close(release)
	waitJob(t, j)
	if p := j.cur.Load(); p != nil {
		t.Errorf("a finished job is writing nothing, got %q", *p)
	}
	if m.transfers.arrivals().receiving(want) {
		t.Error("nothing is arriving once the job is over")
	}
}

// countingFS is the real filesystem that says how many times it was listed.
type countingFS struct {
	remote.FS
	lists *atomic.Int32
}

func (f countingFS) List(dir string) ([]remote.Entry, error) {
	f.lists.Add(1)
	return f.FS.List(dir)
}

// The re-list is tied to a job ENDING, not to the frame clock. The tick runs at
// 120ms while bytes move; re-listing on each one would put a round trip per
// frame on a link that is already busy carrying the transfer — which is why the
// watch loop polls at two seconds and only re-lists when the mtime moved.
func TestATransferInFlightDoesNotRelistOnEveryFrame(t *testing.T) {
	m := sftpFixture(t, 100, 26)
	var lists atomic.Int32
	m.sftp.sides[sideRight].fs = countingFS{FS: m.sftp.sides[sideRight].fs, lists: &lists}
	m.transfers.jobs = append(m.transfers.jobs, runningJob(1, 50, 100))

	for i := 0; i < 8; i++ {
		next, _ := m.Update(xferTickMsg{})
		m = next.(AppModel)
	}
	if n := lists.Load(); n != 0 {
		t.Errorf("a running transfer listed the destination %d times", n)
	}

	m.transfers.jobs[0].state.Store(int32(xferDone))
	next, _ := m.Update(xferTickMsg{})
	m = next.(AppModel)
	if n := lists.Load(); n != 1 {
		t.Errorf("the ending should have listed it exactly once, got %d", n)
	}

	// And it stays listed once: the ending is reported one time.
	next, _ = m.Update(xferTickMsg{})
	if n := lists.Load(); n != 1 {
		t.Errorf("the ending re-listed again on the next frame, %d total", n)
	}
	_ = next
}
