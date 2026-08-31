package ui

import (
	"fmt"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/vulcanshen/sshu/internal/store"
)

// The three top-level surfaces. Screen order is hosts / sftp / ssh (that is what
// the capsules read left to right); build order is hosts → ssh → sftp.
type tabID int

const (
	tabHosts tabID = iota
	tabSFTP
	tabSSH
	tabCount
)

// Capsule labels. "[N]" is doing double duty as the type signal and as the
// hotkey disclosure — one bracket convention for the whole app (§3.4 / §4.4).
var tabLabels = []string{"[1] hosts", "[2] sftp", "[3] ssh"}

// chromeRows is the fixed chrome the panels do NOT get: the capsule row, the
// rule under it, and the footer. Locked at 3 — none of them ever reflows (§1.3).
const chromeRows = 3

// SaveFunc persists the host list. Injected so the UI never reaches for the
// filesystem itself, and so tests can watch what would have been written.
type SaveFunc func([]store.Host) error

type AppModel struct {
	w, h      int
	tab       tabID
	hosts     hostsModel
	ssh       sshModel
	sftp      sftpModel
	transfers transferModel
	pending   pendingTransfer
	save      SaveFunc

	// Floats. At most one of form / confirm / help is up at a time, optionally
	// over the Space menu; the toast rides on top of everything.
	spaceMenu   spaceMenu
	hostPicker  spaceMenu
	transfersUI transfersPopup
	historyUI   historyPopup
	help        helpPopup
	form        hostForm
	picker      filePicker
	confirm     confirmPopup
	input       inputPopup
	toast       toastModel

	// pendingG holds the first half of the gg chord. A chord is a shortcut for an
	// action that already exists, so it costs no core-key slot (§A.0.Y).
	pendingG bool
}

func New(hosts []store.Host, save SaveFunc) AppModel {
	return AppModel{
		hosts:       hostsModel{hosts: hosts},
		ssh:         newSSHModel(),
		sftp:        newSFTPModel(),
		transfersUI: newTransfersPopup(),
		historyUI:   newHistoryPopup(),
		hostPicker:  newHostPicker(),
		save:        save,
		spaceMenu:   newSpaceMenu(),
		help:        newHelpPopup(),
		form:        newHostForm(),
		picker:      newFilePicker(),
		confirm:     newConfirmPopup(),
		input:       newInputPopup(),
		toast:       newToast(),
	}
}

func (m AppModel) Init() tea.Cmd { return nil }

func (m AppModel) panelHeight() int { return m.h - chromeRows }

// layer is the depth the next float opens at: 1 normally, 2 when it is being
// opened from the Space menu, which stays behind it (§6.4).
func (m AppModel) layer() int {
	if m.spaceMenu.isActive() {
		return 2
	}
	return 1
}

