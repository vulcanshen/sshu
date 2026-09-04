package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/vulcanshen/sshu/internal/store"
)

// §11.34 in executable form. Enter asks one question on every field — is this
// form finished? — and the answer decides whether it saves or steps on. What
// "finished" means follows the Auth toggle, because enabled() already knows.

func TestEnterIsNextUntilTheFormIsFinished(t *testing.T) {
	var saved []store.Host
	m := pressA(appWith(sample(), &saved), "A")

	// Auth is password here, which leaves no pick-a-value row enabled — those
	// two keep Enter for their chooser while empty, and this test is about the
	// ordinary rows.
	m.form.fields[fAuth].sel = 0

	// A fresh form has only the port filled in, so Enter is next, over and over.
	seen := map[int]bool{}
	for range fCount + 2 {
		before := m.form.focus
		m = pressA(m, "enter")
		seen[before] = true
		if m.form.submitted {
			t.Fatalf("an unfinished form must not submit (stopped on field %d)", before)
		}
		if m.form.focus == before {
			t.Fatalf("Enter on an unfinished form is next, but focus stayed on %d", before)
		}
	}
	if saved != nil {
		t.Error("nothing should have been written")
	}
	// "next 操作支援 loop": more presses than there are fields, so it came round.
	if len(seen) < 2 || !seen[fName] {
		t.Errorf("next should have looped the whole form, visited %v", seen)
	}
}

// The Credential row is the one exception, and only while it is EMPTY: there,
// "next" would step over the only row that cannot be filled any other way.
func TestTheEmptyPickRowsKeepTheirChooser(t *testing.T) {
	m := pressA(credApp(sample(), nil), "A")
	m.form.fields[fAuth].sel = 2 // credential
	m.form.focus = fCredential
	m = pressA(m, "enter")
	if !m.credPicker.isActive() {
		t.Error("Enter on the empty Credential row must open the chooser, not step past it")
	}
}

func TestEnterSavesTheMomentNothingIsMissing(t *testing.T) {
	var saved []store.Host
	m := pressA(appWith(sample(), &saved), "A")
	m = fillHostForm(m, "finished-box")
	m = pressA(m, "enter")

	if m.form.isActive() {
		t.Fatalf("a finished form must save on Enter; err=%q", m.form.err)
	}
	if len(saved) != len(sample())+1 {
		t.Fatalf("saved %d hosts, want %d", len(saved), len(sample())+1)
	}
}

// What is required follows Auth, and the rows Auth switched off must not hold
// the form hostage — that is the whole reason completeness reads enabled()
// rather than a second list.
func TestWhatIsRequiredFollowsTheAuthChoice(t *testing.T) {
	for _, tc := range []struct {
		name     string
		auth     int
		fill     map[int]string
		complete bool
	}{
		{"password missing", 0, nil, false},
		{"password given", 0, map[int]string{fPassword: "pw"}, true},
		{"key file missing", 1, nil, false},
		{"key file given", 1, map[int]string{fIdentity: "~/.ssh/id"}, true},
		{"credential missing", 2, nil, false},
		{"credential given", 2, map[int]string{fCredential: "ops"}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := pressA(appWith(sample(), nil), "A")
			m.form.fields[fAuth].sel = tc.auth
			for i, v := range map[int]string{fName: "box", fHost: "h", fUser: "root"} {
				m.form.fields[i].value = v
			}
			for i, v := range tc.fill {
				m.form.fields[i].value = v
			}
			if got := m.form.complete(); got != tc.complete {
				t.Errorf("complete = %v, want %v", got, tc.complete)
			}
		})
	}

	// User is the mirror case: a credential host does not want one, so an empty
	// User must not keep the form unfinished.
	m := pressA(appWith(sample(), nil), "A")
	m.form.fields[fAuth].sel = 2
	for i, v := range map[int]string{fName: "box", fHost: "h", fCredential: "ops"} {
		m.form.fields[i].value = v
	}
	if !m.form.complete() {
		t.Error("a credential host has no user of its own — an empty User must not block it")
	}
}

