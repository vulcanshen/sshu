package ui

import (
	"fmt"
	"path"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/vulcanshen/sshu/internal/remote"
	"github.com/vulcanshen/sshu/internal/store"
)

// sftpConnectedMsg carries the result of a dial. Connecting is done off the
// update loop because a dial can take its 15-second timeout to fail, and a
// frozen UI is indistinguishable from a crashed one.
//
// gen is the dial's generation: an answer from a dial the user has already
// moved on from is dropped rather than landing on top of the newer one.
type sftpConnectedMsg struct {
	sd  side
	gen int
	fs  remote.FS
	err error
}

// sftpAction is one contextual action in tab [2]. Same table-is-the-declaration
// trick as the other tabs, with one addition: onMarks says whether the action
// belongs to a files panel, a marks panel, or both.
type sftpAction struct {
	key     string
	label   string
	hint    string
	onFiles bool
	onMarks bool
	// panelOp puts the action in the panel region of the Space menu: it acts on
	// this side as a whole (its marks, its host, its directory) rather than on
	// the row under the cursor. It is also what decides whether the action
	// survives an EMPTY listing — with no row, the row actions have no subject.
	panelOp bool
	run     func(AppModel) (tea.Model, tea.Cmd)
}

// keySelectHost is the one thing a side with no host can do, so it is named:
// appliesTo tests against it rather than against a position in the table.
const keySelectHost = "S"

// Every panel can start a transfer — that is the point of the tab: whatever you
// are looking at, the thing under the cursor can be sent across without first
// navigating somewhere else to say so.
//
// `t` and `T` are two actions, not one: the item under the cursor, or every mark
// on this side. They are the reason hotkey matching tries an exact case before
// it stops caring (hotkeyIndex) — everywhere else in the app either case works.
// Table order is menu order, so it is written in the two regions the menu shows:
// what happens to the row under the cursor, then what happens to this side.
//
// And the CASE says which: lower case acts on the row, upper case acts on the
// panel. This tab is the one that needs the distinction — it is the only place
// where both scopes exist side by side, with the same verb twice ([t]ransfer /
// [T]ransfer all marks, [x] / [X]). A reader has to be able to tell them apart
// without reading the hint column.
//
// `/` is outside the rule (not a letter) and so is Enter (a core key, whose name
// goes in the hint rather than in a bracket, §4.4).
var sftpActions = []sftpAction{
	// item — the row under the cursor
	{key: "enter", label: "Enter", hint: "Enter . open it, or go to a result", onFiles: true, run: AppModel.sftpEnter},
	{key: "m", label: "Mark", hint: "toggle", onFiles: true, run: AppModel.sftpToggleMark},
	// Unmark is the same `m`, and that is not two actions sharing a letter: on a
	// files panel `m` toggles the row's mark, and on a marks panel the row is
	// marked by definition, so the toggle can only take it off. One key, one
	// meaning — "un/mark this" — which is also what frees Unmark from `u`, the
	// one item letter navigation already owns.
	{key: "m", label: "Unmark", hint: "drop from marks", onMarks: true, run: AppModel.sftpUnmark},
	{key: "r", label: "Rename", hint: "this item, here", onFiles: true, onMarks: true, run: AppModel.sftpRename},
	{key: "v", label: "View", hint: "read this item, here", onFiles: true, onMarks: true, run: AppModel.sftpView},
	{key: "e", label: "Edit", hint: "open this item in $EDITOR", onFiles: true, onMarks: true, run: AppModel.sftpEdit},
	{key: "t", label: "Transfer", hint: "this item, to the other side", onFiles: true, onMarks: true, run: AppModel.sftpSendCursor},
	{key: "x", label: "Delete", hint: "this item, on this host", onFiles: true, onMarks: true, run: AppModel.sftpDeleteCursor},

	// panel — this side as a whole
	{key: "/", label: "Search", hint: "everything under here", onFiles: true, panelOp: true, run: AppModel.sftpStartFilter},
	{key: "A", label: "Add", hint: "a file, or name/ for a directory", onFiles: true, panelOp: true, run: AppModel.sftpAdd},
	{key: "T", label: "Transfer all marks", hint: "to the other side", onFiles: true, onMarks: true, panelOp: true, run: AppModel.sftpSendMarks},
	{key: "X", label: "Delete all marks", hint: "erase them, on this host", onFiles: true, onMarks: true, panelOp: true, run: AppModel.sftpDeleteMarks},
	{key: "C", label: "Clear marks", hint: "forget them, change nothing", onFiles: true, onMarks: true, panelOp: true, run: AppModel.sftpResetMarks},
	{key: keySelectHost, label: "Select host", hint: "this directory, or a saved host", onFiles: true, onMarks: true, panelOp: true, run: AppModel.sftpSwitchHost},
	{key: "P", label: "Progress", hint: "transfers, and cancel", onFiles: true, onMarks: true, panelOp: true, run: AppModel.sftpTransfers},
}

