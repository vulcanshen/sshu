package ui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/vulcanshen/sshu/internal/store"
)

// credAction mirrors hostAction: one table behind both the letter hotkey and
// the Space menu row, so §4.2 holds by construction here too.
type credAction struct {
	key       string
	label     string
	hint      string
	needsCred bool
	panelOp   bool
	run       func(AppModel) (tea.Model, tea.Cmd)
}

var credActions = []credAction{
	// item — the credential under the cursor
	{key: "enter", label: "View", hint: "Enter . how this one authenticates", needsCred: true, run: AppModel.openCredDetail},
	{key: "E", label: "Edit", hint: "change this credential", needsCred: true, run: AppModel.openCredEdit},
	{key: "D", label: "Duplicate", hint: "a new credential starting from this one", needsCred: true, run: AppModel.openCredDuplicate},
	{key: "X", label: "Delete", hint: "remove from credentials.yaml", needsCred: true, run: AppModel.askDeleteCred},

	// panel — the table
	{key: "A", label: "Add", hint: "a new credential", panelOp: true, run: AppModel.openCredCreate},
}

func (m AppModel) credsApplicable() ([]string, []credAction) {
	_, hasCred := m.cursorCred()
	var keys []string
	var acts []credAction
	for _, a := range credActions {
		if a.needsCred && !hasCred {
			continue
		}
		keys, acts = append(keys, a.key), append(acts, a)
	}
	return keys, acts
}

// credsKey dispatches one key on the credentials panel. Enter is a row of its
// own now rather than a synonym for [E]dit: it opens the read-only detail, from
// whose foot Enter goes on to the form (§11.29). E still goes straight there,
// so the shortcut is not lost — what changed is that "look at this" no longer
// has to be a form you might type into.
func (m AppModel) credsKey(k string) (tea.Model, tea.Cmd) {
	keys, acts := m.credsApplicable()
	if i := hotkeyIndex(keys, k); i >= 0 {
		return acts[i].run(m)
	}
	m.creds.handleKey(k)
	return m, nil
}

func (m AppModel) cursorCred() (store.Credential, bool) {
	if m.tab != tabPref || m.pref.item != prefCreds {
		return store.Credential{}, false
	}
	return m.creds.rowAt(m.creds.cursor)
}

func (m AppModel) credsMenuItems() []menuItem {
	_, acts := m.credsApplicable()
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

// ---------------------------------------------------------------- actions

func (m AppModel) openCredCreate() (tea.Model, tea.Cmd) {
	return m, m.credFormUI.openCreate(m.layer())
}

func (m AppModel) openCredEdit() (tea.Model, tea.Cmd) {
	c, ok := m.cursorCred()
	if !ok {
		return m, m.toast.show("No credential selected", toastError)
	}
	return m, m.credFormUI.openEdit(c, m.layer())
}

// doEditCred is the offer at the foot of a credential's detail float. It looks
// the credential up by NAME rather than re-reading the cursor, for the same
// reason doConnect does: the float carries what it was opened on, and a second
// source of truth is a chance for the two to disagree.
//
// The form REPLACES the float rather than stacking on it. They are two views of
// one row, not a target opened from a source (§6.4) — leaving the detail
// underneath would mean Esc out of the form lands back on a copy of the row the
// form may just have changed.
func (m AppModel) doEditCred(name string) (tea.Model, tea.Cmd) {
	for _, c := range m.creds.creds {
		if c.Name == name {
			return m, tea.Batch(m.detail.close(), m.credFormUI.openEdit(c, m.layer()))
		}
	}
	return m, m.detail.close()
}

// openCredDuplicate opens a CREATE form holding everything this credential
// holds. Same shape as the host one, and the same reason (§11.35).
func (m AppModel) openCredDuplicate() (tea.Model, tea.Cmd) {
	c, ok := m.cursorCred()
	if !ok {
		return m, m.toast.show("No credential selected", toastError)
	}
	return m, m.credFormUI.openDuplicate(c, m.layer())
}

// hostsUsing counts the hosts that name this credential — the number a delete
// or a rename has to disclose, because those hosts break quietly otherwise.
func (m AppModel) hostsUsing(name string) int {
	n := 0
	for _, h := range m.hosts.hosts {
		if h.Auth == store.AuthCredential && h.Credential == name {
			n++
		}
	}
	return n
}

func (m AppModel) askDeleteCred() (tea.Model, tea.Cmd) {
	c, ok := m.cursorCred()
	if !ok {
		return m, m.toast.show("No credential selected", toastError)
	}
	lines := []string{
		fmt.Sprintf("Delete credential %q?", c.Name),
		"This rewrites credentials.yaml.",
	}
	if n := m.hostsUsing(c.Name); n > 0 {
		verb := "reference"
		if n == 1 {
			verb = "references"
		}
		lines = append(lines, fmt.Sprintf("%s %s it and will fail to connect.",
			plural(n, "host"), verb))
	}
	return m, m.confirm.ask(confirmPopup{
		glyph:  glyphWarn,
		title:  "Confirm",
		lines:  lines,
		accept: "delete",
		warn:   true,
		action: confirmDeleteCred,
		target: c.Name,
	}, m.layer())
}

func (m AppModel) doDeleteCred(name string) (tea.Model, tea.Cmd) {
	creds := make([]store.Credential, 0, len(m.creds.creds))
	for _, c := range m.creds.creds {
		if c.Name != name {
			creds = append(creds, c)
		}
	}
	if err := m.persistCreds(creds); err != nil {
		return m, tea.Batch(m.closeStack(), m.toast.show(err.Error(), toastError))
	}
	m.creds.creds = creds
	m.hosts.creds = creds
	m.creds.cursor = min(m.creds.cursor, max(0, len(creds)-1))
	m.creds.ensureVisible()
	m.changes.add(fmt.Sprintf("credential %q deleted", name))
	return m, tea.Batch(m.closeStack(),
		m.toast.show(fmt.Sprintf("Deleted %q", name), toastInfo))
}

// ------------------------------------------------------------------- form

func (m AppModel) credFormKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var res formResult
	m.credFormUI, res = m.credFormUI.update(msg)
	switch res {
	case formSubmit:
		return m.commitCredForm()
	case formBrowse:
		return m, m.picker.open(identityRoot(), m.credFormUI.layer+1)
	}
	m.syncCredFormError()
	return m, nil
}

