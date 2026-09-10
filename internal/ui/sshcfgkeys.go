package ui

import (
	"fmt"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/vulcanshen/sshu/internal/store"
)

// sshcfgAction mirrors hostAction and credAction: one table behind both the
// letter hotkey and the Space menu row, so §4.2 holds by construction on a
// third panel too.
type sshcfgAction struct {
	key        string
	label      string
	hint       string
	needsBlock bool
	panelOp    bool
	run        func(AppModel) (tea.Model, tea.Cmd)
}

var sshcfgActions = []sshcfgAction{
	// item — the Host block under the cursor
	{key: "enter", label: "View", hint: "Enter . every option this block sets", needsBlock: true, run: AppModel.openSSHCfgDetail},
	{key: "E", label: "Edit", hint: "change this block", needsBlock: true, run: AppModel.openSSHCfgEdit},
	{key: "D", label: "Duplicate", hint: "a new block starting from this one", needsBlock: true, run: AppModel.openSSHCfgDuplicate},
	{key: "X", label: "Delete", hint: "remove from ~/.ssh/config", needsBlock: true, run: AppModel.askDeleteSSHCfg},

	// panel — the table
	{key: "A", label: "Add", hint: "a new Host block", panelOp: true, run: AppModel.openSSHCfgCreate},
}

func (m AppModel) sshcfgApplicable() ([]string, []sshcfgAction) {
	_, _, has := m.cursorSSHCfg()
	var keys []string
	var acts []sshcfgAction
	for _, a := range sshcfgActions {
		if a.needsBlock && !has {
			continue
		}
		keys, acts = append(keys, a.key), append(acts, a)
	}
	return keys, acts
}

func (m AppModel) sshcfgKey(k string) (tea.Model, tea.Cmd) {
	keys, acts := m.sshcfgApplicable()
	if i := hotkeyIndex(keys, k); i >= 0 {
		return acts[i].run(m)
	}
	m.sshcfg.handleKey(k)
	return m, nil
}

// cursorSSHCfg is the block under the cursor AND its position. The position is
// the identity here: two blocks may carry the same pattern and ssh means
// something by that (§11.39), so there is no name to look one up by.
func (m AppModel) cursorSSHCfg() (store.SSHBlock, int, bool) {
	if m.tab != tabPref || m.pref.item != prefSSHConfig {
		return store.SSHBlock{}, -1, false
	}
	b, ok := m.sshcfg.rowAt(m.sshcfg.cursor)
	return b, m.sshcfg.cursor, ok
}

