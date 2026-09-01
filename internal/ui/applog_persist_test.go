package ui

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/vulcanshen/sshu/internal/store"
)

// Every event the panel shows is also written through to applogs.yaml, level
// and detail included — the file is the record that survives the process.
func TestTheLogWritesThrough(t *testing.T) {
	var got []store.LogEntry
	m := newAppLog()
	m.sink = func(e store.LogEntry) error { got = append(got, e); return nil }

	m.info("connected to demo")
	m.errorf("demo · refused", "ssh: connect to host x port 22: Connection refused")

	if len(got) != 2 {
		t.Fatalf("want 2 entries written through, got %d", len(got))
	}
	if got[0].Level != "info" || got[1].Level != "error" {
		t.Errorf("levels must survive: %q, %q", got[0].Level, got[1].Level)
	}
	if !strings.Contains(got[1].Msg, "Connection refused") ||
		!strings.Contains(got[1].Msg, "\n") {
		t.Errorf("the whole multi-line entry goes to disk, got %q", got[1].Msg)
	}
}

// A log that cannot be written complains exactly once, in the log itself, and
// keeps recording in memory — an error about recording errors must not recurse
// or repeat itself per event.
func TestASinkFailureComplainsOnce(t *testing.T) {
	m := newAppLog()
	m.sink = func(store.LogEntry) error { return errors.New("disk full") }

	m.info("one")
	m.info("two")
	m.errorf("three")

	warns := 0
	for _, e := range m.entries {
		if e.level == logWarn && strings.Contains(e.msg, "applogs.yaml") {
			warns++
		}
	}
	if warns != 1 {
		t.Fatalf("want exactly one complaint about the sink, got %d", warns)
	}
	// The events themselves are all still there.
	if len(m.entries) != 4 { // three events + one complaint
		t.Fatalf("in-memory recording must continue, have %d entries", len(m.entries))
	}
}

// What applogs.yaml already held is shown but not counted unread — it predates
// this run — and is not written back through the sink.
func TestPreloadedTailIsReadNotRewritten(t *testing.T) {
	var written int
	m := newAppLog()
	m.sink = func(store.LogEntry) error { written++; return nil }
	m.preload([]store.LogEntry{
		{At: time.Now().Add(-time.Hour), Level: "error", Msg: "old failure"},
	})

	if written != 0 {
		t.Fatalf("preloading must not re-write entries to disk, wrote %d", written)
	}
	if m.unreadErrors() != 0 {
		t.Fatalf("a preloaded error predates this run, unread = %d", m.unreadErrors())
	}
	rows := strings.Join(m.allRows(80), "\n")
	if !strings.Contains(rows, "old failure") {
		t.Error("the preloaded tail must be readable in the panel")
	}

	m.errorf("new failure")
	if m.unreadErrors() != 1 {
		t.Errorf("a fresh error still counts, unread = %d", m.unreadErrors())
	}
	rows = strings.Join(m.allRows(80), "\n")
	if strings.Index(rows, "new failure") > strings.Index(rows, "old failure") {
		t.Error("newest first: the fresh entry must render above the tail")
	}
}
