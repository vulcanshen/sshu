package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// Menu rules from the terminu design principle, and sshu's trial of splitting
// ? from the menus (§11.56). Each test names the rule it pins.

// beforeGlobal is a Space menu without its global region: the rows the panel
// itself contributes.
func beforeGlobal(items []menuItem) []menuItem {
	for i, it := range items {
		if it.header && it.label == menuGlobalRegion {
			if i > 0 && items[i-1].separator {
				i--
			}
			return items[:i]
		}
	}
	return items
}

// globalTail checks that items end in the global region — its title, then the
// one row that opens the global operation popup — after a rule (tdp M2).
func globalTail(t *testing.T, where string, items []menuItem) {
	t.Helper()
	if len(items) < 2 {
		t.Errorf("%s: too short for a global region: %d rows", where, len(items))
		return
	}
	last, head := items[len(items)-1], items[len(items)-2]
	if last.key != globalMenuKey {
		t.Errorf("%s: the last row should open the global operations, got %q", where, last.key)
	}
	if !head.header || head.label != menuGlobalRegion {
		t.Errorf("%s: the global row should sit under %q, got %q", where, menuGlobalRegion, head.label)
	}
	if len(items) > 2 && !items[len(items)-3].separator {
		t.Errorf("%s: a rule should divide the global region from the panel's", where)
	}
	if !items[0].header {
		t.Errorf("%s: a panel's Space menu carries region titles, first row is %q", where, items[0].label)
	}
}

// openGlobal opens the Space menu and runs its global row.
func openGlobal(t *testing.T, m AppModel) AppModel {
	t.Helper()
	m = pressA(m, " ")
	for i, it := range m.spaceMenu.items {
		if it.key == globalMenuKey {
			m.spaceMenu.cursor = i
		}
	}
	m = pressA(m, "enter")
	if !m.globalMenu.isActive() || !m.spaceMenu.isActive() {
		t.Fatal("the global row should open the global operations over the menu")
	}
	return m
}

// tdp M2, M7: every panel's Space menu ends in the same global region — the
// info-only ones and the empty ones too.
func TestEveryPanelMenuEndsInTheGlobalRegion(t *testing.T) {
	hosts := appWith(sample(), nil)
	globalTail(t, "hosts", hosts.menuItems())

	nav := pressA(hosts, "1")
	globalTail(t, "manage nav", nav.menuItems())

	empty := appWith(nil, nil)
	globalTail(t, "empty hosts", empty.menuItems())

	sftp := sftpFixture(t, 100, 26)
	sftp.sftp.focus = panelLeftFiles
	globalTail(t, "file side", sftp.sftpMenuItems())
	sftp.sftp.sides[sideRight].disconnect()
	sftp.sftp.focus = panelRightFiles
	globalTail(t, "hostless side", sftp.sftpMenuItems())

	noSessions := pressA(appWith(sample(), nil), "S")
	globalTail(t, "no sessions", noSessions.menuItems())
	layout := pressA(noSessions, "2")
	globalTail(t, "layout", layout.menuItems())
}

// tdp M7: a panel with nothing of its own to do still says so, above the
// global region, rather than showing only the global row.
func TestAnEmptyPanelSaysThereIsNothingToDo(t *testing.T) {
	m := pressA(appWith(sample(), nil), "S")
	own := beforeGlobal(m.menuItems())
	if len(own) != 1 || !own[0].header || own[0].label != "nothing to do here" {
		t.Errorf("an empty sessions panel should say there is nothing to do, got %v", own)
	}
}

// The global operation popup lists every global operation, in globalActions'
// order, and its rows run — by letter and by Enter. Switching tab clears the
// whole stack (tdp T1); Esc comes back to the Space menu (tdp F4).
func TestTheGlobalPopupRunsTheGlobalOperations(t *testing.T) {
	m := openGlobal(t, appWith(sample(), nil))
	if len(m.globalMenu.items) != len(globalActions) {
		t.Fatalf("the popup should list %d operations, has %d", len(globalActions), len(m.globalMenu.items))
	}
	for i, g := range globalActions {
		if m.globalMenu.items[i].key != g.key {
			t.Errorf("row %d is %q, want %q", i, m.globalMenu.items[i].key, g.key)
		}
	}
	if m.globalMenu.layer <= m.spaceMenu.layer {
		t.Error("the popup should stack above the menu")
	}

	back := pressA(m, "esc")
	if back.globalMenu.isActive() || !back.spaceMenu.isActive() {
		t.Error("Esc should close the popup and leave the menu")
	}

	f := pressA(m, "F")
	if f.tab != tabFT || f.globalMenu.isActive() || f.spaceMenu.isActive() {
		t.Errorf("[F] should switch tab and clear the stack, tab=%d", f.tab)
	}

	for i, it := range m.globalMenu.items {
		if it.key == "S" {
			m.globalMenu.cursor = i
		}
	}
	if s := pressA(m, "enter"); s.tab != tabSSH {
		t.Errorf("Enter on [S]SH should switch to the ssh tab, tab=%d", s.tab)
	}
}

