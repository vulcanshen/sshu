package store

import "testing"

// The grid's arrangement is kept between runs, in the state directory.
func TestTheStateIsKeptBetweenRuns(t *testing.T) {
	t.Setenv("SSHU__CONFIG", t.TempDir())
	t.Setenv("SSHU__STATE", t.TempDir())
	if st := LoadState(); st != (State{}) {
		t.Fatalf("no file should be the zero state, got %+v", st)
	}
	if err := SaveState(State{GridArrange: "custom", GridColumns: 3}); err != nil {
		t.Fatal(err)
	}
	if st := LoadState(); st.GridArrange != "custom" || st.GridColumns != 3 {
		t.Fatalf("read back %+v", st)
	}
}
