package ui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/vulcanshen/sshu/internal/remote"
	"github.com/vulcanshen/sshu/internal/store"
)

// knownAction is the third panel to use the one-table-behind-both shape.
type knownAction struct {
	key        string
	label      string
	hint       string
	needsEntry bool
	panelOp    bool
	run        func(AppModel) (tea.Model, tea.Cmd)
}

// There is deliberately no [D]uplicate here, and its absence is the design.
// Duplicating a host key would mean trusting the SAME key under a second name —
// which is what the name field's comma-separated list already is, in one line
// instead of two. An action with no meaning is worse than a missing one: it
// occupies a letter and teaches the wrong model of the file.
var knownActions = []knownAction{
	// item — the key under the cursor
	{key: "enter", label: "View", hint: "Enter . the whole key, and where it sits", needsEntry: true, run: AppModel.openKnownDetail},
	{key: "E", label: "Edit", hint: "which names this key is trusted for", needsEntry: true, run: AppModel.openKnownEdit},
	{key: "X", label: "Delete", hint: "stop trusting this key", needsEntry: true, run: AppModel.askDeleteKnown},

	// panel — the table
	{key: "A", label: "Add", hint: "ask a host for its key, then decide", panelOp: true, run: AppModel.openKnownAdd},
}

func (m AppModel) knownApplicable() ([]string, []knownAction) {
	_, _, has := m.cursorKnown()
	var keys []string
	var acts []knownAction
	for _, a := range knownActions {
		if a.needsEntry && !has {
			continue
		}
		keys, acts = append(keys, a.key), append(acts, a)
	}
	return keys, acts
}

func (m AppModel) knownKey(k string) (tea.Model, tea.Cmd) {
	keys, acts := m.knownApplicable()
	if i := hotkeyIndex(keys, k); i >= 0 {
		return acts[i].run(m)
	}
	m.known.handleKey(k)
	return m, nil
}

// cursorKnown is the entry under the cursor and its position. Position is the
// identity here for the same reason it is on the Config panel: one name can
// appear on several lines with different key types, all of them valid.
func (m AppModel) cursorKnown() (store.KnownHostEntry, int, bool) {
	if m.tab != tabPref || m.pref.item != prefKnownHosts {
		return store.KnownHostEntry{}, -1, false
	}
	e, ok := m.known.rowAt(m.known.cursor)
	return e, m.known.cursor, ok
}

func (m AppModel) knownMenuItems() []menuItem {
	_, acts := m.knownApplicable()
	var item, panel []menuItem
	for _, a := range acts {
		row := menuItem{label: a.label, key: a.key, hint: a.hint}
		if a.panelOp {
			panel = append(panel, row)
			continue
		}
		item = append(item, row)
	}
	if len(item) == 0 || len(panel) == 0 {
		return append(item, panel...)
	}
	out := []menuItem{{label: menuItemRegion, header: true}}
	out = append(out, item...)
	out = append(out, menuItem{separator: true},
		menuItem{label: menuPanelRegion, header: true})
	return append(out, panel...)
}

// ------------------------------------------------------------------ detail

// knownDetail is what the table cannot show: the whole fingerprint, which is
// the only form anybody can actually compare, and the raw name field — for a
// hashed entry that is the only thing there IS to show.
func knownDetail(e store.KnownHostEntry) []detailSection {
	rows := []detailRow{{label: "Names", value: e.Hosts}}
	if e.Hashed() {
		rows = append(rows, detailRow{label: "Hashed",
			value: "the name cannot be read back from the file"})
	}
	if e.Marker != "" {
		// @revoked inverts what the whole row means, so it is said in the
		// warning colour rather than listed as another attribute.
		rows = append(rows, detailRow{label: "Marker", value: e.Marker,
			warn: e.Marker == "@revoked"})
	}
	rows = append(rows, detailRow{label: "Line",
		value: fmt.Sprintf("%d of ~/.ssh/known_hosts", e.At)})

	key := []detailRow{
		{label: "Type", value: e.Type},
		{label: "Fingerprint", value: e.Fingerprint()},
	}
	if e.Comment != "" {
		key = append(key, detailRow{label: "Comment", value: e.Comment})
	}
	return []detailSection{{title: "Entry", rows: rows}, {title: "Key", rows: key}}
}

// ----------------------------------------------------------------- actions

