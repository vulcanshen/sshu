package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/vulcanshen/sshu/internal/store"
)

// detailView opens the detail float on the hosts table and returns the frame it
// drew. Enter is the door now — the same key that goes on to connect from the
// foot of it (§11.29).
func detailView(t *testing.T, m AppModel) string {
	t.Helper()
	m = settle(pressA(m, "enter"))
	if !m.detail.isActive() {
		t.Fatal("Enter should open the detail popup")
	}
	return ansi.Strip(m.View())
}

// A password host says a password IS stored without saying what it is. The
// assertion that matters is the second half: the plaintext must not reach the
// frame by any route, mask or no mask.
func TestHostViewNeverPrintsThePassword(t *testing.T) {
	m := appWith([]store.Host{{Name: "db", Host: "db.corp", Port: 2222, User: "postgres",
		Auth: store.AuthPassword, Password: "hunter2-in-the-clear"}}, nil)

	v := detailView(t, m)
	if strings.Contains(v, "hunter2-in-the-clear") {
		t.Fatalf("the stored password reached the screen:\n%s", v)
	}
	for _, want := range []string{"password", maskedSecret, "db.corp", "2222", "postgres"} {
		if !strings.Contains(v, want) {
			t.Errorf("the view does not mention %q:\n%s", want, v)
		}
	}
}

// The mask is a fixed token, not a ruler: it must not vary with the length of
// what it hides. A per-rune mask (which is what the FORM draws, where the
// length is your own) would publish it.
func TestHostViewMaskDoesNotLeakTheLength(t *testing.T) {
	short := hostDetail(store.Host{Name: "a", Auth: store.AuthPassword, Password: "x"}, nil)
	long := hostDetail(store.Host{Name: "a", Auth: store.AuthPassword,
		Password: strings.Repeat("x", 40)}, nil)

	got := func(secs []detailSection) string {
		for _, s := range secs {
			for _, r := range s.rows {
				if r.label == "Password" {
					return r.value
				}
			}
		}
		t.Fatal("no Password row")
		return ""
	}
	if a, b := got(short), got(long); a != b {
		t.Errorf("the mask tracks the secret's length: %q vs %q", a, b)
	}
}

// A privatekey host shows the path, because the path is the whole of what it
// has — there is no secret to mask, and a bare "privatekey" answers nothing.
func TestHostViewShowsTheIdentityPath(t *testing.T) {
	m := appWith([]store.Host{{Name: "web", Host: "10.0.0.1", Port: 22, User: "deploy",
		Auth: store.AuthPrivateKey, IdentityFile: "~/.ssh/id_ed25519"}}, nil)

	if v := detailView(t, m); !strings.Contains(v, "~/.ssh/id_ed25519") {
		t.Errorf("the identity file should be shown in full:\n%s", v)
	}
}

// A credential host is shown as what it is AND what it resolves to: the name it
// points at, then the user and secret that name supplies. Naming the credential
// alone would leave the row saying less than the table already does.
func TestHostViewResolvesTheCredential(t *testing.T) {
	m := appWith([]store.Host{{Name: "api", Host: "api.corp", Port: 22,
		Auth: store.AuthCredential, Credential: "shared-deploy"}}, nil)
	m.creds.creds = []store.Credential{{Name: "shared-deploy", User: "deploy",
		Auth: store.AuthPrivateKey, IdentityFile: "~/.ssh/shared"}}

	v := detailView(t, m)
	for _, want := range []string{"credential", "shared-deploy", "deploy", "~/.ssh/shared"} {
		if !strings.Contains(v, want) {
			t.Errorf("the view does not mention %q:\n%s", want, v)
		}
	}
}

