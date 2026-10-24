package tui

import (
	"context"
	"fmt"
	"io/fs"
	"os/exec"
	"path"
	"slices"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	terminu "github.com/vulcanshen/terminu-go"

	"github.com/vulcanshen/sshu/internal/remote"
	"github.com/vulcanshen/sshu/internal/store"
)

// The File transfer screen: two sides, each local or a saved host, each a
// file list over its own marks. [1] left files, [2] left marks, [3] right
// files, [4] right marks. h|l cross to the same panel on the other side
// (tdp K12: a left-right structure). Upload, download and remote to remote
// are one operation: send to the other side's directory.

type transfer struct {
	s      *sshu
	screen *terminu.Screen
	sides  [2]*side
	jobs   []*job
	nextID int

	watchGen int
	ticking  bool // the progress tick is running
	scanning bool // a search tick is running
	scans    []*scan
	editing  bool // an editor is open: one at a time
}

// side is one half: a filesystem, where it is looking, what is marked there.
type side struct {
	t   *transfer
	idx int

	fs        remote.FS
	host      string
	cwd, home string
	entries   []remote.Entry
	err       string

	marks   []string        // absolute paths on this side, in the order marked
	markDir map[string]bool // which marks are directories

	files, marked   *terminu.List
	filesP, markedP *terminu.Panel

	dialing       string
	dialGen       int
	askpass       *askpassServer
	dialCancel    context.CancelFunc
	dialCancelled bool

	watchMTime time.Time
	probing    bool
}

func newTransfer(s *sshu) *transfer {
	t := &transfer{s: s}
	for i := range t.sides {
		t.sides[i] = newSide(t, i)
	}
	l, r := t.sides[0], t.sides[1]
	t.screen = &terminu.Screen{Name: "File transfer", Key: "F", Desc: "copy between any two machines",
		Panels:      []*terminu.Panel{l.filesP, l.markedP, r.filesP, r.markedP},
		NarrowBelow: 72,
		Layout: func(w, h int) []terminu.Rect {
			half := w / 2
			files := max(3, h*2/3)
			if h-files < 3 {
				files = h
			}
			marks := h - files
			return []terminu.Rect{
				{X: 0, Y: 0, W: half, H: files}, {X: 0, Y: files, W: half, H: marks},
				{X: half, Y: 0, W: w - half, H: files}, {X: half, Y: files, W: w - half, H: marks},
			}
		}}
	return t
}

func newSide(t *transfer, i int) *side {
	sd := &side{t: t, idx: i}
	sd.files = &terminu.List{Columns: []terminu.Column{{Title: "Name", Path: true, Shrink: true}, {Title: "Size", Number: true, Drop: 1}},
		Marks: true, EmptyFact: "No host", EmptyHint: "Press [H] to pick a host, or local"}
	sd.marked = &terminu.List{Columns: []terminu.Column{{Path: true, Shrink: true}},
		EmptyFact: "Nothing marked", EmptyHint: "Press [a] on a file to mark it"}
	sd.filesP = &terminu.Panel{Label: "No host", Body: &sideBody{sd: sd, list: sd.files, cwd: true}}
	sd.markedP = &terminu.Panel{Label: "Marks", Body: &sideBody{sd: sd, list: sd.marked}}
	sd.filesP.Actions = sd.fileActions()
	sd.markedP.Actions = sd.markActions()
	return sd
}

func (t *transfer) other(sd *side) *side { return t.sides[1-sd.idx] }

// start begins watching the sides' directories.
func (t *transfer) start() tea.Cmd { return nil }

// --- the panel bodies -------------------------------------------------------------

// sideBody is a side's list with h|l crossing sides. The files list's
// location row (List.Location) says which directory it is: the two sides may
// be one machine's two directories, or two machines' same path.
type sideBody struct {
	sd   *side
	list *terminu.List
	cwd  bool // the files list, not the marks
}

func (b *sideBody) Draw(w, h int, focused bool) []string { return b.list.Draw(w, h, focused) }

