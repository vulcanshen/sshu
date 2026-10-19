package store

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"
)

// Without SSHU__CONFIG and XDG_CONFIG_HOME the settings live in
// ~/.config/sshu on every platform (tdp E3).
func TestTheSettingsDefaultToDotConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("SSHU__CONFIG", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	if got, _ := Dir(); got != filepath.Join(home, ".config", "sshu") {
		t.Fatalf("Dir = %q, want ~/.config/sshu", got)
	}
}

// cleanHome is a home with no sshu in it and none of sshu's variables, and
// forgets whatever an earlier Migrate could not move.
func cleanHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	for _, v := range []string{"SSHU__CONFIG", "SSHU__STATE", "SSHU__DATA", "XDG_CONFIG_HOME"} {
		t.Setenv(v, "")
	}
	t.Cleanup(func() { settingsAt, journalsAt = "", "" })
	return home
}

func write(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

// The settings directory macOS kept in ~/Library/Application Support moves,
// whole, to ~/.config/sshu the first time, and never again (tdp E3).
func TestTheOldMacOSDirectoryMovesOnce(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("the old directory differed only on macOS")
	}
	home := cleanHome(t)
	old := filepath.Join(home, "Library", "Application Support", "sshu")
	write(t, filepath.Join(old, "hosts.yaml"))
	write(t, filepath.Join(old, ".sshukey"))
	moves := Migrate()
	dir := filepath.Join(home, ".config", "sshu")
	if len(moves) == 0 || moves[0].What != "settings" || moves[0].From != old || moves[0].To != dir || moves[0].Err != nil {
		t.Fatalf("moves = %+v, want the settings moved from %q to %q", moves, old, dir)
	}
	for _, f := range []string{"hosts.yaml", ".sshukey"} {
		if !exists(filepath.Join(dir, f)) {
			t.Errorf("%s did not arrive in %s", f, dir)
		}
	}
	if exists(old) {
		t.Errorf("%s is still there", old)
	}
	if got, _ := Dir(); got != dir {
		t.Errorf("Dir = %q, want %q", got, dir)
	}
	if again := Migrate(); len(again) != 0 {
		t.Errorf("a second start moved %+v", again)
	}
}

// An old directory that cannot move stays in use, and the move says why
// (tdp E3).
func TestAnOldDirectoryThatCannotMoveStaysInUse(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("the old directory differed only on macOS")
	}
	home := cleanHome(t)
	old := filepath.Join(home, "Library", "Application Support", "sshu")
	write(t, filepath.Join(old, "hosts.yaml"))
	write(t, filepath.Join(home, ".config")) // a file where the directory goes
	moves := Migrate()
	if len(moves) == 0 || moves[0].What != "settings" || moves[0].Err == nil {
		t.Fatalf("moves = %+v, want the settings move to fail", moves)
	}
	if got, _ := Dir(); got != old {
		t.Errorf("Dir = %q, want the old directory %q", got, old)
	}
	if !exists(filepath.Join(old, "hosts.yaml")) {
		t.Error("the old directory lost hosts.yaml")
	}
}

// The journals the settings directory held move to ~/.sshu: they are data
// (tdp E3). The settings stay.
func TestTheJournalsMoveToTheDataDirectory(t *testing.T) {
	home := cleanHome(t)
	cfg := filepath.Join(home, ".config", "sshu")
	write(t, filepath.Join(cfg, "hosts.yaml"))
	write(t, filepath.Join(cfg, "errors.yaml"))
	write(t, filepath.Join(cfg, "changes.yaml"))
	moves := Migrate()
	data := filepath.Join(home, ".sshu")
	if len(moves) != 1 || moves[0].What != "journals" || moves[0].From != cfg || moves[0].To != data || moves[0].Err != nil {
		t.Fatalf("moves = %+v, want the journals moved from %q to %q", moves, cfg, data)
	}
	for _, f := range []string{"errors.yaml", "changes.yaml"} {
		if !exists(filepath.Join(data, f)) || exists(filepath.Join(cfg, f)) {
			t.Errorf("%s did not move", f)
		}
	}
	if !exists(filepath.Join(cfg, "hosts.yaml")) {
		t.Error("hosts.yaml left the settings directory")
	}
	if got, _ := ErrorsPath(); got != filepath.Join(data, "errors.yaml") {
		t.Errorf("ErrorsPath = %q, want it in %s", got, data)
	}
	if again := Migrate(); len(again) != 0 {
		t.Errorf("a second start moved %+v", again)
	}
}

// Journals that cannot move stay in use where they are.
func TestJournalsThatCannotMoveStayInUse(t *testing.T) {
	home := cleanHome(t)
	cfg := filepath.Join(home, ".config", "sshu")
	write(t, filepath.Join(cfg, "errors.yaml"))
	write(t, filepath.Join(home, ".sshu")) // a file where the directory goes
	moves := Migrate()
	if len(moves) != 1 || moves[0].What != "journals" || moves[0].Err == nil {
		t.Fatalf("moves = %+v, want the journals move to fail", moves)
	}
	if got, _ := ErrorsPath(); got != filepath.Join(cfg, "errors.yaml") {
		t.Errorf("ErrorsPath = %q, want it where it was", got)
	}
}

