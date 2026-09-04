package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/vulcanshen/sshu/internal/store"
)

// §11.35. Duplicate opens a CREATE form already holding the row it copied, and
// the copied name is the thing that stops it saving — no invented "-copy"
// suffix, no special case, just the uniqueness check doing its ordinary job.

var dupCreds = []store.Credential{{Name: "ops", User: "root",
	Auth: store.AuthPrivateKey, IdentityFile: "~/.ssh/id_ed25519"}}

func TestDeleteMovedToXAndDIsDuplicate(t *testing.T) {
	// The bracket is the disclosure, so the tables have to say it.
	for _, a := range hostActions {
		switch a.label {
		case "Delete":
			if a.key != "X" {
				t.Errorf("hosts: Delete is marked %q, want X", a.key)
			}
		case "Duplicate":
			if a.key != "D" {
				t.Errorf("hosts: Duplicate is marked %q, want D", a.key)
			}
		}
	}
	for _, a := range credActions {
		switch a.label {
		case "Delete":
			if a.key != "X" {
				t.Errorf("credentials: Delete is marked %q, want X", a.key)
			}
		case "Duplicate":
			if a.key != "D" {
				t.Errorf("credentials: Duplicate is marked %q, want D", a.key)
			}
		}
	}

	// And pressing them does those things, which is the half a table cannot
	// promise on its own.
	if m := pressA(appWith(sample(), nil), "D"); !m.form.isActive() {
		t.Error("D on a host should open the duplicate form")
	}
	if m := pressA(appWith(sample(), nil), "X"); !m.confirm.isActive() {
		t.Error("X on a host should raise the delete confirmation")
	}
	if m := pressA(credApp(nil, dupCreds), "1", "j", "enter", "D"); !m.credFormUI.isActive() {
		t.Error("D on a credential should open the duplicate form")
	}
	if m := pressA(credApp(nil, dupCreds), "1", "j", "enter", "X"); !m.confirm.isActive() {
		t.Error("X on a credential should raise the delete confirmation")
	}
}

func TestDuplicateOpensACreateFormHoldingTheWholeRow(t *testing.T) {
	src := sample()[1] // a password host, so every auth row has something in it
	m := pressA(appWith(sample(), nil), "j", "D")
	if !m.form.isActive() {
		t.Fatal("D should open a form")
	}
	if m.form.editing != "" {
		t.Errorf("editing = %q — a duplicate CREATES, and that is what makes the name collide", m.form.editing)
	}
	if m.form.focus != fName {
		t.Errorf("focus = %d, want Name: the rename is the first thing to do", m.form.focus)
	}
	for _, tc := range []struct {
		field int
		want  string
	}{
		{fName, src.Name}, {fHost, src.Host}, {fUser, src.User},
		{fPort, itoa(src.Port)}, {fPassword, src.Password},
	} {
		if got := m.form.fields[tc.field].value; got != tc.want {
			t.Errorf("field %d = %q, want the copied %q", tc.field, got, tc.want)
		}
	}
	if m.form.auth() != src.Auth {
		t.Errorf("auth = %q, want the copied %q", m.form.auth(), src.Auth)
	}
	// Everything filled means Enter is save, which is the whole mechanism.
	if !m.form.complete() {
		t.Error("a duplicate arrives complete, or Enter would be next and nothing would be refused")
	}
}

// The refusal IS the feature: it forces the rename, on the field the cursor is
// already sitting on.
func TestTheFirstEnterOnADuplicateIsRefusedForItsName(t *testing.T) {
	var saved []store.Host
	m := pressA(appWith(sample(), &saved), "j", "D")
	m = pressA(m, "enter")

	if !m.form.isActive() {
		t.Fatal("a refusal keeps the form up — the point is to let the user finish")
	}
	if m.form.errIdx != fName {
		t.Errorf("the error should mark Name, got field %d (%q)", m.form.errIdx, m.form.err)
	}
	if !strings.Contains(m.form.err, sample()[1].Name) {
		t.Errorf("the error should name the clash: %q", m.form.err)
	}
	if saved != nil {
		t.Error("nothing may be written until the name is its own")
	}

	// Rename, and the same Enter saves — a second host, the original untouched.
	m.form.focus = fName
	m.form.fields[fName].caret = len([]rune(m.form.fields[fName].value))
	m = typeText(m, "-copy")
	m = pressA(m, "enter")
	if m.form.isActive() {
		t.Fatalf("a renamed duplicate must save; err=%q", m.form.err)
	}
	if len(saved) != len(sample())+1 {
		t.Fatalf("saved %d hosts, want one more than %d", len(saved), len(sample()))
	}
	if indexOfHost(saved, sample()[1].Name) < 0 {
		t.Error("the row it was copied from must still be there")
	}
	if i := indexOfHost(saved, sample()[1].Name+"-copy"); i < 0 {
		t.Error("the copy was not written")
	} else if saved[i].Password != sample()[1].Password {
		t.Error("the copy must carry the secret it copied, or it cannot connect")
	}
}

func TestDuplicatingACredentialWorksTheSameWay(t *testing.T) {
	m := pressA(credApp(nil, dupCreds), "1", "j", "enter", "D")
	if !m.credFormUI.isActive() {
		t.Fatal("D should open a form")
	}
	if m.credFormUI.editing != "" {
		t.Errorf("editing = %q, want empty — a duplicate creates", m.credFormUI.editing)
	}
	if got := m.credFormUI.fields[cIdentity].value; got != dupCreds[0].IdentityFile {
		t.Errorf("IdentityFile = %q, want the copied %q", got, dupCreds[0].IdentityFile)
	}
	m = pressA(m, "enter")
	if m.credFormUI.errIdx != cName {
		t.Errorf("the copied name must be refused on Name, got field %d (%q)",
			m.credFormUI.errIdx, m.credFormUI.err)
	}
}

// A menu row nobody can find is a feature nobody has.
func TestTheMenuShowsBothBrackets(t *testing.T) {
	got := ansi.Strip(pressA(appWith(sample(), nil), " ").View())
	for _, want := range []string{"[D]uplicate", "Delete"} {
		if !strings.Contains(got, want) {
			t.Errorf("the hosts menu should show %q", want)
		}
	}
	if !strings.Contains(got, "[X]") {
		t.Error("Delete must carry its new bracket, or the letter is a secret")
	}
}
