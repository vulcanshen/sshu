package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/vulcanshen/sshu/internal/store"
)

// detailPopup is the §6.1 VIEWPORT class — the same family as `?` help and the
// file viewer: scrollable, cursorless, read-only. It answers one question, "what
// is this row actually made of", for a hosts.yaml entry or a credential.
//
// It exists because the table cannot answer it. Rows shed columns as the
// terminal narrows (§1.4), the auth column is a glyph rather than a word, and
// the one field nobody should ever put on a table — the password — has to be
// visible AS A FACT ("there is one, it is stored") without being visible as a
// value. Opening the edit form to look was the alternative, and a form is a
// thing you can accidentally change.
//
// Labels are dim and values bright, which is the OPPOSITE of the help popup's
// pairing. That is not an inconsistency: §4.4's "bright key, dim description"
// is about a key you press, and there is no key here. The bright half is
// whichever half is the answer.
type detailPopup struct {
	anim     popupAnimator
	title    string
	sections []detailSection
	top      int
	layer    int
	screenW  int
	screenH  int
}

// detailSection groups rows under a dim heading.
type detailSection struct {
	title string
	rows  []detailRow
}

// detailRow is one label/value line. warn paints the value red — a host whose
// credential has gone missing cannot connect, and that is the most useful thing
// this popup can say about it.
type detailRow struct {
	label string
	value string
	warn  bool
}

func newDetailPopup() detailPopup { return detailPopup{anim: newPopupAnimator("detail")} }

func (m detailPopup) isActive() bool      { return m.anim.isActive() }
func (m detailPopup) isInteractive() bool { return m.anim.isInteractive() }
func (m *detailPopup) close() tea.Cmd     { return m.anim.close() }
func (m *detailPopup) setSize(w, h int)   { m.screenW, m.screenH = w, h }

func (m *detailPopup) show(title string, sections []detailSection, layer int) tea.Cmd {
	m.title, m.sections, m.top, m.layer = title, sections, 0, layer
	return m.anim.open()
}

func (m *detailPopup) update(msg tea.KeyMsg) {
	if !m.anim.isInteractive() {
		return
	}
	m.top = moveScroll(m.top, max(0, len(m.lines())-m.visible()), msg.String(), m.visible())
}

// maskedSecret is what a stored password looks like here. Deliberately a FIXED
// width rather than one bullet per rune the way the form draws it: in the form
// you are typing the thing and the length is your own, but a read-only look at
// somebody's saved password has no reason to publish how long it is.
const maskedSecret = "••••••••"

// hostDetail is the [V]iew contents for one host: what it connects to, and how.
//
// A credential host is shown as what it IS plus what it RESOLVES TO — the
// credential's name, then the user and secret it supplies. Resolve treats a
// credential as one package rather than a set of defaults (store.Resolve), so
// the rows it contributes sit together under it rather than being mixed into
// the connection half.
func hostDetail(h store.Host, creds []store.Credential) []detailSection {
	conn := detailSection{title: "Connection", rows: []detailRow{
		{label: "Name", value: h.Name},
		{label: "Host", value: h.Host},
		{label: "Port", value: itoa(h.Port)},
	}}
	// A credential host has no user of its own — the credential supplies it, and
	// it is listed there. Printing an empty User row here would suggest the
	// field was left blank rather than deliberately delegated.
	if h.Auth != store.AuthCredential {
		conn.rows = append(conn.rows, detailRow{label: "User", value: h.User})
	}

	auth := detailSection{title: "Auth", rows: []detailRow{
		{label: "Type", value: string(h.Auth)},
	}}
	switch h.Auth {
	case store.AuthPassword:
		auth.rows = append(auth.rows, detailRow{label: "Password", value: maskedSecret})
	case store.AuthPrivateKey:
		auth.rows = append(auth.rows, detailRow{label: "Identity file", value: h.IdentityFile})
	case store.AuthCredential:
		rh, err := store.Resolve(h, creds)
		if err != nil {
			// The break is ON the Credential row rather than on a line of its
			// own beneath it: the name and the fact that the name resolves to
			// nothing are one piece of information, and splitting them puts an
			// unlabelled sentence in a column of labelled values.
			auth.rows = append(auth.rows, detailRow{label: "Credential",
				value: h.Credential + " — missing, cannot connect", warn: true})
			break
		}
		auth.rows = append(auth.rows,
			detailRow{label: "Credential", value: h.Credential},
			detailRow{label: "User", value: rh.User})
		auth.rows = append(auth.rows, credSecretRow(rh.Auth, rh.IdentityFile)...)
	}
	return []detailSection{conn, auth}
}

