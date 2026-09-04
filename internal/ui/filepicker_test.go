package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/vulcanshen/sshu/internal/store"
)

// fixtureKeys builds a stand-in ~/.ssh and points the picker at it.
func fixtureKeys(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for name, mode := range map[string]os.FileMode{
		"id_ed25519":     0o600,
		"id_ed25519.pub": 0o644,
		"id_rsa":         0o600,
		"known_hosts":    0o644,
		"config":         0o644,
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), mode); err != nil {
			t.Fatal(err)
		}
	}
	old := identityRootOverride
	identityRootOverride = dir
	t.Cleanup(func() { identityRootOverride = old })
	return dir
}

// Enter browses, and only on the EMPTY path field: a filled one moves on,
// and Tab is "next" everywhere. (Tab used to open the picker here; that cost
// the one thing Tab does on every other field.)
func TestEnterBrowsesOnlyOnTheEmptyIdentityField(t *testing.T) {
	fixtureKeys(t)
	m := pressA(appWith(sample(), nil), "A") // create form, auth = privatekey

	m.form.focus = fIdentity
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = settle(next.(AppModel))
	if m.picker.isActive() {
		t.Fatal("Tab is next-field now, it must not open the picker")
	}

	m.form.focus = fIdentity
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = settle(next.(AppModel))
	if !m.picker.isActive() {
		t.Fatal("Enter on the empty IdentityFile should open the picker")
	}
	if m.picker.layer != m.form.layer+1 {
		t.Errorf("the picker should stack above the form: picker=%d form=%d",
			m.picker.layer, m.form.layer)
	}
	if len(m.picker.matches) != 5 {
		t.Errorf("expected the 5 fixture files, got %d", len(m.picker.matches))
	}

	// Filled: Enter is whatever it is on every other field. With the rest of the
	// form filled in that is save, and the form goes away (§11.34).
	m = pressA(m, "esc")
	m = fillHostForm(m, "picker-box")
	m.form.focus = fIdentity
	m = pressA(m, "enter")
	if m.picker.isActive() {
		t.Fatal("Enter on a filled path field must not browse")
	}
	if !m.form.submitted || m.form.err != "" {
		t.Fatalf("Enter on a filled path field should submit, submitted=%v err=%q",
			m.form.submitted, m.form.err)
	}

	// Backspace clears the whole line — a picked path is replaced, not shaved.
	m = openPicker(t)
	m = pressA(m, "esc")
	m.form.fields[fIdentity].value = "~/.ssh/id_ed25519"
	m.form.focus = fIdentity
	m = pressA(m, "backspace")
	if got := m.form.fields[fIdentity].value; got != "" {
		t.Fatalf("Backspace should clear the whole line, left %q", got)
	}
}

func openPicker(t *testing.T) AppModel {
	t.Helper()
	fixtureKeys(t)
	m := pressA(appWith(sample(), nil), "A")
	m.form.focus = fIdentity
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	return settle(next.(AppModel))
}

// Picking writes the path straight into the field — picking IS the confirmation.
func TestPickFillsTheField(t *testing.T) {
	m := openPicker(t)
	m = typeText(m, "ed25519")
	if len(m.picker.matches) == 0 {
		t.Fatal("the filter found nothing")
	}
	want := m.picker.entries[m.picker.matches[m.picker.cursor]].path

	m = pressA(m, "enter")
	if m.picker.isActive() {
		t.Error("picking should close the picker")
	}
	if !m.form.isActive() {
		t.Fatal("the form must still be there to receive the pick")
	}
	if got := m.form.fields[fIdentity].value; got != store.FoldHome(want) {
		t.Errorf("field holds %q, want %q", got, store.FoldHome(want))
	}
	if m.form.focus != fIdentity {
		t.Error("focus should return to the field that was being filled")
	}
}

// Esc unwinds one level: back to the half-filled form, not out to the panel.
func TestPickerCancelKeepsTheForm(t *testing.T) {
	m := openPicker(t)
	m = pressA(m, "esc")
	if m.picker.isActive() {
		t.Error("Esc should close the picker")
	}
	if !m.form.isActive() {
		t.Error("Esc must leave the form standing (§6.4)")
	}
	if m.form.fields[fIdentity].value != "" {
		t.Error("cancelling must not write anything into the field")
	}
}