func (m AppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		m.hosts.setSize(m.w, m.panelHeight())
		m.ssh.setSize(m.w, m.panelHeight())
		m.sftp.setSize(m.w, m.panelHeight())
		m.hostPicker.setSize(m.w, m.h)
		m.transfersUI.setSize(m.w, m.h)
		m.historyUI.setSize(m.w, m.h)
		m.spaceMenu.setSize(m.w, m.h)
		m.help.setSize(m.w, m.h)
		m.form.setSize(m.w, m.h)
		m.picker.setSize(m.w, m.h)
		m.confirm.setSize(m.w, m.h)
		m.input.setSize(m.w, m.h)
		m.toast.setSize(m.w, m.h)
		return m, nil

	case AnimTickMsg:
		// Fan the tick at every animator; each ignores a target that is not its
		// own, so two popups animating at once cannot eat each other's ticks.
		return m, tea.Batch(
			m.spaceMenu.anim.tick(msg),
			m.hostPicker.anim.tick(msg),
			m.transfersUI.anim.tick(msg),
			m.historyUI.anim.tick(msg),
			m.help.anim.tick(msg),
			m.form.anim.tick(msg),
			m.picker.anim.tick(msg),
			m.confirm.anim.tick(msg),
			m.input.anim.tick(msg),
			m.toast.anim.tick(msg),
		)

	case sshTickMsg:
		// One tick drives both jobs: reap what has finished and repaint what is
		// still drawing. It only runs while something is live, so an idle sshu
		// costs nothing.
		ended := m.ssh.reap()
		if len(ended) > 0 {
			m.ssh.setSize(m.w, m.panelHeight())
		}
		// A session dying used to be completely silent. Say so, at the moment it
		// happens — that is the whole of what the history panel was for.
		if msg := endedBadly(ended); msg != "" {
			return m, tea.Batch(m.ssh.tick(), m.toast.show(msg, toastError))
		}
		return m, m.ssh.tick()

	case sftpConnectedMsg:
		return m.sftpConnected(msg)

	case xferTickMsg:
		return m, m.transfers.tick()

	case watchTickMsg:
		return m, m.sftp.onWatchTick(msg)

	case watchResultMsg:
		m.sftp.onWatchResult(msg)
		return m, nil

	case scanTickMsg:
		// A search walk reports the same way a transfer does: the goroutine piles
		// results up, the tick is what puts them on screen, and it stops itself
		// when nothing is walking.
		return m, m.sftp.takeScans()

	case xferDoneMsg:
		// Re-list the destination so what just arrived is on screen without the
		// user having to leave and come back.
		for i := range m.sftp.sides {
			if s := &m.sftp.sides[i]; s.fs != nil && s.cwd != "" {
				s.reload()
			}
		}
		return m, m.transfers.tick()

	case toastExpireMsg:
		return m, m.toast.expire(msg)

	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m AppModel) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Alt+Esc is sshu's own key, not a VTP core key: it exists because panel [5]
	// hands the keyboard to a remote program, and something has to be able to
	// take it back. It is scoped to that one situation — everywhere else it is
	// just Esc, so it is never a dead key. Disclosed in the footer whenever the
	// PTY holds focus, which is the only place it means anything.
	if msg.Type == tea.KeyEscape && msg.Alt {
		if m.inPty() {
			m.ssh.setFocus(panelSessions)
			return m, nil
		}
		return m.closeTop()
	}

	// Esc is one role and it is resolved in exactly one place: close the topmost
	// visible float (§4.3). No popup re-implements cancel.
	if msg.Type == tea.KeyEscape && !m.popupOpen() && m.inPty() {
		m.ssh.currentSession().pty.write(msg) // a bare Esc belongs to the remote
		return m, nil
	}
	if msg.Type == tea.KeyEscape {
		// filu's two-stage Esc: close a float if one is up, otherwise go up a
		// directory. Only the sftp browser has an "up" to go to.
		if !m.popupOpen() && m.tab == tabSFTP && !m.sftp.focus.isMarks() {
			// A filter is the innermost thing Esc can drop, before the directory.
			if s := m.sftp.cur(); s.filtering {
				s.clearFilter()
				return m, nil
			}
			m.sftp.cur().up()
			return m, nil
		}
		return m.closeTop()
	}
	// Ctrl+C is the emergency exit, not a cancel — it works even under a popup,
	// and it takes the sessions with it: an orphaned ssh holding a PTY nobody
	// owns is worse than a slow exit.
	if msg.Type == tea.KeyCtrlC {
		return m.quit()
	}

	// A focused PTY swallows everything else — that is what focusing it means.
	if m.inPty() {
		m.ssh.currentSession().pty.write(msg)
		return m, nil
	}

	// Space and ? close what they open (§A.1 / §A.2). An entry key that only
	// works one way is a trap: the user reaches for the same key to get out,
	// nothing happens, and the surface looks stuck.
	//
	// Resolved here rather than inside each popup, for the same reason Esc is
	// (§4.3): one role, one place, and no float can be the one that forgot. The
	// exception is a float being TYPED INTO — there a space is a space and a
	// question mark is a question mark (§4.5).
	if msg.Type == tea.KeySpace && m.popupOpen() && !m.textFloat() {
		return m.closeTop()
	}
	if msg.String() == "?" && !m.textFloat() && !m.inPty() {
		if m.help.anim.owns() {
			return m, m.help.close()
		}
		// It opens from ON TOP of another float too. §A.2 promises the help is
		// reachable from any surface, and the surface a lost user is most likely
		// to be standing on is the menu they just opened.
		return m, m.help.open(m.layer())
	}

	// A filtering file list claims printable keys before the action table can:
	// while a query is being typed, "m" is a letter, not Mark. Arrows, Enter and
	// Esc fall through — the same split the picker and the form make (§4.5).
	if m.tab == tabSFTP && !m.popupOpen() && m.sftp.cur().filterKey(msg) {
		return m, nil
	}

	// Same rule as popupOpen: a float that is closing has already handed the
	// keyboard back, so it is not offered the key.
	switch {
	case m.transfersUI.anim.owns():
		if i := m.transfersUI.update(msg, len(m.transfers.jobs)); i >= 0 {
			m.transfers.cancelJob(i)
		}
		return m, nil
	case m.historyUI.anim.owns():
		m.historyUI.update(msg, len(m.ssh.history))
		return m, nil
	case m.hostPicker.anim.owns():
		return m.hostPickerKey(msg)
	case m.picker.anim.owns():
		return m.pickerKey(msg)
	case m.form.anim.owns():
		return m.formKey(msg)
	case m.input.anim.owns():
		return m.inputKey(msg)
	case m.confirm.anim.owns():
		return m.confirmKey(msg)
	case m.help.anim.owns():
		m.help.update(msg)
		return m, nil
	case m.spaceMenu.anim.owns():
		return m.menuKey(msg)
	}
	return m.panelKey(msg)
}

