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

// Space opens the mode's key list and closes it again; the mode stays.
func TestSpaceListsTheSelectionKeys(t *testing.T) {
	m := pressA(inSelection(t), " ")
	if !m.modeKeys.isActive() {
		t.Fatal("Space in selection mode should list the mode's keys")
	}
	keys := map[string]bool{}
	for _, it := range m.modeKeys.items {
		keys[it.key] = true
	}
	for _, k := range []string{"y", "v", "V", "h", "j", "k", "l", "w", "e", "b", "0", "$", "u", "d", copyLeaveKey} {
		if !keys[k] {
			t.Errorf("the list is missing %q", k)
		}
	}
	m = pressA(m, " ")
	if m.modeKeys.isActive() || !m.ssh.copy.on {
		t.Error("Space should close the list and leave the mode running")
	}
}

// A row runs by its own key — l moves, as it would with the list closed — and
// running it closes the list.
func TestARowOfTheKeyListRunsByItsKey(t *testing.T) {
	m := inSelection(t)
	col := m.ssh.copy.col
	m = pressA(m, " ", "l")
	if m.modeKeys.isActive() {
		t.Error("running a row should close the list")
	}
	if m.ssh.copy.col != col+1 {
		t.Errorf("l from the list should move the cursor right: col %d → %d", col, m.ssh.copy.col)
	}
	if !m.ssh.copy.on {
		t.Error("the mode should still be on")
	}
}

// The leave row, picked with the arrows and Enter, leaves the mode.
func TestTheLeaveRowLeavesTheMode(t *testing.T) {
	m := pressA(inSelection(t), " ")
	for i, it := range m.modeKeys.items {
		if it.key == copyLeaveKey {
			m.modeKeys.cursor = i
		}
	}
	m = pressA(m, "enter")
	if m.ssh.copy.on || m.modeKeys.isActive() {
		t.Error("the leave row should leave selection mode")
	}
}

// Esc on the list closes the list, not the mode: one layer at a time (tdp K4).
func TestEscOnTheKeyListClosesOnlyTheList(t *testing.T) {
	m := pressA(inSelection(t), " ", "esc")
	if m.modeKeys.isActive() || !m.ssh.copy.on {
		t.Error("Esc should close the list and leave the mode running")
	}
}

// ? is the mode's help, read-only; ? closes it and the mode stays.
func TestQuestionMarkIsTheSelectionHelp(t *testing.T) {
	m := pressA(inSelection(t), "?")
	if !m.help.isActive() || m.help.title != "selection mode" {
		t.Fatalf("? should open the selection mode help, got %q", m.help.title)
	}
	m = pressA(m, "?")
	if m.help.isActive() || !m.ssh.copy.on {
		t.Error("? should close the help and leave the mode running")
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
