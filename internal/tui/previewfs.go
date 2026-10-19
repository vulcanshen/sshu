package tui

import (
	"errors"
	"io/fs"
	"path"
	"time"

	terminu "github.com/vulcanshen/terminu-go"

	"github.com/vulcanshen/sshu/internal/remote"
)

// previewFS is a side's file system as the lib's file preview reads it
// (terminu.PreviewFS), so a remote host's files in a search get the same
// preview, tree and facts as this machine's (components/input/file-picker).
// The lib reads it in the background and keeps what it read while the popup
// is open.
type previewFS struct{ fs remote.FS }

func newPreviewFS(f remote.FS) *previewFS { return &previewFS{fs: f} }

func (p *previewFS) Stat(name string) (fs.FileInfo, error) {
	e, err := p.fs.Stat(name)
	return entryInfo(e), err
}

func (p *previewFS) Lstat(name string) (fs.FileInfo, error) {
	e, err := p.fs.Lstat(name)
	return entryInfo(e), err
}

func (p *previewFS) ReadLink(name string) (string, error) { return p.fs.ReadLink(name) }

func (p *previewFS) ReadDir(name string) ([]fs.FileInfo, error) {
	es, err := p.fs.List(name)
	fis := make([]fs.FileInfo, len(es))
	for i, e := range es {
		fis[i] = entryInfo(e)
	}
	return fis, err
}

// Open gives the file itself: both file systems' open files read from any
// offset too (the end of a zip, a PDF's pages).
func (p *previewFS) Open(name string) (terminu.PreviewFile, error) {
	f, err := p.fs.Open(name)
	if err != nil {
		return nil, err
	}
	pf, ok := f.(terminu.PreviewFile)
	if !ok {
		f.Close()
		return nil, errNoReadAt
	}
	return pf, nil
}

var errNoReadAt = errors.New("this file system cannot read from an offset")

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
