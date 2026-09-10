package store

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestDirPrecedence(t *testing.T) {
	t.Setenv("SSHU_CONFIG", "/tmp/override")
	t.Setenv("XDG_CONFIG_HOME", "/tmp/xdg")
	if got, _ := Dir(); got != "/tmp/override" {
		t.Fatalf("SSHU_CONFIG should win, got %q", got)
	}

	t.Setenv("SSHU_CONFIG", "")
	if got, _ := Dir(); got != "/tmp/xdg/sshu" {
		t.Fatalf("XDG_CONFIG_HOME should win, got %q", got)
	}
}

func TestLoadMissingFileIsEmptyNotError(t *testing.T) {
	f, _, err := LoadFrom(filepath.Join(t.TempDir(), "nope.yaml"))
	if err != nil {
		t.Fatalf("missing file must not error: %v", err)
	}
	if len(f.Hosts) != 0 {
		t.Fatalf("want no hosts, got %d", len(f.Hosts))
	}
}

func TestSaveLoadRoundTripAndPerms(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "hosts.yaml")
	in := File{Hosts: []Host{
		{Name: "prod-web-01", Host: "10.0.3.14", Port: 22, User: "deploy",
			Auth: AuthPrivateKey, IdentityFile: "~/.ssh/id_ed25519"},
		{Name: "db-replica", Host: "db.internal.corp", Port: 2222, User: "postgres",
			Auth: AuthPassword, Password: "s3cr3t"},
	}}
	if err := SaveTo(path, in); err != nil {
		t.Fatalf("save: %v", err)
	}

	st, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := st.Mode().Perm(); perm != 0o600 {
		t.Fatalf("hosts.yaml holds plaintext passwords, want 0600, got %o", perm)
	}

	out, _, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if out.Version != hostsVersion {
		t.Fatalf("version want %d, got %d", hostsVersion, out.Version)
	}
	if !reflect.DeepEqual(out.Hosts, in.Hosts) {
		t.Fatalf("round trip mismatch:\n in %+v\nout %+v", in.Hosts, out.Hosts)
	}
}

func TestSaveRewidensNarrowPerms(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hosts.yaml")
	f := File{Hosts: []Host{{Name: "a", Host: "h", Port: 22, User: "u", Auth: AuthPassword}}}
	if err := SaveTo(path, f); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o644); err != nil { // user widens it by hand
		t.Fatal(err)
	}
	if err := SaveTo(path, f); err != nil {
		t.Fatal(err)
	}
	st, _ := os.Stat(path)
	if perm := st.Mode().Perm(); perm != 0o600 {
		t.Fatalf("save must reassert 0600, got %o", perm)
	}
}

func TestLoadFillsDefaultPort(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hosts.yaml")
	os.WriteFile(path, []byte("version: 1\nhosts:\n  - name: a\n    host: h\n    user: u\n    auth: password\n"), 0o600)
	f, _, err := LoadFrom(path)
	if err != nil {
		t.Fatal(err)
	}
	if f.Hosts[0].Port != DefaultPort {
		t.Fatalf("omitted port should default to %d, got %d", DefaultPort, f.Hosts[0].Port)
	}
}

// A hand-edited file with a name in it twice used to reach the UI whole and
// then behave as if it had not: every lookup resolved to the first, deleting
// "the second one" removed both, and the next save was refused outright. The
// list that comes back is the list the rest of sshu already assumed it had.
func TestLoadDropsDuplicateNamesAndSaysWhich(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hosts.yaml")
	os.WriteFile(path, []byte("version: 1\nhosts:\n"+
		"  - {name: prod, host: a, port: 22, user: u, auth: password}\n"+
		"  - {name: web, host: b, port: 22, user: u, auth: password}\n"+
		"  - {name: prod, host: c, port: 99, user: v, auth: password}\n"+
		"  - {name: prod, host: d, port: 98, user: w, auth: password}\n"), 0o600)

	f, dropped, err := LoadFrom(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Hosts) != 2 {
		t.Fatalf("want the two distinct names, got %+v", f.Hosts)
	}
	// The FIRST prod survives — the one every lookup already resolved to.
	if f.Hosts[0].Name != "prod" || f.Hosts[0].Host != "a" {
		t.Errorf("the first entry of a repeated name should win, got %+v", f.Hosts[0])
	}
	if f.Hosts[1].Name != "web" {
		t.Errorf("a name used once must survive untouched, got %+v", f.Hosts[1])
	}
	// Two entries were dropped, so the name is named twice: that is how many
	// extra copies the file holds, which is what the user has to go and remove.
	if len(dropped) != 2 || dropped[0] != "prod" || dropped[1] != "prod" {
		t.Errorf("want each dropped entry reported by name, got %q", dropped)
	}
	// And the point of the whole exercise: what loaded can now be saved.
	if err := f.Validate(); err != nil {
		t.Errorf("a de-duped list must be saveable, got %v", err)
	}
}