// The move goes item by item (tdp E3): what the new place already has stays
// in the old one, for the user to merge, and the rest goes; the old
// directory stays while something is left in it.
func TestWhatTheNewPlaceHasStaysInTheOld(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("the old directory differed only on macOS")
	}
	home := cleanHome(t)
	old := filepath.Join(home, "Library", "Application Support", "sshu")
	dir := filepath.Join(home, ".config", "sshu")
	write(t, filepath.Join(old, "config.yaml"))
	write(t, filepath.Join(old, "state.yaml"))
	write(t, filepath.Join(dir, "config.yaml"))
	moves := Migrate()
	if len(moves) == 0 || moves[0].Err != nil || !slices.Equal(moves[0].Moved, []string{"state.yaml"}) ||
		!slices.Equal(moves[0].Left, []string{"config.yaml"}) {
		t.Fatalf("moves = %+v, want state.yaml moved and config.yaml left", moves)
	}
	if !exists(filepath.Join(old, "config.yaml")) || !exists(filepath.Join(dir, "state.yaml")) {
		t.Error("the files are not where the move says")
	}
}

// The key and the files whose passwords it seals go together: with a key
// already at the new place, hosts.yaml stays beside the key that opens it.
func TestTheKeyAndWhatItSealsMoveTogether(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("the old directory differed only on macOS")
	}
	home := cleanHome(t)
	old := filepath.Join(home, "Library", "Application Support", "sshu")
	dir := filepath.Join(home, ".config", "sshu")
	write(t, filepath.Join(old, ".sshukey"))
	write(t, filepath.Join(old, "hosts.yaml"))
	write(t, filepath.Join(old, "config.yaml"))
	write(t, filepath.Join(dir, ".sshukey"))
	moves := Migrate()
	if len(moves) == 0 || !slices.Equal(moves[0].Moved, []string{"config.yaml"}) ||
		!slices.Contains(moves[0].Left, "hosts.yaml") || !slices.Contains(moves[0].Left, ".sshukey") {
		t.Fatalf("moves = %+v, want hosts.yaml kept with its key", moves)
	}
	if exists(filepath.Join(dir, "hosts.yaml")) {
		t.Error("hosts.yaml went to a key that cannot open it")
	}
}

// An old directory with nothing in it is taken away, and nothing is said.
func TestAnEmptyOldDirectoryIsTakenAway(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("the old directory differed only on macOS")
	}
	home := cleanHome(t)
	old := filepath.Join(home, "Library", "Application Support", "sshu")
	if err := os.MkdirAll(old, 0o700); err != nil {
		t.Fatal(err)
	}
	if moves := Migrate(); len(moves) != 0 {
		t.Errorf("moves = %+v, want none", moves)
	}
	if exists(old) {
		t.Error("the empty old directory is still there")
	}
}

// A journal the data directory already has stays in the settings directory;
// the others go.
func TestAJournalTheDataDirectoryHasStays(t *testing.T) {
	home := cleanHome(t)
	cfg := filepath.Join(home, ".config", "sshu")
	data := filepath.Join(home, ".sshu")
	write(t, filepath.Join(cfg, "errors.yaml"))
	write(t, filepath.Join(cfg, "changes.yaml"))
	write(t, filepath.Join(data, "errors.yaml"))
	moves := Migrate()
	if len(moves) != 1 || !slices.Equal(moves[0].Moved, []string{"changes.yaml"}) ||
		!slices.Equal(moves[0].Left, []string{"errors.yaml"}) {
		t.Fatalf("moves = %+v, want changes.yaml moved and errors.yaml left", moves)
	}
}

// SSHU__STATE moves state.yaml and SSHU__DATA the journals; neither moves
// the settings (tdp E2, tdp E3).
func TestStateAndDataFollowTheirVariables(t *testing.T) {
	cleanHome(t)
	t.Setenv("SSHU__CONFIG", "/tmp/sshu-config")
	if got, _ := StatePath(); got != "/tmp/sshu-config/state.yaml" {
		t.Errorf("StatePath = %q, want it in the settings directory", got)
	}
	t.Setenv("SSHU__STATE", "/tmp/sshu-state")
	t.Setenv("SSHU__DATA", "/tmp/sshu-data")
	if got, _ := StatePath(); got != "/tmp/sshu-state/state.yaml" {
		t.Errorf("StatePath = %q, want it in SSHU__STATE", got)
	}
	for _, f := range []func() (string, error){ErrorsPath, ConnectionsPath, ChangesPath} {
		if got, _ := f(); filepath.Dir(got) != "/tmp/sshu-data" {
			t.Errorf("journal at %q, want it in SSHU__DATA", got)
		}
	}
	if got, _ := HostsPath(); filepath.Dir(got) != "/tmp/sshu-config" {
		t.Errorf("hosts.yaml at %q: neither variable may move the settings", got)
	}
}