// Delete and Clear sit next to each other on purpose, and their hints say the
// difference in the only terms that matter: one erases files, the other only
// forgets which ones you had picked. They used to be one word apart ("Reset
// marks" / "Delete marks"), which is not enough distance for an irreversible
// action to stand beside a harmless one.

// appliesTo scopes an action to the focused panel.
//
// With no host on this side there is nothing to mark, send or reset — so the
// only thing offered, and the only letter that answers, is picking one. A menu
// full of rows that do nothing is worse than a short menu: it teaches that the
// menu does not mean what it says.
func (a sftpAction) appliesTo(p sftpPanel, hasHost, hasItem bool) bool {
	if !hasHost {
		return a.key == keySelectHost
	}
	// An empty listing, or a marks panel with nothing in it, has no row for a
	// row action to act on. Offering them anyway is the same lie as offering
	// Transfer with no host.
	if !hasItem && !a.panelOp {
		return false
	}
	if p.isMarks() {
		return a.onMarks
	}
	return a.onFiles
}

// sftpApplicable is the action set for the focused panel right now. Both the
// hotkey and the menu read it, so they cannot drift apart (§4.2).
func (m AppModel) sftpApplicable() ([]string, []sftpAction) {
	hasHost := m.sftp.cur().fs != nil
	_, hasItem := m.sftpCursorPath()
	var keys []string
	var acts []sftpAction
	for _, a := range sftpActions {
		if a.appliesTo(m.sftp.focus, hasHost, hasItem) {
			keys, acts = append(keys, a.key), append(acts, a)
		}
	}
	return keys, acts
}

func (m AppModel) sftpKey(k string) (tea.Model, tea.Cmd) {
	keys, acts := m.sftpApplicable()
	if i := hotkeyIndex(keys, k); i >= 0 {
		return acts[i].run(m)
	}
	m.sftp.handleListKey(k)
	return m, nil
}

