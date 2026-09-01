package ui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// confirmAction is what the caller does when the user commits.
type confirmAction int

const (
	confirmNone confirmAction = iota
	confirmConnect
	confirmDelete
	confirmClose         // kill a live session
	confirmDuplicate     // open a second session to the same host
	confirmTransfer      // start a planned transfer, overwrites and all
	confirmQuit          // leave sshu while sessions are live
	confirmDeleteItem    // erase the path under the cursor in tab [2]
	confirmDeleteMarks   // erase every marked path on one side of tab [2]
	confirmDeleteCred    // remove a credential from credentials.yaml
	confirmEditBinary    // open something that does not look like text
	confirmEditOverwrite // write back over a file that changed underneath
)

// confirmPopup is the message class (§6.1): a short question with one yes and
// one no. It is NOT a menu — a menu is "pick one of N", and blurring the two
// would make Enter mean different things on different floats.
type confirmPopup struct {
	anim    popupAnimator
	glyph   string
	title   string
	lines   []string
	accept  string // what Enter does, shown in the hint
	warn    bool   // render the first line in the warning colour
	action  confirmAction
	target  string // host name the action applies to
	layer   int
	screenW int
	screenH int
}

func newConfirmPopup() confirmPopup {
	return confirmPopup{anim: newPopupAnimator("confirm")}
}

func (m confirmPopup) isActive() bool      { return m.anim.isActive() }
func (m confirmPopup) isInteractive() bool { return m.anim.isInteractive() }
func (m *confirmPopup) close() tea.Cmd     { return m.anim.close() }
func (m *confirmPopup) setSize(w, h int)   { m.screenW, m.screenH = w, h }

func (m *confirmPopup) ask(c confirmPopup, layer int) tea.Cmd {
	anim, w, h := m.anim, m.screenW, m.screenH
	*m = c
	m.anim, m.screenW, m.screenH = anim, w, h
	m.layer = layer
	return m.anim.open()
}

// commit reports whether the user accepted. Esc is handled by the caller, since
// cancel is one app-wide role and must not be re-implemented per popup (§4.3).
func (m confirmPopup) commit(msg tea.KeyMsg) bool {
	return m.anim.isInteractive() && msg.String() == "enter"
}

func (m confirmPopup) view() string {
	w := dispW(m.title) + 6
	for _, l := range m.lines {
		w = max(w, dispW(l)+4)
	}
	innerW := popupInnerW(m.screenW, w)

	txt := lipgloss.NewStyle().Foreground(textColor)
	dim := lipgloss.NewStyle().Foreground(dimColor)
	red := lipgloss.NewStyle().Foreground(warnColor)

	rows := make([]string, 0, len(m.lines))
	for i, l := range m.lines {
		style := dim
		if i == 0 {
			style = txt
			if m.warn {
				style = red
			}
		}
		rows = append(rows, style.Render(padRight("  "+l, innerW)))
	}

	hint := hintLegend([][2]string{{"Enter", m.accept}, {"Esc", "cancel"}})
	return drawPopupBox(popupLayerColor(m.layer), " "+m.glyph+" "+m.title+" ", hint,
		animRows(m.anim, capRows(rows, m.screenH)), innerW)
}