// A dangling reference is the most useful thing this popup can report, so it
// says the consequence and not just the name.
func TestHostViewSaysWhenTheCredentialIsGone(t *testing.T) {
	m := appWith([]store.Host{{Name: "api", Host: "api.corp", Port: 22,
		Auth: store.AuthCredential, Credential: "retired"}}, nil)

	v := detailView(t, m)
	if !strings.Contains(v, "retired") || !strings.Contains(v, "cannot connect") {
		t.Errorf("a missing credential should say so and say what it costs:\n%s", v)
	}
}

// The credential popup is the auth half only — a credential has no host, port
// or name of its own to show, and its name is the popup's title.
func TestCredViewIsAuthOnly(t *testing.T) {
	secs := credDetail(store.Credential{Name: "ops", User: "root",
		Auth: store.AuthPassword, Password: "pw"})
	if len(secs) != 1 || secs[0].title != "Auth" {
		t.Fatalf("a credential should render one Auth section, got %+v", secs)
	}
	for _, r := range secs[0].rows {
		if r.label == "Host" || r.label == "Port" || r.label == "Name" {
			t.Errorf("a credential has no %q of its own", r.label)
		}
	}
}

// V is the easter egg, full stop. It used to stand aside wherever a panel had a
// real [V]iew, which meant the letter meant one thing on two surfaces of the
// same tab and the logo on every other. Nothing claims it now, so the surface
// with the most reason to have taken it is the one this checks first.
func TestVIsTheLogoOnEverySurface(t *testing.T) {
	for _, keys := range [][]string{
		{"V"},           // the hosts table, which used to claim it
		{"j", "V"},      // and with a row actually under the cursor
		{"1", "j", "V"}, // the credentials table, which also used to
		{"S", "V"},      // a tab that never claimed it
	} {
		m := pressA(appWith(sample(), nil), keys...)
		if m.detail.isActive() {
			t.Errorf("%v: V must not open a detail float any more", keys)
		}
		if !m.splash.isActive() {
			t.Errorf("%v: V should reveal the logo", keys)
		}
	}
}

// Enter on a host is one float doing both jobs: it says what the row is, and it
// offers the thing Enter was pressed for. The offer is what a separate
// confirmation used to be, and it is at the foot of the answer rather than on
// top of it.
func TestEnterOnAHostShowsItAndOffersConnect(t *testing.T) {
	m := settle(pressA(appWith(sample(), nil), "enter"))
	if !m.detail.isActive() {
		t.Fatal("Enter should open the detail float")
	}
	v := ansi.Strip(m.View())
	h, _ := m.cursorHost()
	if !strings.Contains(v, "Connect to \""+h.Name+"\"?") {
		t.Errorf("the offer should name the host:\n%s", v)
	}
	if !strings.Contains(v, "connect") || !strings.Contains(v, "close") {
		t.Errorf("the hint must disclose both doors:\n%s", v)
	}
	// The facts the old confirmation carried are still on screen — it said the
	// address and the auth method, and this says those and more.
	for _, want := range []string{h.Host, itoa(h.Port), string(h.Auth)} {
		if !strings.Contains(v, want) {
			t.Errorf("the float should still say %q:\n%s", want, v)
		}
	}

	// And the second Enter is the connection.
	m = settle(pressA(m, "enter"))
	if m.detail.isActive() {
		t.Error("committing must take the float down")
	}
	if m.tab != tabSSH {
		t.Errorf("Enter on the offer should hand off to tab [3], got %d", m.tab)
	}
}

// Esc is the whole of "I only wanted to look". Nothing is connected, nothing is
// changed, and the row is where it was.
func TestEscOnTheOfferConnectsNothing(t *testing.T) {
	m := settle(pressA(appWith(sample(), nil), "enter", "esc"))
	if m.detail.isActive() {
		t.Error("Esc should close the float")
	}
	if len(m.ssh.sessions) != 0 {
		t.Error("looking at a host must not connect to it")
	}
	if m.tab != tabPref {
		t.Errorf("Esc should leave the tab alone, got %d", m.tab)
	}
}