func (b *sideBody) Key(key string, msg tea.KeyMsg) (bool, tea.Cmd) {
	switch key {
	case "h", "←":
		return true, b.sd.t.cross(b, 0)
	case "l", "→":
		return true, b.sd.t.cross(b, 1)
	}
	return b.list.Key(key, msg)
}

func (b *sideBody) HasItem() bool  { return b.list.HasItem() }
func (b *sideBody) Scroll() string { return b.list.Scroll() }
func (b *sideBody) Empty() bool    { return b.list.Empty() }
func (b *sideBody) Focus()         { b.list.Focus() }
func (b *sideBody) MoveKeys() []terminu.KeyRow {
	return append(b.list.MoveKeys(), terminu.KeyRow{Key: "h|l", Desc: "the left side, the right side"})
}

// cross moves the focus to the same panel of side to (tdp K12, the user's
// h|l).
func (t *transfer) cross(b *sideBody, to int) tea.Cmd {
	if b.cwd {
		return t.s.a.Focus(t.sides[to].filesP)
	}
	return t.s.a.Focus(t.sides[to].markedP)
}

// --- actions ------------------------------------------------------------------------

func (sd *side) hasHost() bool { return sd.fs != nil }
func (sd *side) noHost() bool  { return sd.fs == nil }
func (sd *side) noMarks() bool { return sd.fs == nil || len(sd.marks) == 0 }

// fileActions are a files panel's: with no host, only picking one. What
// acts on the marks lives on the marks panel (tdp P3: an action about
// another panel is that panel's), the jobs are a global operation. An action
// that opens a popup keeps the Space menu under it (tdp T1, the lib's way);
// the search clears it, as going into a directory does.
func (sd *side) fileActions() []terminu.Action {
	t := sd.t
	// Enter goes into a directory, which clears the Space menu (a
	// drill-down), and reads a file, which keeps it (tdp T1): two actions,
	// one shown at a time.
	onDir := func() bool { e, ok := sd.cursorEntry(); return ok && e.IsDir }
	return []terminu.Action{
		{Key: "Enter", Label: "Open", Desc: "go into this directory", Scope: terminu.ItemScope, Hint: "open",
			Hidden: func() bool { return sd.noHost() || !onDir() }, Do: sd.open},
		{Key: "Enter", Label: "Read", Desc: "this file, here", Scope: terminu.ItemScope, Hint: "read",
			Hidden: func() bool { return sd.noHost() || onDir() }, Do: sd.open},
		{Key: "a", Label: "Append to marks", Desc: "again takes it off", Scope: terminu.ItemScope, Hint: "mark", Hidden: sd.noHost,
			Disabled: func() bool { p, ok := sd.cursorPath(); return ok && t.receiving(p) }, Do: sd.toggleMark},
		{Key: "r", Label: "Rename", Desc: "this item, here", Scope: terminu.ItemScope, Hidden: sd.noHost,
			Do: func() tea.Cmd { return sd.rename(sd.cursorPath()) }},
		{Key: "v", Label: "View", Desc: "read this item, here", Scope: terminu.ItemScope, Hidden: sd.noHost,
			Do: func() tea.Cmd { return sd.view(sd.cursorPath()) }},
		// The editor opens once the file is fetched: Stay keeps the menu under
		// it meanwhile (tdp T1: an editor on the item keeps its source).
		{Key: "e", Label: "Edit", Desc: "open this item in $EDITOR", Scope: terminu.ItemScope, Hidden: sd.noHost, Stay: true,
			Disabled: func() bool {
				e, ok := sd.cursorEntry()
				return ok && (e.IsDir || !e.Mode.IsRegular() && e.Mode&fs.ModeSymlink == 0)
			},
			Do: func() tea.Cmd { return sd.edit(sd.cursorPath()) }},
		asked(terminu.Action{Key: "t", Label: "Transfer", Desc: "this item, to the other side", Scope: terminu.ItemScope, Hint: "transfer",
			Hidden: sd.noHost, Disabled: func() bool { return t.other(sd).noHost() }}, func() (*terminu.Ask, func() tea.Cmd) {
			if p, ok := sd.cursorPath(); ok {
				return sd.queue([]string{p})
			}
			return nil, nil
		}),
		gated(terminu.Action{Key: "x", Label: "Delete", Desc: "this item, on this host", Scope: terminu.ItemScope, Hidden: sd.noHost},
			func() (terminu.Ask, func() tea.Cmd, bool) { return sd.deleteAsk(sd.cursorPath()) }),
		{Key: "/", Label: "Search", Desc: "everything under here", Scope: terminu.PanelScope, Hidden: sd.noHost, Clear: true, Do: sd.search},
		{Key: "A", Label: "Add", Desc: "a file, or name/ for a directory", Scope: terminu.PanelScope, Hidden: sd.noHost, Do: sd.add},
		{Key: "R", Label: "Refresh", Desc: "re-read this directory now", Scope: terminu.PanelScope, Hidden: sd.noHost, Do: sd.refresh},
		{Key: "H", Label: "Host", Desc: "switch this side: this machine, or a saved host", Scope: terminu.PanelScope, Hint: "host",
			Disabled: func() bool { return t.running() > 0 }, Do: sd.pickHost},
		{Key: "D", Label: "Disconnect", Desc: "close it, back to no host", Scope: terminu.PanelScope,
			Hidden:   func() bool { return sd.noHost() && sd.dialing == "" },
			Disabled: func() bool { return t.running() > 0 }, Do: sd.disconnectCmd},
	}
}

