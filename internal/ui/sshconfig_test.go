package ui

import (
	"context"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/vulcanshen/sshu/internal/store"
)

// An sshconfig host puts on the command line only what it has. Nothing
// about how to authenticate, and no default standing in for an empty port
// or user — that default would beat the file (§11.41).
func TestAnSSHConfigHostSendsOnlyWhatItHas(t *testing.T) {
	bare := store.Host{Name: "gw", Host: "gw", Auth: store.AuthSSHConfig}
	got := strings.Join(buildSSHCmd(bare, "/bin/sshu", 9).Args[1:], " ")
	for _, banned := range []string{"-p ", "-i ", "IdentitiesOnly", "@"} {
		if strings.Contains(got, banned) {
			t.Errorf("args %q should not contain %q", got, banned)
		}
	}
	if !strings.HasSuffix(got, " gw") {
		t.Errorf("args %q should end in the bare destination", got)
	}
	if env := strings.Join(sshEnv(bare, "/bin/sshu"), "\n"); strings.Contains(env, "SSH_ASKPASS=") {
		t.Error("an sshconfig host has no stored password for the helper to print")
	}

	// Filled in, they go out — and win over the file, as they always have.
	full := store.Host{Name: "gw", Host: "gw", Port: 2222, User: "jump", Auth: store.AuthSSHConfig}
	got = strings.Join(buildSSHCmd(full, "", 9).Args[1:], " ")
	for _, want := range []string{"-p 2222", "jump@gw"} {
		if !strings.Contains(got, want) {
			t.Errorf("args %q missing %q", got, want)
		}
	}
}

// The sftp dial is ssh's own sftp invocation, in its own session, with
// every question routed to the helper.
func TestTheSFTPCommandIsSSHWithTheSubsystemAndNoTerminal(t *testing.T) {
	h := store.Host{Name: "gw", Host: "gw", Auth: store.AuthSSHConfig}
	cmd := buildSFTPCmd(context.Background(), h, "/bin/sshu", "/tmp/s", 7)
	got := strings.Join(cmd.Args[1:], " ")
	for _, want := range []string{"-s gw sftp", "ConnectTimeout=7", "ClearAllForwardings=yes",
		"PermitLocalCommand=no", "ForwardX11=no"} {
		if !strings.Contains(got, want) {
			t.Errorf("args %q missing %q", got, want)
		}
	}
	if !strings.HasSuffix(got, " sftp") {
		t.Errorf("the subsystem name is the last word, got %q", got)
	}
	if cmd.SysProcAttr == nil || !cmd.SysProcAttr.Setsid {
		t.Error("the child must be a session leader with no controlling terminal")
	}
	env := strings.Join(cmd.Env, "\n")
	for _, want := range []string{"SSH_ASKPASS=/bin/sshu", "SSH_ASKPASS_REQUIRE=force", askpassSockEnv + "=/tmp/s"} {
		if !strings.Contains(env, want) {
			t.Errorf("env missing %q", want)
		}
	}
}

// The form: sshconfig wants a name and a destination and nothing more.
func TestTheFormAsksAnSSHConfigHostForNothingButADestination(t *testing.T) {
	m := pressA(appWith(sample(), nil), "A")
	m.form.fields[fAuth].sel = 3
	for i, v := range map[int]string{fName: "gw", fHost: "gw"} {
		m.form.fields[i].value = v
	}
	m.form.fields[fPort].value = ""
	if !m.form.complete() {
		t.Error("name and host are the whole of what an sshconfig host needs")
	}
	for _, i := range []int{fCredential, fIdentity, fPassword} {
		if m.form.enabled(i) {
			t.Errorf("row %d should be dark on an sshconfig host", i)
		}
	}
	h := m.form.host()
	if h.Auth != store.AuthSSHConfig || h.Port != 0 || h.User != "" {
		t.Errorf("record = %+v, want sshconfig with nothing filled in", h)
	}
	if err := h.Validate(); err != nil {
		t.Errorf("what the form builds should be valid: %v", err)
	}
}

