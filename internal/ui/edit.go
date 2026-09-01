package ui

import (
	"context"
	"io"
	"io/fs"
	"os"
	"path"
	"sync/atomic"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/vulcanshen/sshu/internal/remote"
)

// `[e]dit` opens the item under the cursor in your editor.
//
// kbu does this by running `kubectl edit` inside its embedded PTY and letting
// kubectl do the fetch/edit/apply dance. There is no `sftp edit`, so here the
// dance is sshu's own:
//
//	remote  fetch to a temp file -> $EDITOR -> changed? -> write back
//	local   $EDITOR, on the real file
//
// The local case is not an optimisation. Editing in place keeps the inode, so
// hard links, ownership and xattrs survive; copy-and-rename would break all
// three to buy protection against a network that is not involved.
//
// The editor runs in the embedded PTY — the same one tab [3] uses — so the frame
// stays up and you can still see which host you are editing on.

// editTickEvery drives both the PTY repaint and the spinner. The PTY sets the
// pace: a remote redraw that arrives 50ms late is a redraw you can feel.
const editTickEvery = 50 * time.Millisecond

// spinnerEvery is how many ticks one spinner frame lasts. At the PTY's pace a
// spinner would be a blur, so it steps at a fraction of it.
const spinnerEvery = 3

// headBytes is how much is read to decide whether a file is text. Enough for any
// realistic header, small enough to be free.
const headBytes = 8192

type editPhase int

const (
	editFetching editPhase = iota
	editRunning
	editSaving
)

type editTickMsg struct{}

// editFetchedMsg carries the local copy, or why there is not one.
type editFetchedMsg struct {
	gen  int
	job  editJob
	text bool
	err  error
}

// editSavedMsg is the end of the errand.
type editSavedMsg struct {
	gen      int
	changed  bool
	conflict bool
	err      error
}

// editJob is the errand: where the file came from, what it looked like, and the
// local copy standing in for it.
type editJob struct {
	fsys   remote.FS
	path   string
	mode   fs.FileMode
	stamp  remote.Stamp
	dir    string // temp directory to clean up; empty when editing in place
	local  string // what the editor opens
	digest [32]byte
}

// inPlace reports whether the editor is pointed at the real file, which is the
// case where there is nothing to send back.
func (j editJob) inPlace() bool { return j.dir == "" }

// ------------------------------------------------------------------- popup

type editorPopup struct {
	anim  popupAnimator
	gen   int
	phase editPhase

	name  string
	note  string
	total int64
	got   *atomic.Int64
	ticks int

	cancel context.CancelFunc
	pty    *ptyTerm

	layer   int
	screenW int
	screenH int
}

func newEditorPopup() editorPopup {
	return editorPopup{anim: newPopupAnimator("editor"), got: &atomic.Int64{}}
}

func (m editorPopup) isActive() bool { return m.anim.isActive() }

// running reports whether a live editor holds the keyboard.
func (m editorPopup) running() bool {
	return m.anim.owns() && m.phase == editRunning && m.pty != nil
}

func (m *editorPopup) setSize(w, h int) {
	m.screenW, m.screenH = w, h
	if m.pty != nil {
		m.pty.resize(m.innerW(), m.rows())
	}
}

// The editor gets nearly the whole terminal: it is the thing being used, not a
// thing being glanced at.
func (m editorPopup) innerW() int { return popupInnerW(m.screenW, m.screenW-6) }
func (m editorPopup) rows() int   { return max(3, m.screenH-4) }

// open shows the box before the first byte arrives — the same reason the viewer
// and the dial spinner do.
func (m *editorPopup) open(layer int, name string, total int64) tea.Cmd {
	m.gen++
	m.layer, m.name, m.total = layer, name, total
	m.phase, m.note, m.ticks = editFetching, "", 0
	m.got = &atomic.Int64{}
	return tea.Batch(m.anim.open(), m.tick())
}

func (m editorPopup) tick() tea.Cmd {
	return tea.Tick(editTickEvery, func(time.Time) tea.Msg { return editTickMsg{} })
}

