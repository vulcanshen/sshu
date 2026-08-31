package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/vulcanshen/sshu/internal/store"
)

// Every animator's target, so a test can drive whatever is on screen to rest.
// Every animator name in the app. A popup missing from this list never
// finishes opening under test, so its content renders as nothing and the
// test passes for the wrong reason.
var animTargets = []string{"spacemenu", "hostpicker", "help", "form", "picker",
	"transfers", "history", "viewer", "editor", "confirm", "input", "toast"}

// settle runs the animations to completion — a popup mid-open refuses keys on
// purpose (§6.2), so a test that skips this is testing a half-drawn surface.
func settle(m AppModel) AppModel {
	for i := 0; i <= animFrames; i++ {
		for _, t := range animTargets {
			next, _ := m.Update(AnimTickMsg{Target: t})
			m = next.(AppModel)
		}
	}
	return m
}

func keyMsg(k string) tea.KeyMsg {
	switch k {
	case " ":
		return tea.KeyMsg{Type: tea.KeySpace}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEscape}
	case "alt+esc":
		return tea.KeyMsg{Type: tea.KeyEscape, Alt: true}
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "tab":
		return tea.KeyMsg{Type: tea.KeyTab}
	case "shift+tab":
		return tea.KeyMsg{Type: tea.KeyShiftTab}
	case "backspace":
		return tea.KeyMsg{Type: tea.KeyBackspace}
	case "ctrl+d":
		return tea.KeyMsg{Type: tea.KeyCtrlD}
	case "ctrl+u":
		return tea.KeyMsg{Type: tea.KeyCtrlU}
	case "left":
		return tea.KeyMsg{Type: tea.KeyLeft}
	case "right":
		return tea.KeyMsg{Type: tea.KeyRight}
	case "up":
		return tea.KeyMsg{Type: tea.KeyUp}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	default:
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
	}
}

// pressA sends keys and settles after each, which is what a human sees.
func pressA(m AppModel, keys ...string) AppModel {
	for _, k := range keys {
		next, _ := m.Update(keyMsg(k))
		m = settle(next.(AppModel))
	}
	return m
}

// typeText sends a string one rune at a time, the way a form receives it.
func typeText(m AppModel, s string) AppModel {
	for _, r := range s {
		next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = next.(AppModel)
	}
	return m
}

func appWith(hosts []store.Host, saved *[]store.Host) AppModel {
	save := SaveFunc(nil)
	if saved != nil {
		save = func(list []store.Host) error { *saved = list; return nil }
	}
	m := New(hosts, save)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	return next.(AppModel)
}

// --------------------------------------------------------------- the frame

// A composited float must not change the canvas: the overlay has to preserve
// both the line count and every line's width, or the whole frame shears.
func TestPopupPreservesFrame(t *testing.T) {
	sizes := [][2]int{{100, 30}, {78, 24}, {60, 16}, {40, 12}, {24, 9}}
	opens := map[string][]string{
		"space menu": {" "},
		"help":       {"?"},
		"form":       {"a"},
		"confirm":    {"enter"},
		"menu+form":  {" ", "e"},
		"toast":      {"enter", "enter"}, // connect confirm -> commit -> toast
	}
	for name, keys := range opens {
		for _, sz := range sizes {
			w, h := sz[0], sz[1]
			m := New(sample(), nil)
			next, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: h})
			got := pressA(settle(next.(AppModel)), keys...).View()

			lines := strings.Split(got, "\n")
			if len(lines) != h {
				t.Errorf("%s %dx%d: %d lines, want %d", name, w, h, len(lines), h)
				continue
			}
			for i, l := range lines {
				if lw := lipgloss.Width(l); lw != w {
					t.Errorf("%s %dx%d line %d: width %d, want %d\n%q", name, w, h, i, lw, w, l)
				}
			}
		}
	}
}

// ------------------------------------------------------- §A.1 completeness

// Every contextual action must be reachable from the Space menu, not only from
// its letter. hostActions is the single declaration behind both, so this test is
// really checking that menuItems still renders all of it.
func TestSpaceMenuListsEveryAction(t *testing.T) {
	items := appWith(sample(), nil).menuItems()
	for _, a := range hostActions {
		found := false
		for _, it := range items {
			if it.key == a.key && it.label == a.label {
				found = true
			}
		}
		if !found {
			t.Errorf("action %q (%s) is not in the Space menu — that is a VTP hole", a.label, a.key)
		}
	}
}

