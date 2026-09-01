package ui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// helpPopup is the §A.2 non-contextual entry point: every global action the app
// has, reachable from any surface. Its completeness is the promise — a user who
// never read a README finds the whole global vocabulary here.
type helpPopup struct {
	anim    popupAnimator
	top     int
	layer   int
	screenW int
	screenH int
}

func newHelpPopup() helpPopup { return helpPopup{anim: newPopupAnimator("help")} }

func (m helpPopup) isActive() bool      { return m.anim.isActive() }
func (m helpPopup) isInteractive() bool { return m.anim.isInteractive() }
func (m *helpPopup) open(layer int) tea.Cmd {
	m.layer, m.top = layer, 0
	return m.anim.open()
}
func (m *helpPopup) close() tea.Cmd   { return m.anim.close() }
func (m *helpPopup) setSize(w, h int) { m.screenW, m.screenH = w, h }

// helpEntry is one line: a section header (key == "") or a key/description pair.
type helpEntry struct{ key, desc string }

// helpContent is the whole global vocabulary. The core keys are listed first
// because they are the five a user has to hold to walk the app (§A.0.Y).
var helpContent = []helpEntry{
	{"", "Core keys"},
	{"Alt+P/F/S", "switch tab"},
	{"1-9", "panel of this tab"},
	{"Tab", "next panel in this tab"},
	{"Enter", "confirm / connect"},
	{"Esc", "close popup / cancel"},
	{"Space", "what can I do here"},
	{"?", "this help"},
	{"", "Global"},
	{"q", "quit"},
	{"Ctrl+C", "force quit"},
	{"", "ssh grid"},
	{"Tab", "toggle a session's cell (on [1])"},
	{"Alt+1-9", "focus a grid cell"},
	{"Alt+Esc", "leave the pty, back to [1]"},
	{"", "Navigate"},
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
	m.top = moveScroll(m.top, max(0, len(helpContent)-m.visible()), msg.String(), m.visible())
}

// visible is how many content lines fit; the box costs 4 rows of chrome.
func (m helpPopup) visible() int { return max(1, min(len(helpContent), m.screenH-6)) }

func (m helpPopup) view() string {
	keyW := 0
	for _, e := range helpContent {
		keyW = max(keyW, dispW(e.key))
	}
	innerW := popupInnerW(m.screenW, keyW+29)

	dim := lipgloss.NewStyle().Foreground(dimColor)
	key := lipgloss.NewStyle().Foreground(handColor)
	txt := lipgloss.NewStyle().Foreground(textColor)

	vis := m.visible()
	end := min(len(helpContent), m.top+vis)
	rows := make([]string, 0, vis)
	for _, e := range helpContent[m.top:end] {
		if e.key == "" {
			rows = append(rows, dim.Render(padRight(" "+e.desc, innerW)))
			continue
		}
		rows = append(rows, key.Render(padRight("  "+e.key, keyW+4))+
			txt.Render(padRight(e.desc, innerW-keyW-4)))
	}

	pairs := [][2]string{{"Esc", "close"}}
	if len(helpContent) > vis {
		pairs = append([][2]string{{"j/k", "scroll"}}, pairs...)
	}
	hint := hintLegend(pairs)
	return drawPopupBox(popupLayerColor(m.layer), " "+glyphHelp+" Help ", hint,
		animRows(m.anim, capRows(rows, m.screenH)), innerW)
}
