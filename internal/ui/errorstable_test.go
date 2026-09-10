package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// A remote's final screen: an IP, a path, CJK, and a prompt glyph. Every one of
// those is dense with the characters wrapText prefers to break after.
const remoteNoise = "10.20.12.31 · ❯   other-service/ (放 other service 檔案)" +
	"Last login: Thu Sep  3 12:00:27 2026 from 10.20.15.1 ubuntu in ⊕ pc12031 in ~ ❯"

func logWith(cause string, more ...string) errorsModel {
	var m errorsModel
	m.errorf("prod-web-01", "deploy", cause, more...)
	return m
}

// ------------------------------------------------------------------ the table

// The frame invariant, at every width the panel can be: no row wider than the
// panel it is drawn in, header and selected row included.
func TestErrorRowsNeverExceedThePanel(t *testing.T) {
	m := logWith("Host key verification failed", strings.Split(remoteNoise, "·")...)
	for _, innerW := range []int{12, 16, 20, 22, 24, 30, 46, 60, 100} {
		for i, row := range m.body(innerW, 8) {
			if got := dispW(row); got != innerW {
				t.Errorf("innerW=%d row %d is %d wide: %q", innerW, i, got, ansi.Strip(row))
			}
		}
	}
}

// Every journal names its columns. That is what makes a panel scannable rather
// than readable: "which column is this" answered once at the top instead of
// guessed at on every row.
func TestEveryJournalNamesItsColumns(t *testing.T) {
	e := logWith("Connection refused")
	if head := ansi.Strip(e.body(90, 8)[0]); !strings.Contains(head, "Time") ||
		!strings.Contains(head, "Host") || !strings.Contains(head, "User") ||
		!strings.Contains(head, "Cause") {
		t.Errorf("errors header: %q", head)
	}

	var h historyModel
	h.add("prod-web-01", "deploy", true)
	if head := ansi.Strip(h.body(90, 8)[0]); !strings.Contains(head, "Time") ||
		!strings.Contains(head, "Host") || !strings.Contains(head, "Result") {
		t.Errorf("history header: %q", head)
	}

	var a activityModel
	a.add("host \"x\" added")
	if head := ansi.Strip(a.body(90, 8)[0]); !strings.Contains(head, "Time") ||
		!strings.Contains(head, "Action") {
		t.Errorf("activity header: %q", head)
	}
}

// One entry is ONE row. The whole of what the far end said is behind Enter, so
// the panel can be scanned — which a wall of somebody else's banners could not.
func TestAnErrorIsOneRowWhateverItsLength(t *testing.T) {
	m := logWith("Host key verification failed",
		"@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@",
		"@  WARNING: REMOTE HOST IDENTIFICATION HAS CHANGED!  @",
		"SHA256:uNiVeRsAlLyUnIqUeFiNgErPrInT",
		"Offending ECDSA key in /home/user/.ssh/known_hosts:42")

	body := m.body(90, 8)
	// Header plus exactly one row, and the rest of the panel blank.
	filled := 0
	for _, r := range body {
		if strings.TrimSpace(ansi.Strip(r)) != "" {
			filled++
		}
	}
	if filled != 2 {
		t.Errorf("want a header and one row, got %d filled lines:\n%s",
			filled, ansi.Strip(strings.Join(body, "\n")))
	}
	row := ansi.Strip(body[1])
	if !strings.Contains(row, "Host key verification failed") {
		t.Errorf("the row should carry the cause: %q", row)
	}
	if strings.Contains(row, "SHA256") {
		t.Errorf("the detail belongs behind Enter, not in the row: %q", row)
	}
}

// A cause too long for its column is cut with the app's ellipsis rather than
// pushing the panel border out of line.
func TestALongCauseIsCutWithAnEllipsis(t *testing.T) {
	m := logWith("ssh: connect to host prod-web-01.internal.corp port 22: " +
		"Connection timed out after waiting for the configured interval")
	row := ansi.Strip(m.body(70, 4)[1])
	if !strings.Contains(row, "…") {
		t.Errorf("an over-long cause must show it was cut: %q", row)
	}
	if dispW(row) != 70 {
		t.Errorf("the row must still come to the panel width, got %d: %q", dispW(row), row)
	}
}

// The row says which machine and as whom — the whole point of the split. A
// panel of failures used to be prose you had to read to find that out.
func TestTheRowNamesTheMachine(t *testing.T) {
	m := logWith("Connection refused")
	row := ansi.Strip(m.body(90, 4)[1])
	if !strings.Contains(row, "prod-web-01") || !strings.Contains(row, "deploy") {
		t.Errorf("the row should carry host and user: %q", row)
	}
}

// An entry with no machine behind it — a config file that would not parse —
// says so rather than leaving the columns blank.
func TestAnEntryWithNoHostShowsThePlaceholder(t *testing.T) {
	var m errorsModel
	m.warn("", "", "config.yaml: line 3 is not a setting")
	if row := ansi.Strip(m.body(90, 4)[1]); !strings.Contains(row, jNone) {
		t.Errorf("want the placeholder in the empty columns: %q", row)
	}
}

// ----------------------------------------------------------------- the cursor

// Errors is the one journal with a cursor, because it is the one with somewhere
// to go. It starts on the newest, which is what anybody opening the panel is
// looking for.
func TestTheCursorStartsOnTheNewest(t *testing.T) {
	var m errorsModel
	m.errorf("a", "u", "first")
	m.errorf("b", "u", "second")

	e, ok := m.current()
	if !ok || e.cause != "second" {
		t.Errorf("want the newest under the cursor, got %+v (ok=%v)", e, ok)
	}
	m.handleKey("j", 8)
	if e, _ := m.current(); e.cause != "first" {
		t.Errorf("j should walk back in time, got %q", e.cause)
	}
}

