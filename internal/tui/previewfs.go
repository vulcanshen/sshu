package tui

import (
	"errors"
	"io"
	"io/fs"
	"path"
	"sync"
	"time"

	terminu "github.com/vulcanshen/terminu-go"

	"github.com/vulcanshen/sshu/internal/remote"
)

// previewFS is a side's file system as the lib's file preview reads it
// (terminu.PreviewFS), so a remote host's files in a search get the same
// preview, tree and facts as this machine's (components/input/file-picker).
// The preview is worked out each time it is drawn, and a remote host answers
// over the network: what it read once it keeps, for as long as the search is
// open.
// TODO(lib SS40): the lib reads while it draws, so the first look at an item
// holds the frame until the host answers.
type previewFS struct {
	fs remote.FS

	mu    sync.Mutex
	stat  map[string]infoOrErr
	lstat map[string]infoOrErr
	link  map[string]textOrErr
	dir   map[string]dirOrErr
	head  map[string]textOrErr
}

type infoOrErr struct {
	fi  fs.FileInfo
	err error
}

type textOrErr struct {
	s   string
	err error
}

type dirOrErr struct {
	fis []fs.FileInfo
	err error
}

// previewHead is as much of a file as the preview reads: 64 KB
// (components/input/file-picker).
const previewHead = 64 << 10

func newPreviewFS(f remote.FS) *previewFS {
	return &previewFS{fs: f, stat: map[string]infoOrErr{}, lstat: map[string]infoOrErr{},
		link: map[string]textOrErr{}, dir: map[string]dirOrErr{}, head: map[string]textOrErr{}}
}

func (p *previewFS) Stat(name string) (fs.FileInfo, error) {
	r := cachedRaw(p, p.stat, name, func() infoOrErr { e, err := p.fs.Stat(name); return infoOrErr{entryInfo(e), err} })
	return r.fi, r.err
}

func (p *previewFS) Lstat(name string) (fs.FileInfo, error) {
	r := cachedRaw(p, p.lstat, name, func() infoOrErr { e, err := p.fs.Lstat(name); return infoOrErr{entryInfo(e), err} })
	return r.fi, r.err
}

func (p *previewFS) ReadLink(name string) (string, error) {
	r := cachedRaw(p, p.link, name, func() textOrErr { s, err := p.fs.ReadLink(name); return textOrErr{s, err} })
	return r.s, r.err
}

func (p *previewFS) ReadDir(name string) ([]fs.FileInfo, error) {
	r := cachedRaw(p, p.dir, name, func() dirOrErr {
		es, err := p.fs.List(name)
		fis := make([]fs.FileInfo, len(es))
		for i, e := range es {
			fis[i] = entryInfo(e)
		}
		return dirOrErr{fis, err}
	})
	return r.fis, r.err
}

// Open gives the file: its first 64 KB, all the preview reads of most files,
// from what was read before; the rest — the end of a zip, a whole tar, a
// PDF's pages — from the host when it is asked for.
func (p *previewFS) Open(name string) (terminu.PreviewFile, error) {
	r := cachedRaw(p, p.head, name, func() textOrErr {
		f, err := p.fs.Open(name)
		if err != nil {
			return textOrErr{err: err}
		}
		defer f.Close()
		b, err := io.ReadAll(io.LimitReader(f, previewHead))
		return textOrErr{string(b), err}
	})
	if r.err != nil {
		return nil, r.err
	}
	return &previewFile{fs: p.fs, name: name, head: []byte(r.s)}, nil
}

// previewFile is an open file of a previewFS. Past its cached head it opens
// the file on the host, once, and reads there.
type previewFile struct {
	fs   remote.FS
	name string
	head []byte
	off  int64
	rest io.ReadCloser // opened past the head
}

func (f *previewFile) ReadAt(b []byte, off int64) (int, error) {
	end := off + int64(len(b))
	whole := len(f.head) < previewHead // the head is all there is
	switch {
	case off >= int64(len(f.head)) && whole:
		return 0, io.EOF
	case end <= int64(len(f.head)) || whole:
		n := copy(b, f.head[off:])
		if n < len(b) {
			return n, io.EOF
		}
		return n, nil
	}
	if f.rest == nil {
		rc, err := f.fs.Open(f.name)
		if err != nil {
			return 0, err
		}
		f.rest = rc
	}
	ra, ok := f.rest.(io.ReaderAt)
	if !ok {
		return 0, errNoReadAt
	}
	return ra.ReadAt(b, off)
}

func (f *previewFile) Read(b []byte) (int, error) {
	n, err := f.ReadAt(b, f.off)
	f.off += int64(n)
	if err == io.EOF && n > 0 {
		err = nil
	}
	return n, err
}

func (f *previewFile) Close() error {
	if f.rest != nil {
		return f.rest.Close()
	}
	return nil
}

var errNoReadAt = errors.New("this file system cannot read from an offset")

// cachedRaw is what read gave for name the first time it was asked.
func cachedRaw[T any](p *previewFS, m map[string]T, name string, read func() T) T {
	p.mu.Lock()
	r, ok := m[name]
	p.mu.Unlock()
	if ok {
		return r
	}
	r = read()
	p.mu.Lock()
	m[name] = r
	p.mu.Unlock()
	return r
}

// entryInfo is a listing's entry as a fs.FileInfo.
func entryInfo(e remote.Entry) fs.FileInfo { return entryFileInfo{e} }

type entryFileInfo struct{ e remote.Entry }

func (i entryFileInfo) Name() string { return path.Base(i.e.Name) }
func (i entryFileInfo) Size() int64  { return i.e.Size }
func (i entryFileInfo) Mode() fs.FileMode {
	if i.e.IsDir {
		return i.e.Mode | fs.ModeDir
	}
	return i.e.Mode
}
func (i entryFileInfo) ModTime() time.Time { return i.e.ModTime }
func (i entryFileInfo) IsDir() bool        { return i.e.IsDir }
func (i entryFileInfo) Sys() any           { return nil }