// markActions are a marks panel's: the same verbs on the mark under the
// cursor, and what acts on all of this side's marks.
func (sd *side) markActions() []terminu.Action {
	t := sd.t
	onDir := func() bool { p, ok := sd.markPath(); return ok && sd.markDir[p] }
	return []terminu.Action{
		{Key: "Enter", Label: "Open", Desc: "go to this directory", Scope: terminu.ItemScope, Hint: "open",
			Hidden: func() bool { return !onDir() }, Do: sd.openMark},
		{Key: "Enter", Label: "Read", Desc: "this file, here", Scope: terminu.ItemScope, Hint: "read",
			Hidden: onDir, Do: sd.openMark},
		{Key: "c", Label: "Clear mark", Desc: "forget it, change nothing", Scope: terminu.ItemScope, Hint: "clear", Do: sd.unmarkCursor},
		{Key: "r", Label: "Rename", Desc: "this item, here", Scope: terminu.ItemScope, Do: func() tea.Cmd { return sd.rename(sd.markPath()) }},
		{Key: "v", Label: "View", Desc: "read this item, here", Scope: terminu.ItemScope, Do: func() tea.Cmd { return sd.view(sd.markPath()) }},
		{Key: "e", Label: "Edit", Desc: "open this item in $EDITOR", Scope: terminu.ItemScope, Stay: true,
			Disabled: func() bool { p, ok := sd.markPath(); return ok && sd.markDir[p] },
			Do:       func() tea.Cmd { return sd.edit(sd.markPath()) }},
		asked(terminu.Action{Key: "t", Label: "Transfer", Desc: "this item, to the other side", Scope: terminu.ItemScope, Hint: "transfer",
			Disabled: func() bool { return t.other(sd).noHost() }}, func() (*terminu.Ask, func() tea.Cmd) {
			if p, ok := sd.markPath(); ok {
				return sd.queue([]string{p})
			}
			return nil, nil
		}),
		gated(terminu.Action{Key: "x", Label: "Delete", Desc: "this item, on this host", Scope: terminu.ItemScope},
			func() (terminu.Ask, func() tea.Cmd, bool) { return sd.deleteAsk(sd.markPath()) }),
		asked(terminu.Action{Key: "T", Label: "Transfer all marks", Desc: "to the other side", Scope: terminu.PanelScope, Hidden: sd.noMarks,
			Disabled: func() bool { return t.other(sd).noHost() }}, func() (*terminu.Ask, func() tea.Cmd) {
			return sd.queue(append([]string(nil), sd.marks...))
		}),
		gated(terminu.Action{Key: "X", Label: "Delete all marks", Desc: "erase them, on this host", Scope: terminu.PanelScope, Hidden: sd.noMarks},
			sd.deleteMarksAsk),
		{Key: "C", Label: "Clear marks", Desc: "forget them, change nothing", Scope: terminu.PanelScope, Hidden: sd.noMarks, Do: sd.clearMarks},
	}
}

