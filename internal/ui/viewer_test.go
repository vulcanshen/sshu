package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/vulcanshen/sshu/internal/remote"
)

// viewing puts the cursor on a file it has just written and opens the viewer,
// running the load synchronously so the test is not racing the goroutine.
func viewing(t *testing.T, name, body string, w, h int) AppModel {
	t.Helper()
	m := sftpFixture(t, w, h)
	m.sftp.focus = panelLeftFiles

	cwd := m.sftp.sides[sideLeft].cwd
	p := filepath.Join(cwd, name)
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	m.sftp.sides[sideLeft].reload()
	for i := 0; i < m.sftp.sides[sideLeft].rowCount(); i++ {
		if e, _ := m.sftp.sides[sideLeft].rowAt(i); e.Name == name {
			m.sftp.sides[sideLeft].cursor = i
		}
	}

	m = pressA(m, "v")
	next, _ := m.Update(loadView(m.viewer.gen, m.sftp.sides[sideLeft].fs, p, false)())
	return settle(next.(AppModel))
}

// Text arrives with a line-number gutter, which is the whole point of reading a
// file rather than transferring it and hoping.
func TestViewShowsTextWithLineNumbers(t *testing.T) {
	m := viewing(t, "hello.txt", "alpha\nbeta\ngamma\n", 110, 26)
	if !m.viewer.isActive() {
		t.Fatal("v should open the viewer")
	}
	view := ansi.Strip(m.View())
	for _, want := range []string{"hello.txt", "1 alpha", "2 beta", "3 gamma"} {
		if !strings.Contains(view, want) {
			t.Errorf("the viewer does not show %q:\n%s", want, view)
		}
	}
}

// A glyph is a word (§3), and two surfaces wearing the same one say they mean
// the same thing. The viewer wore the search magnifier until it was seen on
// screen next to an actual search — `/` owns that symbol.
func TestTheViewerDoesNotWearTheSearchGlyph(t *testing.T) {
	m := viewing(t, "hello.txt", "alpha\n", 110, 26)
	view := m.View()
	if !strings.Contains(view, glyphEye) {
		t.Error("the viewer is not wearing its own glyph")
	}
	if strings.Contains(view, glyphSearch) {
		t.Error("the viewer is still wearing the search glyph")
	}
}

// A file that is not text is shown as hex rather than as mojibake.
func TestViewShowsBinaryAsHex(t *testing.T) {
	// A NUL is the deciding byte here: everything else in this content is
	// perfectly valid UTF-8, so if the NUL check went away it would be shown as
	// text. (The earlier fixture had an invalid byte too, which meant the NUL
	// branch was never what the test was actually exercising.)
	if isText([]byte("\x00\x01\x02ABC")) {
		t.Error("a NUL byte means this is not text")
	}
	if !isText([]byte("plain\n")) {
		t.Error("ordinary text should be text")
	}
	if isText([]byte("bad\xff")) {
		t.Error("invalid UTF-8 is not text either")
	}

	m := viewing(t, "blob.bin", "\x00\x01\x02ABC", 110, 26)
	view := ansi.Strip(m.View())
	if m.viewer.kind != viewBinary {
		t.Errorf("kind is %d, want binary", m.viewer.kind)
	}
	if !strings.Contains(view, "00000000") {
		t.Errorf("no hex offset column:\n%s", view)
	}
	if !strings.Contains(view, "|...ABC|") {
		t.Errorf("no ASCII gutter:\n%s", view)
	}
}