// And every menu row must actually run: a row that commits to nothing is worse
// than no row, because the user concludes the app is broken.
func TestEveryMenuRowRuns(t *testing.T) {
	for _, a := range hostActions {
		m := pressA(appWith(sample(), nil), " ")
		if !m.spaceMenu.isActive() {
			t.Fatal("Space did not open the menu")
		}
		m = pressA(m, a.key)
		opened := m.form.isActive() || m.confirm.isActive() || m.tab != tabHosts ||
			m.hosts.filtering
		if !opened {
			t.Errorf("menu row %q committed but nothing happened", a.label)
		}
	}
}

// The item region comes first (§6.6 cursor-first).
func TestMenuRegionsAreCursorFirst(t *testing.T) {
	items := appWith(sample(), nil).menuItems()
	if !items[0].header || items[0].label != menuItemRegion {
		t.Fatalf("first row should be the item-region header, got %+v", items[0])
	}
	iPanel, iConnect := -1, -1
	for i, it := range items {
		if it.header && it.label == menuPanelRegion {
			iPanel = i
		}
		if it.key == "enter" {
			iConnect = i
		}
	}
	if iConnect < 0 || iPanel < 0 || iConnect > iPanel {
		t.Fatalf("item region must precede the panel region (connect=%d panel=%d)", iConnect, iPanel)
	}
}

// With no hosts there is no item region, but the panel region still stands — an
// entry key that opens an empty box is the failure mode §A.1 forbids.
func TestMenuOnEmptyPanelStillOffersCreate(t *testing.T) {
	items := appWith(nil, nil).menuItems()
	for _, it := range items {
		if it.key == "A" {
			return
		}
	}
	t.Fatal("empty hosts panel must still offer Create")
}

// Space must answer on every surface, including the tabs that do not exist yet.
func TestSpaceRespondsOnUnbuiltTabs(t *testing.T) {
	for _, tab := range []string{"2", "3"} {
		m := pressA(appWith(sample(), nil), tab, " ")
		if !m.spaceMenu.isActive() {
			t.Errorf("tab %s: Space must open something, even if it only says there is nothing", tab)
		}
	}
}

// ------------------------------------------------------------ cancel / stack

// Esc pops exactly one level, so a form opened from the menu lands back on it.
func TestEscPopsOneLevel(t *testing.T) {
	m := pressA(appWith(sample(), nil), " ", "E")
	if !m.form.isActive() || !m.spaceMenu.isActive() {
		t.Fatal("expected the form over a still-open Space menu")
	}
	if m.form.layer != 2 {
		t.Errorf("a float over the menu should be layer 2, got %d", m.form.layer)
	}

	m = pressA(m, "esc")
	if m.form.isActive() {
		t.Error("Esc should have closed the form")
	}
	if !m.spaceMenu.isActive() {
		t.Error("Esc must leave the source menu standing (§6.4)")
	}
	m = pressA(m, "esc")
	if m.spaceMenu.isActive() {
		t.Error("a second Esc should close the menu")
	}
}

// Committing ends the errand, so it takes the whole stack with it (§7.1).
func TestCommitTearsDownTheStack(t *testing.T) {
	m := pressA(appWith(sample(), nil), " ", "enter") // menu -> Connect -> confirm
	if !m.confirm.isActive() {
		t.Fatal("expected the connect confirmation")
	}
	m = pressA(m, "enter")
	if m.confirm.isActive() || m.spaceMenu.isActive() {
		t.Error("committing must clear the whole float stack")
	}
	if m.tab != tabSSH {
		t.Errorf("connect should hand off to tab [3], got %d", m.tab)
	}
}

// A popup owns the keyboard: q must not quit out from under one.
func TestQuitIsInertUnderAPopup(t *testing.T) {
	m := pressA(appWith(sample(), nil), " ")
	if _, cmd := m.Update(keyMsg("q")); cmd != nil {
		t.Error("q must not quit while a popup is open — that would make it a half-alias of cancel")
	}
}

// A popup mid-animation is visible but deaf.
func TestPopupIgnoresKeysWhileAnimating(t *testing.T) {
	m := appWith(sample(), nil)
	next, _ := m.Update(keyMsg(" ")) // opened, not settled
	m = next.(AppModel)
	if m.spaceMenu.isInteractive() {
		t.Fatal("a just-opened popup should still be animating")
	}
	before := m.spaceMenu.cursor
	next, _ = m.Update(keyMsg("j"))
	if next.(AppModel).spaceMenu.cursor != before {
		t.Error("keys must not land on a half-drawn surface")
	}
}

