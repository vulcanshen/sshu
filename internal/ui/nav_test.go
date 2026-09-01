package ui

import (
	"strings"
	"testing"
)

// u and d move half a screen. The point is a long list that is walkable without
// holding j down, and an overlap between where you were and where you land.
func TestHalfPageMovesHalfAScreen(t *testing.T) {
	page := 10
	if got := moveCursor(0, 100, "d", page); got != 5 {
		t.Errorf("d from 0 landed on %d, want 5", got)
	}
	if got := moveCursor(20, 100, "u", page); got != 15 {
		t.Errorf("u from 20 landed on %d, want 15", got)
	}
	// ctrl+ spellings are the same movement.
	if a, b := moveCursor(20, 100, "d", page), moveCursor(20, 100, "ctrl+d", page); a != b {
		t.Errorf("ctrl+d moved to %d but d moved to %d", b, a)
	}
	if a, b := moveCursor(20, 100, "u", page), moveCursor(20, 100, "ctrl+u", page); a != b {
		t.Errorf("ctrl+u moved to %d but u moved to %d", b, a)
	}
	// It stops at the ends rather than running off them.
	if got := moveCursor(97, 100, "d", page); got != 99 {
		t.Errorf("d near the end landed on %d, want 99", got)
	}
	if got := moveCursor(2, 100, "u", page); got != 0 {
		t.Errorf("u near the start landed on %d, want 0", got)
	}
	// A one-row panel still moves, or the key would be dead on a short screen.
	if got := moveCursor(0, 100, "d", 1); got != 1 {
		t.Errorf("d on a 1-row page landed on %d, want 1", got)
	}
}

// Every list answers to the same vocabulary — that is the point of there being
// one moveCursor.
func TestEveryListTakesTheSameNavigationKeys(t *testing.T) {
	// [1] hosts
	m := sized(sample(), 78, 12)
	m = press(m, "d")
	if m.hosts.cursor == 0 {
		t.Error("d should move the hosts cursor")
	}
	m = press(m, "u")
	if m.hosts.cursor != 0 {
		t.Errorf("u should come back to the top, cursor=%d", m.hosts.cursor)
	}

	// [2] sftp file list
	s, _ := atRoot(sftpFixture(t, 100, 26))
	s = pressA(s, "G") // to the bottom, so u has somewhere to go
	bottom := s.sftp.sides[sideLeft].cursor
	if bottom == 0 {
		t.Fatal("setup: the fixture needs more than one row")
	}
	s = pressA(s, "u")
	if got := s.sftp.sides[sideLeft].cursor; got == bottom {
		t.Error("u should move the sftp cursor")
	}
}

// Dispatch is exact, so a navigation letter reaches an action only if that
// action DECLARED it — nothing folds onto anything any more. That is what makes
// the reservation hold without a guard inside hotkeyIndex.
func TestNavigationLettersAreReserved(t *testing.T) {
	keys := []string{"U", "D", "C"}
	for _, press := range []string{"u", "d", "c"} {
		if i := hotkeyIndex(keys, press); i >= 0 {
			t.Errorf("%q reached %q; only the declared case fires", press, keys[i])
		}
	}
	for _, want := range keys {
		if i := hotkeyIndex(keys, want); i < 0 || keys[i] != want {
			t.Errorf("the declared key %q selected %v", want, i)
		}
	}
	// And a pair one case apart stays two actions.
	pair := []string{"t", "T"}
	for i, k := range pair {
		if got := hotkeyIndex(pair, k); got != i {
			t.Errorf("%q selected index %d, want %d", k, got, i)
		}
	}
}

// No action may declare a navigation key: the exact-match pass would hand it
// over and the list would stop moving — silently, and only on the panels that
// happen to carry that action.
//
// tab [2]'s Delete sat on `d` for one round, which cost that tab its half-page
// and needed a rule of its own to excuse. Moving it to `x`/`X` put this back to a
// sentence with no exceptions, which is why the exception is gone from here too.
func TestNoActionClaimsANavigationKey(t *testing.T) {
	check := func(table, key, label string) {
		if navKeys[key] {
			t.Errorf("%s: %q takes the navigation key %q", table, label, key)
		}
	}
	for _, a := range hostActions {
		check("hosts", a.key, a.label)
	}
	for _, a := range sshActions {
		check("ssh", a.key, a.label)
	}
	for _, a := range sftpActions {
		check("sftp", a.key, a.label)
	}

}

