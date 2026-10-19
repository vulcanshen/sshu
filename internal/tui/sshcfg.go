package tui

import (
	"fmt"
	"path/filepath"
	"slices"
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
		{Key: "Enter", Label: "View", Desc: "every option this block sets", Scope: terminu.ItemScope, Hint: "view", Stay: true, Do: c.view},
		{Key: "E", Label: "Edit", Desc: "change this block", Scope: terminu.ItemScope, Hint: "edit", Stay: true, Do: c.edit},
		{Key: "D", Label: "Duplicate", Desc: "a new block starting from this one", Scope: terminu.ItemScope, Stay: true, Do: c.duplicate},
		gated(terminu.Action{Key: "X", Label: "Delete", Desc: "remove from ~/.ssh/config", Scope: terminu.ItemScope}, c.deleteAsk),
		{Key: "A", Label: "Add", Desc: "a new Host block", Scope: terminu.PanelScope, Hint: "add", Stay: true, Do: c.add},
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
	return c.s.a.Viewer(newViewer("Host", b.Patterns, glyphCog, lines))
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

func (c *sshcfgSection) deleteAsk() (terminu.Ask, func() tea.Cmd, bool) {
	b, at, ok := c.current()
	if !ok {
		return terminu.Ask{}, nil, false
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
	return ask, func() tea.Cmd {
		next, err := c.s.d.SSHConfig.Delete(at)
		if err != nil {
			return c.s.a.Error("Delete failed", err.Error())
		}
		return c.persist(next, -1, fmt.Sprintf("~/.ssh/config: Host %q deleted", b.Patterns), "Deleted "+b.Patterns)
	}, true
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
		return c.s.a.Error("Save failed", err.Error())
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

// The keywords the form gives a field of their own; the block's other
// options are shown, and changed in the file itself (Edit file).
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
	opened := append([]string{patterns}, values...)
	changed := func() bool { return !slices.Equal(opened, append([]string{patterns}, values...)) }
	text := func(label string, v *string, prompt string, check func(string) error) *terminu.FormField {
		return &terminu.FormField{Label: label, Value: func() string { return *v }, Empty: "not set",
			Edit: func() terminu.Input {
				return terminu.TextInput{Title: label, Glyph: glyphPencil, Prompt: prompt, Verb: "set", Old: *v,
					Check: check, Done: func(s string) tea.Cmd { *v = strings.TrimSpace(s); return nil }}
			}}
	}
	hostRule := required("Host")
	hostF := text("Host", &patterns, "Patterns: prod, prod-*, or *", inputRule(hostRule))
	hostF.Check = func() string { return hostRule(patterns) }
	fields := []*terminu.FormField{hostF,
		text("HostName", &values[0], "The real address ssh connects to", nil),
		text("User", &values[1], "Who to log in as", nil),
		{Label: "Port", Value: func() string { return values[2] }, Empty: "not set",
			Edit: func() terminu.Input {
				return optionalPort("1–65535, or empty to leave the line out", values[2], func(s string) { values[2] = s })
			}},
		{Label: "IdentityFile", Value: func() string { return values[3] }, Empty: "not set", Path: true,
			Edit: func() terminu.Input {
				return identityPicker("IdentityFile", values[3], func(p string) { values[3] = p })
			}},
		// Shown only: the focus passes it by, and Edit file changes it.
		{Label: "Others", Stacked: true, Value: func() string { return firstLine(other) }, Empty: "none",
			Disabled: func() bool { return true }},
	}
	f := &terminu.Form{Title: title, Subject: subject, Glyph: glyphPlus, Name: cmpOr(subject, title), Button: "Save", Fields: fields}
	if at >= 0 {
		f.Glyph = glyphPencil
	}
	// write saves the form and gives the block's place in the file.
	write := func() (int, error) {
		out := store.SSHBlock{Patterns: patterns}
		for i := range blockFields {
			o := fixed[i]
			o.Value = values[i]
			out.Options = append(out.Options, o)
		}
		out.Options = append(out.Options, others...)
		var next store.SSHConfigFile
		var err error
		verb, sel := "added", -1
		if at >= 0 {
			next, err = c.s.d.SSHConfig.Set(at, out)
			verb, sel = "updated", at
		} else {
			next, err = c.s.d.SSHConfig.Add(out)
			sel = len(next.Blocks) - 1
		}
		if err != nil {
			return -1, err
		}
		saved, err := c.s.d.SaveSSHConfig(next)
		if err != nil {
			// What is on disk now is shown; the form stays, the values in it.
			c.s.d.SSHConfig = saved
			c.reload(-1)
			c.s.logError(store.LevelWarn, "", "", err.Error(), err.Error())
			return -1, err
		}
		c.s.d.SSHConfig = saved
		c.reload(sel)
		c.s.mg.refresh()
		c.s.logChange(fmt.Sprintf("~/.ssh/config: Host %q %s", patterns, verb))
		return sel, nil
	}
	f.Submit = func() (tea.Cmd, error) {
		if _, err := write(); err != nil {
			return nil, err
		}
		return c.s.a.Toast(terminu.Info, "Saved "+patterns), nil
	}
	// Edit file opens the file in $EDITOR, which knows every keyword: the
	// form's changes, and a new block, are written first, so the editor opens
	// on what the form shows (tdp textarea: a config file is $EDITOR's).
	// TODO(lib SS19): the button opens the editor over the form, asking first
	// when it has changes, once a form's button can open a terminal popup and
	// have the form read its values again.
	f.Buttons = [][]*terminu.FormButton{{{Label: "Edit file", Submit: func() (tea.Cmd, error) {
		sel := at
		if at < 0 || changed() {
			var err error
			if sel, err = write(); err != nil {
				return nil, err
			}
		}
		return c.editFile(sel), nil
	}}}}
	return c.s.a.Form(f)
}

// editFile opens the file block at lives in with $EDITOR, on its Host line,
// and the block's form again on what the file says once the editor ends.
func (c *sshcfgSection) editFile(at int) tea.Cmd {
	f := c.s.d.SSHConfig
	if at < 0 || at >= len(f.Blocks) {
		return nil
	}
	b := f.Blocks[at]
	file := store.ExpandTilde(f.Path(b.File())) // Path is in ~ form, which the editor takes as it is
	from, _ := b.LineRange()
	argv, err := editorArgv(file, "+"+strconv.Itoa(from))
	if err != nil {
		return c.s.a.Error("Edit failed", err.Error())
	}
	term := &terminu.Terminal{Title: "Edit", Subject: filepath.Base(file), Command: argv, Ends: true, Env: editorEnv()}
	term.OnExit = func(error) tea.Cmd {
		deregisterStop(term)
		return c.reread(at, b.Patterns)
	}
	registerStop(term)
	return c.s.a.OpenTerminal(term)
}

// reread takes ~/.ssh/config from disk after the editor, and opens the form
// of the block that was edited: the one at its place with its patterns, else
// the first with them.
func (c *sshcfgSection) reread(at int, patterns string) tea.Cmd {
	fresh, err := c.s.d.SSHConfig.Reload()
	if err != nil {
		c.s.logError(store.LevelError, "", "", "~/.ssh/config: "+err.Error(), err.Error())
		return c.s.a.Error("Read failed", err.Error())
	}
	c.s.d.SSHConfig = fresh
	i := -1
	if at < len(fresh.Blocks) && fresh.Blocks[at].Patterns == patterns {
		i = at
	} else {
		i = slices.IndexFunc(fresh.Blocks, func(b store.SSHBlock) bool { return b.Patterns == patterns })
	}
	c.reload(i)
	c.s.mg.refresh()
	if i < 0 {
		return c.s.a.Toast(terminu.Info, "Host "+patterns+" is no longer in ~/.ssh/config")
	}
	return c.form("Edit", patterns, fresh.Blocks[i], i)
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
