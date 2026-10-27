package tui

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	terminu "github.com/vulcanshen/terminu-go"

	"github.com/vulcanshen/sshu/internal/remote"
)

// What a side can do to the files themselves: rename, add, delete, search,
// view, edit.

// nameCheck refuses what a name box cannot mean: a path.
func nameCheck(v string) error {
	if strings.ContainsRune(strings.TrimSuffix(strings.TrimSpace(v), "/"), '/') {
		return errors.New("A name can't contain /")
	}
	return nil
}

// rename renames p in place, the box starting from the old name. The
// destination is checked first: os.Rename would overwrite and SFTP's would
// refuse, and one action must not depend on which end it runs on.
func (sd *side) rename(p string, ok bool) tea.Cmd {
	if !ok {
		return nil
	}
	a := sd.t.s.a
	base := path.Base(p)
	return a.Text(terminu.TextInput{Title: "Rename", Subject: base, Glyph: glyphPencil, Prompt: "New name for " + base,
		Verb: "rename", Name: "A name", Old: base,
		Check: func(v string) error {
			v = strings.TrimSpace(v)
			if err := nameCheck(v); err != nil {
				return err
			}
			if v == "" {
				return terminu.ErrEmpty
			}
			if v != base && remote.Exists(sd.fs, remote.Join(path.Dir(p), v)) {
				return errors.New(v + " already exists")
			}
			return nil
		},
		// A rename that fails keeps the box open with the name in it, under
		// the error (tdp F4).
		Commit: func(v string) error {
			dst := remote.Join(path.Dir(p), strings.TrimSpace(v))
			if err := sd.fs.Rename(p, dst); err != nil {
				return err
			}
			// A mark is a path: it moves with the file.
			if sd.isMarked(p) {
				dir := sd.markDir[p]
				sd.unmark(p)
				sd.marks = append(sd.marks, dst)
				sd.markDir[dst] = dir
			}
			return nil
		},
		Done: func(v string) tea.Cmd {
			v = strings.TrimSpace(v)
			sd.reload()
			sd.files.Select(remote.Join(path.Dir(p), v))
			return a.Toast(terminu.Success, "Renamed to "+v)
		}})
}

// add makes a file, or a directory when the name ends in a slash: the way a
// shell, and these lists, write one.
func (sd *side) add() tea.Cmd {
	a := sd.t.s.a
	return a.Text(terminu.TextInput{Title: "Add", Glyph: glyphPlus,
		Prompt: "A name in " + foldHomePath(sd.cwd, sd.home) + ", or name/ for a directory", Verb: "create", Name: "A name",
		Check: func(v string) error {
			v = strings.TrimSpace(v)
			if err := nameCheck(v); err != nil {
				return err
			}
			name := strings.TrimSuffix(v, "/")
			if name == "" {
				return terminu.ErrEmpty
			}
			// Create truncates: an existing name is refused, not emptied.
			if remote.Exists(sd.fs, remote.Join(sd.cwd, name)) {
				return errors.New(name + " already exists")
			}
			return nil
		},
		// One that cannot be made keeps the box open with the name in it,
		// under the error (tdp F4).
		Commit: func(v string) error {
			v = strings.TrimSpace(v)
			p := remote.Join(sd.cwd, strings.TrimSuffix(v, "/"))
			if strings.HasSuffix(v, "/") {
				return sd.fs.MkdirAll(p)
			}
			w, err := sd.fs.Create(p, 0o644)
			if err != nil {
				return err
			}
			return w.Close()
		},
		Done: func(v string) tea.Cmd {
			v = strings.TrimSpace(v)
			name := strings.TrimSuffix(v, "/")
			sd.reload()
			sd.files.Select(remote.Join(sd.cwd, name)) // the new one, in sight (tdp K3)
			kind := "file"
			if strings.HasSuffix(v, "/") {
				kind = "directory"
			}
			return a.Toast(terminu.Success, "Created "+kind+" "+name)
		}})
}

func (sd *side) deleteAsk(p string, ok bool) (terminu.Ask, func() tea.Cmd, bool) {
	if !ok || sd.fs == nil {
		return terminu.Ask{}, nil, false
	}
	a := sd.t.s.a
	return terminu.Ask{Title: "Delete", Subject: path.Base(p), Details: []string{path.Base(p) + " on " + sd.host + "."},
			Question: "Delete permanently?", Verb: "delete"},
		func() tea.Cmd {
			if err := remote.RemoveAll(sd.fs, p); err != nil {
				return a.Error("Delete failed", err.Error())
			}
			sd.unmark(p)
			sd.reload()
			return a.Toast(terminu.Success, "Deleted "+path.Base(p))
		}, true
}