func (m AppModel) openKnownDetail() (tea.Model, tea.Cmd) {
	e, at, ok := m.cursorKnown()
	if !ok {
		return m, m.toast.show("No host key selected", toastError)
	}
	return m, m.detail.show(detailPopup{
		title:    nameOr(knownHostLabel(e), "host key"),
		sections: knownDetail(e),
		prompt:   "Change which names this key is trusted for?",
		accept:   "edit",
		action:   detailEditKnown,
		at:       at,
	}, m.layer())
}

// openKnownEdit asks ONE question, so it is the input class rather than a form
// (§6.1). The key itself is not editable and never will be: retyping 68
// characters of base64 is not an edit, it is a new key — and that is what [A]
// is for, with the fingerprint shown before anything is trusted.
func (m AppModel) openKnownEdit() (tea.Model, tea.Cmd) {
	e, at, ok := m.cursorKnown()
	if !ok {
		return m, m.toast.show("No host key selected", toastError)
	}
	return m, m.askKnownHosts(e, at, m.layer())
}

func (m *AppModel) askKnownHosts(e store.KnownHostEntry, at, layer int) tea.Cmd {
	return m.input.ask(inputPopup{
		glyph:       glyphPencil,
		title:       "Trusted for",
		prompt:      "Which names does this key belong to?",
		value:       e.Hosts,
		accept:      "save",
		action:      inputKnownHosts,
		at:          at,
		placeholder: "one name, or several separated by commas",
	}, layer)
}

// doEditKnown is the offer at the foot of the detail float. Same rule as the
// other two panels: what replaces it is opened and the float closed in the same
// batch, rather than left standing underneath (§6.4).
func (m AppModel) doEditKnown(at int) (tea.Model, tea.Cmd) {
	e, ok := m.known.rowAt(at)
	if !ok {
		return m, m.detail.close()
	}
	return m, tea.Batch(m.detail.close(), m.askKnownHosts(e, at, m.layer()))
}

func (m AppModel) doRenameKnown(at int, hosts string) (tea.Model, tea.Cmd) {
	e, ok := m.known.rowAt(at)
	if !ok {
		return m, m.closeStack()
	}
	if strings.TrimSpace(hosts) == e.Hosts {
		return m, m.closeStack() // nothing to say and nothing to write
	}
	next, err := m.known.file.SetHosts(at, hosts)
	if err != nil {
		return m, tea.Batch(m.closeStack(), m.toast.show(err.Error(), toastError))
	}
	return m.persistKnown(next, at,
		fmt.Sprintf("~/.ssh/known_hosts: %q is now trusted for %q",
			e.Hosts, strings.TrimSpace(hosts)),
		"Saved")
}

func (m AppModel) askDeleteKnown() (tea.Model, tea.Cmd) {
	e, at, ok := m.cursorKnown()
	if !ok {
		return m, m.toast.show("No host key selected", toastError)
	}
	return m, m.confirm.ask(confirmPopup{
		glyph: glyphWarn,
		title: "Confirm",
		lines: []string{
			fmt.Sprintf("Stop trusting the key for %q?", knownHostLabel(e)),
			e.Fingerprint(),
			"The next connection there will ask again.",
		},
		accept: "delete",
		warn:   true,
		action: confirmDeleteKnown,
		at:     at,
	}, m.layer())
}

func (m AppModel) doDeleteKnown(at int) (tea.Model, tea.Cmd) {
	e, ok := m.known.rowAt(at)
	if !ok {
		return m, m.closeStack()
	}
	next, err := m.known.file.Delete(at)
	if err != nil {
		return m, tea.Batch(m.closeStack(), m.toast.show(err.Error(), toastError))
	}
	return m.persistKnown(next, -1,
		fmt.Sprintf("~/.ssh/known_hosts: key for %q removed (%s)", e.Hosts, e.Fingerprint()),
		fmt.Sprintf("Forgot %q", knownHostLabel(e)))
}

// persistKnown is persistSSHCfg's twin, and the reload-on-refusal matters even
// more here: ssh appends to this file every time it meets a host for the first
// time, so somebody else writing while sshu had it open is routine.
func (m AppModel) persistKnown(next store.KnownHostsFile, at int, logLine, toastLine string) (tea.Model, tea.Cmd) {
	saved, err := m.writeKnown(next)
	m.known.file = saved
	if at >= 0 {
		m.known.cursor = at
	}
	m.known.cursor = clamp(m.known.cursor, 0, max(0, len(saved.Entries)-1))
	m.known.ensureVisible()

	if err != nil {
		m.errors.warn("", "", err.Error())
		return m, tea.Batch(m.closeStack(), m.toast.show(err.Error(), toastError))
	}
	m.changes.add(logLine)
	return m, tea.Batch(m.closeStack(), m.toast.show(toastLine, toastInfo))
}

