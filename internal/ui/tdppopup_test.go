package ui

import (
	"fmt"
	"strings"
	"testing"
	"time"

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

// With a popup open, everything below the top one FADES (tdp F8, D2): every
// colour, foreground and background, toward the canvas — so what is drawn with
// a background is still there, darker. The expected values are written out,
// not computed with dimRGB: a test that asked the code for its answer would
// agree with whatever the code says. (The first version stripped the colour
// and redrew in one grey; its test counted foregrounds and could not see the
// tab chip, the cursor bar and the layer colours vanish.)
func TestLowerLayersFadeTheirOwnColours(t *testing.T) {
	withColour(t)
	const (
		// lipgloss writes #89b4fa as 137;179;250 and #A4C0FA as 163;192;250;
		// the fades are worked from what is actually on screen.
		litFocusBg  = "48;2;137;179;250" // focusColor: the tab chip, the cursor bar
		dimFocusBg  = "48;2;78;97;138"   // its fade
		litLayer1Fg = "38;2;163;192;250" // popupLayerColor(1)
		dimLayer1Fg = "38;2;90;103;138"  // its fade
		dimHandBg   = "48;2;100;104;125" // handColor #bac2de faded: a lower menu's cursor
	)
	m := appWith(sample(), nil)
	if !strings.Contains(strings.Split(m.View(), "\n")[0], litFocusBg) {
		t.Fatalf("setup: the tab row's chip should be drawn on %s", litFocusBg)
	}

	m = pressA(m, " ")
	v := m.View()
	row0 := strings.Split(v, "\n")[0]
	if !strings.Contains(row0, dimFocusBg) || strings.Contains(row0, litFocusBg) {
		t.Errorf("under a popup the tab chip keeps its background, faded:\n%q", row0)
	}
	if strings.Count(v, dimFocusBg) < 2 {
		t.Error("the hosts table's cursor bar should still be there, faded, under the menu")
	}
	if !strings.Contains(v, litLayer1Fg) {
		t.Error("the one popup open is the top one, and lit")
	}

	// Two layers: help over the Space menu.
	m = pressA(m, "?")
	v = m.View()
	if !strings.Contains(v, dimLayer1Fg) {
		t.Error("the Space menu under the help should keep its layer colour, faded")
	}
	if strings.Contains(v, litLayer1Fg) {
		t.Error("the Space menu under the help should not be lit")
	}
	if !strings.Contains(v, dimHandBg) {
		t.Error("the Space menu's cursor bar should still be there under the help, faded")
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
	if !strings.Contains(m.View(), litLayer1Fg) {
		t.Error("with the help closing, the Space menu is the top again and lit")
	}
}

// dimANSI fades every colour the way tdp D2 writes it down: 16 and 256
// colours through xterm's palette into 24-bit, foreground and background,
// never lighter than it was; text with no colour gets the faded text colour;
// reverse and the text itself are left alone.
func TestDimFadesEveryColourAndKeepsTheRest(t *testing.T) {
	in := "\x1b[31mred\x1b[0m \x1b[38;5;196mx\x1b[48;2;0;0;0my\x1b[7mz\x1b[39mw\nplain"
	out := dimANSI(in)
	for _, want := range []string{
		"38;2;109;0;0",     // 16-colour red (205,0,0): a channel below the canvas stays 0
		"38;2;131;0;0",     // 256-colour 196 (255,0,0)
		"48;2;0;0;0",       // black never fades UP toward the canvas
		"38;2;109;113;135", // no colour of its own: the text colour, faded
		"\x1b[7m",          // reverse survives
	} {
		if !strings.Contains(out, want) {
			t.Errorf("dimmed output should carry %q:\n%q", want, out)
		}
	}
	if strings.Contains(out, "\x1b[31m") || strings.Contains(out, "38;5;") {
		t.Errorf("no colour should come through undimmed:\n%q", out)
	}
	if ansi.Strip(out) != ansi.Strip(in) {
		t.Errorf("the text must not move: %q -> %q", ansi.Strip(in), ansi.Strip(out))
	}
	if !strings.HasPrefix(strings.Split(out, "\n")[1], "\x1b[38;2;109;113;135m") {
		t.Error("every line starts in the faded text colour")
	}
	// And a reset in the middle of a line brings the faded text colour back
	// at once, or the text after it would be drawn lit.
	if !strings.Contains(out, "\x1b[0;38;2;109;113;135m") {
		t.Errorf("a reset should be followed by the faded text colour:\n%q", out)
	}
}

// A remote program's colours and a selection's background, under a popup, are
// their own colours faded — not one grey.
func TestTheGridUnderAPopupKeepsItsColoursFaded(t *testing.T) {
	withColour(t)
	fakeSSH(t, `printf '\033[31mRED\033[42mONGREEN\033[0m $ '; exec cat`)
	m := pressA(sshApp(t, sample()), "enter", "enter")
	t.Cleanup(func() { m.ssh.stopAll() })
	waitFor(t, "the stand-in to answer", func() bool {
		return len(m.ssh.sessions) == 1 && m.ssh.sessions[0].pty.hasSpoken()
	})
	waitFor(t, "the colours to reach the cell", func() bool {
		return strings.Contains(ansi.Strip(m.View()), "ONGREEN")
	})

	m = pressA(m, "alt+v", "v", "l", "?")
	if !m.help.isActive() {
		t.Fatal("setup: ? in selection mode should open its key reference")
	}
	v := m.View()
	for name, want := range map[string]string{
		"the remote's red":       "38;2;109;0;0",     // ansi 31 (205,0,0) faded
		"the remote's green bg":  "48;2;0;109;0",     // ansi 42 (0,205,0) faded
		"the selection's yellow": "48;2;129;118;104", // selectColor #f9e2af faded
	} {
		if !strings.Contains(v, want) {
			t.Errorf("%s should be there, faded (%s)", name, want)
		}
	}
	if strings.Contains(v, "48;2;249;226;175") {
		t.Error("the selection under the popup should not be lit")
	}
}

// A popup whose content is on its way turns an icon after its title (tdp F7,
// D3): read off the clock, gone when the content lands.
func TestALoadingPopupTurnsAnIconAfterItsTitle(t *testing.T) {
	old := loadingNow
	t.Cleanup(func() { loadingNow = old })
	t0 := time.Unix(1000, 0)
	loadingNow = func() time.Time { return t0 }
	frame := func(at time.Time) string {
		return loadingFrames[(at.UnixNano()/int64(loadingStep))%int64(len(loadingFrames))]
	}
	title := func(v string) string { return ansi.Strip(strings.Split(v, "\n")[0]) }
	hasAny := func(s string) bool {
		for _, f := range loadingFrames {
			if strings.Contains(s, f) {
				return true
			}
		}
		return false
	}

	vw := newViewerPopup()
	vw.setSize(100, 40)
	vw.open(1, "deploy.sh")
	vw.anim.phase = animOpen
	if got := title(vw.view()); !strings.Contains(got, "deploy.sh "+frame(t0)) {
		t.Errorf("a remote read should turn the icon after the title: %q", got)
	}
	t1 := t0.Add(loadingStep)
	loadingNow = func() time.Time { return t1 }
	if got := title(vw.view()); !strings.Contains(got, frame(t1)) || frame(t1) == frame(t0) {
		t.Errorf("90ms later the icon should have turned: %q", got)
	}
	vw.onLoaded(viewLoadedMsg{gen: vw.gen, title: "deploy.sh", lines: []string{"x"}})
	if got := title(vw.view()); hasAny(got) {
		t.Errorf("once the file is in, the icon goes: %q", got)
	}
	vw.showText(1, "log", []string{"x"})
	if got := title(vw.view()); hasAny(got) {
		t.Errorf("text already in hand is not loading: %q", got)
	}

	ed := newEditorPopup()
	ed.setSize(100, 40)
	ed.open(1, "a.txt", 10)
	ed.anim.phase = animOpen
	for phase, want := range map[editPhase]bool{editFetching: true, editSaving: true, editRunning: false} {
		ed.phase = phase
		if got := title(ed.view()); hasAny(got) != want {
			t.Errorf("editor phase %d: icon=%v, want %v: %q", phase, hasAny(got), want, got)
		}
	}

	ka := newKnownAddForm()
	ka.setSize(100, 40)
	ka.anim.phase = animOpen
	if hasAny(title(ka.view())) {
		t.Error("the fetch form is not loading until it is sent")
	}
	ka.scanning = true
	v := ka.view()
	if !hasAny(title(v)) {
		t.Error("waiting for the host key is loading: the icon goes after the title")
	}
	if strings.Contains(ansi.Strip(v), "asking") {
		t.Error("the error row carries errors only, not the wait")
	}
}

// The icon is repainted on a tick only while something is loading.
func TestTheLoadingTickStopsWhenNothingLoads(t *testing.T) {
	m := appWith(sample(), nil)
	if _, cmd := m.Update(loadingTickMsg{}); cmd != nil {
		t.Error("with nothing loading, the tick should not re-arm")
	}
	m.viewer.loading = true
	if _, cmd := m.Update(loadingTickMsg{}); cmd == nil {
		t.Error("while the viewer loads, the tick should re-arm")
	}
}

// Connections and Changes have no row to act on — each panel is its content,
// so Enter opens all of it (tdp K3), and the Space menu says so (M3).
func TestEnterOnAJournalOpensAllOfIt(t *testing.T) {
	long := "Wrote ~/.ssh/config: added Host block prod-* with IdentityFile ~/.ssh/id_ed25519 and ProxyJump bastion.example.com"
	for _, item := range []prefItem{prefConnections, prefChanges} {
		m := appWith(sample(), nil)
		m.tab, m.pref.item, m.pref.focus = tabPref, item, panelPrefContent
		m = pressA(m, "enter")
		if m.viewer.isActive() {
			t.Errorf("%s: an empty journal has nothing to open", item.label())
		}

		at := time.Date(2026, 9, 28, 14, 3, 5, 0, time.UTC)
		m.connections.entries = []connRec{{at: at, host: "db-replica-tokyo-ap-northeast-1", user: "postgres", ok: true}}
		m.changes.entries = []changeRec{{at: at, action: long}}

		found := false
		for _, it := range m.menuItems() {
			if it.key == "enter" && it.label == "Open in full" {
				found = true
			}
		}
		if !found {
			t.Errorf("%s: the Space menu should offer Enter", item.label())
		}

		m = pressA(m, "enter")
		if !m.viewer.isActive() {
			t.Fatalf("%s: Enter should open the journal", item.label())
		}
		text := strings.Join(m.viewer.lines, " ")
		want := "db-replica-tokyo-ap-northeast-1"
		if item == prefChanges {
			want = "bastion.example.com"
		}
		if !strings.Contains(text, want) || !strings.Contains(text, "2026-09-28 14:03:05") {
			t.Errorf("%s: the whole entry, with its date, should be there:\n%s", item.label(), text)
		}
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