// openMark is Enter on a mark: a directory opens in the files panel, a file
// is read — what Enter does to them there (tdp K3).
func (sd *side) openMark() tea.Cmd {
	p, ok := sd.markPath()
	if !ok {
		return nil
	}
	if sd.markDir[p] {
		return sd.goTo(p, "")
	}
	return sd.view(p, true)
}

// --- what a side shows ----------------------------------------------------------------

func (sd *side) cursorEntry() (remote.Entry, bool) {
	r, ok := sd.files.Current()
	if !ok {
		return remote.Entry{}, false
	}
	for _, e := range sd.entries {
		if remote.Join(sd.cwd, e.Name) == r.ID {
			return e, true
		}
	}
	return remote.Entry{}, false
}

func (sd *side) cursorPath() (string, bool) {
	r, ok := sd.files.Current()
	return r.ID, ok && sd.fs != nil
}

func (sd *side) markPath() (string, bool) {
	r, ok := sd.marked.Current()
	return r.ID, ok && sd.fs != nil
}

// redraw puts the side's state into its two lists and its panel labels. The
// host's name is the files panel's whole label, kept as it is
// (components, upper case).
func (sd *side) redraw() {
	sd.filesP.Label, sd.filesP.Name = "", sd.host
	if sd.host == "" {
		sd.filesP.Label = "No host"
	}
	sd.files.Location = ""
	if sd.fs != nil {
		sd.files.Location = foldHomePath(sd.cwd, sd.home)
	}
	sd.filesP.OnEsc = nil
	if sd.fs != nil && remote.Parent(sd.cwd) != sd.cwd {
		sd.filesP.OnEsc = sd.upCmd
	}
	sd.filesP.Loading = sd.dialing != ""
	sd.filesP.LoadingText = "connecting to " + sd.dialing
	sd.filesP.Fail = nil
	switch {
	case sd.fs == nil:
		sd.files.EmptyFact, sd.files.EmptyHint = "No host", "Press [H] to pick a host, or local"
		if sd.err != "" {
			sd.filesP.Fail = &terminu.Failure{Fact: sd.err, Hint: "Press [H] to try again, or pick another host"}
		}
	case sd.err != "" && len(sd.entries) == 0:
		sd.filesP.Fail = &terminu.Failure{Fact: sd.err, Hint: "Press [R] to read it again, or [Esc] to go up"}
	default:
		sd.files.EmptyFact, sd.files.EmptyHint = "Empty directory", "Press [A] to add a file or directory, or [H] to switch host"
	}
	arriving := sd.t.arrivalFrame()
	rows := make([]terminu.Row, len(sd.entries))
	for i, e := range sd.entries {
		p := remote.Join(sd.cwd, e.Name)
		glyph, color, size := glyphFile, terminu.Overlay0, humanSize(e.Size)
		if e.IsDir {
			glyph, color, size = glyphDir, terminu.Blue, "-"
		}
		if arriving != "" && sd.t.receiving(p) {
			glyph, color = arriving, terminu.Green
		}
		rows[i] = terminu.Row{ID: p, Cells: []string{glyph + " " + e.Name, size},
			Styled: [][]terminu.Span{{{Text: glyph, Style: terminu.Fg(color)}, {Text: " " + e.Name}}}}
	}
	sd.files.SetRows(rows)
	sd.syncMarks()
	mrows := make([]terminu.Row, len(sd.marks))
	for i, p := range sd.marks {
		label := p
		if rel, ok := strings.CutPrefix(p, sd.cwd+"/"); ok && sd.cwd != "" {
			label = rel
		}
		mrows[i] = terminu.Row{ID: p, Cells: []string{label}}
	}
	sd.marked.SetRows(mrows)
}

// syncMarks makes the files list show this side's marks: the list keeps
// marks by row, sshu keeps them by path, across directories.
func (sd *side) syncMarks() { sd.files.SetMarked(sd.marks) }

