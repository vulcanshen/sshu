package ui

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/vulcanshen/sshu/internal/remote"
)

// `/` searches the whole subtree, not just the directory on screen.
//
// It is drawn in place rather than in a popup, and that is the decision the rest
// of this file follows from. A result is an ordinary row in panel [4]/[6], so
// `a` marks it, `t` sends it and Enter descends into it — the same keys, doing
// the same things, on a row that happens to have come from three directories
// down. A finder popup would have had to invent a "reveal" step and then hand
// the user back to the panel to do the thing they actually opened it for.
//
// The cost of drawing in place is that the cursor is live in the same list that
// results are streaming into, which is what shapes the two rules below: arrival
// order is never re-sorted, and an empty query shows only the current directory.

// scanTickMsg folds whatever the walk has found into the panel. It is the same
// shape as xferTickMsg: a goroutine does the slow work, the tick is what makes
// it visible, and it stops when nothing is running.
type scanTickMsg struct{}

const scanTickEvery = 120 * time.Millisecond

func scanTickCmd() tea.Cmd {
	return tea.Tick(scanTickEvery, func(time.Time) tea.Msg { return scanTickMsg{} })
}

// searchScan is one running walk. The walker appends under the lock and the
// render path never touches it — the UI thread only takes what has piled up,
// once per tick, so a frame is never waiting on the network.
type searchScan struct {
	mu     sync.Mutex
	buf    []remote.Entry
	done   bool
	capped bool
	cancel context.CancelFunc
}

func (sc *searchScan) push(batch []remote.Entry) {
	sc.mu.Lock()
	sc.buf = append(sc.buf, batch...)
	sc.mu.Unlock()
}

func (sc *searchScan) finish(capped bool) {
	sc.mu.Lock()
	sc.done, sc.capped = true, capped
	sc.mu.Unlock()
}

// drain hands over everything found since the last call and empties the buffer.
func (sc *searchScan) drain() (batch []remote.Entry, done, capped bool) {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	batch, sc.buf = sc.buf, nil
	return batch, sc.done, sc.capped
}

// stop is safe on a nil scan, so every exit from the filter can call it without
// first asking whether one was ever started.
func (sc *searchScan) stop() {
	if sc != nil && sc.cancel != nil {
		sc.cancel()
	}
}

// ------------------------------------------------------------- side wiring

// startFilter opens the query and starts the walk.
//
// The current directory's entries are already in hand, so they ARE the first
// results and the walk starts one level down — no round trip before the panel
// has something to show, and nothing listed twice.
func (s *sftpSideModel) startFilter() tea.Cmd {
	if s.fs == nil {
		return nil
	}
	s.filtering, s.query = true, ""
	s.found = append(s.found[:0], s.entries...)
	s.cursor, s.top = 0, 0
	s.refilter()
	return s.startScan()
}

