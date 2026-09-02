package ui

import (
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/vulcanshen/sshu/internal/store"
)

// The app log is where sshu says what happened while you were not looking.
//
// It replaces the session history popup, and the thing that popup was for was
// never the list of sessions — it was the REASONS. Those are not tab [3]'s
// alone: a transfer that failed, an edit that could not be written back, a host
// key that was refused are all the same shape of news, and until now the only
// place any of them could go was a toast that is gone in two seconds. A record
// you can go back to is what a toast cannot be.
//
// It is a viewport (§6.1): newest first, no cursor, nothing in it can be
// acted on. It lives as preference → logs — a content panel, not a popup —
// and landing it on screen is what marks its errors read.

// logCap bounds the log. Five hundred lines is far more than a session produces
// and still nothing next to a terminal's scrollback.
const logCap = 500

type logLevel int

const (
	logInfo logLevel = iota
	logWarn
	logError
)

func (l logLevel) label() string {
	switch l {
	case logWarn:
		return "WARN"
	case logError:
		return "ERR "
	}
	return "INFO"
}

// name is the level as applogs.yaml spells it.
func (l logLevel) name() string {
	switch l {
	case logWarn:
		return "warn"
	case logError:
		return "error"
	}
	return "info"
}

func levelNamed(name string) logLevel {
	switch name {
	case "warn":
		return logWarn
	case "error":
		return logError
	}
	return logInfo
}

func (l logLevel) colour() lipgloss.Color {
	switch l {
	case logWarn:
		return warnColor
	case logError:
		return warnColor
	}
	return dimColor
}

type logEntry struct {
	at    time.Time
	level logLevel
	msg   string
}

type appLog struct {
	entries []logEntry // oldest first; the view reads it backwards
	// unread counts errors since the log was last opened. It is what puts the
	// key in the footer in bold words rather than quiet ones — news nobody is
	// told about is news that did not arrive.
	unread int
	top    int

	// sink writes each entry through to applogs.yaml. Nil in tests. sinkBroken
	// remembers the first failure so a log that cannot be written complains
	// exactly once instead of once per event.
	sink       func(store.LogEntry) error
	sinkBroken bool
	// clearSink empties that same file. Nil in tests, where clearing is only
	// ever the in-memory half.
	clearSink func() error
}

func newAppLog() appLog { return appLog{} }

func (m appLog) unreadErrors() int { return m.unread }

// markRead is the moment the logs content lands on screen — there is no popup
// to open any more, so being visible is what being read means.
func (m *appLog) markRead() { m.unread = 0 }

// entryLines and entryChars bound one entry. A whole ssh failure fits easily —
// even the host key banner is about fifteen lines — while a remote that decides
// to print a megabyte cannot take the log with it. kbu caps entries the same
// way, at a smaller number, because kbu's entries are single lines.
const (
	entryLines = 40
	entryChars = 4000
)

// add records one event, which may be several lines.
//
// Every line is SANITISED because this is where other machines' output ends up
// — ssh's own words before it gave up are the whole point of the thing — and
// those bytes are the remote's, not ours.
func (m *appLog) add(level logLevel, msg string, more ...string) {
	lines := make([]string, 0, 1+len(more))
	for _, l := range append([]string{msg}, more...) {
		for _, part := range strings.Split(l, "\n") {
			if len(lines) >= entryLines {
				break
			}
			if part = strings.TrimRight(sanitizeLine(part), " "); strings.TrimSpace(part) != "" {
				lines = append(lines, part)
			}
		}
	}
	if len(lines) == 0 {
		return
	}
	text := strings.Join(lines, "\n")
	if r := []rune(text); len(r) > entryChars {
		text = string(r[:entryChars]) + "…"
	}
	e := logEntry{at: time.Now(), level: level, msg: text}
	m.entries = append(m.entries, e)
	if len(m.entries) > logCap {
		m.entries = m.entries[len(m.entries)-logCap:]
	}
	if level == logError {
		m.unread++
	}
	// Written through AFTER the in-memory append: whatever happens to the disk,
	// the panel shows the event. The text is already sanitised and capped.
	if m.sink != nil && !m.sinkBroken {
		if err := m.sink(store.LogEntry{At: e.at, Level: level.name(), Msg: text}); err != nil {
			m.sinkBroken = true
			m.entries = append(m.entries, logEntry{at: time.Now(), level: logWarn,
				msg: "applogs.yaml: " + sanitizeLine(err.Error()) + " — new entries stay in memory only"})
		}
	}
}

