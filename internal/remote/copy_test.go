package remote

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// tree builds a small directory and returns its slash path.
func tree(t *testing.T) string {
	t.Helper()
	root := filepath.ToSlash(t.TempDir())
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(os.MkdirAll(filepath.Join(root, "src", "nested"), 0o755))
	must(os.WriteFile(filepath.Join(root, "src", "a.txt"), []byte("hello"), 0o644))
	must(os.WriteFile(filepath.Join(root, "src", "run.sh"), []byte("#!/bin/sh\n"), 0o755))
	must(os.WriteFile(filepath.Join(root, "src", "nested", "b.bin"), make([]byte, 5000), 0o644))
	must(os.MkdirAll(filepath.Join(root, "dst"), 0o755))
	return root
}

// A plan is made in full before anything is written, so the size of the job is
// known from the first frame rather than discovered halfway through.
func TestPlanExpandsDirectories(t *testing.T) {
	root := tree(t)
	f := Local()

	items, total, err := Plan(f, []string{root + "/src"}, root+"/dst")
	if err != nil {
		t.Fatal(err)
	}
	if total != int64(len("hello")+len("#!/bin/sh\n")+5000) {
		t.Errorf("total bytes = %d", total)
	}

	var dirs, files int
	for _, it := range items {
		if it.IsDir {
			dirs++
		} else {
			files++
		}
	}
	if dirs != 2 { // src and src/nested — an empty directory must arrive too
		t.Errorf("dirs = %d, want 2", dirs)
	}
	if files != 3 {
		t.Errorf("files = %d, want 3", files)
	}
	for _, it := range items {
		if !strings.HasPrefix(it.Dst, root+"/dst/src") {
			t.Errorf("destination %q is outside the target directory", it.Dst)
		}
	}
}

func TestCopyRoundTripsContentAndMode(t *testing.T) {
	root := tree(t)
	f := Local()
	items, _, err := Plan(f, []string{root + "/src"}, root+"/dst")
	if err != nil {
		t.Fatal(err)
	}

	var moved int64
	for _, it := range items {
		if err := CopyItem(context.Background(), f, f, it, func(n int64) { moved += n }); err != nil {
			t.Fatal(err)
		}
	}

	got, err := os.ReadFile(filepath.Join(root, "dst", "src", "a.txt"))
	if err != nil || string(got) != "hello" {
		t.Fatalf("content did not survive: %q %v", got, err)
	}
	if moved == 0 {
		t.Error("progress was never reported")
	}
	// An executable that arrives without its bit is a script that no longer runs.
	st, err := os.Stat(filepath.Join(root, "dst", "src", "run.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm()&0o100 == 0 {
		t.Errorf("the executable bit was lost: %o", st.Mode().Perm())
	}
	if _, err := os.Stat(filepath.Join(root, "dst", "src", "nested", "b.bin")); err != nil {
		t.Errorf("the nested file did not arrive: %v", err)
	}
}

// A cancelled copy must not leave a partial file behind. A truncated file that
// looks like the real one is the worst possible outcome: whatever reads it next
// gets short data with nothing to say so.
func TestCancelledCopyLeavesNoPartialFile(t *testing.T) {
	root := tree(t)
	f := Local()
	big := filepath.Join(root, "big.bin")
	if err := os.WriteFile(big, make([]byte, 4*1024*1024), 0o644); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	dst := root + "/dst/big.bin"
	it := Item{Src: filepath.ToSlash(big), Dst: dst, Size: 4 * 1024 * 1024, Mode: 0o644}

	err := CopyItem(ctx, f, f, it, func(int64) { cancel() }) // cancel on the first block
	if err == nil {
		t.Fatal("a cancelled copy should report an error")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("error should carry the cancellation: %v", err)
	}
	if _, err := os.Stat(filepath.FromSlash(dst)); err == nil {
		t.Error("the partial file was left behind")
	}
}

// Conflicts are found before the transfer starts, so the question is asked once
// for the batch rather than per file with half of it already committed.
func TestConflictsFindsExistingDestinations(t *testing.T) {
	root := tree(t)
	f := Local()
	items, _, err := Plan(f, []string{root + "/src"}, root+"/dst")
	if err != nil {
		t.Fatal(err)
	}
	if n := len(Conflicts(f, items)); n != 0 {
		t.Fatalf("a fresh destination has no conflicts, got %d", n)
	}

	if err := os.MkdirAll(filepath.Join(root, "dst", "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "dst", "src", "a.txt"), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	c := Conflicts(f, items)
	if len(c) != 1 || !strings.HasSuffix(c[0].Dst, "a.txt") {
		t.Errorf("expected a.txt to conflict, got %v", c)
	}
}

// Copying a directory into itself walks forever; it is refused up front.
func TestSameTreeCatchesSelfCopy(t *testing.T) {
	f := Local()
	if !SameTree(f, "/a/b", f, "/a/b/c") {
		t.Error("a destination inside the source should be caught")
	}
	if !SameTree(f, "/a/b", f, "/a/b") {
		t.Error("the same directory should be caught")
	}
	if SameTree(f, "/a/b", f, "/a/c") {
		t.Error("a sibling is not a self-copy")
	}
	if SameTree(f, "/a/b", f, "/a/bc") {
		t.Error("a name prefix is not a path prefix")
	}
}
