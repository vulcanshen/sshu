package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/vulcanshen/sshu/internal/store"
)

// regions splits a menu into its labelled groups, so a test can talk about
// "what is in the item region" rather than about indexes.
func regions(items []menuItem) map[string][]string {
	out := map[string][]string{}
	cur := ""
	for _, it := range items {
		switch {
		case it.separator:
		case it.header:
			cur = it.label
		default:
			out[cur] = append(out[cur], it.key)
		}
	}
	return out
}

// The menu is two labelled groups: what happens to the row under the cursor,
// and what happens to this side.
func TestSFTPMenuHasItemAndPanelRegions(t *testing.T) {
	m := sftpFixture(t, 100, 26)
	m.sftp.focus = panelLeftFiles
	m = pressA(m, "j") // deploy.sh

	items := m.sftpMenuItems()
	if items[0].label != menuItemRegion || !items[0].header {
		t.Fatalf("the first row should be the item region, got %q", items[0].label)
	}

	// A separator between the groups, and the panel label after it.
	sep := -1
	for i, it := range items {
		if it.separator {
			sep = i
		}
	}
	if sep < 0 {
		t.Fatal("the two regions should be divided by a rule")
	}
	if items[sep+1].label != menuPanelRegion || !items[sep+1].header {
		t.Errorf("after the rule comes the panel label, got %q", items[sep+1].label)
	}

	got := regions(items)
	for _, k := range []string{"enter", "a", "r", "t", "x"} {
		if !hasKey(got[menuItemRegion], k) {
			t.Errorf("%q should be an item action, region has %v",
				k, got[menuItemRegion])
		}
	}
	for _, k := range []string{"/", "A", "R", "T", "X", "C", keySelectHost, "D", "J"} {
		if !hasKey(got[menuPanelRegion], k) {
			t.Errorf("%q should be a panel action, region has %v",
				k, got[menuPanelRegion])
		}
	}
}

// A side with no host can do exactly one thing, so Space does it instead of
// drawing a menu whose only row points at it. This holds on the marks panel
// too: the side is what has no host, and its marks panel is just as empty.
func TestSpaceOnAHostlessSideOpensTheHostListItself(t *testing.T) {
	m := New(sample(), nil, store.DefaultConfig())
	next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 26})
	m = settle(next.(AppModel))
	m.tab = tabFT

	for _, p := range []sftpPanel{panelLeftFiles, panelLeftMarks, panelRightFiles} {
		at := m
		at.sftp.focus = p
		at = pressA(at, " ")
		if !at.hostPicker.isActive() {
			t.Errorf("panel %v: Space should have opened the host list", p)
		}
		if at.spaceMenu.isActive() {
			t.Errorf("panel %v: a menu of one row is not an answer", p)
		}
		// It is the first float over the panel, so Esc unwinds straight back
		// there rather than to a menu that was never opened (§6.4).
		if at.hostPicker.layer != 1 {
			t.Errorf("panel %v: the picker should be layer 1, got %d", p, at.hostPicker.layer)
		}
		if at = pressA(at, "esc"); at.hostPicker.isActive() || at.spaceMenu.isActive() {
			t.Errorf("panel %v: Esc should leave nothing standing", p)
		}
	}
}

// Once the side has a host there is a real menu again — the shortcut is about
// having one answer, not about the file transfer tab being different. And it
// reads the side you are ON: with one side connected and the other not, the
// same key gives a different answer on each, which is the whole point of a
// contextual entry key (§A.1).
func TestSpaceAnswersForTheSideYouAreOn(t *testing.T) {
	m := sftpFixture(t, 100, 26)
	m.sftp.sides[sideRight].disconnect()

	m.sftp.focus = panelLeftFiles
	if at := pressA(m, " "); !at.spaceMenu.isActive() || at.hostPicker.isActive() {
		t.Error("the connected side should get the Space menu, not the host list")
	}
	m.sftp.focus = panelRightFiles
	if at := pressA(m, " "); !at.hostPicker.isActive() || at.spaceMenu.isActive() {
		t.Error("the hostless side should get the host list, not the menu")
	}
}

// With no row there is nothing for a row action to be about, so the item region
// goes away — and the letters go with it, or the menu would be lying.
func TestSFTPItemActionsNeedARow(t *testing.T) {
	m := sftpFixture(t, 100, 26)
	empty := filepath.Join(m.sftp.sides[sideLeft].home, "empty")
	if err := os.Mkdir(empty, 0o755); err != nil {
		t.Fatal(err)
	}
	m.sftp.focus = panelLeftFiles
	m.sftp.sides[sideLeft].open(empty)
	if m.sftp.sides[sideLeft].rowCount() != 0 {
		t.Fatal("setup: the directory should be empty")
	}

	for _, it := range m.sftpMenuItems() {
		if hasKey([]string{"enter", "a", "r", "t", "x"}, it.key) {
			t.Errorf("%q is offered with no row to act on", it.key)
		}
		if it.header || it.separator {
			t.Errorf("with one region there should be no labels, found %q", it.label)
		}
	}
	// The hotkey has to agree with the menu, or one of them is lying (§4.2).
	if after := pressA(m, "x"); after.confirm.isActive() {
		t.Error("x asked to delete something that is not there")
	}
	// Panel actions still work — there is still a panel.
	if after := pressA(m, "A"); !after.input.isActive() {
		t.Error("A should still offer to make something in an empty directory")
	}
}