// ------------------------------------------------------------------- form

func TestFormSkipsTheDisabledAuthRow(t *testing.T) {
	m := pressA(appWith(sample(), nil), "A") // create; defaults to privatekey
	if m.form.auth() != store.AuthPrivateKey {
		t.Fatalf("create should default to privatekey, got %q", m.form.auth())
	}
	if m.form.enabled(fPassword) {
		t.Error("Password must be inert while auth is privatekey")
	}

	// Tab from Auth lands on Identity, never on the dead Password row.
	m.form.focus = fAuth
	m.form.moveFocus(1)
	if m.form.focus != fIdentity {
		t.Errorf("Tab from Auth should reach Identity, got field %d", m.form.focus)
	}

	// Flip to password and the pair swaps.
	m.form.focus = fAuth
	m = pressA(m, "left")
	if m.form.auth() != store.AuthPassword {
		t.Fatalf("left should flip the toggle, got %q", m.form.auth())
	}
	if m.form.enabled(fIdentity) || !m.form.enabled(fPassword) {
		t.Error("flipping auth must swap which of Identity / Password is live")
	}
	m.form.focus = fAuth
	m.form.moveFocus(1)
	if m.form.focus != fPassword {
		t.Errorf("Tab from Auth should now reach Password, got field %d", m.form.focus)
	}
}

// The popup must not change height when Auth flips: both rows always exist.
func TestFormHeightIsStableAcrossAuth(t *testing.T) {
	m := pressA(appWith(sample(), nil), "A")
	before := strings.Count(m.form.view(), "\n")
	m.form.focus = fAuth
	m = pressA(m, "left")
	if after := strings.Count(m.form.view(), "\n"); after != before {
		t.Errorf("form height changed with auth: %d -> %d", before, after)
	}
}

// The password is never rendered in clear — not on the card, not in the form.
func TestPasswordIsMasked(t *testing.T) {
	secret := "hunter2trombone"
	hosts := []store.Host{{Name: "a", Host: "h", Port: 22, User: "u",
		Auth: store.AuthPassword, Password: secret}}
	m := pressA(appWith(hosts, nil), "E")
	if !strings.Contains(m.form.view(), strings.Repeat("•", len(secret))) {
		t.Error("the password field should render as bullets")
	}
	if strings.Contains(m.View(), secret) {
		t.Error("the plaintext password must never reach the screen")
	}
}

func TestPortFieldRejectsNonDigits(t *testing.T) {
	m := pressA(appWith(sample(), nil), "A")
	m.form.focus = fPort
	m.form.fields[fPort].value, m.form.fields[fPort].caret = "", 0
	m = typeText(m, "2x2b2")
	if got := m.form.fields[fPort].value; got != "222" {
		t.Errorf("Port should keep only digits, got %q", got)
	}
}

// Space is a character inside a form, not the §A.1 entry key (§4.5).
func TestSpaceTypesInsideTheForm(t *testing.T) {
	m := pressA(appWith(sample(), nil), "A")
	next, _ := m.Update(keyMsg(" "))
	m = next.(AppModel)
	if m.form.fields[fName].value != " " {
		t.Errorf("Space should type a space, got %q", m.form.fields[fName].value)
	}
}

// --------------------------------------------------------------------- CRUD

func TestCreateSavesTheNewHost(t *testing.T) {
	var saved []store.Host
	m := pressA(appWith(sample(), &saved), "A")
	m = typeText(m, "new-box")
	m = pressA(m, "tab")
	m = typeText(m, "10.9.9.9")
	m = pressA(m, "tab", "tab")
	m = typeText(m, "root")
	m = pressA(m, "enter")

	if m.form.isActive() {
		t.Fatalf("form should have closed; error was %q", m.form.err)
	}
	if len(saved) != len(sample())+1 {
		t.Fatalf("saved %d hosts, want %d", len(saved), len(sample())+1)
	}
	got := saved[len(saved)-1]
	if got.Name != "new-box" || got.Host != "10.9.9.9" || got.User != "root" || got.Port != 22 {
		t.Fatalf("saved the wrong record: %+v", got)
	}
	if m.hosts.hosts[m.hosts.cursor].Name != "new-box" {
		t.Error("the cursor should land on what was just saved")
	}
}

