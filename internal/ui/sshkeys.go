package ui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/vulcanshen/sshu/internal/store"
)

// sshAction is one contextual action inside tab [3]. Same trick as hostActions:
// the table is the single declaration behind both the letter hotkey and the
// Space menu row, so §4.2 holds by construction. panel scopes an action to the
// panel it acts on.
type sshAction struct {
	key   string
	label string
	hint  string
	panel sshPanel
	// panelOp puts the action in the panel region of the Space menu: it is about
	// the tab, not about the session under the cursor (same split as tab [2]).
	panelOp bool
	run     func(AppModel) (tea.Model, tea.Cmd)
}

var sshActions = []sshAction{
	// item — the session under the cursor
	{key: "enter", label: "Open", hint: "Enter . show in [5]", panel: panelSessions, run: AppModel.openSession},
	{key: "C", label: "Close", hint: "end this session", panel: panelSessions, run: AppModel.askClose},
	{key: "D", label: "Duplicate", hint: "another to this host", panel: panelSessions, run: AppModel.askDuplicate},
}

// sshKey dispatches one key inside tab [3].
func (m AppModel) sshKey(k string) (tea.Model, tea.Cmd) {
	var keys []string
	var acts []sshAction
	for _, a := range sshActions {
		if a.panel == m.ssh.focus {
			keys, acts = append(keys, a.key), append(acts, a)
		}
	}
	if i := hotkeyIndex(keys, k); i >= 0 {
		return acts[i].run(m)
	}
	m.ssh.handleListKey(k)
	return m, nil
}

// cursorSession is the session under the [4] cursor.
func (m AppModel) cursorSession() *session {
	if m.ssh.focus == panelSessions && m.ssh.curSess < len(m.ssh.sessions) {
		return m.ssh.sessions[m.ssh.curSess]
	}
	return nil
}

// openSession is Enter on [4]. Moving the list cursor deliberately does NOT
// change what [5] shows — switching redraws the remote, and browsing the list
// should not do that — so this is the explicit act of switching.
//
// It asks nothing. Switching opens no connection and closes none: the session
// you leave keeps running, and Enter on the other row brings it straight back.
// A confirmation for something that costs nothing and undoes itself is just a
// keystroke in the way.
func (m AppModel) openSession() (tea.Model, tea.Cmd) {
	s := m.cursorSession()
	if s == nil {
		return m, nil
	}
	m.ssh.current, m.ssh.failed = s.id, nil
	m.ssh.setFocus(panelPty) // also re-applies the geometry for the new session
	return m, m.closeStack()
}

func (m AppModel) askClose() (tea.Model, tea.Cmd) {
	s := m.cursorSession()
	if s == nil {
		return m, nil
	}
	return m, m.confirm.ask(confirmPopup{
		glyph:  glyphWarn,
		title:  "Confirm",
		lines:  []string{fmt.Sprintf("Close the session to %s?", s.host.Name), "Anything running on the remote is killed."},
		accept: "close",
		warn:   true,
		action: confirmClose,
		target: itoa(s.id),
	}, m.layer())
}

// askDuplicate opens a second session to the host already under the cursor, so
// getting another shell on the same box does not mean a trip back to [1].
//
// It asks, like every other path that opens a connection. Switching between
// sessions is free and undoes itself, so Enter does not ask; opening one is not
// free — it authenticates, and the far end notices — so this does. One rule:
// opening a connection asks, moving between them does not.
func (m AppModel) askDuplicate() (tea.Model, tea.Cmd) {
	s := m.cursorSession()
	if s == nil {
		return m, nil
	}
	return m, m.confirm.ask(confirmPopup{
		glyph: glyphConnect,
		title: "Duplicate",
		lines: []string{
			fmt.Sprintf("Open another session to %s?", s.host.Name),
			s.host.Addr() + "  .  " + string(s.host.Auth),
		},
		accept: "open",
		action: confirmDuplicate,
		target: itoa(s.id),
	}, m.layer())
}

// startSession is the one place a connection is actually opened, so it is the
// one place a credential is resolved into the concrete user and auth.
func (m AppModel) startSession(h store.Host) (tea.Model, tea.Cmd) {
	rh, err := store.Resolve(h, m.creds.creds)
	if err != nil {
		m.log.errorf(err.Error())
		return m, tea.Batch(m.closeStack(), m.toast.show(err.Error(), toastError))
	}
	h = rh
	cmd := m.closeStack()
	m.tab = tabSSH
	m.sftp.onScreen = false // same rule switchTab keeps: hidden tabs do not poll
	m.ssh.setSize(m.w, m.panelHeight())
	m.log.info("connecting to " + h.Name + " · " + h.Addr())
	if _, err := m.ssh.connect(h); err != nil {
		return m, tea.Batch(cmd, m.toast.show(err.Error(), toastError))
	}
	// Straight into the remote: connecting is the whole point, and stopping on
	// the list first would just be a keystroke in the way (§7.1 context shift).
	m.ssh.setFocus(panelPty)
	return m, tea.Batch(cmd, m.ssh.tick())
}

// sshMenuItems is tab [3]'s §A.1 contents, in the same two regions tab [2] uses:
// what happens to the session under the cursor, and what is about the tab.
func (m AppModel) sshMenuItems() []menuItem {
	if m.ssh.focus == panelPty {
		return []menuItem{
			{label: "session", header: true},
			{label: "the remote has the keyboard", header: true},
			{separator: true},
			{label: "press alt+esc to come back", header: true},
		}
	}

	var item, panel []menuItem
	for _, a := range sshActions {
		if a.panel != m.ssh.focus {
			continue
		}
		row := menuItem{label: a.label, key: a.key, hint: a.hint}
		if a.panelOp {
			panel = append(panel, row)
			continue
		}
		// With no sessions there is nothing for a session action to be about.
		if len(m.ssh.sessions) > 0 {
			item = append(item, row)
		}
	}
	// One region stays flat — a header over a single group is noise (§6.2).
	if len(item) == 0 {
		return panel
	}

	out := []menuItem{{label: menuItemRegion, header: true}}
	out = append(out, item...)
	out = append(out, menuItem{separator: true},
		menuItem{label: menuPanelRegion, header: true})
	for _, a := range panel {
		out = append(out, a)
	}
	return out
}
