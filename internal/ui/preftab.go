package ui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/vulcanshen/sshu/internal/store"
)

// The preference tab is where sshu's own data lives: the hosts, the
// credentials they can share, and the app log. A fixed side nav on the left,
// one content panel on the right. The nav cursor IS the choice — moving it
// swaps the content immediately, and Enter only moves the keyboard over —
// because a cursor that needs a second key to mean anything is a cursor that
// looks broken while you browse with it.

type prefItem int

const (
	prefHosts prefItem = iota
	prefCreds
	prefLogs
	prefExport
	prefImport
	prefItemCount
)

func (p prefItem) label() string {
	switch p {
	case prefCreds:
		return "Credentials"
	case prefLogs:
		return "Logs"
	case prefExport:
		return "Export"
	case prefImport:
		return "Import"
	}
	return "Hosts"
}

// prefSections groups the nav rows under category headers — kbu's sidebar
// shape. A header is decoration: the cursor never lands on one, and j/k walk
// the items straight through it. SSH is the data connections run on, Events
// is what happened, Operation is what sshu can do to its own config as a
// whole.
var prefSections = []struct {
	header string
	items  []prefItem
}{
	{"SSH", []prefItem{prefHosts, prefCreds}},
	{"Events", []prefItem{prefLogs}},
	// Operation (Export / Import) is MASKED until its design settles: the
	// enum keeps the tail values, the pages stay compiled and tested, but
	// the nav neither draws the section nor stops on its items. Unmasking
	// is putting the row back:
	//	{"Operation", []prefItem{prefExport, prefImport}},
}

type prefPanel int

const (
	panelPrefNav     prefPanel = iota // [1]
	panelPrefContent                  // [2]
)

// Geometry: a fixed left column, for the same reason the ssh tab's is fixed
// (§1.2). 18 holds "credentials" plus its lead and a two-digit badge. Below
// the narrow line the two panels cannot both be useful, so the focused one
// takes the tab.
const (
	prefLeftW   = 18
	prefNarrowW = prefLeftW + 42
)

type prefModel struct {
	focus prefPanel
	item  prefItem // the nav cursor and the content shown: one thing
	w, h  int
}

func (m prefModel) narrow() bool { return m.w < prefNarrowW }

func (m prefModel) panes() (leftW, leftH, rightW, rightH int) {
	if m.narrow() {
		if m.focus == panelPrefNav {
			return m.w, m.h, 0, 0
		}
		return 0, 0, m.w, m.h
	}
	return prefLeftW, m.h, m.w - prefLeftW, m.h
}

func (m *prefModel) setSize(w, h int) { m.w, m.h = w, h }

// navKey moves the nav cursor. The content follows it; there is no separate
// "open" step to forget.
func (m *prefModel) navKey(k string) {
	// Count the items prefSections actually shows. The masked Operation
	// items keep the enum's tail, so the visible ones are exactly 0..n-1
	// and the index IS the item.
	n := 0
	for _, s := range prefSections {
		n += len(s.items)
	}
	m.item = prefItem(moveCursor(int(m.item), n, k, n))
}

func (m prefModel) panelTitle(p prefPanel) string {
	if p == panelPrefNav {
		// The nav holds sshu's own data and operations — the app's name is
		// the shortest honest label for "everything that is sshu's, not a
		// host's".
		return "[1] sshu"
	}
	return "[2] " + m.item.label()
}

// ---------------------------------------------------------------- app glue

// syncPrefSizes hands the content panel's outer size to whichever model is
// standing in it — on resize, focus change and nav movement alike, the same
// discipline sshModel.setFocus keeps.
func (m *AppModel) syncPrefSizes() {
	_, _, rightW, rightH := m.pref.panes()
	m.hosts.setSize(rightW, rightH)
	m.creds.setSize(rightW, rightH)
}

// prefShowed runs whenever the pref tab's content may have changed. Landing
// the LOGS content on screen is what reading it means, so that is the moment
// the unread count goes to zero — not a popup toggle, which no longer exists.
func (m *AppModel) prefShowed() {
	if m.tab == tabPref && m.pref.item == prefLogs {
		m.log.markRead()
	}
}