func (m *AppModel) syncCredFormError() {
	if !m.credFormUI.submitted {
		return
	}
	m.credFormUI.refreshError(m.validateCredForm())
}

func (m AppModel) validateCredForm() (string, int) {
	c := m.credFormUI.credential()
	switch {
	case c.Name == "":
		return "Name is required", cName
	case c.User == "":
		return "User is required", cUser
	}
	for _, other := range m.creds.creds {
		if other.Name == c.Name && c.Name != m.credFormUI.editing {
			return fmt.Sprintf("A credential named %q already exists", c.Name), cName
		}
	}
	return "", -1
}

func (m AppModel) commitCredForm() (tea.Model, tea.Cmd) {
	m.credFormUI.submitted = true
	if msg, field := m.validateCredForm(); msg != "" {
		// Said ONCE, in the form. The error row names the field, marks it red,
		// and moves the cursor to it — a toast on top of that is the same
		// sentence a second time, floating somewhere else, over a popup that is
		// already showing it (§6.7: fix it in place, do not stack).
		//
		// It used to do both, and the host form never did — so the two sibling
		// forms refused differently, which is the part that was actually wrong.
		m.credFormUI.fail(msg, field)
		return m, nil
	}

	c := m.credFormUI.credential()
	creds := append([]store.Credential(nil), m.creds.creds...)
	verb := "added"
	if m.credFormUI.editing != "" {
		i := -1
		for j := range creds {
			if creds[j].Name == m.credFormUI.editing {
				i = j
				break
			}
		}
		if i < 0 {
			m.credFormUI.fail("This credential is gone — it was removed elsewhere", -1)
			return m, nil
		}
		creds[i], verb = c, "updated"
	} else {
		creds = append(creds, c)
	}

	if err := m.persistCreds(creds); err != nil {
		m.credFormUI.fail(err.Error(), -1)
		return m, nil
	}

	m.creds.creds = creds
	m.hosts.creds = creds
	for i := range creds {
		if creds[i].Name == c.Name {
			m.creds.cursor = i
		}
	}
	m.creds.ensureVisible()
	m.changes.add(fmt.Sprintf("credential %q %s (%s, %s)", c.Name, verb, c.User, c.Auth))

	cmds := []tea.Cmd{m.closeStack(),
		m.toast.show(fmt.Sprintf("Saved %q", c.Name), toastInfo)}
	// A RENAME leaves every referring host pointing at the old name, and they
	// break quietly at connect time. Said here, where the rename happened.
	if old := m.credFormUI.editing; old != "" && old != c.Name {
		if n := m.hostsUsing(old); n > 0 {
			warnLine := fmt.Sprintf("%s still reference credential %q", plural(n, "host"), old)
			m.errors.warn("", "", warnLine)
			cmds[1] = m.toast.show(warnLine, toastError)
		}
	}
	return m, tea.Batch(cmds...)
}

func (m AppModel) persistCreds(creds []store.Credential) error {
	if m.saveCreds == nil {
		return nil // tests and dry runs
	}
	return m.saveCreds(creds)
}