// Typing filters, arrows move — no mode to learn.
func TestPickerFiltersAndMoves(t *testing.T) {
	m := openPicker(t)
	all := len(m.picker.matches)

	m = typeText(m, "pub")
	if len(m.picker.matches) != 1 {
		t.Fatalf("\"pub\" should match exactly id_ed25519.pub, got %d", len(m.picker.matches))
	}

	m = pressA(m, "backspace", "backspace", "backspace")
	if len(m.picker.matches) != all {
		t.Fatalf("clearing the query should restore all %d entries, got %d", all, len(m.picker.matches))
	}

	m = pressA(m, "down")
	if m.picker.cursor != 1 {
		t.Errorf("down should move the cursor, got %d", m.picker.cursor)
	}
	m = pressA(m, "up", "up")
	if want := len(m.picker.matches) - 1; m.picker.cursor != want {
		t.Errorf("up off the top should wrap to %d, got %d", want, m.picker.cursor)
	}
}

func TestPickerFrameHolds(t *testing.T) {
	fixtureKeys(t)
	for _, sz := range [][2]int{{100, 30}, {78, 24}, {60, 16}, {40, 12}, {34, 9}} {
		w, h := sz[0], sz[1]
		m := New(sample(), nil, store.DefaultConfig())
		next, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: h})
		m = pressA(settle(next.(AppModel)), "A")
		m.form.focus = fIdentity
		next, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})

		lines := strings.Split(settle(next.(AppModel)).View(), "\n")
		if len(lines) != h {
			t.Errorf("picker %dx%d: %d lines, want %d", w, h, len(lines), h)
			continue
		}
		for i, l := range lines {
			if lw := dispW(l); lw != w {
				t.Errorf("picker %dx%d line %d: width %d, want %d\n%q", w, h, i, lw, w, l)
			}
		}
	}
}

// The hint is the standing disclosure a text-entry surface trades Space for
// (§4.5), so it has to name what THIS field can do.
func TestFormHintIsPerField(t *testing.T) {
	fixtureKeys(t)
	m := pressA(appWith(sample(), nil), "A")

	// Scoped to the hint line. Searching the whole popup would also find the
	// placeholder, which says "browse" on every frame — a loose probe here would
	// pass for the wrong reason.
	m.form.focus = fIdentity
	if h := formHint(m); !strings.Contains(h, "browse") {
		t.Errorf("the IdentityFile field must advertise Tab as browse\n%q", h)
	}
	// Filled, it advertises what it now does instead — save, once the rest of
	// the form is filled in too (§11.34).
	m = fillHostForm(m, "hint-box")
	m.form.focus = fIdentity
	if h := formHint(m); strings.Contains(h, "browse") || !strings.Contains(h, "save") {
		t.Errorf("a filled IdentityFile field must advertise Enter as save\n%q", h)
	}
	m.form.fields[fIdentity].value = ""
	m.form.focus = fAuth
	if h := formHint(m); strings.Contains(h, "browse") {
		t.Errorf("browse must not be advertised on a field that cannot browse\n%q", h)
	}
	if !strings.Contains(formHint(m), "switch") {
		t.Error("the Auth toggle must advertise the arrow keys")
	}
}

// An empty IdentityFile field says how to fill it, rather than sitting blank.
func TestIdentityFieldHasAPlaceholder(t *testing.T) {
	fixtureKeys(t)
	m := pressA(appWith(sample(), nil), "A")
	m.form.focus = fName // so the field renders unfocused, as a user first sees it
	if !strings.Contains(m.form.view(), "enter to browse") {
		t.Error("an empty IdentityFile should tell the user how to fill it")
	}
}

