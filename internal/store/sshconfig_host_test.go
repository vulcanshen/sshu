package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// An sshconfig host is a destination and a name. Everything else — port,
// user, how to authenticate — is ssh's to read from its own file, so the
// record is allowed to say nothing about any of it.
func TestAnSSHConfigHostNeedsOnlyANameAndADestination(t *testing.T) {
	h := Host{Name: "gw", Host: "gw", Auth: AuthSSHConfig}
	if err := h.Validate(); err != nil {
		t.Fatalf("a bare sshconfig host was rejected: %v", err)
	}
	// And it may still say them, in which case they go on the command line
	// and beat the file, exactly as they do for every other kind (§11.41).
	full := Host{Name: "gw", Host: "gw", Port: 2222, User: "jump", Auth: AuthSSHConfig}
	if err := full.Validate(); err != nil {
		t.Fatalf("an sshconfig host with a port and user was rejected: %v", err)
	}
	// What it may NOT do is name a port that is not one.
	bad := Host{Name: "gw", Host: "gw", Port: 70000, Auth: AuthSSHConfig}
	if err := bad.Validate(); err == nil || !strings.Contains(err.Error(), "or empty") {
		t.Errorf("an impossible port should be refused, and the message should say empty is fine: %v", err)
	}
}

// Port 0 is only ever "ssh decides", and only sshconfig lets ssh decide. A
// password host with no port would go out as `-p 0`.
func TestPortZeroIsOnlyForSSHConfig(t *testing.T) {
	for _, auth := range []AuthMethod{AuthPassword, AuthPrivateKey} {
		h := Host{Name: "a", Host: "h", User: "u", Auth: auth, Port: 0}
		if err := h.Validate(); err == nil {
			t.Errorf("%s host with port 0 should be refused", auth)
		}
	}
	c := Host{Name: "a", Host: "h", Auth: AuthCredential, Credential: "ops", Port: 0}
	if err := c.Validate(); err == nil {
		t.Error("a credential host with port 0 should be refused — the credential supplies no port")
	}
}

// Load fills a missing port with 22 for every host that will put one on the
// command line. For an sshconfig host the missing port IS the answer, and
// filling it would send `-p 22` over whatever Port the file says — the
// "connected to the wrong port" of §11.41.
func TestLoadLeavesAnSSHConfigHostWithoutAPort(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hosts.yaml")
	os.WriteFile(path, []byte(`version: 3
hosts:
  - name: gw
    host: gw
    auth: sshconfig
  - name: plain
    host: h
    user: u
    auth: password
    password: p
`), 0o600)
	f, _, err := LoadFrom(path)
	if err != nil {
		t.Fatal(err)
	}
	if f.Hosts[0].Port != 0 {
		t.Errorf("sshconfig host got port %d filled in; ssh should decide", f.Hosts[0].Port)
	}
	if f.Hosts[1].Port != DefaultPort {
		t.Errorf("password host should still get the default port, got %d", f.Hosts[1].Port)
	}
}

// The file says what the record says: no port line for a port that is not
// there. And it comes back the same way — a round trip must not grow a 22.
func TestAnSSHConfigHostIsSavedWithoutAPortLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hosts.yaml")
	in := File{Hosts: []Host{{Name: "gw", Host: "gw", Auth: AuthSSHConfig}}}
	if err := SaveTo(path, in); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(path)
	if strings.Contains(string(raw), "port:") {
		t.Errorf("a port that is ssh's to decide should not be written:\n%s", raw)
	}
	out, _, err := LoadFrom(path)
	if err != nil {
		t.Fatal(err)
	}
	if out.Hosts[0].Port != 0 || out.Hosts[0].User != "" || out.Hosts[0].Auth != AuthSSHConfig {
		t.Errorf("round trip changed the host: %+v", out.Hosts[0])
	}
}

// Addr leaves out what the record does not have, instead of inventing it.
func TestAddrLeavesOutWhatTheHostDoesNotHave(t *testing.T) {
	for _, tc := range []struct {
		h    Host
		want string
	}{
		{Host{Host: "h", Port: 22, User: "u"}, "u@h:22"},
		{Host{Host: "gw", Auth: AuthSSHConfig}, "gw"},
		{Host{Host: "gw", User: "jump", Auth: AuthSSHConfig}, "jump@gw"},
		{Host{Host: "gw", Port: 2222, Auth: AuthSSHConfig}, "gw:2222"},
	} {
		if got := tc.h.Addr(); got != tc.want {
			t.Errorf("%+v: Addr = %q, want %q", tc.h, got, tc.want)
		}
	}
}

// A credential is user + how that user authenticates. An sshconfig
// credential would supply neither, so the kind does not exist there.
func TestSSHConfigIsNotACredentialKind(t *testing.T) {
	c := Credential{Name: "ops", User: "ops", Auth: AuthSSHConfig}
	if err := c.Validate(); err == nil {
		t.Error("a credential of kind sshconfig should be refused")
	}
}

// The v2 → v3 step is a real one: a v2 file says it needs the upgrade, and a
// file stamped by a later sshu is refused. Pinned here because this is the
// version bump that encryption deliberately did NOT make (see hostsVersion).
func TestASSHConfigHostIsWhatMovedTheFileToVersionThree(t *testing.T) {
	if hostsVersion != 3 {
		t.Fatalf("hostsVersion = %d; this test describes the v3 step and needs rewriting if it moved again", hostsVersion)
	}
	if (File{Version: 2}).NeedsUpgrade() != true {
		t.Error("a v2 file should say it needs the upgrade")
	}
	if (File{Version: 3}).NeedsUpgrade() {
		t.Error("a v3 file is current")
	}
}
