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
	t.Cleanup(func() { settingsAt, dataAt = "", "" })
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

// The settings directory macOS kept in ~/Library/Application Support moves
// to ~/.config/sshu the first time, and never again; the data in it goes on
// to ~/.sshu (tdp E3).
func TestTheOldMacOSDirectoryMovesOnce(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("the old directory differed only on macOS")
	}
	home := cleanHome(t)
	old := filepath.Join(home, "Library", "Application Support", "sshu")
	write(t, filepath.Join(old, "config.yaml"))
	write(t, filepath.Join(old, "hosts.yaml"))
	write(t, filepath.Join(old, ".sshukey"))
	moves := Migrate()
	dir := filepath.Join(home, ".config", "sshu")
	data := filepath.Join(home, ".sshu")
	if len(moves) != 2 || moves[0].What != "settings" || moves[0].From != old || moves[0].To != dir || moves[0].Err != nil ||
		moves[1].What != "data" || moves[1].From != dir || moves[1].To != data || moves[1].Err != nil {
		t.Fatalf("moves = %+v, want the settings moved from %q to %q, then the data to %q", moves, old, dir, data)
	}
	if !exists(filepath.Join(dir, "config.yaml")) {
		t.Errorf("config.yaml did not arrive in %s", dir)
	}
	for _, f := range []string{"hosts.yaml", ".sshukey"} {
		if !exists(filepath.Join(data, f)) {
			t.Errorf("%s did not arrive in %s", f, data)
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
	write(t, filepath.Join(old, "config.yaml"))
	write(t, filepath.Join(home, ".config")) // a file where the directory goes
	moves := Migrate()
	if len(moves) == 0 || moves[0].What != "settings" || moves[0].Err == nil {
		t.Fatalf("moves = %+v, want the settings move to fail", moves)
	}
	if got, _ := Dir(); got != old {
		t.Errorf("Dir = %q, want the old directory %q", got, old)
	}
	if !exists(filepath.Join(old, "config.yaml")) {
		t.Error("the old directory lost config.yaml")
	}
}

// What the settings directory held that is data — the hosts and credentials
// the user wrote, the key that seals them, the journals — moves to ~/.sshu
// (tdp E3). The settings stay. errors.yaml, no longer read, is the user's
// record all the same: it moves with the rest, never deleted.
func TestTheDataMovesToTheDataDirectory(t *testing.T) {
	home := cleanHome(t)
	t.Setenv(KeyEnv, "")
	cfg := filepath.Join(home, ".config", "sshu")
	names := []string{".sshukey", "hosts.yaml", "credentials.yaml", "errors.yaml", "changes.yaml"}
	for _, n := range append([]string{"config.yaml"}, names...) {
		write(t, filepath.Join(cfg, n))
	}
	moves := Migrate()
	data := filepath.Join(home, ".sshu")
	if len(moves) != 1 || moves[0].What != "data" || moves[0].From != cfg || moves[0].To != data || moves[0].Err != nil ||
		!slices.Equal(moves[0].Moved, names) {
		t.Fatalf("moves = %+v, want %v moved from %q to %q", moves, names, cfg, data)
	}
	for _, f := range names {
		if !exists(filepath.Join(data, f)) || exists(filepath.Join(cfg, f)) {
			t.Errorf("%s did not move", f)
		}
	}
	if !exists(filepath.Join(cfg, "config.yaml")) {
		t.Error("config.yaml left the settings directory")
	}
	for _, f := range []func() (string, error){HostsPath, CredsPath, KeyPath, ChangesPath} {
		if got, _ := f(); filepath.Dir(got) != data {
			t.Errorf("%q is not in %s", got, data)
		}
	}
	if again := Migrate(); len(again) != 0 {
		t.Errorf("a second start moved %+v", again)
	}
}

// Data that cannot move stays in use where it is.
func TestDataThatCannotMoveStaysInUse(t *testing.T) {
	home := cleanHome(t)
	cfg := filepath.Join(home, ".config", "sshu")
	write(t, filepath.Join(cfg, "hosts.yaml"))
	write(t, filepath.Join(cfg, "errors.yaml"))
	write(t, filepath.Join(home, ".sshu")) // a file where the directory goes
	moves := Migrate()
	if len(moves) != 1 || moves[0].What != "data" || moves[0].Err == nil {
		t.Fatalf("moves = %+v, want the data move to fail", moves)
	}
	for _, f := range []func() (string, error){HostsPath, ChangesPath} {
		if got, _ := f(); filepath.Dir(got) != cfg {
			t.Errorf("%q is not where it was, in %s", got, cfg)
		}
	}
}

// SSHU__DATA says where the data goes: what the settings directory held
// moves there.
func TestTheDataMovesToSSHU__DATA(t *testing.T) {
	home := cleanHome(t)
	cfg := filepath.Join(home, ".config", "sshu")
	data := filepath.Join(home, "elsewhere")
	t.Setenv("SSHU__DATA", data)
	write(t, filepath.Join(cfg, "hosts.yaml"))
	if moves := Migrate(); len(moves) != 1 || moves[0].To != data || moves[0].Err != nil {
		t.Fatalf("moves = %+v, want hosts.yaml moved to %q", moves, data)
	}
	if !exists(filepath.Join(data, "hosts.yaml")) {
		t.Error("hosts.yaml is not in SSHU__DATA")
	}
}

// Data that cannot move to SSHU__DATA stays in use where it is, the variable
// notwithstanding: it is not there.
func TestDataThatCannotMoveToSSHU__DATAStaysInUse(t *testing.T) {
	home := cleanHome(t)
	cfg := filepath.Join(home, ".config", "sshu")
	write(t, filepath.Join(cfg, "hosts.yaml"))
	write(t, filepath.Join(home, "file"))
	t.Setenv("SSHU__DATA", filepath.Join(home, "file", "data"))
	if moves := Migrate(); len(moves) != 1 || moves[0].Err == nil {
		t.Fatalf("moves = %+v, want the move to fail", moves)
	}
	if got, _ := HostsPath(); filepath.Dir(got) != cfg {
		t.Errorf("HostsPath = %q, want it where it was, in %s", got, cfg)
	}
}

// The key and the files it seals go to the data directory together: with
// hosts.yaml already there, the key stays beside the credentials it opens.
func TestTheKeyAndWhatItSealsMoveToTheDataTogether(t *testing.T) {
	home := cleanHome(t)
	t.Setenv(KeyEnv, "")
	cfg := filepath.Join(home, ".config", "sshu")
	data := filepath.Join(home, ".sshu")
	write(t, filepath.Join(cfg, ".sshukey"))
	write(t, filepath.Join(cfg, "credentials.yaml"))
	write(t, filepath.Join(cfg, "errors.yaml"))
	write(t, filepath.Join(data, "hosts.yaml"))
	moves := Migrate()
	if len(moves) != 1 || !slices.Equal(moves[0].Moved, []string{"errors.yaml"}) ||
		!slices.Equal(moves[0].Left, []string{".sshukey", "credentials.yaml"}) {
		t.Fatalf("moves = %+v, want the key kept with its credentials and errors.yaml moved", moves)
	}
	if exists(filepath.Join(data, ".sshukey")) {
		t.Error("the key left credentials.yaml behind")
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

// SSHU__STATE moves state.yaml and SSHU__DATA the data; neither moves the
// settings (tdp E2, tdp E3).
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
	t.Setenv(KeyEnv, "")
	for _, f := range []func() (string, error){HostsPath, CredsPath, KeyPath, ConnectionsPath, ChangesPath} {
		if got, _ := f(); filepath.Dir(got) != "/tmp/sshu-data" {
			t.Errorf("%q, want it in SSHU__DATA", got)
		}
	}
	if got, _ := ConfigPath(); filepath.Dir(got) != "/tmp/sshu-config" {
		t.Errorf("config.yaml at %q: neither variable may move the settings", got)
	}
}
