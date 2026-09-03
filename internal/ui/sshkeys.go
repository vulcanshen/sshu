package ui

import (
	"fmt"
	"strings"

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

// keyCloseAll is a menu-only action: it has no letter, and the key here exists
// only so the menu can dispatch it like every other row. Two things fall out of
// that on their own — bracketHotkey leaves a multi-character key unbracketed, so
// the row draws as plain text, and hotkeyIndex can never match it from a
// keystroke, because no keystroke is spelled "close-all".
//
// It is deliberate, not an oversight (§11.26): closing every session at once is
// destructive and rare, and a letter for it is a letter somebody's hand finds by
// accident on a list they were only scrolling. The menu is the slow path — open
// it, walk to the row, press Enter — and slow is the correct speed here.
const keyCloseAll = "close-all"

var sshActions = []sshAction{
	// item — the session under the cursor
	{key: "enter", label: "Open", hint: "Enter . show and take the keyboard", panel: panelSessions, run: AppModel.openSession},
	{key: "H", label: "Hide", hint: "hide/show this session's cell", panel: panelSessions, run: AppModel.toggleSessionDisplay},
	{key: "C", label: "Close", hint: "end this session", panel: panelSessions, run: AppModel.askClose},
	{key: "D", label: "Duplicate", hint: "another to this host", panel: panelSessions, run: AppModel.askDuplicate},

	// panel — the list as a whole
	{key: keyCloseAll, label: "Close all sessions", hint: "end every one of them", panel: panelSessions, panelOp: true, run: AppModel.askCloseAll},
}

// sshKey dispatches one key inside the ssh tab.
func (m AppModel) sshKey(k string) (tea.Model, tea.Cmd) {
	if m.ssh.focus == panelLayout {
		if m.ssh.layoutKey(k) {
			// Enter on custom: ask for the shape. Prefilled with the current
			// one, because most changes are one digit of it.
			return m, m.input.ask(inputPopup{
				title:  "Custom grid",
				glyph:  glyphGrid,
				prompt: "Columns for the grid, 1-9",
				value:  itoa(clamp(m.ssh.gridC, 1, 9)),
				accept: "apply",
				action: inputGridDims,
			}, m.layer())
		}
		return m, nil
	}
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

// applyGridDims parses the custom grid's "RxC" answer — rows first, the
// order the shape is read aloud in. Any two numbers 1-9 separated by anything
// count; a shape that is not that is refused with the example still on screen.
// applyGridDims takes ONE number, the column count. It used to take rows ×
// columns, and the rows half was very nearly a lie: gridDims returned
// max(rows, ceil(n/cols)), so asking for 2×3 and opening ten sessions gave a
// 2×5 grid. The stated row count only ever mattered when there were too FEW
// cells to fill it, where all it bought was reserved empty space. One number
// says the same thing without the arithmetic that contradicts it (§11.31).
func (m AppModel) applyGridDims(value string) (tea.Model, tea.Cmd) {
	c, ok := parseGridCols(value)
	if !ok {
		return m, tea.Batch(m.closeStack(), m.input.close(),
			m.toast.show("Columns must be a single number, 1-9", toastError))
	}
	m.ssh.gridC = c
	m.ssh.layout = layoutCustom
	m.ssh.applyGeometry()
	return m, tea.Batch(m.closeStack(), m.input.close(),
		m.toast.show("Grid set to "+plural(c, "column"), toastInfo))
}

// parseGridCols reads the one number the prompt asks for. Anything else in the
// box is a refusal rather than something to dig a digit out of: "2x3" is a
// user still answering the OLD question, and quietly taking the 2 would apply
// something they did not ask for.
func parseGridCols(s string) (cols int, ok bool) {
	s = strings.TrimSpace(s)
	if len(s) != 1 || s[0] < '1' || s[0] > '9' {
		return 0, false
	}
	return int(s[0] - '0'), true
}

// toggleSessionDisplay is [H]ide, and Tab is the same thing under the key this
// tab has always used for it.
//
// The LABEL is worded for the direction it almost always runs in — a session's
// cell goes onto the grid the moment it connects, so the thing you reach for is
// taking one off — and the HINT says it toggles. That split is the whole answer
// to a two-way action having a one-way name: the label is what you scan for, so
// it names the common case; the hint is what you read when you are not sure, so
// it tells the truth about both directions (§11.30).
func (m AppModel) toggleSessionDisplay() (tea.Model, tea.Cmd) {
	s := m.cursorSession()
	if s == nil {
		return m, nil
	}
	m.ssh.toggleShown(s.id)
	return m, m.closeStack()
}

// cursorSession is the session under the [4] cursor.
func (m AppModel) cursorSession() *session {
	if m.ssh.focus == panelSessions && m.ssh.curSess < len(m.ssh.sessions) {
		return m.ssh.sessions[m.ssh.curSess]
	}
	return nil
}

// openSession is Enter on [1]: the session's cell joins the grid (if it was
// not already there), the keyboard goes to it, and the side column folds.
//
// It asks nothing. Showing opens no connection and closes none: everything
// else keeps running, and Alt+Esc undoes the whole move. A confirmation for
// something that costs nothing and undoes itself is a keystroke in the way.
func (m AppModel) openSession() (tea.Model, tea.Cmd) {
	s := m.cursorSession()
	if s == nil {
		return m, nil
	}
	m.ssh.showAndFocus(s.id)
	return m, m.closeStack()
}

// askCloseAll is the one action on this tab that is about the whole list. It
// asks like Close does, and for a bigger reason — the count is in the question
// because "close all" reads differently when the number is 1 than when it is 9.
func (m AppModel) askCloseAll() (tea.Model, tea.Cmd) {
	n := m.ssh.liveCount()
	if n == 0 {
		return m, nil
	}
	return m, m.confirm.ask(confirmPopup{
		glyph:  glyphWarn,
		title:  "Confirm",
		lines:  []string{fmt.Sprintf("Close all %s?", plural(n, "session")), "Anything running on the remotes is killed."},
		accept: "close all",
		warn:   true,
		action: confirmCloseAll,
	}, m.layer())
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
//
// land says where the keyboard ends up, and every caller has to say it out loud.
// Connecting from the hosts table lands IN the session: that is what "connect"
// means there, and stopping on a list first would be a keystroke in the way
// (§7.1 context shift). Duplicating from [1] lands back on [1] — see §11.23:
// the Enter that ran it was an Enter on a CONFIRMATION, and a confirmation's
// Enter is not an Enter on the row underneath it.
func (m AppModel) startSession(h store.Host, land sshPanel) (tea.Model, tea.Cmd) {
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
	// connect() already put the new cell on the grid, pointed focusPty at it AND
	// moved the [1] cursor onto the new session — so landing on the list means
	// landing on the row that was just created, with its cell echoing on the
	// grid beside it.
	m.ssh.setFocus(land)
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
			{label: "alt+esc comes back · hold alt, arrows switch cells", header: true},
		}
	}
	if m.ssh.focus == panelLayout {
		return []menuItem{
			{label: "layout", header: true},
			{label: "j/k choose an arrangement — it applies as you move", header: true},
			{label: "Enter on custom asks for rows × columns", header: true},
		}
	}

	var item, panel []menuItem
	for _, a := range sshActions {
		if a.panel != m.ssh.focus {
			continue
		}
		// With no sessions there is nothing for any of these to be about — not
		// the row actions, and not "close all of them" either.
		if len(m.ssh.sessions) == 0 {
			continue
		}
		row := menuItem{label: a.label, key: a.key, hint: a.hint}
		if a.panelOp {
			panel = append(panel, row)
			continue
		}
		item = append(item, row)
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
