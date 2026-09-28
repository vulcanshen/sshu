package ui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// helpPopup is what ? opens, and it only reads (§11.56): from a panel the key
// reference, from a popup that popup's own keys (tdp K6) — what can be pressed
// in that one box and what it does.
type helpPopup struct {
	anim    popupAnimator
	title   string
	entries []helpEntry
	top     int
	layer   int
	screenW int
	screenH int
}

func newHelpPopup() helpPopup { return helpPopup{anim: newPopupAnimator("help")} }

func (m helpPopup) isActive() bool      { return m.anim.isActive() }
func (m helpPopup) isInteractive() bool { return m.anim.isInteractive() }
func (m *helpPopup) open(layer int, title string, entries []helpEntry) tea.Cmd {
	m.layer, m.top, m.title, m.entries = layer, 0, title, entries
	return m.anim.open()
}
func (m *helpPopup) close() tea.Cmd   { return m.anim.close() }
func (m *helpPopup) setSize(w, h int) { m.screenW, m.screenH = w, h }

// helpEntry is one line: a section header (key == "") or a key/description pair.
type helpEntry struct{ key, desc string }

// The fixed parts of a panel's key reference (tdp M4, K6). What ? shows on a
// panel is that panel's own keys first (panelKeyReference), then these: the
// core keys, which a user has to hold to walk the app; on the ssh tab the
// grid's chords — a cell hands ? to the remote, so this is the only place to
// learn them; and the navigation letters.
var coreKeyReference = []helpEntry{
	{"", "core keys"},
	{"M · F · S", "switch tab"},
	{"1-9", "panel of this tab"},
	{"Tab", "next panel in this tab"},
	{"Enter", "confirm / connect"},
	{"Esc", "close popup / cancel"},
	{"Space", "what can I do here"},
	{"?", "this list / a popup's own keys"},
	{"q", "quit"},
	{"Ctrl+C", "quit (twice: at once)"},
}

var gridKeyReference = []helpEntry{
	{"", "ssh grid"},
	{"Alt+arrows", "move between cells"},
	{"Alt+Z", "bigger: zoom panel, then zoom max"},
	{"Alt+Enter", "nested sshu: lock/release, or the whole chain"},
	{"Alt+Esc", "back out one layer at a time"},
	{"PgUp · PgDn", "page this cell's history"},
	{"Alt+v", "select and copy out of this cell"},
	{"hjkl · u · d", "…move there, v / V select, y copies"},
	{"w · e · b", "…by word, forward and back"},
	{"0 · $", "…to either end of the line"},
}

var navKeyReference = []helpEntry{
	{"", "navigate"},
	{"j · k", "move cursor"},
	{"u · d", "half a page"},
	{"gg · G", "first / last"},
}

func (m *helpPopup) update(msg tea.KeyMsg) {
	if !m.anim.isInteractive() {
		return
	}
	// A viewport, so the same keys scroll rather than move a cursor — and it
	// does not wrap, for the same reason [6] does not.
	m.top = moveScroll(m.top, max(0, len(m.entries)-m.visible()), msg.String(), m.visible())
}

// visible is how many content lines fit; the box costs 4 rows of chrome.
func (m helpPopup) visible() int { return max(1, min(len(m.entries), m.screenH-6)) }

func (m helpPopup) view() string {
	keyW := 0
	for _, e := range m.entries {
		keyW = max(keyW, dispW(e.key))
	}
	innerW := popupInnerW(m.screenW)

	dim := lipgloss.NewStyle().Foreground(dimColor)
	key := lipgloss.NewStyle().Foreground(handColor)
	txt := lipgloss.NewStyle().Foreground(textColor)

	vis := m.visible()
	end := min(len(m.entries), m.top+vis)
	rows := make([]string, 0, vis)
	for _, e := range m.entries[min(m.top, len(m.entries)):end] {
		if e.key == "" {
			rows = append(rows, dim.Render(padRight(" "+e.desc, innerW)))
			continue
		}
		rows = append(rows, key.Render(padRight("  "+e.key, keyW+4))+
			txt.Render(padRight(e.desc, innerW-keyW-4)))
	}

	pairs := [][2]string{{"?", "close"}}
	if len(m.entries) > vis {
		pairs = append([][2]string{{"j/k", "scroll"}}, pairs...)
	}
	hint := hintLegend(pairs)
	return drawPopupBox(popupLayerColor(m.layer), " "+glyphHelp+" "+m.title+" ", hint,
		animRows(m.anim, capRows(rows, m.screenH)), innerW)
}
