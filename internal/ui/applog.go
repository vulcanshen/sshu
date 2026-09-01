package ui

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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

	layer   int
	screenW int
	screenH int
}

func newAppLog() appLog { return appLog{anim: newPopupAnimator("applog")} }

func (m appLog) isActive() bool    { return m.anim.isActive() }
func (m *appLog) close() tea.Cmd   { return m.anim.close() }
func (m *appLog) setSize(w, h int) { m.screenW, m.screenH = w, h }
func (m appLog) unreadErrors() int { return m.unread }

// add records one line.
//
// The message is SANITISED because much of what lands here came off another
// machine — ssh's last words before it gave up are the whole point of this
// thing, and those bytes are the remote's, not ours.
func (m *appLog) add(level logLevel, msg string) {
	msg = strings.TrimSpace(sanitizeLine(msg))
	if msg == "" {
		return
	}
	m.entries = append(m.entries, logEntry{at: time.Now(), level: level, msg: msg})
	if len(m.entries) > logCap {
		m.entries = m.entries[len(m.entries)-logCap:]
	}
	if level == logError {
		m.unread++
	}
}

func (m *appLog) info(msg string)   { m.add(logInfo, msg) }
func (m *appLog) warn(msg string)   { m.add(logWarn, msg) }
func (m *appLog) errorf(msg string) { m.add(logError, msg) }

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
	m.top = moveScroll(m.top, max(0, len(m.entries)-m.rows()), msg.String(), m.rows())
}

// logW is the popup's width. Wide, because the lines it holds are somebody
// else's error messages and those are not written to fit.
const logW = 88

func (m appLog) view() string {
	innerW := popupInnerW(m.screenW, logW)

	var rows []string
	if len(m.entries) == 0 {
		rows = emptyBody(innerW, min(m.rows(), 5), "Nothing has happened yet",
			emptyHint("Failed connections and transfers are recorded here", ""))
	} else {
		dim := lipgloss.NewStyle().Foreground(dimColor)
		// The prefix is "15:04:05 ERR  ", and continuation lines are indented
		// under the message rather than under the timestamp.
		const prefixW = 15
		msgW := max(8, innerW-prefixW-1)
		// Newest first: the reason you opened this is almost always the last
		// thing that went wrong.
		end := min(len(m.entries), m.top+m.rows())
		for i := m.top; i < end && len(rows) < m.rows(); i++ {
			e := m.entries[len(m.entries)-1-i]
			lvl := lipgloss.NewStyle().Foreground(e.level.colour())
			// WRAPPED, not truncated. These lines are somebody else's error
			// messages and the part that says why is at the END of them —
			// "…port 22: Connection refused" — so cutting the tail throws away
			// the only word anybody opened the log to read.
			for j, line := range wrapText(e.msg, msgW) {
				if j == 0 {
					rows = append(rows, " "+dim.Render(e.at.Format("15:04:05"))+" "+
						lvl.Render(e.level.label())+" "+line)
					continue
				}
				if len(rows) >= m.rows() {
					break
				}
				rows = append(rows, spaces(prefixW)+line)
			}
		}
	}

	hint := [][2]string{{"j/k", "scroll"}, {"Esc", "close"}}
	if n := len(m.entries); n > m.rows() {
		hint = append([][2]string{{itoa(m.top + 1), "of " + itoa(n)}}, hint...)
	}
	return drawPopupBox(popupLayerColor(m.layer), " "+glyphInfo+" app log ",
		hintLegend(hint), animRows(m.anim, capRows(rows, m.screenH)), innerW)
}
