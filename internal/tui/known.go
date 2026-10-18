package tui

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	terminu "github.com/vulcanshen/terminu-go"

	"github.com/vulcanshen/sshu/internal/remote"
	"github.com/vulcanshen/sshu/internal/store"
)

// Manage → KnownHosts: ~/.ssh/known_hosts. Add asks a host for its key and
// shows the fingerprint before anything is trusted.

type knownSection struct {
	s    *sshu
	sec  *section
	list *terminu.List
}

func newKnownSection(s *sshu) *knownSection {
	k := &knownSection{s: s}
	k.list = &terminu.List{
		Columns:   []terminu.Column{{Title: "Names", Flex: true}, {Title: "Type"}, {Title: "Fingerprint", Flex: true}},
		EmptyFact: "No host keys in ~/.ssh/known_hosts",
		EmptyHint: "Press [A] to ask a host for its key, or [Space] to see what you can do here",
	}
	k.reload(-1)
	return k
}

func (k *knownSection) section() *section {
	k.sec = &section{id: "known", label: "KnownHosts", group: "ssh", body: k.list, actions: k.actions}
	return k.sec
}

func (k *knownSection) actions() []terminu.Action {
	return []terminu.Action{
		{Key: "Enter", Label: "View", Desc: "the whole key, and where it sits", Scope: terminu.ItemScope, Hint: "view", Stay: true, Do: k.view},
		{Key: "E", Label: "Edit", Desc: "which names this key is trusted for", Scope: terminu.ItemScope, Hint: "edit", Stay: true, Do: k.edit},
		{Key: "X", Label: "Delete", Desc: "stop trusting this key", Scope: terminu.ItemScope, Stay: true, Do: k.delete},
		{Key: "A", Label: "Add", Desc: "ask a host for its key, then decide", Scope: terminu.PanelScope, Hint: "add", Stay: true, Do: k.add},
	}
}

// label is the entry's names, or what stands for a hashed name.
func knownLabel(e store.KnownHostEntry) string {
	if e.Hashed() {
		return "(hashed)"
	}
	return e.Hosts
}

func shortKeyType(t string) string {
	return strings.TrimPrefix(strings.TrimPrefix(t, "ssh-"), "ecdsa-sha2-")
}

func (k *knownSection) reload(sel int) {
	es := k.s.d.Known.Entries
	rows := make([]terminu.Row, len(es))
	for i, e := range es {
		name := knownLabel(e)
		if e.Marker != "" {
			name = e.Marker + " " + name
		}
		rows[i] = terminu.Row{ID: strconv.Itoa(i), Cells: []string{name, shortKeyType(e.Type), e.Fingerprint()}}
	}
	k.list.SetRows(rows)
	if sel >= 0 {
		k.list.Select(strconv.Itoa(sel))
	}
}

func (k *knownSection) current() (store.KnownHostEntry, int, bool) {
	r, ok := k.list.Current()
	if !ok {
		return store.KnownHostEntry{}, -1, false
	}
	i, err := strconv.Atoi(r.ID)
	if err != nil || i >= len(k.s.d.Known.Entries) {
		return store.KnownHostEntry{}, -1, false
	}
	return k.s.d.Known.Entries[i], i, true
}

func knownDetail(e store.KnownHostEntry) []detailSection {
	rows := []detailRow{{label: "Names", value: e.Hosts}}
	if e.Hashed() {
		rows = append(rows, detailRow{label: "Hashed", value: "the name cannot be read back from the file"})
	}
	if e.Marker != "" {
		rows = append(rows, detailRow{label: "Marker", value: e.Marker})
	}
	rows = append(rows, detailRow{label: "Line", value: strconv.Itoa(e.At)})
	key := []detailRow{{label: "Type", value: e.Type}, {label: "Fingerprint", value: e.Fingerprint()}}
	if e.Comment != "" {
		key = append(key, detailRow{label: "Comment", value: e.Comment})
	}
	key = append(key, detailRow{label: "Key", value: e.Key})
	return []detailSection{{title: "~/.ssh/known_hosts", rows: rows}, {title: "Key", rows: key}}
}

func (k *knownSection) view() tea.Cmd {
	e, _, ok := k.current()
	if !ok {
		return nil
	}
	lines, _ := detailLines(knownDetail(e))
	return k.s.a.Viewer(newViewer("Host key", knownLabel(e), glyphShield, lines))
}

func (k *knownSection) edit() tea.Cmd {
	e, at, ok := k.current()
	if !ok {
		return nil
	}
	return k.s.a.Text(terminu.TextInput{Title: "Trusted for", Subject: knownLabel(e), Glyph: glyphPencil,
		Prompt: "Which names the key of " + knownLabel(e) + " belongs to, separated by commas", Verb: "save", Name: "Names", Old: e.Hosts,
		Check: func(v string) error {
			v = strings.TrimSpace(v)
			switch {
			case v == "":
				return errors.New("Names can't be empty")
			case strings.ContainsAny(v, " \t"):
				return errors.New("Names can't have spaces: separate them with commas")
			}
			return nil
		},
		Done: func(v string) tea.Cmd {
			next, err := k.s.d.Known.SetHosts(at, v)
			if err != nil {
				return k.s.a.Error("Save failed", err.Error())
			}
			return k.persist(next, at, fmt.Sprintf("~/.ssh/known_hosts: %q is now trusted for %q", e.Hosts, strings.TrimSpace(v)), "Saved")
		}})
}

