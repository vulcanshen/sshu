package ui

import (
	"errors"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/vulcanshen/sshu/internal/store"
)

var errTestDisk = errors.New("disk full")

// ------------------------------------------------------------------ the split

// A failed connection is recorded TWICE, and the two say different things.
// History has the result and nothing else; Errors has the reason. Recording
// the reason in both would give one failure two accounts that can disagree —
// which is what one log holding everything already did.
func TestAFailureIsInBothJournalsSayingDifferentThings(t *testing.T) {
	var h connectionsModel
	var e errorsModel
	h.add("prod-web-01", "deploy", false)
	e.errorf("prod-web-01", "deploy", "prod-web-01 · Connection refused")

	hist := ansi.Strip(strings.Join(h.rows(90), "\n"))
	if !strings.Contains(hist, "prod-web-01") || !strings.Contains(hist, store.ResultFail) {
		t.Errorf("history should carry host and result: %q", hist)
	}
	if strings.Contains(hist, "Connection refused") {
		t.Errorf("the REASON belongs to Errors alone, history has it: %q", hist)
	}

	errs := ansi.Strip(strings.Join(e.body(90, 8), "\n"))
	if !strings.Contains(errs, "Connection refused") {
		t.Errorf("errors should carry the reason: %q", errs)
	}
}

// Warnings live in Errors — they have nowhere else to go and they are the same
// kind of news — but they do NOT raise the badge. The badge means "something
// failed"; diluting it with "something was mentioned" is how a badge stops
// being looked at.
func TestOnlyErrorsCountAsUnread(t *testing.T) {
	var m errorsModel
	m.warn("", "", "config.yaml: line 3 is not a setting")
	if got := m.unreadErrors(); got != 0 {
		t.Errorf("a warning must not raise the badge, unread = %d", got)
	}
	m.errorf("prod-web-01", "deploy", "Connection refused")
	if got := m.unreadErrors(); got != 1 {
		t.Errorf("an error must raise it, unread = %d", got)
	}
	// ...and both are still shown: the panel holds more than the badge counts.
	if body := ansi.Strip(strings.Join(m.body(90, 8), "\n")); !strings.Contains(body, "line 3") {
		t.Errorf("the warning must still be in the panel: %q", body)
	}
}

// Red is "something is wrong", peach is "worth knowing, nothing is broken".
// The old log painted both red, so the panel could not tell you which was
// which without reading every word (§11.48 split the band, §11.49 uses it).
func TestWarningsAndErrorsAreToldApartByColour(t *testing.T) {
	withColour(t)
	var warnOnly, errOnly errorsModel
	warnOnly.warn("", "", "a note")
	errOnly.errorf("", "", "a failure")

	wr, _ := warnOnly.current()
	er, _ := errOnly.current()
	w := warnOnly.row(wr, false, 90)
	e := errOnly.row(er, false, 90)
	if !strings.Contains(w, ansiOf(t, peachColor)) {
		t.Errorf("a warning's timestamp should be peach: %q", w)
	}
	if !strings.Contains(e, ansiOf(t, warnColor)) {
		t.Errorf("an error's timestamp should be red: %q", e)
	}
	if strings.Contains(w, ansiOf(t, warnColor)) {
		t.Errorf("a warning must not wear the error tone: %q", w)
	}
}

// ---------------------------------------------------------------- the history

func TestHistoryColoursItsResult(t *testing.T) {
	withColour(t)
	var m connectionsModel
	m.add("prod-web-01", "deploy", true)
	m.add("db-01", "postgres", false)

	rows := strings.Join(m.rows(90), "\n")
	if !strings.Contains(rows, ansiOf(t, liveColor)) {
		t.Errorf("a success should be green: %q", ansi.Strip(rows))
	}
	if !strings.Contains(rows, ansiOf(t, warnColor)) {
		t.Errorf("a failure should be red: %q", ansi.Strip(rows))
	}
}

// One line per attempt, fixed height, newest first. The panel exists to be
// counted down a column — "that host, three times, two of them red" — and a
// row that can grow breaks the count.
func TestHistoryIsOneRowPerAttemptNewestFirst(t *testing.T) {
	var m connectionsModel
	m.add("first", "u", true)
	m.add("second", "u", true)
	m.add("third", "u", false)

	rows := m.rows(90)
	if len(rows) != 3 {
		t.Fatalf("want one row per attempt, got %d", len(rows))
	}
	if got := ansi.Strip(rows[0]); !strings.Contains(got, "third") {
		t.Errorf("newest first, got %q", got)
	}
}

