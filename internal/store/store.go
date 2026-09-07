// Package store owns sshu's on-disk config: where it lives and how it is read
// and written. The UI layer never touches the filesystem directly.
package store

import (
	"os"
	"path/filepath"
	"strings"
)

// Dir resolves the directory holding hosts.yaml (and later config.yaml /
// state.yaml).
//
// SSHU_CONFIG overrides everything — it names the directory outright, for demo
// recordings and isolated tests. Otherwise XDG_CONFIG_HOME wins on every
// platform when set, so a macOS user can opt into ~/.config/sshu instead of
// being stuck with ~/Library/Application Support; without it os.UserConfigDir
// decides. Go already honours XDG_CONFIG_HOME on Linux, so this only changes
// macOS behaviour.
func Dir() (string, error) {
	if p := os.Getenv("SSHU_CONFIG"); p != "" {
		return p, nil
	}
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" {
		return filepath.Join(x, "sshu"), nil
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "sshu"), nil
}

// HostsPath is the full path to hosts.yaml.
func HostsPath() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "hosts.yaml"), nil
}

// FoldHome is the inverse of ExpandTilde: it writes a path back in ~ form so
// hosts.yaml stays portable between machines and readable to a human.
func FoldHome(p string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return p
	}
	if p == home {
		return "~"
	}
	if strings.HasPrefix(p, home+string(filepath.Separator)) {
		return "~" + strings.TrimPrefix(p, home)
	}
	return p
}

// dedupeByName drops every entry repeating a name an earlier entry already
// took, and reports the dropped entries' names in the order they were dropped.
// Both loaders run it, because both files are keyed by a name and both are
// hand-editable.
//
// The FIRST one wins because that is what the rest of sshu already believed:
// Index, CredsFile.Index and the UI's indexOfHost all stop at the first match,
// so the entry every lookup resolved to was always the first. What a repeated
// name used to produce was a list that behaved as if it had not: the second row
// was on screen but unreachable by name, deleting it removed BOTH rows in one
// keystroke (the delete filters by name), and the next save was refused
// outright by Validate — so the list you were looking at could not be written
// back at all. Dropping the repeat at the door makes the list on screen the
// list every other operation already assumed it had.
//
// It does NOT rewrite the file. The duplicate stays on disk until the user
// saves something, which is the only moment they have asked for the file to
// change.
func dedupeByName[T any](items []T, name func(T) string) ([]T, []string) {
	seen := make(map[string]bool, len(items))
	kept := make([]T, 0, len(items))
	var dropped []string
	for _, it := range items {
		n := name(it)
		if seen[n] {
			dropped = append(dropped, n)
			continue
		}
		seen[n] = true
		kept = append(kept, it)
	}
	if dropped == nil {
		// Nothing repeated: hand back the original slice rather than the copy, so
		// the ordinary path allocates nothing and behaves exactly as before.
		return items, nil
	}
	return kept, dropped
}

// ExpandTilde turns a leading ~ into the user's home directory. Paths in
// hosts.yaml are hand-editable, so ~/.ssh/id_ed25519 has to work.
func ExpandTilde(p string) string {
	if p != "~" && !strings.HasPrefix(p, "~/") {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return p
	}
	return filepath.Join(home, strings.TrimPrefix(p, "~"))
}
