package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// Selection mode is a mode in tdp's sense (K11): the core keys keep a meaning
// in it. Each test failed against the mode that swallowed them all.

func inSelection(t *testing.T) AppModel {
	t.Helper()
	m := pressA(openOne(t), "alt+v")
	if !m.ssh.copy.on {
		t.Fatal("setup: alt+v should open selection mode")
	}
	return m
}

// Space does nothing in a mode (tdp v0.1.10 K11): no menu, no key list, and
// the mode stays exactly where it was. (It used to open a runnable key list.)
func TestSpaceDoesNothingInSelectionMode(t *testing.T) {
	m := inSelection(t)
	row, col := m.ssh.copy.row, m.ssh.copy.col
	m = pressA(m, " ")
	if m.floatsOpen() != 0 || m.help.isActive() {
		t.Errorf("Space in selection mode should open nothing, %d floats open", m.floatsOpen())
	}
	if !m.ssh.copy.on || m.ssh.copy.row != row || m.ssh.copy.col != col {
		t.Error("Space should leave the mode and its cursor as they were")
	}
}

// The mode's keys are pressed, not picked: l moves the cursor at once.
func TestTheModesKeysArePressedDirectly(t *testing.T) {
	m := inSelection(t)
	col := m.ssh.copy.col
	m = pressA(m, "l")
	if m.ssh.copy.col != col+1 {
		t.Errorf("l should move the cursor right: col %d → %d", col, m.ssh.copy.col)
	}
	if !m.ssh.copy.on || m.floatsOpen() != 0 {
		t.Error("a mode key should run in place, with nothing opened")
	}
}

// ? is the mode's key reference, read-only: every key the mode has, and no
// mention of a Space list; ? closes it and the mode stays.
func TestQuestionMarkIsTheSelectionKeyReference(t *testing.T) {
	m := pressA(inSelection(t), "?")
	if !m.help.isActive() || m.help.title != "selection mode" {
		t.Fatalf("? should open the selection mode key reference, got %q", m.help.title)
	}
	keys := map[string]bool{}
	for _, e := range m.help.entries {
		keys[e.key] = true
		if e.key == "Space" {
			t.Error("the reference should not offer Space: it does nothing in a mode")
		}
	}
	for _, k := range []string{"h j k l", "w · e · b", "0 · $", "u · d", "v · V", "y", "Esc", "Alt+v"} {
		if !keys[k] {
			t.Errorf("the reference is missing %q", k)
		}
	}
	m = pressA(m, "?")
	if m.help.isActive() || !m.ssh.copy.on {
		t.Error("? should close the reference and leave the mode running")
	}
}

// The footer leads with ? and does not list Space (tdp K11, M1).
func TestTheSelectionFooterLeadsWithTheKeyReference(t *testing.T) {
	pairs := copyLegendPairs()
	if pairs[0][0] != "?" {
		t.Errorf("the selection footer should lead with ?, got %q", pairs[0][0])
	}
	for _, p := range pairs {
		if p[0] == "space" {
			t.Error("Space does nothing in a mode, so the footer should not list it")
		}
	}
}

// q and Ctrl+C are the leaving flow in here too (tdp K9); saying no comes back
// to the mode, and a second Ctrl+C leaves.
func TestQuitWorksFromSelectionMode(t *testing.T) {
	m := pressA(inSelection(t), "q")
	if !m.quitAsk.isActive() {
		t.Fatal("q with a live session should ask")
	}
	m = pressA(m, "esc")
	if m.quitAsk.isActive() || !m.ssh.copy.on {
		t.Error("Esc should come back to the mode")
	}

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	m = settle(next.(AppModel))
	if !m.quitAsk.isActive() {
		t.Fatal("Ctrl+C should ask, like q")
	}
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Fatal("a second Ctrl+C should leave")
	}
	if _, isQuit := cmd().(tea.QuitMsg); !isQuit {
		t.Error("a second Ctrl+C should leave at once")
	}
}

// Tab is paused in the mode, but it answers (tdp K11).
func TestTabInSelectionModeSaysHowToLeave(t *testing.T) {
	m := pressA(inSelection(t), "tab")
	if !m.toast.isActive() {
		t.Error("Tab in selection mode should say to leave the mode first")
	}
	if !m.ssh.copy.on {
		t.Error("the mode should still be on")
	}
}