// The status slot says how many and how many of them failed, because "did any
// of these go wrong" is the question the panel is opened with.
func TestHistoryStatusCountsFailures(t *testing.T) {
	var m connectionsModel
	if got := m.status(); got != "no connections" {
		t.Errorf("empty status is %q", got)
	}
	m.add("a", "u", true)
	if got := m.status(); strings.Contains(got, "failed") {
		t.Errorf("nothing failed, status should not mention it: %q", got)
	}
	m.add("b", "u", false)
	if got := m.status(); !strings.Contains(got, "1 failed") {
		t.Errorf("want the failure counted, got %q", got)
	}
}

// A host or user that is not known shows the placeholder rather than a blank
// column — the same statement the hosts table's tag line makes.
func TestAJournalRowWithNoUserShowsThePlaceholder(t *testing.T) {
	var m connectionsModel
	m.add("prod-web-01", "", true)
	if got := ansi.Strip(m.rows(90)[0]); !strings.Contains(got, jNone) {
		t.Errorf("want the placeholder where the user would be: %q", got)
	}
}

// --------------------------------------------------------------- the activity

func TestActivityKeepsWhatWasChanged(t *testing.T) {
	var m changesModel
	m.add(`host "prod-web-01" added (deploy@10.0.3.14:22)`)
	m.add(`credential "ops" deleted`)

	rows := ansi.Strip(strings.Join(m.body(90, 8), "\n"))
	if !strings.Contains(rows, `host "prod-web-01" added`) ||
		!strings.Contains(rows, `credential "ops" deleted`) {
		t.Errorf("both changes should be here: %q", rows)
	}
	// Newest first, like every journal — row 0 is the header.
	if first := ansi.Strip(m.body(90, 8)[1]); !strings.Contains(first, "deleted") {
		t.Errorf("newest first, got %q", first)
	}
}

// ------------------------------------------------------------------- the sink

// Each journal writes through to its OWN file. Three sinks, three files: a
// failure landing in connections.yaml would put the reason where the reason is
// deliberately not kept.
func TestEachJournalWritesToItsOwnSink(t *testing.T) {
	var errs []store.ErrorEntry
	var hist []store.ConnectionEntry
	var acts []store.ChangeEntry

	var e errorsModel
	var h connectionsModel
	var a changesModel
	e.sink = func(x store.ErrorEntry) error { errs = append(errs, x); return nil }
	h.sink = func(x store.ConnectionEntry) error { hist = append(hist, x); return nil }
	a.sink = func(x store.ChangeEntry) error { acts = append(acts, x); return nil }

	e.errorf("prod-web-01", "deploy", "Connection refused")
	h.add("prod-web-01", "deploy", false)
	a.add(`host "prod-web-01" added`)

	if len(errs) != 1 || errs[0].Host != "prod-web-01" || errs[0].Level != store.LevelError {
		t.Errorf("errors sink got %+v", errs)
	}
	if len(hist) != 1 || hist[0].Result != store.ResultFail {
		t.Errorf("history sink got %+v", hist)
	}
	if len(acts) != 1 || !strings.Contains(acts[0].Action, "added") {
		t.Errorf("activity sink got %+v", acts)
	}
}

// A journal whose file cannot be written says so ONCE and keeps working in
// memory. Complaining per event would fill the panel with the complaint.
func TestABrokenSinkComplainsOnceAndKeepsGoing(t *testing.T) {
	var m errorsModel
	calls := 0
	m.sink = func(store.ErrorEntry) error { calls++; return errTestDisk }

	m.errorf("a", "u", "first")
	m.errorf("b", "u", "second")
	m.errorf("c", "u", "third")

	if calls != 1 {
		t.Errorf("a broken sink should be tried once, got %d calls", calls)
	}
	// The complaint's own row shows its cause; the sentence about what happens
	// next is the detail behind it, like any other entry.
	body := ansi.Strip(strings.Join(m.body(90, 10), "\n"))
	if n := strings.Count(body, "cannot be written"); n != 1 {
		t.Errorf("the complaint should appear exactly once, got %d:\n%s", n, body)
	}
	for _, want := range []string{"first", "second", "third"} {
		if !strings.Contains(body, want) {
			t.Errorf("%q was lost when the sink broke:\n%s", want, body)
		}
	}
}

// A preloaded journal is all READ: everything in the file predates this run,
// so nothing in it is news.
func TestPreloadedEntriesAreNotUnread(t *testing.T) {
	var m errorsModel
	m.preload([]store.ErrorEntry{
		{Host: "a", User: "u", Level: store.LevelError, Error: "old failure"},
	})
	if got := m.unreadErrors(); got != 0 {
		t.Errorf("what was already on disk is not news, unread = %d", got)
	}
	if body := ansi.Strip(strings.Join(m.body(90, 8), "\n")); !strings.Contains(body, "old failure") {
		t.Errorf("it should still be shown: %q", body)
	}
}