func TestEditSavesInPlace(t *testing.T) {
	var saved []store.Host
	m := pressA(appWith(sample(), &saved), "E")
	m.form.focus = fUser
	m.form.fields[fUser].value, m.form.fields[fUser].caret = "", 0
	m = typeText(m, "ops")
	m = pressA(m, "enter")

	if len(saved) != len(sample()) {
		t.Fatalf("edit must not change the host count: %d", len(saved))
	}
	if saved[0].User != "ops" || saved[0].Name != sample()[0].Name {
		t.Fatalf("edit landed wrong: %+v", saved[0])
	}
}

func TestDeleteRemovesAndSaves(t *testing.T) {
	var saved []store.Host
	// Upper case: lower-case d is half-page-down on the table.
	m := pressA(appWith(sample(), &saved), "D")
	if !m.confirm.isActive() || m.confirm.action != confirmDelete {
		t.Fatal("D should raise the delete confirmation")
	}
	m = pressA(m, "enter")

	if len(saved) != len(sample())-1 {
		t.Fatalf("saved %d hosts, want %d", len(saved), len(sample())-1)
	}
	if indexOfHost(saved, sample()[0].Name) >= 0 {
		t.Error("the deleted host is still in the saved list")
	}
}

// Esc on the confirmation must leave the data alone.
func TestDeleteCancelledChangesNothing(t *testing.T) {
	saved := []store.Host(nil)
	touched := false
	m := New(sample(), func(list []store.Host) error { touched = true; saved = list; return nil })
	next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = pressA(settle(next.(AppModel)), "d", "esc")

	if touched {
		t.Error("cancelling must not write hosts.yaml")
	}
	if len(m.hosts.hosts) != len(sample()) {
		t.Error("cancelling must not change the list")
	}
	_ = saved
}

// Validation keeps the user in the form with the offending field marked, rather
// than stacking an error popup they have to dismiss before fixing anything.
func TestValidationStaysInTheForm(t *testing.T) {
	var saved []store.Host
	m := pressA(appWith(sample(), &saved), "A")
	m = pressA(m, "enter") // nothing filled in

	if !m.form.isActive() {
		t.Fatal("an invalid form must stay open")
	}
	if m.form.errIdx != fName {
		t.Errorf("the error should point at Name, got field %d", m.form.errIdx)
	}
	if saved != nil {
		t.Error("nothing should have been written")
	}
	if !strings.Contains(m.form.view(), m.form.err) {
		t.Error("the error text should be visible in the form")
	}
}

func TestDuplicateNameRejected(t *testing.T) {
	var saved []store.Host
	m := pressA(appWith(sample(), &saved), "A")
	m = typeText(m, sample()[1].Name)
	m = pressA(m, "tab")
	m = typeText(m, "h")
	m = pressA(m, "tab", "tab")
	m = typeText(m, "u")
	m = pressA(m, "enter")

	if !m.form.isActive() || m.form.errIdx != fName {
		t.Fatalf("a duplicate name must be refused on the Name field (err=%q field=%d)",
			m.form.err, m.form.errIdx)
	}
	if saved != nil {
		t.Error("nothing should have been written")
	}
}

// A stale generation must not close the toast that replaced it.
func TestToastGenerationGuard(t *testing.T) {
	m := appWith(sample(), nil)
	m.toast.show("first", toastInfo)
	stale := toastExpireMsg{gen: m.toast.gen}
	m.toast.show("second", toastInfo)
	m = settle(m)

	next, _ := m.Update(stale)
	if !next.(AppModel).toast.isActive() {
		t.Error("an old timer must not retire the toast that replaced it")
	}
}

// TestDumpPopups is not an assertion — run `go test ./internal/ui -run DumpPopups -v`
// to eyeball each float.
func TestDumpPopups(t *testing.T) {
	if !testing.Verbose() {
		t.Skip("run with -v to print the popups")
	}
	for _, tc := range []struct {
		name string
		keys []string
	}{
		{"space menu", []string{" "}},
		{"help", []string{"?"}},
		{"add-host form", []string{"a"}},
		{"edit form (over the menu)", []string{" ", "e"}},
		{"connect confirm", []string{"enter"}},
		{"delete confirm", []string{"d"}},
		{"toast", []string{"enter", "enter"}},
	} {
		m := New(sample(), nil)
		next, _ := m.Update(tea.WindowSizeMsg{Width: 78, Height: 22})
		t.Logf("\n=== %s ===\n%s", tc.name, pressA(settle(next.(AppModel)), tc.keys...).View())
	}
}

