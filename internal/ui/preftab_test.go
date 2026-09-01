package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/vulcanshen/sshu/internal/store"
)

// The nav cursor IS the choice: moving it swaps the content immediately, and
// it wraps like every other list.
func TestPrefNavSwapsContentAsTheCursorMoves(t *testing.T) {
	m := pressA(appWith(sample(), nil), "1") // the digit puts the keyboard on the nav
	if m.pref.focus != panelPrefNav {
		t.Fatal("1 should focus the nav")
	}

	m = pressA(m, "j")
	if v := ansi.Strip(m.View()); !strings.Contains(v, "[2] Credentials") ||
		!strings.Contains(v, "No credentials yet") {
		t.Errorf("moving to credentials should show them:\n%s", v)
	}
	m = pressA(m, "j")
	if v := ansi.Strip(m.View()); !strings.Contains(v, "[2] Logs") {
		t.Errorf("moving to logs should show them:\n%s", v)
	}
	m = pressA(m, "j")
	if v := ansi.Strip(m.View()); !strings.Contains(v, "[2] Hosts") {
		t.Errorf("the nav should wrap back to hosts — the masked Operation items take no cursor stops:\n%s", v)
	}
}

// The tab opens ON its content — the hosts table, where the old hosts tab put
// you — and 1 / 2 / Tab / Enter move the keyboard between the two panels.
func TestPrefKeyboardMovesBetweenNavAndContent(t *testing.T) {
	m := appWith(sample(), nil)
	if m.pref.focus != panelPrefContent {
		t.Fatal("the tab should open on its content")
	}
	m = pressA(m, "1")
	if m.pref.focus != panelPrefNav {
		t.Fatal("1 should focus the nav")
	}
	m = pressA(m, "enter")
	if m.pref.focus != panelPrefContent {
		t.Fatal("Enter on the nav should hand the keyboard to the content")
	}
	m = pressA(m, "tab")
	if m.pref.focus != panelPrefNav {
		t.Fatal("Tab should toggle back to the nav")
	}
	m = pressA(m, "2")
	if m.pref.focus != panelPrefContent {
		t.Fatal("2 should focus the content")
	}
}

// Unread errors are disclosed twice — a count on the nav's logs row and one in
// the footer — and putting the logs on screen is what clears them.
func TestUnreadErrorsAreDisclosedAndClearedByLooking(t *testing.T) {
	m := appWith(sample(), nil)
	m.log.errorf("something broke")

	if foot := ansi.Strip(m.footer()); !strings.Contains(foot, "1 unread error") {
		t.Errorf("the footer must count unread errors, got %q", foot)
	}
	if row := ansi.Strip(m.prefNavRow(prefLogs, 16)); !strings.Contains(row, "1") {
		t.Errorf("the nav's logs row must carry the count, got %q", row)
	}

	m = pressA(m, "1", "j", "j") // nav → credentials → logs: now on screen
	if m.log.unreadErrors() != 0 {
		t.Errorf("having the logs on screen is reading them, unread = %d", m.log.unreadErrors())
	}
	if foot := ansi.Strip(m.footer()); strings.Contains(foot, "unread") {
		t.Errorf("nothing unread, nothing to disclose, got %q", foot)
	}
}

// The frame invariant holds across the pref tab's shapes: both foci, all
// three sections, wide and narrow.
func TestPrefTabPreservesFrame(t *testing.T) {
	for _, sz := range [][2]int{{100, 26}, {70, 18}, {56, 14}, {34, 10}} {
		w, h := sz[0], sz[1]
		for _, keys := range [][]string{
			{}, {"1"}, {"1", "j"}, {"1", "j", "j"},
			{"1", "j", "enter"}, {"2"},
			{"1", "j", "j", "j"}, {"1", "G", "enter"},
		} {
			m := pressA(sized(sample(), w, h), keys...)
			got := m.View()
			lines := strings.Split(got, "\n")
			if len(lines) != h {
				t.Fatalf("%dx%d %v: %d lines, want %d", w, h, keys, len(lines), h)
			}
			for i, l := range lines {
				if lw := lipgloss.Width(l); lw != w {
					t.Errorf("%dx%d %v line %d: width %d, want %d\n%q",
						w, h, keys, i, lw, w, l)
				}
			}
		}
	}
}

// The category headers are app structure — the panel-border blue, not the
// dim of secondary text — and the items under them are not.
func TestNavHeadersAreBlue(t *testing.T) {
	withColour(t)
	const blueHex = "#89b4fa"
	if string(focusColor) != blueHex {
		t.Fatalf("the header accent must be the structure blue %s, got %s", blueHex, focusColor)
	}
	blue := ansiOf(t, focusColor)

	// The nav is sampled UNFOCUSED (the tab opens on the content): a focused
	// panel border wears the same blue, which would put it on every row.
	m := appWith(sample(), nil)
	var header, item string
	for _, row := range strings.Split(m.prefNav(prefLeftW, 20), "\n") {
		if strings.Contains(row, "Events") {
			header = row
		}
		if strings.Contains(row, "Credentials") {
			item = row
		}
	}
	if header == "" || item == "" {
		t.Fatal("could not find the Events header / Credentials item rows")
	}
	if !strings.Contains(header, blue) {
		t.Error("a category header should wear the structure blue")
	}
	if strings.Contains(item, blue) {
		t.Error("an item row must not wear the header's blue")
	}
}

