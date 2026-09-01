package ui

import (
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/vulcanshen/sshu/internal/store"
)

// The form is sshu's fifth popup class, and it exists because menu and form mean
// different things by Enter: a menu runs the row under the cursor, a form
// submits the whole thing whatever field you are on. Folding them together
// would be the hybrid float §6.1 forbids.
//
// Two consequences follow from a popup that eats text:
//   - Space types a space; it is NOT the §A.1 entry key here. The standing
//     border hint takes over the "what can I do" job (§4.5).
//   - j/k are characters, so field navigation is Tab / Shift+Tab / arrows only.

// formResult is what a keystroke asked the caller to do. Browse exists because
// an identity file should be picked, not typed, and only the caller owns the
// picker.
type formResult int

const (
	formNone formResult = iota
	formSubmit
	formBrowse
	// formPickCred asks the caller to open the credential picker — only the
	// caller knows the saved credentials.
	formPickCred
)

type fieldKind int

const (
	fieldText fieldKind = iota
	fieldToggle
)

type formField struct {
	label       string
	kind        fieldKind
	value       string
	caret       int
	placeholder string // shown dim while the field is empty — never a value
	mask        bool   // render as bullets — the password is never shown in clear
	digits      bool   // reject anything but 0-9
	options     []string
	sel         int
}

// Field order. Identity and Password both always exist; exactly one is live at
// a time, decided by Auth.
const (
	fName = iota
	fHost
	fPort
	fUser
	fAuth
	fCredential
	fIdentity
	fPassword
	fCount
)

type hostForm struct {
	anim    popupAnimator
	fields  []formField
	focus   int
	editing string // the name this form started from; "" means create
	err     string
	errIdx  int
	// submitted flips on the first Enter. Before it, the form says nothing about
	// what is missing — an empty form scolding you the moment it opens is noise.
	// After it, the error is re-checked on every keystroke, so a field stops
	// being marked the instant it is fixed rather than at the next submit.
	submitted bool
	layer     int
	screenW   int
	screenH   int
}

func newHostForm() hostForm { return hostForm{anim: newPopupAnimator("form"), errIdx: -1} }

func (m hostForm) isActive() bool      { return m.anim.isActive() }
func (m hostForm) isInteractive() bool { return m.anim.isInteractive() }
func (m *hostForm) close() tea.Cmd     { return m.anim.close() }
func (m *hostForm) setSize(w, h int)   { m.screenW, m.screenH = w, h }

func blankFields() []formField {
	f := make([]formField, fCount)
	f[fName] = formField{label: "Name"}
	f[fHost] = formField{label: "Host"}
	f[fPort] = formField{label: "Port", value: strconv.Itoa(store.DefaultPort), digits: true}
	f[fUser] = formField{label: "User"}
	f[fAuth] = formField{label: "Auth", kind: fieldToggle,
		options: []string{string(store.AuthPassword), string(store.AuthPrivateKey),
			string(store.AuthCredential)}}
	f[fCredential] = formField{label: "Credential",
		placeholder: "enter to choose a saved credential"}
	f[fIdentity] = formField{label: "IdentityFile",
		placeholder: "enter to browse " + store.FoldHome(identityRoot())}
	f[fPassword] = formField{label: "Password", mask: true}
	for i := range f {
		f[i].caret = len([]rune(f[i].value))
	}
	return f
}

// openCreate and openEdit are the two ways in. Both land on Name.
func (m *hostForm) openCreate(layer int) tea.Cmd {
	m.fields, m.focus, m.editing, m.err, m.errIdx = blankFields(), fName, "", "", -1
	m.submitted = false
	m.fields[fAuth].sel = 1 // privatekey — the common case, and the safer default
	m.layer = layer
	return m.anim.open()
}

func (m *hostForm) openEdit(h store.Host, layer int) tea.Cmd {
	f := blankFields()
	f[fName].value = h.Name
	f[fHost].value = h.Host
	f[fPort].value = strconv.Itoa(h.Port)
	f[fUser].value = h.User
	f[fCredential].value = h.Credential
	f[fIdentity].value = h.IdentityFile
	f[fPassword].value = h.Password
	switch h.Auth {
	case store.AuthPrivateKey:
		f[fAuth].sel = 1
	case store.AuthCredential:
		f[fAuth].sel = 2
	}
	for i := range f {
		f[i].caret = len([]rune(f[i].value))
	}
	m.fields, m.focus, m.editing, m.err, m.errIdx = f, fName, h.Name, "", -1
	m.submitted = false
	m.layer = layer
	return m.anim.open()
}