// sftpMenuItems is tab [2]'s §A.1 contents, in two labelled regions: what
// happens to the row under the cursor, and what happens to this side.
//
// WHICH row is not repeated in the header — the cursor is on it, the popup's own
// title names the panel, and the labels are the family's ("item operation" /
// "panel operation", from kbu). Naming the row here was tried and dropped.
//
// A menu with only ONE region stays flat: a header over a single group is noise,
// and the no-host menu is one row that needs no explaining (kbu's rule).
func (m AppModel) sftpMenuItems() []menuItem {
	_, acts := m.sftpApplicable()

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

// ------------------------------------------------------------------ actions

func (m AppModel) sftpEnter() (tea.Model, tea.Cmd) {
	s := m.sftp.cur()
	s.enter()
	// Landing on a search result can put the cursor anywhere in the listing, and
	// a cursor below the fold is a cursor nobody can see.
	s.top = scrollTo(s.top, s.cursor, m.sftp.visibleRows(false))
	return m, m.closeStack()
}

func (m AppModel) sftpStartFilter() (tea.Model, tea.Cmd) {
	tick := m.sftp.cur().startFilter()
	return m, tea.Batch(m.closeStack(), tick)
}

func (m AppModel) sftpToggleMark() (tea.Model, tea.Cmd) {
	m.sftp.cur().toggleMark()
	return m, m.closeStack()
}

func (m AppModel) sftpUnmark() (tea.Model, tea.Cmd) {
	s := m.sftp.cur()
	if p, ok := s.markCursorPath(); ok {
		s.unmark(p)
	}
	return m, m.closeStack()
}

func (m AppModel) sftpResetMarks() (tea.Model, tea.Cmd) {
	m.sftp.cur().resetMarks()
	return m, m.closeStack()
}

// sftpSwitchHost opens the host picker for the focused side. "local" is first
// because uploading from this machine is the commonest thing anyone does here.
// It is the only action a side with no host offers, so it is also the first
// thing a new user meets in this tab.
func (m AppModel) sftpSwitchHost() (tea.Model, tea.Cmd) {
	items := []menuItem{{label: "connect to", header: true},
		{label: remote.LocalLabel, key: remote.LocalLabel, hint: "this directory"}}
	// The picker's own keys are names, not letters, so hotkeyIndex never fires
	// here — j/k and Enter are how it is driven.
	for _, h := range m.hosts.hosts {
		items = append(items, menuItem{label: h.Name, key: "@" + h.Name, hint: h.Addr()})
	}
	m.hostPicker.setItems(items, "switch host", m.layer())
	return m, m.hostPicker.open()
}

// hostPickerKey commits a choice from the host picker.
func (m AppModel) hostPickerKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var key string
	m.hostPicker, key, _ = m.hostPicker.update(msg)
	if key == "" {
		return m, nil
	}
	sd := m.sftp.focus.side()
	if key == remote.LocalLabel {
		m.sftp.sides[sd].connect(remote.Local())
		return m, tea.Batch(m.closeStack(), m.hostPicker.close(), m.sftp.startWatch())
	}

	name := key[1:] // the "@" prefix keeps a host called "local" from colliding
	i := indexOfHost(m.hosts.hosts, name)
	if i < 0 {
		return m, tea.Batch(m.hostPicker.close(), m.toast.show("No such host: "+name, toastError))
	}
	h, err := store.Resolve(m.hosts.hosts[i], m.creds.creds)
	if err != nil {
		m.log.errorf(err.Error())
		return m, tea.Batch(m.hostPicker.close(), m.toast.show(err.Error(), toastError))
	}
	dial := m.sftp.startDial(sd, h)
	return m, tea.Batch(m.closeStack(), m.hostPicker.close(), dial)
}

// dialCmd connects off the update loop.
func dialCmd(sd side, h store.Host, gen int, timeout time.Duration) tea.Cmd {
	return func() tea.Msg {
		// An unknown host is refused rather than waved through. Accepting a key
		// needs a dialog mid-dial, which this path cannot raise yet — so it says
		// how to accept it deliberately instead of pretending it does not matter.
		fsys, err := remote.Dial(h, nil, timeout)
		return sftpConnectedMsg{sd: sd, gen: gen, fs: fsys, err: err}
	}
}

func (m AppModel) sftpConnected(msg sftpConnectedMsg) (tea.Model, tea.Cmd) {
	s := &m.sftp.sides[msg.sd]
	if msg.gen != s.dialGen {
		// The user picked another host while this one was still dialling. Close
		// what arrived — nobody asked for it any more — and say nothing.
		if msg.fs != nil {
			msg.fs.Close()
		}
		return m, nil
	}
	name := s.dialing
	s.dialing = ""

	if msg.err != nil {
		s.fs, s.host, s.err = nil, "", msg.err.Error()
		m.log.errorf("sftp: " + name + " · " + msg.err.Error())
		return m, m.toast.show(msg.err.Error(), toastError)
	}
	s.connect(msg.fs)
	m.log.info("sftp: connected to " + msg.fs.Label())
	// A side that has just connected is something to keep current.
	return m, m.sftp.startWatch()
}