// A new entry arrives at the TOP of a newest-first table, so a cursor resting
// below it would end up on a different row without having moved. It follows
// the shift instead — unless it is already on the newest, where staying is
// what it wants.
func TestTheCursorKeepsItsEntryWhenOneArrives(t *testing.T) {
	var m errorsModel
	m.errorf("a", "u", "first")
	m.errorf("b", "u", "second")
	m.handleKey("j", 8) // onto "first"

	m.errorf("c", "u", "third")
	if e, _ := m.current(); e.cause != "first" {
		t.Errorf("the cursor should still be on the entry it was on, got %q", e.cause)
	}

	// ...and one sitting on the newest stays on the newest.
	var top errorsModel
	top.errorf("a", "u", "first")
	top.errorf("b", "u", "second")
	if e, _ := top.current(); e.cause != "second" {
		t.Fatalf("a cursor at the top should stay there, got %q", e.cause)
	}
}

// The selected row wears the bar and drops its column colours, exactly as the
// hosts table does: the bar answers "you are here" and a row cannot carry both
// signals without the bar winning anyway.
func TestTheSelectedErrorRowWearsTheBar(t *testing.T) {
	withColour(t)
	m := logWith("Connection refused")
	body := m.body(90, 4)
	if !strings.Contains(body[1], ansiBgOf(t, rowSelColor)) {
		t.Errorf("the row under the cursor should wear the bar: %q", body[1])
	}
	if strings.Contains(body[1], ansiOf(t, warnColor)) {
		t.Errorf("a selected row must not keep the level colour: %q", body[1])
	}
}

// ------------------------------------------------------------------ the popup

// Enter opens the whole of it. The row kept the cause; the fingerprint in the
// middle of a host key banner is the reason this panel exists at all.
func TestEnterOpensTheWholeError(t *testing.T) {
	m := appWith(sample(), nil)
	m.errors.errorf("prod-web-01", "deploy", "Host key verification failed",
		"SHA256:uNiVeRsAlLyUnIqUeFiNgErPrInT",
		"Offending ECDSA key in /home/user/.ssh/known_hosts:42")

	m = pressA(m, "1", "j", "j", "j", "j", "enter") // nav → errors → content
	if m.pref.item != prefErrors {
		t.Fatalf("setup: expected the errors content, got item %d", m.pref.item)
	}
	m = pressA(m, "enter")
	if !m.viewer.isActive() {
		t.Fatal("Enter on an error row should open it")
	}
	whole := ansi.Strip(strings.Join(m.viewer.lines, "\n"))
	for _, want := range []string{
		"Host key verification failed",
		"SHA256:uNiVeRsAlLyUnIqUeFiNgErPrInT",
		"known_hosts:42",
	} {
		if !strings.Contains(whole, want) {
			t.Errorf("%q is missing from the popup:\n%s", want, whole)
		}
	}
	// The title says which machine, because that is what you were looking for
	// when you put the cursor here.
	if !strings.Contains(m.viewer.title, "prod-web-01") {
		t.Errorf("the popup should be titled by the machine, got %q", m.viewer.title)
	}
}

// The popup WRAPS rather than clipping. The words that say why are at the end
// of somebody else's error message, so cutting the tail throws away the only
// part anybody opened it for.
func TestThePopupWrapsRatherThanClipping(t *testing.T) {
	long := strings.Repeat("verbose-remote-output ", 20)
	m := appWith(sample(), nil)
	m.errors.errorf("prod-web-01", "deploy", "it failed", long)

	m = pressA(m, "1", "j", "j", "j", "j", "enter", "enter")
	if !m.viewer.isActive() {
		t.Fatal("setup: the popup should be open")
	}
	if len(m.viewer.lines) < 3 {
		t.Fatalf("the long line should have wrapped, got %d lines", len(m.viewer.lines))
	}
	joined := strings.Join(m.viewer.lines, "")
	tail := strings.TrimSpace(long)
	if !strings.Contains(joined, tail[len(tail)-20:]) {
		t.Errorf("the end of the message must survive:\n%q", joined)
	}
}

// ------------------------------------------------------------------- wrapping

// These test the wrap functions themselves, which the popup uses. They outlived
// the panel that used to wrap in place.

// There used to be a test here asserting that wrapPlain does not break an IP
// address at its dots. It passed by accident: wrapPlain fills to the width
// wherever that lands, so whether a dotted quad survives depends entirely on
// where the fixture's text happens to sit — and it moved the moment the column
// widths changed. The property it was reaching for is the one below (plain
// wrap is used precisely BECAUSE wrapText prefers separators, and a dot is a
// separator), which is stated directly rather than by coincidence.

func TestAHostnameStillBreaksAtItsSeparators(t *testing.T) {
	got := wrapText("db-replica-tokyo-ap-northeast-1", 20)
	if len(got) < 2 {
		t.Fatalf("expected it to wrap, got %q", got)
	}
	if !strings.HasSuffix(got[0], "-") {
		t.Errorf("a hostname should break after a separator, got %q", got[0])
	}
}

func TestPlainWrapFillsWhereTextWrapWouldBreakEarly(t *testing.T) {
	const w = 30
	plain := wrapPlain(remoteNoise, w)
	for i, line := range plain[:len(plain)-1] {
		if got := dispW(line); got < w-1 {
			t.Errorf("plain line %d used %d of %d columns: %q", i, got, w, line)
		}
	}
}

func TestWrapAdvancesOnAGlyphWiderThanTheLine(t *testing.T) {
	got := wrapPlain("中文", 1)
	if strings.Join(got, "") != "中文" {
		t.Errorf("a glyph wider than the line must still come out, got %q", got)
	}
}
