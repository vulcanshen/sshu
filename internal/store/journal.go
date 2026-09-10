package store

import (
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// Three journals, one mechanism.
//
// applogs.yaml was one file holding everything that happened, at three levels,
// in one free-text field. That worked while there was one question to answer.
// There are three, and they want different shapes:
//
//	errors.yaml    — what went wrong, and everything the far end said about it
//	history.yaml   — every connection attempt and how it ended, nothing else
//	activity.yaml  — what the user changed through sshu
//
// A single file could not be all three: history wants one fixed-width row per
// entry so a machine's record reads down a column, while an error entry is
// fifteen lines of somebody else's banner. Putting them together meant the
// useful shape of each was the other's noise.
//
// All three are APPEND-oriented, like the log they replace: the document is a
// bare top-level YAML list, so recording an event appends the bytes of a
// one-element list — no read-modify-write on the hot path, and a crash
// mid-write costs one entry rather than the file.

// ErrorEntry is one failure. Host and User are empty for the failures that
// have no connection behind them — a config file that would not parse, a
// version that cannot be written — and the panel shows a placeholder there
// rather than inventing one.
//
// Level is warn or error, and both live here because a warning is the same
// KIND of news: something is not right. Only error counts as unread, so the
// badge still means "something failed", not "something was mentioned".
type ErrorEntry struct {
	At    time.Time `yaml:"at"`
	Host  string    `yaml:"host,omitempty"`
	User  string    `yaml:"user,omitempty"`
	Level string    `yaml:"level"` // warn | error
	// Cause is the headline — one line, what the table shows and what a toast
	// could hold. Error is the whole of it, Cause included: everything the far
	// end printed before it gave up, which is fifteen lines for a host key
	// mismatch and one for a refused connection.
	//
	// The repeat is deliberate. Cause could be derived from Error's first line
	// and is stored anyway, so the file is readable by hand without the reader
	// having to know that rule — and so a headline that is NOT the first line
	// stays possible later without a format change.
	//
	// An entry written before this column existed has no Cause, and the UI
	// falls back to Error's first line rather than showing a blank.
	Cause string `yaml:"cause,omitempty"`
	Error string `yaml:"error"` // may span lines
}

// HistoryEntry is one connection attempt. Result only — the reason lives in
// errors.yaml, and duplicating it here would mean two records of one failure
// that can disagree.
type HistoryEntry struct {
	At     time.Time `yaml:"at"`
	Host   string    `yaml:"host"`
	User   string    `yaml:"user"`
	Result string    `yaml:"result"` // success | fail
}

// ActivityEntry is one change the user made through sshu: a host added, a
// credential deleted, a file transferred, an edit written back. Not connection
// state, and not UI state — locking a pty or zooming a cell changes what you
// are looking at, not what is there.
type ActivityEntry struct {
	At     time.Time `yaml:"at"`
	Action string    `yaml:"action"`
}

// Result values for HistoryEntry, so the two ends agree on the spelling.
const (
	ResultSuccess = "success"
	ResultFail    = "fail"
)

// Level values for ErrorEntry.
const (
	LevelWarn  = "warn"
	LevelError = "error"
)

// journalKeep is how many entries survive a trim — the same depth the UI keeps
// in memory, so a file never remembers less than its panel shows.
const journalKeep = 500

// journalTrimBytes is the size past which an append also trims. A var so tests
// can reach the trim without writing a megabyte first.
var journalTrimBytes = int64(1 << 20)

const errorsHeader = `# sshu errors — one YAML list, newest at the bottom. sshu trims it itself.
#
# WARNING: entries keep everything the far end printed — banners, key
# fingerprints, paths. Same rule as the rest of this directory: keep it out of
# version control, out of syncing folders, out of backups.
`

const historyHeader = `# sshu connection history — one YAML list, newest at the bottom.
#
# Result only. Why a connection failed is in errors.yaml; this file is the
# timeline. It names the hosts and users you connect as, so it belongs in the
# same directory as the rest and out of version control.
`

const activityHeader = `# sshu activity — what was changed through sshu, one YAML list, newest at the
# bottom.
#
# It names hosts, users and paths. Same rule as the rest of this directory:
# keep it out of version control, out of syncing folders, out of backups.
`

// ErrorsPath, HistoryPath and ActivityPath are the three files.
func ErrorsPath() (string, error)   { return journalPath("errors.yaml") }
func HistoryPath() (string, error)  { return journalPath("history.yaml") }
func ActivityPath() (string, error) { return journalPath("activity.yaml") }

func journalPath(name string) (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, name), nil
}

// AppendError, AppendHistory and AppendActivity record one event.
func AppendError(e ErrorEntry) error {
	path, err := ErrorsPath()
	if err != nil {
		return err
	}
	return AppendErrorTo(path, e)
}

