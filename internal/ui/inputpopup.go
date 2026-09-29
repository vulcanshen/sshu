package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// inputAction says what to do with the answer. It exists so the popup itself
// stays a text box and nothing else — the same reason confirmPopup carries an
// action rather than a closure.
type inputAction int

const (
	inputNone inputAction = iota
	inputRename
	inputAdd
	inputGridDims   // the ssh grid's custom columns × rows
	inputKnownHosts // which names a known_hosts key is trusted for
)

// inputPopup is one line of text with a question above it — the input class
// (tdp F1), one field. A confirm asks yes or no; this asks "what should it be
// called". The host form and its siblings are the same class with several
// fields and one submit.
//
// Every answer it takes can be refused — a name with a slash, one already
// taken, a column count that is not a digit — so it keeps an error row from the
// moment it opens, blank until a submit fails (tdp F7, K3). A refused answer
// stays in the box with the reason under it, to be fixed rather than retyped.
type inputPopup struct {
	anim   popupAnimator
	title  string
	glyph  string
	prompt string
	value  string
	action inputAction
	// accept is the verb on the Enter hint. The box is the same box; what
	// pressing Enter DOES is not, and the hint has to say which.
	accept string
	// subject is what the answer is about — the path being renamed. The popup
	// does not interpret it; it hands it back so the caller does not have to
	// remember what was under the cursor two keystrokes ago.
	subject string
	// placeholder is shown in the empty box and vanishes at the first keystroke.
	// Add needs it: that box asks one question with two answers, and the whole
	// difference between them is one character at the end. A rule that small has
	// to be said where the typing happens, not only in the menu that opened it.
	placeholder string
	// at is the subject's POSITION, for a list whose rows have no name to
	// carry — the same field confirmPopup and detailPopup grew, and for the
	// same reason (§11.40).
	at int
	// err is why the last submit was refused; typing clears it, the way the
	// form's error row gives way once the field is being fixed.
	err string

	layer   int
	screenW int
	screenH int
}

func newInputPopup() inputPopup { return inputPopup{anim: newPopupAnimator("input")} }

func (m inputPopup) isActive() bool      { return m.anim.isActive() }
func (m inputPopup) isInteractive() bool { return m.anim.isInteractive() }
func (m *inputPopup) close() tea.Cmd     { return m.anim.close() }
func (m *inputPopup) setSize(w, h int)   { m.screenW, m.screenH = w, h }

// ask opens the box with value already filled in and the cursor at its end.
// Pre-filling matters for a rename: most renames change part of a name, and
// starting from empty makes the common case retype the whole thing.
func (m *inputPopup) ask(p inputPopup, layer int) tea.Cmd {
	p.anim, p.layer = m.anim, layer
	p.screenW, p.screenH = m.screenW, m.screenH
	*m = p
	return m.anim.open()
}

// acceptVerb is what Enter will actually do.
//
// For Add that is not fixed: the trailing slash decides, so the verb changes as
// it is typed. Watching "create file" turn into "create directory" the moment
// you press / is the disclosure — a static label could only describe the rule,
// this one confirms which side of it you are on.
func (m inputPopup) acceptVerb() string {
	if m.action != inputAdd {
		return m.accept
	}
	switch v := strings.TrimSpace(m.value); {
	case v == "":
		return m.accept
	case strings.HasSuffix(v, "/"):
		return "create directory"
	}
	return "create file"
}

// update edits the line. It reports the committed value, or "" — Esc is not
// handled here, because cancelling is resolved in one place for every float
// (tdp K4).
func (m *inputPopup) update(msg tea.KeyMsg) (committed string, done bool) {
	if !m.anim.isInteractive() {
		return "", false
	}
	switch msg.Type {
	case tea.KeyEnter:
		return m.value, true
	case tea.KeyBackspace:
		if r := []rune(m.value); len(r) > 0 {
			m.value = string(r[:len(r)-1])
		}
		m.err = ""
	case tea.KeySpace:
		m.value += " "
		m.err = ""
	case tea.KeyRunes:
		m.value += string(msg.Runes)
		m.err = ""
	}
	return "", false
}

func (m inputPopup) view() string {
	innerW := popupInnerW(m.screenW)
	dim := lipgloss.NewStyle().Foreground(dimColor)
	red := lipgloss.NewStyle().Foreground(warnColor)
	edit := lipgloss.NewStyle().Foreground(editColor)
	cur := lipgloss.NewStyle().Foreground(lipgloss.Color(baseHex)).Background(editColor)

	// Lavender, because this is the field being edited — the same meaning the
	// host form gives it, and the same meaning the cwd crumb gives it (tdp P4).
	value := truncate(m.value, innerW-3)
	line := " " + edit.Render(value) + cur.Render(" ") +
		spaces(max(0, innerW-2-dispW(value)))
	if m.value == "" && m.placeholder != "" {
		ph := truncate(m.placeholder, innerW-3)
		line = " " + cur.Render(" ") + dim.Render(ph) +
			spaces(max(0, innerW-2-dispW(ph)))
	}

	rows := []string{
		dim.Render(padRight(" "+m.prompt, innerW)),
		spaces(innerW),
		line,
		spaces(innerW),
		red.Render(padRight("  "+truncate(m.err, max(0, innerW-2)), innerW)),
	}
	hint := [][2]string{{"Enter", m.acceptVerb()}, {"Esc", "cancel"}}
	return drawPopupBox(popupLayerColor(m.layer), " "+m.glyph+" "+m.title+" ",
		hint, animRows(m.anim, rows), innerW)
}

func spaces(n int) string {
	if n <= 0 {
		return ""
	}
	b := make([]byte, n)
	for i := range b {
		b[i] = ' '
	}
	return string(b)
}
