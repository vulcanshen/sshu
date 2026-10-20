package store

import (
	"os"
	"path/filepath"
	"slices"
)

// Where an older sshu kept something, when Migrate could not move it: Dir
// and DataDir go on using it (tdp E3, "when the place changes").
var settingsAt, dataAt string

// Move is one place Migrate moved things out of, or could not.
type Move struct {
	What     string // "settings" or "data"
	From, To string
	// Moved are the names that went to To.
	Moved []string
	// Left are the names To already had: they stay in From, for the user to
	// merge, and nothing is lost.
	Left []string
	// Err is why nothing moved: sshu goes on using From.
	Err error
}

// sealed are the key and the files whose passwords it seals: they move
// together or not at all, or a password would end up beside a key that
// cannot open it.
var sealed = []string{".sshukey", "hosts.yaml", "credentials.yaml"}

// dataNames are what the settings directory held that tdp E3 counts as data:
// the lists the user wrote, with their key, and the three journals.
var dataNames = append(slices.Clone(sealed), "errors.yaml", "connections.yaml", "changes.yaml")

// Migrate moves what an older sshu kept somewhere tdp E3 no longer puts it,
// once, before anything is read: the settings directory macOS kept in
// ~/Library/Application Support, and the data the settings directory held.
// It moves item by item (tdp E3): what the new place does not have goes
// there, what it already has stays where it was for the user to merge, and
// an old directory left empty is taken away.
func Migrate() []Move {
	var moves []Move
	if m, ok := migrateSettings(); ok {
		moves = append(moves, m)
	}
	if m, ok := migrateData(); ok {
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
	if err != nil || old == "" || !exists(old) {
		return Move{}, false
	}
	entries, err := os.ReadDir(old)
	if err != nil {
		return Move{What: "settings", From: old, To: dir, Err: err}, true
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	m := moveItems("settings", old, dir, 0o700, names, true)
	if m.Err != nil {
		settingsAt = old
	}
	if m.Err == nil && len(m.Moved) == 0 && len(m.Left) == 0 {
		return Move{}, false // an empty old directory, taken away
	}
	return m, true
}

// migrateData moves the data out of the settings directory into DataDir,
// SSHU__DATA's included: the variable says where the data goes, not that it
// is already there.
func migrateData() (Move, bool) {
	from, err := Dir()
	if err != nil {
		return Move{}, false
	}
	to, err := DataDir()
	if err != nil || from == to {
		return Move{}, false
	}
	var names []string
	for _, n := range dataNames {
		if exists(filepath.Join(from, n)) {
			names = append(names, n)
		}
	}
	if len(names) == 0 {
		return Move{}, false
	}
	m := moveItems("data", from, to, 0o700, names, false)
	if m.Err != nil {
		dataAt = from
	}
	return m, true
}

// moveItems moves names from from to to, one by one. What to already has is
// left. The first that cannot move puts back the ones that went, and the
// move reports why. With dropEmpty, from is taken away once nothing is left
// in it.
func moveItems(what, from, to string, mode os.FileMode, names []string, dropEmpty bool) Move {
	m := Move{What: what, From: from, To: to}
	// The sealed ones go together: one already at the new place keeps all
	// of them at the old one.
	keepSealed := slices.ContainsFunc(sealed, func(n string) bool { return exists(filepath.Join(to, n)) })
	var moving []string
	for _, n := range names {
		switch {
		case exists(filepath.Join(to, n)), keepSealed && slices.Contains(sealed, n):
			m.Left = append(m.Left, n)
		default:
			moving = append(moving, n)
		}
	}
	if len(moving) > 0 {
		if m.Err = os.MkdirAll(to, mode); m.Err != nil {
			m.Left = nil
			return m
		}
	}
	for i, n := range moving {
		if m.Err = os.Rename(filepath.Join(from, n), filepath.Join(to, n)); m.Err != nil {
			for _, back := range moving[:i] {
				_ = os.Rename(filepath.Join(to, back), filepath.Join(from, back))
			}
			m.Moved, m.Left = nil, nil
			return m
		}
		m.Moved = append(m.Moved, n)
	}
	if dropEmpty && len(m.Left) == 0 {
		_ = os.Remove(from) // only an empty directory goes
	}
	return m
}
