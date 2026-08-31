// Package remote is the filesystem the sftp tab browses: either this machine or
// a host reached over SFTP, behind one interface so the panel above cannot tell
// the difference.
//
// Both sides of the tab are the same type, which is what makes "upload",
// "download" and "remote to remote" one operation instead of three.
package remote

import (
	"io"
	"io/fs"
	"path"
	"sort"
	"time"
)

// Entry is one directory entry, reduced to what the browser draws.
type Entry struct {
	Name    string
	IsDir   bool
	Size    int64
	Mode    fs.FileMode
	ModTime time.Time
}

// FS is a browsable filesystem.
//
// Paths are always slash-separated and absolute. The local implementation
// converts at its own boundary, so the UI never has to care which side it is
// talking to.
type FS interface {
	// Label is what the panel title shows: "local", or the host's name.
	Label() string
	// Home is where a side starts.
	Home() (string, error)
	// List returns dir's entries, directories first then names, both
	// case-insensitively — the order a person scans in, not the order the
	// server happened to return.
	List(dir string) ([]Entry, error)
	// Stat is used to decide whether a path is a directory before entering it.
	Stat(p string) (Entry, error)
	// Lstat is Stat without following a symlink. Anything DESTRUCTIVE must use
	// it: a symlink to a directory stats as a directory, and a recursive delete
	// that believed Stat would walk into the target and empty it out.
	Lstat(p string) (Entry, error)

	// The write half, used only by transfers. It lives on the same interface so a
	// copy never has to know which end is local — upload, download and
	// remote-to-remote become one code path with different values.
	Open(p string) (io.ReadCloser, error)
	Create(p string, mode fs.FileMode) (io.WriteCloser, error)
	MkdirAll(p string) error
	Remove(p string) error
	// Rename moves a path within this filesystem. Callers check the destination
	// first: os.Rename would overwrite it and SFTP's would refuse, and one
	// operation must not depend on which end it lands on.
	Rename(from, to string) error

	Close() error
}

// Exists reports whether p is there. Transfers use it to find overwrites before
// starting, so the question is asked once for the whole batch rather than per
// file, halfway through.
func Exists(f FS, p string) bool {
	_, err := f.Stat(p)
	return err == nil
}

// RemoveAll deletes p, and everything under it if it is a directory.
//
// It walks with Lstat and with the LISTING's IsDir, both of which report a
// symlink as a symlink. The link is unlinked; whatever it points at is left
// alone. Deleting the contents of a directory somebody merely linked to would be
// the worst kind of surprise this app could produce.
func RemoveAll(f FS, p string) error {
	e, err := f.Lstat(p)
	if err != nil {
		return err
	}
	if e.IsDir {
		entries, err := f.List(p)
		if err != nil {
			return err
		}
		for _, c := range entries {
			if err := RemoveAll(f, Join(p, c.Name)); err != nil {
				return err
			}
		}
	}
	return f.Remove(p)
}

// Join builds a child path. It is package-level rather than a method because
// every FS uses slash paths, so there is nothing per-implementation about it.
func Join(dir, name string) string { return path.Join(dir, name) }

// Parent is dir's parent, or dir itself at the root — going up from "/" must
// not walk off the end.
func Parent(dir string) string {
	p := path.Dir(dir)
	if p == dir {
		return dir
	}
	return p
}

// sortEntries puts directories first, then sorts by name. Applied by every
// implementation so both sides of the tab read the same way.
func sortEntries(es []Entry) {
	sort.SliceStable(es, func(i, j int) bool {
		if es[i].IsDir != es[j].IsDir {
			return es[i].IsDir
		}
		return foldLess(es[i].Name, es[j].Name)
	})
}

// foldLess compares case-insensitively, falling back to the raw comparison so
// the order is total (otherwise "README" and "readme" would sort unstably).
func foldLess(a, b string) bool {
	la, lb := lower(a), lower(b)
	if la != lb {
		return la < lb
	}
	return a < b
}

func lower(s string) string {
	out := []rune(s)
	for i, r := range out {
		if r >= 'A' && r <= 'Z' {
			out[i] = r + 32
		}
	}
	return string(out)
}
