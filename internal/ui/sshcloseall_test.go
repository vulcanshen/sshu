package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

// twoSessions is [1] with two live sessions and the keyboard on the list.
func twoSessions(t *testing.T) AppModel {
	t.Helper()
	m := openOne(t)
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEscape, Alt: true})
	m = settle(next.(AppModel))
	m = pressA(m, "D", "enter") // duplicate: stays on the list
	if len(m.ssh.sessions) != 2 {
		t.Fatalf("setup: expected two sessions, got %d", len(m.ssh.sessions))
	}
	t.Cleanup(func() { m.ssh.stopAll() })
	return m
}

// The row is in the menu, in the panel region, and it draws WITHOUT a bracket —
// there is no letter to put in one.
func TestCloseAllIsAMenuRowWithNoHotkey(t *testing.T) {
	m := twoSessions(t)
	m = pressA(m, " ")
	if !m.spaceMenu.isActive() {
		t.Fatal("Space did not open the menu")
	}

	view := ansi.Strip(m.spaceMenu.view())
	if !strings.Contains(view, "Close all sessions") {
		t.Fatalf("the row is not in the menu:\n%s", view)
	}
	for _, bracket := range []string{"[C]lose all", "[c]lose all", "[close-all]"} {
		if strings.Contains(view, bracket) {
			t.Errorf("the row must not claim a hotkey it does not have (%q):\n%s", bracket, view)
		}
	}
	// Under the PANEL header, not among the row actions: it is about the list,
	// not about the session the cursor happens to be on. Checking that the
	// header is merely present is not enough — it is drawn either way.
	panelAt, rowAt := -1, -1
	for i, it := range m.spaceMenu.items {
		switch {
		case it.header && it.label == menuPanelRegion:
			panelAt = i
		case it.key == keyCloseAll:
			rowAt = i
		}
	}
	if panelAt < 0 || rowAt < 0 {
		t.Fatalf("menu is missing the panel header or the row (header=%d, row=%d):\n%s",
			panelAt, rowAt, view)
	}
	if rowAt < panelAt {
		t.Errorf("Close all sits in the item region (header=%d, row=%d):\n%s", panelAt, rowAt, view)
	}
	// And the row it must not be confused with is still there, with its letter.
	if !strings.Contains(view, "[C]lose") {
		t.Errorf("single-session Close should still show its letter:\n%s", view)
	}
}

// No keystroke reaches it. That is the point of leaving the letter off: closing
// everything is destructive and rare, and a letter is what a hand finds by
// accident on a list it was only scrolling.
func TestNoKeystrokeReachesCloseAll(t *testing.T) {
	for _, k := range []string{"c", "C", "a", "A", "l", "L", "x", "X"} {
		m := twoSessions(t)
		m = pressA(m, k)
		if m.confirm.isActive() && m.confirm.action == confirmCloseAll {
			t.Errorf("%q reached Close all sessions", k)
		}
		m.ssh.stopAll()
	}
}

// Committing it asks first, and the question counts what is about to go.
func TestCloseAllAsksWithTheCount(t *testing.T) {
	m := twoSessions(t)
	m = pressA(m, keyCloseAll)

	if !m.confirm.isActive() {
		t.Fatal("closing everything must ask first")
	}
	if m.confirm.action != confirmCloseAll {
		t.Fatalf("wrong action: %d", m.confirm.action)
	}
	body := ansi.Strip(m.confirm.view())
	if !strings.Contains(body, "2 sessions") {
		t.Errorf("the question should count them:\n%s", body)
	}

	// Esc leaves everything running.
	m = pressA(m, "esc")
	for i, s := range m.ssh.sessions {
		if s.pty.exited() {
			t.Errorf("cancelling killed session %d", i)
		}
	}
}

// And committing takes them all.
func TestCloseAllEndsEverySession(t *testing.T) {
	m := twoSessions(t)
	live := append([]*session(nil), m.ssh.sessions...)

	m = pressA(m, keyCloseAll, "enter")
	for i, s := range live {
		waitFor(t, "session to end", func() bool { return s.pty.exited() })
		if !s.pty.exited() {
			t.Errorf("session %d survived", i)
		}
	}
	if m.ssh.focus != panelSessions {
		t.Errorf("the keyboard should be on the list afterwards, focus=%d", m.ssh.focus)
	}
}

// Running it from inside the menu is the same action — the menu is a
// discoverability shell over the table, not a second implementation of it.
func TestCloseAllRunsFromTheMenuToo(t *testing.T) {
	m := twoSessions(t)
	m = pressA(m, " ")
	for i, it := range m.spaceMenu.items {
		if it.key == keyCloseAll {
			m.spaceMenu.cursor = i
		}
	}
	m = pressA(m, "enter")
	if !m.confirm.isActive() || m.confirm.action != confirmCloseAll {
		t.Fatalf("Enter on the row should ask to close them all, action=%d", m.confirm.action)
	}
}

// With nothing to close it is not offered — the same rule the row actions keep.
func TestCloseAllIsAbsentWithNoSessions(t *testing.T) {
	m := pressA(sshApp(t, sample()), "S") // the ssh tab, no sessions
	if len(m.ssh.sessions) != 0 {
		t.Fatalf("setup: expected no sessions, got %d", len(m.ssh.sessions))
	}
	m = pressA(m, " ")
	if view := ansi.Strip(m.spaceMenu.view()); strings.Contains(view, "Close all sessions") {
		t.Errorf("with nothing to close the row should be gone:\n%s", view)
	}
}
