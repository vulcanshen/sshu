package tui

import (
	"archive/zip"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/vulcanshen/sshu/v3/internal/remote"
	"github.com/vulcanshen/sshu/v3/internal/store"
)

// Esc on what ssh asked answers ssh no, so ssh gives up instead of waiting
// (tdp F6): the password question and the host key question both.
func TestEscOnWhatSSHAskedAnswersNo(t *testing.T) {
	for _, prompt := range []string{"u@box's password: ", "Are you sure you want to continue connecting (yes/no/[fingerprint])? "} {
		hs := newHarness(t, 100, 30, testData(sampleHosts(), sampleCreds()))
		sd := hs.s.ft.sides[0]
		srv, err := newAskpassServer(0, sd.dialGen)
		if err != nil {
			t.Fatal(err)
		}
		sd.askpass, sd.dialing = srv, "box"
		ours, ssh := net.Pipe()
		hs.send(askpassRequestMsg{req: &askpassRequest{side: 0, gen: sd.dialGen, prompt: prompt, conn: ours}})
		hs.contains("box")
		got := make(chan error, 1)
		go func() { _, err := io.ReadAll(ssh); got <- err }()
		hs.keys("Esc")
		select {
		case err := <-got:
			if err != nil {
				t.Errorf("%q: ssh read %v, want the answer closed", prompt, err)
			}
		case <-time.After(2 * time.Second):
			t.Errorf("%q: Esc did not answer ssh", prompt)
		}
		srv.close()
	}
}

// The Jobs menu follows its transfers while it is open, and a running one's
// Enter says what it does: cancel (tdp menu.5).
func TestTheJobsMenuFollowsItsTransfers(t *testing.T) {
	hs := newHarness(t, 100, 30, testData(sampleHosts(), sampleCreds()))
	j := &job{id: 1, label: "5 files → box:/tmp", files: 5, cancel: func() {}}
	j.state.Store(int32(jobRunning))
	hs.s.ft.jobs = append(hs.s.ft.jobs, j)
	hs.keys("J")
	hs.contains("0/5 files")
	hs.contains("Enter:cancel")
	j.filesDone.Store(2)
	hs.contains("2/5 files")
}

// A remote host's files in a search get the lib's preview through
// previewFS: what a side's file system says, as the preview asks it.
func TestPreviewFSGivesTheSidesFiles(t *testing.T) {
	dir := t.TempDir()
	p := filepath.ToSlash(filepath.Join(dir, "notes.txt"))
	if err := os.WriteFile(p, []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	pfs := newPreviewFS(remote.Local())
	if fi, err := pfs.Stat(p); err != nil || fi.Size() != 6 || fi.IsDir() {
		t.Fatalf("Stat = %v, %v", fi, err)
	}
	f, err := pfs.Open(p)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(f)
	if string(b) != "hello\n" {
		t.Errorf("Open read %q", b)
	}
	if fis, err := pfs.ReadDir(filepath.ToSlash(dir)); err != nil || len(fis) != 1 || fis[0].Name() != "notes.txt" {
		t.Errorf("ReadDir = %v, %v", fis, err)
	}
	if fi, err := pfs.Stat(filepath.ToSlash(dir)); err != nil || !fi.IsDir() || !fi.Mode().IsDir() {
		t.Errorf("a directory's Stat = %v, %v", fi, err)
	}
}

// Below sshu's least size only its name and how big a terminal it needs are
// drawn; at it, the screen (tdp L1).
func TestTooSmallATerminalSaysHowBig(t *testing.T) {
	hs := newHarness(t, minWidth-1, minHeight, testData(sampleHosts(), sampleCreds()))
	hs.contains("needs a terminal of at least 60×20")
	hs.lacks("SECTIONS")
	hs = newHarness(t, minWidth, minHeight, testData(sampleHosts(), sampleCreds()))
	hs.contains("SECTIONS")
}

// A hosts table too narrow for its columns shortens its long columns first,
// then takes whole columns away, Tags first; the name never goes, and a long
// name does not squeeze the short columns (tdp panel.5).
func TestANarrowHostsTableTakesWholeColumnsAway(t *testing.T) {
	hs := newHarness(t, 140, 30, testData(sampleHosts(), sampleCreds()))
	hs.contains("TAGS")
	hs = newHarness(t, 60, 30, testData(sampleHosts(), sampleCreds()))
	hs.keys("2")
	hs.lacks("TAGS")
	hs.contains("NAME")

	long := append(sampleHosts(), store.Host{Name: "a-name-far-longer-than-any-column-could-ever-hold-whole",
		Host: "h", Port: 22, User: "u", Auth: store.AuthPassword})
	hs = newHarness(t, 120, 30, testData(long, sampleCreds()))
	hs.keys("2")
	hs.lacks("a-name-far-longer-than-any-column-could-ever-hold-whole") // cut
	hs.contains("a-name-far")
	for _, short := range []string{"PORT", "AUTH", "2222", "privatekey"} {
		hs.contains(short) // the short columns whole
	}
}

// A transfer from the Space menu ends the same way whether it asked about
// overwriting first or not: the confirm is only a door, and the menu closes
// once the transfer starts (tdp T1).
func TestATransferThatAsksEndsAsOneThatDoesNot(t *testing.T) {
	root := twoDirs(t)
	if err := os.WriteFile(filepath.Join(root, "b", "x.txt"), []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	hs := newHarness(t, 120, 36, testData(nil, nil))
	menu := "everything under here"     // a row only the Space menu has
	hs.keys("F", "H", "Enter", "Enter") // [1] local, into a/
	hs.keys("3", "H", "Enter", "j", "Enter")
	hs.keys("1", "Space")
	hs.contains(menu)
	hs.keys("t")
	hs.contains("1 will overwrite")
	hs.keys("Esc")
	hs.contains(menu) // Esc on the door goes back to the menu
	hs.keys("t", "Enter")
	hs.lacks(menu)
	hs.waitFor(func() bool {
		b, _ := os.ReadFile(filepath.Join(root, "b", "x.txt"))
		return string(b) == "hello\n" && hs.s.ft.running() == 0
	}, "the overwrite")
}

// A remote preview reads anywhere in the file: the end of a zip far past
// 64 KB lists what is in it (tdp file-picker.3).
func TestARemotePreviewReadsAZipToItsEnd(t *testing.T) {
	dir := t.TempDir()
	p := filepath.ToSlash(filepath.Join(dir, "big.zip"))
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	big := make([]byte, 200<<10)
	for i := range big {
		big[i] = byte(i*7919 + i/13) // little to squeeze: the zip ends far past 64 KB
	}
	for _, name := range []string{"one.bin", "two.bin"} {
		w, err := zw.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Store})
		if err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write(big)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	st, _ := f.Stat()
	f.Close()
	pf, err := newPreviewFS(remote.Local()).Open(p)
	if err != nil {
		t.Fatal(err)
	}
	defer pf.Close()
	zr, err := zip.NewReader(pf, st.Size())
	if err != nil {
		t.Fatalf("the zip's end could not be read: %v", err)
	}
	if len(zr.File) != 2 {
		t.Errorf("%d files listed, want 2", len(zr.File))
	}
	all, err := io.ReadAll(pf)
	if err != nil || int64(len(all)) != st.Size() {
		t.Errorf("read %d of %d bytes (%v)", len(all), st.Size(), err)
	}
}
