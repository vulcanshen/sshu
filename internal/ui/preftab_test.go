package ui

import (
	"errors"
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
	if row := ansi.Strip(m.prefNavRow(prefLogs, 16, false)); !strings.Contains(row, "1") {
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

// The category headers are app structure — the panel-border blue while the
// nav holds the keyboard — and the items under them are not. Hand the keyboard
// to the content and the whole panel goes down one register together: no blue
// left anywhere, dim items, and a cursor that is still a bar because it is
// what says which section [2] is showing.
//
// The rows are sampled ONE AT A TIME rather than out of prefNav's output: the
// border wears these same colours on every line and would answer for them.
func TestNavDimsWhenTheContentHasTheKeyboard(t *testing.T) {
	withColour(t)
	const blueHex = "#89b4fa"
	if string(focusColor) != blueHex {
		t.Fatalf("the header accent must be the structure blue %s, got %s", blueHex, focusColor)
	}
	blue, quiet := ansiOf(t, focusColor), ansiOf(t, borderDim)

	if h := prefNavHead("Events", 16, true); !strings.Contains(h, blue) {
		t.Errorf("a focused category header should wear the structure blue, got %q", h)
	}
	if h := prefNavHead("Events", 16, false); strings.Contains(h, blue) || !strings.Contains(h, quiet) {
		t.Errorf("an unfocused header should recede to the border's dim, got %q", h)
	}

	m := appWith(sample(), nil) // the tab opens on the content: the nav is unfocused
	if row := m.prefNavRow(prefCreds, 16, true); !strings.Contains(row, ansiOf(t, textColor)) {
		t.Errorf("a focused item should be full text colour, got %q", row)
	}
	if row := m.prefNavRow(prefCreds, 16, false); !strings.Contains(row, ansiOf(t, dimColor)) {
		t.Errorf("an unfocused item should be dim, got %q", row)
	}
	if row := m.prefNavRow(prefHosts, 16, false); !strings.Contains(row, ansiBgOf(t, borderDim)) {
		t.Errorf("the unfocused cursor keeps its bar, one register down, got %q", row)
	}
	// And the panel as a whole, border included, has no blue left in it.
	if nav := m.prefNav(prefLeftW, 20); strings.Contains(nav, blue) {
		t.Error("nothing in an unfocused nav should wear the structure blue")
	}
}

// The log's one action, both ways round: the Space menu row and the letter it
// names. It asks first, then empties the panel AND the file — and it says so
// in a toast rather than in the log it just cleared.
func TestClearLogsAsksThenEmptiesPanelAndFile(t *testing.T) {
	m := appWith(sample(), nil)
	cleared := 0
	m.log.clearSink = func() error { cleared++; return nil }
	m.log.errorf("something broke")
	m.log.info("and then something else")

	m = pressA(m, "1", "j", "j", "enter") // nav → logs → the content
	if m.pref.item != prefLogs || m.pref.focus != panelPrefContent {
		t.Fatal("setup: expected the logs content focused")
	}

	// The menu names the key, and the key is what the menu commits.
	label := ""
	for _, it := range m.menuItems() {
		if it.key == "C" {
			label = bracketHotkey(it.label, it.key)
		}
	}
	if label == "" {
		t.Fatal("the logs menu must offer a way to clear the log")
	}
	if label != "[C]lear logs" {
		t.Errorf("the row should read [C]lear logs, got %q", label)
	}

	m = pressA(m, "C")
	if !m.confirm.isActive() {
		t.Fatal("C should ask first — the log is the only record of what happened")
	}
	if lines := strings.Join(m.confirm.lines, " "); !strings.Contains(lines, "2 entries") ||
		!strings.Contains(lines, "applogs.yaml") {
		t.Errorf("the confirm must count the cost and name the file, got %q", lines)
	}

	m = pressA(m, "enter")
	if n := len(m.log.entries); n != 0 {
		t.Errorf("the panel should be empty, %d entries left", n)
	}
	if cleared != 1 {
		t.Errorf("applogs.yaml should have been emptied exactly once, got %d", cleared)
	}
	if v := ansi.Strip(m.View()); !strings.Contains(v, "Cleared 2 entries") {
		t.Errorf("the toast is where the news goes:\n%s", v)
	}
	if !strings.Contains(ansi.Strip(m.View()), "Nothing has happened yet") {
		t.Error("an emptied log should show its empty state")
	}
}

// A file that refuses to be emptied keeps its entries: a log that says it was
// cleared and is full again after a restart is the worse of the two lies.
func TestClearLogsKeepsEverythingWhenTheFileRefuses(t *testing.T) {
	m := appWith(sample(), nil)
	m.log.clearSink = func() error { return errors.New("applogs.yaml: read-only") }
	m.log.info("one thing happened")

	m = pressA(m, "1", "j", "j", "enter", "C", "enter")
	if len(m.log.entries) != 1 {
		t.Fatalf("the entries must survive a refused clear, got %d", len(m.log.entries))
	}
	if v := ansi.Strip(m.View()); !strings.Contains(v, "read-only") {
		t.Errorf("the refusal must be said out loud:\n%s", v)
	}
}

// Nothing recorded, nothing to clear: no menu row and no key. The same silence
// the hosts table keeps where there is no row to delete.
func TestAnEmptyLogOffersNothingToClear(t *testing.T) {
	m := appWith(sample(), nil)
	m = pressA(m, "1", "j", "j", "enter")
	if len(m.log.entries) != 0 {
		t.Fatalf("setup: the log should be empty, has %d", len(m.log.entries))
	}
	for _, it := range m.menuItems() {
		if it.key == "C" {
			t.Error("an empty log must not offer to be cleared")
		}
	}
	if m = pressA(m, "C"); m.confirm.isActive() {
		t.Error("C on an empty log must not ask anything")
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
	m = pressA(m, "X") // delete moved off D (§11.35)
	if !m.confirm.isActive() {
		t.Fatal("X should ask first")
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
	m = pressA(m, "1", "j", "enter", "X")
	if !m.confirm.isActive() {
		t.Fatal("X should ask")
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
// a filled one saves like every other field, and Backspace clears the whole
// line — a picked path is replaced, not shaved letter by letter.
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

	m = fillCredForm(m, "picker-cred")
	m.credFormUI.focus = cIdentity
	m = pressA(m, "enter")
	if m.picker.isActive() {
		t.Fatal("Enter on a filled path field must not browse")
	}
	// With the rest of the form filled in, Enter on a filled path row is save
	// like everywhere else (§11.34).
	if !m.credFormUI.submitted || m.credFormUI.err != "" {
		t.Fatalf("Enter on a filled path field should submit, submitted=%v err=%q",
			m.credFormUI.submitted, m.credFormUI.err)
	}

	// The save closed the form, so Backspace gets a fresh one to clear.
	m = pressA(m, "A")
	m.credFormUI.fields[cIdentity].value = "~/.ssh/id_ed25519"
	m.credFormUI.fields[cIdentity].caret = 17
	m.credFormUI.focus = cIdentity
	m = pressA(m, "backspace")
	if got := m.credFormUI.fields[cIdentity].value; got != "" {
		t.Fatalf("Backspace should clear the whole line, left %q", got)
	}
}
