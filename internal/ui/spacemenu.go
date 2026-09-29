package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// menuItem is one row of the Space menu. A commit dispatches key to the panel,
// so the menu is a discoverability shell over the letter hotkeys rather than a
// second implementation of them — which is what keeps tdp M3 honest: every letter
// hotkey IS a row here, and every row can be run without knowing its letter.
type menuItem struct {
	label     string
	key       string // dispatched on commit; "enter" for the core-key action
	hint      string
	header    bool // dim region label, not selectable
	separator bool // horizontal rule, not selectable
	// disabled: the action belongs here but cannot run right now. It is NOT
	// the same as leaving the row out (which is what an action that does not
	// apply gets, §sftpApplicable): a row that vanishes teaches that the
	// action does not exist on this panel, and it will be looked for later.
	// The row is dimmed and keeps its own description; the cursor lands on it
	// like any other, and Enter or its letter does nothing (tdp M6). No reason
	// is written in: reasons vary without end, and one that fits a single
	// line today is a line that overflows tomorrow.
	disabled bool
}

// selectable reports whether the cursor can stand on this row.
func (it menuItem) selectable() bool { return !it.header && !it.separator }

// spaceMenu is the Space menu (tdp K5, M2): "what can I do, here, now". The
// same type is the global operation popup, the pickers and the lock menu.
type spaceMenu struct {
	anim    popupAnimator
	items   []menuItem
	cursor  int
	top     int // first row on screen, when the menu is taller than the screen
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
	m.cursor, m.top = m.firstSelectable(), 0
	m.top = scrollTop(0, m.cursor, m.visible(), len(m.items))
}

// visible is how many rows the box shows: all of them, or as many as the
// screen holds — beyond that the menu scrolls with the cursor (tdp F7), so
// j can never walk it onto a row that is not drawn.
func (m spaceMenu) visible() int { return min(len(m.items), popupBudget(m.screenH)) }

func (m spaceMenu) isActive() bool      { return m.anim.isActive() }
func (m spaceMenu) isInteractive() bool { return m.anim.isInteractive() }
func (m *spaceMenu) open() tea.Cmd      { return m.anim.open() }
func (m *spaceMenu) close() tea.Cmd     { return m.anim.close() }
func (m *spaceMenu) setSize(w, h int)   { m.screenW, m.screenH = w, h }

func (m spaceMenu) firstSelectable() int {
	for i, it := range m.items {
		if it.selectable() {
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
		if m.items[at].selectable() {
			m.cursor = at
			m.top = scrollTop(m.top, at, m.visible(), n)
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
		if m.cursor < len(m.items) && !m.items[m.cursor].disabled {
			return m, m.items[m.cursor].key, nil
		}
	default:
		// Letter hotkeys work from inside the menu too: the menu is the slow
		// path and the letter is the fast one, and they must agree. Same
		// exact-then-fold rule as the panel, so `t` and `T` stay distinct here.
		// A dimmed row's letter is swallowed, not passed on (tdp M6).
		var keys []string
		var rows []menuItem
		for _, it := range m.items {
			if !it.selectable() {
				continue
			}
			keys, rows = append(keys, it.key), append(rows, it)
		}
		if i := hotkeyIndex(keys, k); i >= 0 && !rows[i].disabled {
			return m, keys[i], nil
		}
	}
	return m, "", nil
}

func (m spaceMenu) view() string {
	// The label and hint columns are measured; the box is not — its width is
	// the one every popup has (tdp F7, D4).
	labelW, hintW, acts := 0, 0, 0
	for _, it := range m.items {
		if it.selectable() {
			acts++
			labelW = max(labelW, dispW(bracketHotkey(it.label, it.key)))
			hintW = max(hintW, dispW(it.hint))
		}
	}
	vis := m.visible()
	top := scrollTop(m.top, m.cursor, vis, len(m.items))
	// A menu with nothing to run says so: j/k has nowhere to go and Enter has
	// nothing to commit, so the legend names the one key that still works —
	// the same honesty the pty footer keeps (tdp M5). A menu taller than the
	// screen says where in it the box is, the way the viewer does.
	pairs := [][2]string{{"j/k", "move"}, {"Enter", "run"}, {"Esc", "close"}}
	if acts == 0 {
		pairs = [][2]string{{"Esc", "close"}}
	}
	if len(m.items) > vis {
		pairs = append([][2]string{{"", itoa(top+1) + " of " + itoa(len(m.items))}}, pairs...)
	}
	legend := hintLegend(pairs)
	// " " + label + "  " + hint + " "
	innerW := popupInnerW(m.screenW)
	// When the box cannot hold both columns the hint yields: the label is what
	// the action IS, the hint only elaborates on it.
	hintW = max(0, min(hintW, innerW-labelW-3))

	dim := lipgloss.NewStyle().Foreground(dimColor)
	txt := lipgloss.NewStyle().Foreground(textColor)
	cur := lipgloss.NewStyle().Foreground(lipgloss.Color(baseHex)).Background(handColor)
	// The cursor still has to be visible on a row that cannot run, so it drops
	// to the register the app already uses for "highlighted, but not live" —
	// the same one an unfocused panel's chip and nav cursor wear.
	curOff := lipgloss.NewStyle().Foreground(lipgloss.Color(baseHex)).Background(borderDim)

	rows := make([]string, 0, vis)
	for i := top; i < min(len(m.items), top+vis); i++ {
		it := m.items[i]
		switch {
		case it.separator:
			rows = append(rows, dim.Render(" "+strings.Repeat("─", max(0, innerW-2))))
		case it.header:
			rows = append(rows, dim.Render(padRight(" "+it.label, innerW)))
		default:
			label := padRight(" "+bracketHotkey(it.label, it.key), innerW-hintW-1)
			hint := padLeft(it.hint, hintW) + " "
			switch {
			case i == m.cursor && it.disabled:
				rows = append(rows, curOff.Render(label+hint))
			case i == m.cursor:
				rows = append(rows, cur.Render(label+hint))
			case it.disabled:
				rows = append(rows, dim.Render(label+hint))
			default:
				rows = append(rows, txt.Render(label)+dim.Render(hint))
			}
		}
	}

	return drawPopupBox(popupLayerColor(m.layer), " "+glyphMenu+" "+m.title+" ",
		legend, animRows(m.anim, capRows(rows, m.screenH)), innerW)
}