// inputKey feeds the text box and dispatches its answer.
func (m AppModel) inputKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	value, done := m.input.update(msg)
	if !done {
		return m, nil
	}
	switch m.input.action {
	case inputRename:
		return m.doRename(m.input.subject, value)
	case inputNewDir:
		return m.doNewDir(value)
	}
	return m, tea.Batch(m.closeStack(), m.input.close())
}

// closeTop pops one level off the float stack. Cancelling out of a target
// leaves its source standing (§6.4) — Esc on a form opened from the Space menu
// lands back on the menu.
func (m AppModel) closeTop() (tea.Model, tea.Cmd) {
	switch {
	case m.toast.isActive():
		return m, m.toast.close()
	case m.transfersUI.isActive():
		return m, m.transfersUI.close()
	case m.historyUI.isActive():
		return m, m.historyUI.close()
	case m.hostPicker.isActive():
		return m, m.hostPicker.close()
	case m.picker.isActive():
		return m, m.picker.close()
	case m.form.isActive():
		return m, m.form.close()
	case m.input.isActive():
		return m, m.input.close()
	case m.confirm.isActive():
		return m, m.confirm.close()
	case m.help.isActive():
		return m, m.help.close()
	case m.spaceMenu.isActive():
		return m, m.spaceMenu.close()
	}
	return m, nil
}

// closeStack tears the whole stack down. Committing an action is the end of the
// errand, so the user is returned to the panel rather than to a menu they are
// finished with (§7.1).
func (m *AppModel) closeStack() tea.Cmd {
	return tea.Batch(m.picker.close(), m.form.close(), m.confirm.close(),
		m.input.close(), m.help.close(), m.hostPicker.close(),
		m.transfersUI.close(), m.historyUI.close(), m.spaceMenu.close())
}

// ------------------------------------------------------------- panel level

