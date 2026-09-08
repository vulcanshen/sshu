package ui

import (
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/vulcanshen/sshu/internal/store"
)

// sshcfgForm creates and edits one Host block in ~/.ssh/config.
//
// It is the first form in sshu whose FIELD LIST is not fixed, and it had to be.
// ssh has roughly a hundred keywords; five of them are common enough to deserve
// a permanent row, and the rest are whatever this particular block happens to
// use. A form with only the five would open on a `Host *` block — the shape
// every macOS config has, holding AddKeysToAgent, UseKeychain and
// StrictHostKeyChecking — and show five empty boxes while touching nothing the
// block actually does.
//
// So: the five fixed rows, a divider, one row per option the block already
// carries (labelled with the keyword as the FILE spells it), and a last row
// that adds another. Every row is still a single-line text field, which is what
// keeps Enter meaning one thing (§11.34) — a multi-line editor would need Enter
// for a newline and would take the form's only submit key with it.
const (
	sfHost = iota
	sfHostName
	sfUser
	sfPort
	sfIdentity
	sfDivider
	sfFixedCount
)

// sfOptionRows is the fixed rows that are ssh OPTIONS. Host is not among them:
// it is the block's own patterns, not a keyword inside it.
var sfOptionRows = []struct {
	field int
	key   string
}{
	{sfHostName, "HostName"},
	{sfUser, "User"},
	{sfPort, "Port"},
	{sfIdentity, "IdentityFile"},
}

const (
	sshcfgAddLabel = "+ add option"
	sshcfgAddHint  = "Keyword value"
)

type sshcfgForm struct {
	anim   popupAnimator
	fields []formField
	// opts is what the rows below the divider stand for, aligned to
	// fields[sfFixedCount : len-1]. Key is the keyword as the file spells it and
	// At is the line it came from — together they are what lets a save rewrite
	// that exact line instead of appending a second copy ssh would ignore.
	opts []store.SSHOption
	// fixed carries the same two facts for the rows above the divider. An array
	// rather than a map because this model is copied by value on every keystroke.
	fixed [sfFixedCount]store.SSHOption

	focus     int
	editing   int // index of the block being changed, or -1 to create one
	err       string
	errIdx    int
	submitted bool
	layer     int
	screenW   int
	screenH   int
}

func newSSHCfgForm() sshcfgForm {
	return sshcfgForm{anim: newPopupAnimator("sshcfgform"), errIdx: -1, editing: -1}
}

func (m sshcfgForm) isActive() bool    { return m.anim.isActive() }
func (m *sshcfgForm) close() tea.Cmd   { return m.anim.close() }
func (m *sshcfgForm) setSize(w, h int) { m.screenW, m.screenH = w, h }

// addRow is the last field: the one that turns typing into another option row.
func (m sshcfgForm) addRow() int { return sfFixedCount + len(m.opts) }

func blankSSHCfgFields() []formField {
	f := make([]formField, sfFixedCount)
	f[sfHost] = formField{label: "Host", placeholder: "pattern — prod, prod-*, or *"}
	f[sfHostName] = formField{label: "HostName"}
	f[sfUser] = formField{label: "User"}
	f[sfPort] = formField{label: "Port", digits: true}
	f[sfIdentity] = formField{label: "IdentityFile",
		placeholder: "enter to browse " + store.FoldHome(identityRoot())}
	f[sfDivider] = formField{label: "─ other options", separator: true}
	return f
}

func (m *sshcfgForm) openCreate(layer int) tea.Cmd {
	m.fields = append(blankSSHCfgFields(),
		formField{label: sshcfgAddLabel, placeholder: sshcfgAddHint})
	m.opts, m.fixed = nil, [sfFixedCount]store.SSHOption{}
	m.focus, m.editing, m.err, m.errIdx = sfHost, -1, "", -1
	m.submitted = false
	m.layer = layer
	return m.anim.open()
}

// openEdit lays the block out over the form. The five fixed rows take the FIRST
// line with their keyword — the one ssh would use — and every remaining line,
// including a second copy of a keyword that already has a row, becomes an
// option row of its own. Nothing in the block is left out, because a form that
// quietly drops a line deletes it the moment you save.
func (m *sshcfgForm) openEdit(b store.SSHBlock, at, layer int) tea.Cmd {
	f := blankSSHCfgFields()
	f[sfHost].value = b.Patterns

	var fixed [sfFixedCount]store.SSHOption
	taken := make([]bool, len(b.Options))
	for _, r := range sfOptionRows {
		if j := b.IndexOf(r.key); j >= 0 {
			f[r.field].value = b.Options[j].Value
			fixed[r.field] = b.Options[j]
			taken[j] = true
		}
	}

	var opts []store.SSHOption
	for j, o := range b.Options {
		if taken[j] {
			continue
		}
		opts = append(opts, o)
		f = append(f, formField{label: o.Key, value: o.Value})
	}
	f = append(f, formField{label: sshcfgAddLabel, placeholder: sshcfgAddHint})
	for i := range f {
		f[i].caret = len([]rune(f[i].value))
	}

	m.fields, m.opts, m.fixed = f, opts, fixed
	m.focus, m.editing, m.err, m.errIdx = sfHost, at, "", -1
	m.submitted = false
	m.layer = layer
	return m.anim.open()
}

