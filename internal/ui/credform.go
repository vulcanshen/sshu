package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/vulcanshen/sshu/internal/store"
)

// credForm creates and edits credentials. It is the host form's sibling, and
// its IdentityFile row is where the newer field interaction lives: Enter on
// the empty field opens the picker, Enter on a filled one moves on, and
// Backspace clears the whole line — a picked path is replaced, not shaved
// letter by letter.

// Field order.
const (
	cName = iota
	cUser
	cAuth
	cIdentity
	cPassword
	cCount
)

type credForm struct {
	anim      popupAnimator
	fields    []formField
	focus     int
	editing   string // the name this form started from; "" means create
	err       string
	errIdx    int
	submitted bool
	layer     int
	screenW   int
	screenH   int
}

func newCredForm() credForm { return credForm{anim: newPopupAnimator("credform"), errIdx: -1} }

func (m credForm) isActive() bool      { return m.anim.isActive() }
func (m credForm) isInteractive() bool { return m.anim.isInteractive() }
func (m *credForm) close() tea.Cmd     { return m.anim.close() }
func (m *credForm) setSize(w, h int)   { m.screenW, m.screenH = w, h }

func blankCredFields() []formField {
	f := make([]formField, cCount)
	f[cName] = formField{label: "Name"}
	f[cUser] = formField{label: "User"}
	f[cAuth] = formField{label: "Auth", kind: fieldToggle,
		options: []string{string(store.AuthPassword), string(store.AuthPrivateKey)}}
	f[cIdentity] = formField{label: "IdentityFile",
		placeholder: "enter to browse " + store.FoldHome(identityRoot())}
	f[cPassword] = formField{label: "Password", mask: true}
	return f
}

func (m *credForm) openCreate(layer int) tea.Cmd {
	m.fields, m.focus, m.editing, m.err, m.errIdx = blankCredFields(), cName, "", "", -1
	m.submitted = false
	m.fields[cAuth].sel = 1 // privatekey — the safer default, as on the host form
	m.layer = layer
	return m.anim.open()
}

func (m *credForm) openEdit(c store.Credential, layer int) tea.Cmd {
	f := blankCredFields()
	f[cName].value = c.Name
	f[cUser].value = c.User
	f[cIdentity].value = c.IdentityFile
	f[cPassword].value = c.Password
	if c.Auth == store.AuthPrivateKey {
		f[cAuth].sel = 1
	}
	for i := range f {
		f[i].caret = len([]rune(f[i].value))
	}
	m.fields, m.focus, m.editing, m.err, m.errIdx = f, cName, c.Name, "", -1
	m.submitted = false
	m.layer = layer
	return m.anim.open()
}

func (m credForm) auth() store.AuthMethod {
	return store.AuthMethod(m.fields[cAuth].options[m.fields[cAuth].sel])
}

func (m credForm) enabled(i int) bool {
	switch i {
	case cIdentity:
		return m.auth() == store.AuthPrivateKey
	case cPassword:
		return m.auth() == store.AuthPassword
	}
	return true
}

func (m *credForm) moveFocus(d int) {
	for i := 0; i < cCount; i++ {
		m.focus = (m.focus + d + cCount) % cCount
		if m.enabled(m.focus) {
			return
		}
	}
}

func (m *credForm) syncFocus() {
	if !m.enabled(m.focus) {
		m.moveFocus(1)
	}
}

func (m credForm) update(msg tea.KeyMsg) (credForm, formResult) {
	if !m.anim.isInteractive() {
		return m, formNone
	}
	f := &m.fields[m.focus]

	if msg.Alt {
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
		// The path field's Enter: empty opens the picker, filled moves on. So
		// the whole exchange is enter → pick → enter → next field, and
		// replacing a pick is Backspace (the whole line) then Enter again.
		if m.focus == cIdentity {
			if strings.TrimSpace(f.value) == "" {
				return m, formBrowse
			}
			m.moveFocus(1)
			return m, formNone
		}
		return m, formSubmit
	case tea.KeyBackspace:
		if m.focus == cIdentity {
			f.value, f.caret = "", 0
			return m, formNone
		}
	}
	if editField(f, msg) && f.kind == fieldToggle {
		m.syncFocus()
	}
	return m, formNone
}

func (m credForm) credential() store.Credential {
	c := store.Credential{
		Name: strings.TrimSpace(m.fields[cName].value),
		User: strings.TrimSpace(m.fields[cUser].value),
		Auth: m.auth(),
	}
	if c.Auth == store.AuthPrivateKey {
		c.IdentityFile = strings.TrimSpace(m.fields[cIdentity].value)
	} else {
		c.Password = m.fields[cPassword].value
	}
	return c
}

func (m *credForm) fail(msg string, field int) {
	m.err, m.errIdx = msg, field
	if field >= 0 && m.enabled(field) {
		m.focus = field
	}
}

func (m *credForm) refreshError(msg string, field int) {
	m.err, m.errIdx = msg, field
}

func (m credForm) view() string {
	labelW := 0
	for _, f := range m.fields {
		labelW = max(labelW, dispW(f.label))
	}
	innerW := popupInnerW(m.screenW, labelW+38)
	labelCol := min(labelW+4, max(0, innerW-8))
	valueW := max(0, innerW-labelCol-1)

	rows := formBody(m.fields, m.focus, m.errIdx, m.err, m.enabled, innerW, labelCol, valueW)

	glyph, title := glyphPlus, "New credential"
	if m.editing != "" {
		glyph, title = glyphPencil, "Edit credential"
	}
	// The hint names what THIS field does with Enter — on the path row Enter
	// is not "save", and saying so is the standing disclosure (§4.5).
	var pairs [][2]string
	switch {
	case m.focus == cIdentity && strings.TrimSpace(m.fields[cIdentity].value) == "":
		pairs = [][2]string{{"Enter", "browse"}, {"Tab", "next"}, {"Esc", "cancel"}}
	case m.focus == cIdentity:
		pairs = [][2]string{{"Enter", "next"}, {"Backspace", "clear"}, {"Esc", "cancel"}}
	case m.fields[m.focus].kind == fieldToggle:
		pairs = [][2]string{{"Tab", "next"}, {arrowGlyphs, "switch"}, {"Enter", "save"}, {"Esc", "cancel"}}
	default:
		pairs = [][2]string{{"Tab", "next"}, {"Enter", "save"}, {"Esc", "cancel"}}
	}

	return drawPopupBox(popupLayerColor(m.layer), " "+glyph+" "+title+" ", hintLegend(pairs),
		animRows(m.anim, capRows(rows, m.screenH)), innerW)
}