// `x` is the row, `X` is the marks — the same lower/upper split as t and T, and
// deliberately NOT on `d`, which belongs to navigation.
func TestDeleteCursorAndDeleteMarksAreDifferentKeys(t *testing.T) {
	m := sftpFixture(t, 100, 26)
	m.sftp.focus = panelLeftFiles
	m = pressA(m, "j") // deploy.sh, deliberately NOT marked
	victim, _ := m.sftp.cur().cursorPath()
	m = pressA(m, "j", "a") // mark main.go instead
	marked := m.sftp.sides[sideLeft].marks[0]
	m = pressA(m, "k") // back onto deploy.sh

	// It asks first, and cancelling changes nothing.
	m = pressA(m, "x")
	if !m.confirm.isActive() || m.confirm.action != confirmDeleteItem {
		t.Fatal("x must ask before erasing anything")
	}
	m = pressA(m, "esc")
	if _, err := os.Stat(victim); err != nil {
		t.Fatal("cancelling the delete still removed it")
	}

	m = pressA(m, "x", "enter")
	if _, err := os.Stat(victim); err == nil {
		t.Error("x should have deleted the row under the cursor")
	}
	if _, err := os.Stat(marked); err != nil {
		t.Error("x deleted a mark — that is X's job")
	}
	if n := len(m.sftp.sides[sideLeft].marks); n != 1 {
		t.Errorf("the surviving mark was dropped, %d left", n)
	}
}

// Deleting a marked row takes the mark with it: a mark on something that is gone
// fails later, somewhere the reason is no longer on screen.
func TestDeletingAMarkedRowDropsItsMark(t *testing.T) {
	m := sftpFixture(t, 100, 26)
	m.sftp.focus = panelLeftFiles
	m = pressA(m, "j", "a") // mark deploy.sh, cursor still on it
	if len(m.sftp.sides[sideLeft].marks) != 1 {
		t.Fatal("setup: expected one mark")
	}

	m = pressA(m, "x", "enter")
	if n := len(m.sftp.sides[sideLeft].marks); n != 0 {
		t.Errorf("%d marks still point at a deleted path", n)
	}
}

// All three tabs word their regions the same way. A menu whose headers differ
// from another's reads as a different KIND of menu rather than as the same one.
func TestEveryTabWordsItsRegionsTheSameWay(t *testing.T) {
	seen := map[string]bool{}
	collect := func(items []menuItem) {
		for _, it := range items {
			if it.header {
				seen[it.label] = true
			}
		}
	}

	collect(appWith(sample(), nil).menuItems())

	s := sftpFixture(t, 100, 26)
	s.sftp.focus = panelLeftFiles
	collect(s.sftpMenuItems())

	h := openOne(t)
	next, _ := h.Update(keyMsg("alt+esc"))
	h = settle(next.(AppModel))
	if h.ssh.focus != panelSessions {
		t.Fatalf("setup: alt+esc should leave the pty, focus=%d", h.ssh.focus)
	}
	collect(h.sshMenuItems())

	for label := range seen {
		if label != menuItemRegion && label != menuPanelRegion {
			t.Errorf("a menu region is worded %q, want one of the two constants", label)
		}
	}
	if !seen[menuItemRegion] || !seen[menuPanelRegion] {
		t.Errorf("expected both regions across the three tabs, saw %v", seen)
	}
}

// menuLine pulls the one rendered row that carries text out of the menu box.
// Reading the STRUCT would let a render that ignores `disabled` pass.
func menuLine(t *testing.T, view, text string) string {
	t.Helper()
	for _, ln := range strings.Split(view, "\n") {
		if strings.Contains(ansi.Strip(ln), text) {
			return ln
		}
	}
	t.Fatalf("no menu row contains %q in:\n%s", text, ansi.Strip(view))
	return ""
}

// busy parks a job that is definitionally still moving. A real local-to-local
// copy of a one-byte file is over before the next keystroke.
func busy(m AppModel) AppModel {
	m.transfers.jobs = append(m.transfers.jobs, runningJob(1, 50, 100))
	return m
}

