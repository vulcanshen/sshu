package tui

import (
	"bytes"
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

// Open gives the file's first 64 KB, all the preview reads of it.
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
	return nopCloser{bytes.NewReader([]byte(r.s))}, nil
}

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

type nopCloser struct{ *bytes.Reader }

func (nopCloser) Close() error { return nil }

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