// --- moving around --------------------------------------------------------------------

// list reads dir; a failure leaves the old listing with the reason beside it.
func (sd *side) list(dir string) error {
	mtime := dirMTime(sd.fs, dir)
	entries, err := sd.fs.List(dir)
	if err != nil {
		sd.err = err.Error()
		return err
	}
	sd.cwd, sd.entries, sd.err, sd.watchMTime = dir, entries, "", mtime
	return nil
}

// go_ opens dir with the cursor at its top, or on sel.
func (sd *side) goTo(dir, sel string) tea.Cmd {
	if err := sd.list(dir); err != nil {
		sd.redraw()
		sd.t.s.logError(store.LevelError, "Can't read "+dir+" on "+sd.host, err.Error())
		return sd.t.s.a.Toast(terminu.Error, "Can't read "+dir+": "+why(err))
	}
	sd.redraw()
	if sel != "" {
		sd.files.Select(remote.Join(dir, sel))
	} else if rows := sd.files.Rows(); len(rows) > 0 {
		sd.files.Select(rows[0].ID)
	}
	return nil
}

// open is Enter: into a directory; a file is read.
func (sd *side) open() tea.Cmd {
	e, ok := sd.cursorEntry()
	if !ok {
		return nil
	}
	p := remote.Join(sd.cwd, e.Name)
	if e.IsDir {
		return sd.goTo(p, "")
	}
	return sd.view(p, true)
}

// upCmd is Esc on a files panel: one directory up (tdp K4, the app's "one
// level up"); nothing at the root.
func (sd *side) upCmd() tea.Cmd {
	if sd.fs == nil {
		return nil
	}
	parent := remote.Parent(sd.cwd)
	if parent == sd.cwd {
		return nil
	}
	return sd.goTo(parent, path.Base(sd.cwd))
}

func (sd *side) refresh() tea.Cmd {
	if err := sd.list(sd.cwd); err != nil {
		sd.redraw()
		sd.t.s.logError(store.LevelError, "Can't read "+sd.cwd+" on "+sd.host, err.Error())
		return sd.t.s.a.Toast(terminu.Error, "Can't read "+sd.cwd+": "+why(err))
	}
	sd.redraw()
	return sd.t.s.a.Toast(terminu.Success, "Refreshed: "+plural(len(sd.entries), "item"))
}

// reload re-reads the directory in place, the cursor on the same entry: an
// arrival shows up without moving anybody.
func (sd *side) reload() {
	if sd.fs == nil {
		return
	}
	_ = sd.list(sd.cwd)
	sd.redraw()
}

// --- marks ------------------------------------------------------------------------------

func (sd *side) isMarked(p string) bool { return slices.Contains(sd.marks, p) }

func (sd *side) toggleMark() tea.Cmd {
	p, ok := sd.cursorPath()
	if !ok {
		return nil
	}
	if sd.t.receiving(p) {
		return sd.t.s.a.Toast(terminu.Error, "Still arriving: not all of it is here yet")
	}
	if sd.isMarked(p) {
		sd.unmark(p)
	} else {
		sd.marks = append(sd.marks, p)
		e, _ := sd.cursorEntry()
		if sd.markDir == nil {
			sd.markDir = map[string]bool{}
		}
		sd.markDir[p] = e.IsDir
	}
	sd.redraw()
	sd.refreshActions()
	return nil
}

func (sd *side) unmark(p string) {
	sd.marks = slices.DeleteFunc(sd.marks, func(m string) bool { return m == p })
}

func (sd *side) unmarkCursor() tea.Cmd {
	if p, ok := sd.markPath(); ok {
		sd.unmark(p)
		sd.redraw()
		sd.refreshActions()
	}
	return nil
}

func (sd *side) clearMarks() tea.Cmd {
	sd.marks = nil
	sd.redraw()
	sd.refreshActions()
	return nil
}

// refreshActions is a no-op kept for the moments a side's actions change:
// they are registered once and decide with Hidden and Disabled.
func (sd *side) refreshActions() {}

// --- hosts ---------------------------------------------------------------------------------

