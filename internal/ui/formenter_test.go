package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/vulcanshen/sshu/internal/store"
)

// tdp K3 in executable form (§11.59). Enter is submit on every field; a form
// that is not finished does not save, and the submit takes the user to the
// FIRST field that is missing and says so. What "finished" means follows the
// Auth toggle, because enabled() already knows.

// filled is "nothing required is empty" — the question Enter used to ask
// before saving, now the first check of a submit (tdp K3).
func filled(f interface{ missing() (string, int) }) bool {
	_, at := f.missing()
	return at < 0
}

func TestEnterOnAnUnfinishedFormPointsAtTheFirstMissingField(t *testing.T) {
	var saved []store.Host
	m := pressA(appWith(sample(), &saved), "A")

	// Auth is password here, which leaves no pick-a-value row enabled — those
	// two keep Enter for their chooser while empty, and this test is about the
	// ordinary rows.
	m.form.fields[fAuth].sel = 0

	// Name filled, Host empty, and the cursor BELOW Host: Enter goes back up to
	// Host — the first missing field — not on to the next one.
	m.form.fields[fName].value = "box"
	m.form.focus = fUser
	m = pressA(m, "enter")
	if saved != nil {
		t.Fatal("an unfinished form must not be written")
	}
	if m.form.focus != fHost {
		t.Errorf("Enter should point at Host, the first missing field; focus=%d", m.form.focus)
	}
	if !strings.Contains(m.form.err, "Host") {
		t.Errorf("the failed submit should say what is missing, got %q", m.form.err)
	}

	// It is not Tab: pressed again it stays on the problem.
	m = pressA(m, "enter")
	if m.form.focus != fHost {
		t.Errorf("a second Enter should stay on Host; focus=%d", m.form.focus)
	}
}

