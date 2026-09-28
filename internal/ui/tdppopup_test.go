package ui

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// Popup rules from the terminu design principle v0.1.9: one width (F7), a
// height fixed when the popup opens (F7), scrolling instead of cutting (F7),
// the input's error row (F7, K3), and everything below the top dimmed (F8).

func boxWidth(t *testing.T, name, view string) int {
	t.Helper()
	lines := strings.Split(view, "\n")
	w := ansi.StringWidth(lines[0])
	for i, l := range lines {
		if got := ansi.StringWidth(l); got != w {
			t.Errorf("%s: line %d is %d wide, the top border %d", name, i, got, w)
		}
	}
	return w
}

func lineCount(view string) int { return len(strings.Split(view, "\n")) }

// Every popup is min(terminal − 2, 120) wide whatever it holds; the terminal
// class takes the whole terminal less one column each side (tdp F7).
func TestEveryPopupIsTheSameWidth(t *testing.T) {
	fixtureKeys(t)
	for _, w := range []int{80, 100, 200} {
		want := min(w-2, 120)
		open := func(a *popupAnimator) { a.phase = animOpen }

		sm := newSpaceMenu()
		sm.setSize(w, 40)
		sm.setItems([]menuItem{{label: "Go", key: "g"}}, "[1] x", 1)
		open(&sm.anim)

		cf := newConfirmPopup()
		cf.setSize(w, 40)
		cf.lines = []string{"Delete?"}
		open(&cf.anim)

		in := newInputPopup()
		in.setSize(w, 40)
		in.value = "n" // the box is not sized to what is typed
		open(&in.anim)

		ts := newToast()
		ts.setSize(w, 40)
		ts.msg = "Saved"
		open(&ts.anim)

		hp := newHelpPopup()
		hp.setSize(w, 40)
		hp.entries = coreKeyReference
		open(&hp.anim)

		vw := newViewerPopup()
		vw.setSize(w, 40)
		vw.showText(1, "log", []string{"one"})
		open(&vw.anim)

		tr := newTransfersPopup()
		tr.setSize(w, 40)
		open(&tr.anim)

		pk := newFilePicker()
		pk.setSize(w, 40)
		pk.open(identityRoot(), 1)
		open(&pk.anim)

		hf := newHostForm()
		hf.setSize(w, 40)
		hf.openCreate(1)
		open(&hf.anim)

		for name, v := range map[string]string{
			"space menu": sm.view(), "confirm": cf.view(), "input": in.view(),
			"toast": ts.view(), "help": hp.view(), "viewer": vw.view(),
			"jobs": tr.view(nil), "picker": pk.view(), "host form": hf.view(),
		} {
			if got := boxWidth(t, name, v); got != want {
				t.Errorf("%d columns: %s is %d wide, want %d", w, name, got, want)
			}
		}

		ed := newEditorPopup()
		ed.setSize(w, 40)
		ed.open(1, "a.txt", 10)
		open(&ed.anim)
		if got := boxWidth(t, "editor", ed.view()); got != w-2 {
			t.Errorf("%d columns: the editor is %d wide, want the whole terminal less two (%d)",
				w, got, w-2)
		}
		if got := lineCount(ed.view()); got != 40-2 {
			t.Errorf("the editor is %d rows tall while fetching, want %d", got, 40-2)
		}
	}
}

