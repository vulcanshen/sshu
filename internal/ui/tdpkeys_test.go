package ui

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// How keys are written and what the key reference dims (tdp v0.1.14–v0.1.17
// M5, M6, D2). Each test failed against the sshu that wrote "Enter save",
// drew the key reference's keys in the cursor's colour, and listed a key that
// could not run as brightly as one that could.

// sgr is the escape sequence a colour opens with.
func sgr(params string) string { return "\x1b[" + params + "m" }

// A hint and the footer write key:description, no space around the colon and
// one space between items (tdp M5).
func TestHintsAndTheFooterWriteKeyColonDescription(t *testing.T) {
	hint := ansi.Strip(hintLegend([][2]string{{"j/k", "move"}, {"Enter", "run"}, {"Esc", "close"}}))
	if hint != " j/k:move Enter:run Esc:close " {
		t.Errorf("hint = %q", hint)
	}
	footer := ansi.Strip(keyLegend([][2]string{{"Space", "menu"}, {"?", "help"}, {"q", "quit"}}, 30))
	if strings.TrimRight(footer, " ") != " Space:menu ?:help q:quit" {
		t.Errorf("footer = %q", footer)
	}
	if n := ansi.StringWidth(footer); n != 30 {
		t.Errorf("the footer should still fill its width: %d", n)
	}
	// A position is not a key: no colon, and nothing in the key colour.
	if got := ansi.Strip(hintLegend([][2]string{{"", "3 of 12"}, {"Esc", "close"}})); got != " 3 of 12 Esc:close " {
		t.Errorf("position = %q", got)
	}
}

// The key is Blue, and the colon goes with the description in Overlay0 (tdp
// D2). The values are pinned: an expectation derived from the variables would
// agree with whatever the code says.
func TestTheColonIsDrawnWithTheDescription(t *testing.T) {
	withColour(t)
	if focusColor != "#89b4fa" || dimColor != "#6c7086" || textColor != "#cdd6f4" {
		t.Fatalf("the palette moved: key %s, hint %s, text %s", focusColor, dimColor, textColor)
	}
	blue, overlay := ansiOf(t, focusColor), ansiOf(t, dimColor)
	for name, got := range map[string]string{
		"hint":   hintLegend([][2]string{{"Enter", "save"}}),
		"footer": keyLegend([][2]string{{"Enter", "save"}}, 20),
	} {
		if !strings.Contains(got, sgr(blue)+"Enter") {
			t.Errorf("%s: the key should be Blue: %q", name, got)
		}
		if !strings.Contains(got, sgr(overlay)+":save") {
			t.Errorf("%s: the colon and description should be Overlay0: %q", name, got)
		}
	}
	if !strings.Contains(hintLegend([][2]string{{"", "3 of 12"}}), sgr(overlay)+"3 of 12") {
		t.Error("a position is drawn dim")
	}
}

// The key reference: keys Blue, descriptions Text, two columns with no colon;
// a key that is there but cannot run now is listed in the dim register a
// disabled menu row uses (tdp M5, M6, D2).
func TestTheKeyReferenceDimsWhatCannotRunNow(t *testing.T) {
	withColour(t)
	blue, text, dim := ansiOf(t, focusColor), ansiOf(t, textColor), ansiOf(t, dimColor)
	m := sized(sample(), 100, 30)
	m.help.open(1, "test", []helpEntry{{key: "a", desc: "can run"}, {key: "b", desc: "cannot run now", dim: true}})
	m = settle(m)
	v := m.help.view()
	for _, want := range []string{sgr(blue) + "  a", sgr(text) + "can run", sgr(dim) + "  b", sgr(dim) + "cannot run now"} {
		if !strings.Contains(v, want) {
			t.Errorf("the reference is missing %q:\n%q", want, v)
		}
	}
	if strings.Contains(v, sgr(ansiOf(t, handColor))) {
		t.Error("handColor is the cursor, not a key")
	}
	if strings.Contains(ansi.Strip(v), "a:") || strings.Contains(ansi.Strip(v), "[a]") {
		t.Error("a key reference key has no colon and no brackets")
	}
}

// While bytes move, [H]ost and [D]isconnect are dimmed in the menu, and ? — which
// reads the same rows — dims them too instead of listing them as pressable.
func TestTheFileTransferReferenceDimsWhatTheMenuDims(t *testing.T) {
	m := busy(sftpFixture(t, 100, 26))
	m.sftp.focus = panelLeftFiles
	m = pressA(m, "j", "a")
	ref := m.panelKeyReference()
	for _, k := range []string{keySelectHost, "D"} {
		if !hasDimmedKey(ref, k) {
			t.Errorf("%q should be listed dimmed while a transfer runs", k)
		}
	}
	for _, e := range ref {
		if e.key == "r" && e.dim {
			t.Error("a row that can run is not dimmed")
		}
	}
	m = pressA(m, "?")
	if !m.help.isActive() {
		t.Fatal("? should open the key reference")
	}
}