func AppendHistory(e HistoryEntry) error {
	path, err := HistoryPath()
	if err != nil {
		return err
	}
	return AppendHistoryTo(path, e)
}

func AppendActivity(e ActivityEntry) error {
	path, err := ActivityPath()
	if err != nil {
		return err
	}
	return AppendActivityTo(path, e)
}

// The *To variants take an explicit path (tests).
func AppendErrorTo(path string, e ErrorEntry) error {
	return appendJournal(path, e, errorsHeader, loadErrorsFrom)
}

func AppendHistoryTo(path string, e HistoryEntry) error {
	return appendJournal(path, e, historyHeader, loadHistoryFrom)
}

func AppendActivityTo(path string, e ActivityEntry) error {
	return appendJournal(path, e, activityHeader, loadActivityFrom)
}

// LoadErrors, LoadHistory and LoadActivity read a journal back. A missing file
// is the ordinary empty state, exactly as it is for hosts.yaml.
func LoadErrors() ([]ErrorEntry, error) {
	path, err := ErrorsPath()
	if err != nil {
		return nil, err
	}
	return loadErrorsFrom(path)
}

func LoadHistory() ([]HistoryEntry, error) {
	path, err := HistoryPath()
	if err != nil {
		return nil, err
	}
	return loadHistoryFrom(path)
}

func LoadActivity() ([]ActivityEntry, error) {
	path, err := ActivityPath()
	if err != nil {
		return nil, err
	}
	return loadActivityFrom(path)
}

func loadErrorsFrom(path string) ([]ErrorEntry, error) {
	return loadJournal[ErrorEntry](path)
}

func loadHistoryFrom(path string) ([]HistoryEntry, error) {
	return loadJournal[HistoryEntry](path)
}

func loadActivityFrom(path string) ([]ActivityEntry, error) {
	return loadJournal[ActivityEntry](path)
}

// ClearErrors, ClearHistory and ClearActivity empty one journal, keeping its
// header — the same shape ClearLog had, and for the same reason: a file whose
// warning has been deleted is a file whose warning is gone for good.
func ClearErrors() error   { return clearJournalAt(ErrorsPath, errorsHeader) }
func ClearHistory() error  { return clearJournalAt(HistoryPath, historyHeader) }
func ClearActivity() error { return clearJournalAt(ActivityPath, activityHeader) }

func clearJournalAt(path func() (string, error), header string) error {
	p, err := path()
	if err != nil {
		return err
	}
	return ClearJournalTo(p, header)
}

// ClearJournalTo empties a journal at an explicit path (tests). A file that is
// not there is already empty.
func ClearJournalTo(path, header string) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil
	}
	return writeFile0600(path, []byte(header))
}

// ErrorsHeader, HistoryHeader and ActivityHeader expose the headers so the UI
// can clear a journal through ClearJournalTo without restating them.
func ErrorsHeader() string   { return errorsHeader }
func HistoryHeader() string  { return historyHeader }
func ActivityHeader() string { return activityHeader }

// ---------------------------------------------------------------- mechanism

// appendJournal writes one entry at the end of path, creating the file with
// its header when it is not there yet, and trimming when it has grown past
// journalTrimBytes.
//
// The entry is marshalled as a ONE-ELEMENT LIST and appended, which is what
// makes this cheap: what comes back is not a list of lists, because a bare
// "- item" block at the end of a bare list document is simply another item.
func appendJournal[T any](path string, e T, header string, load func(string) ([]T, error)) error {
	body, err := yaml.Marshal([]T{e})
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}

	st, statErr := os.Stat(path)
	if os.IsNotExist(statErr) {
		return writeFile0600(path, append([]byte(header), body...))
	}
	if statErr != nil {
		return statErr
	}

	// Past the cap the whole file is rewritten with only the tail kept. Doing
	// it here rather than on a timer means the cost lands on the append that
	// crossed the line, and never on a read.
	if st.Size() > journalTrimBytes {
		kept, loadErr := load(path)
		if loadErr == nil {
			kept = append(kept, e)
			if len(kept) > journalKeep {
				kept = kept[len(kept)-journalKeep:]
			}
			trimmed, marshalErr := yaml.Marshal(kept)
			if marshalErr == nil {
				return writeFile0600(path, append([]byte(header), trimmed...))
			}
		}
		// A file that cannot be re-read is not a reason to lose the new entry;
		// fall through and append to it as it stands.
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	// Re-assert the mode: a file widened by hand narrows again on the next
	// event, the same rule every other file in this directory follows.
	_ = f.Chmod(0o600)
	_, err = f.Write(body)
	return err
}

func loadJournal[T any](path string) ([]T, error) {
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []T
	if err := yaml.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	if len(out) > journalKeep {
		out = out[len(out)-journalKeep:]
	}
	return out, nil
}
