package ui

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
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
// It is a viewport (§6.1): newest first, no cursor, nothing in it can be acted
// on. `!` opens it and `!` closes it, the same way `?` does for help.

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
	anim    popupAnimator
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

	layer   int
	screenW int
	screenH int
}

func newAppLog() appLog { return appLog{anim: newPopupAnimator("applog")} }

func (m appLog) isActive() bool    { return m.anim.isActive() }
func (m *appLog) close() tea.Cmd   { return m.anim.close() }
func (m *appLog) setSize(w, h int) { m.screenW, m.screenH = w, h }
func (m appLog) unreadErrors() int { return m.unread }

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

func (m *appLog) info(msg string, more ...string)   { m.add(logInfo, msg, more...) }
func (m *appLog) warn(msg string, more ...string)   { m.add(logWarn, msg, more...) }
func (m *appLog) errorf(msg string, more ...string) { m.add(logError, msg, more...) }

// toggle is the whole of `!`: it opens what is closed and closes what is open,
// the same contract `?` has (§A.2). Opening marks the errors as read.
func (m *appLog) toggle(layer int) tea.Cmd {
	if m.anim.owns() {
		return m.anim.close()
	}
	m.layer, m.top, m.unread = layer, 0, 0
	return m.anim.open()
}

func (m appLog) rows() int { return max(1, m.screenH-8) }

func (m *appLog) update(msg tea.KeyMsg) {
	if !m.anim.isInteractive() {
		return
	}
	// Bounded by RENDERED rows, not by entries. One entry can be forty lines of
	// somebody else's failure, and scrolling that indexed entries would let you
	// see the top of it and never the rest.
	n := len(m.allRows(popupInnerW(m.screenW, logW)))
	m.top = moveScroll(m.top, max(0, n-m.rows()), msg.String(), m.rows())
}

// logW is the popup's width. Wide, because the lines it holds are somebody
// else's error messages and those are not written to fit.
const logW = 88

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

func (m appLog) view() string {
	innerW := popupInnerW(m.screenW, logW)

	rows := m.allRows(innerW)
	total := len(rows)
	if total == 0 {
		rows = emptyBody(innerW, min(m.rows(), 5), "Nothing has happened yet",
			emptyHint("Failed connections and transfers are recorded here", ""))
	} else {
		top := clamp(m.top, 0, max(0, total-1))
		rows = rows[top:min(total, top+m.rows())]
	}

	hint := [][2]string{{"j/k", "scroll"}, {"Esc", "close"}}
	if total > m.rows() {
		hint = append([][2]string{{itoa(m.top + 1), "of " + itoa(total)}}, hint...)
	}
	return drawPopupBox(popupLayerColor(m.layer), " "+glyphInfo+" app log ",
		hintLegend(hint), animRows(m.anim, capRows(rows, m.screenH)), innerW)
}
