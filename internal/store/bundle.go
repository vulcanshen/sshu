package store

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// The .sshu bundle: hosts.yaml and credentials.yaml zipped into one file, so
// a setup travels between machines as a single artifact. Export and import
// both live here because the zip layout is one decision, and two files owning
// half of it each would let them drift.

// BundleExt is the bundle's extension. A dedicated one, so a .sshu in a
// directory listing can only be one thing.
const BundleExt = ".sshu"

// bundleEntryCap bounds one entry's decompressed size. The real files are a
// few KiB; the cap only exists so a hostile zip cannot balloon in memory.
const bundleEntryCap = 8 << 20

// ExportBundle writes hosts + creds as a .sshu zip at path. The entries carry
// the same warning headers the standalone files do, and the bundle itself is
// written at 0600 for the same reason they are: a password host travels with
// its password in the clear.
//
// The target directory must already exist — inventing a mistyped one would
// hide the typo — and an existing file is refused rather than overwritten:
// the page this is driven from has no confirm step, so the check IS the
// safety.
func ExportBundle(path string, hosts File, creds CredsFile) error {
	if err := hosts.Validate(); err != nil {
		return err
	}
	if err := creds.Validate(); err != nil {
		return err
	}
	hosts.Version, creds.Version = currentVersion, currentVersion

	hb, err := yaml.Marshal(hosts)
	if err != nil {
		return err
	}
	cb, err := yaml.Marshal(creds)
	if err != nil {
		return err
	}

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, e := range []struct {
		name string
		body []byte
	}{
		{"hosts.yaml", append([]byte(header), hb...)},
		{"credentials.yaml", append([]byte(credsHeader), cb...)},
	} {
		w, err := zw.Create(e.name)
		if err != nil {
			return err
		}
		if _, err := w.Write(e.body); err != nil {
			return err
		}
	}
	if err := zw.Close(); err != nil {
		return err
	}

	dir := filepath.Dir(path)
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		return fmt.Errorf("directory %s does not exist", FoldHome(dir))
	}
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("%s already exists — pick another name", FoldHome(path))
	}
	return writeFile0600(path, buf.Bytes())
}

// ImportBundle reads a .sshu zip back into the two documents. Either entry may
// be absent — a bundle of only hosts is fine — but a zip with neither is some
// other file that happens to end in .sshu, and saying so beats importing
// nothing silently.
func ImportBundle(path string) (File, CredsFile, error) {
	hosts := File{Version: currentVersion}
	creds := CredsFile{Version: currentVersion}

	zr, err := zip.OpenReader(path)
	if err != nil {
		return hosts, creds, fmt.Errorf("%s is not a readable %s bundle", FoldHome(path), BundleExt)
	}
	defer zr.Close()

	found := false
	for _, f := range zr.File {
		switch f.Name {
		case "hosts.yaml":
			raw, err := readZipEntry(f)
			if err != nil {
				return hosts, creds, err
			}
			if err := yaml.Unmarshal(raw, &hosts); err != nil {
				return hosts, creds, fmt.Errorf("hosts.yaml in %s: %w", FoldHome(path), err)
			}
			found = true
		case "credentials.yaml":
			raw, err := readZipEntry(f)
			if err != nil {
				return hosts, creds, err
			}
			if err := yaml.Unmarshal(raw, &creds); err != nil {
				return hosts, creds, fmt.Errorf("credentials.yaml in %s: %w", FoldHome(path), err)
			}
			found = true
		}
	}
	if !found {
		return hosts, creds, fmt.Errorf("%s holds neither hosts.yaml nor credentials.yaml", FoldHome(path))
	}
	// The same leniency LoadFrom extends to a hand-edited file: a missing port
	// is the default, not a rejection.
	for i := range hosts.Hosts {
		if hosts.Hosts[i].Port == 0 {
			hosts.Hosts[i].Port = DefaultPort
		}
	}
	return hosts, creds, nil
}

func readZipEntry(f *zip.File) ([]byte, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(io.LimitReader(rc, bundleEntryCap))
}

// MergeHosts appends the incoming hosts whose names are free. Name is the key
// of hosts.yaml (File.Validate enforces it), so an incoming entry under a
// taken name is SKIPPED, never merged field-by-field: the copy already on
// this machine is the one the user can see and has been trusting. Invalid
// incoming entries are skipped for the same reason Save refuses them.
func MergeHosts(cur, in []Host) (out []Host, added, skipped int) {
	out = append(out, cur...)
	seen := make(map[string]bool, len(cur))
	for _, h := range cur {
		seen[h.Name] = true
	}
	for _, h := range in {
		if h.Port == 0 {
			h.Port = DefaultPort
		}
		if h.Validate() != nil || seen[h.Name] {
			skipped++
			continue
		}
		seen[h.Name] = true
		out = append(out, h)
		added++
	}
	return out, added, skipped
}

// MergeCreds is MergeHosts over credentials.yaml, under the same rule.
func MergeCreds(cur, in []Credential) (out []Credential, added, skipped int) {
	out = append(out, cur...)
	seen := make(map[string]bool, len(cur))
	for _, c := range cur {
		seen[c.Name] = true
	}
	for _, c := range in {
		if c.Validate() != nil || seen[c.Name] {
			skipped++
			continue
		}
		seen[c.Name] = true
		out = append(out, c)
		added++
	}
	return out, added, skipped
}
