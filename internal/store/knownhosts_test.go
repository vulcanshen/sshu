package store

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func b64(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }

// knownFixture is what makes this parser worth having: a section comment over a
// RUN of entries (not over one), a bracketed non-default port, several names on
// one line, a marker, a hashed name that cannot be read back, and a trailing
// comment field.
func knownFixture() string {
	return fmt.Sprintf(`# work
[198.51.100.10]:2222 ssh-ed25519 %s laptop
gw.example.com,198.51.100.11 ssh-rsa %s

# elsewhere
@revoked old.example.com ssh-ed25519 %s
|1|%s|%s ssh-ed25519 %s
@cert-authority   *.example.com   ssh-ed25519   %s   the CA
this-line-is-not-an-entry
`, b64("key-one"), b64("key-two"), b64("key-three"), b64("salt"), b64("hash"),
		b64("key-four"), b64("key-five"))
}

func TestAnUntouchedKnownHostsRendersBackByteForByte(t *testing.T) {
	raw := knownFixture()
	if got := parseKnownHostsFile(raw).Text(); got != raw {
		t.Fatalf("Text() must be the file it was given:\n%q", got)
	}
}

func TestParsingKeepsPortsMarkersCommentsAndHashedNames(t *testing.T) {
	f := parseKnownHostsFile(knownFixture())
	// FIVE, not six: `this-line-is-not-an-entry` has no key type and no key, so
	// it is a line sshu does not understand — and a line it does not understand
	// is one it leaves alone rather than turning into a row you can edit.
	if len(f.Entries) != 5 {
		t.Fatalf("5 entries, got %d: %+v", len(f.Entries), f.Entries)
	}
	if f.Entries[0].Hosts != "[198.51.100.10]:2222" || f.Entries[0].Comment != "laptop" {
		t.Errorf("bracketed port / trailing comment lost: %+v", f.Entries[0])
	}
	if f.Entries[1].Hosts != "gw.example.com,198.51.100.11" {
		t.Errorf("several names on one line must stay one field: %q", f.Entries[1].Hosts)
	}
	if f.Entries[2].Marker != "@revoked" || f.Entries[2].Hosts != "old.example.com" {
		t.Errorf("the marker belongs to the entry, not to the name: %+v", f.Entries[2])
	}
	if !f.Entries[3].Hashed() {
		t.Errorf("a |1| name is a hash, not a name: %q", f.Entries[3].Hosts)
	}
	for i, e := range f.Entries[:3] {
		if e.Hashed() {
			t.Errorf("entry %d is a plain name and must not read as hashed", i)
		}
	}
	if f.Entries[4].Marker != "@cert-authority" || f.Entries[4].Hosts != "*.example.com" ||
		f.Entries[4].Comment != "the CA" {
		t.Errorf("the aligned line parsed wrong: %+v", f.Entries[4])
	}
}

// The whole reason SetHosts splices bytes instead of rebuilding the line: a
// real known_hosts has lines somebody lined up by hand, and a rename must not
// quietly reformat one.
func TestRenamingLeavesEveryOtherByteOnTheLineWhereItWas(t *testing.T) {
	raw := knownFixture()
	f := parseKnownHostsFile(raw)
	if f.Entries[4].Marker == "" {
		t.Fatal("setup: entry 4 should be the marked, generously spaced one")
	}
	g, err := f.SetHosts(4, "*.new.example.com")
	if err != nil {
		t.Fatal(err)
	}
	want := "@cert-authority   *.new.example.com   ssh-ed25519   " + b64("key-five") + "   the CA\n"
	if !strings.Contains(g.Text(), want) {
		t.Errorf("the marker and the alignment must survive:\nwant %q\ngot  %q", want, g.Text())
	}
	if d := changed(raw, g.Text()); len(d) != 1 {
		t.Errorf("one name changed, so one line should differ; got %d:\n%s",
			len(d), strings.Join(d, "\n"))
	}
}

// A line sshu cannot make sense of is a line, not a row. Turning it into one
// would offer an edit that writes a key nobody has.
func TestALineThatIsNotAnEntryIsLeftExactlyAsItIs(t *testing.T) {
	f := parseKnownHostsFile(knownFixture())
	for i, e := range f.Entries {
		if e.Hosts == "this-line-is-not-an-entry" {
			t.Fatalf("entry %d should not exist: %+v", i, e)
		}
	}
	g, err := f.Delete(0)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(g.Text(), "this-line-is-not-an-entry\n") {
		t.Errorf("it must survive a write it had nothing to do with:\n%s", g.Text())
	}
}

func TestFingerprintIsTheSHA256FormPeopleCompare(t *testing.T) {
	f := parseKnownHostsFile(knownFixture())
	fp := f.Entries[0].Fingerprint()
	// Unpadded base64 of a 32-byte digest is 43 characters, and ssh prints no
	// "=" — a padded one would not match what the server told you.
	if !strings.HasPrefix(fp, "SHA256:") || len(fp) != len("SHA256:")+43 ||
		strings.Contains(fp, "=") {
		t.Errorf("not the form ssh prints: %q", fp)
	}
	if f.Entries[1].Fingerprint() == fp {
		t.Error("two different keys must not fingerprint alike")
	}
	// A line whose key is not base64 gets no fingerprint rather than a wrong
	// one: a fingerprint is a thing people trust by reading.
	if got := (KnownHostEntry{Key: "not base64!!"}).Fingerprint(); got != "" {
		t.Errorf("an unreadable key should have no fingerprint, got %q", got)
	}
}