// deleteMarksAsk erases every mark on this side: it deletes what it can and
// says the first failure, so the lists show what is really left.
func (sd *side) deleteMarksAsk() (terminu.Ask, func() tea.Cmd, bool) {
	a := sd.t.s.a
	return terminu.Ask{Title: "Delete", Subject: sd.host,
			Details: []string{plural(len(sd.marks), "marked item") + " on " + sd.host + "."}, Question: "Delete permanently?", Verb: "delete"},
		func() tea.Cmd {
			failed, done := "", 0
			for _, p := range append([]string(nil), sd.marks...) {
				if err := remote.RemoveAll(sd.fs, p); err != nil {
					if failed == "" {
						failed = err.Error()
					}
					continue
				}
				sd.unmark(p)
				done++
			}
			sd.reload()
			if failed != "" {
				return a.Error("Delete failed", failed)
			}
			return a.Toast(terminu.Success, "Deleted "+plural(done, "item"))
		}, true
}

// --- search ------------------------------------------------------------------------

// The search walks the whole subtree under the directory and lists what it
// finds as it goes (tdp finder.7: rows may keep arriving); Enter
// goes to where the one picked lives.

type scanTick struct{}

const scanEvery = 120 * time.Millisecond

type scan struct {
	mu     sync.Mutex
	buf    []remote.Entry
	done   bool
	cancel context.CancelFunc
	finder *terminu.Finder
	sd     *side
	root   string
	local  bool
}

func (sd *side) search() tea.Cmd {
	t := sd.t
	t.stopScans()
	ctx, cancel := context.WithCancel(context.Background())
	sc := &scan{cancel: cancel, sd: sd, root: sd.cwd, local: isLocal(sd.fs)}
	sc.finder = &terminu.Finder{Title: "Search", Subject: foldHomePath(sd.cwd, sd.home), Glyph: glyphEye, Verb: "go there",
		Loading: true, Files: true,
		Done: func(it terminu.FinderItem) tea.Cmd {
			sc.cancel()
			sd.t.s.logOp("Found " + sd.at(it.ID))
			return sd.goToFound(sc.root, strings.TrimPrefix(it.ID, sc.root+"/"))
		}}
	if !isLocal(sd.fs) {
		sc.finder.FS = newPreviewFS(sd.fs) // a remote host's files get the same preview
	}
	var sub []string
	for _, e := range sd.entries {
		if e.IsDir {
			sub = append(sub, remote.Join(sd.cwd, e.Name))
		}
	}
	// The directory itself first, then everything below it.
	for _, e := range sd.entries {
		sc.finder.Items = append(sc.finder.Items, foundItem(sc.root, e, isLocal(sd.fs)))
	}
	t.scans = append(t.scans, sc)
	fsys, root := sd.fs, sd.cwd
	go func() {
		remote.Scan(ctx, fsys, root, sub, remote.SearchCap, func(b []remote.Entry) {
			sc.mu.Lock()
			sc.buf = append(sc.buf, b...)
			sc.mu.Unlock()
		})
		sc.mu.Lock()
		sc.done = true
		sc.mu.Unlock()
	}()
	cmd := t.s.a.Find(sc.finder)
	if !t.scanning {
		t.scanning = true
		cmd = tea.Batch(cmd, t.s.a.Tick(scanEvery, func(time.Time) tea.Msg { return scanTick{} }))
	}
	return cmd
}

// foundItem is one result: its absolute path, shown under the root. The
// glyphs are the ones the lib's preview tree draws, so the list and the
// preview beside it agree (tdp file-picker.3).
func foundItem(root string, e remote.Entry, local bool) terminu.FinderItem {
	icon := glyphTreeFile
	name := e.Name
	if e.IsDir {
		icon, name = glyphTreeDir, e.Name+"/"
	}
	return terminu.FinderItem{ID: remote.Join(root, e.Name), Text: name, Icon: icon, IconColor: terminu.Text}
}

