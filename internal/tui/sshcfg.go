package tui

import (
	"fmt"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	terminu "github.com/vulcanshen/terminu-go"

	"github.com/vulcanshen/sshu/v3/internal/store"
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
	c.sec = &section{id: "sshcfg", label: "Config", group: "ssh", body: c.list, actions: c.actions,
		fail: func() *terminu.Failure { return unreadFailure("~/.ssh/config", c.s.d.SSHConfigErr) }}
	return c.sec
}

func (c *sshcfgSection) actions() []terminu.Action {
	return []terminu.Action{
		{Key: "Enter", Label: "View", Desc: "every option this block sets", Scope: terminu.ItemScope, Hint: "view", Do: c.view},
		{Key: "E", Label: "Edit", Desc: "change this block", Scope: terminu.ItemScope, Hint: "edit", Do: c.edit},
		{Key: "D", Label: "Duplicate", Desc: "a new block starting from this one", Scope: terminu.ItemScope, Do: c.duplicate},
		gated(terminu.Action{Key: "X", Label: "Delete", Desc: "remove from ~/.ssh/config", Scope: terminu.ItemScope}, c.deleteAsk),
		{Key: "A", Label: "Add", Desc: "a new Host block", Scope: terminu.PanelScope, Hint: "add", Do: c.add},
	}
}

// reload draws the blocks. A block's place is its identity: two blocks may
// carry the same pattern, and ssh means something by that.
func (c *sshcfgSection) reload(sel int) {
	f := c.s.d.SSHConfig
	cols := []terminu.Column{{Title: "Host", Shrink: true}, {Title: "HostName", Shrink: true, Drop: 4}, {Title: "User", Drop: 3}, {Title: "Port", Number: true, Drop: 2}}
	files := f.Files() > 1
	if files {
		cols = append(cols, terminu.Column{Title: "File", Path: true, Shrink: true, Drop: 1})
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
	c.s.logOp("Viewed Host " + b.Patterns + " in ~/.ssh/config")
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
		return c.persist(next, -1, fmt.Sprintf("Deleted Host %q from ~/.ssh/config", b.Patterns), "Deleted "+b.Patterns)
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
		return c.s.a.Error("Save failed", err.Error())
	}
	c.s.logChange(logLine)
	return c.s.a.Toast(terminu.Success, toast)
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
	var patterns, other string
	fixed := make([]store.SSHOption, len(blockFields))
	values := make([]string, len(blockFields))
	var others []store.SSHOption
	// load puts a block's lines in the fields: when the form opens, and when
	// the editor has changed the file under it.
	load := func(b store.SSHBlock) {
		patterns, others = b.Patterns, nil
		taken := make([]bool, len(b.Options))
		for i, key := range blockFields {
			fixed[i], values[i] = store.SSHOption{Key: key}, ""
			if j := b.IndexOf(key); j >= 0 {
				fixed[i], values[i], taken[j] = b.Options[j], b.Options[j].Value, true
			}
		}
		var lines []string
		for j, o := range b.Options {
			if !taken[j] {
				others = append(others, o)
				lines = append(lines, o.Key+" "+o.Value)
			}
		}
		other = strings.Join(lines, "\n")
	}
	load(b)
	saved := func() []string { return append([]string{patterns}, values...) }
	opened := saved()
	changed := func() bool { return !slices.Equal(opened, saved()) }
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
	// write saves the form; the block is then at its place in the file.
	write := func() error {
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
			return err
		}
		disk, err := c.s.d.SaveSSHConfig(next)
		if err != nil {
			// What is on disk now is shown; the form stays, the values in it.
			c.s.d.SSHConfig = disk
			c.reload(-1)
			return err // the form, or the question before Edit file, says so and logs it
		}
		c.s.d.SSHConfig = disk
		c.reload(sel)
		c.s.mg.refresh()
		c.s.logChange(fmt.Sprintf("Host %q %s in ~/.ssh/config", patterns, verb))
		at, opened = sel, saved()
		return nil
	}
	f.Submit = func() (tea.Cmd, error) {
		if err := write(); err != nil {
			return nil, err
		}
		return c.s.a.Toast(terminu.Success, "Saved "+patterns), nil
	}
	// Edit file opens the whole file in $EDITOR over the form, on this
	// block's Host line: $EDITOR knows every keyword (tdp textarea.1: a
	// config file is $EDITOR's). The editor reads the file as it is on disk,
	// so a form with changes, or a block not written yet, asks to save first.
	// When the editor closes, the form reads the block again (reread).
	var reread func() tea.Cmd
	edit := func() tea.Cmd {
		if at < 0 || changed() {
			what := "the changes to Host " + patterns
			if at < 0 {
				what = "the new Host " + patterns
			}
			return c.s.a.Confirm(terminu.Ask{Title: "Edit file", Subject: patterns,
				Details:  []string{"The editor opens the file as it is on disk."},
				Question: "Save " + what + " first?", Verb: "save and edit"},
				func() tea.Cmd {
					if err := write(); err != nil {
						return c.s.a.ErrorAbout("Save failed", patterns, err.Error())
					}
					return c.editFile(at, reread)
				})
		}
		return c.editFile(at, reread)
	}
	reread = func() tea.Cmd {
		fresh, err := c.s.d.SSHConfig.Reload()
		c.s.d.SSHConfigErr = err
		if err != nil {
			// What was read before stays on screen (tdp panel.15).
			return c.s.a.Toast(terminu.Error, "Can't read ~/.ssh/config: "+err.Error())
		}
		c.s.d.SSHConfig = fresh
		i := -1
		if at >= 0 && at < len(fresh.Blocks) && fresh.Blocks[at].Patterns == patterns {
			i = at
		} else {
			i = slices.IndexFunc(fresh.Blocks, func(b store.SSHBlock) bool { return b.Patterns == patterns })
		}
		c.reload(i)
		c.s.mg.refresh()
		if i < 0 {
			at = -1 // gone from the file: saving writes it again
			return nil
		}
		at = i
		load(fresh.Blocks[i])
		opened = saved() // what is on disk: nothing of the form's own to save
		return nil
	}
	f.Buttons = [][]*terminu.FormButton{{{Label: "Edit file", Open: edit}}}
	return c.s.a.Form(f)
}

// editFile opens the file block at lives in with $EDITOR, on its Host line,
// in a terminal popup over the form; after runs when the editor ends.
func (c *sshcfgSection) editFile(at int, after func() tea.Cmd) tea.Cmd {
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
	term.OnExit = func(error) tea.Cmd { deregisterStop(term); return after() }
	registerStop(term)
	return c.s.a.OpenTerminal(term)
}

// firstLine is a multi-line value on a form: its first line, and "…" when
// more follow (tdp input.10).
func firstLine(v string) string {
	first, rest, more := strings.Cut(strings.TrimSpace(v), "\n")
	if more && strings.TrimSpace(rest) != "" {
		return first + " …"
	}
	return first
}