func (m hostForm) auth() store.AuthMethod {
	return store.AuthMethod(m.fields[fAuth].options[m.fields[fAuth].sel])
}

// enabled decides whether a field takes part at all. The disabled rows still
// occupy their lines: the popup keeps a constant height as Auth flips, and
// the user can see what the other choices would offer. User goes dark on a
// credential host because the credential supplies it — one package, and the
// form saying otherwise would be the form lying about who connects.
func (m hostForm) enabled(i int) bool {
	switch i {
	case fUser:
		return m.auth() != store.AuthCredential
	case fCredential:
		return m.auth() == store.AuthCredential
	case fIdentity:
		return m.auth() == store.AuthPrivateKey
	case fPassword:
		return m.auth() == store.AuthPassword
	}
	return true
}

func (m *hostForm) moveFocus(d int) {
	for i := 0; i < fCount; i++ {
		m.focus = (m.focus + d + fCount) % fCount
		if m.enabled(m.focus) {
			return
		}
	}
}

// update handles a keystroke and says what the caller should do next.
func (m hostForm) update(msg tea.KeyMsg) (hostForm, formResult) {
	if !m.anim.isInteractive() {
		return m, formNone
	}
	f := &m.fields[m.focus]

	// Alt-modified keys are commands, not characters — swallowed here so that a
	// stray Alt+x cannot type an "x" into the field.
	if msg.Alt {
		return m, formNone
	}

	switch msg.Type {
	case tea.KeyTab, tea.KeyDown:
		// Tab is "next" on every field now. It used to open the picker on the
		// path row, which cost the one thing Tab does everywhere else; the
		// picker moved to Enter, where the empty field has nothing else for
		// Enter to mean.
		m.moveFocus(1)
		return m, formNone
	case tea.KeyShiftTab, tea.KeyUp:
		m.moveFocus(-1)
		return m, formNone
	case tea.KeyEnter:
		// The two pick-a-value fields: Enter on the empty field opens the
		// chooser, Enter on a filled one moves on. The exchange is enter →
		// pick → enter → next field, and replacing a value is Backspace (the
		// whole line) then Enter again.
		switch m.focus {
		case fIdentity:
			if strings.TrimSpace(f.value) == "" {
				return m, formBrowse
			}
			m.moveFocus(1)
			return m, formNone
		case fCredential:
			if strings.TrimSpace(f.value) == "" {
				return m, formPickCred
			}
			m.moveFocus(1)
			return m, formNone
		}
		return m, formSubmit
	case tea.KeyBackspace:
		// A picked value is replaced, not shaved letter by letter.
		if m.focus == fIdentity || m.focus == fCredential {
			f.value, f.caret = "", 0
			return m, formNone
		}
	}
	if editField(f, msg) && f.kind == fieldToggle {
		m.syncFocus() // the toggle may have disabled the focused row
	}
	return m, formNone
}

// editField applies one EDITING keystroke to a field: the caret moves, the
// toggle cycles, text arrives, characters die. Navigation and submit stay
// with the form that owns the field — this is the part every form shares.
// Reports whether it consumed the key.
func editField(f *formField, msg tea.KeyMsg) bool {
	switch msg.Type {
	case tea.KeyLeft:
		if f.kind == fieldToggle {
			f.sel = (f.sel + len(f.options) - 1) % len(f.options)
		} else {
			f.caret = max(0, f.caret-1)
		}
	case tea.KeyRight:
		if f.kind == fieldToggle {
			f.sel = (f.sel + 1) % len(f.options)
		} else {
			f.caret = min(len([]rune(f.value)), f.caret+1)
		}
	case tea.KeyHome:
		f.caret = 0
	case tea.KeyEnd:
		f.caret = len([]rune(f.value))
	case tea.KeyBackspace:
		if r := []rune(f.value); f.kind == fieldText && f.caret > 0 {
			f.value = string(r[:f.caret-1]) + string(r[f.caret:])
			f.caret--
		}
	case tea.KeyDelete:
		if r := []rune(f.value); f.kind == fieldText && f.caret < len(r) {
			f.value = string(r[:f.caret]) + string(r[f.caret+1:])
		}
	case tea.KeySpace:
		insertRune(f, ' ')
	case tea.KeyRunes:
		for _, r := range msg.Runes {
			insertRune(f, r)
		}
	default:
		return false
	}
	return true
}

func insertRune(f *formField, r rune) {
	if f.kind != fieldText {
		return
	}
	if f.digits && (r < '0' || r > '9') {
		return
	}
	rs := []rune(f.value)
	f.value = string(rs[:f.caret]) + string(r) + string(rs[f.caret:])
	f.caret++
}