// isLocal reports this machine's filesystem: the one the lib can preview.
func isLocal(fsys remote.FS) bool { _, ok := remote.LocalPath(fsys, "/"); return ok }

func (t *transfer) onScanTick() tea.Cmd {
	running := false
	kept := t.scans[:0]
	for _, sc := range t.scans {
		sc.mu.Lock()
		batch, done := sc.buf, sc.done
		sc.buf = nil
		sc.mu.Unlock()
		for _, e := range batch {
			sc.finder.Items = append(sc.finder.Items, foundItem(sc.root, e, sc.local))
		}
		if done {
			sc.finder.Loading = false
			continue
		}
		running = true
		kept = append(kept, sc)
	}
	t.scans = kept
	if !running {
		t.scanning = false
		return nil
	}
	return t.s.a.Tick(scanEvery, func(time.Time) tea.Msg { return scanTick{} })
}

func (t *transfer) stopScans() {
	for _, sc := range t.scans {
		sc.cancel()
	}
}

// goToFound goes where a result lives and puts the cursor on it.
func (sd *side) goToFound(root, rel string) tea.Cmd {
	rel = strings.TrimSuffix(rel, "/")
	dir, base := path.Dir(rel), path.Base(rel)
	if dir == "." {
		dir = ""
	}
	return sd.goTo(remote.Join(root, dir), base)
}

// --- view --------------------------------------------------------------------------

type viewLoaded struct {
	v     *terminu.Viewer
	lines []string
	lang  string
	title string // "Directory" when it turned out to be one
}

func (m viewLoaded) apply() {
	m.v.Lines, m.v.Lang, m.v.Loading = m.lines, m.lang, false
	if m.title != "" {
		m.v.Title = m.title
	}
}

// view opens p for reading: text coloured by its name, binary as hex, a
// directory as its listing. The viewer opens first and fills when the bytes
// arrive, so a slow link does not look like a key that did nothing.
func (sd *side) view(p string, ok bool) tea.Cmd {
	if !ok {
		return nil
	}
	fsys := sd.fs
	sd.t.s.logOp("Viewed " + sd.at(p))
	v := newViewer("File", path.Base(p), glyphEye, nil)
	v.Loading = true
	load := func() tea.Msg {
		e, err := fsys.Lstat(p)
		if err == nil && e.Mode&fs.ModeSymlink != 0 {
			e, err = fsys.Stat(p)
		}
		if err != nil {
			return viewLoaded{v: v, lines: []string{"unreadable: " + err.Error()}}
		}
		if e.IsDir {
			return viewLoaded{v: v, lines: dirLines(fsys, p), title: "Directory"}
		}
		data, err := remote.Peek(fsys, p, remote.PeekCap)
		switch {
		case err != nil:
			return viewLoaded{v: v, lines: []string{"unreadable: " + err.Error()}}
		case len(data) == 0:
			return viewLoaded{v: v, lines: []string{"empty file"}}
		case isText(data):
			return viewLoaded{v: v, lines: textLines(data), lang: path.Base(p)}
		}
		return viewLoaded{v: v, lines: hexDump(data)}
	}
	return tea.Batch(sd.t.s.a.Viewer(v), load)
}

func dirLines(fsys remote.FS, p string) []string {
	entries, err := fsys.List(p)
	if err != nil {
		return []string{"unreadable: " + err.Error()}
	}
	if len(entries) == 0 {
		return []string{"empty directory"}
	}
	out := make([]string, len(entries))
	for i, e := range entries {
		if e.IsDir {
			out[i] = e.Name + "/  -"
		} else {
			out[i] = e.Name + "  " + humanSize(e.Size)
		}
	}
	return out
}

func isText(data []byte) bool { return bytes.IndexByte(data, 0) < 0 && utf8.Valid(data) }

// textLines splits text into lines as they are: the viewer draws a control
// character by its name and copies the text unchanged (tdp note.2).
func textLines(data []byte) []string {
	return strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
}

func hexDump(data []byte) []string {
	var out []string
	for off := 0; off < len(data); off += 16 {
		row := data[off:min(off+16, len(data))]
		var hex, asc strings.Builder
		for i := range 16 {
			if i < len(row) {
				fmt.Fprintf(&hex, "%02x ", row[i])
				if c := row[i]; c >= 0x20 && c < 0x7f {
					asc.WriteByte(c)
				} else {
					asc.WriteByte('.')
				}
			} else {
				hex.WriteString("   ")
			}
		}
		out = append(out, fmt.Sprintf("%08x  %s|%s|", off, hex.String(), asc.String()))
	}
	return out
}

