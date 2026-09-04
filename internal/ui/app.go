package ui

import (
	"fmt"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/vulcanshen/sshu/internal/store"
)

// The three top-level surfaces. Screen order is manage / file transfer /
// ssh (that is what the strip reads left to right).
type tabID int

const (
	tabPref tabID = iota
	tabFT
	tabSSH
	tabCount
)

// Strip labels. Each opens with the key that reaches it, and the key is a bare
// SHIFTED letter — the tabs are the app's top-level structure, and reaching for
// them should not cost a modifier.
//
// They used to be Alt chords, with a lit [Alt] lead segment in front of the
// strip, so that switching tab worked even while a remote held the keyboard.
// That is no longer bought: inside a pty every key belongs to the far end and
// Alt+Esc comes out first (§11.21). What the chord cost was three of the app's
// most reachable keys, permanently.
//
// [M]anage rather than Preference: the tab holds hosts, credentials and logs —
// records, not settings — so the old name was answering for the wrong thing.
var tabLabels = []string{"[M]anage", "[F]ile transfer", "[S]SH"}

// chromeRows is the fixed chrome the panels do NOT get: the capsule row, the
// rule under it, and the footer. Locked at 3 — none of them ever reflows (§1.3).
const chromeRows = 3

// SaveFunc persists the host list. Injected so the UI never reaches for the
// filesystem itself, and so tests can watch what would have been written.
type SaveFunc func([]store.Host) error

type AppModel struct {
	w, h       int
	tab        tabID
	pref       prefModel
	exportPage bundlePage
	importPage bundlePage
	hosts      hostsModel
	creds      credsModel
	saveCreds  func([]store.Credential) error
	ssh        sshModel
	sftp       sftpModel
	transfers  transferModel
	cfg        store.Config
	pending    pendingTransfer
	// pendingEdit is the file currently being edited, and it outlives the popup
	// on purpose: the overwrite question is asked after the box is gone, and its
	// answer needs the local copy that box was standing on.
	pendingEdit editJob
	save        SaveFunc

	// Floats. At most one of form / confirm / help is up at a time, optionally
	// over the Space menu; the toast rides on top of everything.
	spaceMenu   spaceMenu
	splash      splashModel
	hostPicker  spaceMenu
	credPicker  spaceMenu
	transfersUI transfersPopup
	log         appLog
	viewer      viewerPopup
	detail      detailPopup
	editorUI    editorPopup
	help        helpPopup
	form        hostForm
	credFormUI  credForm
	picker      filePicker
	confirm     confirmPopup
	input       inputPopup
	toast       toastModel

	// pendingG holds the first half of the gg chord. A chord is a shortcut for an
	// action that already exists, so it costs no core-key slot (§A.0.Y).
	pendingG bool
}

// New builds the app. cfg is config.yaml, already loaded — the UI never reads
// the filesystem itself.
func New(hosts []store.Host, save SaveFunc, cfg store.Config) AppModel {
	m := AppModel{
		cfg: cfg,
		// The tab opens ON its content — the hosts table, exactly where the
		// old hosts tab put you. The nav is chrome you visit (1, or Tab).
		pref:        prefModel{focus: panelPrefContent},
		exportPage:  newExportPage(),
		importPage:  newImportPage(),
		hosts:       hostsModel{hosts: hosts},
		ssh:         newSSHModel(),
		sftp:        newSFTPModel(),
		transfersUI: newTransfersPopup(),
		log:         newAppLog(),
		viewer:      newViewerPopup(),
		detail:      newDetailPopup(),
		editorUI:    newEditorPopup(),
		hostPicker:  newHostPicker(),
		credPicker:  newCredPicker(),
		save:        save,
		spaceMenu:   newSpaceMenu(),
		splash:      newSplashModel(),
		help:        newHelpPopup(),
		form:        newHostForm(),
		credFormUI:  newCredForm(),
		picker:      newFilePicker(),
		confirm:     newConfirmPopup(),
		input:       newInputPopup(),
		toast:       newToast(),
	}
	m.ssh.timeout, m.sftp.timeout = cfg.Timeout(), cfg.Timeout()
	return m
}

// WithCredentials wires credentials.yaml in: the list, and how to save it.
func (m AppModel) WithCredentials(creds []store.Credential, save func([]store.Credential) error) AppModel {
	m.creds.creds = creds
	m.hosts.creds = creds
	m.saveCreds = save
	return m
}

// WithLog wires the app log to applogs.yaml: tail is what the file already
// held (shown, all read), sink is where each new entry goes, clear is how
// [C]lear logs empties the file. Applied before WithStartupError so a startup
// complaint lands after the tail and on disk.
func (m AppModel) WithLog(tail []store.LogEntry, sink func(store.LogEntry) error,
	clear func() error) AppModel {
	m.log.preload(tail)
	m.log.sink = sink
	m.log.clearSink = clear
	return m
}

