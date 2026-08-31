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
