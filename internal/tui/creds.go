package tui

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	terminu "github.com/vulcanshen/terminu-go"

	"github.com/vulcanshen/sshu/internal/store"
)

// Manage → Credentials: credentials.yaml, a user and how it authenticates,
// defined once and named by any number of hosts.

type credsSection struct {
	s    *sshu
	sec  *section
	list *terminu.List
}

func newCredsSection(s *sshu) *credsSection {
	c := &credsSection{s: s}
	c.list = &terminu.List{
		Columns:   []terminu.Column{{Title: "Name", Shrink: true}, {Title: "User", Drop: 3}, {Title: "Auth", Drop: 2}, {Title: "Used by", Number: true, Drop: 1}},
		EmptyFact: "No credentials yet",
		EmptyHint: "Press [A] to add one, or [Space] to see what you can do here",
	}
	c.reload("")
	return c
}

func (c *credsSection) section() *section {
	c.sec = &section{id: "creds", label: "Credentials", group: "sshu", body: c.list, actions: c.actions}
	return c.sec
}

func (c *credsSection) actions() []terminu.Action {
	return []terminu.Action{
		{Key: "Enter", Label: "View", Desc: "how this one authenticates", Scope: terminu.ItemScope, Hint: "view", Do: c.view},
		{Key: "E", Label: "Edit", Desc: "change this credential", Scope: terminu.ItemScope, Hint: "edit", Do: c.edit},
		{Key: "D", Label: "Duplicate", Desc: "a new credential starting from this one", Scope: terminu.ItemScope, Do: c.duplicate},
		gated(terminu.Action{Key: "X", Label: "Delete", Desc: "remove from credentials.yaml", Scope: terminu.ItemScope}, c.deleteAsk),
		{Key: "A", Label: "Add", Desc: "a new credential", Scope: terminu.PanelScope, Hint: "add", Do: c.add},
	}
}

func (c *credsSection) reload(sel string) {
	rows := make([]terminu.Row, len(c.s.d.Creds))
	for i, x := range c.s.d.Creds {
		rows[i] = terminu.Row{ID: x.Name, Cells: []string{
			x.Name, x.User, authGlyph(string(x.Auth)) + " " + string(x.Auth), fmt.Sprint(c.hostsUsing(x.Name)),
		}}
	}
	c.list.SetRows(rows)
	if sel != "" {
		c.list.Select(sel)
	}
}

// hostsUsing counts the hosts that name this credential: what a delete or a
// rename has to say, because those hosts break quietly otherwise.
func (c *credsSection) hostsUsing(name string) int {
	n := 0
	for _, h := range c.s.d.Hosts {
		if h.Auth == store.AuthCredential && h.Credential == name {
			n++
		}
	}
	return n
}

func (c *credsSection) current() (store.Credential, int, bool) {
	r, ok := c.list.Current()
	if !ok {
		return store.Credential{}, -1, false
	}
	i := slices.IndexFunc(c.s.d.Creds, func(x store.Credential) bool { return x.Name == r.ID })
	if i < 0 {
		return store.Credential{}, -1, false
	}
	return c.s.d.Creds[i], i, true
}

func (c *credsSection) view() tea.Cmd {
	x, _, ok := c.current()
	if !ok {
		return nil
	}
	lines, _ := detailLines(credDetail(x))
	return c.s.a.Viewer(newViewer("Credential", x.Name, glyphCred, lines))
}

func (c *credsSection) deleteAsk() (terminu.Ask, func() tea.Cmd, bool) {
	x, _, ok := c.current()
	if !ok {
		return terminu.Ask{}, nil, false
	}
	ask := terminu.Ask{Title: "Delete", Subject: x.Name, Details: []string{"This rewrites credentials.yaml."},
		Question: "Delete credential " + x.Name + "?", Verb: "delete"}
	if n := c.hostsUsing(x.Name); n > 0 {
		verb := "reference"
		if n == 1 {
			verb = "references"
		}
		ask.Details = append(ask.Details, plural(n, "host")+" "+verb+" it and will fail to connect.")
		ask.Warn = []int{1}
	}
	return ask, func() tea.Cmd {
		creds := slices.DeleteFunc(slices.Clone(c.s.d.Creds), func(y store.Credential) bool { return y.Name == x.Name })
		if err := c.s.d.SaveCreds(creds); err != nil {
			return c.s.a.Error("Delete failed", "Can't write credentials.yaml: "+err.Error())
		}
		c.s.d.Creds = creds
		c.changed("")
		c.s.logChange(fmt.Sprintf("credential %q deleted", x.Name))
		return c.s.a.Toast(terminu.Success, "Deleted "+x.Name)
	}, true
}

// changed redraws what shows credentials: this list and the hosts' users.
func (c *credsSection) changed(sel string) {
	c.reload(sel)
	c.s.mg.hosts.reload("")
	c.s.mg.refresh()
}

