package remote

import (
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

// LocalLabel is what the local side is called in the host picker and the panel
// title. A name, not a path: "local" is a place you understand without reading.
const LocalLabel = "local"

// localFS is this machine. It exists so the sftp tab can have a local side
// without the panel above branching on which kind of side it is drawing.
type localFS struct{}

// Local returns the local filesystem.
func Local() FS { return localFS{} }

func (localFS) Label() string { return LocalLabel }

func (localFS) Home() (string, error) {
	h, err := os.UserHomeDir()
	if err != nil {
		return "/", err
	}
	return filepath.ToSlash(h), nil
}

func (localFS) List(dir string) ([]Entry, error) {
	des, err := os.ReadDir(filepath.FromSlash(dir))
	if err != nil {
		return nil, err
	}
	out := make([]Entry, 0, len(des))
	for _, de := range des {
		info, err := de.Info()
		if err != nil {
			// A vanished or unreadable entry should not empty the whole
			// listing; show what is knowable and move on.
			out = append(out, Entry{Name: de.Name(), IsDir: de.IsDir()})
			continue
		}
		out = append(out, Entry{
			Name: de.Name(), IsDir: de.IsDir(),
			Size: info.Size(), Mode: info.Mode(), ModTime: info.ModTime(),
		})
	}
	sortEntries(out)
	return out, nil
}

func (localFS) Stat(p string) (Entry, error) {
	info, err := os.Stat(filepath.FromSlash(p))
	if err != nil {
		return Entry{}, err
	}
	return Entry{
		Name: info.Name(), IsDir: info.IsDir(),
		Size: info.Size(), Mode: info.Mode(), ModTime: info.ModTime(),
	}, nil
}

func (localFS) Lstat(p string) (Entry, error) {
	info, err := os.Lstat(filepath.FromSlash(p))
	if err != nil {
		return Entry{}, err
	}
	return Entry{
		Name: info.Name(), IsDir: info.IsDir(),
		Size: info.Size(), Mode: info.Mode(), ModTime: info.ModTime(),
	}, nil
}

func (localFS) Rename(from, to string) error {
	return os.Rename(filepath.FromSlash(from), filepath.FromSlash(to))
}

// os.Rename already replaces, so here the two are the same call. The interface
// splits them because the far end does not.
func (localFS) Replace(from, to string) error {
	return os.Rename(filepath.FromSlash(from), filepath.FromSlash(to))
}

func (localFS) Close() error { return nil }

func (localFS) Open(p string) (io.ReadCloser, error) {
	return os.Open(filepath.FromSlash(p))
}

func (localFS) Create(p string, mode fs.FileMode) (io.WriteCloser, error) {
	// The mode comes from the source, so a transferred script stays executable.
	// 0600 is the floor for anything arriving without one.
	if mode == 0 {
		mode = 0o600
	}
	return os.OpenFile(filepath.FromSlash(p), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode.Perm())
}

func (localFS) MkdirAll(p string) error {
	return os.MkdirAll(filepath.FromSlash(p), 0o755)
}

func (localFS) Remove(p string) error {
	return os.Remove(filepath.FromSlash(p))
}