// The bracket marking shows [C]reate, so shift+C must work — the marking is what
// the user reads, and a key that only answers to the lowercase form makes the
// disclosure a lie. Both cases fire, from the panel and from inside the menu.
// The marked key is the ONLY key. What is on screen is the whole binding: what
// it names fires, and nothing else does.
//
// This replaces a rule that also accepted the other case. That rule came from a
// real bug — the display said [C] while the binding was `c` — but the fix landed
// in the wrong place: printing the key as declared solved it, and the loose
// matching stayed on as a binding nothing discloses.
func TestOnlyTheMarkedCaseFires(t *testing.T) {
	fired := func(m AppModel) bool {
		return m.form.isActive() || m.confirm.isActive() || m.tab != tabHosts ||
			m.hosts.filtering
	}
	for _, a := range hostActions {
		if len(a.key) != 1 {
			continue // core-key actions carry their key in the hint, not a bracket
		}
		other := strings.ToLower(a.key)
		if other == a.key {
			other = strings.ToUpper(a.key)
		}
		if other == a.key {
			continue // a symbol has no other case to be wrong about
		}

		if !fired(pressA(appWith(sample(), nil), a.key)) {
			t.Errorf("panel: the marked key %q (%s) did nothing", a.key, a.label)
		}
		if !fired(pressA(appWith(sample(), nil), " ", a.key)) {
			t.Errorf("space menu: the marked key %q (%s) did nothing", a.key, a.label)
		}
		if fired(pressA(appWith(sample(), nil), other)) {
			t.Errorf("panel: %q fired %s, which is marked [%s]", other, a.label, a.key)
		}
		if fired(pressA(appWith(sample(), nil), " ", other)) {
			t.Errorf("space menu: %q fired %s, marked [%s]", other, a.label, a.key)
		}
	}
}

// The one the user reported: [C]lose is shift+C, and a bare c is not a binding.
func TestLowercaseDoesNotFireAnUppercaseAction(t *testing.T) {
	for _, tc := range []struct {
		tab   tabID
		lower string
		live  func(AppModel) bool
		what  string
	}{
		{tabHosts, "e", func(m AppModel) bool { return m.form.isActive() }, "[E]dit"},
		{tabHosts, "a", func(m AppModel) bool { return m.form.isActive() }, "[A]dd"},
	} {
		m := appWith(sample(), nil)
		m.tab = tc.tab
		if after := pressA(m, tc.lower); tc.live(after) {
			t.Errorf("%q fired %s", tc.lower, tc.what)
		}
	}

	// tab [2]: `c` must not clear the marks — lower case is the row here.
	s := sftpFixture(t, 100, 26)
	s.sftp.focus = panelLeftFiles
	s = pressA(s, "m")
	if n := len(s.sftp.sides[sideLeft].marks); n != 1 {
		t.Fatal("setup: expected one mark")
	}
	s = pressA(s, "c")
	if n := len(s.sftp.sides[sideLeft].marks); n != 1 {
		t.Errorf("c fired [C]lear marks, %d marks left", n)
	}
	s = pressA(s, "C")
	if n := len(s.sftp.sides[sideLeft].marks); n != 0 {
		t.Errorf("C should clear the marks, %d left", n)
	}
}

// Case-insensitivity must not leak into navigation: G is jump-to-last and must
// not become g, which is the first half of the gg chord.
func TestNavigationKeysStayCaseSensitive(t *testing.T) {
	m := pressA(appWith(sample(), nil), "G")
	if m.hosts.cursor != len(sample())-1 {
		t.Fatalf("G should jump to the last host, cursor=%d", m.hosts.cursor)
	}
	m = pressA(m, "g")
	if !m.pendingG {
		t.Error("lowercase g must still arm the gg chord")
	}
	if m.hosts.cursor != len(sample())-1 {
		t.Error("a lone g must not move the cursor")
	}
}

