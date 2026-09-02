package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// menuItem is one row of the Space menu. A commit dispatches key to the panel,
// so the menu is a discoverability shell over the letter hotkeys rather than a
// second implementation of them — which is what keeps §4.2 honest: every letter
// hotkey IS a row here, and every row can be run without knowing its letter.
type menuItem struct {
	label     string
	key       string // dispatched on commit; "enter" for the core-key action
	hint      string
	header    bool // dim region label, not selectable
	separator bool // horizontal rule, not selectable
}

// spaceMenu is the §A.1 contextual entry point: "what can I do, here, now".
type spaceMenu struct {
	anim    popupAnimator
	items   []menuItem
	cursor  int
	title   string
	layer   int
	screenW int
	screenH int
}

// newHostPicker is a second spaceMenu instance reused as tab [2]'s host chooser.
// The distinct animator name keeps its ticks from colliding with the Space menu,
// which stays open behind it.
func newHostPicker() spaceMenu {
	return spaceMenu{anim: newPopupAnimator("hostpicker")}
}

func newSpaceMenu() spaceMenu {
	return spaceMenu{anim: newPopupAnimator("spacemenu")}
}

// newCredPicker is the same reuse for the host form's credential chooser.
func newCredPicker() spaceMenu {
	return spaceMenu{anim: newPopupAnimator("credpicker")}
}

func (m *spaceMenu) setItems(items []menuItem, title string, layer int) {
	m.items, m.title, m.layer = items, title, layer
	m.cursor = m.firstSelectable()
}

func (m spaceMenu) isActive() bool      { return m.anim.isActive() }
func (m spaceMenu) isInteractive() bool { return m.anim.isInteractive() }
func (m *spaceMenu) open() tea.Cmd      { return m.anim.open() }
func (m *spaceMenu) close() tea.Cmd     { return m.anim.close() }
func (m *spaceMenu) setSize(w, h int)   { m.screenW, m.screenH = w, h }

func (m spaceMenu) firstSelectable() int {
	for i, it := range m.items {
		if !it.header && !it.separator {
			return i
		}
	}
	return 0
}

// step moves d rows, skipping the labels and rules, and WRAPS at the ends —
// off the bottom is the top. It walks at most len(items) places so a menu of
// nothing but headers terminates instead of spinning.
func (m *spaceMenu) step(d int) {
	n := len(m.items)
	if n == 0 {
		return
	}
	at := m.cursor
	for i := 0; i < n; i++ {
		at = (at + d + n) % n
		if !m.items[at].header && !m.items[at].separator {
			m.cursor = at
			return
		}
	}
}

// update handles one keystroke. The returned string is the committed hotkey
// ("" when nothing committed) — the caller dispatches it and closes the menu.
func (m spaceMenu) update(msg tea.KeyMsg) (spaceMenu, string, tea.Cmd) {
	if !m.anim.isInteractive() {
		return m, "", nil
	}
	switch k := msg.String(); k {
	case "j", "down":
		m.step(1)
	case "k", "up":
		m.step(-1)
	case "enter":
		if m.cursor < len(m.items) {
			return m, m.items[m.cursor].key, nil
		}
	default:
		// Letter hotkeys work from inside the menu too: the menu is the slow
		// path and the letter is the fast one, and they must agree. Same
		// exact-then-fold rule as the panel, so `t` and `T` stay distinct here.
		var keys []string
		for _, it := range m.items {
			if it.header || it.separator {
				continue
			}
			keys = append(keys, it.key)
		}
		if i := hotkeyIndex(keys, k); i >= 0 {
			return m, keys[i], nil
		}
	}
	return m, "", nil
}

func (m spaceMenu) view() string {
	// Everything that has to fit inside the box gets measured: an action row
	// (label column plus hint column), a header on a line of its own, the
	// title, and the legend along the bottom border.
	//
	// Headers used to be skipped here, which measured a menu that is ALL
	// description — "nothing recorded yet", "j/k choose a section" — at zero.
	// It came out a stub with its own words clipped and its legend cut
	// mid-key: a box that reads as breakage rather than as an answer.
	labelW, hintW, headW, acts := 0, 0, 0, 0
	for _, it := range m.items {
		switch {
		case it.separator:
		case it.header:
			headW = max(headW, dispW(it.label)+2)
		default:
			acts++
			labelW = max(labelW, dispW(bracketHotkey(it.label, it.key)))
			hintW = max(hintW, dispW(it.hint))
		}
	}
	// A menu with nothing to run says so: j/k has nowhere to go and Enter has
	// nothing to commit, so the legend names the one key that still works —
	// the same honesty the pty footer keeps (§4.4).
	legend := hintLegend([][2]string{{"j/k", "move"}, {"Enter", "run"}, {"Esc", "close"}})
	if acts == 0 {
		legend = hintLegend([][2]string{{"Esc", "close"}})
	}
	// " " + label + "  " + hint + " "
	innerW := popupInnerW(m.screenW,
		max(dispW(m.title)+6, labelW+hintW+4, headW, dispW(legend)+1))
	// When the box cannot hold both columns the hint yields: the label is what
	// the action IS, the hint only elaborates on it.
	hintW = max(0, min(hintW, innerW-labelW-3))

	dim := lipgloss.NewStyle().Foreground(dimColor)
	txt := lipgloss.NewStyle().Foreground(textColor)
	cur := lipgloss.NewStyle().Foreground(lipgloss.Color(baseHex)).Background(handColor)

	rows := make([]string, 0, len(m.items))
	for i, it := range m.items {
		switch {
		case it.separator:
			rows = append(rows, dim.Render(" "+strings.Repeat("─", max(0, innerW-2))))
		case it.header:
			rows = append(rows, dim.Render(padRight(" "+it.label, innerW)))
		default:
			label := padRight(" "+bracketHotkey(it.label, it.key), innerW-hintW-1)
			hint := padLeft(it.hint, hintW) + " "
			if i == m.cursor {
				rows = append(rows, cur.Render(label+hint))
			} else {
				rows = append(rows, txt.Render(label)+dim.Render(hint))
			}
		}
	}

	return drawPopupBox(popupLayerColor(m.layer), " "+glyphMenu+" "+m.title+" ",
		legend, animRows(m.anim, capRows(rows, m.screenH)), innerW)
}