func (s *sftpSideModel) startScan() tea.Cmd {
	sub := make([]string, 0, 8)
	for _, e := range s.entries {
		if e.IsDir {
			sub = append(sub, remote.Join(s.cwd, e.Name))
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	sc := &searchScan{cancel: cancel}
	s.scan, s.scanning, s.capped = sc, true, false

	fsys, root := s.fs, s.cwd
	go func() {
		sc.finish(remote.Scan(ctx, fsys, root, sub, remote.SearchCap, sc.push))
	}()
	return scanTickCmd()
}

// clearFilter drops the query and stops the walk. Leaving the search has to stop
// the round trips too — otherwise Esc looks free while the connection is still
// being worked.
//
// It also lands the cursor on the row it was on. A result from three levels down
// is not in this directory at all, so the lookup below simply does not find it
// and the cursor goes to the top — which is the honest answer for a row that is
// not here. That needs no special case; it falls out of looking it up by name.
func (s *sftpSideModel) clearFilter() {
	want := ""
	if e, ok := s.rowAt(s.cursor); ok {
		want = e.Name
	}

	s.scan.stop()
	s.scan, s.scanning, s.capped = nil, false, false
	s.filtering, s.query, s.found, s.matches = false, "", nil, nil

	s.cursor, s.top = 0, 0
	if want != "" {
		for i, e := range s.entries {
			if e.Name == want {
				s.cursor = i
				break
			}
		}
	}
}

// takeScan folds this side's arrived results into the listing. It reports
// whether the walk is still running, which is what keeps the tick alive.
func (s *sftpSideModel) takeScan() bool {
	if s.scan == nil {
		return false
	}
	batch, done, capped := s.scan.drain()
	s.found = append(s.found, batch...)
	s.scanning, s.capped = !done, capped
	if len(batch) > 0 {
		s.refilter()
	}
	return s.scanning
}

// refilter reruns the match over everything found so far.
//
// Two rules, both from drawing in place:
//
// An empty query is not "everything below here" — it is the directory you are
// standing in. Pressing `/` and typing nothing must leave the listing exactly as
// it was; the subtree comes into view only once there is something to look for.
//
// Matches keep ARRIVAL order and are never ranked by score. Results stream in
// while the cursor is live in the same list, so re-sorting on each batch would
// move the row out from under the user's hand between one keypress and the next.
// Breadth-first arrival is already the ranking that matters here: what is near
// is what they meant. (filu ranks by score, but its finder is a popup whose
// cursor is parked until the query is finished.)
func (s *sftpSideModel) refilter() {
	s.matches = s.matches[:0]
	for i, e := range s.found {
		if s.query == "" {
			if !strings.Contains(e.Name, "/") {
				s.matches = append(s.matches, i)
			}
			continue
		}
		if _, ok := fuzzyScore(e.Name, s.query); ok {
			s.matches = append(s.matches, i)
		}
	}
	s.cursor = clamp(s.cursor, 0, max(0, len(s.matches)-1))
	s.top = clamp(s.top, 0, max(0, len(s.matches)-1))
}

// filterKey edits the query. Letters type and arrows move — the same split the
// file picker and the host form make, so there is no mode to learn here either
// (§4.5). It reports whether it consumed the key.
func (s *sftpSideModel) filterKey(msg tea.KeyMsg) bool {
	if !s.filtering {
		return false
	}
	switch msg.Type {
	case tea.KeyRunes:
		s.query += string(msg.Runes)
	case tea.KeySpace:
		s.query += " "
	case tea.KeyBackspace:
		if r := []rune(s.query); len(r) > 0 {
			s.query = string(r[:len(r)-1])
		} else {
			s.clearFilter()
			return true
		}
	default:
		return false // arrows, Enter and Esc belong to the panel
	}
	// The query changed, so the cursor goes back to the top: after the list is
	// rebuilt, wherever it pointed is not where anyone is looking. (Streaming
	// results do NOT do this — see refilter.)
	s.cursor, s.top = 0, 0
	s.refilter()
	return true
}

// filterLabel is the query row shown in place of the path while searching.
//
// The prompt is the search glyph, not a literal "/". The key that opens the
// search is "/", but echoing it as the prompt makes a query that CONTAINS a
// slash unreadable — searching for /tmp would render as //tmp, and the user
// cannot tell which slash is theirs. The glyph says the same thing and cannot
// collide with what is being typed.
func (s sftpSideModel) filterLabel() string { return glyphSearch + " " + s.query }

// scanNote is the right-hand end of that row: how much of what has been seen is
// being shown, and whether the walk is still going. Both halves matter — a count
// that stops climbing means the search is finished, not that it stalled.
func (s sftpSideModel) scanNote() string {
	if !s.filtering {
		return ""
	}
	note := fmt.Sprintf("%d of %d", len(s.matches), len(s.found))
	switch {
	case s.scanning:
		note += " …"
	case s.capped:
		note += " capped"
	}
	return note
}

// ------------------------------------------------------------- tab wiring

// takeScans folds both sides in and keeps ticking while either is still walking.
// Both sides can search at once; they are independent connections.
func (m *sftpModel) takeScans() tea.Cmd {
	running := false
	for i := range m.sides {
		if m.sides[i].takeScan() {
			running = true
		}
	}
	if !running {
		return nil
	}
	return scanTickCmd()
}
