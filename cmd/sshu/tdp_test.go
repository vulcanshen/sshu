package main

import (
	"os"
	"path/filepath"
	"testing"

	terminu "github.com/vulcanshen/terminu-go"
	"github.com/vulcanshen/terminu-go/tdptest"

	"github.com/vulcanshen/sshu/internal/tui"
)

// newApp is sshu as main makes it, all but Run: its files where HOME and the
// SSHU__ variables say.
func newApp() *terminu.App { return tui.New(data()).App() }

// TestTDP launches sshu as for the first time on the sizes and icon widths
// tdptest walks, every screen's Space menu, global operations, App Log and
// key reference opened and closed (tdp L4, K4, L1).
func TestTDP(t *testing.T) {
	tdptest.Check(t, tdptest.App{Name: "sshu", New: newApp})
}

// sandbox is a home of its own for one launch, none of sshu's variables set.
func sandbox(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	for _, v := range []string{"XDG_CONFIG_HOME", "SSHU__CONFIG", "SSHU__STATE", "SSHU__DATA", "SSHU__KEY_FILE"} {
		t.Setenv(v, "")
	}
	return home
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

// A hosts.yaml sshu cannot read stops it at the start (tdp E3, when it cannot
// be read): Enter puts it aside as hosts.yaml.bak, never over one already
// there, and sshu goes on with no hosts; the next file asked about comes
// after it.
func TestAnUnreadableHostsFileIsMovedAside(t *testing.T) {
	home := sandbox(t)
	data := filepath.Join(home, ".sshu")
	writeFile(t, filepath.Join(data, "hosts.yaml"), "hosts: [\n")
	writeFile(t, filepath.Join(data, "hosts.yaml.bak"), "an older one\n")
	writeFile(t, filepath.Join(data, "changes.yaml"), "- {\n")
	d := tdptest.Start(t, newApp(), 80, 40)
	if !d.Has("MOVE ASIDE") || !d.Has("~/.sshu/hosts.yaml can't be read") || !d.Has("Move it to hosts.yaml.bak.1") {
		t.Fatalf("no MOVE ASIDE for hosts.yaml:\n%s", screen(d))
	}
	d.Press("Enter")
	if _, err := os.Stat(filepath.Join(data, "hosts.yaml.bak.1")); err != nil {
		t.Fatal("hosts.yaml was not put aside as hosts.yaml.bak.1")
	}
	if b, _ := os.ReadFile(filepath.Join(data, "hosts.yaml.bak")); string(b) != "an older one\n" {
		t.Errorf("hosts.yaml.bak was written over: %q", b)
	}
	if !d.Has("changes.yaml can't be read") {
		t.Fatalf("no MOVE ASIDE for changes.yaml, the next one:\n%s", screen(d))
	}
	d.Press("Enter")
	if d.Has("MOVE ASIDE") || !d.Has("No hosts yet") {
		t.Fatalf("not on to an empty hosts list:\n%s", screen(d))
	}
	if d.Quit() {
		t.Fatal("sshu left")
	}
}

// Esc on the question leaves sshu and touches nothing.
func TestEscOnMoveAsideLeaves(t *testing.T) {
	home := sandbox(t)
	hosts := filepath.Join(home, ".sshu", "hosts.yaml")
	writeFile(t, hosts, "hosts: [\n")
	d := tdptest.Start(t, newApp(), 80, 40)
	d.Press("Esc")
	if !d.Quit() {
		t.Fatalf("Esc did not leave:\n%s", screen(d))
	}
	if b, _ := os.ReadFile(hosts); string(b) != "hosts: [\n" {
		t.Errorf("hosts.yaml changed: %q", b)
	}
}

// A key file that is there and is not a key is asked about, not hosts.yaml:
// moved aside, a new key takes its place, and the passwords it sealed stay
// sealed, for the old key to open again.
func TestAnUnreadableKeyIsMovedAsideAndANewOneMade(t *testing.T) {
	home := sandbox(t)
	key := filepath.Join(home, ".sshu", ".sshukey")
	writeFile(t, key, "not a key\n")
	d := tdptest.Start(t, newApp(), 80, 40)
	if !d.Has("~/.sshu/.sshukey can't be read") {
		t.Fatalf("no MOVE ASIDE for the key:\n%s", screen(d))
	}
	d.Press("Enter")
	if b, err := os.ReadFile(key); err != nil || string(b) == "not a key\n" {
		t.Errorf("no new key: %q, %v", b, err)
	}
	if b, _ := os.ReadFile(key + ".bak"); string(b) != "not a key\n" {
		t.Errorf("the old key is not in .sshukey.bak: %q", b)
	}
}

func screen(d *tdptest.Driver) string {
	s := ""
	for _, l := range d.Screen() {
		s += l + "\n"
	}
	return s
}