func (m AppModel) writeKnown(f store.KnownHostsFile) (store.KnownHostsFile, error) {
	if m.saveKnownHosts == nil {
		return f, nil // tests and dry runs
	}
	return m.saveKnownHosts(f)
}

// ----------------------------------------------------------- fetch and trust

// hostKeyScannedMsg carries the handshake's answer back onto the UI loop.
type hostKeyScannedMsg struct {
	host string
	port int
	key  remote.HostKey
	err  error
}

// knownScanTickMsg keeps the spinner moving while the handshake is out.
type knownScanTickMsg struct{}

func knownScanTick() tea.Cmd {
	return tea.Tick(dialTickEvery, func(time.Time) tea.Msg { return knownScanTickMsg{} })
}

func scanHostKeyCmd(host string, port int, timeout time.Duration) tea.Cmd {
	return func() tea.Msg {
		k, err := remote.ScanHostKey(host, port, timeout)
		return hostKeyScannedMsg{host: host, port: port, key: k, err: err}
	}
}

func (m AppModel) openKnownAdd() (tea.Model, tea.Cmd) {
	return m, m.knownAddUI.open(m.layer())
}

func (m AppModel) knownAddKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var res formResult
	m.knownAddUI, res = m.knownAddUI.update(msg)
	if res != formSubmit {
		return m, nil
	}
	host, port := m.knownAddUI.target()
	if port < 1 || port > 65535 {
		m.knownAddUI.fail("Port must be 1-65535", kfPort)
		return m, nil
	}
	m.knownAddUI.submitted = true
	m.knownAddUI.scanning, m.knownAddUI.spin = true, 0
	m.knownAddUI.err, m.knownAddUI.errIdx = "", -1
	return m, tea.Batch(scanHostKeyCmd(host, port, m.cfg.Timeout()), knownScanTick())
}

// hostKeyScanned is the moment the decision becomes the user's. sshu has the
// key and has written nothing: the fingerprint goes on screen and Enter is the
// only thing that trusts it — the same shape as ssh's own first-connect
// question, and for the same reason it is a question at all.
func (m AppModel) hostKeyScanned(msg hostKeyScannedMsg) (tea.Model, tea.Cmd) {
	if !m.knownAddUI.scanning {
		return m, nil // cancelled while it was out
	}
	if msg.err != nil {
		m.knownAddUI.fail(msg.err.Error(), -1)
		return m, nil
	}
	m.pendingKey = pendingHostKey{
		hosts: knownHostsField(msg.host, msg.port),
		typ:   msg.key.Type,
		key:   msg.key.Key,
	}
	return m, tea.Batch(m.knownAddUI.close(), m.confirm.ask(confirmPopup{
		glyph: glyphWarn,
		title: "Confirm",
		lines: []string{
			fmt.Sprintf("%s offered this key:", m.pendingKey.hosts),
			shortKeyType(msg.key.Type) + "  " + msg.key.Fingerprint,
			"Trust it? Check it against the server first.",
		},
		accept: "trust",
		warn:   true,
		action: confirmTrustHostKey,
	}, m.layer()))
}

// knownHostsField is how ssh writes an address in this file: bare on port 22,
// bracketed with the port on anything else. Getting this wrong writes a line
// ssh will never match.
func knownHostsField(host string, port int) string {
	if port == store.DefaultPort {
		return host
	}
	return "[" + host + "]:" + itoa(port)
}

// pendingHostKey is a key that has been fetched and not yet trusted. It
// outlives the form for the same reason pendingEdit outlives its popup: the
// question is asked after that box is gone.
type pendingHostKey struct {
	hosts string
	typ   string
	key   string
}

func (m AppModel) doTrustHostKey() (tea.Model, tea.Cmd) {
	p := m.pendingKey
	m.pendingKey = pendingHostKey{}
	if p.key == "" {
		return m, m.closeStack()
	}
	next, err := m.known.file.Add(store.KnownHostEntry{
		Hosts: p.hosts, Type: p.typ, Key: p.key,
	})
	if err != nil {
		return m, tea.Batch(m.closeStack(), m.toast.show(err.Error(), toastError))
	}
	fp := store.KnownHostEntry{Type: p.typ, Key: p.key}.Fingerprint()
	return m.persistKnown(next, len(next.Entries)-1,
		fmt.Sprintf("~/.ssh/known_hosts: trusted %s for %q", fp, p.hosts),
		fmt.Sprintf("Trusting %q", p.hosts))
}
