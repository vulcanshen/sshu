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

func logWith(msg string) appLog {
	m := newAppLog()
	m.errorf(msg)
	return m
}

// No line the log draws may be wider than the panel it is drawn in. It used to
// be: a floor of 8 on the message column meant any panel under 24 columns wrote
// lines wider than itself, and the outer fitLines then cut the words off.
func TestLogRowsNeverExceedThePanel(t *testing.T) {
	m := logWith(remoteNoise)
	for _, innerW := range []int{12, 16, 20, 22, 24, 30, 46, 60, 100} {
		for i, row := range m.allRows(innerW) {
			if got := dispW(row); got > innerW {
				t.Errorf("innerW=%d row %d is %d wide: %q", innerW, i, got, ansi.Strip(row))
			}
		}
	}
}

// Every line except the last is FULL. Somebody else's error message is wrapped
// rather than truncated so the whole of it can be read; breaking early to land
// on a separator throws a third of each line away and reads as damage.
func TestLogFillsEveryLineItWraps(t *testing.T) {
	const innerW = 60
	m := logWith(remoteNoise)
	rows := m.allRows(innerW)
	if len(rows) < 3 {
		t.Fatalf("expected the entry to wrap over several lines, got %d", len(rows))
	}

	// The message column is what is left after the timestamp gutter.
	const gutter = 15
	for i, row := range rows[:len(rows)-1] {
		msg := strings.TrimRight(ansi.Strip(row)[gutter:], " ")
		if w := dispW(msg); w < innerW-gutter-2 {
			t.Errorf("row %d used %d of %d columns — it broke early:\n%q",
				i, w, innerW-gutter-1, ansi.Strip(row))
		}
	}
}

// An IP address is not a path with parts worth keeping together. Breaking after
// its dots is what made "10.20.12.31" come out as "10.20.12." and "31".
func TestLogDoesNotBreakAnIPAtItsDots(t *testing.T) {
	m := logWith(strings.Repeat("x", 40) + "10.20.12.31" + strings.Repeat("y", 40))
	for _, row := range m.allRows(46) {
		if strings.HasSuffix(strings.TrimRight(ansi.Strip(row), " "), "10.20.12.") {
			t.Errorf("the log broke an IP after a dot: %q", ansi.Strip(row))
		}
	}
}

// ...but a hostname still does break at its separators. That rule was written
// for the sessions list and it stays there — the two callers want opposite
// things, which is why there are two functions.
func TestAHostnameStillBreaksAtItsSeparators(t *testing.T) {
	got := wrapText("db-replica-tokyo-ap-northeast-1", 20)
	if len(got) < 2 {
		t.Fatalf("expected it to wrap, got %q", got)
	}
	if !strings.HasSuffix(got[0], "-") {
		t.Errorf("the first line should end on a separator, got %q", got[0])
	}
}

// And the plain wrap fills the same string instead.
func TestPlainWrapFillsWhereTextWrapWouldBreakEarly(t *testing.T) {
	// 25, not 20: at 20 the name happens to break exactly on a separator and
	// the two agree, which would test nothing.
	const w = 25
	name := "db-replica-tokyo-ap-northeast-1"
	sep, plain := wrapText(name, w), wrapPlain(name, w)
	if dispW(plain[0]) != w {
		t.Errorf("wrapPlain should fill the line: %d of %d (%q)", dispW(plain[0]), w, plain[0])
	}
	if dispW(sep[0]) >= dispW(plain[0]) {
		t.Errorf("this is the case where they differ: sep=%q plain=%q", sep[0], plain[0])
	}
	// Both must still say the whole thing.
	if strings.Join(plain, "") != name || strings.Join(sep, "") != name {
		t.Error("wrapping must not lose or add characters")
	}
}

// Not exceeding the panel is half the promise; the other half is that most of
// the panel is MESSAGE. The 15-column gutter is what threatens that: hold it on
// a 20-column panel and every line is four characters of text after fifteen
// blanks — the line still fits the box, and the log still reads as damage.
//
// This is the shape of the bug that started this: text delivered a few
// characters at a time down a narrow channel with empty space either side.
func TestTheGutterYieldsBeforeTheMessageDoes(t *testing.T) {
	m := logWith(remoteNoise)
	for _, innerW := range []int{20, 24, 30, 46, 60, 100} {
		rows := m.allRows(innerW)
		if len(rows) < 2 {
			t.Fatalf("innerW=%d: expected the entry to wrap, got %d rows", innerW, len(rows))
		}
		// Continuation lines only: the first carries the timestamp, and the
		// last is short because it is the end of the message.
		for i, row := range rows[1:max(1, len(rows)-1)] {
			stripped := ansi.Strip(row)
			text := strings.TrimLeft(stripped, " ")
			gutter := dispW(stripped) - dispW(text)
			if gutter > dispW(text) {
				t.Errorf("innerW=%d row %d: %d columns of gutter against %d of message:\n%q",
					innerW, i+1, gutter, dispW(text), stripped)
			}
		}
	}
}

// The gutter earns its place on a panel that can spare it: continuation lines
// indent under the message so a multi-line entry reads as one block instead of
// as a paragraph nobody can find the start of.
func TestAWidePanelKeepsTheGutter(t *testing.T) {
	m := logWith(remoteNoise)
	rows := m.allRows(80)
	if len(rows) < 2 {
		t.Fatalf("expected the entry to wrap, got %d rows", len(rows))
	}
	stripped := ansi.Strip(rows[1])
	if lead := dispW(stripped) - dispW(strings.TrimLeft(stripped, " ")); lead < 10 {
		t.Errorf("a wide panel should indent its continuation lines, got %d columns: %q", lead, stripped)
	}
}

// And it still says something at widths no real panel reaches.
func TestANarrowLogPanelStillShowsItsWords(t *testing.T) {
	m := logWith(remoteNoise)
	for _, innerW := range []int{12, 16, 20, 24} {
		rows := m.allRows(innerW)
		if strings.TrimSpace(ansi.Strip(strings.Join(rows, ""))) == "" {
			t.Errorf("innerW=%d: the log drew nothing but blanks", innerW)
		}
	}
}

// A one-column box and a two-column glyph: nothing fits. Taking zero runes is
// the obvious answer and it loops forever — the line has to move on and
// overflow by a column, which the caller clips.
func TestWrapAdvancesOnAGlyphWiderThanTheLine(t *testing.T) {
	got := wrapPlain("中文", 1)
	if strings.Join(got, "") != "中文" {
		t.Fatalf("nothing may be lost or added: %q", got)
	}
	if len(got) != 2 {
		t.Errorf("expected one line per glyph that cannot share one, got %q", got)
	}
}

// The log is a viewport over RENDERED rows, so a panel narrow enough to change
// how many rows there are must not strand the scroll position past the end.
func TestLogScrollStaysInRangeAcrossWidths(t *testing.T) {
	m := logWith(remoteNoise)
	m.scrollKey("G", 30, 4)
	wide := m.body(100, 4)
	if len(wide) != 4 {
		t.Fatalf("body must fill the box: %d rows", len(wide))
	}
	for _, row := range wide {
		if dispW(row) != 100 {
			t.Errorf("body row is %d wide, want 100: %q", dispW(row), ansi.Strip(row))
		}
	}
}
