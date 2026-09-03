package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/vulcanshen/sshu/internal/store"
)

// detailView opens [V]iew on the hosts table and returns the frame it drew.
func detailView(t *testing.T, m AppModel) string {
	t.Helper()
	m = settle(pressA(m, "V"))
	if !m.detail.isActive() {
		t.Fatal("V should open the detail popup")
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

// V is the easter egg everywhere EXCEPT where a panel has a real use for it.
// The two halves are one test because the bug is the pair coming apart: a
// splash that never yields shadows the row, and one that always yields loses
// the egg.
func TestVGoesToThePanelThatClaimsItAndToTheLogoOtherwise(t *testing.T) {
	m := pressA(appWith(sample(), nil), "V")
	if !m.detail.isActive() {
		t.Error("V on the hosts table should open the detail popup")
	}
	if m.splash.isActive() {
		t.Error("the easter egg must stand aside for a panel that claims V")
	}

	// The ssh tab claims nothing, so the egg is still there.
	m = pressA(appWith(sample(), nil), "S", "V")
	if !m.splash.isActive() {
		t.Error("V where no panel wants it should still reveal the logo")
	}

	// An empty table has nothing to view, so the key is free again.
	m = pressA(appWith(nil, nil), "V")
	if m.detail.isActive() {
		t.Error("there is no row to view on an empty table")
	}
	if !m.splash.isActive() {
		t.Error("with nothing to view, V belongs to the logo again")
	}
}
