package store

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// ------------------------------------------------------------------- parsing

// Space is the only separator. Everything else is part of a tag — which is the
// rule that lets "k8s:prod" and "web/db" be written without an escape to learn.
func TestOnlySpaceSeparatesTags(t *testing.T) {
	got := ParseTags("prod k8s:prod web/db needs-vpn a,b")
	want := []string{"prod", "k8s:prod", "web/db", "needs-vpn", "a,b"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("commas, colons and slashes are literal:\n got %q\nwant %q", got, want)
	}
}

// Runs of whitespace collapse rather than producing empty tags, because a
// double space is a typo and an empty tag is not a thing.
func TestRunsOfSpaceDoNotMakeEmptyTags(t *testing.T) {
	if got := ParseTags("  prod   tokyo  "); !reflect.DeepEqual(got, []string{"prod", "tokyo"}) {
		t.Errorf("want two tags, got %q", got)
	}
}

// The same tag twice says nothing the once did not.
func TestRepeatedTagsAreDroppedKeepingTheFirstOrder(t *testing.T) {
	got := ParseTags("prod tokyo prod db tokyo")
	want := []string{"prod", "tokyo", "db"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("want first-seen order with repeats gone:\n got %q\nwant %q", got, want)
	}
}

// Case is the user's, not sshu's. Folding here would rewrite what they typed;
// the query end folds instead, which is where folding belongs.
func TestTagCaseIsPreservedExactly(t *testing.T) {
	got := ParseTags("Prod TOKYO ap-northeast-1")
	want := []string{"Prod", "TOKYO", "ap-northeast-1"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("case must survive:\n got %q\nwant %q", got, want)
	}
	// ...and differing case is NOT a repeat, for the same reason.
	if got := ParseTags("prod Prod"); len(got) != 2 {
		t.Errorf("prod and Prod are two tags, got %q", got)
	}
}

// Nothing in gives nil out, not an empty non-nil slice: nil is what yaml omits.
func TestNoTagsIsNilSoTheKeyIsOmitted(t *testing.T) {
	if got := ParseTags("   "); got != nil {
		t.Errorf("whitespace only must give nil, got %#v", got)
	}
	if got := NormalizeTags([]string{"", "  "}); got != nil {
		t.Errorf("blanks only must give nil, got %#v", got)
	}
}

func TestJoinTagsIsParseTagsInverse(t *testing.T) {
	in := []string{"prod", "k8s:prod", "web/db"}
	if got := ParseTags(JoinTags(in)); !reflect.DeepEqual(got, in) {
		t.Errorf("round trip through the form field:\n got %q\nwant %q", got, in)
	}
}

// ------------------------------------------------------------------- storage

func TestTagsSurviveTheFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hosts.yaml")
	in := File{Hosts: []Host{
		{Name: "web-01", Host: "10.0.0.1", Port: 22, User: "deploy",
			Auth: AuthPassword, Password: "pw", Tags: []string{"prod", "tokyo"}},
		{Name: "db-01", Host: "10.0.0.2", Port: 5432, User: "postgres",
			Auth: AuthPassword, Password: "pw"},
	}}
	if err := SaveTo(path, in); err != nil {
		t.Fatal(err)
	}
	out, _, err := LoadFrom(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(out.Hosts[0].Tags, []string{"prod", "tokyo"}) {
		t.Errorf("tags lost in the round trip: %q", out.Hosts[0].Tags)
	}
	// A host without tags must not grow an empty key — the file is read by hand.
	if raw, _ := os.ReadFile(path); strings.Contains(string(raw), "tags: []") {
		t.Errorf("an untagged host wrote an empty list:\n%s", raw)
	}
}

// A hand-edited file goes through the same cleaning the form does, because the
// file is an equally supported way in.
func TestAHandEditedFileGetsItsTagsNormalized(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hosts.yaml")
	os.WriteFile(path, []byte(`version: 2
hosts:
  - name: web-01
    host: 10.0.0.1
    port: 22
    user: deploy
    auth: password
    tags: ["prod", "prod", "  ", " tokyo "]
`), 0o600)

	out, _, err := LoadFrom(path)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"prod", "tokyo"}; !reflect.DeepEqual(out.Hosts[0].Tags, want) {
		t.Errorf("want %q after cleaning, got %q", want, out.Hosts[0].Tags)
	}
}

// ------------------------------------------------------------------ versions