// WithStartupError records something that went wrong before the first frame.
// It is a method rather than a New parameter because the caller may or may not
// have one, and a nil-able argument for "nothing was wrong" reads worse than
// not calling this at all.
func (m AppModel) WithStartupError(msg string) AppModel {
	m.log.errorf(msg)
	return m
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
		m.pref.setSize(m.w, m.panelHeight())
		m.syncPrefSizes()
		m.ssh.setSize(m.w, m.panelHeight())
		m.sftp.setSize(m.w, m.panelHeight())
		m.hostPicker.setSize(m.w, m.h)
		m.credPicker.setSize(m.w, m.h)
		m.transfersUI.setSize(m.w, m.h)
		m.viewer.setSize(m.w, m.h)
		m.detail.setSize(m.w, m.h)
		m.editorUI.setSize(m.w, m.h)
		m.spaceMenu.setSize(m.w, m.h)
		m.help.setSize(m.w, m.h)
		m.form.setSize(m.w, m.h)
		m.credFormUI.setSize(m.w, m.h)
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
			m.credPicker.anim.tick(msg),
			m.transfersUI.anim.tick(msg),
			m.viewer.anim.tick(msg),
			m.detail.anim.tick(msg),
			m.editorUI.anim.tick(msg),
			m.help.anim.tick(msg),
			m.form.anim.tick(msg),
			m.credFormUI.anim.tick(msg),
			m.picker.anim.tick(msg),
			m.confirm.anim.tick(msg),
			m.input.anim.tick(msg),
			m.toast.anim.tick(msg),
		)

	case splashTickMsg, splashIdentityMsg, splashHintMsg:
		var cmd tea.Cmd
		m.splash, cmd = m.splash.update(msg)
		return m, cmd

	case sshTickMsg:
		// One tick drives both jobs: reap what has finished and repaint what is
		// still drawing. It only runs while something is live, so an idle sshu
		// costs nothing.
		m.ssh.spinAt++
		m.ssh.sweepStalled()
		ended := m.ssh.reap()
		if len(ended) > 0 {
			m.ssh.setSize(m.w, m.panelHeight())
		}
		// Two channels, two jobs: the toast is "this just happened" and is gone
		// in two seconds; the log is the record you can still read afterwards.
		// A session dying used to have only the first, which meant looking away
		// for a moment was the same as never being told.
		var bad []*session
		for _, s := range ended {
			line := s.host.Name + " · " + s.reason
			if s.ok {
				m.log.info(line)
				continue
			}
			// The headline plus everything ssh printed. A refused connection is
			// one line either way; a host key mismatch is fifteen, and the
			// fingerprint you need is in the middle of them.
			m.log.errorf(line, s.detail...)
			bad = append(bad, s)
		}
		if msg := endedBadlyToast(bad); msg != "" {
			return m, tea.Batch(m.ssh.tick(), m.toast.show(msg, toastError))
		}
		return m, m.ssh.tick()

	case sftpConnectedMsg:
		return m.sftpConnected(msg)

	case viewLoadedMsg:
		m.viewer.onLoaded(msg)
		return m, nil

	case editTickMsg:
		return m.onEditTick()

	case editFetchedMsg:
		return m.onEditFetched(msg)

	case editSavedMsg:
		return m.onEditSaved(msg)

	case dialTickMsg:
		// Turns the connecting spinner. A dial can take fifteen seconds and the
		// panel has to look alive for all of them.
		return m, m.sftp.onDialTick()

	case xferTickMsg:
		if m.logFinishedTransfers() {
			m.relistSides() // the spinner stops and the listing catches up together
		}
		m.transfers.spinAt++ // the summary's dots turn on the tick that repaints them
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
		m.logFinishedTransfers()
		m.relistSides()
		return m, m.transfers.tick()

	case toastExpireMsg:
		return m, m.toast.expire(msg)

	case clipboardDoneMsg:
		if msg.err != nil {
			return m, m.toast.show(clipboardFailure(msg.err), toastError)
		}
		return m, m.toast.show("copied "+plural(msg.lines, "line"), toastInfo)

	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m AppModel) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// The easter-egg splash owns the keyboard until dismissed — any key closes
	// it, and nothing underneath sees the press.
	if m.splash.isActive() {
		var cmd tea.Cmd
		m.splash, cmd = m.splash.update(msg)
		return m, cmd
	}

	// A running editor owns the keyboard completely, the way panel [5] does:
	// Esc is vim's Esc, q is a letter, Space is a space. It is checked before
	// everything else because every rule below would otherwise take a key the
	// editor needs.
	//
	// One key is kept: Alt+Esc abandons the edit — in tab [3] it means "take the
	// keyboard back", and here there is nowhere else to take it, so leaving is
	// what taking it back is.
	//
	// Ctrl+C is NOT kept. It used to quit sshu from in here, on the grounds that
	// an emergency exit which means something different in one PTY has stopped
	// being one. That reasoning was backwards: inside an editor Ctrl+C is the
	// editor's own key, and reaching for it — to cancel an insert, to interrupt
	// something — and losing every session instead is not an emergency exit, it
	// is the emergency. The exit is one keystroke further away and always
	// advertised: Alt+Esc, then Ctrl+C.
	if m.editorUI.running() {
		if msg.Type == tea.KeyEscape && msg.Alt {
			return m.abandonEdit()
		}
		m.editorUI.pty.write(msg)
		return m, nil
	}

	// Selection mode owns its panel completely, and is checked before every
	// other ssh chord for the reason the editor block above is checked first:
	// the cell has stopped following the remote, so it is a different surface,
	// and a chord that zoomed it or moved the keyboard off it mid-sweep would
	// throw the selection away for a gesture nobody meant to make (§11.33).
	if m.ssh.copy.on {
		if !m.ssh.copyAlive() {
			m.ssh.copy.stop() // the session ended underneath the mode
		} else {
			return m.copyModeKey(msg)
		}
	}

	// Alt+v freezes the focused cell and puts a selection cursor on it, so its
	// text can be swept to the system clipboard. It is a chord because every
	// bare key in a pty belongs to the remote, and v because that is the key
	// that starts a selection once you are inside (§11.33).
	if m.tab == tabSSH && !m.popupOpen() && msg.Alt &&
		msg.Type == tea.KeyRunes && string(msg.Runes) == "v" && m.inPty() {
		m.ssh.startCopy()
		return m, nil
	}

	// Hold Alt and steer: the arrows move the keyboard to the neighbouring
	// cell in that direction. Spatial, so nothing has to be numbered and a
	// reflow cannot invalidate what you memorised. (Alt+digits were tried and
	// dropped: window managers own Alt+1..9 on the LOCAL side — AeroSpace,
	// tiling tools — so the chords never even reached sshu.) Alt+arrows are
	// dead keys in a bare terminal, which is what makes them takeable.
	// Alt+Enter fills the screen with the focused cell. Enter is "go in" all
	// over sshu, and this is going further in — which is also why Alt+Esc, the
	// key that comes back OUT, is what leaves it (§11.25). With one cell on the
	// grid there is nothing to zoom, so the chord is not taken and travels on
	// to the remote like any other.
	if m.tab == tabSSH && !m.popupOpen() && msg.Alt && msg.Type == tea.KeyEnter {
		if m.ssh.toggleZoom() {
			return m, nil
		}
	}

	if m.tab == tabSSH && !m.popupOpen() && msg.Alt && m.ssh.focus == panelPty {
		switch msg.Type {
		case tea.KeyLeft:
			m.ssh.moveCell(-1, 0)
			return m, nil
		case tea.KeyRight:
			m.ssh.moveCell(1, 0)
			return m, nil
		case tea.KeyUp:
			m.ssh.moveCell(0, -1)
			return m, nil
		case tea.KeyDown:
			m.ssh.moveCell(0, 1)
			return m, nil
		}
	}

	// Alt+Esc is sshu's own key, not a VTP core key: it exists because panel [5]
	// hands the keyboard to a remote program, and something has to be able to
	// take it back. It is scoped to that one situation — everywhere else it is
	// just Esc, so it is never a dead key. Disclosed in the footer whenever the
	// PTY holds focus, which is the only place it means anything.
	if msg.Type == tea.KeyEscape && msg.Alt {
		if m.ptyFocused() {
			// One step at a time, outward. A zoom is a layer the user put
			// themselves inside, so the key that comes back out takes it off
			// first and hands the keyboard back second — pressing it once and
			// landing two levels away is how a way-out key stops being
			// predictable (§11.25).
			if m.ssh.zoomed {
				m.ssh.zoomed = false
				m.ssh.applyGeometry()
				return m, nil
			}
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
		// A search is the innermost thing Esc can drop, on either tab that has
		// one.
		if !m.popupOpen() && m.tab == tabPref && m.pref.item == prefHosts && m.hosts.filtering {
			m.hosts.clearFilter()
			return m, nil
		}
		// An Operation page is left the way any text surface is: Esc hands
		// the keyboard back to the nav. It HAS to be Esc — the page swallows
		// the digits and Tab that move focus everywhere else (§4.5).
		if m.textPage() {
			m.pref.focus = panelPrefNav
			m.syncPrefSizes()
			return m, nil
		}
		// filu's two-stage Esc: close a float if one is up, otherwise go up a
		// directory. Only the sftp browser has an "up" to go to.
		if !m.popupOpen() && m.tab == tabFT && !m.sftp.focus.isMarks() {
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
	//
	// Everywhere except inside a remote. In there it is the far end's interrupt,
	// and it is the most reflexive key a shell has: reaching for it to kill a
	// runaway command and losing every session in the app instead is not an
	// emergency exit, it is the emergency. Nothing is stranded by giving it up —
	// Alt+Esc takes the keyboard back and Ctrl+C is itself again on the far side
	// of it, and the footer has been advertising that key the whole time.
	if msg.Type == tea.KeyCtrlC && !m.inPty() {
		return m.quit()
	}

	// A focused PTY swallows everything else — that is what focusing it means.
	if m.inPty() {
		// Except the two keys that are about sshu's own window onto the remote
		// rather than about the remote. They are taken back only while the far
		// end has no use for them (§11.19).
		if m.ssh.scrollKey(msg.String()) {
			return m, nil
		}
		m.ssh.currentSession().pty.write(msg)
		return m, nil
	}
	// Focused, but the far end has not spoken yet. The keys are NOT forwarded:
	// ssh is not reading its stdin while it waits for a connection, so they
	// would sit in a buffer and be delivered to the remote shell minutes later —
	// a `q` meant for sshu, run on somebody else's machine. They are swallowed
	// instead, because the panel does have the keyboard; the way out is the
	// Alt+Esc the footer is already advertising.
	if m.ptyFocused() {
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
	if msg.String() == "?" && !m.typing() && !m.inPty() {
		if m.help.anim.owns() {
			return m, m.help.close()
		}
		// It opens from ON TOP of another float too. §A.2 promises the help is
		// reachable from any surface, and the surface a lost user is most likely
		// to be standing on is the menu they just opened.
		return m, m.help.open(m.layer())
	}
	// V is the hidden u-family easter egg: the logo, revealed. It is the
	// LOWEST claim on the key in the app — a panel with a real [V]iew takes
	// it, and the logo gets what is left (§11.29).
	if msg.String() == "V" && !m.typing() && !m.popupOpen() && !m.panelClaimsView() {
		return m, m.splash.show()
	}

	// The tab keys. They sit HERE — below the pty, below the floats — rather
	// than at the top of this function, which is where they lived as Alt chords
	// so that they could reach past a remote. A bare letter cannot do that and
	// should not try: inside a pty every key is the far end's, and Alt+Esc is
	// the way out (§11.21). Under a popup they do nothing either — a tab
	// switching beneath a form would strand the form over a surface it knows
	// nothing about.
	if !m.popupOpen() && !m.typing() {
		if t, ok := tabForKey(msg.String()); ok {
			return m.switchTab(t)
		}
	}

	// A filtering file list claims printable keys before the action table can:
	// while a query is being typed, "a" is a letter, not Append. Arrows, Enter and
	// Esc fall through — the same split the picker and the form make (§4.5).
	if m.tab == tabFT && !m.popupOpen() && m.sftp.cur().filterKey(msg) {
		return m, nil
	}
	if m.tab == tabPref && m.pref.item == prefHosts && !m.popupOpen() && m.hosts.filterKey(msg) {
		return m, nil
	}
	// An Operation page (Export / Import) is a text surface IN a panel: while
	// it holds focus it claims every printable key, Tab and Enter, the same
	// §4.5 split every other typing surface makes.
	if m.textPage() {
		return m.bundlePageKey(msg)
	}

	// Same rule as popupOpen: a float that is closing has already handed the
	// keyboard back, so it is not offered the key.
	switch {
	case m.transfersUI.anim.owns():
		if i := m.transfersUI.update(msg, len(m.transfers.jobs)); i >= 0 {
			m.transfers.cancelJob(i)
		}
		return m, nil
	case m.viewer.anim.owns():
		m.viewer.update(msg)
		return m, nil
	case m.detail.anim.owns():
		m.detail.update(msg)
		return m, nil
	case m.editorUI.anim.owns():
		// Fetching or writing: the only keys are Esc and Space, and both were
		// resolved above as "close the topmost float".
		return m, nil
	case m.hostPicker.anim.owns():
		return m.hostPickerKey(msg)
	case m.credPicker.anim.owns():
		return m.credPickerKey(msg)
	case m.picker.anim.owns():
		return m.pickerKey(msg)
	case m.form.anim.owns():
		return m.formKey(msg)
	case m.credFormUI.anim.owns():
		return m.credFormKey(msg)
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
	case inputAdd:
		return m.doAdd(value)
	case inputGridDims:
		return m.applyGridDims(value)
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
	case m.viewer.isActive():
		return m, m.viewer.close()
	case m.detail.isActive():
		return m, m.detail.close()
	case m.hostPicker.isActive():
		return m, m.hostPicker.close()
	case m.credPicker.isActive():
		return m, m.credPicker.close()
	case m.picker.isActive():
		return m, m.picker.close()
	case m.form.isActive():
		return m, m.form.close()
	case m.credFormUI.isActive():
		return m, m.credFormUI.close()
	case m.input.isActive():
		return m, m.input.close()
	case m.confirm.isActive():
		// Cancelling is free on every confirm but the two edit ones, which are
		// standing on a local copy that has to be cleaned up or told about.
		return m, tea.Batch(m.confirm.close(), m.declineEdit())
	case m.editorUI.isActive():
		return m, m.closeEdit(false)
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
	return tea.Batch(m.picker.close(), m.form.close(), m.credFormUI.close(),
		m.confirm.close(), m.input.close(), m.help.close(), m.hostPicker.close(),
		m.credPicker.close(), m.transfersUI.close(), m.viewer.close(), m.detail.close(),
		m.editorUI.close(), m.spaceMenu.close())
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
	case "1", "2", "3", "4", "5", "6", "7", "8", "9":
		// A digit addresses a panel OF THE CURRENT TAB, and does nothing where no
		// such panel is on screen. The rule reads off the screen: the numbers you
		// can see are the numbers you can press. Every digit is free for this
		// now that the tabs live on Alt chords — each tab numbers its panels
		// from 1.
		m.focusPanelDigit(k)
		return m, nil
	case "tab", "shift+tab":
		// Tab cycles the panels OF THE CURRENT TAB and wraps there. It does not
		// cross into another tab: one key, one job, and the job is the thing you
		// can see. Changing tab is an Alt chord.
		//
		// It used to run off the end into the next tab — "Tab walks surfaces, not
		// tabs" — which made the same keystroke mean two different sizes of move
		// depending on where in the cycle you happened to be.
		back := k == "shift+tab"
		switch m.tab {
		case tabPref:
			if m.pref.focus == panelPrefNav {
				m.pref.focus = panelPrefContent
			} else {
				m.pref.focus = panelPrefNav
			}
			m.syncPrefSizes()
			m.prefShowed()
		case tabSSH:
			// This tab repurposes Tab: the grid is not somewhere Tab may wander
			// (it would swallow the key), so on the list it toggles the cursor
			// session's cell instead — the thing the list does all day.
			m.ssh.toggleCursorShown()
		case tabFT:
			m.sftp.cycleFocus(back)
		}
		return m, nil
	case " ":
		// A list of one is not a list. A side with no host can do exactly one
		// thing, and Space's whole promise is "what can I do here" — so it
		// answers with the thing rather than with a menu whose only row points
		// at it. Routed through sftpKey rather than straight at the picker, so
		// this shortcut is the letter `H` by definition and cannot drift from
		// it — the running-transfer guard included (§4.2).
		if m.tab == tabFT && m.sftp.cur().fs == nil {
			return m.sftpKey(keySelectHost)
		}
		m.spaceMenu.setItems(m.menuItems(), m.menuTitle(), 1)
		return m, m.spaceMenu.open()
	case "g":
		m.pendingG = true
		return m, nil
	}

	return m.dispatchKey(k)
}

// tabForKey resolves a tab-switch key. Shifted only: the lowercase letters are
// the panels' own vocabulary (m, f and s all mean something somewhere), and the
// case is what tells the two apart — the same exact-case rule hotkeyIndex keeps.
func tabForKey(k string) (tabID, bool) {
	switch k {
	case "M":
		return tabPref, true
	case "F":
		return tabFT, true
	case "S":
		return tabSSH, true
	}
	return 0, false
}

// typing reports whether a printable key is a CHARACTER right now rather than a
// command: a list being filtered, an Operation page, or a float being typed
// into. Every bare global letter asks this before it acts.
//
// It exists because the alternative — each key remembering its own list of
// exceptions — had already failed twice: V opened the splash and ? opened the
// help while a filename was being typed into a search (§11.21). One question,
// asked in one place, so a new global key cannot forget half the answer.
func (m AppModel) typing() bool {
	if m.textFloat() || m.textPage() {
		return true
	}
	if m.popupOpen() {
		return false // a float has the keyboard; the panel behind it is not being typed into
	}
	if m.tab == tabFT && m.sftp.cur().filtering {
		return true
	}
	return m.tab == tabPref && m.pref.item == prefHosts && m.hosts.filtering
}

// switchTab is the one place the tab changes. The sftp watcher only runs
// while its tab is on screen, and a second caller that forgot onScreen is how
// a hidden tab ends up polling a connection nobody is looking at.
func (m AppModel) switchTab(t tabID) (tea.Model, tea.Cmd) {
	m.tab = t
	m.sftp.onScreen = t == tabFT
	if t == tabPref {
		m.syncPrefSizes()
		m.prefShowed()
	}
	if t == tabFT {
		return m, m.sftp.startWatch()
	}
	return m, nil
}

// focusPanelDigit maps a digit onto the current tab's panels, numbered from 1
// in the order they are drawn. A digit with no panel behind it does nothing.
func (m *AppModel) focusPanelDigit(k string) {
	switch m.tab {
	case tabPref:
		if p, ok := map[string]prefPanel{
			"1": panelPrefNav, "2": panelPrefContent,
		}[k]; ok {
			m.pref.focus = p
			m.syncPrefSizes()
			m.prefShowed()
		}
	case tabSSH:
		if p, ok := map[string]sshPanel{
			"1": panelSessions, "2": panelLayout,
		}[k]; ok {
			m.ssh.setFocus(p)
		}
	case tabFT:
		if p, ok := map[string]sftpPanel{
			"1": panelLeftFiles, "2": panelLeftMarks,
			"3": panelRightFiles, "4": panelRightMarks,
		}[k]; ok {
			m.sftp.focus = p
		}
	}
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
	case tabPref:
		return m.prefKey(k)
	case tabFT:
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
	// The editor's temp file is deliberately NOT removed here: leaving on the
	// way out is exactly when an unsaved edit is worth keeping.
	m.editorUI.stop()
	return m, tea.Quit
}

// ptyFocused reports whether panel [5] holds the keyboard: there is a live
// session on screen and no float over it.
func (m AppModel) ptyFocused() bool {
	if m.tab != tabSSH || m.ssh.focus != panelPty || m.popupOpen() {
		return false
	}
	s := m.ssh.currentSession()
	return s != nil && s.state == sessLive && s.pty != nil
}

// inPty is narrower: the keystrokes belong to a REMOTE, which is only true once
// there is one to belong to. Until ssh has said something there is no session at
// the far end to type into — only a socket being opened — so the two questions
// stopped being the same question.
func (m AppModel) inPty() bool {
	return m.ptyFocused() && m.ssh.currentSession().pty.hasSpoken()
}

// textFloat reports whether a float that is being typed into owns the keyboard.
// That is what makes Space a character rather than a key (§4.5), and it is the
// one exception to the entry keys closing what they opened.
func (m AppModel) textFloat() bool {
	return m.form.anim.owns() || m.credFormUI.anim.owns() ||
		m.picker.anim.owns() || m.input.anim.owns()
}

// logFinishedTransfers records each job's ending exactly once. Swept from the
// transfer messages rather than hooked into the copy goroutine, so the log
// write happens on the UI loop like every other entry.
// logFinishedTransfers reports whether any job ENDED on this pass. The caller
// re-lists on a true: the destination is only whole once the last byte is
// written, and waiting for the two-second watch poll leaves a spinner turning
// over a file that is already finished.
func (m *AppModel) logFinishedTransfers() bool {
	ended := false
	for _, j := range m.transfers.jobs {
		if j.logged {
			continue
		}
		// ended rides along with logged: they mean the same thing — this job
		// reached its end on THIS pass — and a separate flag set before the
		// switch would be turned back off by the next job that is still
		// running.
		switch j.status() {
		case xferDone:
			j.logged, ended = true, true
			m.log.info("transfer done: " + j.label)
		case xferCancelled:
			j.logged, ended = true, true
			m.log.info("transfer cancelled: " + j.label)
		case xferFailed:
			j.logged, ended = true, true
			m.log.errorf("transfer failed: "+j.label, j.err())
		}
	}
	return ended
}

// relistSides re-lists both connected sides in place. What arrived should show
// up without the user navigating away and back.
func (m *AppModel) relistSides() {
	for i := range m.sftp.sides {
		if s := &m.sftp.sides[i]; s.fs != nil && s.cwd != "" {
			s.reload()
		}
	}
}

// popupOpen reports whether any float owns the keyboard. A float on its way out
// does not (popupAnimator.owns) — the keyboard is back on the panel the moment
// the action commits, not when the animation finishes.
func (m AppModel) popupOpen() bool {
	return m.form.anim.owns() || m.credFormUI.anim.owns() || m.picker.anim.owns() ||
		m.confirm.anim.owns() || m.input.anim.owns() || m.help.anim.owns() ||
		m.spaceMenu.anim.owns() || m.hostPicker.anim.owns() ||
		m.credPicker.anim.owns() || m.transfersUI.anim.owns() ||
		m.viewer.anim.owns() || m.editorUI.anim.owns() || m.detail.anim.owns()
}

// panelClaimsView reports whether the focused panel's own table takes V right
// now. It asks the SAME applicable-actions function the menu asks rather than
// restating the condition, so the easter egg cannot come to disagree with the
// row it is standing aside for — including on an empty table, where there is
// nothing to view and the logo is welcome to the key (§4.2).
func (m AppModel) panelClaimsView() bool {
	if m.tab != tabPref || m.pref.focus != panelPrefContent {
		return false
	}
	switch m.pref.item {
	case prefHosts:
		keys, _ := m.hostsApplicable()
		return hotkeyIndex(keys, "V") >= 0
	case prefCreds:
		keys, _ := m.credsApplicable()
		return hotkeyIndex(keys, "V") >= 0
	}
	return false
}

// hostsKey dispatches one key on the hosts panel: an action from the table, or
// otherwise navigation.
func (m AppModel) hostsKey(k string) (tea.Model, tea.Cmd) {
	keys, acts := m.hostsApplicable()
	if i := hotkeyIndex(keys, k); i >= 0 {
		return acts[i].run(m)
	}
	m.hosts.handleKey(k)
	return m, nil
}

func (m AppModel) cursorHost() (store.Host, bool) {
	if m.tab != tabPref || m.pref.item != prefHosts {
		return store.Host{}, false
	}
	return m.hosts.rowAt(m.hosts.cursor)
}

func (m AppModel) hostsStartFilter() (tea.Model, tea.Cmd) {
	m.hosts.startFilter()
	return m, m.closeStack()
}

// ------------------------------------------------------------- Space menu

// hostAction is one contextual action on the hosts panel. This table is the
// single declaration behind BOTH the letter hotkey and the Space menu row,
// which is how §4.2 stays true by construction rather than by discipline: you
// cannot add a hotkey without adding a menu entry, because they are one thing.
//
// The two flags answer two different questions, which is why they are two:
// needsHost is "is there anything to do this to", panelOp is "which region of
// the menu does it belong in". Search needs a list but is not about the cursor's
// host, so it is the one action that says yes to both.
type hostAction struct {
	key       string
	label     string
	hint      string
	needsHost bool
	panelOp   bool
	run       func(AppModel) (tea.Model, tea.Cmd)
}

// The key string is also the bracket marking, so it is written in the case the
// menu should show (bracketHotkey). Either case still fires it — see hotkeyIndex.
var hostActions = []hostAction{
	// item — the host under the cursor
	{key: "enter", label: "Connect", hint: "Enter . ssh session", needsHost: true, run: AppModel.askConnect},
	{key: "V", label: "View", hint: "what this host is, secrets masked", needsHost: true, run: AppModel.openHostView},
	{key: "E", label: "Edit", hint: "change this host", needsHost: true, run: AppModel.openEdit},
	{key: "D", label: "Duplicate", hint: "a new host starting from this one", needsHost: true, run: AppModel.openHostDuplicate},
	{key: "X", label: "Delete", hint: "remove from hosts.yaml", needsHost: true, run: AppModel.askDelete},

	// panel — the table
	{key: "A", label: "Add", hint: "a new host", panelOp: true, run: AppModel.openCreate},
	{key: "/", label: "Search", hint: "name, user, host, port", needsHost: true, panelOp: true, run: AppModel.hostsStartFilter},
}

// hostsApplicable is what panel [1] can do right now. Both the hotkey and the
// menu read it, so they cannot drift apart (§4.2).
func (m AppModel) hostsApplicable() ([]string, []hostAction) {
	_, hasHost := m.cursorHost()
	var keys []string
	var acts []hostAction
	for _, a := range hostActions {
		if a.needsHost && !hasHost {
			continue
		}
		keys, acts = append(keys, a.key), append(acts, a)
	}
	return keys, acts
}

// The two region labels, in kbu's wording. They are constants because three
// menus use them and a menu whose regions are worded differently from another's
// reads as a different KIND of menu rather than as the same one.
const (
	menuItemRegion  = "item operation"
	menuPanelRegion = "panel operation"
)

// menuTitle names the surface the Space menu belongs to — the focused PANEL,
// not the tab. In a split tab "what can I do here" depends on which panel you
// are standing in, so a title that only says the tab cannot tell [4] from [6].
func (m AppModel) menuTitle() string {
	switch m.tab {
	case tabSSH:
		return m.ssh.panelTitle(m.ssh.focus)
	case tabFT:
		return m.sftp.panelTitle(m.sftp.focus)
	}
	if m.pref.focus == panelPrefNav {
		return m.pref.panelTitle(panelPrefNav)
	}
	return m.pref.panelTitle(panelPrefContent)
}

// menuItems is the §A.1 contents for whichever tab is up.
func (m AppModel) menuItems() []menuItem {
	switch m.tab {
	case tabSSH:
		return m.sshMenuItems()
	case tabFT:
		return m.sftpMenuItems()
	}
	if m.pref.focus == panelPrefNav {
		return []menuItem{
			{label: "sshu", header: true},
			{label: "j/k choose a section — Enter opens it", header: true},
		}
	}
	switch m.pref.item {
	case prefCreds:
		return m.credsMenuItems()
	case prefLogs:
		if len(m.log.entries) == 0 {
			return []menuItem{
				{label: "app log", header: true},
				{label: "nothing recorded yet", header: true},
			}
		}
		return []menuItem{
			{label: "app log", header: true},
			{label: "newest first — j/k scroll", header: true},
			{separator: true},
			{label: "Clear logs", key: "C", hint: "erase every entry"},
		}
	case prefExport, prefImport:
		return []menuItem{
			{label: "operation", header: true},
			{label: "a form — letters type, Enter runs it", header: true},
		}
	}
	_, acts := m.hostsApplicable()

	var item, panel []menuItem
	for _, a := range acts {
		row := menuItem{label: a.label, key: a.key, hint: a.hint}
		if a.panelOp {
			panel = append(panel, row)
			continue
		}
		item = append(item, row)
	}
	// One region stays flat — a header over a single group is noise (§6.2). An
	// empty table is exactly that case: Add, and nothing else.
	if len(item) == 0 || len(panel) == 0 {
		return append(item, panel...)
	}

	out := []menuItem{{label: menuItemRegion, header: true}}
	out = append(out, item...)
	out = append(out, menuItem{separator: true},
		menuItem{label: menuPanelRegion, header: true})
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

// openHostDuplicate opens a CREATE form already holding everything the row
// under the cursor holds (§11.35). Nothing is written yet and no name is
// invented: the form arrives complete, so Enter means save (§11.34), and the
// name it arrives holding is taken — by the row it was copied from. The first
// Enter is therefore refused ON the Name field, which is where the cursor
// already is. Forcing the rename is the uniqueness check doing its ordinary
// job, not a special case bolted on for duplication.
func (m AppModel) openHostDuplicate() (tea.Model, tea.Cmd) {
	h, ok := m.cursorHost()
	if !ok {
		return m, m.toast.show("No host selected", toastError)
	}
	return m, m.form.openDuplicate(h, m.layer())
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
	// Resolve NOW, so the confirmation shows who the connection will actually
	// run as — and so a missing credential fails here, with a sentence, rather
	// than three keystrokes later inside ssh.
	rh, err := store.Resolve(h, m.creds.creds)
	if err != nil {
		m.log.errorf(err.Error())
		return m, m.toast.show(err.Error(), toastError)
	}
	authNote := string(h.Auth)
	if h.Auth == store.AuthCredential {
		authNote = "credential " + h.Credential
	}
	return m, m.confirm.ask(confirmPopup{
		glyph: glyphConnect,
		title: "Connect",
		lines: []string{
			fmt.Sprintf("Connect to %s?", h.Name),
			rh.Addr() + "  ·  " + authNote,
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
	case confirmCloseAll:
		return m.doCloseAll()
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
	case confirmDeleteCred:
		return m.doDeleteCred(m.confirm.target)
	case confirmEditBinary:
		return m.launchEditor()
	case confirmEditOverwrite:
		return m.saveEditForced()
	case confirmClearLogs:
		return m.doClearLogs()
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
	m.log.info(fmt.Sprintf("host %q deleted", name))
	return m, tea.Batch(m.closeStack(),
		m.toast.show(fmt.Sprintf("Deleted %q", name), toastInfo))
}

// doConnect is the §7.1 context shift: an ssh session is a long-lived target, so
// the whole float stack goes before the tab switches — coming back out of a
// session onto a stale menu would just be disorienting. It lands in the remote,
// because reaching a remote is what the key was pressed for.
func (m AppModel) doConnect() (tea.Model, tea.Cmd) {
	i := indexOfHost(m.hosts.hosts, m.confirm.target)
	if i < 0 {
		return m, m.closeStack()
	}
	return m.startSession(m.hosts.hosts[i], panelPty)
}

// doDuplicate takes the host off the SESSION rather than looking it up in
// hosts.yaml: the entry may since have been renamed or deleted, and duplicating
// what is on screen should not depend on that.
//
// It lands back on [1], not in the new session (§11.23): duplicating is a list
// operation, and the only thing on that list that means "take me in" is Enter
// on a row.
func (m AppModel) doDuplicate(id string) (tea.Model, tea.Cmd) {
	s := m.sessionByID(id)
	if s == nil {
		return m, m.closeStack()
	}
	return m.startSession(s.host, panelSessions)
}

func (m AppModel) doClose(id string) (tea.Model, tea.Cmd) {
	s := m.sessionByID(id)
	if s == nil {
		return m, m.closeStack()
	}
	s.pty.stop() // the reaper notices on the next tick and logs it
	m.ssh.setFocus(panelSessions)
	return m, tea.Batch(m.closeStack(), m.ssh.tick(),
		m.toast.show("Closed "+s.host.Name, toastInfo))
}

// doCloseAll kills every live session. Like doClose it only stops the
// processes — the reaper picks them up on the next tick, which is what logs
// each one with the reason it ended and takes its cell off the grid. Doing that
// work here as well would mean two paths that have to agree about what "ended"
// means.
func (m AppModel) doCloseAll() (tea.Model, tea.Cmd) {
	n := m.ssh.liveCount()
	if n == 0 {
		return m, m.closeStack()
	}
	m.ssh.stopAll()
	m.ssh.setFocus(panelSessions)
	return m, tea.Batch(m.closeStack(), m.ssh.tick(),
		m.toast.show("Closing "+plural(n, "session"), toastInfo))
}

func (m AppModel) sessionByID(id string) *session {
	for _, s := range m.ssh.sessions {
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
	case formPickCred:
		return m.openCredPicker()
	}
	m.syncFormError()
	return m, nil
}

// openCredPicker lists the saved credentials over the form. An empty list is
// still an answer: it says where credentials come from.
func (m AppModel) openCredPicker() (tea.Model, tea.Cmd) {
	items := []menuItem{{label: "use credential", header: true}}
	for _, c := range m.creds.creds {
		items = append(items, menuItem{label: c.Name, key: "@" + c.Name,
			hint: c.User + " · " + string(c.Auth)})
	}
	if len(m.creds.creds) == 0 {
		items = append(items,
			menuItem{label: "none saved yet — add them in manage → credentials", header: true})
	}
	m.credPicker.setItems(items, "credentials", m.form.layer+1)
	return m, m.credPicker.open()
}

// credPickerKey commits a choice into the form's Credential field.
func (m AppModel) credPickerKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var key string
	m.credPicker, key, _ = m.credPicker.update(msg)
	if key == "" {
		return m, nil
	}
	name := strings.TrimPrefix(key, "@")
	m.form.fields[fCredential].value = name
	m.form.fields[fCredential].caret = len([]rune(name))
	m.form.focus = fCredential
	m.syncFormError()
	return m, m.credPicker.close()
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
// Both forms use the one picker; whichever form is standing under it owns the
// answer.
func (m AppModel) pickerKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	path, done := m.picker.update(msg)
	if !done {
		return m, nil
	}
	if m.credFormUI.isActive() {
		m.credFormUI.fields[cIdentity].value = path
		m.credFormUI.fields[cIdentity].caret = len([]rune(path))
		m.credFormUI.focus = cIdentity
		m.syncCredFormError()
		return m, m.picker.close()
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
	verb := "added"
	if m.form.editing != "" {
		verb = "updated"
	}
	m.log.info(fmt.Sprintf("host %q %s (%s)", h.Name, verb, h.Addr()))
	return m, tea.Batch(m.closeStack(),
		m.toast.show(fmt.Sprintf("Saved %q", h.Name), toastInfo))
}

// validateForm checks the fields and says WHICH one is wrong, which is the part
// store.Validate cannot do — it validates the document, not a form. store stays
// the authority: SaveTo re-validates before it writes, so a rule added there is
// still enforced even if this layer misses it.
func (m AppModel) validateForm() (string, int) {
	name := strings.TrimSpace(m.form.fields[fName].value)
	credential := m.form.auth() == store.AuthCredential
	switch {
	case name == "":
		return "Name is required", fName
	case strings.TrimSpace(m.form.fields[fHost].value) == "":
		return "Host is required", fHost
	case !credential && strings.TrimSpace(m.form.fields[fUser].value) == "":
		return "User is required", fUser
	}
	if credential {
		cn := strings.TrimSpace(m.form.fields[fCredential].value)
		if cn == "" {
			return "Choose a credential (Enter opens the list)", fCredential
		}
		found := false
		for _, c := range m.creds.creds {
			if c.Name == cn {
				found = true
				break
			}
		}
		if !found {
			return fmt.Sprintf("No credential named %q — see manage → credentials", cn), fCredential
		}
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

// copyModeKey runs one keystroke of a pty cell's selection mode.
//
// Alt+v and Alt+Esc are the two ways out, and they are the same two ways that
// leave any other layer in this tab — Alt+Esc peels one layer at a time
// (§11.25), and selection mode is a layer. Everything else the mode either uses
// or swallows.
func (m AppModel) copyModeKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.Alt && (msg.Type == tea.KeyEscape ||
		(msg.Type == tea.KeyRunes && string(msg.Runes) == "v")) {
		m.ssh.copy.stop()
		return m, nil
	}
	text, yanked := m.ssh.copy.key(msg.String())
	if !yanked {
		return m, nil
	}
	return m, copyToClipboard(text)
}
