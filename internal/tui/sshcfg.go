package tui

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	terminu "github.com/vulcanshen/terminu-go"

	"github.com/vulcanshen/sshu/internal/store"
)

// Manage → Config: the Host blocks of ~/.ssh/config, Include followed. The
// file is ssh's; sshu edits a block's own lines and leaves every other line,
// comments included, as it was.

type sshcfgSection struct {
	s    *sshu
	sec  *section
	list *terminu.List
}

func newSSHCfgSection(s *sshu) *sshcfgSection {
	c := &sshcfgSection{s: s}
	c.list = &terminu.List{
		EmptyFact: "No Host blocks in ~/.ssh/config",
		EmptyHint: "Press [A] to add one, or [Space] to see what you can do here",
	}
	c.reload(-1)
	return c
}

func (c *sshcfgSection) section() *section {
	c.sec = &section{id: "sshcfg", label: "Config", group: "ssh", body: c.list, actions: c.actions}
	return c.sec
}

func (c *sshcfgSection) actions() []terminu.Action {
	return []terminu.Action{
		{Key: "Enter", Label: "View", Desc: "every option this block sets", Scope: terminu.ItemScope, Hint: "view", Do: c.view},
		{Key: "E", Label: "Edit", Desc: "change this block", Scope: terminu.ItemScope, Hint: "edit", Do: c.edit},
		{Key: "D", Label: "Duplicate", Desc: "a new block starting from this one", Scope: terminu.ItemScope, Do: c.duplicate},
		{Key: "X", Label: "Delete", Desc: "remove from ~/.ssh/config", Scope: terminu.ItemScope, Do: c.delete},
		{Key: "A", Label: "Add", Desc: "a new Host block", Scope: terminu.PanelScope, Hint: "add", Do: c.add},
	}
}

// reload draws the blocks. A block's place is its identity: two blocks may
// carry the same pattern, and ssh means something by that.
func (c *sshcfgSection) reload(sel int) {
	f := c.s.d.SSHConfig
	cols := []terminu.Column{{Title: "Host"}, {Title: "HostName", Flex: true}, {Title: "User"}, {Title: "Port", Number: true}}
	files := f.Files() > 1
	if files {
		cols = append(cols, terminu.Column{Title: "File", Path: true, Flex: true})
	}
	c.list.Columns = cols
	rows := make([]terminu.Row, len(f.Blocks))
	for i, b := range f.Blocks {
		cells := []string{b.Patterns, b.Option("HostName"), b.Option("User"), b.Option("Port")}
		if files {
			cells = append(cells, store.FoldHome(f.Path(b.File())))
		}
		rows[i] = terminu.Row{ID: strconv.Itoa(i), Cells: cells}
	}
	c.list.SetRows(rows)
	if sel >= 0 {
		c.list.Select(strconv.Itoa(sel))
	}
}

func (c *sshcfgSection) current() (store.SSHBlock, int, bool) {
	r, ok := c.list.Current()
	if !ok {
		return store.SSHBlock{}, -1, false
	}
	i, err := strconv.Atoi(r.ID)
	if err != nil || i >= len(c.s.d.SSHConfig.Blocks) {
		return store.SSHBlock{}, -1, false
	}
	return c.s.d.SSHConfig.Blocks[i], i, true
}

// blockDetail is what the table cannot show: every option line and where in
// which file the block lives.
func blockDetail(b store.SSHBlock, file string) []detailSection {
	from, to := b.LineRange()
	out := []detailSection{{title: file, rows: []detailRow{
		{label: "Host", value: b.Patterns},
		{label: "Lines", value: fmt.Sprintf("%d-%d", from, to)},
	}}}
	opts := detailSection{title: "Options"}
	for _, o := range b.Options {
		opts.rows = append(opts.rows, detailRow{label: o.Key, value: o.Value})
	}
	if len(opts.rows) == 0 {
		opts.rows = []detailRow{{label: "none", value: "this block sets nothing"}}
	}
	return append(out, opts)
}

func (c *sshcfgSection) view() tea.Cmd {
	b, _, ok := c.current()
	if !ok {
		return nil
	}
	lines, _ := detailLines(blockDetail(b, store.FoldHome(c.s.d.SSHConfig.Path(b.File()))))
	return c.s.a.Viewer(&terminu.Viewer{Title: "Host", Subject: b.Patterns, Glyph: glyphCog, Lines: lines})
}