// The height is fixed when the popup opens (tdp F7): narrowing the picker's
// list, a remote preview arriving, a form growing an option — none of them
// moves the frame.
func TestAPopupKeepsTheHeightItOpenedWith(t *testing.T) {
	fixtureKeys(t)

	pk := newFilePicker()
	pk.setSize(100, 40)
	pk.open(identityRoot(), 1)
	pk.anim.phase = animOpen
	before := lineCount(pk.view())
	for _, r := range "rsa" {
		pk.update(keyMsg(string(r)))
	}
	if len(pk.matches) >= len(pk.entries) {
		t.Fatalf("setup: typing should narrow the list, %d of %d", len(pk.matches), len(pk.entries))
	}
	if got := lineCount(pk.view()); got != before {
		t.Errorf("the picker changed height as it narrowed: %d -> %d", before, got)
	}

	vw := newViewerPopup()
	vw.setSize(100, 40)
	vw.open(1, "file")
	vw.anim.phase = animOpen
	before = lineCount(vw.view())
	// It opens at full height, with the wait in the middle of it rather than
	// in a five-row stub at the top.
	for i, l := range strings.Split(ansi.Strip(vw.view()), "\n") {
		if strings.Contains(l, "reading") && (i < before/2-3 || i > before/2+3) {
			t.Errorf("the wait should sit mid-box: line %d of %d", i, before)
		}
	}
	vw.onLoaded(viewLoadedMsg{gen: vw.gen, title: "file", lines: []string{"a", "b"}})
	if got := lineCount(vw.view()); got != before {
		t.Errorf("the viewer changed height when the file arrived: %d -> %d", before, got)
	}

	sc := newSSHCfgForm()
	sc.setSize(100, 40)
	sc.openCreate(1)
	sc.anim.phase = animOpen
	before = lineCount(sc.view())
	// Its height is the form it opened with — fields, blank, error row, and
	// four rows of frame — not the whole screen.
	if want := len(sc.fields) + 2 + 4; before != want {
		t.Errorf("the ssh config form opened %d rows tall, want %d", before, want)
	}
	for i := 0; i < 3; i++ {
		sc.fields = append(sc.fields, formField{label: fmt.Sprintf("Opt%d", i)})
	}
	if got := lineCount(sc.view()); got != before {
		t.Errorf("the ssh config form grew with its options: %d -> %d", before, got)
	}
}

// A menu taller than the screen scrolls with the cursor (tdp F7): it used to be
// cut, and j walked the cursor onto rows that were never drawn.
func TestAMenuTallerThanTheScreenScrolls(t *testing.T) {
	sm := newSpaceMenu()
	sm.setSize(100, 20)
	var items []menuItem
	for i := 0; i < 40; i++ {
		items = append(items, menuItem{label: fmt.Sprintf("Row %02d", i), key: fmt.Sprintf("k%d", i)})
	}
	sm.setItems(items, "[1] x", 1)
	sm.anim.phase = animOpen
	for i := 0; i < 39; i++ {
		sm.step(1)
	}
	v := ansi.Strip(sm.view())
	if !strings.Contains(v, "Row 39") {
		t.Errorf("the cursor's row is off screen:\n%s", v)
	}
	if strings.Contains(v, "Row 00") {
		t.Errorf("a 40-row menu on a 20-row screen should have scrolled:\n%s", v)
	}
	if !strings.Contains(lastLine(v), "of 40") {
		t.Errorf("a scrolled menu says where it is, got %q", lastLine(v))
	}
}

// Jobs is a menu (tdp F1): it scrolls with its cursor, and Enter opens the job
// in full — the bar has room for the start of a failure, not the reason.
func TestJobsScrollsAndEnterOpensTheJob(t *testing.T) {
	m := appWith(sample(), nil)
	for i := 0; i < 30; i++ {
		j := &transferJob{label: fmt.Sprintf("job %02d", i), files: 1}
		m.transfers.jobs = append(m.transfers.jobs, j)
	}
	why := "open /far/away/deploy.sh: permission denied, and here is the part that says why"
	last := m.transfers.jobs[29]
	last.errText.Store(&why)
	last.state.Store(int32(xferFailed))

	m.transfersUI.open(1, len(m.transfers.jobs))
	m = settle(m)
	for i := 0; i < 29; i++ {
		m = pressA(m, "j")
	}
	if v := ansi.Strip(m.transfersUI.view(m.transfers.jobs)); !strings.Contains(v, "job 29") {
		t.Errorf("the cursor's job is off screen:\n%s", v)
	}

	m = pressA(m, "enter")
	if !m.viewer.isActive() || !m.transfersUI.isActive() {
		t.Fatalf("Enter should open the job over Jobs: viewer=%v jobs=%v",
			m.viewer.isActive(), m.transfersUI.isActive())
	}
	if v := ansi.Strip(m.viewer.view()); !strings.Contains(v, "the part that says why") {
		t.Errorf("the job's whole failure should be there:\n%s", v)
	}
	m = pressA(m, "esc")
	if m.viewer.isActive() || !m.transfersUI.isActive() {
		t.Error("Esc should close the job and leave Jobs standing (tdp F4)")
	}
}

