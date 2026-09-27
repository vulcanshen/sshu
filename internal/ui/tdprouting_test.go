package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// Routing rules from the terminu design principle. Each test names the rule it
// pins, and each failed against the routing it replaced.

// tdp F3: a float that is already closing gave the keyboard back, so a second
// Esc closes the one under it rather than starting the same close over again.
func TestEscPassesAClosingFloatBy(t *testing.T) {
	m := pressA(appWith(sample(), nil), " ", "A") // the host form, over the menu
	if !m.form.isActive() || !m.spaceMenu.isActive() {
		t.Fatal("setup: the form should stand on the menu")
	}
	next, _ := m.Update(keyMsg("esc")) // the form starts to close
	m = next.(AppModel)
	next, _ = m.Update(keyMsg("esc"))
	m = next.(AppModel)
	if m.spaceMenu.anim.owns() {
		t.Error("the second Esc should have closed the menu under the closing form")
	}
}

// The toast too — it is the float this used to go wrong on.
func TestEscPassesAClosingToastBy(t *testing.T) {
	m := pressA(appWith(sample(), nil), " ")
	m.toast.show("saved", toastInfo)
	m = settle(m)
	next, _ := m.Update(keyMsg("esc")) // the toast starts to close
	m = next.(AppModel)
	next, _ = m.Update(keyMsg("esc"))
	m = next.(AppModel)
	if m.spaceMenu.anim.owns() {
		t.Error("the second Esc should have closed the menu under the closing toast")
	}
}

// tdp D3, K6: the help opened over a confirm is on top — for the keys, for Esc
// and on screen. Enter on the help must not accept the confirm it covers.
func TestHelpStaysOnTopOfAConfirm(t *testing.T) {
	m := pressA(appWith(sample(), nil), "X", "?")
	if !m.confirm.isActive() || !m.help.isActive() {
		t.Fatal("setup: the help should stand on the delete confirm")
	}
	if !strings.Contains(m.View(), glyphHelp+" confirm") {
		t.Error("the confirm's help should be drawn above the confirm")
	}
	if strings.Contains(m.View(), "Delete host") {
		t.Error("the confirm is showing through the help it is under")
	}

	n := len(m.hosts.hosts)
	m = pressA(m, "enter")
	if len(m.hosts.hosts) != n || !m.confirm.isActive() {
		t.Fatal("Enter on the help accepted the confirm under it")
	}
	m = pressA(m, "esc")
	if m.help.isActive() {
		t.Error("Esc should close the help first")
	}
	if !m.confirm.isActive() {
		t.Error("...and leave the confirm under it standing")
	}
}

// tdp K9, K8: Ctrl+C in a field being typed into is still the leaving flow. Its
// question goes on top of the form; saying no lands back on the form, and Enter
// on the question leaves rather than submitting the form under it.
func TestCtrlCInAFormAsksOnTopOfIt(t *testing.T) {
	m := openOne(t)
	m = pressA(m, "alt+esc", "M", "A")
	if !m.form.isActive() {
		t.Fatal("setup: the host form should be open")
	}
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	m = settle(next.(AppModel))
	if !m.quitAsk.isActive() || !m.form.isActive() {
		t.Fatal("Ctrl+C with a live session should ask, over the form")
	}
	m = pressA(m, "esc")
	if m.quitAsk.isActive() || !m.form.isActive() {
		t.Fatal("Esc should close only the question")
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	m = settle(next.(AppModel))
	_, cmd := m.Update(keyMsg("enter"))
	if cmd == nil {
		t.Fatal("Enter on the question should leave")
	}
	if _, isQuit := cmd().(tea.QuitMsg); !isQuit {
		t.Error("Enter went to the form under the question")
	}
}

// tdp K8: q in a field is a letter.
func TestQIsALetterInAForm(t *testing.T) {
	m := pressA(appWith(sample(), nil), "A", "q")
	if m.quitAsk.isActive() || !m.form.isActive() {
		t.Fatal("q in a field must not start the leaving flow")
	}
	if !strings.Contains(m.form.fields[m.form.focus].value, "q") {
		t.Error("the q did not land in the field")
	}
}

// tdp K9: q on the leaving flow's own question does not ask a second time.
func TestQOnTheQuitQuestionDoesNothing(t *testing.T) {
	m := openOne(t)
	m = pressA(m, "alt+esc", "q")
	if !m.quitAsk.isActive() {
		t.Fatal("setup: q with a live session should ask")
	}
	next, cmd := m.Update(keyMsg("q"))
	if cmd != nil {
		t.Error("q on the question should do nothing")
	}
	if !next.(AppModel).quitAsk.anim.owns() {
		t.Error("the question should still be up")
	}
}

// tdp K10, M1: a cell that has the keyboard but whose far end has not spoken
// swallows every key but Alt+Esc — so the footer says that key, and nothing it
// cannot keep.
func TestConnectingCellFooterDisclosesTheWayOut(t *testing.T) {
	silentSSH(t)
	m := pressA(sshApp(t, sample()), "enter", "enter")
	t.Cleanup(func() { m.ssh.stopAll() })
	if !m.ptyFocused() || m.inPty() {
		t.Fatal("setup: the cell should have the keyboard while still connecting")
	}
	f := m.footer()
	if !strings.Contains(f, "alt+esc") {
		t.Errorf("the connecting cell's footer should offer the way out, got %q", f)
	}
	for _, lie := range []string{"space", "quit", "select"} {
		if strings.Contains(f, lie) {
			t.Errorf("the footer offers %q, which does nothing here: %q", lie, f)
		}
	}
}