func (m AppModel) prefKey(k string) (tea.Model, tea.Cmd) {
	if m.pref.focus == panelPrefNav {
		if k == "enter" {
			m.pref.focus = panelPrefContent
			m.syncPrefSizes()
			return m, nil
		}
		m.pref.navKey(k)
		m.syncPrefSizes()
		m.prefShowed()
		return m, nil
	}
	switch m.pref.item {
	case prefCreds:
		return m.credsKey(k)
	case prefExport, prefImport:
		// The page claimed its keys in handleKey (textPage) before the global
		// vocabulary ran; nothing is left to do here.
		return m, nil
	case prefLogs:
		_, _, rightW, rightH := m.pref.panes()
		m.log.scrollKey(k, max(1, rightW-2), max(1, rightH-2))
		return m, nil
	}
	return m.hostsKey(k)
}

func (m AppModel) prefView() string {
	leftW, leftH, rightW, rightH := m.pref.panes()
	if leftW <= 0 {
		return m.prefContent(rightW, rightH)
	}
	if rightW <= 0 {
		return m.prefNav(leftW, leftH)
	}
	return joinHorizontal(m.prefNav(leftW, leftH), m.prefContent(rightW, rightH))
}

func (m AppModel) prefNav(w, h int) string {
	innerW, innerH := w-2, h-2
	rows := make([]string, 0, max(0, innerH))
	head := lipgloss.NewStyle().Foreground(focusColor)
	for _, sec := range prefSections {
		rows = append(rows, head.Render(padRight(" "+sec.header, innerW)))
		for _, it := range sec.items {
			rows = append(rows, m.prefNavRow(it, innerW))
		}
	}
	return panelChrome(innerW, fitLines(rows, innerW, innerH),
		m.pref.panelTitle(panelPrefNav), m.pref.focus == panelPrefNav)
}

// prefNavRow is one section row. The current one wears the cursor bar; the
// logs row carries the unread-error count, because news nobody is told about
// is news that did not arrive.
func (m AppModel) prefNavRow(it prefItem, innerW int) string {
	body := lipgloss.NewStyle().Foreground(textColor)
	tail := lipgloss.NewStyle().Foreground(warnColor)
	if it == m.pref.item {
		bar := lipgloss.NewStyle().Foreground(lipgloss.Color(baseHex)).Background(handColor)
		body, tail = bar, bar
	}
	badge := ""
	if it == prefLogs {
		if n := m.log.unreadErrors(); n > 0 {
			badge = itoa(n) + " "
		}
	}
	nameW := max(1, innerW-dispW(badge))
	return body.Render(padRight("  "+it.label(), nameW)) + tail.Render(badge)
}

func (m AppModel) prefContent(w, h int) string {
	focused := m.pref.focus == panelPrefContent
	title := m.pref.panelTitle(panelPrefContent)
	switch m.pref.item {
	case prefCreds:
		return m.creds.view(title, focused)
	case prefExport:
		innerW, innerH := w-2, h-2
		body := m.exportPage.body(exportIntro, exportWarn, "export", focused, innerW)
		return panelChrome(innerW, fitLines(body, innerW, innerH), title, focused)
	case prefImport:
		innerW, innerH := w-2, h-2
		body := m.importPage.body(importIntro, "", "import", focused, innerW)
		return panelChrome(innerW, fitLines(body, innerW, innerH), title, focused)
	case prefLogs:
		innerW, innerH := w-2, h-2
		// fitLines like every content body: the log empty state returns
		// fewer rows than the panel is tall, and on a narrow terminal there
		// is no neighbouring panel to prop the frame up.
		return panelChrome(innerW, fitLines(m.log.body(innerW, innerH), innerW, innerH), title, focused)
	}
	return m.hosts.view(title, focused)
}

// prefStatus is the tab-row slot: it describes the content being shown.
func (m AppModel) prefStatus() string {
	switch m.pref.item {
	case prefCreds:
		return m.creds.status()
	case prefExport:
		// What the bundle would hold, so the slot answers "is it worth it".
		return plural(len(m.hosts.hosts), "host") + " · " + plural(len(m.creds.creds), "credential")
	case prefImport:
		return "merge a " + store.BundleExt + " bundle"
	case prefLogs:
		switch n := len(m.log.entries); n {
		case 0:
			return "log empty"
		case 1:
			return "1 entry"
		default:
			return itoa(n) + " entries"
		}
	}
	return m.hosts.status()
}
