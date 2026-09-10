package ui

import (
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/vulcanshen/sshu/internal/store"
)

// Three journals where there was one log.
//
// The old app log answered every question at once: what broke, what you
// connected to, what you changed. One free-text line per event, three levels,
// no columns. That is the right shape for none of the three.
//
// Errors keeps the thing the log was actually for — everything the far end
// said before it gave up — but puts the WHO and WHEN in columns above it, so a
// panel of failures can be scanned rather than read. History is one fixed row
// per connection attempt and nothing else, so a machine's record reads down a
// column. Activity is what you changed, which is a sentence and needs no
// columns at all.
//
// All three are viewports (§6.1): newest first, no cursor, nothing in them can
// be acted on, and scrolling is over RENDERED ROWS rather than over entries —
// one error is not one row.

// journalCap bounds each journal in memory, matching store.journalKeep so a
// panel never shows less than its file remembers.
const journalCap = 500

// entryLines and entryChars bound ONE entry. A whole ssh failure fits easily —
// even the host key banner is about fifteen lines — while a remote that decides
// to print a megabyte cannot take the journal with it.
const (
	entryLines = 40
	entryChars = 4000
)

// errLevel is how bad an entry in Errors is. Two values, not three: the old log
// carried info as well, and info is what Activity and History now hold in a
// shape that suits them.
type errLevel int

const (
	levelWarn errLevel = iota
	levelError
)

// name is the level as errors.yaml spells it.
func (l errLevel) name() string {
	if l == levelError {
		return store.LevelError
	}
	return store.LevelWarn
}

func levelNamed(name string) errLevel {
	if name == store.LevelError {
		return levelError
	}
	return levelWarn
}

// colour is where the two bands §11.48 separated finally both get used. Red is
// "something is wrong"; peach is "worth knowing, nothing is broken" — which is
// exactly the difference between a refused connection and a config file that
// had a line sshu could not honour. The old log painted both red, so the panel
// could not tell you which was which without reading every word.
func (l errLevel) colour() lipgloss.Color {
	if l == levelError {
		return warnColor
	}
	return peachColor
}

// Column geometry. The time column is fixed at what "15:04:05" measures, and
// result at what "success" does — neither can ever be wider, so neither should
// move with the data beside it.
const (
	jTimeW   = 8
	jResultW = 7
	jGap     = 2
	// Minimums below which a column stops carrying information. Host wins the
	// larger share for the same reason Name does in the hosts table: it is
	// what you are scanning for.
	jMinHostW = 8
	jMinUserW = 5
)

// jHostUserW shares the space left after the fixed columns between host and
// user. fixed is whatever else the row spends besides the leading space, the
// time column and the gap after it.
//
// Two columns come off the top, not one: a leading space AND a trailing one.
// Without the second, a value that exactly fills its column touches the panel
// border — "success" is seven cells and the result column is seven, so History
// was the row that showed it.
func jHostUserW(innerW, fixed int) (hostW, userW int) {
	free := innerW - 2 - jTimeW - jGap - fixed
	if free < jMinHostW+jGap+jMinUserW {
		// No room for two columns. The host takes what there is: a row that
		// cannot say which machine is a row that says nothing.
		return max(1, free), 0
	}
	free -= jGap
	hostW = max(jMinHostW, free*60/100)
	userW = max(jMinUserW, free-hostW)
	if over := hostW + userW - free; over > 0 {
		hostW = max(jMinHostW, hostW-over)
	}
	return hostW, userW
}

// jNone stands in for a host or user an entry does not have — a config file
// that would not parse has no machine behind it. Same placeholder the hosts
// table uses for a missing tag line, because it is the same statement: this
// field is part of the shape and is empty, rather than absent.
const jNone = "—"

func jField(s string) string {
	if strings.TrimSpace(s) == "" {
		return jNone
	}
	return s
}

func jTime(t time.Time) string { return t.Format("15:04:05") }

// ------------------------------------------------------------------- errors

type errorRec struct {
	at    time.Time
	host  string
	user  string
	level errLevel // levelWarn | levelError
	text  string   // may span lines
}

// errorsModel is manage → Logs → Errors.
type errorsModel struct {
	entries []errorRec // oldest first; the view reads it backwards
	// unread counts ERRORS since the panel was last on screen. Warnings do not
	// count: the badge means "something failed", and diluting it with "something
	// was mentioned" is how a badge stops being looked at.
	unread int
	top    int

	// sink writes each entry through to errors.yaml. Nil in tests. sinkBroken
	// remembers the first failure so a journal that cannot be written complains
	// exactly once instead of once per event.
	sink       func(store.ErrorEntry) error
	sinkBroken bool
	clearSink  func() error
}

