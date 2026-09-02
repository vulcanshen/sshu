package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/vulcanshen/sshu/internal/store"
)

func credApp(hosts []store.Host, creds []store.Credential) AppModel {
	m := New(hosts, nil, store.DefaultConfig()).WithCredentials(creds, nil)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	return settle(next.(AppModel))
}

// Auth gains a third answer. Choosing it swaps which rows are alive: the
// credential row lights up, and User goes dark WITH the key/password rows —
// the credential supplies the user, and a live User field would be the form
// offering two answers to "who connects".
func TestHostFormCredentialAuthSwapsTheRows(t *testing.T) {
	m := pressA(credApp(sample(), nil), "A") // defaults to privatekey
	m.form.fields[fAuth].sel = 2             // credential
	if got := m.form.auth(); got != store.AuthCredential {
		t.Fatalf("option 2 should be credential, got %q", got)
	}
	if m.form.enabled(fUser) {
		t.Error("User must go dark — the credential supplies it")
	}
	if !m.form.enabled(fCredential) {
		t.Error("the Credential row must be live")
	}
	if m.form.enabled(fIdentity) || m.form.enabled(fPassword) {
		t.Error("the concrete auth rows must both be dark")
	}
}

// Enter on the empty Credential field lists the saved credentials; committing
// one writes its name into the field. This is the whole exchange the user
// walks: enter → pick → enter → saved. It used to cost one Enter more, which
// also threw the cursor back to Name.
func TestCredentialPickerFillsTheFieldThenTheNextEnterSaves(t *testing.T) {
	creds := []store.Credential{{Name: "ops", User: "root", Auth: store.AuthPassword, Password: "x"}}
	m := pressA(credApp(sample(), creds), "A")
	m = typeText(m, "box")
	m.form.focus = fHost
	m = typeText(m, "10.0.0.9")
	m.form.fields[fAuth].sel = 2
	m.form.focus = fCredential

	m = pressA(m, "enter")
	if !m.credPicker.isActive() {
		t.Fatal("Enter on the empty Credential field should open the picker")
	}
	m = pressA(m, "enter") // commit the only credential
	if m.credPicker.isActive() {
		t.Error("committing should close the picker")
	}
	if got := m.form.fields[fCredential].value; got != "ops" {
		t.Fatalf("the field should hold the credential's name, got %q", got)
	}
	if !m.form.isActive() {
		t.Fatal("the form must still be standing under the pick")
	}

	// Backspace replaces the whole pick — it is not shaved letter by letter —
	// and it leaves the form standing so the field can be filled again.
	m = pressA(m, "backspace")
	if m.form.fields[fCredential].value != "" {
		t.Error("Backspace should clear the whole line")
	}
	if !m.form.isActive() {
		t.Fatal("Backspace must not close the form")
	}

	// Empty again, so Enter still opens the list. Then ONE Enter saves.
	m = pressA(m, "enter", "enter")
	if got := m.form.fields[fCredential].value; got != "ops" {
		t.Fatalf("re-picking should refill the field, got %q", got)
	}
	m = pressA(m, "enter")
	if m.form.isActive() {
		t.Fatalf("Enter on a filled Credential field should save, err=%q", m.form.err)
	}
	i := indexOfHost(m.hosts.hosts, "box")
	if i < 0 {
		t.Fatal("the host was never written")
	}
	if got := m.hosts.hosts[i].Credential; got != "ops" {
		t.Errorf("the saved host should reference the credential, got %q", got)
	}
}

// Submitting a credential host checks the reference: absent or dangling, the
// form says so instead of writing a host that cannot connect.
func TestHostFormValidatesTheCredentialReference(t *testing.T) {
	m := pressA(credApp(sample(), nil), "A")
	m = typeText(m, "box")
	m.form.focus = fHost
	m = typeText(m, "10.0.0.9")
	m.form.fields[fAuth].sel = 2

	m.form.focus = fName
	m = pressA(m, "enter") // submit with no credential named
	if m.form.err == "" || m.form.errIdx != fCredential {
		t.Fatalf("want the error on the Credential field, got %q at %d", m.form.err, m.form.errIdx)
	}

	m.form.fields[fCredential].value = "ghost"
	m.form.focus = fName
	m = pressA(m, "enter")
	if !strings.Contains(m.form.err, "ghost") {
		t.Fatalf("a dangling reference must be named, got %q", m.form.err)
	}
}

// Connecting resolves the credential at the door: the session runs as the
// credential's user, with the credential's auth.
func TestConnectResolvesTheCredential(t *testing.T) {
	aliveSSH(t)
	hosts := []store.Host{{Name: "web", Host: "127.0.0.1", Port: 22,
		Auth: store.AuthCredential, Credential: "ops"}}
	creds := []store.Credential{{Name: "ops", User: "opsuser", Auth: store.AuthPassword, Password: "x"}}
	m := pressA(credApp(hosts, creds), "enter", "enter")
	t.Cleanup(func() { m.ssh.stopAll() })

	if len(m.ssh.sessions) != 1 {
		t.Fatalf("expected one session, got %d", len(m.ssh.sessions))
	}
	s := m.ssh.sessions[0]
	if s.host.User != "opsuser" || s.host.Auth != store.AuthPassword {
		t.Fatalf("the session should run as the credential: %+v", s.host)
	}
}

// A dangling reference fails AT THE CONFIRM, with a sentence — not three
// keystrokes later inside ssh.
func TestConnectWithAMissingCredentialSaysSo(t *testing.T) {
	hosts := []store.Host{{Name: "web", Host: "h", Port: 22,
		Auth: store.AuthCredential, Credential: "gone"}}
	m := pressA(credApp(hosts, nil), "enter")

	if m.confirm.isActive() {
		t.Fatal("the confirm must not open over a connection that cannot be made")
	}
	if len(m.ssh.sessions) != 0 {
		t.Fatal("no session should have been opened")
	}
	found := false
	for _, e := range m.log.entries {
		if e.level == logError && strings.Contains(e.msg, "gone") {
			found = true
		}
	}
	if !found {
		t.Error("the missing credential should be in the log, by name")
	}
}

// The hosts table shows who a credential host will connect as — the
// credential's user in the user column, its name in the auth column.
func TestHostsTableShowsTheCredentialsUser(t *testing.T) {
	hosts := []store.Host{{Name: "web", Host: "10.0.0.1", Port: 22,
		Auth: store.AuthCredential, Credential: "ops"}}
	creds := []store.Credential{{Name: "ops", User: "opsuser", Auth: store.AuthPassword, Password: "x"}}
	view := ansi.Strip(credApp(hosts, creds).View())

	if !strings.Contains(view, "opsuser") {
		t.Errorf("the user column should show the credential's user:\n%s", view)
	}
	if !strings.Contains(view, "ops") {
		t.Errorf("the auth column should carry the credential's name:\n%s", view)
	}

	// A dangling reference shows "?" rather than inventing a user.
	view = ansi.Strip(credApp(hosts, nil).View())
	if !strings.Contains(view, "?") {
		t.Errorf("a dangling credential should read as ?, got:\n%s", view)
	}
}