// One end-to-end pass over the credentials section: add through the form,
// edit with Enter, delete with a confirm — the list, the save func, the table
// and the log all see each step.
func TestCredentialAddEditDelete(t *testing.T) {
	var saved [][]store.Credential
	m := appWith(sample(), nil)
	m.saveCreds = func(l []store.Credential) error {
		saved = append(saved, append([]store.Credential(nil), l...))
		return nil
	}

	// Into the credentials section, and open the form.
	m = pressA(m, "1", "j", "enter")
	if m.pref.item != prefCreds || m.pref.focus != panelPrefContent {
		t.Fatal("setup: expected the credentials content focused")
	}
	m = pressA(m, "A")
	if !m.credFormUI.isActive() {
		t.Fatal("A should open the credential form")
	}

	// Name, user, auth → password, the password itself, save.
	m = typeText(m, "ops")
	m = pressA(m, "tab")
	m = typeText(m, "root")
	m = pressA(m, "tab", "left") // the auth toggle: privatekey → password
	m = pressA(m, "tab")
	m = typeText(m, "hunter2")
	m = pressA(m, "enter")

	if len(saved) != 1 || len(saved[0]) != 1 {
		t.Fatalf("expected one save of one credential, got %v", saved)
	}
	if c := saved[0][0]; c.Name != "ops" || c.User != "root" ||
		c.Auth != store.AuthPassword || c.Password != "hunter2" {
		t.Fatalf("saved the wrong credential: %+v", c)
	}
	if v := ansi.Strip(m.View()); !strings.Contains(v, "ops") {
		t.Errorf("the new row is not in the table:\n%s", v)
	}
	joined := ""
	for _, e := range m.log.entries {
		joined += e.msg + "\n"
	}
	if !strings.Contains(joined, `credential "ops" added`) {
		t.Errorf("adding a credential is an event, log has:\n%s", joined)
	}

	// Enter edits, prefilled.
	m = pressA(m, "enter")
	if !m.credFormUI.isActive() || m.credFormUI.fields[cName].value != "ops" {
		t.Fatal("Enter should open the edit form prefilled")
	}
	// Two Escapes: the save toast is still up and Esc pops one float at a
	// time, topmost first.
	m = pressA(m, "esc", "esc")

	// Delete asks, then rewrites.
	m = pressA(m, "D")
	if !m.confirm.isActive() {
		t.Fatal("D should ask first")
	}
	m = pressA(m, "enter")
	if len(saved) != 2 || len(saved[1]) != 0 {
		t.Fatalf("expected a second save with no credentials, got %v", saved)
	}
	if v := ansi.Strip(m.View()); !strings.Contains(v, "No credentials yet") {
		t.Errorf("the empty state should be back:\n%s", v)
	}
}

// Deleting a credential that hosts still name says so, with a count — those
// hosts break quietly otherwise.
func TestDeleteCredentialWarnsAboutReferencingHosts(t *testing.T) {
	hosts := []store.Host{
		{Name: "web", Host: "h", Port: 22, Auth: store.AuthCredential, Credential: "ops"},
		{Name: "db", Host: "h2", Port: 22, Auth: store.AuthCredential, Credential: "ops"},
	}
	m := appWith(hosts, nil)
	m.creds.creds = []store.Credential{{Name: "ops", User: "root", Auth: store.AuthPassword}}
	m = pressA(m, "1", "j", "enter", "D")
	if !m.confirm.isActive() {
		t.Fatal("D should ask")
	}
	found := false
	for _, l := range m.confirm.lines {
		if strings.Contains(l, "2 hosts reference it") {
			found = true
		}
	}
	if !found {
		t.Errorf("the confirm must name the fallout, lines: %q", m.confirm.lines)
	}
}

// The credential form's path field: Enter on the empty field browses, Enter on
// a filled one moves on, and Backspace clears the whole line — a picked path
// is replaced, not shaved letter by letter.
func TestCredIdentityFieldEnterAndBackspace(t *testing.T) {
	m := appWith(nil, nil)
	m = pressA(m, "1", "j", "enter", "A")
	if !m.credFormUI.isActive() {
		t.Fatal("setup: no form")
	}
	m.credFormUI.focus = cIdentity // auth defaults to privatekey, so it is live

	m = pressA(m, "enter")
	if !m.picker.isActive() {
		t.Fatal("Enter on the empty path field should open the picker")
	}
	m = pressA(m, "esc") // back to the form

	m.credFormUI.fields[cIdentity].value = "~/.ssh/id_ed25519"
	m.credFormUI.fields[cIdentity].caret = 5
	m = pressA(m, "enter")
	if m.picker.isActive() {
		t.Fatal("Enter on a filled path field must not browse")
	}
	if m.credFormUI.focus == cIdentity {
		t.Fatal("Enter on a filled path field should move to the next field")
	}

	m.credFormUI.focus = cIdentity
	m = pressA(m, "backspace")
	if got := m.credFormUI.fields[cIdentity].value; got != "" {
		t.Fatalf("Backspace should clear the whole line, left %q", got)
	}
}