// credDetail is the [V]iew contents for one credential: the auth half only,
// which is the whole of what a credential is. Its name is the popup's title.
func credDetail(c store.Credential) []detailSection {
	rows := []detailRow{
		{label: "User", value: c.User},
		{label: "Type", value: string(c.Auth)},
	}
	rows = append(rows, credSecretRow(c.Auth, c.IdentityFile)...)
	return []detailSection{{title: "Auth", rows: rows}}
}

// credSecretRow is the one row that says how the secret is held — shared so a
// credential reads identically on its own popup and inside a host's.
func credSecretRow(auth store.AuthMethod, identity string) []detailRow {
	if auth == store.AuthPrivateKey {
		return []detailRow{{label: "Identity file", value: identity}}
	}
	return []detailRow{{label: "Password", value: maskedSecret}}
}

// labelW is the widest label; a section heading is not a label.
func (m detailPopup) labelW() int {
	w := 0
	for _, s := range m.sections {
		for _, r := range s.rows {
			w = max(w, dispW(r.label))
		}
	}
	return w
}

// lines flattens the sections into the rows the viewport scrolls, so the
// scroll maths and the drawing agree on what a row is. A blank label marks a
// heading; the drawing tells them apart by index against the same walk.
func (m detailPopup) lines() []detailRow {
	var out []detailRow
	for i, s := range m.sections {
		if i > 0 {
			out = append(out, detailRow{})
		}
		out = append(out, detailRow{label: "", value: s.title})
		out = append(out, s.rows...)
	}
	return out
}

func (m detailPopup) visible() int {
	return max(1, min(len(m.lines()), m.screenH-6))
}

func (m detailPopup) view() string {
	// Sized to what it holds, the way the confirm box is: an identity path and
	// a masked password want very different widths, and a fixed one either cuts
	// the path or leaves half the box empty under the bullets.
	labelW, valueW := m.labelW(), 0
	for _, r := range m.lines() {
		valueW = max(valueW, dispW(r.value))
	}
	innerW := popupInnerW(m.screenW, max(labelW+4+valueW+1, dispW(m.title)+6))
	// Same yielding rule the form uses: on a narrow terminal the label column
	// gives way rather than squeezing the value out of existence.
	labelCol := min(labelW+4, max(0, innerW-8))
	valueW = max(0, innerW-labelCol-1)

	dim := lipgloss.NewStyle().Foreground(dimColor)
	txt := lipgloss.NewStyle().Foreground(textColor)
	red := lipgloss.NewStyle().Foreground(warnColor)

	all := m.lines()
	vis := m.visible()
	end := min(len(all), m.top+vis)
	rows := make([]string, 0, vis)
	for _, r := range all[m.top:end] {
		if r.label == "" {
			// A heading, a spacer, or the warning that has no field name — all
			// three are one full-width line rather than an empty label column.
			style := dim
			if r.warn {
				style = red
			}
			rows = append(rows, style.Render(padRight(" "+r.value, innerW)))
			continue
		}
		value := txt.Render(padRight(truncate(r.value, valueW), valueW))
		if r.warn {
			value = red.Render(padRight(truncate(r.value, valueW), valueW))
		}
		rows = append(rows, dim.Render(padRight("  "+r.label, labelCol))+value)
	}

	pairs := [][2]string{{"Esc", "close"}}
	if len(all) > vis {
		pairs = append([][2]string{{"j/k", "scroll"}}, pairs...)
	}
	return drawPopupBox(popupLayerColor(m.layer), " "+glyphEye+" "+m.title+" ",
		hintLegend(pairs), animRows(m.anim, capRows(rows, m.screenH)), innerW)
}

// openHostView and openCredView are the two [V]iew actions. Both read the row
// under the cursor through the same accessor the rest of the table uses, so a
// filtered list opens the row you are looking at rather than the Nth of the
// unfiltered one.
func (m AppModel) openHostView() (tea.Model, tea.Cmd) {
	h, ok := m.cursorHost()
	if !ok {
		return m, nil
	}
	return m, m.detail.show(nameOr(h.Name, "host"), hostDetail(h, m.creds.creds), m.layer())
}

func (m AppModel) openCredView() (tea.Model, tea.Cmd) {
	c, ok := m.cursorCred()
	if !ok {
		return m, nil
	}
	return m, m.detail.show(nameOr(c.Name, "credential"), credDetail(c), m.layer())
}

// nameOr keeps the title from collapsing to a bare glyph on an entry whose name
// is somehow empty — hand-edited files reach the UI unvalidated (store.LoadFrom).
func nameOr(name, fallback string) string {
	if strings.TrimSpace(name) == "" {
		return fallback
	}
	return name
}