// syncFocus keeps the cursor off a row that Auth just disabled.
func (m *hostForm) syncFocus() {
	if !m.enabled(m.focus) {
		m.moveFocus(1)
	}
}

// host builds the record. Port parsing is total: an empty or absurd value falls
// out of store's Validate with a message the form can show.
func (m hostForm) host() store.Host {
	port, _ := strconv.Atoi(strings.TrimSpace(m.fields[fPort].value))
	h := store.Host{
		Name: strings.TrimSpace(m.fields[fName].value),
		Host: strings.TrimSpace(m.fields[fHost].value),
		Port: port,
		User: strings.TrimSpace(m.fields[fUser].value),
		Auth: m.auth(),
	}
	switch h.Auth {
	case store.AuthPrivateKey:
		h.IdentityFile = strings.TrimSpace(m.fields[fIdentity].value)
	case store.AuthCredential:
		// The credential supplies the user — writing a stale one beside it
		// would put two answers to "who connects" in the same record.
		h.Credential = strings.TrimSpace(m.fields[fCredential].value)
		h.User = ""
	default:
		h.Password = m.fields[fPassword].value
	}
	return h
}

// fail parks an error on the form instead of stacking another popup on top of
// it — the error must be visible without blocking the fix (§6.7). It jumps focus
// to the offending field, because this is the submit path: the user asked to be
// taken to the problem.
func (m *hostForm) fail(msg string, field int) {
	m.err, m.errIdx = msg, field
	if field >= 0 && m.enabled(field) {
		m.focus = field
	}
}

// refreshError re-marks the form from a fresh validation WITHOUT moving focus.
// This runs on every keystroke after the first submit, so the message always
// describes the form as it stands now. Moving focus here would yank the cursor
// out from under someone mid-word.
func (m *hostForm) refreshError(msg string, field int) {
	m.err, m.errIdx = msg, field
}

func (m hostForm) view() string {
	labelW := 0
	for _, f := range m.fields {
		labelW = max(labelW, dispW(f.label))
	}
	// Wide enough that the three-way Auth toggle shows all its options on a
	// normal terminal; narrow ones still fall back to the selected-only form.
	innerW := popupInnerW(m.screenW, labelW+52)
	// On a narrow terminal the label column yields rather than squeezing the
	// value out of existence — a truncated label still reads, an empty value does
	// not.
	labelCol := min(labelW+4, max(0, innerW-8))
	valueW := max(0, innerW-labelCol-1)

	rows := formBody(m.fields, m.focus, m.errIdx, m.err, m.enabled, innerW, labelCol, valueW)

	glyph, title := glyphPlus, "New host"
	if m.editing != "" {
		glyph, title = glyphPencil, "Edit host"
	}
	// The hint is contextual: it names what THIS field can do. That is the
	// standing disclosure a text-entry surface trades the Space entry key for
	// (§4.5), so it has to be accurate per field, not generic.
	// The hint names what THIS field does with Enter — on the two pick-a-value
	// rows Enter is not "save", and saying so is the standing disclosure (§4.5).
	var pairs [][2]string
	switch {
	case m.focus == fIdentity && strings.TrimSpace(m.fields[fIdentity].value) == "":
		pairs = [][2]string{{"Enter", "browse"}, {"Tab", "next"}, {"Esc", "cancel"}}
	case m.focus == fCredential && strings.TrimSpace(m.fields[fCredential].value) == "":
		pairs = [][2]string{{"Enter", "choose"}, {"Tab", "next"}, {"Esc", "cancel"}}
	case m.focus == fIdentity || m.focus == fCredential:
		pairs = [][2]string{{"Enter", "next"}, {"Backspace", "clear"}, {"Esc", "cancel"}}
	case m.fields[m.focus].kind == fieldToggle:
		pairs = [][2]string{{"Tab", "next"}, {arrowGlyphs, "switch"}, {"Enter", "save"}, {"Esc", "cancel"}}
	default:
		pairs = [][2]string{{"Tab", "next"}, {"Enter", "save"}, {"Esc", "cancel"}}
	}

	return drawPopupBox(popupLayerColor(m.layer), " "+glyph+" "+title+" ", hintLegend(pairs),
		animRows(m.anim, capRows(rows, m.screenH)), innerW)
}

