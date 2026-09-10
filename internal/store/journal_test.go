package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func at(sec int) time.Time {
	return time.Date(2026, 9, 10, 12, 0, sec, 0, time.UTC)
}

// ---------------------------------------------------------------- round trip

// Each journal survives the file, keeps its header, and lands at 0600 — the
// same handling every file in this directory gets, because all three name
// hosts and users and one of them keeps whatever the far end printed.
func TestEachJournalRoundTripsAtTheRightMode(t *testing.T) {
	dir := t.TempDir()
	ep := filepath.Join(dir, "errors.yaml")
	hp := filepath.Join(dir, "history.yaml")
	ap := filepath.Join(dir, "activity.yaml")

	if err := AppendErrorTo(ep, ErrorEntry{At: at(1), Host: "prod-web-01",
		User: "deploy", Level: LevelError, Error: "Connection refused"}); err != nil {
		t.Fatal(err)
	}
	if err := AppendHistoryTo(hp, HistoryEntry{At: at(2), Host: "prod-web-01",
		User: "deploy", Result: ResultSuccess}); err != nil {
		t.Fatal(err)
	}
	if err := AppendActivityTo(ap, ActivityEntry{At: at(3),
		Action: `host "prod-web-01" added`}); err != nil {
		t.Fatal(err)
	}

	for _, p := range []string{ep, hp, ap} {
		st, err := os.Stat(p)
		if err != nil {
			t.Fatal(err)
		}
		if perm := st.Mode().Perm(); perm != 0o600 {
			t.Errorf("%s: want 0600, got %o", filepath.Base(p), perm)
		}
		raw, _ := os.ReadFile(p)
		if !strings.HasPrefix(string(raw), "# sshu ") {
			t.Errorf("%s lost its warning header:\n%s", filepath.Base(p), raw)
		}
	}

	errs, err := loadErrorsFrom(ep)
	if err != nil || len(errs) != 1 {
		t.Fatalf("errors: %v, %d entries", err, len(errs))
	}
	if errs[0].Host != "prod-web-01" || errs[0].User != "deploy" ||
		errs[0].Level != LevelError || errs[0].Error != "Connection refused" {
		t.Errorf("errors round trip: %+v", errs[0])
	}

	hist, err := loadHistoryFrom(hp)
	if err != nil || len(hist) != 1 {
		t.Fatalf("history: %v, %d entries", err, len(hist))
	}
	if hist[0].Result != ResultSuccess || hist[0].Host != "prod-web-01" {
		t.Errorf("history round trip: %+v", hist[0])
	}

	acts, err := loadActivityFrom(ap)
	if err != nil || len(acts) != 1 {
		t.Fatalf("activity: %v, %d entries", err, len(acts))
	}
	if acts[0].Action != `host "prod-web-01" added` {
		t.Errorf("activity round trip: %+v", acts[0])
	}
}

// An append is an APPEND: the file stays one flat list rather than becoming a
// list of one-element lists. That is what makes recording an event cheap, and
// it is the property that silently breaks if the marshalling changes.
func TestAppendingKeepsTheFileOneFlatList(t *testing.T) {
	p := filepath.Join(t.TempDir(), "history.yaml")
	for i := 0; i < 5; i++ {
		if err := AppendHistoryTo(p, HistoryEntry{At: at(i), Host: "h", User: "u",
			Result: ResultSuccess}); err != nil {
			t.Fatal(err)
		}
	}
	got, err := loadHistoryFrom(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 5 {
		t.Fatalf("want 5 entries in one list, got %d:\n%s", len(got), mustRead(t, p))
	}
}

// An entry with empty host and user — a config file that would not parse —
// keeps those keys OUT of the file rather than writing empty strings, so a
// hand-read journal does not suggest a machine that was never involved.
func TestAnEntryWithNoHostOmitsTheKeys(t *testing.T) {
	p := filepath.Join(t.TempDir(), "errors.yaml")
	if err := AppendErrorTo(p, ErrorEntry{At: at(1), Level: LevelWarn,
		Error: "config.yaml: line 3 is not a setting"}); err != nil {
		t.Fatal(err)
	}
	raw := mustRead(t, p)
	if strings.Contains(raw, "host:") || strings.Contains(raw, "user:") {
		t.Errorf("empty host/user should be omitted:\n%s", raw)
	}
}

// ---------------------------------------------------------------------- trim

// A journal trims its own tail rather than growing without bound, and the trim
// keeps the NEWEST entries — the ones somebody is about to look at.
func TestAJournalTrimsItsOwnTail(t *testing.T) {
	p := filepath.Join(t.TempDir(), "activity.yaml")

	old := journalTrimBytes
	journalTrimBytes = 512
	defer func() { journalTrimBytes = old }()

	for i := 0; i < journalKeep+50; i++ {
		if err := AppendActivityTo(p, ActivityEntry{At: at(i % 60),
			Action: "entry " + itoaTest(i)}); err != nil {
			t.Fatal(err)
		}
	}
	got, err := loadActivityFrom(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) > journalKeep {
		t.Errorf("want at most %d entries kept, got %d", journalKeep, len(got))
	}
	// The last one written must still be there: trimming from the wrong end
	// would leave a journal that forgets what just happened.
	last := got[len(got)-1].Action
	if want := "entry " + itoaTest(journalKeep+49); last != want {
		t.Errorf("newest entry is %q, want %q", last, want)
	}
	if !strings.HasPrefix(mustRead(t, p), "# sshu ") {
		t.Error("the trim dropped the warning header")
	}
}

// --------------------------------------------------------------------- clear

func TestClearEmptiesTheFileButKeepsItsHeader(t *testing.T) {
	p := filepath.Join(t.TempDir(), "errors.yaml")
	if err := AppendErrorTo(p, ErrorEntry{At: at(1), Level: LevelError,
		Error: "boom"}); err != nil {
		t.Fatal(err)
	}
	if err := ClearJournalTo(p, ErrorsHeader()); err != nil {
		t.Fatal(err)
	}
	raw := mustRead(t, p)
	if !strings.HasPrefix(raw, "# sshu ") {
		t.Errorf("the header must survive a clear:\n%s", raw)
	}
	if strings.Contains(raw, "boom") {
		t.Errorf("the entry survived the clear:\n%s", raw)
	}
	got, err := loadErrorsFrom(p)
	if err != nil || len(got) != 0 {
		t.Errorf("a cleared journal should load empty, got %d entries (%v)", len(got), err)
	}
}

func TestClearingAMissingJournalIsNotAnError(t *testing.T) {
	p := filepath.Join(t.TempDir(), "nope.yaml")
	if err := ClearJournalTo(p, ErrorsHeader()); err != nil {
		t.Errorf("clearing what is not there should be fine, got %v", err)
	}
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Error("clearing a missing journal must not create it")
	}
}

func TestLoadingAMissingJournalIsEmpty(t *testing.T) {
	dir := t.TempDir()
	if got, err := loadErrorsFrom(filepath.Join(dir, "a.yaml")); err != nil || got != nil {
		t.Errorf("errors: want nil,nil got %v,%v", got, err)
	}
	if got, err := loadHistoryFrom(filepath.Join(dir, "b.yaml")); err != nil || got != nil {
		t.Errorf("history: want nil,nil got %v,%v", got, err)
	}
	if got, err := loadActivityFrom(filepath.Join(dir, "c.yaml")); err != nil || got != nil {
		t.Errorf("activity: want nil,nil got %v,%v", got, err)
	}
}

// ------------------------------------------------------------------- helpers

func mustRead(t *testing.T, p string) string {
	t.Helper()
	raw, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func itoaTest(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
