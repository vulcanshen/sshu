package store

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCredsRoundTripAndPerms(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "credentials.yaml")
	in := CredsFile{Credentials: []Credential{
		{Name: "deploy-key", User: "deploy", Auth: AuthPrivateKey,
			IdentityFile: "~/.ssh/id_ed25519"},
		{Name: "ops-pw", User: "ops", Auth: AuthPassword, Password: "s3cr3t"},
	}}
	if err := SaveCredsTo(path, in); err != nil {
		t.Fatalf("save: %v", err)
	}

	st, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := st.Mode().Perm(); perm != 0o600 {
		t.Fatalf("credentials.yaml holds plaintext passwords, want 0600, got %o", perm)
	}

	out, err := LoadCredsFrom(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(out.Credentials) != 2 || out.Credentials[0] != in.Credentials[0] ||
		out.Credentials[1] != in.Credentials[1] {
		t.Fatalf("round trip mismatch:\n in %+v\nout %+v", in.Credentials, out.Credentials)
	}
}

func TestCredsMissingFileIsEmptyNotError(t *testing.T) {
	f, err := LoadCredsFrom(filepath.Join(t.TempDir(), "nope.yaml"))
	if err != nil {
		t.Fatalf("missing file must not error: %v", err)
	}
	if len(f.Credentials) != 0 {
		t.Fatalf("want no credentials, got %d", len(f.Credentials))
	}
}

func TestCredValidation(t *testing.T) {
	base := Credential{Name: "a", User: "u", Auth: AuthPassword}
	for _, tc := range []struct {
		name   string
		break_ func(*Credential)
	}{
		{"empty name", func(c *Credential) { c.Name = " " }},
		{"empty user", func(c *Credential) { c.User = "" }},
		{"bad auth", func(c *Credential) { c.Auth = "agent" }},
		// A credential naming a credential would be indirection with no floor.
		{"credential auth", func(c *Credential) { c.Auth = AuthCredential }},
	} {
		c := base
		tc.break_(&c)
		if c.Validate() == nil {
			t.Errorf("%s: want an error", tc.name)
		}
	}
	if err := base.Validate(); err != nil {
		t.Errorf("the base credential should be valid: %v", err)
	}
	if err := (CredsFile{Credentials: []Credential{base, base}}).Validate(); err == nil {
		t.Error("duplicate names must be rejected")
	}
}

// A credential is one package: the host takes user AND auth from it together.
// Whatever the host row used to say about its user is overwritten, so "who does
// this connect as" has exactly one answer.
func TestResolveTakesTheWholePackage(t *testing.T) {
	creds := []Credential{{Name: "ops-pw", User: "ops", Auth: AuthPassword, Password: "pw"}}
	h := Host{Name: "web", Host: "10.0.0.1", Port: 22, User: "stale",
		Auth: AuthCredential, Credential: "ops-pw"}

	got, err := Resolve(h, creds)
	if err != nil {
		t.Fatal(err)
	}
	if got.User != "ops" || got.Auth != AuthPassword || got.Password != "pw" {
		t.Fatalf("want the credential's user+auth, got %+v", got)
	}
	// The concrete methods pass through untouched.
	plain := Host{Name: "db", Host: "h", Port: 22, User: "postgres", Auth: AuthPassword}
	if got, _ := Resolve(plain, creds); got != plain {
		t.Fatalf("a non-credential host must pass through, got %+v", got)
	}
}

func TestResolveMissingCredentialSaysSo(t *testing.T) {
	h := Host{Name: "web", Host: "h", Port: 22, Auth: AuthCredential, Credential: "gone"}
	if _, err := Resolve(h, nil); err == nil {
		t.Fatal("a missing credential must be an error, not a silent pass-through")
	}
}

// auth: credential is legal on a host — the user field may then be empty,
// because the credential supplies it — but the reference itself is required.
func TestAHostMayNameACredential(t *testing.T) {
	h := Host{Name: "web", Host: "h", Port: 22, Auth: AuthCredential, Credential: "ops"}
	if err := h.Validate(); err != nil {
		t.Fatalf("credential host with no user should validate: %v", err)
	}
	h.Credential = ""
	if err := h.Validate(); err == nil {
		t.Fatal("auth: credential with no credential named must be rejected")
	}
	plain := Host{Name: "web", Host: "h", Port: 22, Auth: AuthPassword}
	if err := plain.Validate(); err == nil {
		t.Fatal("a password host still requires a user")
	}
}