// --- edit --------------------------------------------------------------------------

// Editing a remote file is sshu's own dance: fetch it beside sshu, run
// $EDITOR on it in a terminal popup, and write it back only if it changed —
// and only after asking, if it changed on the far side meanwhile. A local
// file is edited where it is.

type editJob struct {
	fsys   remote.FS
	path   string
	mode   fs.FileMode
	stamp  remote.Stamp
	dir    string // the temporary directory, "" when edited in place
	local  string // what the editor opens
	digest [32]byte
}

type editFetched struct {
	job  editJob
	text bool
	err  error
}

type editSaved struct {
	job      editJob
	changed  bool
	conflict bool
	err      error
}

func (sd *side) edit(p string, ok bool) tea.Cmd {
	if !ok {
		return nil
	}
	a := sd.t.s.a
	if sd.t.editing {
		return a.Toast(terminu.Error, "An editor is already open")
	}
	e, err := sd.fs.Stat(p)
	switch {
	case err != nil:
		return a.Error("Edit failed", "Can't read "+path.Base(p)+": "+err.Error())
	case e.IsDir:
		return a.Toast(terminu.Error, "Can't edit a directory")
	case !e.Mode.IsRegular():
		return a.Toast(terminu.Error, "Not a regular file")
	}
	job := editJob{fsys: sd.fs, path: p, mode: e.Mode.Perm(), stamp: remote.Stamp{Size: e.Size, ModTime: e.ModTime}}
	sd.t.editing = true
	if lp, ok := remote.LocalPath(sd.fs, p); ok {
		job.local = lp
		return func() tea.Msg {
			d, err := remote.Digest(lp)
			job.digest = d
			return editFetched{job: job, text: isText(head(lp)), err: err}
		}
	}
	fetch := func() tea.Msg {
		dir, err := os.MkdirTemp("", "sshu-edit-")
		if err != nil {
			return editFetched{err: err}
		}
		local, err := remote.Fetch(context.Background(), job.fsys, job.path, dir, func(int64) {})
		if err == nil {
			job.dir, job.local = dir, local
			job.digest, err = remote.Digest(local)
		}
		if err != nil {
			_ = os.RemoveAll(dir)
			return editFetched{err: err}
		}
		return editFetched{job: job, text: isText(head(local))}
	}
	return tea.Batch(a.Toast(terminu.Info, "Reading "+path.Base(p)+"…"), fetch)
}

func head(p string) []byte {
	f, err := os.Open(p)
	if err != nil {
		return nil
	}
	defer f.Close()
	buf := make([]byte, 8192)
	n, _ := io.ReadFull(f, buf)
	return buf[:n]
}

func (t *transfer) onEditFetched(m editFetched) tea.Cmd {
	a := t.s.a
	if m.err != nil {
		t.editing = false
		return a.Error("Edit failed", "Can't open the file: "+m.err.Error())
	}
	if !m.text {
		// Esc on the question drops the fetched copy (tdp F6).
		t.editing = false
		return a.Confirm(terminu.Ask{Title: "Edit", Subject: path.Base(m.job.path),
			Details:  []string{path.Base(m.job.path) + " does not look like text.", "Saving it from an editor can corrupt it."},
			Warn:     []int{1},
			Question: "Edit it anyway?", Verb: "edit anyway",
			OnCancel: func() tea.Cmd { t.endEdit(m.job, false); return nil }},
			func() tea.Cmd { return t.runEditor(m.job) })
	}
	return t.runEditor(m.job)
}

// runEditor opens $EDITOR on the file in a terminal popup. However it ends —
// by itself, or stopped by the exit key — what is saved is a question about
// the file, not the editor's exit code: vim's :cq writes the file and exits
// 1, and a stopped vim exits 1 having written nothing (design §7.3.4).
func (t *transfer) runEditor(job editJob) tea.Cmd {
	a := t.s.a
	t.editing = true
	argv, err := editorArgv(job.local)
	if err != nil {
		t.endEdit(job, false)
		return a.Error("Edit failed", err.Error())
	}
	term := &terminu.Terminal{Title: "Edit", Subject: path.Base(job.path), Command: argv, Ends: true, Env: editorEnv()}
	term.OnExit = func(error) tea.Cmd {
		deregisterStop(term)
		return saveEdit(job, false)
	}
	registerStop(term)
	return a.OpenTerminal(term)
}