func (m AppModel) sshcfgMenuItems() []menuItem {
	_, acts := m.sshcfgApplicable()
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

// ----------------------------------------------------------------- detail

// sshcfgDetail is what the table cannot show: every option line, including the
// ones with no column, and where in the file the block lives — because the
// answer to "why is this host behaving oddly" is usually a keyword three rows
// down that sshu has no opinion about.
func sshcfgDetail(b store.SSHBlock, file string) []detailSection {
	from, to := b.LineRange()
	// The FILE is the section's heading, in the same shape a host's detail uses
	// (§11.42): with Include followed this block may live somewhere else
	// entirely, and "which file, which lines" is exactly what somebody about to
	// hand-edit it needs. It used to be buried in the Lines value — true, but
	// said in the one place nobody reads first.
	out := []detailSection{{title: glyphFileCog + " " + file, rows: []detailRow{
		{label: "Host", value: b.Patterns},
		{label: "Lines", value: fmt.Sprintf("%d-%d", from, to)},
	}}}

	opts := detailSection{title: "Options"}
	for _, o := range b.Options {
		opts.rows = append(opts.rows, detailRow{label: o.Key, value: o.Value})
	}
	if len(opts.rows) == 0 {
		opts.rows = []detailRow{{label: "none", value: "this block sets nothing"}}
	}
	return append(out, opts)
}

// ---------------------------------------------------------------- actions

func (m AppModel) openSSHCfgDetail() (tea.Model, tea.Cmd) {
	b, at, ok := m.cursorSSHCfg()
	if !ok {
		return m, m.toast.show("No Host block selected", toastError)
	}
	return m, m.detail.show(detailPopup{
		title:    nameOr(b.Patterns, "Host block"),
		sections: sshcfgDetail(b, m.sshcfg.file.Path(b.File())),
		prompt:   fmt.Sprintf("Edit %q?", b.Patterns),
		accept:   "edit",
		action:   detailEditSSHCfg,
		at:       at,
	}, m.layer())
}

func (m AppModel) openSSHCfgCreate() (tea.Model, tea.Cmd) {
	return m, m.sshcfgFormUI.openCreate(m.layer())
}

func (m AppModel) openSSHCfgEdit() (tea.Model, tea.Cmd) {
	b, at, ok := m.cursorSSHCfg()
	if !ok {
		return m, m.toast.show("No Host block selected", toastError)
	}
	return m, m.sshcfgFormUI.openEdit(b, at, m.layer())
}

// doEditSSHCfg is the offer at the foot of a block's detail float. The form
// REPLACES the float for the same reason the credential one does: they are two
// views of one row, and leaving the detail underneath would mean Esc out of the
// form lands on a copy of what the form just changed (§6.4, §11.29).
func (m AppModel) doEditSSHCfg(at int) (tea.Model, tea.Cmd) {
	b, ok := m.sshcfg.rowAt(at)
	if !ok {
		return m, m.detail.close()
	}
	return m, tea.Batch(m.detail.close(), m.sshcfgFormUI.openEdit(b, at, m.layer()))
}

func (m AppModel) openSSHCfgDuplicate() (tea.Model, tea.Cmd) {
	b, _, ok := m.cursorSSHCfg()
	if !ok {
		return m, m.toast.show("No Host block selected", toastError)
	}
	return m, m.sshcfgFormUI.openDuplicate(b, m.layer())
}

func (m AppModel) askDeleteSSHCfg() (tea.Model, tea.Cmd) {
	b, at, ok := m.cursorSSHCfg()
	if !ok {
		return m, m.toast.show("No Host block selected", toastError)
	}
	// The FILE by name. With Include followed, `[X]` can rewrite a file the user
	// did not open and may not remember owning — a confirmation that said
	// "~/.ssh/config" while touching config.d/work would be lying at the exact
	// moment it matters.
	lines := []string{
		fmt.Sprintf("Delete Host %q?", b.Patterns),
		"This rewrites " + m.sshcfg.file.Path(b.File()) + ".",
	}
	// The option count is the part the table understates: a `Host *` row shows
	// two empty columns and is carrying everything.
	if n := len(b.Options); n > 0 {
		verb := "go"
		if n == 1 {
			verb = "goes"
		}
		lines = append(lines, plural(n, "option")+" "+verb+" with it.")
	}
	// And who downstream notices. Deleting a block that supplies a HostName
	// breaks every sshu host whose `host` field is the name it defines — the
	// same disclosure deleting a credential makes, for the same reason: it
	// breaks quietly, at connect time, somewhere else.
	if matched, resolving := m.hostsThroughBlock(at); resolving > 0 {
		verb := "resolve"
		if resolving == 1 {
			verb = "resolves"
		}
		lines = append(lines, fmt.Sprintf("%s %s through it and will not connect.",
			plural(resolving, "sshu host"), verb))
	} else if matched > 0 {
		verb := "inherit"
		if matched == 1 {
			verb = "inherits"
		}
		lines = append(lines, fmt.Sprintf("%s %s options from it.",
			plural(matched, "sshu host"), verb))
	}
	return m, m.confirm.ask(confirmPopup{
		glyph:  glyphWarn,
		title:  "Confirm",
		lines:  lines,
		accept: "delete",
		warn:   true,
		action: confirmDeleteSSHCfg,
		at:     at,
	}, m.layer())
}

// hostsThroughBlock counts the sshu hosts this block has a word in, and how
// many of them take their HostName from it.
//
// The second number is the severe one: a host whose `host` field is a name this
// block DEFINES stops resolving the moment the block goes. The first is the
// milder "it also loses whatever else this block was contributing", which is
// what `Host *` produces for every host in the list.
//
// Credentials are deliberately not in this count. A credential is sshu's own
// way of storing a user and an auth method; nothing in it ever reaches
// ~/.ssh/config, so it has nothing to say about a Host block going away.
func (m AppModel) hostsThroughBlock(at int) (matched, resolving int) {
	b, ok := m.sshcfg.rowAt(at)
	if !ok {
		return 0, 0
	}
	for _, h := range m.hosts.hosts {
		if !b.Matches(h.Host) {
			continue
		}
		matched++
		for _, o := range m.sshcfg.file.Effective(h.Host) {
			if strings.EqualFold(o.Key, "HostName") {
				if o.Block == at {
					resolving++
				}
				break
			}
		}
	}
	return matched, resolving
}

func (m AppModel) doDeleteSSHCfg(at int) (tea.Model, tea.Cmd) {
	b, ok := m.sshcfg.rowAt(at)
	if !ok {
		return m, m.closeStack()
	}
	next, err := m.sshcfg.file.Delete(at)
	if err != nil {
		return m, tea.Batch(m.closeStack(), m.toast.show(err.Error(), toastError))
	}
	return m.persistSSHCfg(next, -1,
		fmt.Sprintf("~/.ssh/config: Host %q deleted", b.Patterns),
		fmt.Sprintf("Deleted %q", b.Patterns))
}

// persistSSHCfg writes the file and puts whatever is now on disk on screen.
//
// That last part is the point of the return value: this file has other editors,
// so a save can be REFUSED because somebody else got there first, and the
// refusal hands back their version. Adopting it means the panel is showing the
// truth by the time the message about it lands — the alternative is a list that
// still shows the change next to a toast saying it did not happen.
//
// at is where to leave the cursor, or -1 to leave it where it was.
func (m AppModel) persistSSHCfg(next store.SSHConfigFile, at int, logLine, toastLine string) (tea.Model, tea.Cmd) {
	saved, err := m.writeSSHCfg(next)
	m.sshcfg.file = saved
	if at >= 0 {
		m.sshcfg.cursor = at
	}
	m.sshcfg.cursor = clamp(m.sshcfg.cursor, 0, max(0, len(saved.Blocks)-1))
	m.sshcfg.ensureVisible()

	if err != nil {
		m.errors.warn("", "", err.Error())
		return m, tea.Batch(m.closeStack(), m.toast.show(err.Error(), toastError))
	}
	m.activity.add(logLine)
	return m, tea.Batch(m.closeStack(), m.toast.show(toastLine, toastInfo))
}

func (m AppModel) writeSSHCfg(f store.SSHConfigFile) (store.SSHConfigFile, error) {
	if m.saveSSHConfig == nil {
		return f, nil // tests and dry runs
	}
	return m.saveSSHConfig(f)
}

// -------------------------------------------------------------------- form

func (m AppModel) sshcfgFormKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var res formResult
	m.sshcfgFormUI, res = m.sshcfgFormUI.update(msg)
	switch res {
	case formSubmit:
		return m.commitSSHCfgForm()
	case formBrowse:
		return m, m.picker.open(identityRoot(), m.sshcfgFormUI.layer+1)
	}
	m.syncSSHCfgFormError()
	return m, nil
}