// A field that has been fixed must stop being marked immediately — not at the
// next submit. Leaving the red on a filled-in field trains the user to ignore it.
func TestFormErrorClearsAsFieldsAreFixed(t *testing.T) {
	m := pressA(appWith(sample(), nil), "A")

	// Nothing is said before the first submit: an empty form has not been asked
	// for anything yet.
	if m.form.err != "" {
		t.Fatalf("a fresh form should not complain yet, got %q", m.form.err)
	}

	m = pressA(m, "enter")
	if m.form.errIdx != fName {
		t.Fatalf("submit should mark Name, got field %d", m.form.errIdx)
	}

	// Fill Name: the mark must move on to the next real problem, not linger.
	m = typeText(m, "box")
	if m.form.errIdx == fName {
		t.Errorf("Name is filled but still marked: %q", m.form.err)
	}
	if m.form.errIdx != fHost {
		t.Errorf("the error should have moved to Host, got field %d (%q)", m.form.errIdx, m.form.err)
	}

	// Focus must not jump around while typing.
	if m.form.focus != fName {
		t.Errorf("live re-validation must not steal focus, focus=%d", m.form.focus)
	}

	m.form.focus = fHost
	m = typeText(m, "h.example.com")
	if m.form.errIdx != fUser {
		t.Errorf("the error should have moved to User, got field %d (%q)", m.form.errIdx, m.form.err)
	}

	m.form.focus = fUser
	m = typeText(m, "root")
	if m.form.err != "" {
		t.Errorf("everything is filled; the error should be gone, got %q", m.form.err)
	}
	if m.form.errIdx != -1 {
		t.Errorf("no field should be marked, got %d", m.form.errIdx)
	}
}

// No two actions reachable from the same panel may answer to the same key.
//
// Case MAY be significant — `t` (transfer this item) and `T` (transfer every
// mark) are two actions in one table — so the test is about reachability, not
// about letters being distinct: every action must be what its own key selects.
// That one assertion catches both failure modes at once, an outright duplicate
// and a case-sibling that the fold-fallback would swallow.
//
// It replaces a stricter rule ("no two keys may match case-insensitively"),
// which was right while every hotkey was case-insensitive and would now forbid
// the t/T pair outright.
func TestNoHotkeyCollisions(t *testing.T) {
	type entry struct{ key, label string }
	tables := map[string][]entry{}

	for _, a := range hostActions {
		tables["hosts"] = append(tables["hosts"], entry{a.key, a.label})
	}
	for _, a := range sshActions {
		k := "ssh/panel" + itoa(int(a.panel))
		tables[k] = append(tables[k], entry{a.key, a.label})
	}
	// The sftp table is split by which panels an action applies to: two actions
	// only collide if the same panel can reach both.
	for _, a := range sftpActions {
		if a.onFiles {
			tables["sftp/files"] = append(tables["sftp/files"], entry{a.key, a.label})
		}
		if a.onMarks {
			tables["sftp/marks"] = append(tables["sftp/marks"], entry{a.key, a.label})
		}
	}

	for name, es := range tables {
		keys := make([]string, len(es))
		for i, e := range es {
			keys[i] = e.key
		}
		for i, a := range es {
			if got := hotkeyIndex(keys, a.key); got != i {
				t.Errorf("%s: %q(%s) is unreachable — its own key selects %q(%s)",
					name, a.label, a.key, es[got].label, es[got].key)
			}
			// A pair that differs only in case has nothing but the bracket to
			// tell it apart on screen, so the marking must actually differ.
			for _, b := range es[i+1:] {
				if a.key != b.key && sameHotkey(a.key, b.key) &&
					bracketHotkey(a.label, a.key) == bracketHotkey(b.label, b.key) {
					t.Errorf("%s: %q and %q share a letter but mark it the same way",
						name, a.label, b.label)
				}
			}
		}
	}
}

// The Auth field is a radio group, and it says so with radio glyphs rather than
// with ASCII parentheses.
func TestAuthFieldUsesRadioGlyphs(t *testing.T) {
	m := pressA(appWith(sample(), nil), "A")
	if !m.form.isActive() {
		t.Fatal("setup: the form should be open")
	}
	view := ansi.Strip(m.form.view())

	if !strings.Contains(view, glyphRadioOn) {
		t.Error("the selected option should carry the filled radio glyph")
	}
	if !strings.Contains(view, glyphRadioOff) {
		t.Error("the other option should carry the hollow one")
	}
	if strings.Contains(view, "(\u2022)") || strings.Contains(view, "( )") {
		t.Error("the ASCII radio buttons are still there")
	}
}
