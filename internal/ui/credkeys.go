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
	{key: "enter", label: "Edit", hint: "Enter . change this credential", needsCred: true, run: AppModel.openCredEdit},
	{key: "D", label: "Delete", hint: "remove from credentials.yaml", needsCred: true, run: AppModel.askDeleteCred},

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
	m.creds.cursor = min(m.creds.cursor, max(0, len(creds)-1))
	m.creds.ensureVisible()
	m.log.info(fmt.Sprintf("credential %q deleted", name))
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
	for i := range creds {
		if creds[i].Name == c.Name {
			m.creds.cursor = i
		}
	}
	m.creds.ensureVisible()
	m.log.info(fmt.Sprintf("credential %q %s (%s, %s)", c.Name, verb, c.User, c.Auth))

	cmds := []tea.Cmd{m.closeStack(),
		m.toast.show(fmt.Sprintf("Saved %q", c.Name), toastInfo)}
	// A RENAME leaves every referring host pointing at the old name, and they
	// break quietly at connect time. Said here, where the rename happened.
	if old := m.credFormUI.editing; old != "" && old != c.Name {
		if n := m.hostsUsing(old); n > 0 {
			warnLine := fmt.Sprintf("%s still reference credential %q", plural(n, "host"), old)
			m.log.warn(warnLine)
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
