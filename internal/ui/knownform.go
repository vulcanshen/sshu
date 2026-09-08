package ui

import (
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/vulcanshen/sshu/internal/store"
)

// knownAddForm asks which machine to fetch a host key FROM.
//
// It does not ask for the key. The alternative — a field for 68 characters of
// base64 — is a form nobody would use guarding a file that decides whether you
// are talking to the machine you meant: you would have to have got that string
// from somewhere else anyway, and the somewhere else is the server. So `[A]`
// does what `ssh-keyscan` does and what ssh itself does on a first connect:
// open the connection far enough to see the key, stop before authenticating,
// and show the fingerprint to accept or refuse.
//
// The form stays on screen while that happens, because a fifteen-second wait
// behind a closed popup is indistinguishable from nothing having happened.
const (
	kfHost = iota
	kfPort
	kfCount
)

type knownAddForm struct {
	anim      popupAnimator
	fields    []formField
	focus     int
	err       string
	errIdx    int
	submitted bool
	// scanning is a handshake in flight. The fields stop accepting keys while
	// it is true: the answer coming back is about the address that was sent,
	// and letting the box be edited underneath would make the confirmation name
	// something else.
	scanning bool
	spin     int
	layer    int
	screenW  int
	screenH  int
}

func newKnownAddForm() knownAddForm {
	return knownAddForm{anim: newPopupAnimator("knownadd"), errIdx: -1}
}

func (m knownAddForm) isActive() bool    { return m.anim.isActive() }
func (m *knownAddForm) close() tea.Cmd   { return m.anim.close() }
func (m *knownAddForm) setSize(w, h int) { m.screenW, m.screenH = w, h }

func (m *knownAddForm) open(layer int) tea.Cmd {
	f := make([]formField, kfCount)
	f[kfHost] = formField{label: "Host", placeholder: "name or address to ask"}
	f[kfPort] = formField{label: "Port", value: strconv.Itoa(store.DefaultPort), digits: true}
	for i := range f {
		f[i].caret = len([]rune(f[i].value))
	}
	m.fields, m.focus, m.err, m.errIdx = f, kfHost, "", -1
	m.submitted, m.scanning, m.spin = false, false, 0
	m.layer = layer
	return m.anim.open()
}

func (m knownAddForm) enabled(int) bool { return true }

// complete is §11.34's question. Port is pre-filled, so in practice this is
// "has the host been typed" — and Enter on a form with an empty host steps to
// it rather than sending a handshake to nowhere.
func (m knownAddForm) complete() bool {
	return strings.TrimSpace(m.fields[kfHost].value) != "" &&
		strings.TrimSpace(m.fields[kfPort].value) != ""
}

func (m *knownAddForm) moveFocus(d int) {
	m.focus = (m.focus + d + kfCount) % kfCount
}

func (m knownAddForm) update(msg tea.KeyMsg) (knownAddForm, formResult) {
	if !m.anim.isInteractive() || m.scanning || msg.Alt {
		return m, formNone
	}
	switch msg.Type {
	case tea.KeyTab, tea.KeyDown:
		m.moveFocus(1)
		return m, formNone
	case tea.KeyShiftTab, tea.KeyUp:
		m.moveFocus(-1)
		return m, formNone
	case tea.KeyEnter:
		if !m.complete() {
			m.moveFocus(1)
			return m, formNone
		}
		return m, formSubmit
	}
	editField(&m.fields[m.focus], msg)
	return m, formNone
}

// target is what the form is asking about.
func (m knownAddForm) target() (string, int) {
	port, err := strconv.Atoi(strings.TrimSpace(m.fields[kfPort].value))
	if err != nil {
		port = store.DefaultPort
	}
	return strings.TrimSpace(m.fields[kfHost].value), port
}

func (m *knownAddForm) fail(msg string, field int) {
	m.scanning = false
	m.err, m.errIdx = msg, field
	if field >= 0 && field < len(m.fields) {
		m.focus = field
	}
}

func (m knownAddForm) view() string {
	labelW := 0
	for _, f := range m.fields {
		labelW = max(labelW, dispW(f.label))
	}
	// The box widens for its error. Two fields make a narrow box, and the thing
	// that lands in it is a network error — "dial tcp 127.0.0.1:1: connect:
	// connection refused" is the whole answer, and half of it is not.
	want := labelW + 38
	if m.err != "" {
		want = max(want, dispW(m.err)+6)
	}
	innerW := popupInnerW(m.screenW, want)
	labelCol := min(labelW+4, max(0, innerW-8))
	valueW := max(0, innerW-labelCol-1)

	// While scanning the focus marker comes off every row: nothing here is
	// being edited, and a lit field says otherwise.
	focus := m.focus
	if m.scanning {
		focus = -1
	}
	rows := formBody(m.fields, focus, m.errIdx, m.err, m.enabled, innerW, labelCol, valueW)

	pairs := [][2]string{{"Tab", "next"}, {"Enter", "fetch"}, {"Esc", "cancel"}}
	if !m.complete() {
		pairs = [][2]string{{"Tab", "next"}, {"Enter", "next"}, {"Esc", "cancel"}}
	}
	if m.scanning {
		host, port := m.target()
		spin := spinnerFrames[m.spin%len(spinnerFrames)]
		// Replaces the error row rather than adding a line, so the box does not
		// change height the moment it starts working.
		rows[len(rows)-1] = padRight("  "+spin+" asking "+host+":"+itoa(port)+" for its key", innerW)
		pairs = [][2]string{{"Esc", "cancel"}}
	}

	return drawPopupBox(popupLayerColor(m.layer), " "+glyphPlus+" Add known host ",
		hintLegend(pairs), animRows(m.anim, capRows(rows, m.screenH)), innerW)
}