func (c *credsSection) add() tea.Cmd {
	return c.form("New credential", "", store.Credential{Auth: store.AuthPrivateKey}, -1)
}

func (c *credsSection) edit() tea.Cmd {
	x, i, ok := c.current()
	if !ok {
		return nil
	}
	return c.form("Edit", x.Name, x, i)
}

func (c *credsSection) duplicate() tea.Cmd {
	x, _, ok := c.current()
	if !ok {
		return nil
	}
	return c.form("Duplicate", x.Name, x, -1)
}

var credMethods = []store.AuthMethod{store.AuthPassword, store.AuthPrivateKey}

func (c *credsSection) form(title, subject string, start store.Credential, replace int) tea.Cmd {
	name, user, identity, password := start.Name, start.User, start.IdentityFile, start.Password
	auth := &terminu.FormField{Label: "Auth", Options: []string{"password", "privatekey"},
		Selected: max(slices.Index(credMethods, start.Auth), 0)}
	method := func() store.AuthMethod { return credMethods[auth.Selected] }
	nameRule := func(v string) string {
		v = strings.TrimSpace(v)
		switch {
		case v == "":
			return "Name can't be empty"
		case slices.ContainsFunc(c.s.d.Creds, func(y store.Credential) bool { return y.Name == v }) &&
			(replace < 0 || c.s.d.Creds[replace].Name != v):
			return "Name " + v + " is taken"
		}
		return ""
	}
	userRule := required("User")
	nameF := &terminu.FormField{Label: "Name", Value: func() string { return name }, Empty: "not set",
		Edit: func() terminu.Input {
			return terminu.TextInput{Title: "Name", Glyph: glyphPencil, Prompt: "What hosts call this credential", Verb: "set",
				Old: name, Check: inputRule(nameRule), Done: func(v string) tea.Cmd { name = strings.TrimSpace(v); return nil }}
		},
		Check: func() string { return nameRule(name) }}
	userF := &terminu.FormField{Label: "User", Value: func() string { return user }, Empty: "not set",
		Edit: func() terminu.Input {
			return terminu.TextInput{Title: "User", Glyph: glyphPencil, Prompt: "Who to log in as", Verb: "set",
				Old: user, Check: inputRule(userRule), Done: func(v string) tea.Cmd { user = strings.TrimSpace(v); return nil }}
		},
		Check: func() string { return userRule(user) }}
	identityF := &terminu.FormField{Label: "Identity file", Value: func() string { return identity }, Empty: "not set", Path: true,
		Disabled: func() bool { return method() != store.AuthPrivateKey },
		Edit: func() terminu.Input {
			return identityPicker("Identity file", identity, func(p string) { identity = p })
		}}
	passwordF := &terminu.FormField{Label: "Password", Empty: "not set", Value: func() string { return maskIf(password) },
		Disabled: func() bool { return method() != store.AuthPassword },
		Edit: func() terminu.Input {
			return terminu.PasswordInput{Title: "Password", Glyph: glyphLock, Prompt: "The password to log in with; it is stored encrypted",
				Verb: "set", Set: password != "", Done: func(v string, clear bool) tea.Cmd { password = v; return nil }}
		}}
	f := &terminu.Form{Title: title, Subject: subject, Glyph: glyphPlus, Name: cmpOr(subject, title), Button: "Save",
		Fields: []*terminu.FormField{nameF, userF, auth, identityF, passwordF}}
	if replace >= 0 {
		f.Glyph = glyphPencil
	}
	f.Submit = func() (tea.Cmd, error) {
		x := store.Credential{Name: name, User: user, Auth: method()}
		if x.Auth == store.AuthPassword {
			x.Password = password
		} else {
			x.IdentityFile = identity
		}
		if err := x.Validate(); err != nil {
			return nil, err
		}
		creds := slices.Clone(c.s.d.Creds)
		verb, old := "added", ""
		if replace >= 0 {
			old = creds[replace].Name
			creds[replace] = x
			verb = "updated"
		} else {
			creds = append(creds, x)
		}
		if err := c.s.d.SaveCreds(creds); err != nil {
			return nil, errors.New("Can't write credentials.yaml: " + err.Error())
		}
		c.s.d.Creds = creds
		c.changed(x.Name)
		c.s.logChange(fmt.Sprintf("credential %q %s (%s, %s)", x.Name, verb, x.User, x.Auth))
		// A rename leaves the hosts that named it pointing at the old name.
		if old != "" && old != x.Name {
			if n := c.hostsUsing(old); n > 0 {
				line := fmt.Sprintf("%s still reference credential %q", plural(n, "host"), old)
				return c.s.a.Toast(terminu.Warning, line), nil
			}
		}
		return c.s.a.Toast(terminu.Success, "Saved "+x.Name), nil
	}
	return c.s.a.Form(f)
}