// An offer is only made when it can be kept. A host whose credential is gone
// cannot be connected to, so the float shows why and offers nothing — the
// refusal is the red row, said once and where the user is already looking.
func TestABrokenCredentialIsShownAndNotOffered(t *testing.T) {
	m := settle(pressA(appWith([]store.Host{{Name: "api", Host: "api.corp", Port: 22,
		Auth: store.AuthCredential, Credential: "retired"}}, nil), "enter"))
	if !m.detail.isActive() {
		t.Fatal("Enter should still open the float — that is where the reason is")
	}
	v := ansi.Strip(m.View())
	if !strings.Contains(v, "retired") || !strings.Contains(v, "cannot connect") {
		t.Errorf("the float must say which credential and what it costs:\n%s", v)
	}
	if strings.Contains(v, "Connect to") {
		t.Errorf("an offer that cannot be kept must not be made:\n%s", v)
	}
	// And Enter is inert: it belongs to the viewport when there is no offer.
	m = settle(pressA(m, "enter"))
	if len(m.ssh.sessions) != 0 || m.tab != tabPref {
		t.Error("Enter must not connect a host that cannot resolve")
	}
}

// The credentials table gets the same shape, with its own verb: Enter opens the
// read-only answer, and Enter again goes on to the form. Looking no longer
// means opening a thing you can type into.
func TestEnterOnACredentialShowsItAndOffersEdit(t *testing.T) {
	creds := []store.Credential{{Name: "ops", User: "root",
		Auth: store.AuthPrivateKey, IdentityFile: "~/.ssh/id_ed25519"}}
	// "1", "j", "enter" is the nav: credentials selected, keyboard handed to the
	// content. The Enter after that is the one on the row.
	m := settle(pressA(credApp(nil, creds), "1", "j", "enter", "enter"))
	if !m.detail.isActive() {
		t.Fatal("Enter should open the detail float")
	}
	v := ansi.Strip(m.View())
	if !strings.Contains(v, `Edit "ops"?`) {
		t.Errorf("the offer should name the credential:\n%s", v)
	}
	if !strings.Contains(v, "edit") || !strings.Contains(v, "close") {
		t.Errorf("the hint must disclose both doors:\n%s", v)
	}

	m = settle(pressA(m, "enter"))
	if !m.credFormUI.isActive() || m.credFormUI.editing != "ops" {
		t.Fatalf("Enter on the offer should open the edit form, editing %q",
			m.credFormUI.editing)
	}
	// The form REPLACES the float; two views of one row do not stack.
	if m.detail.isActive() {
		t.Error("the detail float should be gone once the form is up")
	}
}

// The offer is a fixed footer, not one more scrollable line: the hint promises
// Enter does something, and a promise that can scroll out of the box is not one.
//
// The screen is SHORT on purpose. At a comfortable height the float shows every
// line it has and there is nothing to scroll, so the same assertion would hold
// whether the offer were pinned or not — it would pass for the wrong reason.
// Sixteen rows is short enough that the content genuinely does not fit.
func TestTheOfferCannotBeScrolledAway(t *testing.T) {
	m := appWith([]store.Host{{Name: "api", Host: "api.corp", Port: 22,
		Auth: store.AuthCredential, Credential: "shared-deploy"}}, nil)
	m.creds.creds = []store.Credential{{Name: "shared-deploy", User: "deploy",
		Auth: store.AuthPrivateKey, IdentityFile: "~/.ssh/shared"}}
	next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 16})
	m = settle(pressA(next.(AppModel), "enter"))

	if len(m.detail.lines()) <= m.detail.visible() {
		t.Fatalf("setup: the float fits, so scrolling proves nothing (%d lines, %d visible)",
			len(m.detail.lines()), m.detail.visible())
	}
	for range 40 { // all the way to the bottom and then some
		m = pressA(m, "j")
	}
	if v := ansi.Strip(m.View()); !strings.Contains(v, `Connect to "api"?`) {
		t.Errorf("the offer scrolled out of its own box:\n%s", v)
	}
}