// openDuplicate is openEdit that creates. Every At is cleared: those line
// numbers belong to the block this was copied FROM, and a new block's lines do
// not exist yet.
func (m *sshcfgForm) openDuplicate(b store.SSHBlock, layer int) tea.Cmd {
	cmd := m.openEdit(b, -1, layer)
	m.editing = -1
	for i := range m.opts {
		m.opts[i].At = 0
	}
	for i := range m.fixed {
		m.fixed[i].At = 0
	}
	return cmd
}

// enabled is what formBody and moveFocus ask. Only the divider is off: it is a
// rule, not a row anybody types into.
func (m sshcfgForm) enabled(i int) bool { return i != sfDivider }

// complete is §11.34's question, answered for a file where almost everything is
// optional: a Host block needs a pattern and nothing else. HostName, User and
// the rest are absent from most real blocks on purpose, so demanding them would
// make Enter refuse to save a block that is already correct.
func (m sshcfgForm) complete() bool {
	return strings.TrimSpace(m.fields[sfHost].value) != ""
}

func (m *sshcfgForm) moveFocus(d int) {
	n := len(m.fields)
	for i := 0; i < n; i++ {
		m.focus = (m.focus + d + n) % n
		if m.enabled(m.focus) {
			return
		}
	}
}

func (m sshcfgForm) update(msg tea.KeyMsg) (sshcfgForm, formResult) {
	if !m.anim.isInteractive() {
		return m, formNone
	}
	if msg.Alt {
		return m, formNone
	}
	f := &m.fields[m.focus]

	switch msg.Type {
	case tea.KeyTab, tea.KeyDown:
		if m.focus == m.addRow() && strings.TrimSpace(f.value) != "" {
			m.takeAddRow()
			return m, formNone
		}
		m.moveFocus(1)
		return m, formNone
	case tea.KeyShiftTab, tea.KeyUp:
		m.moveFocus(-1)
		return m, formNone
	case tea.KeyEnter:
		// The empty path row keeps the picker, as it does on both sibling forms:
		// it is the only row that cannot sensibly be typed.
		if m.focus == sfIdentity && strings.TrimSpace(f.value) == "" {
			return m, formBrowse
		}
		// On the add row Enter means "add this one", which is still §11.34's one
		// question — is this finished? A half-typed option is not, so Enter
		// finishes it rather than saving the block around it.
		if m.focus == m.addRow() && strings.TrimSpace(f.value) != "" {
			m.takeAddRow()
			return m, formNone
		}
		if !m.complete() {
			m.moveFocus(1)
			return m, formNone
		}
		return m, formSubmit
	case tea.KeyBackspace:
		if m.focus == sfIdentity {
			f.value, f.caret = "", 0
			return m, formNone
		}
	}
	editField(f, msg)
	return m, formNone
}

// takeAddRow turns `Keyword value` into a row of its own and lands the cursor
// on it, so the next thing typed is that option's value.
//
// A keyword the form already has a row for does NOT get a second one — the
// cursor goes to the existing row instead. ssh reads only the first of a
// repeated keyword, so a second row would be a line that looks like it works
// and does nothing.
func (m *sshcfgForm) takeAddRow() {
	at := m.addRow()
	key, value, _ := strings.Cut(strings.TrimSpace(m.fields[at].value), " ")
	value = strings.TrimSpace(value)

	if bad := badKeyword(key); bad != "" {
		m.fail(bad, at)
		return
	}
	m.err, m.errIdx = "", -1
	m.fields[at].value, m.fields[at].caret = "", 0

	if j := m.rowFor(key); j >= 0 {
		m.focus = j
		return
	}

	m.opts = append(m.opts, store.SSHOption{Key: key, Value: value})
	m.fields = append(m.fields, formField{})
	copy(m.fields[at+1:], m.fields[at:])
	m.fields[at] = formField{label: key, value: value, caret: len([]rune(value))}
	m.fields[at+1] = formField{label: sshcfgAddLabel, placeholder: sshcfgAddHint}
	m.focus = at
}