func (m AppModel) panelKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	k := msg.String()

	// Resolve a pending g first, so a stray g never lingers into the next key.
	if m.pendingG {
		m.pendingG = false
		if k == "g" {
			m.hosts.handleKey("gg")
			return m, nil
		}
	}

	switch k {
	case "q":
		// Quitting kills every live session and every running transfer. That is a
		// real cost, so it gets a confirmation — but only when there is something
		// to lose. A transfer counts: dropping a half-copied file is worse than
		// dropping an idle shell, and warning about the shell but not about that
		// would be an odd place to draw the line.
		if lines := m.quitCost(); len(lines) > 0 {
			return m, m.confirm.ask(confirmPopup{
				glyph:  glyphWarn,
				title:  "Quit",
				lines:  append(lines, "Quit sshu?"),
				accept: "quit",
				warn:   true,
				action: confirmQuit,
			}, m.layer())
		}
		return m.quit()
	case "1":
		m.tab = tabHosts
		m.sftp.onScreen = false
		return m, nil
	case "2":
		m.tab = tabSFTP
		m.sftp.onScreen = true
		return m, m.sftp.startWatch()
	case "3":
		m.tab = tabSSH
		m.sftp.onScreen = false
		return m, nil
	case "4", "5", "6", "7":
		// A digit addresses a panel OF THE CURRENT TAB, and does nothing where no
		// such panel is on screen. The rule reads off the screen: the numbers you
		// can see are the numbers you can press.
		//
		// It used to jump across tabs — 4 from anywhere landed on tab [3]'s
		// sessions — which meant a digit did something the screen never showed.
		switch m.tab {
		case tabSSH:
			if p, ok := map[string]sshPanel{
				"4": panelSessions, "5": panelPty,
			}[k]; ok {
				m.ssh.setFocus(p)
			}
		case tabSFTP:
			if p, ok := map[string]sftpPanel{
				"4": panelLeftFiles, "5": panelLeftMarks,
				"6": panelRightFiles, "7": panelRightMarks,
			}[k]; ok {
				m.sftp.focus = p
			}
		}
		return m, nil
	case "tab", "shift+tab":
		// Tab cycles the panels OF THE CURRENT TAB and wraps there. It does not
		// cross into another tab: one key, one job, and the job is the thing you
		// can see. Changing tab is 1/2/3 (or the capsules).
		//
		// It used to run off the end into the next tab — "Tab walks surfaces, not
		// tabs" — which made the same keystroke mean two different sizes of move
		// depending on where in the cycle you happened to be.
		back := k == "shift+tab"
		switch m.tab {
		case tabSSH:
			m.ssh.cycleFocus(back)
		case tabSFTP:
			m.sftp.cycleFocus(back)
		}
		return m, nil
	case " ":
		m.spaceMenu.setItems(m.menuItems(), m.menuTitle(), 1)
		return m, m.spaceMenu.open()
	case "g":
		m.pendingG = true
		return m, nil
	}

	return m.dispatchKey(k)
}

// dispatchKey routes a key to the ACTIVE TAB's action table.
//
// Both ways of running an action come through here: the letter hotkey and the
// Space menu committing a row. They used to be separate — the menu called the
// hosts handler directly — so a menu row in tab [3] ran tab [1]'s action of the
// same letter, and tab [3]'s [C]lose opened tab [1]'s new-host form (which was
// on `c` at the time). Having one router is what
// makes that class of bug impossible rather than merely fixed.
func (m AppModel) dispatchKey(k string) (tea.Model, tea.Cmd) {
	switch m.tab {
	case tabHosts:
		return m.hostsKey(k)
	case tabSFTP:
		return m.sftpKey(k)
	case tabSSH:
		return m.sshKey(k)
	}
	return m, nil
}

// quitCost is what leaving costs right now, one line per kind. Empty means
// nothing is running, and the confirmation is skipped.
func (m AppModel) quitCost() []string {
	var lines []string
	if n := m.ssh.liveCount(); n > 0 {
		lines = append(lines, plural(n, "live session")+" will be closed.")
	}
	if n := m.transfers.runningCount(); n > 0 {
		lines = append(lines, plural(n, "running transfer")+" will be cancelled.")
	}
	return lines
}

// quit is the single exit. Every way out goes through it — q, the quit confirm
// and Ctrl+C — so none of them can forget one of the three things that have to
// be let go of.
func (m AppModel) quit() (tea.Model, tea.Cmd) {
	m.ssh.stopAll()
	m.transfers.cancelAll()
	m.sftp.closeAll()
	return m, tea.Quit
}

// inPty reports whether keystrokes belong to a remote right now.
func (m AppModel) inPty() bool {
	if m.tab != tabSSH || m.ssh.focus != panelPty || m.popupOpen() {
		return false
	}
	s := m.ssh.currentSession()
	return s != nil && s.state == sessLive && s.pty != nil
}