func (m errorsModel) unreadErrors() int { return m.unread }
func (m *errorsModel) markRead()        { m.unread = 0 }

// add records one failure. The text is sanitised and capped here, exactly as
// the app log did it, because this is still where other machines' output lands.
func (m *errorsModel) add(level errLevel, host, user, msg string, more ...string) {
	text := joinSanitised(msg, more...)
	if text == "" {
		return
	}
	e := errorRec{at: time.Now(), host: host, user: user, level: level, text: text}
	m.entries = append(m.entries, e)
	if len(m.entries) > journalCap {
		m.entries = m.entries[len(m.entries)-journalCap:]
	}
	if level == levelError {
		m.unread++
	}
	// Written through AFTER the in-memory append: whatever happens to the disk,
	// the panel shows the event.
	if m.sink != nil && !m.sinkBroken {
		if err := m.sink(store.ErrorEntry{At: e.at, Host: host, User: user,
			Level: level.name(), Error: text}); err != nil {
			m.sinkBroken = true
			m.entries = append(m.entries, errorRec{at: time.Now(), level: levelWarn,
				text: "errors.yaml: " + sanitizeLine(err.Error()) + " — new entries stay in memory only"})
		}
	}
}

func (m *errorsModel) errorf(host, user, msg string, more ...string) {
	m.add(levelError, host, user, msg, more...)
}

func (m *errorsModel) warn(host, user, msg string, more ...string) {
	m.add(levelWarn, host, user, msg, more...)
}

func (m *errorsModel) preload(tail []store.ErrorEntry) {
	if len(tail) > journalCap {
		tail = tail[len(tail)-journalCap:]
	}
	out := make([]errorRec, 0, len(tail))
	for _, e := range tail {
		out = append(out, errorRec{at: e.At, host: e.Host, user: e.User,
			level: levelNamed(e.Level), text: e.Error})
	}
	m.entries = append(out, m.entries...)
}

func (m *errorsModel) clear() error {
	if m.clearSink != nil {
		if err := m.clearSink(); err != nil {
			return err
		}
	}
	m.entries, m.top, m.unread = nil, 0, 0
	return nil
}

func (m *errorsModel) scrollKey(k string, innerW, innerH int) {
	n := len(m.allRows(innerW))
	m.top = moveScroll(m.top, max(0, n-innerH), k, innerH)
}

// allRows draws every entry newest first: one column row naming when, where
// and as whom, then the failure itself wrapped underneath.
//
// The columns are the change from the old log. A panel of failures used to be
// a wall of prose you had to read to find which machine each one was about;
// the machine is the first thing you want and it is now in a fixed place.
//
// The text is WRAPPED rather than truncated, which the columns above it are
// not: these are somebody else's error messages, and the part that says why is
// at the END of them ("…port 22: Connection refused"), so cutting the tail
// throws away the only words anybody opened this panel to read.
func (m errorsModel) allRows(innerW int) []string {
	dim := lipgloss.NewStyle().Foreground(dimColor)
	txt := lipgloss.NewStyle().Foreground(textColor)
	hostW, userW := jHostUserW(innerW, 0)

	// The message is indented under the columns rather than under the
	// timestamp, so an entry reads as one block. Below the width where that
	// leaves less room for words than for blank, it goes back to the margin.
	msgW, indent := innerW-1-jGap, jGap
	if msgW < 2*jGap {
		msgW, indent = max(2, innerW-1), 0
	}

	var rows []string
	for i := len(m.entries) - 1; i >= 0; i-- {
		e := m.entries[i]
		// The timestamp carries the level, because it is the one cell every
		// entry has. Red is "something is wrong", peach is "worth knowing" —
		// the two bands §11.48 separated, used here for exactly that split.
		stamp := lipgloss.NewStyle().Foreground(e.level.colour())

		head := " " + stamp.Render(jTime(e.at)) + spaces(jGap) +
			txt.Render(padRight(jField(e.host), hostW))
		if userW > 0 {
			head += spaces(jGap) + dim.Render(padRight(jField(e.user), userW))
		}
		rows = append(rows, clipANSI(head, innerW))

		for _, para := range strings.Split(e.text, "\n") {
			// wrapPlain, not wrapText: this is somebody else's output, and
			// preferring a separator in it wastes a third of every line.
			for _, line := range wrapPlain(para, msgW) {
				rows = append(rows, spaces(indent)+dim.Render(line))
			}
		}
	}
	return rows
}

