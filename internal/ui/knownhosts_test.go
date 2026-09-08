package ui

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/vulcanshen/sshu/internal/remote"
	"github.com/vulcanshen/sshu/internal/store"
)

func kb64(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }

// knownUIFixture: a section comment over a run, a bracketed port, a revoked
// key, and a hashed name whose host cannot be read back.
func knownUIFixture() string {
	return fmt.Sprintf(`# work
[198.51.100.10]:2222 ssh-ed25519 %s laptop
gw.example.com ssh-rsa %s
@revoked old.example.com ssh-ed25519 %s
|1|%s|%s ecdsa-sha2-nistp256 %s
`, kb64("key-one"), kb64("key-two"), kb64("key-three"), kb64("salt"), kb64("hash"), kb64("key-four"))
}

// knownApp is the manage tab with the KnownHosts panel holding the keyboard,
// backed by a real file — the only way to check what sshu actually wrote.
func knownApp(t *testing.T, raw string) (AppModel, string) {
	t.Helper()
	p := filepath.Join(t.TempDir(), "known_hosts")
	if err := os.WriteFile(p, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := store.LoadKnownHostsFrom(p)
	if err != nil {
		t.Fatal(err)
	}
	m := New(nil, nil, store.DefaultConfig()).
		WithKnownHosts(f, func(g store.KnownHostsFile) (store.KnownHostsFile, error) {
			return store.SaveKnownHostsTo(p, g)
		})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	// nav → hosts → credentials → config → known hosts, then into the content.
	return pressA(settle(next.(AppModel)), "1", "j", "j", "j", "enter"), p
}

func TestKnownHostsListsTheKeysAndCountsTheHashedOnes(t *testing.T) {
	m, _ := knownApp(t, knownUIFixture())
	v := ansi.Strip(m.View())
	for _, want := range []string{"[2] KnownHosts", "[198.51.100.10]:2222", "gw.example.com", "ed25519"} {
		if !strings.Contains(v, want) {
			t.Errorf("the table should show %q:\n%s", want, v)
		}
	}
	// A hashed name is unreadable BY DESIGN, so the panel says so rather than
	// printing the HMAC and letting it look like a rendering fault.
	if !strings.Contains(v, "(hashed)") {
		t.Errorf("a hashed row must say what it is:\n%s", v)
	}
	if st := m.prefStatus(); !strings.Contains(st, "1 hashed") {
		t.Errorf("the status should count them, got %q", st)
	}
}

func TestARevokedKeyIsNotShownAsATrustedOne(t *testing.T) {
	m, _ := knownApp(t, knownUIFixture())
	// It inverts what the row means, so the marker rides in front of the name
	// rather than being left to the detail float.
	if v := ansi.Strip(m.View()); !strings.Contains(v, "@revoked old.example.com") {
		t.Errorf("the marker must be on the row:\n%s", v)
	}
}

func TestEnterOnAKeyShowsTheWholeFingerprintAndOffersTheRename(t *testing.T) {
	m, _ := knownApp(t, knownUIFixture())
	m = pressA(m, "enter")
	if !m.detail.isActive() {
		t.Fatal("Enter should open the detail float")
	}
	body := ansi.Strip(m.detail.view())
	full := store.KnownHostEntry{Type: "ssh-ed25519", Key: kb64("key-one")}.Fingerprint()
	// The table can only fit a slice of it; comparing a fingerprint means
	// reading all of it, so all of it has to be somewhere.
	if !strings.Contains(body, full) {
		t.Errorf("the float should carry the whole fingerprint %q:\n%s", full, body)
	}
	if m.detail.action != detailEditKnown {
		t.Errorf("the foot should offer the rename, got action %d", m.detail.action)
	}

	m = pressA(m, "enter")
	if !m.input.isActive() || m.detail.isActive() {
		t.Fatalf("the box should replace the float, input=%v detail=%v",
			m.input.isActive(), m.detail.isActive())
	}
}

// The key is never editable, so "edit" here asks exactly one question — which
// makes it the input class, not a form (§6.1).
func TestEditIsOneQuestionAndRewritesOnlyTheNameField(t *testing.T) {
	m, p := knownApp(t, knownUIFixture())
	m = pressA(m, "E")
	if !m.input.isActive() {
		t.Fatal("E should open the input box")
	}
	if m.input.value != "[198.51.100.10]:2222" {
		t.Errorf("it should start from the names it has, got %q", m.input.value)
	}

	m.input.value = "laptop.example.com"
	m = pressA(m, "enter")

	got := diskText(t, p)
	want := strings.Replace(knownUIFixture(), "[198.51.100.10]:2222", "laptop.example.com", 1)
	if got != want {
		t.Errorf("only the name field should have moved:\nwant %q\ngot  %q", want, got)
	}
}

func TestDeleteAsksWithTheFingerprintThenForgetsTheKey(t *testing.T) {
	m, p := knownApp(t, knownUIFixture())
	m = pressA(m, "X")
	if !m.confirm.isActive() {
		t.Fatal("X should ask first")
	}
	lines := strings.Join(m.confirm.lines, " ")
	full := store.KnownHostEntry{Type: "ssh-ed25519", Key: kb64("key-one")}.Fingerprint()
	if !strings.Contains(lines, full) {
		t.Errorf("the question should show what is being forgotten, got %q", lines)
	}
	if !strings.Contains(lines, "ask again") {
		t.Errorf("it should say what happens next, got %q", lines)
	}

	m = pressA(m, "enter")
	got := diskText(t, p)
	if strings.Contains(got, "198.51.100.10") {
		t.Errorf("the entry should be gone:\n%s", got)
	}
	// UNLIKE the Config panel: a comment here labels a run of entries, so it
	// stays when one of them goes.
	if !strings.HasPrefix(got, "# work\n") {
		t.Errorf("the section comment must survive:\n%s", got)
	}
}

func TestAHashedRowCanStillBeForgotten(t *testing.T) {
	m, p := knownApp(t, knownUIFixture())
	// Down to the hashed one. Its name cannot be read, but the row is still a
	// row: deleting it is the whole reason somebody would come here.
	m = pressA(m, "j", "j", "j")
	e, _, ok := m.cursorKnown()
	if !ok || !e.Hashed() {
		t.Fatalf("setup: expected the hashed row, got %+v", e)
	}
	m = pressA(m, "X", "enter")
	if got := diskText(t, p); strings.Contains(got, "|1|") {
		t.Errorf("the hashed entry should be gone:\n%s", got)
	}
}

// [A] fetches. This drives the UI half of that: the form goes into its waiting
// state, the answer arrives as a message, and NOTHING is written until the
// fingerprint has been shown and accepted.
func TestAddFetchesThenAsksBeforeWritingAnything(t *testing.T) {
	m, p := knownApp(t, knownUIFixture())
	before := diskText(t, p)

	m = pressA(m, "A")
	if !m.knownAddUI.isActive() {
		t.Fatal("A should open the fetch form")
	}
	m.knownAddUI.fields[kfHost].value = "new.example.com"
	m = pressA(m, "enter")
	if !m.knownAddUI.scanning {
		t.Fatal("Enter should start the fetch and say so")
	}
	if v := ansi.Strip(m.View()); !strings.Contains(v, "asking new.example.com:22") {
		t.Errorf("the wait must be visible:\n%s", v)
	}

	key := remote.HostKey{Type: "ssh-ed25519", Key: kb64("fetched"),
		Fingerprint: store.KnownHostEntry{Key: kb64("fetched")}.Fingerprint()}
	next, _ := m.Update(hostKeyScannedMsg{host: "new.example.com", port: 22, key: key})
	m = settle(next.(AppModel))

	if !m.confirm.isActive() {
		t.Fatal("the key should arrive as a question, not as a write")
	}
	if lines := strings.Join(m.confirm.lines, " "); !strings.Contains(lines, key.Fingerprint) {
		t.Errorf("the question must show the fingerprint, got %q", lines)
	}
	if diskText(t, p) != before {
		t.Fatal("nothing may be written before the question is answered")
	}

	m = pressA(m, "enter")
	if got := diskText(t, p); got != before+"new.example.com ssh-ed25519 "+kb64("fetched")+"\n" {
		t.Errorf("the accepted key should be appended:\n%q", got)
	}
}

func TestANonDefaultPortIsWrittenTheWaySshWritesIt(t *testing.T) {
	m, p := knownApp(t, knownUIFixture())
	m = pressA(m, "A")
	m.knownAddUI.fields[kfHost].value = "odd.example.com"
	m.knownAddUI.fields[kfPort].value = "2222"
	m = pressA(m, "enter")

	key := remote.HostKey{Type: "ssh-ed25519", Key: kb64("odd")}
	next, _ := m.Update(hostKeyScannedMsg{host: "odd.example.com", port: 2222, key: key})
	m = settle(next.(AppModel))
	m = pressA(m, "enter")

	// Bracketed with the port, or ssh will never match the line it just wrote.
	if got := diskText(t, p); !strings.Contains(got, "[odd.example.com]:2222 ssh-ed25519 ") {
		t.Errorf("wrong address form:\n%s", got)
	}
}

func TestAFailedFetchStaysInTheFormAndSaysWhy(t *testing.T) {
	m, p := knownApp(t, knownUIFixture())
	before := diskText(t, p)

	m = pressA(m, "A")
	m.knownAddUI.fields[kfHost].value = "nowhere.example.com"
	m = pressA(m, "enter")

	next, _ := m.Update(hostKeyScannedMsg{host: "nowhere.example.com", port: 22,
		err: fmt.Errorf("nowhere.example.com:22: connection refused")})
	m = settle(next.(AppModel))

	if !m.knownAddUI.isActive() || m.knownAddUI.scanning {
		t.Fatalf("the form should still be there and no longer waiting, active=%v scanning=%v",
			m.knownAddUI.isActive(), m.knownAddUI.scanning)
	}
	if !strings.Contains(m.knownAddUI.err, "connection refused") {
		t.Errorf("the reason should be in the form, got %q", m.knownAddUI.err)
	}
	// And it is READABLE. Two fields make a narrow box; a network error is the
	// longest thing that ever lands in it, and half a reason is not one.
	if v := ansi.Strip(m.knownAddUI.view()); !strings.Contains(v, "connection refused") {
		t.Errorf("the box must widen for its error:\n%s", v)
	}
	if diskText(t, p) != before {
		t.Error("a failed fetch must write nothing")
	}
}

// The absence of [D]uplicate is a decision, not an oversight: it would mean
// trusting the same key under a second name, which the comma-separated name
// field already does in one line.
func TestThereIsNoDuplicateActionOnKnownHosts(t *testing.T) {
	m, _ := knownApp(t, knownUIFixture())
	for _, it := range m.menuItems() {
		if it.key == "D" {
			t.Errorf("known hosts should not offer %q", it.label)
		}
	}
	for _, a := range knownActions {
		if a.key == "D" {
			t.Errorf("knownActions should not declare %q", a.label)
		}
	}
}

func TestASaveRefusedByAnotherWriterPutsTheirFileOnScreen(t *testing.T) {
	m, p := knownApp(t, knownUIFixture())

	// ssh appends here on every first connect; this is the routine case.
	theirs := knownUIFixture() + "fresh.example.com ssh-ed25519 " + kb64("fresh") + "\n"
	if err := os.WriteFile(p, []byte(theirs), 0o600); err != nil {
		t.Fatal(err)
	}

	m = pressA(m, "X", "enter")
	if got := diskText(t, p); got != theirs {
		t.Fatalf("their write must survive:\n%s", got)
	}
	if n := len(m.log.entries); n == 0 ||
		!strings.Contains(m.log.entries[n-1].msg, "changed on disk") {
		t.Errorf("the refusal must be recorded, log is %+v", m.log.entries)
	}
	if len(m.known.file.Entries) != 5 {
		t.Errorf("the panel should be showing their file, got %d entries",
			len(m.known.file.Entries))
	}
}

func TestKnownHostsTableHeaderAndRowsAlign(t *testing.T) {
	e := store.KnownHostEntry{Hosts: "[198.51.100.10]:2222", Type: "ecdsa-sha2-nistp256",
		Key: kb64("some-key-material"), Comment: "laptop"}
	var m knownModel
	for w := 12; w <= 140; w++ {
		host, typ, fp := knownCols(w)
		m.w, m.h = w+2, 10
		head := dispW(m.tableBody(w, 3)[0])
		row := dispW(m.row(e, false, host, typ, fp, w))
		sel := dispW(m.row(e, true, host, typ, fp, w))
		if head != w || row != w || sel != w {
			t.Errorf("w=%d: header=%d row=%d selected=%d, all should be %d", w, head, row, sel, w)
		}
	}
}

func TestShortKeyTypeKeepsWhatDistinguishesThem(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"ssh-ed25519", "ed25519"},
		{"ssh-rsa", "rsa"},
		{"ssh-dss", "dsa"},
		{"ecdsa-sha2-nistp256", "ecdsa-256"},
		{"ecdsa-sha2-nistp521", "ecdsa-521"},
		{"sk-ssh-ed25519@openssh.com", "sk-ed25519"},
	} {
		if got := shortKeyType(tc.in); got != tc.want {
			t.Errorf("shortKeyType(%q) = %q, want %q", tc.in, got, tc.want)
		}
		if len(tc.want) > knownTypeW {
			t.Errorf("%q is %d cells, wider than the column's %d", tc.want, len(tc.want), knownTypeW)
		}
	}
}
