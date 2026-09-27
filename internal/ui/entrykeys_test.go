package ui

import (
	"strings"
	"testing"
)

// The audit, as a table: every kind of float, and what Space does on it.
//
// Space closes the Space menu it opened — an entry key that only works one way
// is a trap. On every other float it does nothing (tdp K5, F6): a confirm that
// Space dismissed would be Space doing Esc's job. On a float being typed into,
// a space is a space (tdp K8).
//
// Listing them all rather than the one that was reported is the point: a new
// float that gets it wrong fails here.
func TestSpaceClosesOnlyTheSpaceMenu(t *testing.T) {
	inSFTP := func(t *testing.T, keys ...string) AppModel {
		t.Helper()
		m := sftpFixture(t, 100, 26)
		m.sftp.focus = panelLeftFiles
		return pressA(m, keys...)
	}
	onHosts := func(keys ...string) func(*testing.T) AppModel {
		return func(t *testing.T) AppModel {
			t.Helper()
			return pressA(appWith(sample(), nil), keys...)
		}
	}

	for _, tc := range []struct {
		name   string
		open   func(*testing.T) AppModel
		live   func(AppModel) bool
		closes bool // only the Space menu
	}{
		{"space menu", onHosts(" "),
			func(m AppModel) bool { return m.spaceMenu.isActive() }, true},
		{"help", onHosts("?"),
			func(m AppModel) bool { return m.help.isActive() }, false},
		{"confirm", onHosts("X"),
			func(m AppModel) bool { return m.confirm.isActive() }, false},
		{"host picker", func(t *testing.T) AppModel { return inSFTP(t, "H") },
			func(m AppModel) bool { return m.hostPicker.isActive() }, false},
		{"transfers", func(t *testing.T) AppModel { return inSFTP(t, "J") },
			func(m AppModel) bool { return m.transfersUI.isActive() }, false},

		// Typed into: the space lands as a character (TestSpaceTypesIntoTheRenameBox).
		{"host form", onHosts("A"),
			func(m AppModel) bool { return m.form.isActive() }, false},
		{"file picker", openPicker,
			func(m AppModel) bool { return m.picker.isActive() }, false},
		{"rename", func(t *testing.T) AppModel { return inSFTP(t, "r") },
			func(m AppModel) bool { return m.input.isActive() }, false},
	} {
		m := tc.open(t)
		if !tc.live(m) {
			t.Fatalf("%s: setup did not open it", tc.name)
		}
		m = pressA(m, " ")
		switch {
		case tc.closes && tc.live(m):
			t.Errorf("%s: Space should have closed it", tc.name)
		case !tc.closes && !tc.live(m):
			t.Errorf("%s: Space must not close it (tdp K5)", tc.name)
		}
	}
}

// Space on a float stacked above the Space menu does nothing either — not even
// to the menu underneath, which is not the float being looked at.
func TestSpaceOnAFloatAboveTheMenuDoesNothing(t *testing.T) {
	m := pressA(appWith(sample(), nil), " ", "X") // delete confirm, from the menu
	if !m.confirm.isActive() || !m.spaceMenu.isActive() {
		t.Fatal("setup: the confirm should stand on the menu")
	}
	m = pressA(m, " ")
	if !m.confirm.isActive() || !m.spaceMenu.isActive() {
		t.Error("Space must leave both the confirm and the menu under it alone")
	}
}

// And where Space is a character, it really lands as one.
func TestSpaceTypesIntoTheRenameBox(t *testing.T) {
	m := sftpFixture(t, 100, 26)
	m.sftp.focus = panelLeftFiles
	m = pressA(m, "r", " ")

	if !m.input.isActive() {
		t.Fatal("Space closed the rename box instead of typing into it")
	}
	if !strings.HasSuffix(m.input.value, " ") {
		t.Errorf("the space did not land: %q", m.input.value)
	}
}

// The other entry key toggles the same way, and §A.2 promises it from ANY
// surface — including from on top of the menu a lost user just opened.
func TestQuestionMarkTogglesTheHelp(t *testing.T) {
	m := pressA(appWith(sample(), nil), "?")
	if !m.help.isActive() {
		t.Fatal("? should open the help")
	}
	m = pressA(m, "?")
	if m.help.isActive() {
		t.Error("? should close it again")
	}

	m = pressA(appWith(sample(), nil), " ", "?")
	if !m.help.isActive() {
		t.Fatal("? must reach the help from inside the Space menu")
	}
	if !m.spaceMenu.isActive() {
		t.Error("the menu should still be underneath it (§6.4)")
	}
	if m.help.layer < 2 {
		t.Errorf("the help should stack above the menu, layer=%d", m.help.layer)
	}

	// Space does nothing on the help — it is not the Space menu (tdp K5) —
	// and ? closes it, leaving the menu standing. Then Space closes the menu.
	m = pressA(m, " ")
	if !m.help.isActive() || !m.spaceMenu.isActive() {
		t.Error("Space on the help must not close anything")
	}
	m = pressA(m, "?")
	if m.help.isActive() {
		t.Error("? should have closed the help")
	}
	if !m.spaceMenu.isActive() {
		t.Error("...and left the menu standing")
	}
	m = pressA(m, " ")
	if m.spaceMenu.isActive() {
		t.Error("Space should close the menu once it is on top again")
	}
}

// A question mark typed into a field is a question mark.
func TestQuestionMarkIsACharacterInAForm(t *testing.T) {
	m := pressA(appWith(sample(), nil), "A")
	if !m.form.isActive() {
		t.Fatal("setup: the form should be open")
	}
	m = pressA(m, "?")
	if m.help.isActive() {
		t.Error("? opened the help from inside a field being typed into")
	}
	if !m.form.isActive() {
		t.Error("the form closed")
	}
}
