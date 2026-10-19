package store

import (
	"os"
	"path/filepath"
)

// Where an older sshu kept something, when Migrate could not move it: Dir
// and DataDir go on using it (tdp E3, "when the place changes").
var settingsAt, journalsAt string

// Move is one thing Migrate moved, or could not.
type Move struct {
	What     string // "settings" or "journals"
	From, To string
	Err      error // nil when it moved
}

// journalNames are the three journals.
var journalNames = []string{"errors.yaml", "connections.yaml", "changes.yaml"}

// Migrate moves what an older sshu kept somewhere tdp E3 no longer puts it,
// once, before anything is read: the settings directory macOS kept in
// ~/Library/Application Support, and the journals the settings directory
// held, which tdp E3 counts as data. It moves only into a place that does not
// have them yet; what cannot move stays in use where it is.
func Migrate() []Move {
	var moves []Move
	if m, ok := migrateSettings(); ok {
		moves = append(moves, m)
	}
	if m, ok := migrateJournals(); ok {
		moves = append(moves, m)
	}
	return moves
}

func migrateSettings() (Move, bool) {
	if os.Getenv("SSHU__CONFIG") != "" {
		return Move{}, false
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return Move{}, false
	}
	dir, err := e3Dir()
	old := oldDir(home)
	if err != nil || old == "" || exists(dir) || !exists(old) {
		return Move{}, false
	}
	m := Move{What: "settings", From: old, To: dir}
	if m.Err = os.MkdirAll(filepath.Dir(dir), 0o755); m.Err == nil {
		m.Err = os.Rename(old, dir)
	}
	if m.Err != nil {
		settingsAt = old
	}
	return m, true
}

func migrateJournals() (Move, bool) {
	if os.Getenv("SSHU__DATA") != "" {
		return Move{}, false
	}
	from, err := Dir()
	if err != nil {
		return Move{}, false
	}
	to, err := DataDir()
	if err != nil || from == to {
		return Move{}, false
	}
	var names []string
	for _, n := range journalNames {
		if exists(filepath.Join(from, n)) && !exists(filepath.Join(to, n)) {
			names = append(names, n)
		}
	}
	if len(names) == 0 {
		return Move{}, false
	}
	m := Move{What: "journals", From: from, To: to}
	if m.Err = os.MkdirAll(to, 0o700); m.Err != nil {
		journalsAt = from
		return m, true
	}
	for i, n := range names {
		if m.Err = os.Rename(filepath.Join(from, n), filepath.Join(to, n)); m.Err != nil {
			// The three stay together: put back the ones that went.
			for _, back := range names[:i] {
				_ = os.Rename(filepath.Join(to, back), filepath.Join(from, back))
			}
			journalsAt = from
			break
		}
	}
	return m, true
}
