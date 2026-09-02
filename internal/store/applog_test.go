package store

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAppendLogRoundTripAndPerms(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "applogs.yaml")
	in := []LogEntry{
		{At: time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC), Level: "info", Msg: "connected"},
		{At: time.Date(2026, 9, 1, 10, 0, 5, 0, time.UTC), Level: "error",
			Msg: "demo · refused\nssh: connect to host x port 22: Connection refused"},
	}
	for _, e := range in {
		if err := AppendLogTo(path, e); err != nil {
			t.Fatalf("append: %v", err)
		}
	}

	st, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := st.Mode().Perm(); perm != 0o600 {
		t.Fatalf("the log keeps other machines' output, want 0600, got %o", perm)
	}

	out, err := LoadLogFrom(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("want 2 entries, got %d", len(out))
	}
	// Oldest first, multi-line survives, timestamps survive.
	if !out[0].At.Equal(in[0].At) || out[0].Level != "info" || out[0].Msg != in[0].Msg {
		t.Errorf("entry 0 mismatch: %+v", out[0])
	}
	if !strings.Contains(out[1].Msg, "Connection refused") ||
		!strings.Contains(out[1].Msg, "\n") {
		t.Errorf("the multi-line message must survive the trip: %q", out[1].Msg)
	}
}

// The file trims itself: past the size threshold an append rewrites it down to
// the newest logKeep entries, so it cannot grow without bound.
func TestTheLogTrimsItsOwnTail(t *testing.T) {
	path := filepath.Join(t.TempDir(), "applogs.yaml")
	old := logTrimBytes
	logTrimBytes = 512
	defer func() { logTrimBytes = old }()

	msg := func(i int) string {
		return fmt.Sprintf("event-%s-%02d", strings.Repeat("x", 40), i)
	}
	for i := 0; i < 40; i++ {
		e := LogEntry{At: time.Now(), Level: "info", Msg: msg(i)}
		if err := AppendLogTo(path, e); err != nil {
			t.Fatalf("append %d: %v", i, err)
		}
	}
	st, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if st.Size() > 8*512 {
		t.Fatalf("the file should have been trimmed, is %d bytes", st.Size())
	}
	out, err := LoadLogFrom(path)
	if err != nil {
		t.Fatalf("a trimmed file must still parse: %v", err)
	}
	if len(out) == 0 {
		t.Fatal("trimming must keep the tail, not empty the file")
	}
	// The NEWEST entries survive — the last append is the last entry — and it is
	// the OLDEST that get dropped: a trim that keeps the head would still pass a
	// bare tail check, because later appends land after whatever survives.
	if out[len(out)-1].Msg != msg(39) {
		t.Errorf("the newest entry must survive a trim, tail is %q", out[len(out)-1].Msg)
	}
	for _, e := range out {
		if e.Msg == msg(0) {
			t.Error("the oldest entry must be what a trim throws away, and it is still here")
		}
	}
}

// Clearing empties the file without losing what the file IS: the warning
// header stays, the permission stays, and the next append still lands.
func TestClearLogEmptiesTheFileButKeepsItsHeader(t *testing.T) {
	path := filepath.Join(t.TempDir(), "applogs.yaml")
	if err := AppendLogTo(path, LogEntry{At: time.Now(), Level: "info", Msg: "connected"}); err != nil {
		t.Fatalf("append: %v", err)
	}
	if err := ClearLogTo(path); err != nil {
		t.Fatalf("clear: %v", err)
	}

	out, err := LoadLogFrom(path)
	if err != nil || len(out) != 0 {
		t.Fatalf("a cleared log should read back empty, got %v, %v", out, err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "WARNING:") {
		t.Errorf("the warning is about the file, not about its entries:\n%s", raw)
	}
	st, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := st.Mode().Perm(); perm != 0o600 {
		t.Errorf("a cleared log keeps its 0600, got %o", perm)
	}

	// Still the file the next event appends to.
	if err := AppendLogTo(path, LogEntry{At: time.Now(), Level: "info", Msg: "after"}); err != nil {
		t.Fatalf("append after clear: %v", err)
	}
	if out, _ := LoadLogFrom(path); len(out) != 1 || out[0].Msg != "after" {
		t.Errorf("want just the new entry, got %+v", out)
	}
}

func TestClearLogMissingFileIsNotAnError(t *testing.T) {
	if err := ClearLogTo(filepath.Join(t.TempDir(), "nope.yaml")); err != nil {
		t.Fatalf("nothing recorded, nothing to erase: %v", err)
	}
}

func TestLoadLogMissingFileIsEmpty(t *testing.T) {
	out, err := LoadLogFrom(filepath.Join(t.TempDir(), "nope.yaml"))
	if err != nil || out != nil {
		t.Fatalf("missing file: want nil, nil — got %v, %v", out, err)
	}
}
