package ui

import (
	"strings"
	"testing"
)

// The audit, as a table: every float in the app, and whether Space dismisses it.
//
// An entry key that only works one way is a trap — the user reaches for the same
// key to get out, nothing happens, and the surface looks stuck. The one
// exception is a float being typed into, where a space is a space (§4.5).
//
// A new float that forgets either half of this fails here, which is the point of
// listing them all rather than testing the one that was reported.
func TestSpaceDismissesEveryFloat(t *testing.T) {
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
		name string
		open func(*testing.T) AppModel
		live func(AppModel) bool
		text bool // Space is a character here, not a key
	}{
		{"space menu", onHosts(" "),
			func(m AppModel) bool { return m.spaceMenu.isActive() }, false},
		{"help", onHosts("?"),
			func(m AppModel) bool { return m.help.isActive() }, false},
		{"confirm", onHosts("D"),
			func(m AppModel) bool { return m.confirm.isActive() }, false},
		{"host picker", func(t *testing.T) AppModel { return inSFTP(t, "S") },
			func(m AppModel) bool { return m.hostPicker.isActive() }, false},
		{"transfers", func(t *testing.T) AppModel { return inSFTP(t, "P") },
			func(m AppModel) bool { return m.transfersUI.isActive() }, false},

		{"host form", onHosts("A"),
			func(m AppModel) bool { return m.form.isActive() }, true},
		{"file picker", openPicker,
			func(m AppModel) bool { return m.picker.isActive() }, true},
		{"rename", func(t *testing.T) AppModel { return inSFTP(t, "r") },
			func(m AppModel) bool { return m.input.isActive() }, true},
	} {
		m := tc.open(t)
		if !tc.live(m) {
			t.Fatalf("%s: setup did not open it", tc.name)
		}
		m = pressA(m, " ")
		switch {
		case tc.text && !tc.live(m):
			t.Errorf("%s: Space is a character here and must not dismiss it", tc.name)
		case !tc.text && tc.live(m):
			t.Errorf("%s: Space should have dismissed it", tc.name)
		}
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

	// Space unwinds one level, in the order the user built the stack.
	m = pressA(m, " ")
	if m.help.isActive() {
		t.Error("Space should have closed the help")
	}
	if !m.spaceMenu.isActive() {
		t.Error("...and left the menu standing")
	}
	m = pressA(m, " ")
	if m.spaceMenu.isActive() {
		t.Error("a second Space should close the menu too")
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