// pickHost lists this machine first, then the saved hosts.
func (sd *side) pickHost() tea.Cmd {
	t := sd.t
	acts := []terminu.Action{{Label: remote.LocalLabel, Desc: "this machine, from where sshu started",
		Do: func() tea.Cmd { return sd.connect(remote.Local()) }}}
	for _, h := range t.s.d.Hosts {
		h := h
		acts = append(acts, terminu.Action{Label: h.Name, Desc: h.Addr(), Do: func() tea.Cmd { return sd.dial(h) }})
	}
	return t.s.a.Menu("Host", terminu.MenuSection{Actions: acts})
}

// connect points the side at fsys, in its starting directory. Marks go: they
// were paths on the previous host.
func (sd *side) connect(fsys remote.FS) tea.Cmd {
	sd.endDial()
	if sd.fs != nil {
		sd.fs.Close()
	}
	sd.fs, sd.host, sd.err, sd.dialing = fsys, fsys.Label(), "", ""
	sd.marks = nil
	home, err := fsys.Home()
	if err != nil {
		home = "/"
	}
	sd.home = home
	if err := sd.list(remote.StartDir(fsys, home)); err != nil {
		_ = sd.list(home)
	}
	sd.redraw()
	if rows := sd.files.Rows(); len(rows) > 0 {
		sd.files.Select(rows[0].ID)
	}
	return sd.t.watch()
}

// disconnectCmd puts the side back to no host; a dial on its way is called off.
func (sd *side) disconnectCmd() tea.Cmd {
	host := cmpOr(sd.dialing, sd.host)
	if sd.dialing != "" && sd.dialCancel != nil {
		sd.dialCancelled = true
		sd.dialCancel()
		return sd.t.s.a.Toast(terminu.Info, "Calling off the connection to "+host)
	}
	sd.endDial()
	if sd.fs != nil {
		sd.fs.Close()
	}
	gen := sd.dialGen + 1
	sd.fs, sd.host, sd.cwd, sd.home, sd.entries, sd.err, sd.marks = nil, "", "", "", nil, "", nil
	sd.dialing, sd.dialGen, sd.dialCancelled = "", gen, false
	sd.redraw()
	return sd.t.s.a.Toast(terminu.Success, "Disconnected from "+host)
}

type dialedMsg struct {
	side, gen int
	name      string
	fs        remote.FS
	err       error
}