// textFloat reports whether a float that is being typed into owns the keyboard.
// That is what makes Space a character rather than a key (§4.5), and it is the
// one exception to the entry keys closing what they opened.
func (m AppModel) textFloat() bool {
	return m.form.anim.owns() || m.picker.anim.owns() || m.input.anim.owns()
}

// popupOpen reports whether any float owns the keyboard. A float on its way out
// does not (popupAnimator.owns) — the keyboard is back on the panel the moment
// the action commits, not when the animation finishes.
func (m AppModel) popupOpen() bool {
	return m.form.anim.owns() || m.picker.anim.owns() || m.confirm.anim.owns() ||
		m.input.anim.owns() || m.help.anim.owns() || m.spaceMenu.anim.owns() ||
		m.hostPicker.anim.owns() || m.transfersUI.anim.owns() ||
		m.historyUI.anim.owns()
}

// hostsKey dispatches one key on the hosts panel: an action from the table, or
// otherwise navigation.
func (m AppModel) hostsKey(k string) (tea.Model, tea.Cmd) {
	keys := make([]string, len(hostActions))
	for i, a := range hostActions {
		keys[i] = a.key
	}
	if i := hotkeyIndex(keys, k); i >= 0 {
		return hostActions[i].run(m)
	}
	m.hosts.handleKey(k)
	return m, nil
}

func (m AppModel) cursorHost() (store.Host, bool) {
	if m.tab != tabHosts || m.hosts.cursor >= len(m.hosts.hosts) {
		return store.Host{}, false
	}
	return m.hosts.hosts[m.hosts.cursor], true
}

// ------------------------------------------------------------- Space menu

// hostAction is one contextual action on the hosts panel. This table is the
// single declaration behind BOTH the letter hotkey and the Space menu row,
// which is how §4.2 stays true by construction rather than by discipline: you
// cannot add a hotkey without adding a menu entry, because they are one thing.
//
// needsHost marks the actions that apply to the cursor's card; they form the
// item region, which the menu lists first (§6.6 cursor-first).
type hostAction struct {
	key       string
	label     string
	hint      string
	needsHost bool
	run       func(AppModel) (tea.Model, tea.Cmd)
}

// The key string is also the bracket marking, so it is written in the case the
// menu should show (bracketHotkey). Either case still fires it — see hotkeyIndex.
var hostActions = []hostAction{
	{key: "enter", label: "Connect", hint: "Enter . ssh session", needsHost: true, run: AppModel.askConnect},
	{key: "E", label: "Edit", hint: "change this host", needsHost: true, run: AppModel.openEdit},
	{key: "D", label: "Delete", hint: "remove from hosts.yaml", needsHost: true, run: AppModel.askDelete},
	{key: "A", label: "Add", hint: "a new host", run: AppModel.openCreate},
}

// menuTitle names the surface the Space menu belongs to — the focused PANEL,
// not the tab. In a split tab "what can I do here" depends on which panel you
// are standing in, so a title that only says the tab cannot tell [4] from [6].
func (m AppModel) menuTitle() string {
	switch m.tab {
	case tabSSH:
		return m.ssh.panelTitle(m.ssh.focus)
	case tabSFTP:
		return m.sftp.panelTitle(m.sftp.focus)
	}
	return tabLabels[m.tab]
}

// menuItems is the §A.1 contents for whichever tab is up.
func (m AppModel) menuItems() []menuItem {
	switch m.tab {
	case tabSSH:
		return m.sshMenuItems()
	case tabSFTP:
		return m.sftpMenuItems()
	}
	h, hasHost := m.cursorHost()

	var item, panel []menuItem
	for _, a := range hostActions {
		row := menuItem{label: a.label, key: a.key, hint: a.hint}
		if a.needsHost {
			if hasHost {
				item = append(item, row)
			}
			continue
		}
		panel = append(panel, row)
	}

	var out []menuItem
	if len(item) > 0 {
		out = append(out, menuItem{label: "host . " + h.Name, header: true})
		out = append(out, item...)
		out = append(out, menuItem{separator: true})
	}
	out = append(out, menuItem{label: "panel", header: true})
	return append(out, panel...)
}

