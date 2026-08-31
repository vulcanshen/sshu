package ui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/vulcanshen/sshu/internal/remote"
)

// Keeping a listing current, without a watch.
//
// SFTP has no change notification — the protocol has no such message, so there
// is nothing to subscribe to. The alternatives are running inotifywait over ssh
// (needs a tool installed on the far end, and it is "execute a command on the
// remote", which this tab deliberately does not do) or asking.
//
// So sshu asks — but it asks the cheap question. A full listing every second
// would be a real cost: ReadDir returns every entry with its attributes, which
// on a big directory over a slow link is seconds of traffic for a tab that is
// mostly idle. Instead each tick STATS THE DIRECTORY and compares its mtime,
// which is one small round trip; only when that moves does it re-list.
//
// What that buys: entries appearing, disappearing or being renamed show up on
// their own. What it does not: a file being written in place does not change its
// directory's mtime, so a growing log keeps its old size until something else
// re-lists. Leaving the directory and coming back is the manual refresh.
//
// Both halves of the probe are network calls, so they run on a goroutine and
// come back as a message. A background refresh must never make the UI wait.

// watchEvery is the probe interval. Two seconds is under the threshold where a
// stale listing starts to mislead, and it keeps the idle cost to one small
// round trip per side.
const watchEvery = 2 * time.Second

// watchTickMsg drives the loop. gen kills the ticks of a superseded loop, so
// re-entering the tab cannot leave two loops probing in parallel.
type watchTickMsg struct{ gen int }

// watchResultMsg is one probe's answer. entries is nil unless the directory
// actually changed — the common case carries nothing but a timestamp.
type watchResultMsg struct {
	sd      side
	cwd     string
	mtime   time.Time
	entries []remote.Entry
	changed bool
}

// startWatch begins a fresh loop and orphans any previous one.
func (m *sftpModel) startWatch() tea.Cmd {
	m.watchGen++
	return m.rearmWatch()
}

// rearmWatch schedules the next tick of the CURRENT loop, or stops if there is
// nothing to watch: no side connected, or the tab is not on screen. An idle sshu
// should cost nothing.
func (m sftpModel) rearmWatch() tea.Cmd {
	if !m.onScreen || (m.sides[0].fs == nil && m.sides[1].fs == nil) {
		return nil
	}
	gen := m.watchGen
	return tea.Tick(watchEvery, func(time.Time) tea.Msg { return watchTickMsg{gen: gen} })
}

// onWatchTick launches one probe per eligible side and re-arms.
func (m *sftpModel) onWatchTick(msg watchTickMsg) tea.Cmd {
	if msg.gen != m.watchGen {
		return nil // a superseded loop
	}
	cmds := []tea.Cmd{m.rearmWatch()}
	for i := range m.sides {
		s := &m.sides[i]
		// One probe at a time per side, and never while a search is walking —
		// that already has the connection busy, and its results are a snapshot
		// anyway.
		if s.fs == nil || s.cwd == "" || s.probing || s.scanning {
			continue
		}
		s.probing = true
		cmds = append(cmds, watchProbe(side(i), s.fs, s.cwd, s.watchMTime))
	}
	return tea.Batch(cmds...)
}

// watchProbe is the off-loop half: stat, and re-list only if the stat moved.
func watchProbe(sd side, fsys remote.FS, cwd string, known time.Time) tea.Cmd {
	return func() tea.Msg {
		st, err := fsys.Stat(cwd)
		if err != nil {
			// The connection may have gone or the directory may be gone. Either
			// way this is not the place to report it — the next thing the user
			// does will say so properly.
			return watchResultMsg{sd: sd, cwd: cwd, mtime: known}
		}
		if st.ModTime.Equal(known) {
			return watchResultMsg{sd: sd, cwd: cwd, mtime: st.ModTime}
		}
		entries, err := fsys.List(cwd)
		if err != nil {
			return watchResultMsg{sd: sd, cwd: cwd, mtime: known}
		}
		return watchResultMsg{sd: sd, cwd: cwd, mtime: st.ModTime,
			entries: entries, changed: true}
	}
}

// onWatchResult installs a refreshed listing, if it is still the answer to a
// question this side is asking.
func (m *sftpModel) onWatchResult(msg watchResultMsg) {
	s := &m.sides[msg.sd]
	s.probing = false
	if s.fs == nil || s.cwd != msg.cwd {
		return // the side moved on while the probe was in flight
	}
	if !msg.changed {
		s.watchMTime = msg.mtime
		return
	}
	s.applyWatch(msg.entries, msg.mtime)
}

// applyWatch swaps in a fresh listing, keeping the cursor on the SAME ENTRY
// rather than on the same row.
//
// That distinction is the whole reason this is not just an assignment: a file
// appearing above the cursor would otherwise slide it onto a different name
// without the user touching anything, and the next `t` would transfer something
// they did not mean.
func (s *sftpSideModel) applyWatch(entries []remote.Entry, mtime time.Time) {
	want := ""
	if e, ok := s.rowAt(s.cursor); ok {
		want = e.Name
	}
	s.entries, s.watchMTime, s.err = entries, mtime, ""

	// A search's results are a snapshot of when it ran; refreshing the directory
	// underneath one keeps it honest for when the search is dropped, but does not
	// disturb what is on screen.
	if s.filtering {
		return
	}
	if want != "" {
		for i, e := range s.entries {
			if e.Name == want {
				s.cursor = i
				break
			}
		}
	}
	s.cursor = clamp(s.cursor, 0, max(0, s.rowCount()-1))
	s.top = clamp(s.top, 0, max(0, s.rowCount()-1))
}

// dirMTime is the directory's own timestamp, or the zero time if it cannot be
// had. Callers take it BEFORE listing: a change landing between the two then
// shows up as one redundant refresh, which is the harmless direction. Taking it
// after would record the new timestamp against the old listing and the change
// would never be noticed at all.
func dirMTime(fsys remote.FS, dir string) time.Time {
	e, err := fsys.Stat(dir)
	if err != nil {
		return time.Time{}
	}
	return e.ModTime
}
