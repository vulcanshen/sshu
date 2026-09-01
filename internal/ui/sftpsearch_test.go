package ui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/vulcanshen/sshu/internal/remote"
)

// settleScan drives the scan tick until neither side is walking. The walk is a
// goroutine, so a test that reads its results without this is racing it.
func settleScan(t *testing.T, m AppModel) AppModel {
	t.Helper()
	for i := 0; i < 500; i++ {
		next, _ := m.Update(scanTickMsg{})
		m = next.(AppModel)
		if !m.sftp.sides[sideLeft].scanning && !m.sftp.sides[sideRight].scanning {
			return m
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("the scan never finished")
	return m
}

// The whole promise of the subtree search is that finding something is the same
// as being able to act on it. It was not: every letter goes into the query while
// a search is showing (§4.5), so m / t / v / e / x all typed instead of acting,
// and Esc threw the results away and left the cursor at the top of the current
// directory. The search could tell you where a file was and then make you walk
// there yourself.
//
// Enter is the one key a search does not swallow, so Enter is what takes you
// there — the same thing every other finder does with it.
func TestEnterGoesToWhatTheSearchFound(t *testing.T) {
	m := sftpFixture(t, 100, 26)
	m.sftp.focus = panelLeftFiles
	cwd := m.sftp.sides[sideLeft].cwd
	// Several files, and the target is NOT the first one: landing at the top of
	// the listing has to be distinguishable from landing on what was found.
	for _, n := range []string{"aaa.txt", "bbb.txt", "sprite.png", "zzz.txt"} {
		if err := os.WriteFile(filepath.Join(cwd, "assets", n), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	m = pressA(m, "/")
	m = typeText(m, "sprite")
	m = settleScan(t, m)
	if e, ok := m.sftp.cur().rowAt(m.sftp.cur().cursor); !ok || e.Name != "assets/sprite.png" {
		t.Fatalf("the search did not land on the file: %q", e.Name)
	}

	m = pressA(m, "enter")
	s := m.sftp.cur()
	if s.filtering {
		t.Error("going to the result should leave the search")
	}
	if want := filepath.Join(cwd, "assets"); s.cwd != want {
		t.Errorf("landed in %q, want %q", s.cwd, want)
	}
	e, ok := s.rowAt(s.cursor)
	if !ok || e.Name != "sprite.png" {
		t.Fatalf("the cursor is on %q, want sprite.png", e.Name)
	}

	// And now the row is an ordinary row, which is what the docs promised.
	m = pressA(m, "m")
	if len(m.sftp.cur().marks) != 1 {
		t.Errorf("m did not mark it: %d marks", len(m.sftp.cur().marks))
	}
}

// A directory result still just opens, and it also leaves the search behind
// rather than keeping a query over a listing it no longer describes.
func TestEnterOnADirectoryResultOpensIt(t *testing.T) {
	m := sftpFixture(t, 100, 26)
	m.sftp.focus = panelLeftFiles
	cwd := m.sftp.sides[sideLeft].cwd

	m = pressA(m, "/")
	m = typeText(m, "assets")
	m = settleScan(t, m)
	m = pressA(m, "enter")

	s := m.sftp.cur()
	if s.filtering {
		t.Error("the search should be over")
	}
	if want := filepath.Join(cwd, "assets"); s.cwd != want {
		t.Errorf("landed in %q, want %q", s.cwd, want)
	}
}

// atRoot puts the left side at the top of the fixture tree, which is the only
// place in it with anything worth searching for underneath.
func atRoot(m AppModel) (AppModel, string) {
	root := m.sftp.sides[sideLeft].home
	m.sftp.focus = panelLeftFiles
	m.sftp.sides[sideLeft].open(root)
	return m, root
}

// `/` reaches the whole subtree, not just the directory on screen — and what it
// finds is an ordinary row, so marking it records the real absolute path.
func TestSearchFindsFilesBelowTheDirectory(t *testing.T) {
	m, root := atRoot(sftpFixture(t, 120, 30))

	m = pressA(m, "/")
	m = typeText(m, "deploy")
	m = settleScan(t, m)

	s := m.sftp.sides[sideLeft]
	if s.rowCount() == 0 {
		t.Fatal("deploy.sh is three directories down and was not found")
	}
	e, _ := s.rowAt(0)
	if want := "Documents/sideproj/app/deploy.sh"; e.Name != want {
		t.Fatalf("row 0 is %q, want %q — the path relative to the search root", e.Name, want)
	}

	m.sftp.cur().toggleMark()
	want := filepath.ToSlash(filepath.Join(root, "Documents/sideproj/app/deploy.sh"))
	if got := m.sftp.sides[sideLeft].marks; len(got) != 1 || got[0] != want {
		t.Errorf("mark is %v, want [%s]", got, want)
	}
}

// An empty query is the directory you are standing in, not everything below it.
// Pressing `/` and typing nothing must leave the listing exactly as it was, even
// after the walk has finished and the deeper results are in hand.
func TestSearchEmptyQueryShowsOnlyTheCurrentDirectory(t *testing.T) {
	m, _ := atRoot(sftpFixture(t, 120, 30))
	before := m.sftp.sides[sideLeft].rowCount()

	m = pressA(m, "/")
	m = settleScan(t, m)

	s := m.sftp.sides[sideLeft]
	if len(s.found) <= before {
		t.Fatalf("the walk found %d entries, no more than the %d already here",
			len(s.found), before)
	}
	if got := s.rowCount(); got != before {
		t.Errorf("an empty query shows %d rows, want this directory's own %d", got, before)
	}
}

// Results stream in while the cursor is live in the same list, so arrival order
// is never re-ranked: the row under the user's hand must still be there after a
// batch lands.
func TestStreamingResultsDoNotMoveTheCursor(t *testing.T) {
	s := &sftpSideModel{markedSet: map[string]bool{}, cwd: "/srv"}
	s.entries = []remote.Entry{{Name: "alpha.txt"}, {Name: "beta.txt"}}
	s.filtering, s.query = true, "a"
	s.found = append([]remote.Entry(nil), s.entries...)
	s.refilter()

	s.cursor = 1
	was, ok := s.rowAt(s.cursor)
	if !ok {
		t.Fatal("setup: expected two matches")
	}

	// A hit from a subdirectory, one that any scoring would rank first.
	sc := &searchScan{}
	sc.push([]remote.Entry{{Name: "a/aardvark.txt"}})
	sc.finish(false)
	s.scan = sc
	s.takeScan()

	now, ok := s.rowAt(s.cursor)
	if !ok || now.Name != was.Name {
		t.Errorf("the cursor was on %q and a new batch moved it to %q", was.Name, now.Name)
	}
}

// Leaving a search lands the cursor on the row it was on — searching for a file
// and then losing it on the way out is worse than not searching.
func TestLeavingASearchKeepsTheRow(t *testing.T) {
	m, _ := atRoot(sftpFixture(t, 100, 26))
	// Documents, not backups: backups is already row 0 here, so landing on it
	// would also be what losing the row looks like.
	m = pressA(m, "/")
	m = typeText(m, "Documents")
	m = settleScan(t, m)

	want, ok := m.sftp.cur().cursorEntry()
	if !ok || want.Name != "Documents" {
		t.Fatalf("setup: the cursor is on %q", want.Name)
	}

	m = pressA(m, "esc")
	got, ok := m.sftp.cur().cursorEntry()
	if !ok || got.Name != "Documents" {
		t.Errorf("Esc landed on %q, want Documents", got.Name)
	}
	if m.sftp.sides[sideLeft].cursor == 0 {
		t.Error("that is also row 0 — the test cannot tell the two outcomes apart")
	}
}

// A result from three levels down is not in this directory, so there is nowhere
// honest to put the cursor for it — that case goes to the top rather than to
// whichever row happens to hold the same index.
func TestLeavingASearchOnADeepResultGoesToTheTop(t *testing.T) {
	m, _ := atRoot(sftpFixture(t, 100, 26))
	m = pressA(m, "/")
	m = typeText(m, "deploy")
	m = settleScan(t, m)

	e, ok := m.sftp.cur().cursorEntry()
	if !ok || !strings.Contains(e.Name, "/") {
		t.Fatalf("setup: expected a deep result, got %q", e.Name)
	}

	m = pressA(m, "esc")
	if got := m.sftp.sides[sideLeft].cursor; got != 0 {
		t.Errorf("cursor is at %d, want the top", got)
	}
}

// Leaving the search has to stop the round trips, not just hide the results.
func TestDroppingTheSearchStopsTheWalk(t *testing.T) {
	m, _ := atRoot(sftpFixture(t, 120, 30))

	m = pressA(m, "/")
	if !m.sftp.sides[sideLeft].scanning {
		t.Fatal("/ should have started a walk")
	}

	m = pressA(m, "esc")
	s := m.sftp.sides[sideLeft]
	if s.scan != nil || s.scanning {
		t.Error("Esc must stop the walk, not just drop its results")
	}
	if cmd := m.sftp.takeScans(); cmd != nil {
		t.Error("the tick must stop once nothing is walking")
	}
}

// stop() is what turns "the user left the search" into "the walk is over". A
// scan that was never started must survive it too — every exit path calls stop
// without first asking whether there was anything to stop.
func TestScanStopCancelsItsContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	sc := &searchScan{cancel: cancel}
	sc.stop()
	if ctx.Err() == nil {
		t.Error("stop must cancel the walk's context")
	}

	var none *searchScan
	none.stop() // must not panic
}

// ...and clearFilter is what calls it. Without this link the walk keeps making
// round trips while the panel looks idle.
func TestClearingTheFilterCancelsItsWalk(t *testing.T) {
	cancelled := false
	s := &sftpSideModel{
		filtering: true,
		scan:      &searchScan{cancel: func() { cancelled = true }},
	}
	s.clearFilter()
	if !cancelled {
		t.Error("clearFilter must cancel the walk it started")
	}
}

// Changing directory ends the search with it: a result set is relative to the
// directory it was rooted at, and the walk under the old one is now wasted work.
func TestEnteringADirectoryEndsTheSearch(t *testing.T) {
	m, _ := atRoot(sftpFixture(t, 120, 30))

	m = pressA(m, "/")
	m = typeText(m, "doc")
	m = settleScan(t, m)
	m = pressA(m, "enter")

	s := m.sftp.sides[sideLeft]
	if s.filtering || s.scanning || s.scan != nil {
		t.Error("descending into a result should end the search and its walk")
	}
	if len(s.found) != 0 {
		t.Errorf("%d results survived the directory change", len(s.found))
	}
}

// The count says how much of what has been seen is on screen, and whether the
// walk is still going — a number that stops climbing has to mean finished, not
// stalled.
func TestScanNoteSaysWhereTheWalkIs(t *testing.T) {
	s := sftpSideModel{filtering: true}
	s.found = make([]remote.Entry, 840)
	s.matches = make([]int, 12)

	s.scanning = true
	if got, want := s.scanNote(), "12 of 840 …"; got != want {
		t.Errorf("while walking: %q, want %q", got, want)
	}
	s.scanning = false
	if got, want := s.scanNote(), "12 of 840"; got != want {
		t.Errorf("when done: %q, want %q", got, want)
	}
	s.capped = true
	if got := s.scanNote(); !strings.Contains(got, "capped") {
		t.Errorf("a capped walk must say so, got %q", got)
	}
}

// A half-printed count is not a shortened count, it is a DIFFERENT number: cut
// "12 of 840" to fit and it reads "12 of 8". So when it does not fit it is
// dropped whole, and the query — the thing being typed — keeps the room.
func TestNarrowSearchRowDropsTheCountRatherThanCutIt(t *testing.T) {
	s := sftpSideModel{filtering: true, query: "a-fairly-long-query"}
	s.found = make([]remote.Entry, 840)
	s.matches = make([]int, 12)

	row := searchRow(s, 24)
	if got := dispW(row); got != 24 {
		t.Fatalf("row is %d cells, want 24: %q", got, row)
	}
	if plain := ansi.Strip(row); strings.ContainsAny(plain, "0123456789") {
		t.Errorf("the count did not fit and was sliced instead of dropped: %q", plain)
	}

	if plain := ansi.Strip(searchRow(s, 60)); !strings.Contains(plain, "12 of 840") {
		t.Errorf("with room to spare the count should be there, got %q", plain)
	}
}

// The frame invariant again, this time with deep relative paths in the rows and
// a count sharing the path line — both of which are new ways to be a cell wide.
func TestSearchPreservesFrame(t *testing.T) {
	for _, sz := range [][2]int{{120, 40}, {100, 26}, {80, 20}, {72, 16}, {50, 12}, {30, 9}} {
		w, h := sz[0], sz[1]
		m, _ := atRoot(sftpFixture(t, w, h))

		m = pressA(m, "/")
		m = typeText(m, "e") // matches deep, long-pathed results
		m = settleScan(t, m)

		for _, q := range []string{"", strings.Repeat("x", 40)} {
			m.sftp.sides[sideLeft].query += q
			m.sftp.sides[sideLeft].refilter()
			for i, l := range strings.Split(m.View(), "\n") {
				if lw := dispW(l); lw != w {
					t.Errorf("%dx%d query=%dch line %d is %d cells, want %d: %q",
						w, h, len(m.sftp.sides[sideLeft].query), i, lw, w, l)
					break
				}
			}
		}
	}
}
