package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"slices"
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

// Enter on the question starts sshu as on its first start (tdp E3-18): for
// each of sshu's own files, put aside, what is on disk is what a first start
// leaves — the key made again, the files a first start does not write still
// not there — and Manage shows nothing of what the file held.
func TestMovingAsideStartsAsOnTheFirstStart(t *testing.T) {
	first := func() []string {
		home := sandbox(t)
		d := tdptest.Start(t, newApp(), 80, 40)
		if d.Has("MOVE ASIDE") {
			t.Fatalf("a first start asked:\n%s", screen(d))
		}
		return filesIn(t, home)
	}()
	for _, c := range []struct{ at, body string }{
		{".config/sshu/config.yaml", "theme: [\n"},
		{".config/sshu/state.yaml", "grid_columns: [\n"},
		{".sshu/.sshukey", "not a key\n"},
		{".sshu/hosts.yaml", "hosts: [\n"},
		{".sshu/credentials.yaml", "credentials: [\n"},
		{".sshu/connections.yaml", "- {\n"},
		{".sshu/changes.yaml", "- {\n"},
	} {
		home := sandbox(t)
		writeFile(t, filepath.Join(home, c.at), c.body)
		d := tdptest.Start(t, newApp(), 80, 40)
		if !d.Has(filepath.Base(c.at) + " can't be read") {
			t.Fatalf("%s: no MOVE ASIDE:\n%s", c.at, screen(d))
		}
		d.Press("Enter")
		if d.Has("MOVE ASIDE") || d.Quit() {
			t.Fatalf("%s: not started after Enter:\n%s", c.at, screen(d))
		}
		var got []string
		for _, f := range filesIn(t, home) {
			if f != c.at+".bak" {
				got = append(got, f)
			}
		}
		if !slices.Equal(got, first) {
			t.Errorf("%s put aside: the files are %v, a first start leaves %v", c.at, got, first)
		}
		if !d.Has("No hosts yet") {
			t.Errorf("%s put aside: Manage is not as on a first start:\n%s", c.at, screen(d))
		}
	}
}

// A file an older sshu wrote and this one does not read — errors.yaml,
// applogs.yaml — stays, with no toast; the App Log names it at every start
// (tdp E3-23). One that is not there is not named.
func TestFilesNoLongerUsedAreNamedInTheAppLog(t *testing.T) {
	home := sandbox(t)
	errs := filepath.Join(home, ".sshu", "errors.yaml")
	applogs := filepath.Join(home, ".config", "sshu", "applogs.yaml")
	writeFile(t, errs, "- {at: 2026-10-01T10:00:00Z, level: error, error: old}\n")
	writeFile(t, applogs, "[]\n")
	for range 2 { // and again at the next start: the App Log starts empty
		d := tdptest.Start(t, newApp(), 120, 40)
		if d.Has("No longer used") {
			t.Fatalf("a toast for a file no longer used:\n%s", screen(d))
		}
		d.Press("!")
		for _, want := range []string{
			"INFO No longer used: ~/.config/sshu/applogs.yaml; delete it if you don't need it",
			"INFO No longer used: ~/.sshu/errors.yaml; delete it if you don't need it",
		} {
			if !d.Has(want) {
				t.Errorf("no %q in the App Log:\n%s", want, screen(d))
			}
		}
		if d.Has("~/.config/sshu/errors.yaml") {
			t.Errorf("a file that is not there is named:\n%s", screen(d))
		}
		for _, p := range []string{errs, applogs} {
			if _, err := os.Stat(p); err != nil {
				t.Errorf("%s is gone: %v", p, err)
			}
		}
	}
}

// filesIn is every file under home, as paths from it, sorted.
func filesIn(t *testing.T, home string) []string {
	t.Helper()
	var out []string
	err := filepath.WalkDir(home, func(p string, e fs.DirEntry, err error) error {
		if err == nil && !e.IsDir() {
			rel, _ := filepath.Rel(home, p)
			out = append(out, rel)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(out)
	return out
}

func screen(d *tdptest.Driver) string {
	s := ""
	for _, l := range d.Screen() {
		s += l + "\n"
	}
	return s
}
