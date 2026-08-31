package ui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// History used to be panel [6]: a third of the left column, permanently, for a
// list nothing could be done to and that was empty most of the time.
//
// It is the same shape of thing as a transfer — occasional information you
// consult, not a surface you work on — so it gets the same treatment tab [2]
// gives transfers: a count in the tab row ("3 live · 1 past"), a popup on
// demand, and a toast at the moment it matters. What was actually valuable was
// never the panel; it was the answer to "which one died, and why", and a toast
// answers that at the instant the question occurs.
//
// It has no cursor, for the reason it never had one: nothing here can be acted
// on. Reconnecting is done from [1], where the host is. So this scrolls.
type historyPopup struct {
	anim    popupAnimator
	top     int
	layer   int
	screenW int
	screenH int
}

func newHistoryPopup() historyPopup {
	return historyPopup{anim: newPopupAnimator("history")}
}

func (m historyPopup) isActive() bool      { return m.anim.isActive() }
func (m historyPopup) isInteractive() bool { return m.anim.isInteractive() }
func (m *historyPopup) close() tea.Cmd     { return m.anim.close() }
func (m *historyPopup) setSize(w, h int)   { m.screenW, m.screenH = w, h }

func (m *historyPopup) open(layer int) tea.Cmd {
	m.layer, m.top = layer, 0
	return m.anim.open()
}

// visible is how many rows fit; the box costs 4 of them.
func (m historyPopup) visible(n int) int { return max(1, min(n, m.screenH-6)) }

func (m *historyPopup) update(msg tea.KeyMsg, n int) {
	if !m.anim.isInteractive() {
		return
	}
	m.top = moveScroll(m.top, max(0, n-m.visible(n)), msg.String(), m.visible(n))
}

// sshHistoryW is the popup's width. Wide enough that a row fits on ONE line —
// name, reason and end time together — which is what the panel never had room
// for. In [6] the time had to go on the name line and the reason on a second,
// and neither line was ever full.
const sshHistoryW = 56

func (m historyPopup) view(items []*session) string {
	innerW := popupInnerW(m.screenW, sshHistoryW)
	dim := lipgloss.NewStyle().Foreground(dimColor)
	txt := lipgloss.NewStyle().Foreground(textColor)

	var rows []string
	if len(items) == 0 {
		rows = []string{dim.Render(padRight("  nothing has ended yet", innerW))}
	}
	end := min(len(items), m.top+m.visible(len(items)))
	for _, s := range items[min(m.top, len(items)):end] {
		// Colour belongs to the REASON only — how a session ended is a property
		// of the ending, not of the host (the rule [6] had, kept).
		reasonFG := warnColor
		if s.ok {
			reasonFG = liveColor
		}
		at := s.ended.Format("15:04:05")
		reason := s.reason

		// Fixed right-hand slot for the time, then the reason beside it, then the
		// name takes what is left — measured, so a long host name cannot push the
		// two facts off the edge.
		lead := "  "
		nameW := max(6, innerW-dispW(lead)-len(at)-dispW(reason)-4)
		rows = append(rows,
			lead+txt.Render(padRight(s.host.Name, nameW))+"  "+
				lipgloss.NewStyle().Foreground(reasonFG).Render(reason)+
				dim.Render(padLeft(at, max(0, innerW-dispW(lead)-nameW-2-dispW(reason)))))
	}

	hint := hintLegend([][2]string{{"j/k", "scroll"}, {"Esc", "close"}})
	return drawPopupBox(popupLayerColor(m.layer), " "+glyphConnect+" History ",
		hint, animRows(m.anim, capRows(rows, m.screenH)), innerW)
}

// endedBadly is what to say out loud when sessions finish. A clean exit is what
// you asked for by typing `exit`, so it says nothing; a failure is the thing you
// would otherwise have to go looking for.
func endedBadly(ended []*session) string {
	var bad []*session
	for _, s := range ended {
		if !s.ok {
			bad = append(bad, s)
		}
	}
	switch len(bad) {
	case 0:
		return ""
	case 1:
		return bad[0].host.Name + " · " + bad[0].reason
	default:
		return plural(len(bad), "session") + " ended badly · press H for history"
	}
}
