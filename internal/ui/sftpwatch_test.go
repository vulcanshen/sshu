package ui

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/vulcanshen/sshu/internal/remote"
)

// probeNow runs one refresh cycle synchronously: the probe the tick would have
// launched, and the message it would have produced.
func probeNow(t *testing.T, m AppModel, sd side) AppModel {
	t.Helper()
	s := m.sftp.sides[sd]
	next, _ := m.Update(watchProbe(sd, s.fs, s.cwd, s.watchMTime)())
	return next.(AppModel)
}

// A file that appears while sshu is looking at the directory shows up without
// anyone pressing anything.
func TestWatchPicksUpANewFile(t *testing.T) {
	m := sftpFixture(t, 100, 26)
	m.sftp.focus = panelLeftFiles
	cwd := m.sftp.sides[sideLeft].cwd
	before := m.sftp.sides[sideLeft].rowCount()

	if err := os.WriteFile(filepath.Join(cwd, "arrived.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Pin the baseline to something definitely older, so the test does not depend
	// on the filesystem's timestamp granularity to notice the write.
	m.sftp.sides[sideLeft].watchMTime = time.Unix(1, 0)

	m = probeNow(t, m, sideLeft)
	if got := m.sftp.sides[sideLeft].rowCount(); got != before+1 {
		t.Fatalf("listing has %d rows, want %d", got, before+1)
	}
	found := false
	for i := 0; i < m.sftp.sides[sideLeft].rowCount(); i++ {
		if e, _ := m.sftp.sides[sideLeft].rowAt(i); e.Name == "arrived.txt" {
			found = true
		}
	}
	if !found {
		t.Error("the new file is not in the refreshed listing")
	}
}

// The cheap half is the point: when nothing changed, the probe costs one stat
// and carries no listing back.
func TestWatchDoesNotRelistAnUnchangedDirectory(t *testing.T) {
	m := sftpFixture(t, 100, 26)
	s := m.sftp.sides[sideLeft]

	msg, ok := watchProbe(sideLeft, s.fs, s.cwd, s.watchMTime)().(watchResultMsg)
	if !ok {
		t.Fatal("probe did not answer with a watchResultMsg")
	}
	if msg.changed || msg.entries != nil {
		t.Errorf("an unchanged directory was re-listed: changed=%v, %d entries",
			msg.changed, len(msg.entries))
	}
	if msg.mtime.IsZero() {
		t.Error("the probe should still report the timestamp it saw")
	}
}

// A refresh must not slide the cursor onto a different file. If it did, a file
// appearing above the cursor would silently re-aim the next `t`.
func TestWatchKeepsTheCursorOnTheSameEntry(t *testing.T) {
	s := &sftpSideModel{markedSet: map[string]bool{}, cwd: "/srv"}
	s.entries = []remote.Entry{{Name: "b.txt"}, {Name: "c.txt"}}
	s.cursor = 1 // on c.txt

	s.applyWatch([]remote.Entry{{Name: "a.txt"}, {Name: "b.txt"}, {Name: "c.txt"}},
		time.Unix(2, 0))

	if e, _ := s.rowAt(s.cursor); e.Name != "c.txt" {
		t.Errorf("the cursor moved to %q when a file appeared above it", e.Name)
	}
}

// A probe that comes back after the side has moved on is not an answer to any
// question the side is asking.
func TestWatchDropsAStaleResult(t *testing.T) {
	m := sftpFixture(t, 100, 26)
	before := m.sftp.sides[sideLeft].rowCount()

	next, _ := m.Update(watchResultMsg{
		sd: sideLeft, cwd: "/somewhere/else", changed: true,
		entries: []remote.Entry{{Name: "wrong.txt"}}, mtime: time.Unix(9, 0),
	})
	m = next.(AppModel)

	if got := m.sftp.sides[sideLeft].rowCount(); got != before {
		t.Errorf("a result for another directory was installed: %d rows, want %d",
			got, before)
	}
}

// A tab nobody is looking at should not be working the connection.
func TestWatchStopsWhenTheTabIsNotOnScreen(t *testing.T) {
	m := sftpFixture(t, 100, 26)
	m.sftp.onScreen = true
	if m.sftp.rearmWatch() == nil {
		t.Fatal("the loop should run while the tab is on screen")
	}
	m.sftp.onScreen = false
	if m.sftp.rearmWatch() != nil {
		t.Error("the loop should stop when the tab is not on screen")
	}

	// And leaving the tab through the digits is what turns it off.
	m.sftp.onScreen = true
	m = pressA(m, "M")
	if m.sftp.onScreen {
		t.Error("leaving for tab [1] should stop the refresh loop")
	}
	m = pressA(m, "F")
	if !m.sftp.onScreen {
		t.Error("coming back to tab [2] should start it again")
	}
}

// A superseded loop dies rather than doubling the probes.
func TestWatchRetiresASupersededLoop(t *testing.T) {
	m := sftpFixture(t, 100, 26)
	m.sftp.onScreen = true
	m.sftp.startWatch()
	stale := watchTickMsg{gen: m.sftp.watchGen}
	m.sftp.startWatch() // a second loop retires the first

	if cmd := m.sftp.onWatchTick(stale); cmd != nil {
		t.Error("a tick from the retired loop should do nothing")
	}
	if m.sftp.sides[sideLeft].probing {
		t.Error("the retired tick launched a probe")
	}
}