func TestFuzzyScore(t *testing.T) {
	if _, ok := fuzzyScore("id_ed25519", "ided"); !ok {
		t.Error("a subsequence should match")
	}
	if _, ok := fuzzyScore("id_ed25519", "zzz"); ok {
		t.Error("a non-subsequence should not match")
	}
	if _, ok := fuzzyScore("id_ed25519", "ID_ED"); !ok {
		t.Error("matching should be case-insensitive")
	}
	if _, ok := fuzzyScore("anything", ""); !ok {
		t.Error("an empty query matches everything")
	}

	// A contiguous run beats the same letters scattered.
	run, _ := fuzzyScore("id_ed25519", "ed25")
	scattered, _ := fuzzyScore("everything_deleted_2_x_5", "ed25")
	if run <= scattered {
		t.Errorf("a contiguous run should outrank a scattered match: %d vs %d", run, scattered)
	}
}

// A missing ~/.ssh must not crash or silently show an empty box: it says so.
func TestPickerWithNoRootExplainsItself(t *testing.T) {
	old := identityRootOverride
	identityRootOverride = filepath.Join(t.TempDir(), "does-not-exist")
	t.Cleanup(func() { identityRootOverride = old })

	m := pressA(appWith(sample(), nil), "A")
	m.form.focus = fIdentity
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = settle(next.(AppModel))

	if m.picker.note == "" {
		t.Fatal("a missing root must be explained, not shown as an empty list")
	}
	if !strings.Contains(m.picker.view(), "type the path") {
		t.Error("the picker should point at the fallback: typing the path")
	}
}

func TestTabIsNextOnEveryField(t *testing.T) {
	fixtureKeys(t)
	m := pressA(appWith(sample(), nil), "A")

	m.form.focus = fName
	m = pressA(m, "tab")
	if m.picker.isActive() {
		t.Fatal("Tab on Name must not open the picker")
	}
	if m.form.focus == fName {
		t.Error("Tab on Name should have moved to the next field")
	}
	if m.form.fields[fName].value != "" {
		t.Error("Tab must never type into a field")
	}

	// Tab is next on the path field too — the picker moved to Enter.
	m.form.focus = fIdentity
	m = pressA(m, "tab")
	if m.picker.isActive() {
		t.Fatal("Tab on IdentityFile must not open the picker any more")
	}
	if m.form.focus == fIdentity {
		t.Error("Tab on IdentityFile should have moved on")
	}

	// An Alt combination is swallowed, never typed.
	m.form.focus = fIdentity
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}, Alt: true})
	if got := settle(next.(AppModel)).form.fields[fIdentity].value; got != "" {
		t.Errorf("an Alt key typed %q into the field", got)
	}
}

// Shift+Tab and the arrows still move off the path field.
func TestPathFieldCanStillBeLeft(t *testing.T) {
	fixtureKeys(t)
	m := pressA(appWith(sample(), nil), "A")

	m.form.focus = fIdentity
	m = pressA(m, "down")
	if m.form.focus == fIdentity || m.picker.isActive() {
		t.Errorf("down should move off the field, focus=%d", m.form.focus)
	}

	m.form.focus = fIdentity
	m = pressA(m, "shift+tab")
	if m.form.focus == fIdentity || m.picker.isActive() {
		t.Errorf("shift+tab should move off the field, focus=%d", m.form.focus)
	}
}

// TestDumpPicker is not an assertion — run with -v to eyeball it.
func TestDumpPicker(t *testing.T) {
	if !testing.Verbose() {
		t.Skip("run with -v to print the picker")
	}
	fixtureKeys(t)
	m := New(sample(), nil, store.DefaultConfig())
	next, _ := m.Update(tea.WindowSizeMsg{Width: 78, Height: 22})
	m = pressA(settle(next.(AppModel)), "A")
	m.form.focus = fIdentity
	t.Logf("\n=== form, IdentityFile focused ===\n%s", m.View())

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = settle(next.(AppModel))
	t.Logf("\n=== picker ===\n%s", m.View())
	t.Logf("\n=== picker, filtered \"ed\" ===\n%s", typeText(m, "ed").View())
}

// formHint is the popup bottom border, which is where the hint lives.
func formHint(m AppModel) string {
	lines := strings.Split(m.form.view(), "\n")
	return lines[len(lines)-1]
}
