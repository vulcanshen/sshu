package store

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func bundleHosts() []Host {
	return []Host{
		{Name: "web", Host: "10.0.0.1", Port: 22, User: "root", Auth: AuthPassword, Password: "pw"},
		{Name: "db", Host: "10.0.0.2", Port: 2200, User: "ops", Auth: AuthCredential, Credential: "ops"},
	}
}

func bundleCreds() []Credential {
	return []Credential{{Name: "ops", User: "ops", Auth: AuthPrivateKey, IdentityFile: "~/.ssh/id"}}
}

// A bundle round-trips: what Export wrote, Import reads back — entries, ports
// and secrets intact — and the file lands at 0600 like the YAML it carries.
func TestBundleRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x"+BundleExt)
	if err := ExportBundle(path, File{Hosts: bundleHosts()}, CredsFile{Credentials: bundleCreds()}); err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm() != 0o600 {
		t.Errorf("bundle mode %o, want 0600 — it carries plaintext passwords", st.Mode().Perm())
	}
	h, c, err := ImportBundle(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(h.Hosts) != 2 || h.Hosts[0].Name != "web" || h.Hosts[0].Password != "pw" || h.Hosts[1].Port != 2200 {
		t.Errorf("hosts did not round-trip: %+v", h.Hosts)
	}
	if len(c.Credentials) != 1 || c.Credentials[0].IdentityFile != "~/.ssh/id" {
		t.Errorf("credentials did not round-trip: %+v", c.Credentials)
	}
}

// Export refuses to invent a directory or overwrite a file: the page driving
// it has no confirm step, so these checks are the safety.
func TestExportRefusesBadTargets(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "nope", "x"+BundleExt)
	if err := ExportBundle(missing, File{}, CredsFile{}); err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Errorf("a missing directory must be an error, got %v", err)
	}
	path := filepath.Join(dir, "x"+BundleExt)
	if err := ExportBundle(path, File{}, CredsFile{}); err != nil {
		t.Fatal(err)
	}
	if err := ExportBundle(path, File{}, CredsFile{}); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Errorf("an existing bundle must be refused, got %v", err)
	}
}

// A file that is not a zip, a missing file, and a zip holding neither YAML are
// all named for what they are instead of importing nothing silently.
func TestImportRejectsNonBundles(t *testing.T) {
	dir := t.TempDir()

	notZip := filepath.Join(dir, "junk"+BundleExt)
	if err := os.WriteFile(notZip, []byte("not a zip"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ImportBundle(notZip); err == nil {
		t.Error("a non-zip must fail to import")
	}

	if _, _, err := ImportBundle(filepath.Join(dir, "absent"+BundleExt)); err == nil {
		t.Error("a missing file must fail to import")
	}

	empty := filepath.Join(dir, "empty"+BundleExt)
	f, err := os.Create(empty)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	w, _ := zw.Create("readme.txt")
	if _, err := w.Write([]byte("hi")); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	f.Close()
	if _, _, err := ImportBundle(empty); err == nil || !strings.Contains(err.Error(), "neither") {
		t.Errorf("a zip without the YAMLs must say so, got %v", err)
	}
}

// Merge keeps what this machine already has: a taken name is skipped whole,
// never merged field-by-field; an invalid entry and an in-bundle duplicate
// are skipped too, and a missing port gets the default before validation.
func TestMergeDedupsByName(t *testing.T) {
	out, added, skipped := MergeHosts(bundleHosts(), []Host{
		{Name: "web", Host: "1.2.3.4", Port: 22, User: "evil", Auth: AuthPassword, Password: "x"},  // taken
		{Name: "new", Host: "10.0.0.9", User: "root", Auth: AuthPassword, Password: "x"},           // port 0 → default
		{Name: "", Host: "h", Port: 22, User: "u", Auth: AuthPassword},                             // invalid
		{Name: "new", Host: "10.0.0.9", Port: 22, User: "root", Auth: AuthPassword, Password: "x"}, // dup in bundle
	})
	if added != 1 || skipped != 3 {
		t.Fatalf("added %d skipped %d, want 1 and 3", added, skipped)
	}
	if len(out) != 3 || out[2].Name != "new" || out[2].Port != DefaultPort {
		t.Errorf("merged list wrong: %+v", out)
	}
	if out[0].User != "root" {
		t.Errorf("the existing entry must win, got %+v", out[0])
	}

	outC, addedC, skippedC := MergeCreds(bundleCreds(), []Credential{
		{Name: "ops", User: "evil", Auth: AuthPassword, Password: "x"},
		{Name: "fresh", User: "root", Auth: AuthPassword, Password: "x"},
	})
	if addedC != 1 || skippedC != 1 || len(outC) != 2 || outC[1].Name != "fresh" {
		t.Errorf("cred merge wrong: %+v (added %d skipped %d)", outC, addedC, skippedC)
	}
	if outC[0].User != "ops" {
		t.Errorf("the existing credential must win: %+v", outC[0])
	}
}