// hostsThroughBlock counts the sshu hosts this block has a word in, and how
// many take their HostName from it: those stop resolving when it goes.
func (c *sshcfgSection) hostsThroughBlock(at int) (matched, resolving int) {
	f := c.s.d.SSHConfig
	b := f.Blocks[at]
	for _, h := range c.s.d.Hosts {
		if !b.Matches(h.Host) {
			continue
		}
		matched++
		for _, o := range f.Effective(h.Host) {
			if strings.EqualFold(o.Key, "HostName") {
				if o.Block == at {
					resolving++
				}
				break
			}
		}
	}
	return matched, resolving
}

func (c *sshcfgSection) delete() tea.Cmd {
	b, at, ok := c.current()
	if !ok {
		return nil
	}
	file := store.FoldHome(c.s.d.SSHConfig.Path(b.File()))
	ask := terminu.Ask{Title: "Delete", Subject: b.Patterns, Details: []string{"This rewrites " + file + "."},
		Question: "Delete Host " + b.Patterns + "?", Verb: "delete"}
	if n := len(b.Options); n > 0 {
		verb := "go"
		if n == 1 {
			verb = "goes"
		}
		ask.Details = append(ask.Details, plural(n, "option")+" "+verb+" with it.")
	}
	if matched, resolving := c.hostsThroughBlock(at); resolving > 0 {
		verb := "resolve"
		if resolving == 1 {
			verb = "resolves"
		}
		ask.Warn = []int{len(ask.Details)}
		ask.Details = append(ask.Details, fmt.Sprintf("%s %s through it and will not connect.", plural(resolving, "sshu host"), verb))
	} else if matched > 0 {
		verb := "inherit"
		if matched == 1 {
			verb = "inherits"
		}
		ask.Details = append(ask.Details, fmt.Sprintf("%s %s options from it.", plural(matched, "sshu host"), verb))
	}
	return c.s.a.Confirm(ask, func() tea.Cmd {
		next, err := c.s.d.SSHConfig.Delete(at)
		if err != nil {
			return c.s.a.Error("Delete", err.Error())
		}
		return c.persist(next, -1, fmt.Sprintf("~/.ssh/config: Host %q deleted", b.Patterns), "Deleted "+b.Patterns)
	})
}

// persist writes the file and shows what is now on disk: another editor may
// have got there first, and the refusal hands back its version.
func (c *sshcfgSection) persist(next store.SSHConfigFile, sel int, logLine, toast string) tea.Cmd {
	saved, err := c.s.d.SaveSSHConfig(next)
	c.s.d.SSHConfig = saved
	c.reload(sel)
	c.s.mg.refresh()
	if err != nil {
		c.s.logError(store.LevelWarn, "", "", err.Error(), err.Error())
		return c.s.a.Error("Save", err.Error())
	}
	c.s.logChange(logLine)
	return c.s.a.Toast(terminu.Info, toast)
}

func (c *sshcfgSection) add() tea.Cmd { return c.form("New Host block", "", store.SSHBlock{}, -1) }

func (c *sshcfgSection) edit() tea.Cmd {
	b, at, ok := c.current()
	if !ok {
		return nil
	}
	return c.form("Edit", b.Patterns, b, at)
}

func (c *sshcfgSection) duplicate() tea.Cmd {
	b, _, ok := c.current()
	if !ok {
		return nil
	}
	// A new block owns no lines yet: its options are written fresh.
	fresh := store.SSHBlock{Patterns: b.Patterns}
	for _, o := range b.Options {
		fresh.Options = append(fresh.Options, store.SSHOption{Key: o.Key, Value: o.Value})
	}
	return c.form("Duplicate", b.Patterns, fresh, -1)
}

// The keywords the form gives a field of their own; the rest are the block's
// other options, one per line.
var blockFields = []string{"HostName", "User", "Port", "IdentityFile"}

