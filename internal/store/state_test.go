package store

import (
	"os"
	"path/filepath"
	"testing"
)

// The grid's arrangement is kept between runs, in the state directory.
func TestTheStateIsKeptBetweenRuns(t *testing.T) {
	t.Setenv("SSHU__CONFIG", t.TempDir())
	t.Setenv("SSHU__STATE", t.TempDir())
	if st, err := LoadState(); st != (State{}) || err != nil {
		t.Fatalf("no file should be the zero state, got %+v, %v", st, err)
	}
	if err := SaveState(State{GridArrange: "custom", GridColumns: 3}); err != nil {
		t.Fatal(err)
	}
	if st, err := LoadState(); st.GridArrange != "custom" || st.GridColumns != 3 || err != nil {
		t.Fatalf("read back %+v, %v", st, err)
	}
}

// A state.yaml that cannot be read says why, for startup to ask about moving
// it aside (tdp E3), and is the zero State meanwhile.
func TestAStateThatCannotBeReadSaysWhy(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SSHU__STATE", dir)
	if err := os.WriteFile(filepath.Join(dir, "state.yaml"), []byte("grid_columns: [\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if st, err := LoadState(); err == nil || st != (State{}) {
		t.Fatalf("LoadState = %+v, %v; want the zero state and why", st, err)
	}
}
