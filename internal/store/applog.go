package store

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// applogs.yaml is the app log on disk. Unlike hosts.yaml it is APPEND-oriented:
// the document is a bare top-level YAML list, so recording an event is
// appending the bytes of a one-element list to the end of the file — no
// read-modify-write on the hot path, and a crash mid-write costs one entry,
// not the file.
//
// The file holds what failed connections printed — banners, fingerprints,
// paths off other people's machines — so it gets the same handling as the
// files that hold passwords: 0600 re-asserted, a warning header, and a place
// in the same directory the rest of the secrets live.

// LogEntry is one recorded event. Msg may span lines; the UI capped and
// sanitised it before it got here.
type LogEntry struct {
	At    time.Time `yaml:"at"`
	Level string    `yaml:"level"` // info | warn | error
	Msg   string    `yaml:"msg"`
}

// logKeep is how many entries survive a trim — the same depth the UI keeps in
// memory, so the file never remembers less than the panel shows.
const logKeep = 500

// logTrimBytes is the size past which an append also trims the file. A var so
// tests can reach the trim without writing a megabyte first.
var logTrimBytes = int64(1 << 20)

const applogHeader = `# sshu app log — one YAML list, newest at the bottom. sshu trims it itself.
#
# WARNING: failed-connection entries keep everything the far end printed —
# banners, key fingerprints, paths. Same rule as the rest of this directory:
# keep it out of version control, out of syncing folders, out of backups.
`

// LogPath is the full path to applogs.yaml.
func LogPath() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "applogs.yaml"), nil
}

// AppendLog records one event at the end of applogs.yaml.
func AppendLog(e LogEntry) error {
	path, err := LogPath()
	if err != nil {
		return err
	}
	return AppendLogTo(path, e)
}

// AppendLogTo is AppendLog against an explicit path (tests).
func AppendLogTo(path string, e LogEntry) error {
	body, err := yaml.Marshal([]LogEntry{e})
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	st, err := f.Stat()
	if err != nil {
		f.Close()
		return err
	}
	if st.Size() == 0 {
		if _, err := f.WriteString(applogHeader); err != nil {
			f.Close()
			return err
		}
	}
	_, werr := f.Write(body)
	cerr := f.Close()
	// Re-assert 0600 the way every secret-bearing file here does: a file that
	// was widened by hand narrows again on the next event.
	_ = os.Chmod(path, 0o600)
	if werr != nil {
		return werr
	}
	if cerr != nil {
		return cerr
	}
	if st.Size() > logTrimBytes {
		return trimLog(path)
	}
	return nil
}

// trimLog rewrites the file down to its newest logKeep entries. An unreadable
// file is truncated to just the tail that DID parse — the log is a record, not
// data; keeping it appendable beats preserving a corrupt head.
func trimLog(path string) error {
	entries, _ := LoadLogFrom(path)
	if len(entries) > logKeep {
		entries = entries[len(entries)-logKeep:]
	}
	body, err := yaml.Marshal(entries)
	if err != nil {
		return err
	}
	// Bound by bytes as well as by count: a few hundred forty-line screens can
	// out-size the threshold with the count cap intact, and a trim that leaves
	// the file over its own trigger just schedules the next full rewrite one
	// append away. Halve until the result is genuinely smaller than the trigger.
	for int64(len(body)) > logTrimBytes/2 && len(entries) > 1 {
		entries = entries[len(entries)/2:]
		if body, err = yaml.Marshal(entries); err != nil {
			return err
		}
	}
	return writeFile0600(path, append([]byte(applogHeader), body...))
}

// ClearLog empties applogs.yaml. The HEADER stays: the warning above the
// entries is about the file rather than about any one of them, and a cleared
// log is still the file the next event appends to.
func ClearLog() error {
	path, err := LogPath()
	if err != nil {
		return err
	}
	return ClearLogTo(path)
}

// ClearLogTo is ClearLog against an explicit path (tests).
func ClearLogTo(path string) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil // never written, nothing to erase
	}
	return writeFile0600(path, []byte(applogHeader))
}

// LoadLog reads applogs.yaml back, oldest first. Missing is an empty log.
func LoadLog() ([]LogEntry, error) {
	path, err := LogPath()
	if err != nil {
		return nil, err
	}
	return LoadLogFrom(path)
}

// LoadLogFrom is LoadLog against an explicit path (tests).
func LoadLogFrom(path string) ([]LogEntry, error) {
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var entries []LogEntry
	if err := yaml.Unmarshal(raw, &entries); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return entries, nil
}