// What Enter points at follows Auth: a password host's first missing field may
// be the Password, which the old "is it filled" question knew and validation
// did not.
func TestEnterPointsAtAMissingPassword(t *testing.T) {
	var saved []store.Host
	m := pressA(appWith(sample(), &saved), "A")
	m.form.fields[fAuth].sel = 0 // password
	for i, v := range map[int]string{fName: "box", fHost: "h", fUser: "root"} {
		m.form.fields[i].value = v
	}
	m.form.focus = fName
	m = pressA(m, "enter")
	if saved != nil || m.form.focus != fPassword || !strings.Contains(m.form.err, "Password") {
		t.Errorf("Enter should point at Password; focus=%d err=%q saved=%v", m.form.focus, m.form.err, saved)
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
			if got := filled(m.form); got != tc.complete {
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
	if !filled(m.form) {
		t.Error("a credential host has no user of its own — an empty User must not block it")
	}
}

// Enter is save whether or not the form is finished (tdp K3): what is missing
// is said by the error row when a save fails, not by the legend flipping.
func TestTheHintAlwaysSaysEnterSaves(t *testing.T) {
	m := pressA(appWith(sample(), nil), "A")
	m.form.focus = fName
	if h := formHint(m); !strings.Contains(h, "Enter:save") || strings.Contains(h, "Enter:next") {
		t.Errorf("an unfinished form still advertises Enter as save\n%q", h)
	}
	m = fillHostForm(m, "hint-box")
	if h := formHint(m); !strings.Contains(h, "Enter:save") {
		t.Errorf("a finished form must advertise Enter as save\n%q", h)
	}
}

// ------------------------------------------------------------- the sibling

func TestCredFormEnterFollowsTheSameRule(t *testing.T) {
	m := pressA(appWith(nil, nil), "1", "j", "enter", "A")
	if !m.credFormUI.isActive() {
		t.Fatal("setup: no form")
	}
	m.credFormUI.fields[cName].value = "ops"
	m.credFormUI.focus = cPassword
	m = pressA(m, "enter")
	if !m.credFormUI.isActive() {
		t.Fatal("an unfinished credential form must not save")
	}
	if m.credFormUI.focus != cUser || !strings.Contains(m.credFormUI.err, "User") {
		t.Errorf("Enter should point at User, the first missing field; focus=%d err=%q",
			m.credFormUI.focus, m.credFormUI.err)
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
	if filled(m.credFormUI) {
		t.Error("privatekey without a key file is not finished")
	}
	m.credFormUI.fields[cAuth].sel = 0 // password
	if filled(m.credFormUI) {
		t.Error("password without a password is not finished")
	}
	m.credFormUI.fields[cPassword].value = "pw"
	if !filled(m.credFormUI) {
		t.Error("password with a password is finished — the key row is dark and must not count")
	}
}

// ---------------------------------------------------------------- the hotkey

// Edit was reachable only by Enter, which prints no bracket — so the only way
// to learn it was to press Enter and see what happened. It has a letter now.
//
// Enter still ends at the same form, but by way of the read-only float
// (§11.29): E is the shortcut for people who already know what the row holds,
// Enter is for people who are looking. Both are checked here, because the pair
// coming apart — a shortcut that stops working, or a look that opens a form you
// can type into — is the failure this row exists to prevent.
func TestCredentialEditIsOnEAndEnterStillReachesIt(t *testing.T) {
	creds := []store.Credential{{Name: "ops", User: "root",
		Auth: store.AuthPrivateKey, IdentityFile: "~/.ssh/id_ed25519"}}
	for _, keys := range [][]string{{"E"}, {"enter", "enter"}} {
		m := settle(pressA(credApp(nil, creds), "1", "j", "enter"))
		m = settle(pressA(m, keys...))
		if !m.credFormUI.isActive() {
			t.Errorf("%v should reach the edit form", keys)
			continue
		}
		if m.credFormUI.editing != "ops" {
			t.Errorf("%v opened a form editing %q, want the row under the cursor",
				keys, m.credFormUI.editing)
		}
	}

	// A single Enter must stop at the float. If it went straight through, the
	// look would be the form again and the change would have bought nothing.
	m1 := settle(pressA(credApp(nil, creds), "1", "j", "enter", "enter"))
	if m1.credFormUI.isActive() {
		t.Error("one Enter should stop at the read-only float, not open the form")
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

// The known_hosts fetch form: Enter with no host points at Host and says so,
// rather than sending a handshake to nowhere (tdp K3).
func TestTheFetchFormPointsAtAMissingHost(t *testing.T) {
	m, _ := knownApp(t, knownUIFixture())
	m = pressA(m, "A")
	m.knownAddUI.focus = kfPort
	m = pressA(m, "enter")
	if m.knownAddUI.scanning {
		t.Fatal("an empty host must not start a fetch")
	}
	if m.knownAddUI.focus != kfHost || !strings.Contains(m.knownAddUI.err, "Host") {
		t.Errorf("Enter should point at Host; focus=%d err=%q", m.knownAddUI.focus, m.knownAddUI.err)
	}
}

// The ~/.ssh/config form: a block with no Host pattern points at it (tdp K3).
func TestTheConfigFormPointsAtAMissingPattern(t *testing.T) {
	m, _ := cfgApp(t, cfgFixture)
	m = pressA(m, "A")
	if !m.sshcfgFormUI.isActive() {
		t.Fatal("setup: A should open the form")
	}
	m.sshcfgFormUI.focus = sfHostName
	m = pressA(m, "enter")
	if !m.sshcfgFormUI.isActive() {
		t.Fatal("a block with no pattern must not be written")
	}
	if m.sshcfgFormUI.focus != sfHost || !strings.Contains(m.sshcfgFormUI.err, "Host pattern") {
		t.Errorf("Enter should point at the pattern; focus=%d err=%q", m.sshcfgFormUI.focus, m.sshcfgFormUI.err)
	}
}

// On the credential form too, what Enter points at follows Auth: a password
// credential with no password is caught by the "filled in" half of the check.
func TestCredFormEnterPointsAtAMissingPassword(t *testing.T) {
	m := pressA(appWith(nil, nil), "1", "j", "enter", "A")
	for i, v := range map[int]string{cName: "ops", cUser: "root"} {
		m.credFormUI.fields[i].value = v
	}
	m.credFormUI.fields[cAuth].sel = 0 // password
	m.credFormUI.focus = cName
	m = pressA(m, "enter")
	if !m.credFormUI.isActive() || m.credFormUI.focus != cPassword ||
		!strings.Contains(m.credFormUI.err, "Password") {
		t.Errorf("Enter should point at Password; focus=%d err=%q", m.credFormUI.focus, m.credFormUI.err)
	}
}
