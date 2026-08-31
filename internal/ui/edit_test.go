package ui

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/vulcanshen/sshu/internal/remote"
)

// ------------------------------------------------------------ which editor

// The order is the user's choice first and vi only as a floor. vi is a fallback,
// not a dependency — nothing here should ever require it to be installed.
func TestEditorResolutionOrder(t *testing.T) {
	t.Setenv("VISUAL", "hx")
	t.Setenv("EDITOR", "nano")
	if got, _ := resolveEditor(); got != "hx" {
		t.Errorf("with $VISUAL set the editor is %q, want hx", got)
	}

	t.Setenv("VISUAL", "")
	if got, _ := resolveEditor(); got != "nano" {
		t.Errorf("with only $EDITOR set the editor is %q, want nano", got)
	}

	t.Setenv("EDITOR", "  ")
	got, err := resolveEditor()
	if err != nil {
		// No vi on this machine, which is the other legal answer — but then the
		// error has to say what to do about it.
		if !strings.Contains(err.Error(), "$EDITOR") {
			t.Errorf("the error does not name the fix: %v", err)
		}
	} else if got != fallbackEditor {
		t.Errorf("with nothing set the editor is %q, want %s", got, fallbackEditor)
	}
}

// $EDITOR is shell syntax, so it goes through sh — but the FILENAME must not.
// It comes off somebody else's machine, and a file called `; rm -rf ~` has to
// arrive as an argument, not as a second command.
func TestTheFilenameIsAnArgumentNotAScript(t *testing.T) {
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", "vim -u NONE")

	const nasty = `/tmp/x/; touch /tmp/pwned #`
	c, err := editorCommand(nasty)
	if err != nil {
		t.Fatal(err)
	}
	// The editor's own words survive: `vim -u NONE` is two arguments and a flag,
	// not a program called "vim -u NONE".
	script := c.Args[2]
	if !strings.HasPrefix(script, "vim -u NONE") {
		t.Errorf("the editor line was mangled: %q", script)
	}
	if strings.Contains(script, nasty) {
		t.Fatalf("the filename was interpolated into the script: %q", script)
	}
	if !slices.Contains(c.Args, nasty) {
		t.Errorf("the filename is not among the arguments: %q", c.Args)
	}
}

// The editor runs inside vt10x, which does not answer the queries a "capable
// terminal" gets asked. So it is not told it is talking to one — otherwise nvim
// waits for a reply that never comes and hangs on the way out.
func TestTheEditorIsNotToldWhichTerminalThisIs(t *testing.T) {
	t.Setenv("TERM", "xterm-ghostty")
	t.Setenv("TERM_PROGRAM", "ghostty")
	t.Setenv("KITTY_WINDOW_ID", "3")
	t.Setenv("COLORTERM", "truecolor")
	t.Setenv("SSHU_KEEP_ME", "yes")

	env := editorEnv()
	for _, banned := range []string{"TERM_PROGRAM=", "KITTY_WINDOW_ID=", "COLORTERM="} {
		for _, v := range env {
			if strings.HasPrefix(v, banned) {
				t.Errorf("%s reached the editor", strings.TrimSuffix(banned, "="))
			}
		}
	}
	if !slices.Contains(env, "TERM=xterm-256color") {
		t.Error("TERM is not pinned to what the emulator implements")
	}
	if slices.Contains(env, "TERM=xterm-ghostty") {
		t.Error("the outer terminal's TERM was passed through")
	}
	if !slices.Contains(env, "SSHU_KEEP_ME=yes") {
		t.Error("an unrelated variable was dropped")
	}
}

// ------------------------------------------------------------- the errand

// notLocal makes a real filesystem take the REMOTE path through edit.
// remote.LocalPath type-asserts, and a wrapper is not the type it looks for — so
// the fetch/temp/write-back half runs, against a disk the test can read.
type notLocal struct{ remote.FS }

func (notLocal) Label() string { return "elsewhere" }

