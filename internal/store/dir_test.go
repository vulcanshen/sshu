package store

import (
	"os"
	"path/filepath"
	"runtime"
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

// A macOS user whose settings are still where os.UserConfigDir put them keeps
// them until ~/.config/sshu exists (TODO(tdp SS1)); once it does, it wins.
func TestTheOldMacOSDirectoryIsReadUntilTheNewOneExists(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("the old directory differed only on macOS")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("SSHU__CONFIG", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	old := filepath.Join(home, "Library", "Application Support", "sshu")
	if err := os.MkdirAll(old, 0o700); err != nil {
		t.Fatal(err)
	}
	if got, _ := Dir(); got != old {
		t.Fatalf("Dir = %q, want the old directory %q", got, old)
	}
	dir := filepath.Join(home, ".config", "sshu")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if got, _ := Dir(); got != dir {
		t.Fatalf("Dir = %q, want %q once it exists", got, dir)
	}
}

// The journals follow SSHU__STATE, and the settings directory without it
// (tdp E2, E3).
func TestTheJournalsFollowTheStateDirectory(t *testing.T) {
	t.Setenv("SSHU__CONFIG", "/tmp/sshu-config")
	t.Setenv("SSHU__STATE", "")
	if got, _ := ErrorsPath(); got != "/tmp/sshu-config/errors.yaml" {
		t.Fatalf("ErrorsPath = %q, want it in the settings directory", got)
	}
	t.Setenv("SSHU__STATE", "/tmp/sshu-state")
	for _, f := range []func() (string, error){ErrorsPath, ConnectionsPath, ChangesPath} {
		if got, _ := f(); filepath.Dir(got) != "/tmp/sshu-state" {
			t.Errorf("journal at %q, want it in SSHU__STATE", got)
		}
	}
	if got, _ := HostsPath(); filepath.Dir(got) != "/tmp/sshu-config" {
		t.Errorf("hosts.yaml at %q: SSHU__STATE must not move the settings", got)
	}
}
