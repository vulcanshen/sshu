package remote

import (
	"os"
	"path/filepath"
	"testing"
)

// A recursive delete must not follow a symlink out of the tree. Deleting the
// contents of a directory somebody merely linked to is the worst surprise this
// app could produce.
func TestRemoveAllDoesNotFollowASymlink(t *testing.T) {
	root := t.TempDir()
	keep := filepath.Join(root, "keep")
	if err := os.MkdirAll(keep, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(keep, "precious.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(keep, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	if err := RemoveAll(Local(), filepath.ToSlash(link)); err != nil {
		t.Fatalf("removing the link failed: %v", err)
	}
	if _, err := os.Lstat(link); err == nil {
		t.Error("the link itself should be gone")
	}
	if _, err := os.Stat(filepath.Join(keep, "precious.txt")); err != nil {
		t.Error("the delete walked through the link and emptied its target")
	}
}