// The two files count separately. This is the test that stops the constants
// being folded back into one: credentials.yaml did not change when tags
// arrived, and stamping it v2 would make an older sshu refuse a file it reads
// perfectly well.
func TestHostsAndCredentialsCarryTheirOwnVersions(t *testing.T) {
	if hostsVersion == credsVersion {
		t.Fatalf("the numbers are equal (%d), which makes this test blind — "+
			"if that is genuinely correct, the test needs rewriting, not deleting",
			hostsVersion)
	}
	dir := t.TempDir()
	hp, cp := filepath.Join(dir, "hosts.yaml"), filepath.Join(dir, "credentials.yaml")

	if err := SaveTo(hp, File{Hosts: []Host{
		{Name: "web", Host: "h", Port: 22, User: "u", Auth: AuthPassword, Password: "p"}}}); err != nil {
		t.Fatal(err)
	}
	if err := SaveCredsTo(cp, CredsFile{Credentials: []Credential{
		{Name: "ops", User: "ops", Auth: AuthPassword, Password: "p"}}}); err != nil {
		t.Fatal(err)
	}

	h, _, _ := LoadFrom(hp)
	c, _, _ := LoadCredsFrom(cp)
	if h.Version != hostsVersion {
		t.Errorf("hosts.yaml stamped %d, want %d", h.Version, hostsVersion)
	}
	if c.Version != credsVersion {
		t.Errorf("credentials.yaml stamped %d, want %d", c.Version, credsVersion)
	}
}

// An older file is reported as older, so the caller can upgrade it. Before
// this, LoadFrom seeded Version with the current number and the field could
// never disagree with the build — the version was decoration.
func TestAnOlderFileSaysItIsOlder(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hosts.yaml")
	os.WriteFile(path, []byte("version: 1\nhosts:\n  - name: web\n    host: h\n    user: u\n    auth: password\n"), 0o600)

	out, _, err := LoadFrom(path)
	if err != nil {
		t.Fatal(err)
	}
	if out.Version != 1 {
		t.Errorf("want the file's own version 1, got %d", out.Version)
	}
	if !out.NeedsUpgrade() {
		t.Error("a v1 file must ask to be upgraded")
	}
}

// A file with no version key at all — written before sshu stamped them — is
// also older, and must not be mistaken for current.
func TestAFileWithNoVersionKeyIsTreatedAsOldest(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hosts.yaml")
	os.WriteFile(path, []byte("hosts:\n  - name: web\n    host: h\n    user: u\n    auth: password\n"), 0o600)

	out, _, _ := LoadFrom(path)
	if out.Version != versionUnset {
		t.Errorf("want %d for an unstamped file, got %d", versionUnset, out.Version)
	}
	if !out.NeedsUpgrade() {
		t.Error("an unstamped file must ask to be upgraded")
	}
}

// A missing file is the first run, and the first run writes today's format —
// so it is NOT an upgrade candidate. Getting this wrong would have every fresh
// install rewrite a file it just invented.
func TestAMissingFileIsCurrentNotOld(t *testing.T) {
	out, _, err := LoadFrom(filepath.Join(t.TempDir(), "nope.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if out.NeedsUpgrade() {
		t.Errorf("a first run has nothing to upgrade, got version %d", out.Version)
	}
}

// The check that gives the number a job: a file from a NEWER sshu is not
// overwritten. Without it, an older build silently drops whatever fields it
// does not know about — which is exactly what a downgrade to v1.5.1 does to
// tags today.
func TestSaveRefusesToOverwriteANewerFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hosts.yaml")
	os.WriteFile(path, []byte("version: 99\nhosts: []\n"), 0o600)

	err := SaveTo(path, File{Hosts: []Host{
		{Name: "web", Host: "h", Port: 22, User: "u", Auth: AuthPassword, Password: "p"}}})
	if err == nil {
		t.Fatal("saving over a version-99 file must be refused")
	}
	if !strings.Contains(err.Error(), "newer sshu") {
		t.Errorf("the refusal must say why, got %q", err)
	}
	// ...and it must not have written anything.
	raw, _ := os.ReadFile(path)
	if !strings.Contains(string(raw), "version: 99") {
		t.Errorf("the newer file was clobbered anyway:\n%s", raw)
	}
}

func TestSaveCredsAlsoRefusesANewerFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "credentials.yaml")
	os.WriteFile(path, []byte("version: 99\ncredentials: []\n"), 0o600)

	err := SaveCredsTo(path, CredsFile{Credentials: []Credential{
		{Name: "ops", User: "ops", Auth: AuthPassword, Password: "p"}}})
	if err == nil || !strings.Contains(err.Error(), "newer sshu") {
		t.Errorf("want a refusal naming the reason, got %v", err)
	}
}

// A file that cannot be parsed is NOT refused. The version cannot be
// established, and refusing then locks the user out of the one program that
// can fix the file — the same reasoning that makes a malformed list drop rows
// rather than halt.
func TestAnUnparseableFileDoesNotBlockSaving(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hosts.yaml")
	os.WriteFile(path, []byte("this: is: not: yaml: at: all\n\t\tbroken\n"), 0o600)

	if err := SaveTo(path, File{Hosts: []Host{
		{Name: "web", Host: "h", Port: 22, User: "u", Auth: AuthPassword, Password: "p"}}}); err != nil {
		t.Errorf("a broken file must be replaceable, got %v", err)
	}
}