// form is the Host block form. at is the block edited, -1 for a new one.
// Each option keeps the line it came from, so an untouched one is written
// back exactly as it was; a value emptied takes its line out.
func (c *sshcfgSection) form(title, subject string, b store.SSHBlock, at int) tea.Cmd {
	patterns := b.Patterns
	fixed := make([]store.SSHOption, len(blockFields))
	values := make([]string, len(blockFields))
	taken := make([]bool, len(b.Options))
	for i, key := range blockFields {
		fixed[i] = store.SSHOption{Key: key}
		if j := b.IndexOf(key); j >= 0 {
			fixed[i], values[i], taken[j] = b.Options[j], b.Options[j].Value, true
		}
	}
	var others []store.SSHOption
	var lines []string
	for j, o := range b.Options {
		if !taken[j] {
			others = append(others, o)
			lines = append(lines, o.Key+" "+o.Value)
		}
	}
	other := strings.Join(lines, "\n")
	text := func(label string, v *string, prompt string) *terminu.FormField {
		return &terminu.FormField{Label: label, Value: func() string { return *v }, Empty: "not set",
			Edit: func() terminu.Input {
				return terminu.TextInput{Title: label, Glyph: glyphPencil, Prompt: prompt, Verb: "set", Old: *v,
					Done: func(s string) tea.Cmd { *v = strings.TrimSpace(s); return nil }}
			}}
	}
	hostF := text("Host", &patterns, "Patterns: prod, prod-*, or *")
	hostF.Check = func() string {
		if patterns == "" {
			return "Host can't be empty"
		}
		return ""
	}
	fields := []*terminu.FormField{hostF,
		text("HostName", &values[0], "The real address ssh connects to"),
		text("User", &values[1], "Who to log in as"),
		{Label: "Port", Value: func() string { return values[2] }, Empty: "not set",
			Edit: func() terminu.Input {
				return optionalPort("1–65535, or empty to leave the line out", values[2], func(s string) { values[2] = s })
			}},
		{Label: "IdentityFile", Value: func() string { return values[3] }, Empty: "not set", Path: true,
			Edit: func() terminu.Input {
				return identityPicker("IdentityFile", values[3], func(p string) { values[3] = p })
			}},
		{Label: "Other options", Stacked: true, Value: func() string { return firstLine(other) },
			Empty: "none",
			Edit: func() terminu.Input {
				return terminu.Textarea{Title: "Other options", Glyph: glyphCog, Name: "Other options", Old: other,
					Done: func(s string) tea.Cmd { other = s; return nil }}
			},
			Check: func() string { _, err := parseOptions(other); return errText(err) }},
	}
	f := &terminu.Form{Title: title, Subject: subject, Glyph: glyphPlus, Name: cmpOr(subject, title), Button: "Save", Fields: fields}
	if at >= 0 {
		f.Glyph = glyphPencil
	}
	f.Submit = func() (tea.Cmd, error) {
		parsed, err := parseOptions(other)
		if err != nil {
			return nil, err
		}
		out := store.SSHBlock{Patterns: patterns}
		for i := range blockFields {
			o := fixed[i]
			o.Value = values[i]
			out.Options = append(out.Options, o)
		}
		out.Options = append(out.Options, reuseLines(others, parsed)...)
		var next store.SSHConfigFile
		verb, sel := "added", -1
		if at >= 0 {
			next, err = c.s.d.SSHConfig.Set(at, out)
			verb, sel = "updated", at
		} else {
			next, err = c.s.d.SSHConfig.Add(out)
			sel = len(next.Blocks) - 1
		}
		if err != nil {
			return nil, err
		}
		return c.persist(next, sel, fmt.Sprintf("~/.ssh/config: Host %q %s", patterns, verb), "Saved "+patterns), nil
	}
	return c.s.a.Form(f)
}

// parseOptions reads the other options, one "Keyword value" per line.
func parseOptions(text string) ([]store.SSHOption, error) {
	var out []store.SSHOption
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		key, value, ok := strings.Cut(line, " ")
		if !ok || strings.TrimSpace(value) == "" {
			return nil, errors.New("Other options: " + line + " needs a value after the keyword")
		}
		out = append(out, store.SSHOption{Key: key, Value: strings.TrimSpace(value)})
	}
	return out, nil
}

// reuseLines gives each option the line of the first old option with the
// same keyword that is not yet taken, so editing one value leaves the others'
// lines where they were.
func reuseLines(old, parsed []store.SSHOption) []store.SSHOption {
	used := make([]bool, len(old))
	for i, p := range parsed {
		for j, o := range old {
			if !used[j] && strings.EqualFold(o.Key, p.Key) {
				used[j] = true
				parsed[i].At = o.At
				break
			}
		}
	}
	return parsed
}

func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// firstLine is a multi-line value on a form: its first line, and "…" when
// more follow (components/input/README, how a value is shown).
func firstLine(v string) string {
	first, rest, more := strings.Cut(strings.TrimSpace(v), "\n")
	if more && strings.TrimSpace(rest) != "" {
		return first + " …"
	}
	return first
}