// De-duping is not a rewrite. The file keeps its duplicates until the user
// saves something, which is the only moment they asked for it to change.
func TestLoadLeavesTheDuplicateOnDisk(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hosts.yaml")
	raw := []byte("version: 1\nhosts:\n" +
		"  - {name: prod, host: a, port: 22, user: u, auth: password}\n" +
		"  - {name: prod, host: c, port: 99, user: v, auth: password}\n")
	os.WriteFile(path, raw, 0o600)

	if _, dropped, err := LoadFrom(path); err != nil || len(dropped) != 1 {
		t.Fatalf("load: %v, dropped %q", err, dropped)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(raw) {
		t.Errorf("loading must not rewrite the file:\nwant %s\ngot  %s", raw, after)
	}
}

// A file with nothing repeated reports nothing. A warning that fires on the
// ordinary case is a warning nobody reads.
func TestLoadWithNoDuplicatesSaysNothing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hosts.yaml")
	os.WriteFile(path, []byte("version: 1\nhosts:\n"+
		"  - {name: prod, host: a, port: 22, user: u, auth: password}\n"+
		"  - {name: web, host: b, port: 22, user: u, auth: password}\n"), 0o600)

	f, dropped, err := LoadFrom(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(dropped) != 0 {
		t.Errorf("nothing was repeated, got %q", dropped)
	}
	if len(f.Hosts) != 2 {
		t.Errorf("want both hosts, got %+v", f.Hosts)
	}
}

func TestValidate(t *testing.T) {
	ok := Host{Name: "a", Host: "h", Port: 22, User: "u", Auth: AuthPassword}
	if err := ok.Validate(); err != nil {
		t.Fatalf("valid host rejected: %v", err)
	}
	for name, h := range map[string]Host{
		"no name":  {Host: "h", Port: 22, User: "u", Auth: AuthPassword},
		"no host":  {Name: "a", Port: 22, User: "u", Auth: AuthPassword},
		"no user":  {Name: "a", Host: "h", Port: 22, Auth: AuthPassword},
		"port 0":   {Name: "a", Host: "h", Port: 0, User: "u", Auth: AuthPassword},
		"port big": {Name: "a", Host: "h", Port: 70000, User: "u", Auth: AuthPassword},
		"bad auth": {Name: "a", Host: "h", Port: 22, User: "u", Auth: "kerberos"},
	} {
		if err := h.Validate(); err == nil {
			t.Errorf("%s: want error, got nil", name)
		}
	}

	dup := File{Hosts: []Host{ok, ok}}
	if err := dup.Validate(); err == nil {
		t.Error("duplicate names must be rejected — name is the CRUD key")
	}
}

func TestExpandTilde(t *testing.T) {
	home, _ := os.UserHomeDir()
	if got := ExpandTilde("~/.ssh/id_ed25519"); got != filepath.Join(home, ".ssh/id_ed25519") {
		t.Fatalf("got %q", got)
	}
	if got := ExpandTilde("/abs/path"); got != "/abs/path" {
		t.Fatalf("absolute path must be untouched, got %q", got)
	}
	if got := ExpandTilde("~notuser/x"); got != "~notuser/x" {
		t.Fatalf("~user form is not tilde expansion here, got %q", got)
	}
}