func (m AppModel) menuKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var key string
	m.spaceMenu, key, _ = m.spaceMenu.update(msg)
	if key == "" {
		return m, nil
	}
	// The menu stays behind whatever it launches (§6.4); the target decides
	// whether committing tears the stack down.
	return m.dispatchKey(key)
}

// ---------------------------------------------------------------- actions

func (m AppModel) openCreate() (tea.Model, tea.Cmd) {
	return m, m.form.openCreate(m.layer())
}

func (m AppModel) openEdit() (tea.Model, tea.Cmd) {
	h, ok := m.cursorHost()
	if !ok {
		return m, m.toast.show("No host selected", toastError)
	}
	return m, m.form.openEdit(h, m.layer())
}

func (m AppModel) askDelete() (tea.Model, tea.Cmd) {
	h, ok := m.cursorHost()
	if !ok {
		return m, m.toast.show("No host selected", toastError)
	}
	return m, m.confirm.ask(confirmPopup{
		glyph: glyphWarn,
		title: "Confirm",
		lines: []string{
			fmt.Sprintf("Delete host %q?", h.Name),
			"This rewrites hosts.yaml.",
		},
		accept: "delete",
		warn:   true,
		action: confirmDelete,
		target: h.Name,
	}, m.layer())
}

func (m AppModel) askConnect() (tea.Model, tea.Cmd) {
	h, ok := m.cursorHost()
	if !ok {
		return m, m.toast.show("No host selected", toastError)
	}
	return m, m.confirm.ask(confirmPopup{
		glyph: glyphConnect,
		title: "Connect",
		lines: []string{
			fmt.Sprintf("Connect to %s?", h.Name),
			h.Addr() + "  ·  " + string(h.Auth),
		},
		accept: "connect",
		action: confirmConnect,
		target: h.Name,
	}, m.layer())
}

func (m AppModel) confirmKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if !m.confirm.commit(msg) {
		return m, nil
	}
	switch m.confirm.action {
	case confirmDelete:
		return m.doDelete(m.confirm.target)
	case confirmConnect:
		return m.doConnect()
	case confirmClose:
		return m.doClose(m.confirm.target)
	case confirmDuplicate:
		return m.doDuplicate(m.confirm.target)
	case confirmTransfer:
		return m.startTransfer()
	case confirmQuit:
		return m.quit()
	case confirmDeleteItem:
		return m.doDeleteItem(m.confirm.target)
	case confirmDeleteMarks:
		return m.doDeleteMarks()
	}
	return m, m.closeStack()
}

func (m AppModel) doDelete(name string) (tea.Model, tea.Cmd) {
	hosts := make([]store.Host, 0, len(m.hosts.hosts))
	for _, h := range m.hosts.hosts {
		if h.Name != name {
			hosts = append(hosts, h)
		}
	}
	if err := m.persist(hosts); err != nil {
		return m, tea.Batch(m.closeStack(), m.toast.show(err.Error(), toastError))
	}
	m.hosts.hosts = hosts
	m.hosts.cursor = min(m.hosts.cursor, max(0, len(hosts)-1))
	m.hosts.ensureVisible()
	return m, tea.Batch(m.closeStack(),
		m.toast.show(fmt.Sprintf("Deleted %q", name), toastInfo))
}

// doConnect is the §7.1 context shift: an ssh session is a long-lived target, so
// the whole float stack goes before the tab switches — coming back out of a
// session onto a stale menu would just be disorienting.
func (m AppModel) doConnect() (tea.Model, tea.Cmd) {
	i := indexOfHost(m.hosts.hosts, m.confirm.target)
	if i < 0 {
		return m, m.closeStack()
	}
	return m.startSession(m.hosts.hosts[i])
}

// doDuplicate takes the host off the SESSION rather than looking it up in
// hosts.yaml: the entry may since have been renamed or deleted, and duplicating
// what is on screen should not depend on that.
func (m AppModel) doDuplicate(id string) (tea.Model, tea.Cmd) {
	s := m.sessionByID(id)
	if s == nil {
		return m, m.closeStack()
	}
	return m.startSession(s.host)
}

