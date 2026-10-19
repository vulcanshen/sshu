package tui

import (
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/vulcanshen/sshu/internal/remote"
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
// Enter says what it does: cancel (components/dialog/menu, work lists).
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
// previewFS, which reads each thing once: the preview is drawn every frame
// and the host answers over the network.
func TestARemotePreviewReadsOnce(t *testing.T) {
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
	if err := os.Remove(p); err != nil {
		t.Fatal(err)
	}
	if _, err := pfs.Stat(p); err != nil {
		t.Error("Stat asked the host again: it should have kept what it read")
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

// A hosts table too narrow for its columns takes whole columns away, Tags
// first, before it narrows the rest; the name never goes (layout/panel).
func TestANarrowHostsTableTakesWholeColumnsAway(t *testing.T) {
	hs := newHarness(t, 140, 30, testData(sampleHosts(), sampleCreds()))
	hs.contains("TAGS")
	hs = newHarness(t, 60, 30, testData(sampleHosts(), sampleCreds()))
	hs.keys("2")
	hs.lacks("TAGS")
	hs.contains("NAME")
}