func TestRenamingAnEntryTouchesOnlyItsHostField(t *testing.T) {
	raw := knownFixture()
	f := parseKnownHostsFile(raw)
	g, err := f.SetHosts(0, "laptop.example.com")
	if err != nil {
		t.Fatal(err)
	}
	diff := changed(raw, g.Text())
	if len(diff) != 1 {
		t.Fatalf("one name changed, so one line should differ; got %d:\n%s",
			len(diff), strings.Join(diff, "\n"))
	}
	// The key and the comment on that same line are not sshu's to retype.
	if !strings.Contains(g.Text(),
		"laptop.example.com ssh-ed25519 "+b64("key-one")+" laptop\n") {
		t.Errorf("the rest of the line should be untouched:\n%s", g.Text())
	}
}

func TestRenamingRefusesSpacesBecauseTheySplitTheLine(t *testing.T) {
	f := parseKnownHostsFile(knownFixture())
	if _, err := f.SetHosts(0, "a b"); err == nil {
		t.Fatal("a space would make the second name look like the key type")
	}
	if _, err := f.SetHosts(0, "  "); err == nil {
		t.Fatal("a key trusted for nothing is not an entry")
	}
}

func TestDeletingAnEntryLeavesTheCommentAboveIt(t *testing.T) {
	f := parseKnownHostsFile(knownFixture())
	g, err := f.Delete(0)
	if err != nil {
		t.Fatal(err)
	}
	out := g.Text()
	if strings.Contains(out, "198.51.100.10") {
		t.Errorf("the entry should be gone:\n%s", out)
	}
	// Deliberately UNLIKE ~/.ssh/config, where a block takes its own label with
	// it: `# work` here sits over a run of entries, and removing one row is not
	// a reason to delete the heading over the rest.
	if !strings.HasPrefix(out, "# work\n") {
		t.Errorf("the section comment must survive:\n%s", out)
	}
	if n := len(g.Entries); n != 4 {
		t.Errorf("4 entries should be left, got %d", n)
	}
}

func TestAddAppendsOneLine(t *testing.T) {
	raw := knownFixture()
	f := parseKnownHostsFile(raw)
	g, err := f.Add(KnownHostEntry{
		Hosts: "new.example.com", Type: "ssh-ed25519", Key: b64("key-five"), Comment: "added by sshu",
	})
	if err != nil {
		t.Fatal(err)
	}
	want := raw + "new.example.com ssh-ed25519 " + b64("key-five") + " added by sshu\n"
	if g.Text() != want {
		t.Errorf("got:\n%q\nwant:\n%q", g.Text(), want)
	}
}

func TestAddRefusesWhatWouldNotBeAnEntry(t *testing.T) {
	f := parseKnownHostsFile("")
	if _, err := f.Add(KnownHostEntry{Type: "ssh-ed25519", Key: b64("k")}); err == nil {
		t.Error("a key with no host is trusted for nothing")
	}
	if _, err := f.Add(KnownHostEntry{Hosts: "h", Type: "ssh-ed25519"}); err == nil {
		t.Error("a known host with no key is not a known host")
	}
}

func TestSaveRefusesWhenSshAppendedWhileSshuHadItOpen(t *testing.T) {
	p := filepath.Join(t.TempDir(), "known_hosts")
	raw := knownFixture()
	if err := os.WriteFile(p, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := LoadKnownHostsFrom(p)
	if err != nil {
		t.Fatal(err)
	}
	g, err := f.Delete(0)
	if err != nil {
		t.Fatal(err)
	}

	// This is the ordinary case here, not an exotic one: ssh appends a line
	// every time it meets a host for the first time.
	theirs := raw + "fresh.example.com ssh-ed25519 " + b64("key-six") + "\n"
	if err := os.WriteFile(p, []byte(theirs), 0o600); err != nil {
		t.Fatal(err)
	}

	back, err := SaveKnownHostsTo(p, g)
	if err == nil {
		t.Fatal("saving over ssh's own write must be refused")
	}
	if got, _ := os.ReadFile(p); string(got) != theirs {
		t.Fatalf("their file must be left exactly as it was:\n%s", got)
	}
	if back.Text() != theirs {
		t.Errorf("the refusal should carry the reloaded file, got %q", back.Text())
	}
	if _, err := SaveKnownHostsTo(p, back); err != nil {
		t.Errorf("the reloaded copy should save cleanly: %v", err)
	}
}

func TestSaveKeepsTheModeKnownHostsAlreadyHad(t *testing.T) {
	p := filepath.Join(t.TempDir(), "known_hosts")
	if err := os.WriteFile(p, []byte(knownFixture()), 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := LoadKnownHostsFrom(p)
	if err != nil {
		t.Fatal(err)
	}
	g, _ := f.Delete(0)
	if _, err := SaveKnownHostsTo(p, g); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o644 {
		t.Fatalf("mode should be left alone, got %04o", fi.Mode().Perm())
	}
}

func TestAFailedWriteHandsBackWhatIsStillOnDisk(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root writes into a read-only directory anyway")
	}
	dir := filepath.Join(t.TempDir(), "d")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, "known_hosts")
	raw := knownFixture()
	if err := os.WriteFile(p, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := LoadKnownHostsFrom(p)
	if err != nil {
		t.Fatal(err)
	}
	g, _ := f.Delete(0)

	// Readable, not writable: the read-back check passes and the atomic write's
	// temp file cannot be created.
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(dir, 0o700) })

	back, err := SaveKnownHostsTo(p, g)
	if err == nil {
		t.Fatal("the write should have failed")
	}
	// The caller adopts whatever comes back, so it has to be the file as it
	// still IS — not the change that did not land.
	if back.Text() != raw {
		t.Errorf("a failed write must hand back the untouched file, got %q", back.Text())
	}
}
