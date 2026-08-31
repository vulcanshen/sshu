package remote

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

// The local copy keeps the original name, because that is what an editor reads
// to decide what the file IS: syntax rules, filetype detection and modelines all
// key off it.
func TestFetchKeepsTheFilename(t *testing.T) {
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "nginx.conf"), []byte("server {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	got, err := Fetch(context.Background(), Local(), filepath.ToSlash(filepath.Join(src, "nginx.conf")), dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(got) != "nginx.conf" {
		t.Errorf("the copy is called %q, want nginx.conf", filepath.Base(got))
	}
	body, _ := os.ReadFile(got)
	if string(body) != "server {}\n" {
		t.Errorf("the copy holds %q", body)
	}
}

// breakingFS creates for real — so a write that goes to the wrong path really
// does truncate it — and then fails part way through.
type breakingFS struct {
	FS
	after int
}

type breakingWriter struct {
	w     io.WriteCloser
	n     int
	after int
}

var errBroke = errors.New("link dropped")

func (b *breakingWriter) Write(p []byte) (int, error) {
	if b.n >= b.after {
		return 0, errBroke
	}
	n := min(len(p), b.after-b.n)
	if _, err := b.w.Write(p[:n]); err != nil {
		return 0, err
	}
	b.n += n
	return n, errBroke
}

func (b *breakingWriter) Close() error { return b.w.Close() }

func (f breakingFS) Create(p string, mode fs.FileMode) (io.WriteCloser, error) {
	w, err := f.FS.Create(p, mode)
	if err != nil {
		return nil, err
	}
	return &breakingWriter{w: w, after: f.after}, nil
}

// The whole point of the sibling-then-move dance: a write that dies half way
// must not leave a half file where the real one was. A truncated config is worse
// than a failed save, because nothing about it says it is truncated.
func TestWriteBackLeavesTheOriginalWhenTheWriteFails(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "app.conf")
	const original = "listen 80;\nroot /srv;\n"
	if err := os.WriteFile(target, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	edit := filepath.Join(t.TempDir(), "app.conf")
	if err := os.WriteFile(edit, []byte("listen 443;\nroot /srv;\nssl on;\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	f := breakingFS{FS: Local(), after: 4}
	if err := WriteBack(f, edit, filepath.ToSlash(target), 0o644); err == nil {
		t.Fatal("a broken write should be reported, not swallowed")
	}
	body, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("the original is gone: %v", err)
	}
	if string(body) != original {
		t.Errorf("the original was damaged by a failed save:\n got %q\nwant %q", body, original)
	}
}

// Editing ~/.zshrc must not turn it into a regular file. Renaming over a symlink
// replaces the link, and the dotfile quietly stops pointing at the repository it
// came from — so a symlink is written THROUGH instead.
func TestWriteBackWritesThroughASymlink(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "zshrc")
	link := filepath.Join(dir, ".zshrc")
	if err := os.WriteFile(real, []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("no symlinks here: %v", err)
	}
	edit := filepath.Join(t.TempDir(), ".zshrc")
	if err := os.WriteFile(edit, []byte("new\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := WriteBack(Local(), edit, filepath.ToSlash(link), 0o644); err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(link)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Error("the symlink was replaced by a regular file")
	}
	body, _ := os.ReadFile(real)
	if string(body) != "new\n" {
		t.Errorf("the link's target holds %q, want the edit", body)
	}
}

// An atomic replace is the normal path, and it has to actually land.
func TestWriteBackReplacesARegularFile(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "notes.md")
	if err := os.WriteFile(target, []byte("before\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	edit := filepath.Join(t.TempDir(), "notes.md")
	if err := os.WriteFile(edit, []byte("after\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := WriteBack(Local(), edit, filepath.ToSlash(target), 0o644); err != nil {
		t.Fatal(err)
	}
	body, _ := os.ReadFile(target)
	if string(body) != "after\n" {
		t.Errorf("the target holds %q, want the edit", body)
	}
	// And no debris beside it.
	if _, err := os.Stat(target + editTempSuffix); err == nil {
		t.Error("the sibling temp file was left behind")
	}
}