// ---------------------------------------------------------------- transfers

// sftpSendCursor and sftpSendMarks are the two transfer entry points named in
// the design: one item from wherever you are, or everything this side has
// marked. Both land in the OTHER side's current directory.
func (m AppModel) sftpSendCursor() (tea.Model, tea.Cmd) {
	p, ok := m.sftpCursorPath()
	if !ok {
		return m, nil
	}
	return m.sftpQueue([]string{p})
}

func (m AppModel) sftpSendMarks() (tea.Model, tea.Cmd) {
	s := m.sftp.cur()
	if len(s.marks) == 0 {
		return m, m.toast.show("Nothing marked on this side", toastError)
	}
	return m.sftpQueue(append([]string(nil), s.marks...))
}

// sftpQueue plans a transfer and asks about overwrites before starting one.
//
// The plan runs first, in full: how many files, how many bytes, and which of
// them already exist at the far end. Asking mid-copy would mean putting the
// question after half the batch is already committed, which is not a question
// any more.
func (m AppModel) sftpQueue(paths []string) (tea.Model, tea.Cmd) {
	src, dst := m.sftp.cur(), m.sftp.other()
	if dst.fs == nil {
		return m, tea.Batch(m.closeStack(),
			m.toast.show("The other side has no host yet", toastError))
	}
	for _, p := range paths {
		if remote.SameTree(src.fs, p, dst.fs, dst.cwd) {
			return m, tea.Batch(m.closeStack(),
				m.toast.show("That would copy a directory into itself", toastError))
		}
	}

	items, total, err := remote.Plan(src.fs, paths, dst.cwd)
	if err != nil {
		return m, tea.Batch(m.closeStack(), m.toast.show(err.Error(), toastError))
	}
	files := 0
	for _, it := range items {
		if !it.IsDir {
			files++
		}
	}
	if files == 0 {
		return m, tea.Batch(m.closeStack(), m.toast.show("Nothing to transfer", toastError))
	}

	m.pending = pendingTransfer{
		items: items, total: total, files: files,
		label: fmt.Sprintf("%s → %s:%s", plural(files, "file"), dst.host,
			fitPath(foldHomePath(dst.cwd, dst.home), 28)),
		srcSide: m.sftp.focus.side(),
	}

	if n := len(remote.Conflicts(dst.fs, items)); n > 0 {
		return m, m.confirm.ask(confirmPopup{
			glyph: glyphWarn,
			title: "Overwrite",
			lines: []string{
				fmt.Sprintf("%s, %d will overwrite.", plural(files, "file"), n),
				"Transfer to " + dst.host + ":" + dst.cwd + "?",
			},
			accept: "transfer",
			warn:   true,
			action: confirmTransfer,
		}, m.layer())
	}
	return m.startTransfer()
}

// startTransfer commits the pending plan.
func (m AppModel) startTransfer() (tea.Model, tea.Cmd) {
	p := m.pending
	if len(p.items) == 0 {
		return m, m.closeStack()
	}
	src := &m.sftp.sides[p.srcSide]
	dst := &m.sftp.sides[1-p.srcSide]
	m.pending = pendingTransfer{}

	cmd := m.transfers.start(src.fs, dst.fs, p.items, p.total, p.label)
	return m, tea.Batch(m.closeStack(), cmd)
}

// pendingTransfer holds a planned job across the overwrite confirmation.
type pendingTransfer struct {
	items   []remote.Item
	total   int64
	files   int
	label   string
	srcSide side
}

// ------------------------------------------------------------ destructive

