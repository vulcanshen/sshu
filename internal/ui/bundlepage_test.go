package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/vulcanshen/sshu/internal/store"
)

// openPage steps straight onto an Operation page — cursor choice + content
// focus, the state navigating there would produce. Straight, because the
// section is masked out of the nav for now and j/k cannot reach it.
func openPage(m AppModel, item prefItem) AppModel {
	m.pref.item = item
	m.pref.focus = panelPrefContent
	m.syncPrefSizes()
	return m
}

// The Export page writes a real .sshu: 0600, both YAMLs inside, the extension
// appended when the filename left it off — and the round trip through
// ImportBundle brings the same entries back.
func TestExportPageWritesABundle(t *testing.T) {
	dir := t.TempDir()
	m := appWith(sample(), nil)
	m.creds.creds = []store.Credential{{Name: "ops", User: "root", Auth: store.AuthPassword, Password: "pw"}}

	m = openPage(m, prefExport)
	if !m.textPage() {
		t.Fatal("setup: the export page should hold the keyboard")
	}
	m.exportPage.fields[0].value = dir
	m.exportPage.fields[0].caret = len([]rune(dir))
	m.exportPage.fields[1].value = "backup" // no extension on purpose
	m.exportPage.fields[1].caret = len("backup")
	m = pressA(m, "enter")

	path := filepath.Join(dir, "backup"+store.BundleExt)
	st, err := os.Stat(path)
	if err != nil {
		t.Fatalf("the bundle was not written: %v", err)
	}
	if st.Mode().Perm() != 0o600 {
		t.Errorf("bundle mode %o, want 0600", st.Mode().Perm())
	}
	h, c, err := store.ImportBundle(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(h.Hosts) != len(sample()) || len(c.Credentials) != 1 {
		t.Errorf("bundle holds %d hosts and %d credentials, want %d and 1",
			len(h.Hosts), len(c.Credentials), len(sample()))
	}
	if m.exportPage.done == "" || m.exportPage.err != "" {
		t.Errorf("the page should report success, done=%q err=%q", m.exportPage.done, m.exportPage.err)
	}
	if v := ansi.Strip(m.View()); !strings.Contains(v, "Exported") {
		t.Errorf("the success line should be on the page:\n%s", v)
	}
}

// The Import page merges under the Name-is-key rule: new entries land in the
// lists AND in both save funcs, taken names are skipped, and the summary
// counts both.
func TestImportPageMergesAndDedups(t *testing.T) {
	bundle := filepath.Join(t.TempDir(), "in"+store.BundleExt)
	if err := store.ExportBundle(bundle, store.File{Hosts: []store.Host{
		{Name: "prod-web-01", Host: "1.2.3.4", Port: 22, User: "evil", Auth: store.AuthPassword, Password: "x"},
		{Name: "brand-new", Host: "10.9.9.9", Port: 22, User: "root", Auth: store.AuthPassword, Password: "x"},
	}}, store.CredsFile{Credentials: []store.Credential{
		{Name: "fresh", User: "root", Auth: store.AuthPassword, Password: "x"},
	}}); err != nil {
		t.Fatal(err)
	}

	var savedHosts []store.Host
	var savedCreds []store.Credential
	m := appWith(sample(), &savedHosts)
	m.saveCreds = func(l []store.Credential) error { savedCreds = l; return nil }

	m = openPage(m, prefImport)
	m.importPage.fields[0].value = bundle
	m.importPage.fields[0].caret = len([]rune(bundle))
	m = pressA(m, "enter")

	if got := len(m.hosts.hosts); got != len(sample())+1 {
		t.Fatalf("host list has %d entries, want %d", got, len(sample())+1)
	}
	for _, h := range m.hosts.hosts {
		if h.Name == "prod-web-01" && h.User == "evil" {
			t.Error("the existing prod-web-01 must win over the imported one")
		}
	}
	if len(savedHosts) != len(sample())+1 || len(savedCreds) != 1 {
		t.Errorf("both saves must run: %d hosts, %d creds", len(savedHosts), len(savedCreds))
	}
	if m.hosts.creds[0].Name != "fresh" {
		t.Error("the hosts table's credential mirror must follow the import")
	}
	if !strings.Contains(m.importPage.done, "1 host · 1 credential") ||
		!strings.Contains(m.importPage.done, "1 skipped") {
		t.Errorf("the summary must count added and skipped, got %q", m.importPage.done)
	}
}

// A page is a §4.5 text surface: the global keys type instead of firing — q
// does not quit, V does not open the splash, ? does not open help, a digit
// goes into the field — and Esc is the one way back to the nav.
func TestOperationPageTypesInsteadOfActing(t *testing.T) {
	m := openPage(appWith(sample(), nil), prefExport)
	m.exportPage.fields[1].value = ""
	m.exportPage.fields[1].caret = 0
	m.exportPage.focus = 1

	m = pressA(m, "q", "V", "?", "1")
	if m.splash.isActive() || m.help.isActive() {
		t.Error("V and ? must type into the field, not open floats")
	}
	if got := m.exportPage.fields[1].value; got != "qV?1" {
		t.Errorf("the keys should have been typed, field holds %q", got)
	}

	m = pressA(m, "esc")
	if m.pref.focus != panelPrefNav {
		t.Error("Esc should hand the keyboard back to the nav")
	}
	if m2 := pressA(m, "V"); !m2.splash.isActive() {
		t.Error("off the page, V is the easter egg again")
	}
}

// Submitting with a bad target parks the error on the page (§6.7) — and
// editing any field clears the stale verdict.
func TestExportPageFailsOnThePage(t *testing.T) {
	m := openPage(appWith(sample(), nil), prefExport)
	m.exportPage.fields[0].value = "/nonexistent-sshu-dir/nope"
	m.exportPage.fields[0].caret = len("/nonexistent-sshu-dir/nope")
	m = pressA(m, "enter")
	if m.exportPage.err == "" {
		t.Fatal("a missing directory must fail on the page")
	}
	m = pressA(m, "x")
	if m.exportPage.err != "" {
		t.Error("typing should clear the stale error")
	}
}
