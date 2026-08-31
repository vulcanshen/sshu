package ui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// toastLife is how long a toast sits before dismissing itself.
const toastLife = 2200 * time.Millisecond

type toastKind int

const (
	toastInfo toastKind = iota
	toastError
)

// toastExpireMsg retires one toast. gen is a generation guard: without it, the
// timer from a toast the user already replaced would close its successor.
type toastExpireMsg struct{ gen int }

// toastModel is transient feedback — the message class with an auto-dismiss.
// Esc still kills it immediately: no float may make the user wait out a timer
// (§6.5).
type toastModel struct {
	anim    popupAnimator
	msg     string
	kind    toastKind
	gen     int
	screenW int
	screenH int
}

func newToast() toastModel { return toastModel{anim: newPopupAnimator("toast")} }

func (m toastModel) isActive() bool    { return m.anim.isActive() }
func (m *toastModel) setSize(w, h int) { m.screenW, m.screenH = w, h }
func (m *toastModel) close() tea.Cmd   { return m.anim.close() }

func (m *toastModel) show(msg string, kind toastKind) tea.Cmd {
	m.msg, m.kind = msg, kind
	m.gen++
	gen := m.gen
	return tea.Batch(
		m.anim.open(),
		tea.Tick(toastLife, func(time.Time) tea.Msg { return toastExpireMsg{gen: gen} }),
	)
}

func (m *toastModel) expire(msg toastExpireMsg) tea.Cmd {
	if msg.gen != m.gen {
		return nil // a newer toast replaced this one
	}
	return m.anim.close()
}

func (m toastModel) view() string {
	innerW := popupInnerW(m.screenW, dispW(m.msg)+4)
	style := lipgloss.NewStyle().Foreground(textColor)
	title := " " + glyphInfo + " Info "
	if m.kind == toastError {
		style = lipgloss.NewStyle().Foreground(warnColor)
		title = " " + glyphWarn + " Error "
	}
	rows := []string{style.Render(padRight("  "+m.msg, innerW))}
	return drawPopupBox(popupLayerColor(1), title, hintLegend([][2]string{{"Esc", "close"}}),
		animRows(m.anim, capRows(rows, m.screenH)), innerW)
}