// sftpRename renames the item under the cursor, in place. It is the one action
// here that changes a name rather than a location, so it asks for the new one
// pre-filled with the old: most renames edit part of a name.
func (m AppModel) sftpRename() (tea.Model, tea.Cmd) {
	p, ok := m.sftpCursorPath()
	if !ok {
		return m, m.toast.show("Nothing under the cursor", toastError)
	}
	return m, m.input.ask(inputPopup{
		glyph:   glyphPencil,
		title:   "Rename",
		prompt:  "New name for " + path.Base(p),
		value:   path.Base(p),
		accept:  "rename",
		action:  inputRename,
		subject: p,
	}, m.layer())
}

// sftpView opens the item under the cursor for reading — filu's preview, in a
// popup, over whichever filesystem this side is pointed at.
//
// The popup opens FIRST and loads behind itself: a remote read takes as long as
// the link takes, and a key that shows nothing until the bytes arrive looks like
// a key that did nothing.
func (m AppModel) sftpView() (tea.Model, tea.Cmd) {
	s := m.sftp.cur()
	p, ok := m.sftpCursorPath()
	if !ok || s.fs == nil {
		return m, m.toast.show("Nothing under the cursor", toastError)
	}
	isDir := false
	if e, ok := m.sftp.cur().rowAt(m.sftp.cur().cursor); ok && !m.sftp.focus.isMarks() {
		isDir = e.IsDir
	} else if e, err := s.fs.Lstat(p); err == nil {
		isDir = e.IsDir
	}

	open := m.viewer.open(m.layer(), path.Base(p))
	return m, tea.Batch(open, loadView(m.viewer.gen, s.fs, p, isDir))
}

// sftpEdit opens the item under the cursor in $EDITOR.
//
// Stat, not Lstat: a symlink to a config file is a config file, and opening it
// is what any editor would do. The write-back is the half that has to know the
// difference, and it takes the question up again there.
func (m AppModel) sftpEdit() (tea.Model, tea.Cmd) {
	s := m.sftp.cur()
	p, ok := m.sftpCursorPath()
	if !ok || s.fs == nil {
		return m, m.toast.show("Nothing under the cursor", toastError)
	}
	// One editor at a time. Two of them writing back in an order nobody can
	// predict is not a feature. (kbu makes the same refusal, for the same
	// reason.)
	if m.editorUI.isActive() {
		return m, m.toast.show("An editor is already open", toastError)
	}
	e, err := s.fs.Stat(p)
	if err != nil {
		return m, m.toast.show("Cannot read "+path.Base(p), toastError)
	}
	if e.IsDir {
		return m, m.toast.show("Cannot edit a directory", toastError)
	}
	// A device, socket or fifo has no contents to read and put back; read-modify
	// -write on one means something else entirely.
	if !e.Mode.IsRegular() {
		return m, m.toast.show("Not a regular file", toastError)
	}
	return m.startEdit(editJob{
		fsys:  s.fs,
		path:  p,
		mode:  e.Mode.Perm(),
		stamp: remote.Stamp{Size: e.Size, ModTime: e.ModTime},
	}, e.Size)
}

// sftpAdd makes something in the directory being browsed. It is a panel action,
// not an item one — the cursor has nothing to do with where it lands, which is
// also why it is upper case (§7.3.2).
//
// One box makes both kinds, and the TRAILING SLASH says which. That is not a new
// convention: it is how a shell has written directories forever, and how this
// app's own listings draw them. Two keys for "make a file" and "make a
// directory" would have been two hotkeys and two menu rows to say one thing.
func (m AppModel) sftpAdd() (tea.Model, tea.Cmd) {
	s := m.sftp.cur()
	if s.fs == nil {
		return m, nil
	}
	return m, m.input.ask(inputPopup{
		glyph:       glyphPlus,
		title:       "Add",
		prompt:      "In " + fitPath(foldHomePath(s.cwd, s.home), 44),
		placeholder: "name, or name/ for a directory",
		accept:      "create",
		action:      inputAdd,
	}, m.layer())
}

