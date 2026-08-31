package remote

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// collect runs a scan over root's subdirectories and returns the names in the
// order they were emitted — the order is the thing being tested, so it is never
// sorted here.
func collect(t *testing.T, ctx context.Context, root string, limit int) ([]string, bool) {
	t.Helper()
	f := Local()
	var subs []string
	entries, err := f.List(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.IsDir {
			subs = append(subs, Join(root, e.Name))
		}
	}
	var got []string
	capped := Scan(ctx, f, root, subs, limit, func(batch []Entry) {
		for _, e := range batch {
			got = append(got, e.Name)
		}
	})
	return got, capped
}

// Results are named relative to the search root, which is what lets the panel
// keep treating them as ordinary rows: Join(cwd, Name) is still the real path.
func TestScanNamesResultsRelativeToRoot(t *testing.T) {
	root := tree(t)
	got, capped := collect(t, context.Background(), root, SearchCap)
	if capped {
		t.Error("a four-entry tree should not hit the cap")
	}

	want := map[string]bool{
		"src/nested": true, "src/a.txt": true, "src/run.sh": true,
		"src/nested/b.bin": true,
	}
	for _, name := range got {
		if strings.HasPrefix(name, "/") {
			t.Errorf("%q is absolute; results are relative to the root", name)
		}
		delete(want, name)
	}
	if len(want) > 0 {
		t.Errorf("never found %v (got %v)", want, got)
	}
}

// Breadth-first is a promise about waiting, not about tidiness: over SFTP every
// directory is a round trip, so what is near has to arrive before what is far.
//
// Two siblings are what makes the difference visible. A directory is listed in
// one call, so its own children always arrive together whichever order the walk
// takes; only a SIBLING's shallow entries can lose the race to a deep one.
func TestScanIsBreadthFirst(t *testing.T) {
	root := filepath.ToSlash(t.TempDir())
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(os.MkdirAll(filepath.Join(root, "a", "deep"), 0o755))
	must(os.MkdirAll(filepath.Join(root, "b"), 0o755))
	must(os.WriteFile(filepath.Join(root, "a", "deep", "far.txt"), []byte("x"), 0o644))
	must(os.WriteFile(filepath.Join(root, "b", "near.txt"), []byte("x"), 0o644))

	got, _ := collect(t, context.Background(), root, SearchCap)

	far, near := -1, -1
	for i, name := range got {
		switch name {
		case "a/deep/far.txt":
			far = i
		case "b/near.txt":
			near = i
		}
	}
	if far < 0 || near < 0 {
		t.Fatalf("expected both entries, got %v", got)
	}
	if far < near {
		t.Errorf("depth 3 arrived at %d, before the sibling's depth 2 at %d: %v",
			far, near, got)
	}
}

// The cap is reported, not silently applied — a truncated result set that claims
// to be complete is worse than a slow one.
func TestScanReportsItsCap(t *testing.T) {
	root := tree(t)
	got, capped := collect(t, context.Background(), root, 2)
	if !capped {
		t.Error("stopping at the limit must report capped")
	}
	if len(got) != 2 {
		t.Errorf("emitted %d entries, want 2: %v", len(got), got)
	}
}

// Cancelling has to stop the round trips, not just hide the results: leaving a
// search must stop working the connection.
func TestScanStopsWhenCancelled(t *testing.T) {
	root := tree(t)
	f := Local()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	batches := 0
	Scan(ctx, f, root, []string{Join(root, "src"), Join(root, "dst")}, SearchCap,
		func([]Entry) {
			batches++
			cancel() // the walk must not queue another directory after this
		})
	if batches != 1 {
		t.Errorf("kept walking after cancel: %d batches", batches)
	}
}

// One unreadable directory is ordinary on someone else's machine; losing every
// result below the walk because of it is not a reasonable answer to it.
func TestScanSkipsAnUnreadableDirectory(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can read anything")
	}
	root := tree(t)
	locked := filepath.Join(root, "locked")
	if err := os.Mkdir(locked, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(locked, 0o755) })

	got, _ := collect(t, context.Background(), root, SearchCap)
	found := false
	for _, name := range got {
		if name == "src/nested/b.bin" {
			found = true
		}
	}
	if !found {
		t.Errorf("an unreadable directory ended the whole walk: %v", got)
	}
}