// editorArgv runs $VISUAL or $EDITOR through sh, so an editor with arguments
// ("code -w") works; vi when neither is set. args go before the file ("+12":
// the line to start on).
func editorArgv(p string, args ...string) ([]string, error) {
	ed := ""
	for _, k := range []string{"VISUAL", "EDITOR"} {
		if v := strings.TrimSpace(os.Getenv(k)); v != "" {
			ed = v
			break
		}
	}
	if ed == "" {
		if _, err := exec.LookPath("vi"); err != nil {
			return nil, errors.New("no editor found: set $EDITOR")
		}
		ed = "vi"
	}
	return append(append([]string{"sh", "-c", ed + ` "$@"`, "sh"}, args...), p), nil
}

// editorEnv hides the outer terminal's name from the editor: it runs in
// sshu's emulator, not there, and features it would ask that terminal for
// are not here.
func editorEnv() []string {
	var out []string
	for _, k := range []string{"TERM_PROGRAM", "TERM_PROGRAM_VERSION", "TERM_SESSION_ID", "KITTY_WINDOW_ID",
		"KITTY_PUBLIC_KEY", "ITERM_SESSION_ID", "ITERM_PROFILE", "LC_TERMINAL", "LC_TERMINAL_VERSION",
		"WEZTERM_EXECUTABLE", "WEZTERM_PANE", "GHOSTTY_RESOURCES_DIR"} {
		out = append(out, k+"=")
	}
	return out
}

// saveEdit writes the edit back if it changed. A remote file that changed
// meanwhile is not overwritten without asking.
func saveEdit(job editJob, force bool) tea.Cmd {
	return func() tea.Msg {
		now, err := remote.Digest(job.local)
		if err != nil {
			return editSaved{job: job, err: err}
		}
		if now == job.digest {
			return editSaved{job: job}
		}
		if job.dir == "" {
			return editSaved{job: job, changed: true}
		}
		if !force {
			if cur, serr := remote.StampOf(job.fsys, job.path); serr == nil && cur != job.stamp {
				return editSaved{job: job, changed: true, conflict: true}
			}
		}
		if err := remote.WriteBack(job.fsys, job.local, job.path, job.mode); err != nil {
			return editSaved{job: job, changed: true, err: err}
		}
		return editSaved{job: job, changed: true}
	}
}

func (t *transfer) onEditSaved(m editSaved) tea.Cmd {
	a := t.s.a
	name := path.Base(m.job.path)
	where := m.job.fsys.Label()
	switch {
	case m.conflict:
		// Esc keeps the copy where it is, and says where (tdp F6).
		t.editing = false
		return a.Confirm(terminu.Ask{Title: "Overwrite", Subject: name,
			Details: []string{name + " changed on " + where + " since you opened it.", "Saving replaces what is there now.",
				"[Esc] keeps your edit at " + m.job.local + "."}, Warn: []int{1},
			Question: "Overwrite it?", Verb: "overwrite",
			OnCancel: func() tea.Cmd {
				t.endEdit(m.job, true)
				t.s.logChange("edit: not saved " + name + " (" + where + " changed); your copy is at " + m.job.local)
				return a.Toast(terminu.Warning, "Not saved: your edit is at "+m.job.local)
			}},
			func() tea.Cmd { return saveEdit(m.job, true) })
	case m.err != nil:
		t.endEdit(m.job, true)
		t.s.logError("error", "Can't save the edit of "+name+": "+m.err.Error(), "your copy is at "+m.job.local)
		return a.Error("Edit failed", "Not saved: "+m.err.Error(), "Your copy is at "+m.job.local)
	case !m.changed:
		t.endEdit(m.job, false)
		return a.Toast(terminu.Info, "No changes to "+name)
	}
	t.endEdit(m.job, false)
	t.s.logChange("edit: saved " + name + " (" + where + ")")
	for _, sd := range t.sides {
		sd.reload()
	}
	return a.Toast(terminu.Success, "Saved "+name)
}

func (t *transfer) endEdit(job editJob, keep bool) {
	t.editing = false
	if !keep && job.dir != "" {
		_ = os.RemoveAll(job.dir)
	}
}