// doAdd is the committed half. Same three refusals as a rename — empty, a
// separator, or a name that is already taken — because they are the same three
// ways to mean something other than what you typed.
//
// Only the LAST slash is the type marker. One anywhere else makes it a path, and
// a path typed into a name box is a mistake far more often than it is an
// intention: `a/b/` would have to silently make two directories, and silence is
// the wrong answer to "did you mean this".
func (m AppModel) doAdd(name string) (tea.Model, tea.Cmd) {
	s := m.sftp.cur()
	name = strings.TrimSpace(name)
	if s.fs == nil || name == "" {
		return m, tea.Batch(m.closeStack(), m.input.close())
	}
	isDir := strings.HasSuffix(name, "/")
	name = strings.TrimSuffix(name, "/")
	if name == "" || strings.ContainsRune(name, '/') {
		return m, tea.Batch(m.closeStack(), m.input.close(),
			m.toast.show("A name cannot contain /", toastError))
	}

	p := remote.Join(s.cwd, name)
	// Checked first, because Create TRUNCATES: without this, adding a name that
	// is already there would empty the file instead of refusing.
	if remote.Exists(s.fs, p) {
		return m, tea.Batch(m.closeStack(), m.input.close(),
			m.toast.show(name+" already exists", toastError))
	}
	if err := addItem(s.fs, p, isDir); err != nil {
		return m, tea.Batch(m.closeStack(), m.input.close(),
			m.toast.show(err.Error(), toastError))
	}

	s.reload()
	// Land the cursor on what was just made. Making something is almost always
	// the first half of doing something with it, and hunting for it in a long
	// listing is the second half nobody asked for.
	for i := 0; i < s.rowCount(); i++ {
		if e, ok := s.rowAt(i); ok && e.Name == name {
			s.cursor = i
			break
		}
	}
	kind := "file"
	if isDir {
		kind = "directory"
	}
	return m, tea.Batch(m.closeStack(), m.input.close(),
		m.toast.show("Created "+kind+" "+name, toastInfo))
}

// addItem makes the empty thing. 0644 is what `touch` gives, and the local end
// applies the umask to it the way any other create does.
func addItem(fsys remote.FS, p string, isDir bool) error {
	if isDir {
		return fsys.MkdirAll(p)
	}
	w, err := fsys.Create(p, 0o644)
	if err != nil {
		return err
	}
	return w.Close()
}

// doRename is the committed half. The destination is checked first: os.Rename
// would overwrite it and SFTP's would refuse, and one action must not depend on
// which end of the transfer it happens to be running on.
func (m AppModel) doRename(subject, name string) (tea.Model, tea.Cmd) {
	s := m.sftp.cur()
	if s.fs == nil {
		return m, m.closeStack()
	}
	name = strings.TrimSpace(name)
	if name == "" || name == path.Base(subject) {
		return m, tea.Batch(m.closeStack(), m.input.close())
	}
	if strings.ContainsRune(name, '/') {
		return m, tea.Batch(m.closeStack(), m.input.close(),
			m.toast.show("A name cannot contain /", toastError))
	}

	dst := remote.Join(path.Dir(subject), name)
	if remote.Exists(s.fs, dst) {
		return m, tea.Batch(m.closeStack(), m.input.close(),
			m.toast.show(name+" already exists", toastError))
	}
	if err := s.fs.Rename(subject, dst); err != nil {
		return m, tea.Batch(m.closeStack(), m.input.close(),
			m.toast.show(err.Error(), toastError))
	}
	// A mark is a path, so a renamed mark is a mark on something that is no
	// longer there. Move it with the file rather than leaving it dangling.
	if s.markedSet[subject] {
		s.unmark(subject)
		s.markedSet[dst] = true
		s.marks = append(s.marks, dst)
	}
	s.reload()
	return m, tea.Batch(m.closeStack(), m.input.close(),
		m.toast.show("Renamed to "+name, toastInfo))
}

