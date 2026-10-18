package tui

import (
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	terminu "github.com/vulcanshen/terminu-go"

	"github.com/vulcanshen/sshu/internal/store"
)

// Manage → Logs: three journals, each a table newest first. Errors keeps
// everything the far end printed behind Enter; Connections and Changes open
// whole, every entry in full.

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
	return terminu.Action{Key: "C", Label: "Clear " + label, Desc: "erase " + file, Scope: terminu.PanelScope, Stay: true,
		Do: func() tea.Cmd {
			return s.a.Confirm(terminu.Ask{Title: "Clear", Subject: label, Details: []string{"This empties " + file + "."},
				Question: "Clear " + label + "?", Verb: "clear"},
				func() tea.Cmd {
					if clear != nil {
						if err := clear(); err != nil {
							return s.a.Error("Clear failed", "Can't write "+file+": "+err.Error())
						}
					}
					after()
					s.mg.journalsChanged()
					return s.a.Toast(terminu.Info, "Cleared "+label)
				})
		}}
}

// --- Errors -------------------------------------------------------------------

type errorsSection struct {
	s     *sshu
	sec   *section
	list  *terminu.List
	shown int // error entries there were when Errors was last on screen
}

func newErrorsSection(s *sshu) *errorsSection {
	e := &errorsSection{s: s}
	e.list = &terminu.List{
		Columns:   []terminu.Column{{Title: "Time"}, {Title: "Host"}, {Title: "Cause", Flex: true}},
		EmptyFact: "Nothing has gone wrong",
		EmptyHint: "Failed connections and unreadable files land here; press [Space] to see what you can do",
	}
	e.reload()
	e.shown = e.count()
	return e
}

func (e *errorsSection) section() *section {
	e.sec = &section{id: "errors", label: "Errors", group: "logs", body: e.list, actions: e.actions, shown: e.seen}
	return e.sec
}

func (e *errorsSection) actions() []terminu.Action {
	acts := []terminu.Action{
		{Key: "Enter", Label: "Open", Desc: "everything the far end said", Scope: terminu.ItemScope, Hint: "open", Stay: true, Do: e.open},
	}
	if len(e.s.d.Errors) > 0 {
		acts = append(acts, clearAction(e.s, "errors", "errors.yaml", e.s.d.ClearErrors, func() {
			e.s.d.Errors = nil
			e.shown = 0
		}))
	}
	return acts
}

// count is how many entries are errors: only those count as unread.
func (e *errorsSection) count() int {
	n := 0
	for _, x := range e.s.d.Errors {
		if x.Level != store.LevelWarn {
			n++
		}
	}
	return n
}

func (e *errorsSection) seen()       { e.shown = e.count() }
func (e *errorsSection) unread() int { return max(e.count()-e.shown, 0) }

func (e *errorsSection) reload() {
	es := e.s.d.Errors
	if len(es) > journalCap {
		es = es[len(es)-journalCap:]
		e.s.d.Errors = es
	}
	rows := make([]terminu.Row, 0, len(es))
	for i := len(es) - 1; i >= 0; i-- {
		x := es[i]
		color := terminu.Red
		if x.Level == store.LevelWarn {
			color = terminu.Peach
		}
		rows = append(rows, terminu.Row{ID: strconv.Itoa(i), Cells: []string{
			jTime(x.At), whoCell(x.User, x.Host), terminu.Fg(color).Render(cause(x)),
		}})
	}
	e.list.SetRows(rows)
}

// cause is an entry's headline: written down, or the first line of the rest.
func cause(x store.ErrorEntry) string {
	if x.Cause != "" {
		return x.Cause
	}
	first, _, _ := strings.Cut(x.Error, "\n")
	return first
}

func (e *errorsSection) open() tea.Cmd {
	r, ok := e.list.Current()
	if !ok {
		return nil
	}
	i, _ := strconv.Atoi(r.ID)
	if i >= len(e.s.d.Errors) {
		return nil
	}
	x := e.s.d.Errors[i]
	lines := []string{jDate(x.At) + "  " + who(x.User, x.Host) + "  " + x.Level, ""}
	lines = append(lines, strings.Split(x.Error, "\n")...)
	return e.s.a.Viewer(newViewer("Error", who(x.User, x.Host), glyphLog, lines))
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
		Columns:   []terminu.Column{{Title: "Time"}, {Title: "Host", Flex: true}, {Title: "Result"}},
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
		{Key: "Enter", Label: "Open", Desc: "the whole journal, each entry in full", Scope: terminu.PanelScope, Hint: "open", Stay: true, Do: c.open},
		clearAction(c.s, "connections", "connections.yaml", c.s.d.ClearConns, func() { c.s.d.Conns = nil }),
	}
}

func resultText(r string) string {
	if r == store.ResultSuccess {
		return terminu.Fg(terminu.Green).Render(r)
	}
	return terminu.Fg(terminu.Red).Render(r)
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
		rows = append(rows, terminu.Row{ID: strconv.Itoa(i), Cells: []string{jTime(x.At), whoCell(x.User, x.Host), resultText(x.Result)}})
	}
	c.list.SetRows(rows)
}

func (c *connsSection) open() tea.Cmd {
	var lines []string
	for i := len(c.s.d.Conns) - 1; i >= 0; i-- {
		x := c.s.d.Conns[i]
		lines = append(lines, jDate(x.At)+"  "+who(x.User, x.Host)+"  "+x.Result)
	}
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
		Columns:   []terminu.Column{{Title: "Time"}, {Title: "Change", Flex: true}},
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
		{Key: "Enter", Label: "Open", Desc: "the whole journal, each entry in full", Scope: terminu.PanelScope, Hint: "open", Stay: true, Do: c.open},
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
	return c.s.a.Viewer(newViewer("Changes", "", glyphLog, lines))
}