// A refused answer stays in the input with the reason in its error row, and the
// row was there before anything failed (tdp F7, K3).
func TestARefusedAnswerStaysInTheInput(t *testing.T) {
	m := sftpFixture(t, 100, 26)
	m.sftp.focus = panelLeftFiles
	m = pressA(m, "A")
	before := lineCount(m.input.view())
	m = typeText(m, "a/b")
	m = pressA(m, "enter")

	if !m.input.isActive() {
		t.Fatal("a refused name should leave the box open")
	}
	if m.toast.isActive() {
		t.Error("the reason belongs in the box, not in a toast")
	}
	v := m.input.view()
	if !strings.Contains(ansi.Strip(v), "A name cannot contain /") {
		t.Errorf("the error row should say why:\n%s", ansi.Strip(v))
	}
	if got := lineCount(v); got != before {
		t.Errorf("the box changed height when the error arrived: %d -> %d", before, got)
	}
	m = typeText(m, "x")
	if strings.Contains(ansi.Strip(m.input.view()), "cannot contain") {
		t.Error("typing should clear the error, the way the form's does")
	}
}

var sgrRe = regexp.MustCompile("\x1b\\[([0-9;]*)m")

// foregrounds lists every 24-bit foreground an ANSI string sets.
func foregrounds(s string) []string {
	var out []string
	for _, m := range sgrRe.FindAllStringSubmatch(s, -1) {
		if strings.HasPrefix(m[1], "38;2;") {
			out = append(out, m[1])
		}
	}
	return out
}

// With a popup open, everything below the top one is dim (tdp F8): the base
// screen in the dim colour, a popup underneath with its frame in a dimmed
// version of its own layer colour. Only the top popup is lit.
func TestEverythingBelowTheTopPopupIsDim(t *testing.T) {
	withColour(t)
	m := appWith(sample(), nil)
	dim := ansiOf(t, dimColor)

	plain := m.View()
	if fg := foregrounds(strings.Split(plain, "\n")[0]); len(fg) == 0 || allEqual(fg, dim) {
		t.Fatalf("setup: the tab row should carry colour of its own, got %v", fg)
	}

	m = pressA(m, " ")
	for _, fg := range foregrounds(strings.Split(m.View(), "\n")[0]) {
		if fg != dim {
			t.Fatalf("under a popup the tab row is dim only, found %s", fg)
		}
	}
	if !strings.Contains(m.View(), ansiOf(t, popupLayerColor(1))) {
		t.Error("the one popup open is the top one, and lit")
	}

	// Two layers: help over the Space menu.
	m = pressA(m, "?")
	v := m.View()
	if !strings.Contains(v, ansiOf(t, dimmedLayerColor(1))) {
		t.Error("the Space menu under the help should keep a dimmed version of its layer colour")
	}
	if strings.Contains(v, ansiOf(t, popupLayerColor(1))) {
		t.Error("the Space menu under the help should not be lit")
	}
	if !strings.Contains(v, ansiOf(t, popupLayerColor(2))) {
		t.Error("the help on top should be lit")
	}

	// The top is whoever holds the keyboard: the moment the help starts to
	// close, the menu is lit again.
	next, _ := m.Update(keyMsg("?"))
	m = next.(AppModel)
	if !m.help.isActive() || m.help.anim.owns() {
		t.Fatal("setup: the help should be mid-close")
	}
	if !strings.Contains(m.View(), ansiOf(t, popupLayerColor(1))) {
		t.Error("with the help closing, the Space menu is the top again and lit")
	}
}

// A toast is not a layer (tdp F8): it holds no keyboard, and nothing under it
// is dimmed.
func TestAToastDimsNothing(t *testing.T) {
	withColour(t)
	m := appWith(sample(), nil)
	before := strings.Split(m.View(), "\n")[0]
	m.toast.show("Saved", toastInfo)
	m.toast.anim.phase = animOpen
	if got := strings.Split(m.View(), "\n")[0]; got != before {
		t.Errorf("a toast dimmed the screen:\n%q\n%q", before, got)
	}
}

func allEqual(xs []string, v string) bool {
	for _, x := range xs {
		if x != v {
			return false
		}
	}
	return true
}