func (k *knownSection) delete() tea.Cmd {
	e, at, ok := k.current()
	if !ok {
		return nil
	}
	return k.s.a.Confirm(terminu.Ask{Title: "Delete", Subject: knownLabel(e),
		Details:  []string{e.Fingerprint(), "The next connection there will ask again."},
		Question: "Stop trusting the key for " + knownLabel(e) + "?", Verb: "delete"},
		func() tea.Cmd {
			next, err := k.s.d.Known.Delete(at)
			if err != nil {
				return k.s.a.Error("Delete failed", err.Error())
			}
			return k.persist(next, -1, fmt.Sprintf("~/.ssh/known_hosts: key for %q removed (%s)", e.Hosts, e.Fingerprint()),
				"Forgot "+knownLabel(e))
		})
}

func (k *knownSection) persist(next store.KnownHostsFile, sel int, logLine, toast string) tea.Cmd {
	saved, err := k.s.d.SaveKnown(next)
	k.s.d.Known = saved
	k.reload(sel)
	k.s.mg.refresh()
	if err != nil {
		k.s.logError(store.LevelWarn, "", "", err.Error(), err.Error())
		return k.s.a.Error("Save failed", err.Error())
	}
	k.s.logChange(logLine)
	return k.s.a.Toast(terminu.Info, toast)
}

// knownName is how known_hosts writes a host on a port: bare on 22.
func knownName(host string, port int) string {
	if port == store.DefaultPort {
		return host
	}
	return "[" + host + "]:" + strconv.Itoa(port)
}

// add asks a host for its key: a form for where, then the fingerprint and a
// question before anything is written.
func (k *knownSection) add() tea.Cmd {
	host, port := "", strconv.Itoa(store.DefaultPort)
	hostF := &terminu.FormField{Label: "Host", Value: func() string { return host }, Empty: "not set",
		Edit: func() terminu.Input {
			return terminu.TextInput{Title: "Host", Glyph: glyphPencil, Prompt: "The name or address to ask", Verb: "set",
				Old: host, Check: inputRule(required("Host")), Done: func(v string) tea.Cmd { host = strings.TrimSpace(v); return nil }}
		},
		Check: func() string { return required("Host")(host) }}
	portF := &terminu.FormField{Label: "Port", Value: func() string { return port },
		Edit: func() terminu.Input {
			return terminu.NumberInput{Title: "Port", Glyph: glyphPencil, Prompt: "1–65535", Verb: "set",
				Old: port, Min: 1, Max: 65535, Done: func(v string) tea.Cmd { port = v; return nil }}
		},
		Check: func() string {
			if port == "" {
				return "Port can't be empty"
			}
			return ""
		}}
	return k.s.a.Form(&terminu.Form{Title: "Ask for key", Glyph: glyphShield, Name: "Ask for key", Button: "Ask",
		Fields: []*terminu.FormField{hostF, portF},
		SubmitLong: func(ctx context.Context) (tea.Cmd, error) {
			p, _ := strconv.Atoi(port)
			type scanned struct {
				key remote.HostKey
				err error
			}
			got := make(chan scanned, 1)
			go func() {
				key, err := remote.ScanHostKey(host, p, k.s.d.Config.Timeout())
				got <- scanned{key, err}
			}()
			var key remote.HostKey
			select {
			case <-ctx.Done():
				return nil, ctx.Err() // the form is gone: nobody reads this
			case r := <-got:
				if r.err != nil {
					return nil, r.err
				}
				key = r.key
			}
			names := knownName(host, p)
			return k.trust(names, key), nil
		}})
}

// trust shows the key a host offered and writes it only on Enter.
func (k *knownSection) trust(names string, key remote.HostKey) tea.Cmd {
	return k.s.a.Confirm(terminu.Ask{Title: "Trust", Subject: names,
		Details:  []string{names + " offered this key:", shortKeyType(key.Type) + "  " + key.Fingerprint},
		Warn:     []int{},
		Question: "Trust it? Check it against the server first.", Verb: "trust"},
		func() tea.Cmd {
			next, err := k.s.d.Known.Add(store.KnownHostEntry{Hosts: names, Type: key.Type, Key: key.Key})
			if err != nil {
				return k.s.a.Error("Trust failed", err.Error())
			}
			fp := store.KnownHostEntry{Type: key.Type, Key: key.Key}.Fingerprint()
			return k.persist(next, len(next.Entries)-1, fmt.Sprintf("~/.ssh/known_hosts: trusted %s for %q", fp, names),
				"Trusting "+names)
		})
}