// A directory shows one level of its listing — over SFTP each level is another
// round trip, and "what is in here" is answered by the first one.
func TestViewShowsADirectoryListing(t *testing.T) {
	m := sftpFixture(t, 110, 26)
	m.sftp.focus = panelLeftFiles
	m.sftp.sides[sideLeft].cursor = 0 // "assets", a directory

	e, _ := m.sftp.cur().cursorEntry()
	if !e.IsDir {
		t.Fatalf("setup: the cursor is on %q, want a directory", e.Name)
	}
	p, _ := m.sftp.cur().cursorPath()
	if err := os.WriteFile(filepath.Join(p, "logo.png"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	m = pressA(m, "v")
	next, _ := m.Update(loadView(m.viewer.gen, m.sftp.sides[sideLeft].fs, p, true)())
	m = settle(next.(AppModel))

	if m.viewer.kind != viewDir {
		t.Errorf("kind is %d, want dir", m.viewer.kind)
	}
	if !strings.Contains(ansi.Strip(m.View()), "logo.png") {
		t.Error("the directory's contents are not shown")
	}
}

// Escape sequences in someone else's file must not reach the terminal. This
// matters more here than it did in filu: the bytes come off another machine.
func TestViewStripsControlSequences(t *testing.T) {
	m := viewing(t, "nasty.txt", "before\x1b[2J\x1b[31mafter\n", 110, 26)
	view := m.View()

	// What must be gone is the ESC BYTE. The characters after it survive as
	// ordinary text — "[2J" without its escape is three harmless characters — so
	// asserting THEY are absent would be asserting something that was never true.
	//
	// The check is for the sequence sshu itself never emits: it draws SGR
	// colours, never a clear-screen.
	if strings.Contains(view, "\x1b[2J") {
		t.Error("a clear-screen from the file reached the terminal")
	}
	plain := ansi.Strip(view)
	if !strings.Contains(plain, "before") || !strings.Contains(plain, "after") {
		t.Errorf("the text either side of it was lost:\n%s", plain)
	}
	// And a tab becomes spaces rather than a variable-width jump.
	m = viewing(t, "tabbed.txt", "a\tb\n", 110, 26)
	if strings.Contains(ansi.Strip(m.View()), "\t") {
		t.Error("a raw tab reached the screen")
	}
}

// It is a viewport: it scrolls, it has no cursor, and it does not wrap.
func TestViewScrollsAndDoesNotWrap(t *testing.T) {
	var b strings.Builder
	for i := 1; i <= 200; i++ {
		b.WriteString("line " + itoa(i) + "\n")
	}
	m := viewing(t, "long.txt", b.String(), 110, 26)

	if m.viewer.top != 0 {
		t.Fatalf("setup: expected to start at the top, got %d", m.viewer.top)
	}
	m = pressA(m, "k")
	if m.viewer.top != 0 {
		t.Error("scrolling up from the top should stop, not wrap")
	}
	m = pressA(m, "d")
	if m.viewer.top == 0 {
		t.Error("d should scroll half a page")
	}
	m = pressA(m, "G")
	last := m.viewer.top
	m = pressA(m, "j")
	if m.viewer.top != last {
		t.Error("scrolling past the end should stop, not wrap")
	}
}

// The read is capped: pressing a key on a multi-gigabyte log is not a mistake
// you have to wait out.
func TestViewIsCapped(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(root, "huge.bin")
	if err := os.WriteFile(p, make([]byte, remote.PeekCap*3), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := remote.Peek(remote.Local(), filepath.ToSlash(p), remote.PeekCap)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != remote.PeekCap {
		t.Errorf("read %d bytes, want the %d-byte cap", len(got), remote.PeekCap)
	}
}

// A preview the user has moved on from must not land on top of the one they are
// looking at now.
func TestASupersededViewCannotLand(t *testing.T) {
	m := viewing(t, "first.txt", "first\n", 110, 26)
	stale := m.viewer.gen - 1

	next, _ := m.Update(viewLoadedMsg{gen: stale, title: "other.txt",
		lines: []string{"wrong"}})
	m = next.(AppModel)

	if strings.Contains(ansi.Strip(m.View()), "wrong") {
		t.Error("a stale load was installed")
	}
}

// The frame invariant, with the popup composited over both panel layouts.
func TestViewerPreservesFrame(t *testing.T) {
	for _, sz := range [][2]int{{120, 40}, {110, 26}, {100, 26}, {80, 20}, {72, 16}, {50, 12}} {
		w, h := sz[0], sz[1]
		// One line far longer than any popup, because that is the line a
		// viewer is most likely to be handed and the only one that can shear
		// the frame. Highlighted, so it carries ANSI that must not be cut
		// mid-sequence either.
		long := "// " + strings.Repeat("wide ", 60) + "\n"
		m := viewing(t, "frame.go", "package main\n"+long+"func main() {}\n", w, h)
		for i, l := range strings.Split(m.View(), "\n") {
			if lw := dispW(l); lw != w {
				t.Errorf("%dx%d line %d is %d cells, want %d: %q", w, h, i, lw, w, l)
				break
			}
		}
	}
}