// jobFor builds the errand the way sftpEdit does, with the file already fetched.
func jobFor(t *testing.T, target, body string) editJob {
	t.Helper()
	local := filepath.Join(t.TempDir(), filepath.Base(target))
	if err := os.WriteFile(local, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	digest, err := remote.Digest(local)
	if err != nil {
		t.Fatal(err)
	}
	stamp, err := remote.StampOf(notLocal{remote.Local()}, filepath.ToSlash(target))
	if err != nil {
		t.Fatal(err)
	}
	return editJob{
		fsys:   notLocal{remote.Local()},
		path:   filepath.ToSlash(target),
		mode:   0o644,
		stamp:  stamp,
		dir:    filepath.Dir(local),
		local:  local,
		digest: digest,
	}
}

// Opening a file and quitting is not a write. An editor rewrites on :w even when
// nothing changed, so mtime cannot answer this — the content has to.
func TestAnEditThatChangedNothingIsNotWrittenBack(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(target, []byte("port: 8080\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}

	job := jobFor(t, target, "port: 8080\n")
	msg, _ := saveEdit(1, job, false)().(editSavedMsg)
	if msg.changed {
		t.Error("an untouched file was reported as changed")
	}
	after, _ := os.Stat(target)
	if !after.ModTime().Equal(before.ModTime()) {
		t.Error("the file was rewritten even though nothing changed")
	}
}

// Somebody else wrote to the file while the editor was open. Replacing their
// work without a word is the one outcome this feature must not have.
func TestAFileThatChangedUnderneathIsNotOverwritten(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "hosts")
	if err := os.WriteFile(target, []byte("127.0.0.1 localhost\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	job := jobFor(t, target, "127.0.0.1 localhost\n")

	// Their change, and then ours.
	const theirs = "127.0.0.1 localhost\n10.0.0.2 db\n"
	if err := os.WriteFile(target, []byte(theirs), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(job.local, []byte("127.0.0.1 localhost\n8.8.8.8 dns\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	msg, _ := saveEdit(1, job, false)().(editSavedMsg)
	if !msg.conflict {
		t.Fatal("a file that moved underneath was saved over without asking")
	}
	body, _ := os.ReadFile(target)
	if string(body) != theirs {
		t.Errorf("their work was overwritten anyway: %q", body)
	}

	// And answering yes is what actually writes.
	forced, _ := saveEdit(1, job, true)().(editSavedMsg)
	if forced.err != nil || !forced.changed {
		t.Fatalf("the forced save did not go through: %+v", forced)
	}
	if body, _ := os.ReadFile(target); !strings.Contains(string(body), "8.8.8.8") {
		t.Errorf("the forced save wrote %q", body)
	}
}

// ---------------------------------------------------------------- the tab

// atFile writes a file into the left side's directory and parks the cursor on it.
func atFile(t *testing.T, m AppModel, name, body string) (AppModel, string) {
	t.Helper()
	p := filepath.Join(m.sftp.sides[sideLeft].cwd, name)
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	m.sftp.sides[sideLeft].reload()
	for i := range m.sftp.sides[sideLeft].rowCount() {
		if e, _ := m.sftp.sides[sideLeft].rowAt(i); e.Name == name {
			m.sftp.sides[sideLeft].cursor = i
		}
	}
	return m, p
}

func editFixture(t *testing.T, editor string, w, h int) AppModel {
	t.Helper()
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", editor)
	m := sftpFixture(t, w, h)
	m.sftp.focus = panelLeftFiles
	return m
}

// drive runs the model the way the runtime does — executing what Update returns
// and feeding the messages back — until done says so. The edit flow is genuinely
// asynchronous (a real PTY has to start, run and exit), so a test that only
// pressed the key would be asserting against the first frame.
func drive(t *testing.T, m AppModel, cmd tea.Cmd, done func(AppModel) bool) AppModel {
	t.Helper()
	queue := []tea.Cmd{cmd}
	deadline := time.Now().Add(10 * time.Second)
	for len(queue) > 0 && time.Now().Before(deadline) {
		c := queue[0]
		queue = queue[1:]
		if c == nil {
			continue
		}
		msg := c()
		if batch, ok := msg.(tea.BatchMsg); ok {
			queue = append(queue, batch...)
			continue
		}
		if msg == nil {
			continue
		}
		next, out := m.Update(msg)
		m = next.(AppModel)
		queue = append(queue, out)
		if done(m) {
			return m
		}
	}
	t.Fatal("the edit never reached its outcome")
	return m
}

// pressEdit presses e and runs the flow to its outcome.
func pressEdit(t *testing.T, m AppModel, done func(AppModel) bool) AppModel {
	t.Helper()
	next, cmd := m.Update(keyMsg("e"))
	return drive(t, next.(AppModel), cmd, done)
}

// End to end: the key starts a real editor on a real PTY, and what it wrote comes
// back. `sh` is standing in for vim, which is the whole point of not depending on
// any particular editor.
func TestEditRunsTheEditorAndSavesWhatItChanged(t *testing.T) {
	m := editFixture(t, `printf 'edited\n' >`, 110, 30)
	m, p := atFile(t, m, "notes.txt", "before\n")
	// The remote path, so the fetch and the write-back both run.
	m.sftp.sides[sideLeft].fs = notLocal{m.sftp.sides[sideLeft].fs}

	m = settle(pressEdit(t, m, func(m AppModel) bool { return m.toast.isActive() }))

	body, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "edited\n" {
		t.Errorf("the file holds %q, want what the editor wrote", body)
	}
	if view := ansi.Strip(m.View()); !strings.Contains(view, "Saved notes.txt") {
		t.Errorf("nothing said it was saved:\n%s", view)
	}
}

// On this machine there is nothing to fetch. Editing in place keeps the inode,
// so hard links and ownership survive — a copy and a rename would break both to
// protect against a network that is not involved.
func TestALocalFileIsEditedWhereItLives(t *testing.T) {
	m := editFixture(t, `printf 'in place\n' >`, 110, 30)
	m, p := atFile(t, m, "local.txt", "before\n")

	link := p + ".hardlink"
	if err := os.Link(p, link); err != nil {
		t.Skipf("no hard links here: %v", err)
	}

	m = pressEdit(t, m, func(m AppModel) bool { return m.toast.isActive() })

	if body, _ := os.ReadFile(p); string(body) != "in place\n" {
		t.Errorf("the file holds %q", body)
	}
	// The link still sees the edit, which is only true if the inode never moved.
	if body, _ := os.ReadFile(link); string(body) != "in place\n" {
		t.Errorf("the hard link was broken: it holds %q", body)
	}
}

// The test for text is "no NUL and valid UTF-8", and a Latin-1 config file fails
// it while being perfectly editable. So it asks — refusing would be a guess
// overruling a person about their own file.
func TestEditAsksBeforeOpeningSomethingThatIsNotText(t *testing.T) {
	m := editFixture(t, "true", 110, 30)
	m, _ = atFile(t, m, "blob.bin", "\x00\x01\x02binary")

	m = pressEdit(t, m, func(m AppModel) bool { return m.confirm.isActive() })
	m = settle(m)

	if m.confirm.action != confirmEditBinary {
		t.Fatalf("no question was asked; confirm action is %d", m.confirm.action)
	}
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "edit anyway") {
		t.Errorf("the question offers no way through:\n%s", view)
	}
}

// A directory, a device, a socket: there is nothing to read out and put back.
func TestEditRefusesWhatIsNotAFile(t *testing.T) {
	m := editFixture(t, "true", 110, 30)
	if err := os.Mkdir(filepath.Join(m.sftp.sides[sideLeft].cwd, "adir"), 0o755); err != nil {
		t.Fatal(err)
	}
	m.sftp.sides[sideLeft].reload()
	for i := range m.sftp.sides[sideLeft].rowCount() {
		if e, _ := m.sftp.sides[sideLeft].rowAt(i); e.Name == "adir" {
			m.sftp.sides[sideLeft].cursor = i
		}
	}
	next, _ := m.Update(keyMsg("e"))
	m = settle(next.(AppModel))

	if m.editorUI.isActive() {
		t.Error("an editor was opened on a directory")
	}
	if view := ansi.Strip(m.View()); !strings.Contains(view, "Cannot edit a directory") {
		t.Errorf("nothing said why:\n%s", view)
	}
}

// Two editors writing back in an order nobody can predict is not a feature.
//
// Two things stop it, and they cover different moments. While the box owns the
// keyboard, `e` never reaches the action table at all. While it is CLOSING it
// has already handed the keyboard back (popupAnimator.owns), and that is the
// window the refusal in sftpEdit is for — without it, starting again there would
// overwrite pendingEdit and strand the temp directory it was pointing at.
func TestOnlyOneEditorAtATime(t *testing.T) {
	m := editFixture(t, "true", 110, 30)
	m, _ = atFile(t, m, "one.txt", "a\n")
	m.editorUI.phase = editRunning
	_ = m.editorUI.anim.open()

	next, _ := m.Update(keyMsg("e"))
	if got := next.(AppModel).editorUI.name; got != "" {
		t.Errorf("the key reached the table and started %q", got)
	}

	// Now the closing window, where the key does get through.
	m.editorUI.pendingClose(t)
	after, _ := m.sftpEdit()
	m2 := settle(after.(AppModel))
	if view := ansi.Strip(m2.View()); !strings.Contains(view, "already open") {
		t.Errorf("a second editor was allowed to start:\n%s", view)
	}
}

// pendingClose puts the box in the state it is in while its closing animation
// runs: on screen, but no longer holding the keyboard.
func (m *editorPopup) pendingClose(t *testing.T) {
	t.Helper()
	_ = m.anim.close()
	if m.anim.owns() || !m.isActive() {
		t.Fatal("a closing popup should be visible and not own the keyboard")
	}
}

// The frame invariant, for a popup that is mostly somebody else's program.
func TestEditorPopupPreservesFrame(t *testing.T) {
	for _, sz := range [][2]int{{120, 40}, {100, 26}, {80, 20}, {60, 14}} {
		w, h := sz[0], sz[1]
		m := editFixture(t, "true", w, h)
		m, _ = atFile(t, m, "frame.txt", strings.Repeat("wide ", 200)+"\n")
		m.editorUI.phase = editFetching
		_ = m.editorUI.open(1, "frame.txt", 1000)
		m = settle(m)

		for i, line := range strings.Split(m.View(), "\n") {
			if got := dispW(ansi.Strip(line)); got != w {
				t.Fatalf("%dx%d line %d is %d cells, want %d:\n%q", w, h, i, got, w, line)
			}
		}
	}
}