// The legend is the whole disclosure for the rule: it answers "why did Enter
// not save" before the question gets asked.
func TestTheHintSaysWhetherEnterSavesOrSteps(t *testing.T) {
	m := pressA(appWith(sample(), nil), "A")
	m.form.focus = fName
	if h := formHint(m); !strings.Contains(h, "next") || strings.Contains(h, "save") {
		t.Errorf("an unfinished form must advertise Enter as next\n%q", h)
	}
	m = fillHostForm(m, "hint-box")
	if h := formHint(m); !strings.Contains(h, "save") {
		t.Errorf("a finished form must advertise Enter as save\n%q", h)
	}
}

// ------------------------------------------------------------- the sibling

func TestCredFormEnterFollowsTheSameRule(t *testing.T) {
	m := pressA(appWith(nil, nil), "1", "j", "enter", "A")
	if !m.credFormUI.isActive() {
		t.Fatal("setup: no form")
	}
	m.credFormUI.focus = cName
	before := m.credFormUI.focus
	m = pressA(m, "enter")
	if m.credFormUI.submitted {
		t.Fatal("an unfinished credential form must not submit")
	}
	if m.credFormUI.focus == before {
		t.Error("Enter on an unfinished form is next, so the focus has to move")
	}

	m = fillCredForm(m, "ops")
	m = pressA(m, "enter")
	if m.credFormUI.isActive() {
		t.Errorf("a finished credential form must save on Enter; err=%q", m.credFormUI.err)
	}
}

// The key file is required under privatekey and irrelevant under password —
// the same enabled()-shaped rule as the host form.
func TestTheCredFormRequiresWhatItsAuthUses(t *testing.T) {
	m := pressA(appWith(nil, nil), "1", "j", "enter", "A")
	for i, v := range map[int]string{cName: "ops", cUser: "root"} {
		m.credFormUI.fields[i].value = v
	}
	m.credFormUI.fields[cAuth].sel = 1 // privatekey
	if m.credFormUI.complete() {
		t.Error("privatekey without a key file is not finished")
	}
	m.credFormUI.fields[cAuth].sel = 0 // password
	if m.credFormUI.complete() {
		t.Error("password without a password is not finished")
	}
	m.credFormUI.fields[cPassword].value = "pw"
	if !m.credFormUI.complete() {
		t.Error("password with a password is finished — the key row is dark and must not count")
	}
}

// ---------------------------------------------------------------- the hotkey

// Edit was reachable only by Enter, which prints no bracket — so the only way
// to learn it was to press Enter and see what happened. It has a letter now,
// and Enter still works, because on a credential "go in" and "edit" are the
// same door.
func TestCredentialEditIsOnEAndEnterStillWorks(t *testing.T) {
	creds := []store.Credential{{Name: "ops", User: "root",
		Auth: store.AuthPrivateKey, IdentityFile: "~/.ssh/id_ed25519"}}
	for _, key := range []string{"E", "enter"} {
		m := pressA(credApp(nil, creds), "1", "j", "enter")
		m = pressA(m, key)
		if !m.credFormUI.isActive() {
			t.Errorf("%s should open the edit form", key)
			continue
		}
		if m.credFormUI.editing != "ops" {
			t.Errorf("%s opened a form editing %q, want the row under the cursor",
				key, m.credFormUI.editing)
		}
	}

	// And the marking says E, or the letter is a secret again.
	for _, a := range credActions {
		if a.label == "Edit" && a.key != "E" {
			t.Errorf("the Edit row is marked %q — the bracket is the disclosure", a.key)
		}
	}
	m := pressA(credApp(nil, creds), "1", "j", "enter", " ")
	if got := ansi.Strip(m.View()); !strings.Contains(got, "[E]dit") {
		t.Error("the Space menu must show the bracket, or the letter is undiscoverable")
	}
}