// stop cancels the fetch and kills the editor. Every way out goes through it, so
// none of them can leave a process holding a PTY nobody owns.
func (m *editorPopup) stop() {
	if m.cancel != nil {
		m.cancel()
		m.cancel = nil
	}
	m.pty.stop()
	m.pty = nil
}

func (m *editorPopup) close() tea.Cmd {
	m.stop()
	return m.anim.close()
}

func (m editorPopup) view() string {
	innerW := m.innerW()
	title := " " + glyphPencil + " " + m.name + " "

	if m.phase == editRunning && m.pty != nil {
		// No padding rows: two lines of blank border is two lines of editor.
		return drawPopupBoxPad(popupLayerColor(m.layer), title,
			hintLegend([][2]string{{"alt+esc", "abandon"}}),
			animRows(m.anim, m.pty.render(innerW, m.rows())), innerW, false)
	}
	return drawPopupBox(popupLayerColor(m.layer), title,
		hintLegend([][2]string{{"Esc", "cancel"}}),
		animRows(m.anim, capRows(m.waitingRows(innerW), m.screenH)), innerW)
}

// waitingRows is the spinner while bytes are moving. Same answer as the dial
// spinner to the same complaint: a frame that never changes is what stuck looks
// like, and over a slow link this wait is the whole experience.
func (m editorPopup) waitingRows(innerW int) []string {
	dim := lipgloss.NewStyle().Foreground(dimColor)
	hand := lipgloss.NewStyle().Foreground(handColor)

	if m.note != "" {
		return []string{centerLine(innerW, m.note, dim.Render(m.note))}
	}

	spin := spinnerFrames[(m.ticks/spinnerEvery)%len(spinnerFrames)]
	verb := "reading"
	if m.phase == editSaving {
		verb = "writing"
	}
	prog := ""
	if m.got != nil {
		if got := m.got.Load(); got > 0 {
			prog = "  " + humanSize(got)
			if m.total > 0 {
				prog += " / " + humanSize(m.total)
			}
		}
	}
	plain := spin + " " + verb + " " + m.name + prog
	return []string{centerLine(innerW, plain,
		hand.Render(spin)+dim.Render(" "+verb+" ")+hand.Render(m.name)+dim.Render(prog))}
}

// ---------------------------------------------------------------- the work

// fileHead reads the first n bytes, for the is-this-text question.
func fileHead(p string, n int) []byte {
	f, err := os.Open(p)
	if err != nil {
		return nil
	}
	defer f.Close()
	buf := make([]byte, n)
	k, _ := io.ReadFull(f, buf)
	return buf[:k]
}

// probeEdit prepares a LOCAL file off the update loop. The digest is a full read
// of the file, and the update loop does not do full reads.
func probeEdit(gen int, job editJob) tea.Cmd {
	return func() tea.Msg {
		d, err := remote.Digest(job.local)
		if err != nil {
			return editFetchedMsg{gen: gen, err: err}
		}
		job.digest = d
		return editFetchedMsg{gen: gen, job: job, text: isText(fileHead(job.local, headBytes))}
	}
}

// fetchEdit brings a remote file down.
//
// The is-this-text probe happens here rather than before: the bytes are needed
// anyway, so asking the question over the network first would spend a round trip
// learning something the download answers for nothing.
func fetchEdit(ctx context.Context, gen int, job editJob, got *atomic.Int64) tea.Cmd {
	return func() tea.Msg {
		dir, err := os.MkdirTemp("", "sshu-edit-")
		if err != nil {
			return editFetchedMsg{gen: gen, err: err}
		}
		local, err := remote.Fetch(ctx, job.fsys, job.path, dir, func(n int64) { got.Add(n) })
		if err != nil {
			_ = os.RemoveAll(dir)
			return editFetchedMsg{gen: gen, err: err}
		}
		job.dir, job.local = dir, local
		if job.digest, err = remote.Digest(local); err != nil {
			_ = os.RemoveAll(dir)
			return editFetchedMsg{gen: gen, err: err}
		}
		return editFetchedMsg{gen: gen, job: job, text: isText(fileHead(local, headBytes))}
	}
}