// sftpDeleteCursor erases the one thing under the cursor. It is `x` to
// Delete all marks' `X`, the same lower/upper split as [t]ransfer and
// [T]ransfer all marks: this one, or every mark.
//
// The pair is on `x`, not on `d`, because `d` is half-page-down. Delete did sit
// on `d` briefly and it cost this tab its half-page — and, worse, it put Delete's
// scope on a different mechanism from Transfer's. The alternative considered was
// letting the PANEL pick the scope (D on a files panel = this item, D on a marks
// panel = all marks), which breaks on the marks panel: that panel has its own
// cursor, so it cannot stand for "all marks" without leaving no way to delete
// just one of them — and a mark found by a subtree search is not reachable from
// the file list to try again.
func (m AppModel) sftpDeleteCursor() (tea.Model, tea.Cmd) {
	s := m.sftp.cur()
	p, ok := m.sftpCursorPath()
	if !ok {
		return m, m.toast.show("Nothing under the cursor", toastError)
	}
	return m, m.confirm.ask(confirmPopup{
		glyph:  glyphWarn,
		title:  "Delete",
		lines:  []string{path.Base(p) + " on " + s.host + ".", "Delete permanently?"},
		accept: "delete",
		warn:   true,
		target: p,
		action: confirmDeleteItem,
	}, m.layer())
}

// doDeleteItem is the committed half.
func (m AppModel) doDeleteItem(p string) (tea.Model, tea.Cmd) {
	s := m.sftp.cur()
	if s.fs == nil || p == "" {
		return m, m.closeStack()
	}
	if err := remote.RemoveAll(s.fs, p); err != nil {
		return m, tea.Batch(m.closeStack(), m.toast.show(err.Error(), toastError))
	}
	// A mark on something that no longer exists is a mark that fails later, in a
	// place where the reason is no longer on screen.
	s.unmark(p)
	s.reload()
	return m, tea.Batch(m.closeStack(),
		m.toast.show("Deleted "+path.Base(p), toastInfo))
}

// sftpDeleteMarks erases every marked path on this side. It is the only action
// in the app that destroys data on a remote machine, so it asks first and the
// question says the count and the host — "3 files" is not enough when both
// sides look alike.
func (m AppModel) sftpDeleteMarks() (tea.Model, tea.Cmd) {
	s := m.sftp.cur()
	if len(s.marks) == 0 {
		return m, m.toast.show("Nothing marked on this side", toastError)
	}
	return m, m.confirm.ask(confirmPopup{
		glyph: glyphWarn,
		title: "Delete",
		lines: []string{
			plural(len(s.marks), "marked item") + " on " + s.host + ".",
			"Delete permanently?",
		},
		accept: "delete",
		warn:   true,
		action: confirmDeleteMarks,
	}, m.layer())
}

// doDeleteMarks is the committed half. It deletes what it can and reports the
// first failure rather than stopping at it: a batch that half-succeeded should
// leave the list showing what actually remains.
func (m AppModel) doDeleteMarks() (tea.Model, tea.Cmd) {
	s := m.sftp.cur()
	if s.fs == nil || len(s.marks) == 0 {
		return m, m.closeStack()
	}
	marks := append([]string(nil), s.marks...)
	failed := ""
	done := 0
	for _, p := range marks {
		if err := remote.RemoveAll(s.fs, p); err != nil {
			if failed == "" {
				failed = err.Error()
			}
			continue
		}
		s.unmark(p)
		done++
	}
	s.reload()

	if failed != "" {
		return m, tea.Batch(m.closeStack(), m.toast.show(failed, toastError))
	}
	return m, tea.Batch(m.closeStack(),
		m.toast.show("Deleted "+plural(done, "item"), toastInfo))
}

// sftpCursorPath is the path under the cursor, whichever kind of panel is
// focused. Both Rename and Transfer need it, and they must agree on it.
func (m AppModel) sftpCursorPath() (string, bool) {
	s := m.sftp.cur()
	if m.sftp.focus.isMarks() {
		return s.markCursorPath()
	}
	return s.cursorPath()
}

// sftpTransfers opens the detail view.
func (m AppModel) sftpTransfers() (tea.Model, tea.Cmd) {
	return m, m.transfersUI.open(m.layer())
}