func (m AppModel) doClose(id string) (tea.Model, tea.Cmd) {
	s := m.sessionByID(id)
	if s == nil {
		return m, m.closeStack()
	}
	s.pty.stop() // the reaper moves it to history on the next tick
	m.ssh.setFocus(panelSessions)
	return m, tea.Batch(m.closeStack(), m.ssh.tick(),
		m.toast.show("Closed "+s.host.Name, toastInfo))
}

func (m AppModel) sessionByID(id string) *session {
	for _, s := range append(append([]*session{}, m.ssh.sessions...), m.ssh.history...) {
		if itoa(s.id) == id {
			return s
		}
	}
	return nil
}

// ------------------------------------------------------------------- form

func (m AppModel) formKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var res formResult
	m.form, res = m.form.update(msg)
	switch res {
	case formSubmit:
		return m.commitForm()
	case formBrowse:
		// The picker stacks on top of the form, which stays behind it (§6.4):
		// cancelling the pick returns to the half-filled form, not to the panel.
		return m, m.picker.open(identityRoot(), m.form.layer+1)
	}
	m.syncFormError()
	return m, nil
}

// syncFormError keeps a submitted form's error honest as the user edits. Before
// the first submit it does nothing: a form that has never been submitted has no
// business complaining yet.
func (m *AppModel) syncFormError() {
	if !m.form.submitted {
		return
	}
	m.form.refreshError(m.validateForm())
}

// pickerKey takes the chosen path straight into the field the picker was opened
// for. There is no intermediate confirmation: picking IS the confirmation.
func (m AppModel) pickerKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	path, done := m.picker.update(msg)
	if !done {
		return m, nil
	}
	m.form.fields[fIdentity].value = path
	m.form.fields[fIdentity].caret = len([]rune(path))
	m.form.focus = fIdentity
	m.syncFormError() // a pick is an edit; the error must react to it too
	return m, m.picker.close()
}

func (m AppModel) commitForm() (tea.Model, tea.Cmd) {
	m.form.submitted = true
	if msg, field := m.validateForm(); msg != "" {
		m.form.fail(msg, field)
		return m, nil
	}

	h := m.form.host()
	hosts := append([]store.Host(nil), m.hosts.hosts...)
	if m.form.editing != "" {
		i := indexOfHost(hosts, m.form.editing)
		if i < 0 {
			m.form.fail("This host is gone — it was removed elsewhere", -1)
			return m, nil
		}
		hosts[i] = h
	} else {
		hosts = append(hosts, h)
	}

	if err := m.persist(hosts); err != nil {
		m.form.fail(err.Error(), -1)
		return m, nil
	}

	m.hosts.hosts = hosts
	if i := indexOfHost(hosts, h.Name); i >= 0 {
		m.hosts.cursor = i // land the cursor on what was just saved
	}
	m.hosts.ensureVisible()
	return m, tea.Batch(m.closeStack(),
		m.toast.show(fmt.Sprintf("Saved %q", h.Name), toastInfo))
}

// validateForm checks the fields and says WHICH one is wrong, which is the part
// store.Validate cannot do — it validates the document, not a form. store stays
// the authority: SaveTo re-validates before it writes, so a rule added there is
// still enforced even if this layer misses it.
func (m AppModel) validateForm() (string, int) {
	name := strings.TrimSpace(m.form.fields[fName].value)
	switch {
	case name == "":
		return "Name is required", fName
	case strings.TrimSpace(m.form.fields[fHost].value) == "":
		return "Host is required", fHost
	case strings.TrimSpace(m.form.fields[fUser].value) == "":
		return "User is required", fUser
	}
	port, err := strconv.Atoi(strings.TrimSpace(m.form.fields[fPort].value))
	if err != nil || port < 1 || port > 65535 {
		return "Port must be 1-65535", fPort
	}
	for _, h := range m.hosts.hosts {
		if h.Name == name && name != m.form.editing {
			return fmt.Sprintf("A host named %q already exists", name), fName
		}
	}
	return "", -1
}

func (m AppModel) persist(hosts []store.Host) error {
	if m.save == nil {
		return nil // tests and dry runs
	}
	return m.save(hosts)
}

func indexOfHost(hosts []store.Host, name string) int {
	for i, h := range hosts {
		if h.Name == name {
			return i
		}
	}
	return -1
}