func (m errorsModel) body(innerW, innerH int) []string {
	rows := m.allRows(innerW)
	if len(rows) == 0 {
		return emptyBody(innerW, innerH, "Nothing has failed",
			emptyHint("Refused connections, failed transfers and edits that could not be written back land here", ""))
	}
	top := clamp(m.top, 0, max(0, len(rows)-1))
	return fitLines(rows[top:min(len(rows), top+innerH)], innerW, innerH)
}

func (m errorsModel) status() string {
	if len(m.entries) == 0 {
		return "no errors"
	}
	return journalEntries(len(m.entries))
}

// ------------------------------------------------------------------ history

type histRec struct {
	at   time.Time
	host string
	user string
	ok   bool
}

// historyModel is manage → Logs → History: every connection attempt and how it
// ended. The reason is deliberately NOT here — it is in Errors, and recording
// it twice would mean two accounts of one failure that can disagree.
type historyModel struct {
	entries []histRec
	top     int

	sink       func(store.HistoryEntry) error
	sinkBroken bool
	clearSink  func() error
}

func (m *historyModel) add(host, user string, ok bool) {
	e := histRec{at: time.Now(), host: host, user: user, ok: ok}
	m.entries = append(m.entries, e)
	if len(m.entries) > journalCap {
		m.entries = m.entries[len(m.entries)-journalCap:]
	}
	if m.sink != nil && !m.sinkBroken {
		result := store.ResultFail
		if ok {
			result = store.ResultSuccess
		}
		if err := m.sink(store.HistoryEntry{At: e.at, Host: host, User: user,
			Result: result}); err != nil {
			m.sinkBroken = true
		}
	}
}

func (m *historyModel) preload(tail []store.HistoryEntry) {
	if len(tail) > journalCap {
		tail = tail[len(tail)-journalCap:]
	}
	out := make([]histRec, 0, len(tail))
	for _, e := range tail {
		out = append(out, histRec{at: e.At, host: e.Host, user: e.User,
			ok: e.Result == store.ResultSuccess})
	}
	m.entries = append(out, m.entries...)
}

func (m *historyModel) clear() error {
	if m.clearSink != nil {
		if err := m.clearSink(); err != nil {
			return err
		}
	}
	m.entries, m.top = nil, 0
	return nil
}

func (m *historyModel) scrollKey(k string, innerW, innerH int) {
	m.top = moveScroll(m.top, max(0, len(m.entries)-innerH), k, innerH)
}

// rows is one line per attempt, newest first. Fixed height on purpose: this
// panel exists to be counted down a column — "that host, three times this
// afternoon, two of them red" — and a row that can grow breaks the count.
func (m historyModel) rows(innerW int) []string {
	dim := lipgloss.NewStyle().Foreground(dimColor)
	txt := lipgloss.NewStyle().Foreground(textColor)
	ok := lipgloss.NewStyle().Foreground(liveColor)
	bad := lipgloss.NewStyle().Foreground(warnColor)

	hostW, userW := jHostUserW(innerW, jResultW+jGap)

	out := make([]string, 0, len(m.entries))
	for i := len(m.entries) - 1; i >= 0; i-- {
		e := m.entries[i]
		style, word := ok, store.ResultSuccess
		if !e.ok {
			style, word = bad, store.ResultFail
		}
		row := " " + dim.Render(jTime(e.at)) + spaces(jGap) +
			txt.Render(padRight(jField(e.host), hostW))
		if userW > 0 {
			row += spaces(jGap) + dim.Render(padRight(jField(e.user), userW))
		}
		row += spaces(jGap) + style.Render(padRight(word, jResultW))
		out = append(out, clipANSI(row, innerW))
	}
	return out
}

func (m historyModel) body(innerW, innerH int) []string {
	if len(m.entries) == 0 {
		return emptyBody(innerW, innerH, "No connections yet",
			emptyHint("Every ssh and sftp connection is recorded here, with its result", ""))
	}
	rows := m.rows(innerW)
	top := clamp(m.top, 0, max(0, len(rows)-1))
	return fitLines(rows[top:min(len(rows), top+innerH)], innerW, innerH)
}

