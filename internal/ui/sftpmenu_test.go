package ui

import (
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
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
	for _, k := range []string{"/", "A", "T", "X", "C", keySelectHost, "P"} {
		if !hasKey(got[menuPanelRegion], k) {
			t.Errorf("%q should be a panel action, region has %v",
				k, got[menuPanelRegion])
		}
	}
}

// One region stays flat: a header over a single group is noise, and the no-host
// menu is one row that needs no explaining.
func TestSFTPMenuStaysFlatWithOneRegion(t *testing.T) {
	m := New(sample(), nil, store.DefaultConfig())
	next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 26})
	m = settle(next.(AppModel))
	m.tab = tabFT
	m.sftp.focus = panelLeftFiles

	for _, it := range m.sftpMenuItems() {
		if it.header || it.separator {
			t.Errorf("a one-row menu should carry no labels, found %q", it.label)
		}
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