// preload seeds the log with what applogs.yaml already held. Everything in it
// predates this run, so nothing is unread — and nothing is re-written through.
func (m *appLog) preload(tail []store.LogEntry) {
	if len(tail) > logCap {
		tail = tail[len(tail)-logCap:]
	}
	entries := make([]logEntry, 0, len(tail))
	for _, e := range tail {
		entries = append(entries, logEntry{at: e.At, level: levelNamed(e.Level), msg: e.Msg})
	}
	m.entries = append(entries, m.entries...)
}

// clear erases the log — the panel and the file both. The FILE goes first:
// a panel wiped while applogs.yaml survives fills straight back up on the next
// start, which is the one outcome "clear" must not have.
func (m *appLog) clear() error {
	if m.clearSink != nil {
		if err := m.clearSink(); err != nil {
			return err
		}
	}
	m.entries, m.top, m.unread = nil, 0, 0
	return nil
}

// logEntries counts entries in words. Three places say it — the status slot,
// the confirmation and the toast that follows it — and a count worded three
// ways reads as three different numbers.
func logEntries(n int) string {
	if n == 1 {
		return "1 entry"
	}
	return itoa(n) + " entries"
}

func (m *appLog) info(msg string, more ...string)   { m.add(logInfo, msg, more...) }
func (m *appLog) warn(msg string, more ...string)   { m.add(logWarn, msg, more...) }
func (m *appLog) errorf(msg string, more ...string) { m.add(logError, msg, more...) }

// scrollKey scrolls the panel. Bounded by RENDERED rows, not by entries: one
// entry can be forty lines of somebody else's failure, and scrolling that
// indexed entries would let you see the top of it and never the rest.
func (m *appLog) scrollKey(k string, innerW, innerH int) {
	n := len(m.allRows(innerW))
	m.top = moveScroll(m.top, max(0, n-innerH), k, innerH)
}

// allRows is every line the log would draw, newest entry first. The viewport
// scrolls over THESE rather than over entries, because one entry is not one row.
func (m appLog) allRows(innerW int) []string {
	dim := lipgloss.NewStyle().Foreground(dimColor)
	// The prefix is "15:04:05 ERR  ", and the continuation lines of an entry are
	// indented under its message rather than under its timestamp.
	const prefixW = 15
	msgW := max(8, innerW-prefixW-1)

	var rows []string
	for i := len(m.entries) - 1; i >= 0; i-- {
		e := m.entries[i]
		lvl := lipgloss.NewStyle().Foreground(e.level.colour())
		// An entry can be many lines — a whole ssh failure — and each of them is
		// WRAPPED rather than truncated: these are somebody else's error
		// messages and the part that says why is at the END of them ("…port 22:
		// Connection refused"), so cutting the tail throws away the only words
		// anybody opened the log to read.
		first := true
		for _, para := range strings.Split(e.msg, "\n") {
			for _, line := range wrapText(para, msgW) {
				if first {
					rows = append(rows, " "+dim.Render(e.at.Format("15:04:05"))+" "+
						lvl.Render(e.level.label())+" "+line)
					first = false
					continue
				}
				rows = append(rows, spaces(prefixW)+line)
			}
		}
	}
	return rows
}

// body renders the log into a content panel of innerW×innerH.
func (m appLog) body(innerW, innerH int) []string {
	rows := m.allRows(innerW)
	if len(rows) == 0 {
		return emptyBody(innerW, innerH, "Nothing has happened yet",
			emptyHint("Connections, transfers and edits are recorded here", ""))
	}
	top := clamp(m.top, 0, max(0, len(rows)-1))
	return fitLines(rows[top:min(len(rows), top+innerH)], innerW, innerH)
}