// Jobs: c is dimmed on a job that has finished, and with no job at all there
// is nothing to move to, open or cancel. The hint lists only what works now.
func TestJobsListsCancelOnlyForAJobThatCanStop(t *testing.T) {
	m := appWith(sample(), nil)
	m.transfers.jobs = append(m.transfers.jobs, runningJob(1, 1, 10), &transferJob{id: 2, label: "done", files: 1})
	m.transfers.jobs[1].state.Store(int32(xferDone))
	m.transfersUI.open(1, len(m.transfers.jobs))
	m = settle(m)

	cancel := func(m AppModel) (helpEntry, bool) {
		_, ref := m.popupHelp()
		for _, e := range ref {
			if e.key == "c" {
				return e, true
			}
		}
		return helpEntry{}, false
	}
	if e, ok := cancel(m); !ok || e.dim {
		t.Errorf("c on a running job should be listed bright: %+v %v", e, ok)
	}
	if !strings.Contains(ansi.Strip(m.transfersUI.view(m.transfers.jobs)), "c:cancel") {
		t.Error("the hint should offer c on a running job")
	}
	m = pressA(m, "j")
	if e, ok := cancel(m); !ok || !e.dim {
		t.Errorf("c on a finished job should be listed dimmed: %+v %v", e, ok)
	}
	if strings.Contains(ansi.Strip(m.transfersUI.view(m.transfers.jobs)), "c:cancel") {
		t.Error("the hint should not offer c on a finished job")
	}

	m.transfers.jobs = nil
	_, ref := m.popupHelp()
	for _, e := range ref {
		if e.key == "c" || e.key == "Enter" || e.key == "j/k" {
			t.Errorf("with no job, %q has nothing to act on", e.key)
		}
	}
}

// Tab does not move between panels on the ssh tab (a deviation from K2): that
// is not "cannot run now", it is not there, so the ssh tab does not list it.
func TestTabIsListedOnlyWhereItMovesBetweenPanels(t *testing.T) {
	has := func(ref []helpEntry) bool {
		for _, e := range ref {
			if e.key == "Tab" {
				return true
			}
		}
		return false
	}
	if !has(appWith(sample(), nil).panelKeyReference()) {
		t.Error("Manage should list Tab")
	}
	ssh := appWith(sample(), nil)
	ssh.tab = tabSSH
	if has(ssh.panelKeyReference()) {
		t.Error("the ssh tab should not list Tab")
	}
}

// The layout panel's menu only describes it, so its keys come from the key
// reference: Enter asks for columns only on custom (tdp M4, M6).
func TestTheLayoutReferenceDimsEnterOffCustom(t *testing.T) {
	m := appWith(sample(), nil)
	m.tab = tabSSH
	m.ssh.setFocus(panelLayout)
	m.ssh.layout = layoutHorizontal
	if !hasDimmedKey(m.panelKeyReference(), "Enter") {
		t.Error("Enter should be dimmed off custom")
	}
	m.ssh.layout = layoutCustom
	for _, e := range m.panelKeyReference() {
		if e.key == "Enter" && e.dim {
			t.Error("Enter should be bright on custom")
		}
	}
	if _, ok := findEntry(m.panelKeyReference(), "j/k"); !ok {
		t.Error("the layout keys should be listed")
	}
}

func findEntry(ref []helpEntry, key string) (helpEntry, bool) {
	for _, e := range ref {
		if e.key == key {
			return e, true
		}
	}
	return helpEntry{}, false
}

// With no mark on this side the mark actions have nothing to be about: not in
// the menu, not in ?, and their letters do nothing (tdp M6).
func TestTheMarkActionsNeedAMark(t *testing.T) {
	m := sftpFixture(t, 100, 26)
	m.sftp.focus = panelLeftFiles
	m = pressA(m, "j")
	for _, k := range []string{"T", "X", "C"} {
		if _, ok := menuRow(m.sftpMenuItems(), k); ok {
			t.Errorf("%q is offered with nothing marked", k)
		}
		if _, ok := findEntry(m.panelKeyReference(), k); ok {
			t.Errorf("? lists %q with nothing marked", k)
		}
		if got := pressA(m, k); got.toast.isActive() || got.confirm.isActive() {
			t.Errorf("%q with nothing marked should do nothing", k)
		}
	}
	m = pressA(m, "a")
	for _, k := range []string{"T", "X", "C"} {
		if it, ok := menuRow(m.sftpMenuItems(), k); !ok || it.disabled {
			t.Errorf("%q should be offered once something is marked", k)
		}
	}
}