// Swapping the filesystem under a side would break a transfer in flight —
// both sides are on every one of them. The two actions that do that stay in
// the menu and DIM rather than disappearing: they belong on this panel, they
// are only unavailable this second, and a row that vanishes teaches that the
// action does not live here at all.
func TestASideCannotChangeItsFilesystemMidTransfer(t *testing.T) {
	withColour(t)
	m := busy(sftpFixture(t, 100, 26))
	m.sftp.focus = panelLeftFiles

	frozen := map[string]bool{keySelectHost: true, "D": true}
	seen := 0
	for _, it := range m.sftpMenuItems() {
		if it.header || it.separator {
			continue
		}
		if it.disabled != frozen[it.key] {
			t.Errorf("%q disabled=%v, want %v", it.key, it.disabled, frozen[it.key])
		}
		if frozen[it.key] {
			seen++
		}
	}
	if seen != len(frozen) {
		t.Fatalf("both frozen rows should still be listed, found %d", seen)
	}

	// And it has to reach the screen. The probe is the LABEL's ink: every row
	// carries a dim hint column, so "contains dim" would pass on a row that
	// never dimmed at all.
	m = pressA(m, " ")
	view := m.spaceMenu.view()
	live, text := ansiOf(t, dimColor), ansiOf(t, textColor)
	if row := menuLine(t, view, "[H]ost"); strings.Contains(row, text) ||
		!strings.Contains(row, live) {
		t.Errorf("the frozen row's label should be dim, not ordinary text:\n%q", row)
	}
	if row := menuLine(t, view, "[T]ransfer all marks"); !strings.Contains(row, text) {
		t.Errorf("a live row must keep its ordinary ink:\n%q", row)
	}

	// The cursor can still land on it — that is how you find out WHY it is dim
	// — but the bar drops to the app's "highlighted, not live" register rather
	// than lighting up like a row that would run.
	for i, it := range m.spaceMenu.items {
		if it.key == keySelectHost {
			m.spaceMenu.cursor = i
		}
	}
	row := menuLine(t, m.spaceMenu.view(), "[H]ost")
	if !strings.Contains(row, ansiBgOf(t, borderDim)) {
		t.Errorf("the cursor on a frozen row should wear the quiet bar:\n%q", row)
	}
	if strings.Contains(row, ansiBgOf(t, handColor)) {
		t.Errorf("the live cursor bar promises the row will run:\n%q", row)
	}
}

// Nothing moving, nothing frozen — the dimming is about the transfer, not
// about the two actions being special.
func TestTheSameRowsAreLiveWhenNothingIsMoving(t *testing.T) {
	m := sftpFixture(t, 100, 26)
	m.sftp.focus = panelLeftFiles
	for _, it := range m.sftpMenuItems() {
		if it.disabled {
			t.Errorf("%q is dim with nothing running", it.key)
		}
	}
	if m = pressA(m, keySelectHost); !m.hostPicker.isActive() {
		t.Error("S should still open the host picker when the queue is idle")
	}
}

// The letter and the menu row are the same action (§4.2), so the refusal has
// to meet both — and it says where to stop the transfer rather than just
// declining. The menu is left standing: a refusal is not a commit, and the row
// it refused is still on screen explaining itself.
func TestTheFrozenActionRefusesFromBothTheKeyAndTheMenu(t *testing.T) {
	m := busy(sftpFixture(t, 100, 26))
	m.sftp.focus = panelLeftFiles
	host := m.sftp.sides[sideLeft].host

	// The bare letter, at the panel.
	after := pressA(m, keySelectHost)
	if after.hostPicker.isActive() {
		t.Error("H opened the host picker while a transfer was running")
	}
	if !strings.Contains(ansi.Strip(after.View()), "J]obs") {
		t.Errorf("the refusal must say where to stop it:\n%s", ansi.Strip(after.View()))
	}

	// The same action committed from the menu, with Enter on its row.
	inMenu := pressA(m, " ")
	for i, it := range inMenu.spaceMenu.items {
		if it.key == keySelectHost {
			inMenu.spaceMenu.cursor = i
		}
	}
	inMenu = pressA(inMenu, "enter")
	if inMenu.hostPicker.isActive() {
		t.Error("Enter on the dimmed row ran it anyway")
	}
	if !inMenu.spaceMenu.isActive() {
		t.Error("a refusal is not a commit — the menu should still be up")
	}
	if !strings.Contains(ansi.Strip(inMenu.View()), "J]obs") {
		t.Error("the menu path must give the same reason as the key")
	}

	// Neither path may have moved the side.
	for _, got := range []AppModel{after, inMenu} {
		if got.sftp.sides[sideLeft].host != host {
			t.Errorf("the side changed host under a running transfer: %q",
				got.sftp.sides[sideLeft].host)
		}
	}
}

// D is frozen by the same rule, and for the same reason.
func TestDisconnectIsFrozenMidTransferToo(t *testing.T) {
	m := busy(sftpFixture(t, 100, 26))
	m.sftp.focus = panelLeftFiles
	host := m.sftp.sides[sideLeft].host

	m = pressA(m, "D")
	if m.sftp.sides[sideLeft].host != host {
		t.Fatal("D pulled the floor out from under a running transfer")
	}
	if !strings.Contains(ansi.Strip(m.View()), "J]obs") {
		t.Errorf("the refusal should point at where a transfer is stopped:\n%s",
			ansi.Strip(m.View()))
	}
}
