package store

import (
	"os"
	"path/filepath"
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
	f, err := LoadFrom(filepath.Join(t.TempDir(), "nope.yaml"))
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

	out, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if out.Version != currentVersion {
		t.Fatalf("version want %d, got %d", currentVersion, out.Version)
	}
	if len(out.Hosts) != 2 || out.Hosts[0] != in.Hosts[0] || out.Hosts[1] != in.Hosts[1] {
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
	f, err := LoadFrom(path)
	if err != nil {
		t.Fatal(err)
	}
	if f.Hosts[0].Port != DefaultPort {
		t.Fatalf("omitted port should default to %d, got %d", DefaultPort, f.Hosts[0].Port)
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
