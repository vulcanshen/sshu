package tui

import (
	"strconv"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	terminu "github.com/vulcanshen/terminu-go"

	"github.com/vulcanshen/sshu/v3/internal/store"
)

// Manage → Logs: two journals, each a table newest first, opening whole with
// every entry in full. What went wrong is the App Log's (!), with everything
// the far end printed.

// journalCap is how many entries a table shows, as many as the file keeps.
const journalCap = 500

func jTime(t time.Time) string { return t.Format("15:04:05") }
func jDate(t time.Time) string { return t.Format("2006-01-02 15:04:05") }

// who is user@host, or the half there is, or a dash.
func who(user, host string) string {
	switch {
	case host == "":
		return "—"
	case user == "":
		return host
	}
	return user + "@" + host
}

// whoCell is who in a table, cut so the cause beside it keeps its room.
func whoCell(user, host string) string { return terminu.Truncate(who(user, host), 28) }

// clearAction is a journal's [C]lear: it asks, then empties the file.
func clearAction(s *sshu, label, file string, clear func() error, after func()) terminu.Action {
	return gated(terminu.Action{Key: "C", Label: "Clear " + label, Desc: "erase " + file, Scope: terminu.PanelScope},
		func() (terminu.Ask, func() tea.Cmd, bool) {
			return terminu.Ask{Title: "Clear", Subject: label, Details: []string{"This empties " + file + "."},
					Question: "Clear " + label + "?", Verb: "clear"},
				func() tea.Cmd {
					if clear != nil {
						if err := clear(); err != nil {
							return s.a.Error("Clear failed", "Can't write "+file+": "+err.Error())
						}
					}
					after()
					s.mg.journalsChanged()
					return s.a.Toast(terminu.Success, "Cleared "+label)
				}, true
		})
}

// --- Connections --------------------------------------------------------------

type connsSection struct {
	s    *sshu
	sec  *section
	list *terminu.List
}

func newConnsSection(s *sshu) *connsSection {
	c := &connsSection{s: s}
	c.list = &terminu.List{
		Columns:   []terminu.Column{{Title: "Time", Drop: 1}, {Title: "Host", Shrink: true}, {Title: "Result", Drop: 2}},
		EmptyFact: "No connections yet",
		EmptyHint: "Every ssh and sftp connection lands here; press [M] then a host to make one",
	}
	c.reload()
	return c
}

func (c *connsSection) section() *section {
	c.sec = &section{id: "conns", label: "Connections", group: "logs", body: c.list, actions: c.actions}
	return c.sec
}

func (c *connsSection) actions() []terminu.Action {
	if len(c.s.d.Conns) == 0 {
		return nil
	}
	return []terminu.Action{
		{Key: "Enter", Label: "Open", Desc: "the whole journal, each entry in full", Scope: terminu.PanelScope, Hint: "open", Do: c.open},
		clearAction(c.s, "connections", "connections.yaml", c.s.d.ClearConns, func() { c.s.d.Conns = nil }),
	}
}

// resultSpans is a connection's result in its colour.
func resultSpans(r string) []terminu.Span {
	if r == store.ResultSuccess {
		return []terminu.Span{{Text: r, Style: terminu.Fg(terminu.Green)}}
	}
	return []terminu.Span{{Text: r, Style: terminu.Fg(terminu.Red)}}
}

func (c *connsSection) reload() {
	es := c.s.d.Conns
	if len(es) > journalCap {
		es = es[len(es)-journalCap:]
		c.s.d.Conns = es
	}
	rows := make([]terminu.Row, 0, len(es))
	for i := len(es) - 1; i >= 0; i-- {
		x := es[i]
		rows = append(rows, terminu.Row{ID: strconv.Itoa(i), Cells: []string{jTime(x.At), whoCell(x.User, x.Host), x.Result},
			Styled: [][]terminu.Span{nil, nil, resultSpans(x.Result)}})
	}
	c.list.SetRows(rows)
}

func (c *connsSection) open() tea.Cmd {
	var lines []string
	for i := len(c.s.d.Conns) - 1; i >= 0; i-- {
		x := c.s.d.Conns[i]
		lines = append(lines, jDate(x.At)+"  "+who(x.User, x.Host)+"  "+x.Result)
	}
	c.s.logOp("Viewed the connections journal")
	return c.s.a.Viewer(newViewer("Connections", "", glyphLog, lines))
}

// --- Changes ------------------------------------------------------------------

type changesSection struct {
	s    *sshu
	sec  *section
	list *terminu.List
}

func newChangesSection(s *sshu) *changesSection {
	c := &changesSection{s: s}
	c.list = &terminu.List{
		Columns:   []terminu.Column{{Title: "Time", Drop: 1}, {Title: "Change", Shrink: true}},
		EmptyFact: "Nothing changed yet",
		EmptyHint: "What you change through sshu lands here; press [Space] to see what you can do",
	}
	c.reload()
	return c
}

func (c *changesSection) section() *section {
	c.sec = &section{id: "changes", label: "Changes", group: "logs", body: c.list, actions: c.actions}
	return c.sec
}

func (c *changesSection) actions() []terminu.Action {
	if len(c.s.d.Changes) == 0 {
		return nil
	}
	return []terminu.Action{
		{Key: "Enter", Label: "Open", Desc: "the whole journal, each entry in full", Scope: terminu.PanelScope, Hint: "open", Do: c.open},
		clearAction(c.s, "changes", "changes.yaml", c.s.d.ClearChgs, func() { c.s.d.Changes = nil }),
	}
}

func (c *changesSection) reload() {
	es := c.s.d.Changes
	if len(es) > journalCap {
		es = es[len(es)-journalCap:]
		c.s.d.Changes = es
	}
	rows := make([]terminu.Row, 0, len(es))
	for i := len(es) - 1; i >= 0; i-- {
		rows = append(rows, terminu.Row{ID: strconv.Itoa(i), Cells: []string{jTime(es[i].At), es[i].Action}})
	}
	c.list.SetRows(rows)
}

func (c *changesSection) open() tea.Cmd {
	var lines []string
	for i := len(c.s.d.Changes) - 1; i >= 0; i-- {
		lines = append(lines, jDate(c.s.d.Changes[i].At)+"  "+c.s.d.Changes[i].Action)
	}
	c.s.logOp("Viewed the changes journal")
	return c.s.a.Viewer(newViewer("Changes", "", glyphLog, lines))
}