// saveEdit decides what leaving the editor meant, and acts on it.
//
// force skips the stamp check, and is only ever set by the user answering the
// question the stamp check asked.
func saveEdit(gen int, job editJob, force bool) tea.Cmd {
	return func() tea.Msg {
		now, err := remote.Digest(job.local)
		if err != nil {
			return editSavedMsg{gen: gen, err: err}
		}
		if now == job.digest {
			return editSavedMsg{gen: gen}
		}
		// A local file was edited where it lives — the editor already wrote it.
		if job.inPlace() {
			return editSavedMsg{gen: gen, changed: true}
		}
		if !force {
			if cur, serr := remote.StampOf(job.fsys, job.path); serr == nil && cur != job.stamp {
				return editSavedMsg{gen: gen, changed: true, conflict: true}
			}
		}
		if err := remote.WriteBack(job.fsys, job.local, job.path, job.mode); err != nil {
			return editSavedMsg{gen: gen, changed: true, err: err}
		}
		return editSavedMsg{gen: gen, changed: true}
	}
}

// ----------------------------------------------------------------- the flow

// startEdit opens the popup and puts the file within an editor's reach.
func (m AppModel) startEdit(job editJob, size int64) (tea.Model, tea.Cmd) {
	open := m.editorUI.open(m.layer(), path.Base(job.path), size)
	if lp, ok := remote.LocalPath(job.fsys, job.path); ok {
		job.local = lp
		return m, tea.Batch(open, probeEdit(m.editorUI.gen, job))
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.editorUI.cancel = cancel
	return m, tea.Batch(open, fetchEdit(ctx, m.editorUI.gen, job, m.editorUI.got))
}

func (m AppModel) onEditFetched(msg editFetchedMsg) (tea.Model, tea.Cmd) {
	if msg.gen != m.editorUI.gen || !m.editorUI.isActive() {
		// A superseded fetch still has a temp directory to its name.
		if msg.job.dir != "" {
			_ = os.RemoveAll(msg.job.dir)
		}
		return m, nil
	}
	m.editorUI.cancel = nil
	if msg.err != nil {
		name := m.editorUI.name
		return m, tea.Batch(m.editorUI.close(),
			m.toast.show("Cannot open "+name+": "+msg.err.Error(), toastError))
	}
	m.pendingEdit = msg.job
	if !msg.text {
		// Ask rather than refuse. The test is "no NUL and valid UTF-8", and a
		// Latin-1 config file fails it while being perfectly editable — refusing
		// would be a guess overruling a person about their own file. The box
		// steps aside while the question is asked, because a spinner that has
		// nothing left to spin for is a lie.
		return m, tea.Batch(m.editorUI.anim.close(), m.confirm.ask(confirmPopup{
			glyph: glyphWarn,
			title: "Not text",
			lines: []string{
				path.Base(msg.job.path) + " does not look like text.",
				"Saving it from an editor can corrupt it.",
			},
			accept: "edit anyway",
			warn:   true,
			action: confirmEditBinary,
		}, m.editorUI.layer+1))
	}
	return m.launchEditor()
}

// launchEditor hands the keyboard to $EDITOR.
func (m AppModel) launchEditor() (tea.Model, tea.Cmd) {
	name := path.Base(m.pendingEdit.path)
	cmd, err := editorCommand(m.pendingEdit.local)
	if err != nil {
		return m, tea.Batch(m.confirm.close(), m.closeEdit(false),
			m.toast.show(err.Error(), toastError))
	}
	p, err := startPty(cmd, m.editorUI.innerW(), m.editorUI.rows())
	if err != nil {
		return m, tea.Batch(m.confirm.close(), m.closeEdit(false),
			m.toast.show("Cannot edit "+name+": "+err.Error(), toastError))
	}
	m.editorUI.pty, m.editorUI.phase = p, editRunning
	return m, tea.Batch(m.confirm.close(), m.editorUI.anim.open(), m.editorUI.tick())
}

func (m AppModel) onEditTick() (tea.Model, tea.Cmd) {
	if !m.editorUI.isActive() {
		return m, nil
	}
	m.editorUI.ticks++
	if m.editorUI.phase == editRunning && m.editorUI.pty.exited() {
		return m.editorExited()
	}
	return m, m.editorUI.tick()
}

// editorExited is the moment the editor closed. What happens next is a question
// about content, not about the editor's exit code: an editor that was quit with
// :cq still wrote the file, and one that exited 0 may have changed nothing.
func (m AppModel) editorExited() (tea.Model, tea.Cmd) {
	m.editorUI.stop()
	m.editorUI.phase = editSaving
	m.editorUI.got = &atomic.Int64{}
	return m, tea.Batch(saveEdit(m.editorUI.gen, m.pendingEdit, false), m.editorUI.tick())
}

func (m AppModel) onEditSaved(msg editSavedMsg) (tea.Model, tea.Cmd) {
	if msg.gen != m.editorUI.gen {
		return m, nil
	}
	name, local, where := path.Base(m.pendingEdit.path), m.pendingEdit.local, ""
	if m.pendingEdit.fsys != nil {
		where = m.pendingEdit.fsys.Label()
	}
	switch {
	case msg.conflict:
		// Somebody else wrote to it while the editor was open. The local copy is
		// kept whichever way this goes — declining must not be the thing that
		// loses the work.
		return m, tea.Batch(m.editorUI.close(), m.confirm.ask(confirmPopup{
			glyph: glyphWarn,
			title: "Changed on " + where,
			lines: []string{
				name + " changed since you opened it.",
				"Saving replaces what is there now.",
			},
			accept: "overwrite",
			warn:   true,
			action: confirmEditOverwrite,
		}, 1))
	case msg.err != nil:
		m.log.errorf("edit: "+name+" · "+msg.err.Error(), "your copy is at "+local)
		return m, tea.Batch(m.closeEdit(true),
			m.toast.show("Not saved: "+msg.err.Error()+" — your copy is at "+local, toastError))
	case !msg.changed:
		return m, tea.Batch(m.closeEdit(false),
			m.toast.show("No changes to "+name, toastInfo))
	}
	dest := where
	if dest == "" {
		dest = "local"
	}
	m.log.info("edit: saved " + name + " (" + dest + ")")
	return m, tea.Batch(m.closeEdit(false), m.toast.show("Saved "+name, toastInfo))
}

// saveEditForced is the yes to the overwrite question.
func (m AppModel) saveEditForced() (tea.Model, tea.Cmd) {
	job := m.pendingEdit
	if job.local == "" {
		return m, m.closeStack()
	}
	m.editorUI.gen++
	m.editorUI.phase, m.editorUI.note = editSaving, ""
	m.editorUI.got = &atomic.Int64{}
	return m, tea.Batch(m.confirm.close(), m.editorUI.anim.open(),
		saveEdit(m.editorUI.gen, job, true), m.editorUI.tick())
}

// closeEdit ends the errand and cleans up after it.
//
// keep leaves the local copy on disk, and is used whenever the write-back did
// not happen: that file is the user's work, and tidying it away would be the app
// throwing out the thing it was asked to help make.
func (m *AppModel) closeEdit(keep bool) tea.Cmd {
	job := m.pendingEdit
	m.pendingEdit = editJob{}
	if !keep && job.dir != "" {
		_ = os.RemoveAll(job.dir)
	}
	return m.editorUI.close()
}

// abandonEdit is Alt+Esc inside a running editor: kill it, keep nothing. In tab
// [3] that key means "take the keyboard back", and here there is no other panel
// to take it back to — so it means leaving, and the hint on the box says so.
func (m AppModel) abandonEdit() (tea.Model, tea.Cmd) {
	name := path.Base(m.pendingEdit.path)
	return m, tea.Batch(m.closeEdit(false),
		m.toast.show("Abandoned "+name, toastInfo))
}

// declineEdit is Esc on one of the two edit questions. Neither is free: the
// binary one is standing on a temp file to throw away, and the overwrite one on
// an edit that must not vanish silently.
func (m *AppModel) declineEdit() tea.Cmd {
	switch m.confirm.action {
	case confirmEditBinary:
		return m.closeEdit(false)
	case confirmEditOverwrite:
		local := m.pendingEdit.local
		return tea.Batch(m.closeEdit(true),
			m.toast.show("Not saved — your edit is at "+local, toastInfo))
	}
	return nil
}