func (m historyModel) status() string {
	if len(m.entries) == 0 {
		return "no connections"
	}
	failed := 0
	for _, e := range m.entries {
		if !e.ok {
			failed++
		}
	}
	if failed == 0 {
		return plural(len(m.entries), "connection")
	}
	return plural(len(m.entries), "connection") + " · " + itoa(failed) + " failed"
}

// ----------------------------------------------------------------- activity

type actRec struct {
	at     time.Time
	action string
}

// activityModel is manage → Logs → Activity: what you changed through sshu.
// Hosts and credentials added or deleted, ~/.ssh files edited, bundles moved,
// files transferred, edits written back.
//
// NOT connection state, and not UI state. Locking a pty or zooming a cell
// changes what you are looking at rather than what is there, and a record of
// those would bury the changes that outlive the session.
type activityModel struct {
	entries []actRec
	top     int

	sink       func(store.ActivityEntry) error
	sinkBroken bool
	clearSink  func() error
}

func (m *activityModel) add(action string) {
	action = joinSanitised(action)
	if action == "" {
		return
	}
	e := actRec{at: time.Now(), action: action}
	m.entries = append(m.entries, e)
	if len(m.entries) > journalCap {
		m.entries = m.entries[len(m.entries)-journalCap:]
	}
	if m.sink != nil && !m.sinkBroken {
		if err := m.sink(store.ActivityEntry{At: e.at, Action: action}); err != nil {
			m.sinkBroken = true
		}
	}
}

func (m *activityModel) preload(tail []store.ActivityEntry) {
	if len(tail) > journalCap {
		tail = tail[len(tail)-journalCap:]
	}
	out := make([]actRec, 0, len(tail))
	for _, e := range tail {
		out = append(out, actRec{at: e.At, action: e.Action})
	}
	m.entries = append(out, m.entries...)
}

func (m *activityModel) clear() error {
	if m.clearSink != nil {
		if err := m.clearSink(); err != nil {
			return err
		}
	}
	m.entries, m.top = nil, 0
	return nil
}

func (m *activityModel) scrollKey(k string, innerW, innerH int) {
	n := len(m.allRows(innerW))
	m.top = moveScroll(m.top, max(0, n-innerH), k, innerH)
}

func (m activityModel) allRows(innerW int) []string {
	dim := lipgloss.NewStyle().Foreground(dimColor)
	txt := lipgloss.NewStyle().Foreground(textColor)

	msgW, indent := innerW-1-jTimeW-jGap, 1+jTimeW+jGap
	if msgW < jTimeW {
		msgW, indent = max(2, innerW-1), 0
	}

	var rows []string
	for i := len(m.entries) - 1; i >= 0; i-- {
		e := m.entries[i]
		first := true
		for _, line := range wrapPlain(e.action, msgW) {
			if first {
				rows = append(rows, clipANSI(" "+dim.Render(jTime(e.at))+spaces(jGap)+
					txt.Render(line), innerW))
				first = false
				continue
			}
			rows = append(rows, spaces(indent)+txt.Render(line))
		}
	}
	return rows
}

func (m activityModel) body(innerW, innerH int) []string {
	rows := m.allRows(innerW)
	if len(rows) == 0 {
		return emptyBody(innerW, innerH, "Nothing changed yet",
			emptyHint("Hosts, credentials, ~/.ssh files, transfers and edits are recorded here", ""))
	}
	top := clamp(m.top, 0, max(0, len(rows)-1))
	return fitLines(rows[top:min(len(rows), top+innerH)], innerW, innerH)
}

func (m activityModel) status() string {
	if len(m.entries) == 0 {
		return "nothing recorded"
	}
	return journalEntries(len(m.entries))
}

// ------------------------------------------------------------------- shared

// joinSanitised is the one place other machines' bytes are made safe to draw,
// shared by errors and activity so the two cannot drift on what "safe" means.
// Blank lines are dropped, the whole thing is capped, and every line is run
// through sanitizeLine.
func joinSanitised(msg string, more ...string) string {
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
		return ""
	}
	text := strings.Join(lines, "\n")
	if r := []rune(text); len(r) > entryChars {
		text = string(r[:entryChars]) + "…"
	}
	return text
}

// journalEntries counts entries in words. Said in the status slot, the
// confirmation and the toast that follows it — a count worded three ways reads
// as three different numbers.
func journalEntries(n int) string {
	if n == 1 {
		return "1 entry"
	}
	return itoa(n) + " entries"
}