// The half-page is the bare letter in EVERY tab, including the one that has a
// Delete. Nothing about being on a destructive panel changes how you move.
func TestDScrollsInEveryTab(t *testing.T) {
	m, _ := atRoot(sftpFixture(t, 100, 26))
	m.sftp.focus = panelLeftFiles
	if n := m.sftp.sides[sideLeft].rowCount(); n < 2 {
		t.Fatal("setup: the fixture needs more than one row")
	}

	m = pressA(m, "d")
	if m.confirm.isActive() {
		t.Fatal("d must not be Delete — that is x")
	}
	if got := m.sftp.sides[sideLeft].cursor; got == 0 {
		t.Error("d should scroll half a page in tab [2]")
	}
	// ctrl+d is the same movement, still.
	before := m.sftp.sides[sideLeft].cursor
	m = pressA(m, "gg", "ctrl+d")
	if got := m.sftp.sides[sideLeft].cursor; got != before {
		t.Errorf("ctrl+d moved to %d, want the same %d as d", got, before)
	}

	h := press(sized(sample(), 78, 12), "d")
	if h.hosts.cursor == 0 {
		t.Error("d should scroll on panel [1] too")
	}
}

// Nothing WANDERS into [5]. Entering it is a decision, because every key after
// it belongs to the remote and Alt+Esc is the only way back.
//
// `l` used to cross into it, mirroring tab [2]'s h/l. It was redundant — Enter
// on a session already shows it and focuses it — and a navigation key that hands
// the keyboard away is a navigation key you can trip over. The deliberate ways
// in are Enter (TestDigitsAddressPanelsOfTheCurrentTab covers `5`, and the Space
// menu's Open covers Enter); the ways that must NOT work are here.
func TestNothingWandersIntoThePty(t *testing.T) {
	m := appWith(sample(), nil)
	m.tab = tabSSH

	for _, k := range []string{"l", "right", "h", "left", "j", "k", "G"} {
		m.ssh.setFocus(panelSessions)
		m = press(m, k)
		if m.ssh.focus == panelPty {
			t.Errorf("%q walked into the pty", k)
		}
	}

	m.ssh.setFocus(panelSessions)
	for i := 0; i < 4; i++ {
		m = press(m, "tab")
		if m.ssh.focus == panelPty {
			t.Fatal("tab must never walk into the pty")
		}
	}
}

// Tab cycles the panels of the tab you are on and wraps there. Changing tab is
// what the digits are for — one key, one job.
func TestTabStaysInsideTheCurrentTab(t *testing.T) {
	// [1] has a single panel, so Tab has nowhere to go and must not leave.
	m := sized(sample(), 100, 26)
	for i := 0; i < 3; i++ {
		m = press(m, "tab")
		if m.tab != tabPref {
			t.Fatalf("tab %d left the hosts tab, now %d", i, m.tab)
		}
	}

	// The ssh tab has one list panel, so Tab holds still there — and never
	// walks into the pty.
	m = press(m, "alt+S")
	for i := 0; i < 6; i++ {
		m = press(m, "tab")
		if m.tab != tabSSH {
			t.Fatalf("tab %d left the ssh tab", i)
		}
		if m.ssh.focus != panelSessions {
			t.Fatalf("tab %d moved to panel %d, want [1]", i, m.ssh.focus)
		}
	}
}

// The help popup lists the vocabulary it now has — a key that exists and is not
// in here is a key nobody finds (§A.2).
func TestHelpListsTheNavigationVocabulary(t *testing.T) {
	var keys []string
	for _, e := range helpContent {
		keys = append(keys, e.key)
	}
	joined := strings.Join(keys, " | ")
	for _, want := range []string{"u · d", "Tab"} {
		if !strings.Contains(joined, want) {
			t.Errorf("help does not mention %q (has %s)", want, joined)
		}
	}
}