// formBody renders any field form's rows: labels, values, and the standing
// error row — always present, blank when clean, so the popup does not change
// height the moment validation fails.
func formBody(fields []formField, focus, errIdx int, errMsg string,
	enabled func(int) bool, innerW, labelCol, valueW int) []string {
	dim := lipgloss.NewStyle().Foreground(dimColor)
	txt := lipgloss.NewStyle().Foreground(textColor)
	edit := lipgloss.NewStyle().Foreground(editColor)
	red := lipgloss.NewStyle().Foreground(warnColor)

	rows := make([]string, 0, len(fields)+2)
	for i, f := range fields {
		on := enabled(i)
		focused := i == focus

		lStyle := dim
		switch {
		case i == errIdx:
			lStyle = red
		case focused:
			lStyle = edit
		case on:
			lStyle = txt
		}
		label := lStyle.Render(padRight("  "+f.label, labelCol))

		var value string
		switch {
		case !on:
			value = dim.Render(padRight("—", valueW))
		case f.kind == fieldToggle:
			value = renderToggle(f, focused, valueW)
		default:
			value = renderTextValue(f, focused, valueW)
		}
		rows = append(rows, label+value+" ")
	}
	return append(rows, strings.Repeat(" ", innerW), red.Render(padRight("  "+errMsg, innerW)))
}

// renderToggle draws the segmented control with radio glyphs — filled on the
// choice, hollow on the rest. If the slot cannot hold every option it shows only
// the current one: an option cut in half would read as a different value.
func renderToggle(f formField, focused bool, w int) string {
	dim := lipgloss.NewStyle().Foreground(dimColor)
	on := lipgloss.NewStyle().Foreground(textColor)
	if focused {
		on = lipgloss.NewStyle().Foreground(editColor).Bold(true)
	}

	label := func(i int) string {
		if i == f.sel {
			return glyphRadioOn + " " + f.options[i]
		}
		return glyphRadioOff + " " + f.options[i]
	}
	join := func(ix []int) string {
		parts := make([]string, 0, len(ix))
		for _, i := range ix {
			parts = append(parts, label(i))
		}
		return strings.Join(parts, "  ")
	}

	shown := make([]int, 0, len(f.options))
	for i := range f.options {
		shown = append(shown, i)
	}
	if dispW(join(shown)) > w {
		shown = []int{f.sel}
	}
	plain := join(shown)
	if dispW(plain) > w {
		return on.Render(padRight(plain, w)) // last resort: keep the frame intact
	}

	styled := make([]string, 0, len(shown))
	for _, i := range shown {
		style := dim
		if i == f.sel {
			style = on
		}
		styled = append(styled, style.Render(label(i)))
	}
	return strings.Join(styled, "  ") + strings.Repeat(" ", w-dispW(plain))
}

// renderTextValue draws a value with a block caret on the focused field. The
// caret is a reversed cell rather than a trailing bar so it reads the same
// whether it sits inside the text or past the end.
func renderTextValue(f formField, focused bool, w int) string {
	txt := lipgloss.NewStyle().Foreground(textColor)
	dim := lipgloss.NewStyle().Foreground(dimColor)
	edit := lipgloss.NewStyle().Foreground(editColor)
	cur := lipgloss.NewStyle().Foreground(lipgloss.Color(baseHex)).Background(editColor)

	if w <= 0 {
		return "" // no room for a value at all; the label already took the row
	}
	shown := f.value
	if f.mask {
		shown = strings.Repeat("•", len([]rune(f.value)))
	}

	// An empty field says what belongs in it. Dim, so it can never be misread as
	// a value the user already entered.
	if f.value == "" && f.placeholder != "" {
		ph := truncate(f.placeholder, max(0, w-1))
		if !focused {
			return dim.Render(padRight(ph, w))
		}
		return cur.Render(" ") + dim.Render(ph) + strings.Repeat(" ", max(0, w-1-dispW(ph)))
	}
	if !focused {
		return txt.Render(padRight(shown, w))
	}

	rs := []rune(shown)
	caret := min(f.caret, len(rs))
	// Scroll the window so the caret stays visible in a value longer than the slot.
	// Guarded: with a one-cell slot the offset would run past the end of the value.
	if w > 1 && caret >= w-1 {
		start := min(caret-w+2, len(rs))
		rs = rs[start:]
		caret -= start
	}
	if caret >= len(rs) {
		rs = append(rs, ' ') // a cell for the caret to sit on past the end
	}

	head, at, tail := string(rs[:caret]), string(rs[caret]), string(rs[caret+1:])
	return edit.Render(head) + cur.Render(at) + edit.Render(tail) +
		strings.Repeat(" ", max(0, w-dispW(head+at+tail)))
}