// The Space menu itself carries no global letters: they belong to the popup.
func TestTheSpaceMenuDoesNotRunGlobalLetters(t *testing.T) {
	m := pressA(appWith(sample(), nil), " ", "F")
	if m.tab != tabPref || !m.spaceMenu.isActive() {
		t.Error("F in the Space menu is not one of its rows and must do nothing")
	}
}

// tdp M6: the tab already on screen is a dimmed row in the popup, and running
// it does nothing — by letter or by Enter.
func TestTheTabYouAreOnIsDimAndDoesNothing(t *testing.T) {
	m := openGlobal(t, appWith(sample(), nil))
	for i, it := range m.globalMenu.items {
		if it.key == "M" {
			if !it.disabled {
				t.Error("[M]anage should be dim on the manage tab")
			}
			m.globalMenu.cursor = i
		}
		if it.key == "F" && it.disabled {
			t.Error("[F]ile transfer should be live from the manage tab")
		}
	}
	if after := pressA(m, "M"); !after.globalMenu.isActive() {
		t.Error("the dimmed row's letter must do nothing, popup included")
	}
	next, cmd := m.Update(keyMsg("enter"))
	if cmd != nil {
		if _, isQuit := cmd().(tea.QuitMsg); isQuit {
			t.Fatal("Enter on a dimmed row quit")
		}
	}
	if !next.(AppModel).globalMenu.anim.owns() {
		t.Error("Enter on a dimmed row closed the popup")
	}
}

// ? only reads (tdp K6, M4): on a panel it is the key reference — that
// panel's own keys, then the core keys — and nothing on it runs; a letter
// pressed there is not a global operation.
func TestQuestionMarkIsTheKeyReference(t *testing.T) {
	m := pressA(appWith(sample(), nil), "?")
	if !m.help.isActive() || m.help.title != "key reference" {
		t.Fatal("? on a panel should open the key reference")
	}
	view := m.help.view()
	for _, want := range []string{"this panel", "Hosts", "duplicate", "core keys", "gg · G"} {
		if !strings.Contains(view, want) {
			t.Errorf("the key reference should show %q:\n%s", want, view)
		}
	}
	// The grid's chords belong to the ssh tab's reference, not to every one.
	if strings.Contains(view, "Alt+Esc") {
		t.Errorf("the hosts panel's reference lists the grid's chords:\n%s", view)
	}
	ssh := pressA(appWith(sample(), nil), "S", "?")
	if !strings.Contains(ssh.help.view(), "Alt+Esc") {
		t.Error("the ssh tab's reference should list the grid's chords")
	}
	if after := pressA(m, "S"); after.tab != tabPref || !after.help.isActive() {
		t.Error("a letter on the key reference must not run anything")
	}
}

// tdp K9, F4: quitting from the global popup asks over it, and saying no lands
// back on it.
func TestQuitFromTheGlobalPopupAsksOverIt(t *testing.T) {
	m := openOne(t)
	m = pressA(m, "alt+esc")
	m = openGlobal(t, m)
	m = pressA(m, "q")
	if !m.quitAsk.isActive() || !m.globalMenu.isActive() {
		t.Fatal("q with a live session should ask, over the popup")
	}
	if m = pressA(m, "esc"); m.quitAsk.isActive() || !m.globalMenu.isActive() {
		t.Error("Esc should come back to the popup")
	}
}

// tdp K6: ? on a popup is that popup's own help — its keys, no global rows.
func TestAPopupsHelpIsItsOwn(t *testing.T) {
	m := pressA(appWith(sample(), nil), " ", "?")
	if !m.help.isActive() || m.help.title != "space menu" {
		t.Fatal("? on the Space menu should open the menu's own help")
	}
	var keys []string
	for _, e := range m.help.entries {
		keys = append(keys, e.key+" "+e.desc)
	}
	joined := strings.Join(keys, " | ")
	if !strings.Contains(joined, "Space close") || !strings.Contains(joined, "Enter") {
		t.Errorf("the Space menu's help should say how the menu works, has %s", joined)
	}
	if strings.Contains(joined, "Alt+Esc") || strings.Contains(joined, "switch tab") {
		t.Errorf("a popup's help lists only that popup's keys, has %s", joined)
	}

	c := pressA(appWith(sample(), nil), "X", "?")
	if c.help.title != "confirm" || c.help.entries[0].desc != "delete" {
		t.Errorf("the confirm's help should name its Enter, got %q %v", c.help.title, c.help.entries)
	}
}