// dial connects the side to a saved host, off the update loop. An sshconfig
// host goes through the real ssh, its questions relayed to sshu.
func (sd *side) dial(h store.Host) tea.Cmd {
	t := sd.t
	rh, err := store.Resolve(h, t.s.d.Creds)
	if err != nil {
		t.s.logError(store.LevelError, "Connect to "+h.Name+" failed: "+err.Error(), "")
		return t.s.a.Error("Connect failed", err.Error())
	}
	sd.endDial()
	sd.dialGen++
	gen := sd.dialGen
	sd.dialing, sd.dialCancelled, sd.err = h.Name, false, ""
	sd.redraw()
	budget := t.s.d.Config.Timeout()
	if rh.Auth != store.AuthSSHConfig {
		return func() tea.Msg {
			fsys, err := remote.Dial(rh, nil, budget)
			return dialedMsg{side: sd.idx, gen: gen, name: h.Name, fs: fsys, err: err}
		}
	}
	srv, err := newAskpassServer(sd.idx, gen)
	if err != nil {
		sd.dialing, sd.err = "", err.Error()
		sd.redraw()
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	cmd := sftpCmd(ctx, rh, selfPath(), srv.path(), int(budget/time.Second))
	sd.askpass, sd.dialCancel = srv, cancel
	pipe := func() tea.Msg {
		registerProc(cmd)
		fsys, err := remote.DialPipe(h.Name, cmd)
		if err != nil {
			deregisterProc(cmd)
			cancel()
			return dialedMsg{side: sd.idx, gen: gen, name: h.Name, err: err}
		}
		return dialedMsg{side: sd.idx, gen: gen, name: h.Name, fs: &trackedFS{FS: fsys, cmd: cmd, cancel: cancel}}
	}
	return tea.Batch(pipe, srv.next())
}

// trackedFS closes the ssh under an sshconfig host's connection with it.
type trackedFS struct {
	remote.FS
	cmd    *exec.Cmd
	cancel context.CancelFunc
}

func (f *trackedFS) Close() error {
	err := f.FS.Close()
	f.cancel()
	deregisterProc(f.cmd)
	return err
}

func (sd *side) endDial() {
	sd.askpass.close()
	if sd.dialCancel != nil {
		sd.dialCancel()
	}
	sd.askpass, sd.dialCancel = nil, nil
}

// dialReason is why a connection to name failed, for a toast and a panel's
// middle: short text, where a \n is drawn as \n (components, control
// characters). The remote package starts the error with the name, and what
// ssh printed may run over several lines, a banner first and the reason
// last. all is the whole of it, for the App Log.
func dialReason(name string, err error) (reason, all string) {
	all = strings.TrimPrefix(err.Error(), name+": ")
	reason = all
	if i := strings.LastIndexByte(all, '\n'); i >= 0 {
		reason = strings.TrimSpace(all[i+1:])
	}
	return reason, all
}

func (t *transfer) dialed(m dialedMsg) tea.Cmd {
	sd := t.sides[m.side]
	if m.gen != sd.dialGen {
		if m.fs != nil {
			m.fs.Close() // somebody moved on while it was dialling
		}
		return nil
	}
	cancelled := sd.dialCancelled
	sd.askpass.close()
	sd.askpass, sd.dialCancel, sd.dialing = nil, nil, ""
	user := t.userFor(m.name)
	if m.err != nil {
		t.s.logConn(store.ConnectionEntry{Host: m.name, User: user, Result: store.ResultFail})
		if cancelled {
			sd.redraw()
			return t.s.a.Toast(terminu.Info, "Connection to "+m.name+" called off")
		}
		reason, all := dialReason(m.name, m.err)
		sd.err = m.name + ": " + reason
		sd.redraw()
		detail := ""
		if all != reason {
			detail = all
		}
		t.s.logError(store.LevelError, "Connect to "+who(user, m.name)+" failed: "+reason, detail)
		return t.s.a.Toast(terminu.Error, "Connect to "+m.name+" failed: "+reason)
	}
	t.s.logConn(store.ConnectionEntry{Host: m.name, User: user, Result: store.ResultSuccess})
	return sd.connect(m.fs)
}

// userFor is who a saved host connects as, for the journals.
func (t *transfer) userFor(name string) string {
	for _, h := range t.s.d.Hosts {
		if h.Name == name {
			if r, err := store.Resolve(h, t.s.d.Creds); err == nil {
				return r.User
			}
			return h.User
		}
	}
	return ""
}

// askpassArrived asks the user what ssh asked. Esc on the question answers
// ssh no, and ssh gives up (tdp F6).
func (t *transfer) askpassArrived(r *askpassRequest) tea.Cmd {
	sd := t.sides[r.side]
	if r.gen != sd.dialGen || sd.askpass == nil {
		r.refuse()
		return nil
	}
	next := sd.askpass.next()
	lines := strings.Split(strings.TrimRight(r.prompt, " \n"), "\n")
	if r.isConfirm() {
		return tea.Batch(next, t.s.a.Confirm(terminu.Ask{Title: "Trust", Subject: sd.dialing,
			Details: lines, Question: "Trust this key and connect to " + sd.dialing + "?", Verb: "trust and connect",
			OnCancel: func() tea.Cmd { r.refuse(); return nil }},
			func() tea.Cmd { r.answer("yes"); return nil }))
	}
	return tea.Batch(next, t.s.a.Password(terminu.PasswordInput{Title: "Connect", Subject: sd.dialing, Glyph: glyphLock,
		Prompt: strings.Join(lines, " "), Verb: "send", Name: "The answer",
		OnCancel: func() tea.Cmd { r.refuse(); return nil },
		Done:     func(v string, _ bool) tea.Cmd { r.answer(v); return nil }}))
}

// --- watching ----------------------------------------------------------------------

// SFTP cannot push a change, so the directories on screen are probed every
// couple of seconds and re-read only when their timestamp moved.
const watchEvery = 2 * time.Second

type watchTick struct{ gen int }

type watchResult struct {
	side    int
	cwd     string
	mtime   time.Time
	entries []remote.Entry
	changed bool
}

func (t *transfer) watch() tea.Cmd {
	t.watchGen++
	return t.rearm()
}

func (t *transfer) rearm() tea.Cmd {
	if t.sides[0].fs == nil && t.sides[1].fs == nil {
		return nil
	}
	gen := t.watchGen
	return t.s.a.Tick(watchEvery, func(time.Time) tea.Msg { return watchTick{gen: gen} })
}

func (t *transfer) onWatchTick(m watchTick) tea.Cmd {
	if m.gen != t.watchGen {
		return nil
	}
	cmds := []tea.Cmd{t.rearm()}
	if t.s.a.Screen() != t.screen {
		return tea.Batch(cmds...) // nobody is looking: keep the connection quiet
	}
	for _, sd := range t.sides {
		if sd.fs == nil || sd.cwd == "" || sd.probing {
			continue
		}
		sd.probing = true
		fsys, cwd, known, i := sd.fs, sd.cwd, sd.watchMTime, sd.idx
		cmds = append(cmds, func() tea.Msg {
			st, err := fsys.Stat(cwd)
			if err != nil || st.ModTime.Equal(known) {
				return watchResult{side: i, cwd: cwd, mtime: known}
			}
			entries, err := fsys.List(cwd)
			if err != nil {
				return watchResult{side: i, cwd: cwd, mtime: known}
			}
			return watchResult{side: i, cwd: cwd, mtime: st.ModTime, entries: entries, changed: true}
		})
	}
	return tea.Batch(cmds...)
}

func (t *transfer) onWatchResult(m watchResult) {
	sd := t.sides[m.side]
	sd.probing = false
	if sd.fs == nil || sd.cwd != m.cwd {
		return
	}
	sd.watchMTime = m.mtime
	if m.changed {
		sd.entries = m.entries
		sd.redraw()
	}
}

func dirMTime(fsys remote.FS, dir string) time.Time {
	e, err := fsys.Stat(dir)
	if err != nil {
		return time.Time{}
	}
	return e.ModTime
}

// --- messages ------------------------------------------------------------------------

func (t *transfer) onMsg(msg tea.Msg) (tea.Cmd, bool) {
	switch m := msg.(type) {
	case dialedMsg:
		return t.dialed(m), true
	case askpassRequestMsg:
		return t.askpassArrived(m.req), true
	case watchTick:
		return t.onWatchTick(m), true
	case watchResult:
		t.onWatchResult(m)
		return nil, true
	case xferTick:
		return t.onXferTick(), true
	case scanTick:
		return t.onScanTick(), true
	case viewLoaded:
		m.apply()
		return nil, true
	case editFetched:
		return t.onEditFetched(m), true
	case editSaved:
		return t.onEditSaved(m), true
	}
	return nil, false
}

// --- small helpers ----------------------------------------------------------------------

func humanSize(n int64) string {
	switch {
	case n < 1024:
		return fmt.Sprintf("%d B", n)
	case n < 1024*1024:
		return fmt.Sprintf("%.1f kB", float64(n)/1024)
	case n < 1024*1024*1024:
		return fmt.Sprintf("%.1f MB", float64(n)/(1024*1024))
	default:
		return fmt.Sprintf("%.1f GB", float64(n)/(1024*1024*1024))
	}
}

// foldHomePath writes p under home as ~.
func foldHomePath(p, home string) string {
	if home == "" || home == "/" {
		return p
	}
	if p == home {
		return "~"
	}
	if rest, ok := strings.CutPrefix(p, home+"/"); ok {
		return "~/" + rest
	}
	return p
}

// closeAll ends both sides' connections, on the way out.
func (t *transfer) closeAll() {
	for _, sd := range t.sides {
		sd.endDial()
		if sd.fs != nil {
			sd.fs.Close()
			sd.fs = nil
		}
	}
}