// Switching to sshconfig empties a Port still at its default — 22 sent to
// ssh would beat the file's Port — and switching away puts it back. A port
// the user typed survives both.
func TestSwitchingToSSHConfigDropsTheDefaultPortAndSwitchingBackRestoresIt(t *testing.T) {
	m := pressA(appWith(sample(), nil), "A", "tab", "tab", "tab") // Name, Host, Port → Auth
	if m.form.focus != fAuth {
		t.Fatalf("focus %d, want the Auth row", m.form.focus)
	}
	m = pressA(m, "right", "right") // privatekey → credential → sshconfig
	if got := m.form.auth(); got != store.AuthSSHConfig {
		t.Fatalf("auth = %q", got)
	}
	if got := m.form.fields[fPort].value; got != "" {
		t.Errorf("Port should be emptied, is %q", got)
	}
	if m.form.fields[fPort].placeholder != "ssh decides" || m.form.fields[fUser].placeholder != "ssh decides" {
		t.Error("the empty rows should say where the answer comes from")
	}
	m = pressA(m, "left")
	if got := m.form.fields[fPort].value; got != "22" {
		t.Errorf("Port should be 22 again on a credential host, is %q", got)
	}

	m.form.fields[fPort].value = "2222"
	m = pressA(m, "right")
	if got := m.form.fields[fPort].value; got != "2222" {
		t.Errorf("a typed port must survive the switch, got %q", got)
	}
}

// An sshconfig host under edit opens with an empty Port, not "0".
func TestEditingAnSSHConfigHostShowsNoPortNotZero(t *testing.T) {
	hosts := []store.Host{{Name: "gw", Host: "gw", Auth: store.AuthSSHConfig}}
	m := pressA(appWith(hosts, nil), "E")
	if got := m.form.fields[fPort].value; got != "" {
		t.Errorf("Port = %q, want empty", got)
	}
	if m.form.fields[fAuth].sel != 3 {
		t.Errorf("the toggle should sit on sshconfig, sel = %d", m.form.fields[fAuth].sel)
	}
}

// The row: the file's glyph, and a dash where a port would be.
func TestTheRowShowsADashForAPortSSHDecides(t *testing.T) {
	h := store.Host{Name: "gw", Host: "gw", Auth: store.AuthSSHConfig}
	cols := tableCols{name: 12, user: 8, host: 12, port: true, auth: true}
	line := ansi.Strip(renderHostRow(h, "", cols, false, 80)[0])
	if !strings.Contains(line, portUnset) {
		t.Errorf("row should carry the dash, got %q", line)
	}
	if !strings.Contains(line, glyphFileCog) || !strings.Contains(line, "sshconfig") {
		t.Errorf("row should say sshconfig with the file's glyph, got %q", line)
	}
	if strings.Contains(hostHaystack(h), "0") {
		t.Errorf("a port ssh decides is not a 0 to search for: %q", hostHaystack(h))
	}
}

// The detail: an empty port or user says where it comes from, the auth
// section says nothing is stored, and a host no block matches is told so.
func TestTheDetailOfAnSSHConfigHostSaysWhatIsLeftToSSH(t *testing.T) {
	h := store.Host{Name: "gw", Host: "gw", Auth: store.AuthSSHConfig}
	var text []string
	for _, sec := range hostDetail(h, nil, store.SSHConfigFile{}, 10) {
		text = append(text, sec.title)
		for _, r := range sec.rows {
			text = append(text, r.label+" "+r.value)
		}
	}
	all := strings.Join(text, "\n")
	for _, want := range []string{"Port " + sshDecides, "User " + sshDecides,
		"none stored", "no block matches"} {
		if !strings.Contains(all, want) {
			t.Errorf("detail missing %q:\n%s", want, all)
		}
	}

	// A password host with nothing in the file gets no such section — for
	// it, an empty answer is the normal case.
	p := store.Host{Name: "p", Host: "h", Port: 22, User: "u", Auth: store.AuthPassword}
	for _, sec := range hostDetail(p, nil, store.SSHConfigFile{}, 10) {
		if strings.Contains(sec.title, "config") {
			t.Errorf("a password host should draw no config section when nothing matches")
		}
	}
}

// Everywhere an address is rendered, a missing user or port is left out
// rather than shown as "@host" or ":0".
func TestAnAddressLeavesOutWhatTheHostDoesNotHave(t *testing.T) {
	h := store.Host{Name: "gw", Host: "gw", Auth: store.AuthSSHConfig}
	if got := destination(h); got != "gw" {
		t.Errorf("destination = %q", got)
	}
	if got := fitUserHost("", "gw.example", 20); got != "gw.example" {
		t.Errorf("fitUserHost with no user = %q", got)
	}
	if got := fitUserHost("", "gw.example.internal", 6); got != truncate("gw.example.internal", 6) {
		t.Errorf("a bare host still fits the room: %q", got)
	}
}