func (m *AppModel) syncSSHCfgFormError() {
	if !m.sshcfgFormUI.submitted {
		return
	}
	m.sshcfgFormUI.refreshError(m.validateSSHCfgForm())
}

// validateSSHCfgForm has far less to say than its siblings, and that is the
// file's doing: ssh validates its own keywords, and sshu refusing a value it
// merely does not recognise would make this panel worse than an editor. The two
// checks left are the ones that would produce a block ssh cannot read at all.
func (m AppModel) validateSSHCfgForm() (string, int) {
	f := m.sshcfgFormUI
	if strings.TrimSpace(f.fields[sfHost].value) == "" {
		return "Host pattern is required", sfHost
	}
	if p := strings.TrimSpace(f.fields[sfPort].value); p != "" {
		if n, err := strconv.Atoi(p); err != nil || n < 1 || n > 65535 {
			return "Port must be 1-65535", sfPort
		}
	}
	return "", -1
}

func (m AppModel) commitSSHCfgForm() (tea.Model, tea.Cmd) {
	m.sshcfgFormUI.submitted = true
	if msg, field := m.validateSSHCfgForm(); msg != "" {
		// Said once, in the form, as the credential form learned to (§11.34).
		m.sshcfgFormUI.fail(msg, field)
		return m, nil
	}

	b := m.sshcfgFormUI.block()
	next, err := m.sshcfg.file, error(nil)
	verb, focus := "added", -1

	if at := m.sshcfgFormUI.editing; at >= 0 {
		if at >= len(m.sshcfg.file.Blocks) {
			m.sshcfgFormUI.fail("This block is gone — the file changed underneath", -1)
			return m, nil
		}
		next, err = m.sshcfg.file.Set(at, b)
		verb, focus = "updated", at
	} else {
		next, err = m.sshcfg.file.Add(b)
		focus = len(next.Blocks) - 1
	}
	if err != nil {
		m.sshcfgFormUI.fail(err.Error(), -1)
		return m, nil
	}

	return m.persistSSHCfg(next, focus,
		fmt.Sprintf("~/.ssh/config: Host %q %s", b.Patterns, verb),
		fmt.Sprintf("Saved %q", b.Patterns))
}
