package tui

import (
	"os"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	terminu "github.com/vulcanshen/terminu-go"

	"github.com/vulcanshen/sshu/v3/internal/store"
)

// Every kind of popup sshu opens, over sshu's own screens, keeps every row
// exactly as wide as the terminal, with icons and powerline symbols one cell
// or two (tdp E5, tdp E6, tdp L4). The lib measures each popup's frame on its own;
// this is the app's half: its screens with its popups on them. The terminal
// popup is in editor_pty_test.go.
func TestEveryPopupKeepsTheScreenFullWidth(t *testing.T) {
	twoDirs(t)
	defer terminu.SetIconWidth(1, 1)
	runs := []struct{ iw, size [2]int }{
		{[2]int{1, 1}, [2]int{80, 40}}, {[2]int{2, 1}, [2]int{80, 40}}, {[2]int{2, 2}, [2]int{80, 40}},
		{[2]int{2, 2}, [2]int{60, 30}},
	}
	for _, r := range runs {
		w, h := r.size[0], r.size[1]
		d := testData(sampleHosts(), sampleCreds())
		d.Moves = []store.Move{{What: "settings", From: "/home/u/old", To: "/home/u/.config/sshu", Err: os.ErrPermission}}
		hs := newHarness(t, w, h, d)
		terminu.SetIconWidth(r.iw[0], r.iw[1])
		open := func(title, what string, keys ...string) {
			t.Helper()
			hs.keys(keys...)
			if !hs.shows(title) {
				t.Fatalf("%s did not open (no %q):\n%s", what, title, hs.text())
			}
			hs.fullWidth(w, what)
		}

		open("NOT MOVED", "error popup")
		hs.keys("Esc")
		hs.fullWidth(w, "toast")
		open("CONNECT", "connect confirm", "2", "Enter")
		open("EDIT", "host form", "Esc", "E")
		open("CREDENTIAL", "viewer", "Esc", "1", "j", "2", "Enter")
		open("GLOBAL", "global operation popup", "Esc", "Space", "G", "Enter")
		hs.keys("Esc", "Esc", "1", "k") // the global popup goes back to the Space menu first
		// sshu's own inputs, each opened from a one-field form.
		for _, in := range []struct {
			title string
			input func() terminu.Input
		}{
			{"IDENTITY", func() terminu.Input { return identityPicker("Identity file", "", func(string) {}) }},
			{"PORT", func() terminu.Input { return optionalPort("1–65535", "", func(string) {}) }},
			{"PASSWORD", func() terminu.Input {
				return terminu.PasswordInput{Title: "Password", Glyph: glyphLock, Prompt: "The password", Verb: "set",
					Done: func(string, bool) tea.Cmd { return nil }}
			}},
			{"CREDENTIAL", func() terminu.Input {
				return terminu.Select{Title: "Credential", Glyph: glyphCred, Options: []string{"ops", "deploy"},
					Done: func(int) tea.Cmd { return nil }}
			}},
		} {
			hs.run(hs.s.a.Form(&terminu.Form{Title: "Test", Button: "Save", Submit: func() (tea.Cmd, error) { return nil, nil },
				Fields: []*terminu.FormField{{Label: "x", Value: func() string { return "" }, Edit: in.input}}}))
			open(in.title, strings.ToLower(in.title)+" input", "Enter")
			hs.keys("Esc", "Esc")
		}
		open("HOST", "host picker", "F", "H")
		hs.keys("Enter")
		open("DIRECTORY", "directory viewer", "v")
		open("SEARCH", "search", "Esc", "/")
		hs.keys("Esc")
	}
}
