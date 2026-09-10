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
// All three are TABLES now, with named columns, because every one of them is
// scanned rather than read — you are looking for a machine, a time, or a
// result, and prose makes you read each entry to find out whether it is the
// one. One entry is one row in all three, so a panel can be counted down a
// column.
//
// Errors is the one with somewhere further to go: its rows carry a cause, and
// the whole of what the far end printed is behind Enter. That split is what
// lets the panel be scannable AND keep the fifteen lines of a host key
// mismatch — the two things a single free-text log could not do at once.

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
	// The header row every journal carries, and the reason the panels can be
	// scanned: "which column is this" answered once at the top instead of
	// guessed at on every row.
	jHeaderRows = 1
)

// jCols is the width of the host and user columns, and of whatever takes the
// rest of the row — a cause, an action, or nothing.
//
// Two columns come off the top, not one: a leading space AND a trailing one.
// Without the second, a value that exactly fills its column touches the panel
// border — "success" is seven cells and the result column is seven, so History
// was the row that showed it.
func jCols(innerW, tailFixed int, wantTail bool) (hostW, userW, tailW int) {
	free := innerW - 2 - jTimeW - jGap - tailFixed
	if wantTail {
		free -= jGap
	}
	if free < jMinHostW+jGap+jMinUserW {
		// No room for two columns. The host takes what there is: a row that
		// cannot say which machine is a row that says nothing.
		return max(1, free), 0, 0
	}
	free -= jGap
	hostW = max(jMinHostW, free*30/100)
	userW = max(jMinUserW, free*20/100)
	if wantTail {
		tailW = max(1, free-hostW-userW)
		return hostW, userW, tailW
	}
	// No tail column: host and user split what there is, host taking more.
	hostW = max(jMinHostW, free*60/100)
	userW = max(jMinUserW, free-hostW)
	if over := hostW + userW - free; over > 0 {
		hostW = max(jMinHostW, hostW-over)
	}
	return hostW, userW, 0
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

// jHeader draws a journal's column names. Dim, because it is a label and never
// the thing being read — the same register the hosts table's header wears.
func jHeader(innerW int, cells ...string) string {
	dim := lipgloss.NewStyle().Foreground(dimColor)
	return dim.Render(padRight(" "+strings.Join(cells, spaces(jGap)), innerW))
}

// ------------------------------------------------------------------- errors

type errorRec struct {
	at    time.Time
	host  string
	user  string
	level errLevel // levelWarn | levelError
	// cause is the headline, one line: what a row shows and a toast could hold.
	cause string
	// text is the whole of it, cause included — everything the far end printed
	// before it gave up. Behind Enter, because a row of it would be unreadable
	// and a panel of rows of it was what the old log looked like.
	text string
}

// errorsModel is manage → Logs → Errors.
type errorsModel struct {
	entries []errorRec // oldest first; the table reads it backwards
	// unread counts ERRORS since the panel was last on screen. Warnings do not
	// count: the badge means "something failed", and diluting it with "something
	// was mentioned" is how a badge stops being looked at.
	unread int
	// cursor and top index the table NEWEST FIRST, which is the order it is
	// drawn in. Errors is the one journal with a cursor, because it is the one
	// with somewhere to go: Enter opens what the row could not fit.
	cursor int
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

// at maps a row on screen — newest first — back to the entry behind it.
func (m errorsModel) at(i int) (errorRec, bool) {
	if i < 0 || i >= len(m.entries) {
		return errorRec{}, false
	}
	return m.entries[len(m.entries)-1-i], true
}

// current is the entry under the cursor, which is what Enter opens.
func (m errorsModel) current() (errorRec, bool) { return m.at(m.cursor) }

// add records one failure. cause is the headline; more is everything else the
// far end said. Both are sanitised and capped here, because this is where
// other machines' output lands.
func (m *errorsModel) add(level errLevel, host, user, cause string, more ...string) {
	text := joinSanitised(cause, more...)
	if text == "" {
		return
	}
	// The headline is the first line of what was recorded, not the raw
	// argument: sanitising can empty it, and a row showing a cause the panel
	// does not have would be a row pointing at nothing.
	head := text
	if i := strings.IndexByte(head, '\n'); i >= 0 {
		head = head[:i]
	}
	e := errorRec{at: time.Now(), host: host, user: user, level: level,
		cause: head, text: text}
	m.entries = append(m.entries, e)
	if len(m.entries) > journalCap {
		m.entries = m.entries[len(m.entries)-journalCap:]
	}
	if level == levelError {
		m.unread++
	}
	// A new entry arrives at the TOP of a newest-first table, so a cursor
	// resting anywhere below it would drift onto a different row without
	// moving. It follows the shift instead, unless it is already at the top —
	// where "newest" is where it wants to be anyway.
	if m.cursor > 0 {
		m.cursor = min(m.cursor+1, len(m.entries)-1)
	}
	// Written through AFTER the in-memory append: whatever happens to the disk,
	// the panel shows the event.
	if m.sink != nil && !m.sinkBroken {
		if err := m.sink(store.ErrorEntry{At: e.at, Host: host, User: user,
			Level: level.name(), Cause: head, Error: text}); err != nil {
			m.sinkBroken = true
			m.entries = append(m.entries, errorRec{at: time.Now(), level: levelWarn,
				cause: "errors.yaml cannot be written",
				text:  "errors.yaml: " + sanitizeLine(err.Error()) + " — new entries stay in memory only"})
		}
	}
}

func (m *errorsModel) errorf(host, user, cause string, more ...string) {
	m.add(levelError, host, user, cause, more...)
}

func (m *errorsModel) warn(host, user, cause string, more ...string) {
	m.add(levelWarn, host, user, cause, more...)
}

func (m *errorsModel) preload(tail []store.ErrorEntry) {
	if len(tail) > journalCap {
		tail = tail[len(tail)-journalCap:]
	}
	out := make([]errorRec, 0, len(tail))
	for _, e := range tail {
		// A file written before Errors had a cause column still opens: the
		// headline is the first line of what it does have.
		cause := e.Cause
		if cause == "" {
			cause = e.Error
			if i := strings.IndexByte(cause, '\n'); i >= 0 {
				cause = cause[:i]
			}
		}
		out = append(out, errorRec{at: e.At, host: e.Host, user: e.User,
			level: levelNamed(e.Level), cause: cause, text: e.Error})
	}
	m.entries = append(out, m.entries...)
}

func (m *errorsModel) clear() error {
	if m.clearSink != nil {
		if err := m.clearSink(); err != nil {
			return err
		}
	}
	m.entries, m.top, m.cursor, m.unread = nil, 0, 0, 0
	return nil
}

// handleKey walks the rows. Same vocabulary as every other list in the app, so
// a key added there lands here too (nav.go).
func (m *errorsModel) handleKey(k string, innerH int) {
	if len(m.entries) == 0 {
		return
	}
	m.cursor = moveCursor(m.cursor, len(m.entries), k, m.visibleRows(innerH))
	m.ensureVisible(innerH)
}

func (m errorsModel) visibleRows(innerH int) int { return max(1, innerH-jHeaderRows) }

func (m *errorsModel) ensureVisible(innerH int) {
	vis := m.visibleRows(innerH)
	if m.cursor < m.top {
		m.top = m.cursor
	}
	if m.cursor >= m.top+vis {
		m.top = m.cursor - vis + 1
	}
	m.top = max(0, m.top)
}

// row draws one entry: when, where, as whom, and why in one line.
//
// The cause is TRUNCATED where every other column is padded, and that is the
// trade this panel makes: a row you can scan, with the whole of it one Enter
// away. The old shape wrapped the failure under its own columns, which kept
// every word and made a panel of failures unscannable.
func (m errorsModel) row(e errorRec, selected bool, innerW int) string {
	hostW, userW, causeW := jCols(innerW, 0, true)

	plain := " " + padRight(jTime(e.at), jTimeW) + spaces(jGap) + padRight(jField(e.host), hostW)
	if userW > 0 {
		plain += spaces(jGap) + padRight(jField(e.user), userW)
	}
	if causeW > 0 {
		plain += spaces(jGap) + padRight(e.cause, causeW)
	}

	if selected {
		bar := lipgloss.NewStyle().Foreground(lipgloss.Color(baseHex)).Background(rowSelColor)
		return bar.Render(padRight(plain, innerW))
	}

	// The timestamp carries the level, because it is the one cell every entry
	// has. Red is "something is wrong", peach is "worth knowing" — the two
	// bands §11.48 separated, used here for exactly that split.
	stamp := lipgloss.NewStyle().Foreground(e.level.colour())
	dim := lipgloss.NewStyle().Foreground(dimColor)
	txt := lipgloss.NewStyle().Foreground(textColor)

	out := " " + stamp.Render(padRight(jTime(e.at), jTimeW)) +
		spaces(jGap) + txt.Render(padRight(jField(e.host), hostW))
	if userW > 0 {
		out += spaces(jGap) + dim.Render(padRight(jField(e.user), userW))
	}
	if causeW > 0 {
		out += spaces(jGap) + txt.Render(padRight(e.cause, causeW))
	}
	return out + spaces(max(0, innerW-dispW(plain)))
}

func (m errorsModel) body(innerW, innerH int) []string {
	if len(m.entries) == 0 {
		return emptyBody(innerW, innerH, "Nothing has failed",
			emptyHint("Refused connections, failed transfers and edits that could not be written back land here", ""))
	}
	hostW, userW, causeW := jCols(innerW, 0, true)
	cells := []string{padRight("Time", jTimeW), padRight("Host", hostW)}
	if userW > 0 {
		cells = append(cells, padRight("User", userW))
	}
	if causeW > 0 {
		cells = append(cells, padRight("Cause", causeW))
	}

	out := make([]string, 0, innerH)
	out = append(out, jHeader(innerW, cells...))
	vis := m.visibleRows(innerH)
	top := clamp(m.top, 0, max(0, len(m.entries)-1))
	for i := top; i < len(m.entries) && len(out) < top+vis+jHeaderRows; i++ {
		e, ok := m.at(i)
		if !ok {
			break
		}
		out = append(out, m.row(e, i == m.cursor, innerW))
	}
	return fitLines(out, innerW, innerH)
}

func (m errorsModel) status() string {
	if len(m.entries) == 0 {
		return "no errors"
	}
	return itoa(m.cursor+1) + "/" + journalEntries(len(m.entries))
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

func (m *historyModel) scrollKey(k string, innerH int) {
	m.top = moveScroll(m.top, max(0, len(m.entries)-(innerH-jHeaderRows)), k, innerH)
}

// rows is one line per attempt, newest first. Fixed height on purpose: this
// panel exists to be counted down a column — "that host, three times this
// afternoon, two of them red" — and a row that can grow breaks the count.
func (m historyModel) rows(innerW int) []string {
	dim := lipgloss.NewStyle().Foreground(dimColor)
	txt := lipgloss.NewStyle().Foreground(textColor)
	ok := lipgloss.NewStyle().Foreground(liveColor)
	bad := lipgloss.NewStyle().Foreground(warnColor)

	hostW, userW, _ := jCols(innerW, jResultW+jGap, false)

	out := make([]string, 0, len(m.entries))
	for i := len(m.entries) - 1; i >= 0; i-- {
		e := m.entries[i]
		style, word := ok, store.ResultSuccess
		if !e.ok {
			style, word = bad, store.ResultFail
		}
		row := " " + dim.Render(padRight(jTime(e.at), jTimeW)) + spaces(jGap) +
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
	hostW, userW, _ := jCols(innerW, jResultW+jGap, false)
	cells := []string{padRight("Time", jTimeW), padRight("Host", hostW)}
	if userW > 0 {
		cells = append(cells, padRight("User", userW))
	}
	cells = append(cells, padRight("Result", jResultW))

	rows := m.rows(innerW)
	top := clamp(m.top, 0, max(0, len(rows)-1))
	out := append([]string{jHeader(innerW, cells...)},
		rows[top:min(len(rows), top+max(1, innerH-jHeaderRows))]...)
	return fitLines(out, innerW, innerH)
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

func (m *activityModel) scrollKey(k string, innerH int) {
	m.top = moveScroll(m.top, max(0, len(m.entries)-(innerH-jHeaderRows)), k, innerH)
}

func (m activityModel) rows(innerW int) []string {
	dim := lipgloss.NewStyle().Foreground(dimColor)
	txt := lipgloss.NewStyle().Foreground(textColor)
	actionW := max(1, innerW-2-jTimeW-jGap)

	out := make([]string, 0, len(m.entries))
	for i := len(m.entries) - 1; i >= 0; i-- {
		e := m.entries[i]
		out = append(out, clipANSI(" "+dim.Render(padRight(jTime(e.at), jTimeW))+
			spaces(jGap)+txt.Render(padRight(e.action, actionW)), innerW))
	}
	return out
}

func (m activityModel) body(innerW, innerH int) []string {
	if len(m.entries) == 0 {
		return emptyBody(innerW, innerH, "Nothing changed yet",
			emptyHint("Hosts, credentials, ~/.ssh files, transfers and edits are recorded here", ""))
	}
	actionW := max(1, innerW-2-jTimeW-jGap)
	rows := m.rows(innerW)
	top := clamp(m.top, 0, max(0, len(rows)-1))
	out := append([]string{jHeader(innerW, padRight("Time", jTimeW), padRight("Action", actionW))},
		rows[top:min(len(rows), top+max(1, innerH-jHeaderRows))]...)
	return fitLines(out, innerW, innerH)
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