// rowFor is the field already standing for this keyword, or -1.
func (m sshcfgForm) rowFor(key string) int {
	for _, r := range sfOptionRows {
		if strings.EqualFold(r.key, key) {
			return r.field
		}
	}
	for j, o := range m.opts {
		if strings.EqualFold(o.Key, key) {
			return sfFixedCount + j
		}
	}
	return -1
}

// badKeyword rejects what cannot be an option line, and says why.
func badKeyword(key string) string {
	if key == "" {
		return "Type a keyword, then its value"
	}
	for _, r := range key {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-':
		default:
			return "A keyword is letters, digits and hyphens"
		}
	}
	// Either of these ENDS the block at that line and starts another, so half
	// the options above it would silently belong somewhere else.
	if k := strings.ToLower(key); k == "host" || k == "match" {
		return strconv.Quote(key) + " starts a new block — it cannot be an option"
	}
	return ""
}

// block is what the form is describing. An option with an empty value is left
// out, which is how a line gets deleted: clearing the field is the only gesture
// that could mean "take this out", and it is the one people reach for.
func (m sshcfgForm) block() store.SSHBlock {
	b := store.SSHBlock{Patterns: strings.TrimSpace(m.fields[sfHost].value)}
	for _, r := range sfOptionRows {
		v := strings.TrimSpace(m.fields[r.field].value)
		if v == "" {
			continue
		}
		// The file's own spelling wins over the canonical one: a config that
		// says `hostname` is not wrong, and correcting it would be a diff line
		// nobody asked for.
		key := r.key
		if m.fixed[r.field].Key != "" {
			key = m.fixed[r.field].Key
		}
		b.Options = append(b.Options,
			store.SSHOption{Key: key, Value: v, At: m.fixed[r.field].At})
	}
	for j, o := range m.opts {
		v := strings.TrimSpace(m.fields[sfFixedCount+j].value)
		if v == "" {
			continue
		}
		b.Options = append(b.Options, store.SSHOption{Key: o.Key, Value: v, At: o.At})
	}
	return b
}

func (m *sshcfgForm) fail(msg string, field int) {
	m.err, m.errIdx = msg, field
	if field >= 0 && field < len(m.fields) && m.enabled(field) {
		m.focus = field
	}
}

func (m *sshcfgForm) refreshError(msg string, field int) { m.err, m.errIdx = msg, field }

// window is the slice of fields on screen. Unlike the other two forms this one
// has no fixed height — a `Host *` block can carry a dozen options — so the
// rows scroll with the cursor rather than being cut off at the bottom by
// capRows, which would leave a field you can Tab to and cannot see.
func (m sshcfgForm) window() (int, int) {
	// capRows allows screenH-6; formBody adds a blank row and an error row.
	vis := max(3, m.screenH-8)
	if len(m.fields) <= vis {
		return 0, len(m.fields)
	}
	lo := clamp(m.focus-vis/2, 0, len(m.fields)-vis)
	return lo, lo + vis
}

func (m sshcfgForm) view() string {
	labelW := 0
	for _, f := range m.fields {
		labelW = max(labelW, dispW(f.label))
	}
	innerW := popupInnerW(m.screenW, labelW+38)
	labelCol := min(labelW+4, max(0, innerW-8))
	valueW := max(0, innerW-labelCol-1)

	lo, hi := m.window()
	rows := formBody(m.fields[lo:hi], m.focus-lo, m.errIdx-lo, m.err,
		func(i int) bool { return m.enabled(i + lo) }, innerW, labelCol, valueW)

	glyph, title := glyphPlus, "New Host block"
	if m.editing >= 0 {
		glyph, title = glyphPencil, "Edit Host block"
	}

	enter := "save"
	if !m.complete() {
		enter = "next"
	}
	var pairs [][2]string
	switch {
	case m.focus == m.addRow():
		pairs = [][2]string{{"Enter", "add option"}, {"Esc", "cancel"}}
	case m.focus == sfIdentity && strings.TrimSpace(m.fields[sfIdentity].value) == "":
		pairs = [][2]string{{"Enter", "browse"}, {"Tab", "next"}, {"Esc", "cancel"}}
	case m.focus == sfIdentity:
		pairs = [][2]string{{"Enter", enter}, {"Backspace", "clear"}, {"Esc", "cancel"}}
	default:
		pairs = [][2]string{{"Tab", "next"}, {"Enter", enter}, {"Esc", "cancel"}}
	}

	return drawPopupBox(popupLayerColor(m.layer), " "+glyph+" "+title+" ", hintLegend(pairs),
		animRows(m.anim, capRows(rows, m.screenH)), innerW)
}