// A regular file can be edited; its row is not dimmed.
func TestEditIsLiveOnAFile(t *testing.T) {
	m := sftpFixture(t, 100, 26)
	m.sftp.focus = panelLeftFiles
	m = pressA(m, "j") // deploy.sh
	if it, ok := menuRow(m.sftpMenuItems(), "e"); !ok || it.disabled {
		t.Errorf("edit should be live on a file: %+v", it)
	}
}

// A menu row run by Enter carries it in the label, in front (tdp M5, D4).
func TestAnEnterRowCarriesItsKeyInTheLabel(t *testing.T) {
	if got := bracketHotkey("Connect", "enter"); got != "[Enter] Connect" {
		t.Errorf("got %q", got)
	}
	m := appWith(sample(), nil)
	m = pressA(m, "1", "enter", " ")
	v := ansi.Strip(m.spaceMenu.view())
	if !strings.Contains(v, "[Enter] Connect") {
		t.Errorf("the hosts menu should write [Enter] Connect:\n%s", v)
	}
	if strings.Contains(v, "Enter .") {
		t.Errorf("the key is in the label now, not the description:\n%s", v)
	}
}

// Errors has a cursor and Enter opens the entry: an item operation, so it is in
// the Space menu, and ? lists it (tdp M3, M4).
func TestErrorsOffersEnterInItsMenu(t *testing.T) {
	m := appWith(sample(), nil)
	m.errors.errorf("prod-web-01", "deploy", "Host key verification failed", "SHA256:abc")
	m = pressA(m, "1", "j", "j", "j", "j", "enter")
	if m.pref.item != prefErrors {
		t.Fatalf("setup: expected the errors content, got item %d", m.pref.item)
	}
	if it, ok := menuRow(m.menuItems(), "enter"); !ok || it.label != "Open" {
		t.Fatalf("Errors should offer [Enter] Open: %+v", it)
	}
	if _, ok := findEntry(m.panelKeyReference(), "Enter"); !ok {
		t.Error("? should list Enter on Errors")
	}
	m = pressA(m, " ")
	for i, it := range m.spaceMenu.items {
		if it.key == "enter" {
			m.spaceMenu.cursor = i
		}
	}
	m = pressA(m, "enter")
	if !m.viewer.isActive() || !strings.Contains(m.viewer.title, "prod-web-01") {
		t.Error("the menu row should open the entry, as Enter does")
	}
}

// Keys in a sentence are in square brackets (tdp M5), and the two sentences that
// named keys sshu no longer has now name the ones it does: [H] shows or hides a
// cell (§11.56), and custom asks only for columns (§11.31).
func TestSentencesBracketTheirKeys(t *testing.T) {
	m := appWith(sample(), nil)
	m.tab = tabSSH
	grid := ansi.Strip(strings.Join(m.ssh.gridEmpty(80, 10), "\n"))
	if !strings.Contains(grid, "[H] in [1] shows or hides") || !strings.Contains(grid, "[Enter]") {
		t.Errorf("the empty grid still names the old key:\n%s", grid)
	}
	if strings.Contains(grid, "Tab") {
		t.Errorf("Tab does not toggle a cell on the ssh tab:\n%s", grid)
	}
	m.ssh.setFocus(panelLayout)
	var said []string
	for _, it := range m.menuItems() {
		said = append(said, it.label)
	}
	joined := strings.Join(said, "\n")
	if !strings.Contains(joined, "[Enter] on custom asks for the number of columns") ||
		strings.Contains(joined, "rows ×") {
		t.Errorf("the layout menu says the wrong thing:\n%s", joined)
	}
	hosts := ansi.Strip(strings.Join(sized(nil, 100, 24).hosts.emptyState(60, 10), "\n"))
	if !strings.Contains(hosts, "[Space]") {
		t.Errorf("the empty state should bracket Space:\n%s", hosts)
	}
}

// Not only directories: a fifo has no text to edit either, so its row dims. A
// symlink is followed by the edit, so it is left live for the edit to judge.
func TestEditDimsWhatIsNotARegularFile(t *testing.T) {
	m := sftpFixture(t, 100, 26)
	m.sftp.focus = panelLeftFiles
	s := &m.sftp.sides[sideLeft]
	if err := syscall.Mkfifo(filepath.Join(s.cwd, "pipe"), 0o644); err != nil {
		t.Skipf("no fifo here: %v", err)
	}
	if err := os.Symlink(filepath.Join(s.cwd, "deploy.sh"), filepath.Join(s.cwd, "link.sh")); err != nil {
		t.Fatal(err)
	}
	s.reload()
	on := func(name string) menuItem {
		for i := range s.rowCount() {
			if e, _ := s.rowAt(i); e.Name == name {
				s.cursor = i
			}
		}
		it, _ := menuRow(m.sftpMenuItems(), "e")
		return it
	}
	if !on("pipe").disabled {
		t.Error("edit should be dimmed on a fifo")
	}
	if on("link.sh").disabled {
		t.Error("edit should stay live on a symlink; the edit follows it")
	}
}
