package remote

import (
	"context"
	"crypto/sha256"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"time"
)

// Editing is the one operation that moves a file BOTH ways: down to somewhere a
// local program can open it, and back again if it changed. Both halves live here
// for the same reason the rest of this package does — so the tab above never has
// to know which side it is pointed at.

// Stamp is what a file looked like when it was read.
//
// It is taken again before the write-back. Minutes can pass inside an editor,
// and somebody else can write to the same file during them; overwriting that
// silently is the one outcome this feature must not have.
type Stamp struct {
	Size    int64
	ModTime time.Time
}

// StampOf reads p's stamp as it is now.
func StampOf(f FS, p string) (Stamp, error) {
	e, err := f.Stat(p)
	if err != nil {
		return Stamp{}, err
	}
	return Stamp{Size: e.Size, ModTime: e.ModTime}, nil
}

// LocalPath answers "can an editor be pointed straight at this file?".
//
// When it can, there is no copy at all. Editing in place keeps the inode, so
// hard links, ownership and extended attributes survive — a round trip through a
// temp file and a rename would break all three, and on this machine there is no
// network failure to buy protection from.
func LocalPath(f FS, p string) (string, bool) {
	if _, ok := f.(localFS); ok {
		return filepath.FromSlash(p), true
	}
	return "", false
}

// StartDir is where a side should open when it connects.
//
// A remote side opens at its home: there is nowhere else it could mean. THIS
// machine opens where sshu was launched, because that is the directory you were
// standing in when you decided you needed sshu — `cd ~/release && sshu` should
// already be looking at the release, not at a home directory full of dotfiles
// you then have to navigate out of.
//
// sshu never changes its own working directory, so asking for it here is the
// same as having captured it at startup.
func StartDir(f FS, home string) string {
	if _, ok := f.(localFS); ok {
		if wd, err := os.Getwd(); err == nil {
			return filepath.ToSlash(wd)
		}
	}
	return home
}

// Fetch copies p into dir under its own name and returns the local path.
//
// The name is kept because editors read it: syntax rules, filetype detection and
// modelines all key off it, and nginx.conf opened as sshu-edit-417 is a
// different editing session than the one you asked for.
//
// There is no size limit, deliberately. What edit needs is "never write back a
// partial read", and that is bought by reading the whole file or failing — not
// by refusing large ones. It streams, so size costs time and disk, never memory,
// and ctx is what makes the time cancellable.
func Fetch(ctx context.Context, f FS, p, dir string, onBytes func(int64)) (string, error) {
	name := path.Base(p)
	if name == "" || name == "." || name == "/" {
		name = "file"
	}
	dst := filepath.Join(dir, name)

	r, err := f.Open(p)
	if err != nil {
		return "", err
	}
	defer r.Close()

	w, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return "", err
	}
	err = pump(ctx, w, r, onBytes)
	if cerr := w.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		_ = os.Remove(dst)
		return "", err
	}
	return dst, nil
}

// Digest is how "you edited this" is told apart from "you looked at it and
// quit". Not mtime: an editor rewrites the file on :w even when every byte is
// the same, so mtime answers a different question than the one being asked.
func Digest(p string) ([32]byte, error) {
	f, err := os.Open(p)
	if err != nil {
		return [32]byte{}, err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return [32]byte{}, err
	}
	var out [32]byte
	copy(out[:], h.Sum(nil))
	return out, nil
}

// editTempSuffix names the sibling file a write-back lands in first.
const editTempSuffix = ".sshu-tmp"

// WriteBack replaces p's contents with the file at src.
//
// The bytes go to a sibling name first and are then moved over the target, so
// the file is never seen half-written: a link that drops mid-upload leaves the
// original intact rather than a truncated config that the next process to read
// it will believe.
//
// Two cases step around that, and both are deliberate:
//
//   - a SYMLINK is written through. Renaming over one replaces the link with a
//     regular file — the dotfile you edited would quietly stop pointing at the
//     repository it came from.
//   - a directory that will not take a new file (your file, somebody else's
//     directory) falls back to writing in place, because the alternative is not
//     being able to save at all.
//
// Each trades the atomic replace for a short window where the file is
// incomplete. In both, that is the smaller harm.
func WriteBack(f FS, src, p string, mode fs.FileMode) error {
	r, err := os.Open(src)
	if err != nil {
		return err
	}
	defer r.Close()

	inPlace := false
	if e, lerr := f.Lstat(p); lerr == nil && e.Mode&fs.ModeSymlink != 0 {
		inPlace = true
	}

	dst := p + editTempSuffix
	var w io.WriteCloser
	if !inPlace {
		if w, err = f.Create(dst, mode); err != nil {
			inPlace = true
		}
	}
	if inPlace {
		dst = p
		if w, err = f.Create(dst, mode); err != nil {
			return err
		}
	}

	_, err = io.Copy(w, r)
	if cerr := w.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		if !inPlace {
			_ = f.Remove(dst)
		}
		return err
	}
	if inPlace {
		return nil
	}
	if err := f.Replace(dst, p); err != nil {
		_ = f.Remove(dst)
		return err
	}
	return nil
}
