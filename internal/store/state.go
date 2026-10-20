package store

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// state.yaml is what the UI keeps between runs that is neither settings nor
// data (tdp E3, state): how the SSH grid is arranged. A value set in a
// one-value-per-row panel takes effect, and is kept, the moment it is set
// (components/layout/panel). sshu owns the file; it is rewritten whole.

// State is the whole of state.yaml.
type State struct {
	// GridArrange is horizontal, vertical or custom; GridColumns the custom
	// grid's columns.
	GridArrange string `yaml:"grid_arrange,omitempty"`
	GridColumns int    `yaml:"grid_columns,omitempty"`
}

const stateHeader = "# sshu state: how the SSH grid is arranged. sshu writes this file.\n"

// StatePath is the full path to state.yaml.
func StatePath() (string, error) {
	dir, err := StateDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "state.yaml"), nil
}

// LoadState reads state.yaml; a missing one is the zero State. One that
// cannot be read is the zero State and why, for startup to ask about (tdp
// E3, when it cannot be read).
func LoadState() (State, error) {
	var st State
	p, err := StatePath()
	if err != nil {
		return st, nil // nowhere to keep it: SaveState says so
	}
	raw, err := os.ReadFile(p)
	if os.IsNotExist(err) {
		return st, nil
	}
	if err != nil {
		return st, err
	}
	if err := yaml.Unmarshal(raw, &st); err != nil {
		return State{}, err
	}
	return st, nil
}

// SaveState writes state.yaml.
func SaveState(st State) error {
	p, err := StatePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	out, err := yaml.Marshal(st)
	if err != nil {
		return err
	}
	return writeFile0600(p, append([]byte(stateHeader), out...))
}
