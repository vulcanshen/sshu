package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/vulcanshen/sshu/internal/store"
)

// detailAction is what Enter does on a detail float, or detailNone when the
// float is only being read.
type detailAction int

const (
	detailNone detailAction = iota
	detailConnect
	detailEditCred
	detailEditSSHCfg
	detailEditKnown
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
// It is ALSO the last thing between the row and what Enter does to it (§11.29):
// prompt and accept turn the foot of the float into the question a separate
// confirmation used to ask. Reading and deciding were two floats over the same
// row, and the confirmation said strictly less than this one already does.
//
// Labels are dim and values bright, which is the OPPOSITE of the help popup's
// pairing. That is not an inconsistency: §4.4's "bright key, dim description"
// is about a key you press, and there is no key here. The bright half is
// whichever half is the answer.
type detailPopup struct {
	anim     popupAnimator
	title    string
	sections []detailSection
	// prompt is the question at the foot of the float; accept is the verb the
	// hint gives Enter. Both empty means there is nothing to commit, and Enter
	// belongs to the viewport.
	prompt string
	accept string
	action detailAction
	target string // the name the action applies to
	// at is the same thing for a row that has no name: a ~/.ssh/config block is
	// identified by where it sits, because two of them may share a pattern and
	// ssh means something by that (§11.39).
	at      int
	top     int
	layer   int
	screenW int
	screenH int
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
	// depth indents a row under the one above it, two cells a level.
	//
	// The ~/.ssh/config sections are the only three-level thing in this float —
	// a FILE, the Host blocks inside it, and each block's options — and without
	// a depth the middle level had nowhere to sit: a heading and a label row
	// are otherwise one cell apart, which reads as "these are siblings".
	depth int
}

func newDetailPopup() detailPopup { return detailPopup{anim: newPopupAnimator("detail")} }

func (m detailPopup) isActive() bool      { return m.anim.isActive() }
func (m detailPopup) isInteractive() bool { return m.anim.isInteractive() }
func (m *detailPopup) close() tea.Cmd     { return m.anim.close() }
func (m *detailPopup) setSize(w, h int)   { m.screenW, m.screenH = w, h }

// show replaces the float's contents wholesale, keeping the things that belong
// to the screen rather than to this particular float. Same shape as
// confirmPopup.ask, and for the same reason: a caller that has to remember to
// clear last time's prompt would eventually forget.
func (m *detailPopup) show(c detailPopup, layer int) tea.Cmd {
	anim, w, h := m.anim, m.screenW, m.screenH
	*m = c
	m.anim, m.screenW, m.screenH = anim, w, h
	m.top, m.layer = 0, layer
	return m.anim.open()
}

// commit reports that Enter was pressed on a float that is listening. Whether
// there is anything to commit is detailCommit's question and ONLY its question:
// asking it here as well would be the same rule written twice, and Enter on a
// float with no offer is inert either way — a viewport has no use for the key.
// Esc is the caller's, exactly as it is on the confirmation box (§4.3).
func (m detailPopup) commit(msg tea.KeyMsg) bool {
	return m.anim.isInteractive() && msg.String() == "enter"
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

// hostDetail is the contents for one host: what it connects to, and how.
//
// A credential host is shown as what it IS plus what it RESOLVES TO — the
// credential's name, then the user and secret it supplies. Resolve treats a
// credential as one package rather than a set of defaults (store.Resolve), so
// the rows it contributes sit together under it rather than being mixed into
// the connection half.
func hostDetail(h store.Host, creds []store.Credential, cfg store.SSHConfigFile,
	timeoutSecs int) []detailSection {
	port := ""
	if h.Port > 0 {
		port = itoa(h.Port)
	}
	conn := detailSection{title: "Connection", rows: []detailRow{
		{label: "Name", value: h.Name},
		{label: "Host", value: h.Host},
		{label: "Port", value: orSSHDecides(port)},
	}}
	// A credential host has no user of its own — the credential supplies it, and
	// it is listed there. Printing an empty User row here would suggest the
	// field was left blank rather than deliberately delegated.
	if h.Auth != store.AuthCredential {
		conn.rows = append(conn.rows, detailRow{label: "User", value: orSSHDecides(h.User)})
	}
	// Tags sit in the first section because that section is really "what is this
	// record", which is also where Name lives — neither is a connection
	// parameter. Shown only when there ARE some: an empty row would read as a
	// field left blank, and for most hosts having no tags is the answer, not an
	// omission. Same judgement the form makes by marking the row optional.
	if len(h.Tags) > 0 {
		conn.rows = append(conn.rows, detailRow{label: "Tags", value: strings.Join(h.Tags, " ")})
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
	case store.AuthSSHConfig:
		// The one fact the row cannot show: nothing is stored. The key, the
		// agent, a password — ssh finds or asks for them at connect time.
		auth.rows = append(auth.rows,
			detailRow{label: "Secrets", value: "none stored — ssh asks at connect time"})
	}
	return append([]detailSection{conn, auth}, sshConfigSections(h, creds, cfg, timeoutSecs)...)
}

// sshDecides is what an sshconfig host's empty Port or User row says: not a
// blank, which reads as a field somebody forgot, but where the answer will
// come from. Only such a host can have either empty (store.Host.Validate).
const sshDecides = "— ssh decides"

func orSSHDecides(v string) string {
	if v == "" {
		return sshDecides
	}
	return v
}

// sshConfigSections is what ~/.ssh/config will contribute to this host: ONE
// SECTION PER BLOCK that gets a word in, in the order ssh reads them — so the
// section nearest the top is the one whose values won.
//
// It has to be the union rather than "which block does this host match",
// because that is what ssh does: it merges every matching block and takes the
// first value for each keyword. A host normally matches several — its own, and
// the `Host *` every config ends up with — and takes something from each.
//
// The whole thing is absent when the file has nothing to say about this host,
// which is most of them: a host whose `host` field is a real address usually
// only meets `Host *`, and one that meets nothing at all draws nothing.
func sshConfigSections(h store.Host, creds []store.Credential, cfg store.SSHConfigFile,
	timeoutSecs int) []detailSection {
	opts := cfg.Effective(h.Host)
	if len(opts) == 0 {
		if h.Auth != store.AuthSSHConfig {
			return nil
		}
		// For every other kind an empty answer is the normal case and is not
		// drawn. For a host that chose to be resolved BY this file, no block
		// matching it is the one thing worth saying: the connection is still
		// tried, with ssh's defaults, and the user should know that is what
		// they are getting.
		return []detailSection{{title: glyphFileCog + " ~/.ssh/config", rows: []detailRow{
			{label: "Host " + h.Host, value: "no block matches — ssh will use its defaults", warn: true},
		}}}
	}
	// Resolved, because that is what buildSSHCmd is handed: a credential host's
	// user and key come from the credential, and those are the values that go
	// on the command line and beat the file.
	rh := h
	if r, err := store.Resolve(h, creds); err == nil {
		rh = r
	}
	// What sshu puts on the command line, which outranks the file. Keyed by the
	// keyword it beats, valued by what sshu sends instead — so a row can say
	// whether it is being overridden with something DIFFERENT, which is the only
	// case worth a red line. `User vulcan` beaten by `vulcan@` changes nothing,
	// and colouring it would be crying wolf on every host.
	type sent struct{ value, phrase string }
	sends := map[string]sent{
		"connecttimeout": {itoa(timeoutSecs), "-o ConnectTimeout=" + itoa(timeoutSecs)},
	}
	// Only what actually goes on the command line. An sshconfig host with no
	// port or user of its own sends neither, so the file's values stand and
	// there is nothing to mark (§11.52).
	if rh.Port > 0 {
		sends["port"] = sent{itoa(rh.Port), "-p " + itoa(rh.Port)}
	}
	if rh.User != "" {
		sends["user"] = sent{rh.User, rh.User + "@"}
	}
	if rh.Auth == store.AuthPrivateKey && rh.IdentityFile != "" {
		sends["identityfile"] = sent{rh.IdentityFile, "-i " + rh.IdentityFile}
		sends["identitiesonly"] = sent{"yes", "-o IdentitiesOnly=yes"}
	}

	// One section per FILE, the blocks inside it indented under its name and
	// their options indented again — three levels, because there are three
	// things: a file, the blocks in it, and what each block sets.
	//
	// Grouped by RUN, not by file outright. The order of these sections IS the
	// precedence order, and an Include sitting between two Host blocks really
	// does make ssh read root → included → root. Collecting all of one file's
	// blocks together would tidy that into a lie about which value won.
	var out []detailSection
	curBlock, curFile := -1, -1
	for _, o := range opts {
		blk := cfg.Blocks[o.Block]
		if blk.File() != curFile {
			curFile, curBlock = blk.File(), -1
			out = append(out, detailSection{
				title: glyphFileCog + " " + cfg.Path(curFile),
			})
		}
		sec := &out[len(out)-1]
		if o.Block != curBlock {
			curBlock = o.Block
			// A blank line between blocks of the same file, so two `Host` lines
			// a few rows apart do not read as one list.
			if len(sec.rows) > 0 {
				sec.rows = append(sec.rows, detailRow{})
			}
			sec.rows = append(sec.rows, detailRow{value: "Host " + o.From, depth: 1})
		}
		// The SAME depth as the Host line above it, not one more: a heading
		// already sits one cell left of its own labels — that is what
		// Connection(1)/Name(2) does — so nesting them by depth as well would
		// step 2 cells then 3, which reads as a level that is not there.
		row := detailRow{label: o.Key, value: o.Value, depth: 1}
		// The four sshu passes on the command line are the ones it silently
		// wins, INCLUDING when it is winning with a default nobody chose. A row
		// that printed only the file's value would be saying the wrong thing.
		if mine, ok := sends[strings.ToLower(o.Key)]; ok && !strings.EqualFold(mine.value, o.Value) {
			row.value, row.warn = o.Value+" — sshu sends "+mine.phrase, true
		}
		sec.rows = append(sec.rows, row)
	}

	// And what could ALSO apply and is not in the list above. A section headed
	// "what ssh will use" that quietly omits a source is worse than no section:
	// it would be read as complete, and it is not.
	//
	// Includes that sshu DID follow are not here — their blocks are up there
	// with the rest, under their own file's name. What is left is the ones it
	// could not: a pattern matching nothing, a file it could not read.
	//
	// Named, not counted. "1 file, not read" was both vague and WRONG: the one
	// counted Include DIRECTIVES, and a single one can be a glob standing for
	// ten files. What a reader needs is the string to go and look at.
	var maybe []detailRow
	for _, inc := range cfg.Unread {
		maybe = append(maybe, detailRow{label: "Include", value: inc})
	}
	for _, cond := range cfg.MatchConditions {
		maybe = append(maybe, detailRow{label: "Match",
			value: nameOr(cond, "(no condition)") + " — not evaluated"})
	}
	if len(maybe) > 0 {
		out = append(out, detailSection{title: "~/.ssh/config · may also apply", rows: maybe})
	}
	return out
}

// credDetail is the contents for one credential: the auth half only, which is
// the whole of what a credential is. Its name is the popup's title.
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

// labelW is the widest label AT ITS OWN INDENT; a section heading is not a
// label. Counting the indent is what keeps an indented label from running into
// the value column that was sized without it.
func (m detailPopup) labelW() int {
	w := 0
	for _, s := range m.sections {
		for _, r := range s.rows {
			w = max(w, dispW(r.label)+2*r.depth)
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

// promptRows is what the offer costs the scrolling area: a spacer and the
// question. It is a FIXED footer rather than one more scrollable line, because
// the hint promises Enter does something and a promise that can scroll off the
// top of the box is not one.
func (m detailPopup) promptRows() int {
	if m.prompt == "" {
		return 0
	}
	return 2
}

func (m detailPopup) visible() int {
	return max(1, min(len(m.lines()), m.screenH-6-m.promptRows()))
}

func (m detailPopup) view() string {
	// Sized to what it holds, the way the confirm box is: an identity path and
	// a masked password want very different widths, and a fixed one either cuts
	// the path or leaves half the box empty under the bullets.
	labelW, valueW := m.labelW(), 0
	for _, r := range m.lines() {
		valueW = max(valueW, dispW(r.value))
	}
	innerW := popupInnerW(m.screenW,
		max(labelW+4+valueW+1, dispW(m.title)+6, dispW(m.prompt)+4))
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
		pad := strings.Repeat(" ", 2*r.depth)
		if r.label == "" {
			// A heading, a spacer, or the warning that has no field name — all
			// three are one full-width line rather than an empty label column.
			style := dim
			if r.warn {
				style = red
			}
			rows = append(rows, style.Render(padRight(" "+pad+r.value, innerW)))
			continue
		}
		value := txt.Render(padRight(truncate(r.value, valueW), valueW))
		if r.warn {
			value = red.Render(padRight(truncate(r.value, valueW), valueW))
		}
		rows = append(rows, dim.Render(padRight("  "+pad+r.label, labelCol))+value)
	}

	// The offer sits under a blank line, at the foot of the box and outside the
	// scroll — the same position the confirmation's question held, so the eye
	// looks where it already looked.
	if m.prompt != "" {
		rows = append(rows, dim.Render(strings.Repeat(" ", innerW)),
			txt.Render(padRight(" "+truncate(m.prompt, max(0, innerW-1)), innerW)))
	}

	pairs := [][2]string{{"Esc", "close"}}
	if m.accept != "" {
		pairs = append([][2]string{{"Enter", m.accept}}, pairs...)
	}
	if len(all) > vis {
		pairs = append([][2]string{{"j/k", "scroll"}}, pairs...)
	}
	// The glyph says what the float IS, not what it offers: a read-only look at
	// a row, whichever door happens to be at the bottom of it.
	return drawPopupBox(popupLayerColor(m.layer), " "+glyphEye+" "+m.title+" ",
		hintLegend(pairs), animRows(m.anim, capRows(rows, m.screenH)), innerW)
}

// openHost and openCredDetail are what Enter does on the two tables. Both read
// the row under the cursor through the same accessor the rest of the table
// uses, so a filtered list opens the row you are looking at rather than the Nth
// of the unfiltered one.
func (m AppModel) openHost() (tea.Model, tea.Cmd) {
	h, ok := m.cursorHost()
	if !ok {
		return m, m.toast.show("No host selected", toastError)
	}
	c := detailPopup{title: nameOr(h.Name, "host"), sections: hostDetail(h, m.creds.creds, m.sshcfg.file, m.cfg.Seconds())}
	// The offer is only made when it can be kept. A host whose credential is
	// gone cannot be connected to, and the row inside already says which
	// credential and what it costs, in red — that is the refusal, said once and
	// in the place the user is looking (§11.34). It used to be a toast over a
	// float that never opened, which said less and landed somewhere else.
	if _, err := store.Resolve(h, m.creds.creds); err == nil {
		c.prompt = fmt.Sprintf("Connect to %q?", h.Name)
		c.accept, c.action, c.target = "connect", detailConnect, h.Name
	}
	return m, m.detail.show(c, m.layer())
}

func (m AppModel) openCredDetail() (tea.Model, tea.Cmd) {
	c, ok := m.cursorCred()
	if !ok {
		return m, m.toast.show("No credential selected", toastError)
	}
	return m, m.detail.show(detailPopup{
		title:    nameOr(c.Name, "credential"),
		sections: credDetail(c),
		prompt:   fmt.Sprintf("Edit %q?", c.Name),
		accept:   "edit",
		action:   detailEditCred,
		target:   c.Name,
	}, m.layer())
}

// nameOr keeps the title from collapsing to a bare glyph on an entry whose name
// is somehow empty — hand-edited files reach the UI unvalidated (store.LoadFrom).
func nameOr(name, fallback string) string {
	if strings.TrimSpace(name) == "" {
		return fallback
	}
	return name
}
