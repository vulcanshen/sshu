package tui

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	terminu "github.com/vulcanshen/terminu-go"

	"github.com/vulcanshen/sshu/internal/store"
)

// Manage → Hosts: hosts.yaml as a table. Enter connects, after showing what
// it connects to (tdp F1: a confirm may carry the details to read first).

type hostsSection struct {
	s    *sshu
	sec  *section
	list *terminu.List
}

func newHostsSection(s *sshu) *hostsSection {
	h := &hostsSection{s: s}
	h.list = &terminu.List{
		Columns: []terminu.Column{
			// Name, address and tags are long and shrink; past their least,
			// tags go first, the address last: name and address are what the
			// table is scanned by (layout/panel).
			{Title: "Name", Shrink: true}, {Title: "User", Drop: 4}, {Title: "Host", Shrink: true, Drop: 5},
			{Title: "Port", Number: true, Drop: 3}, {Title: "Auth", Drop: 2}, {Title: "Tags", Shrink: true, Drop: 1},
		},
		Filterable: true,
		EmptyFact:  "No hosts yet",
		EmptyHint:  "Press [A] to add a host, or [Space] to see what you can do here",
	}
	h.reload("")
	return h
}

func (h *hostsSection) section() *section {
	h.sec = &section{id: "hosts", label: "Hosts", group: "sshu", body: h.list, actions: h.actions}
	return h.sec
}

func (h *hostsSection) actions() []terminu.Action {
	// Connecting opens a shell on another screen: the confirm is a door, and
	// on Enter the menu goes with it (tdp T1).
	var target store.Host
	ask := func() *terminu.Ask {
		x, _, ok := h.current()
		if !ok {
			return nil
		}
		target = x
		lines, warn := detailLines(hostDetail(x, h.s.d.Creds, h.s.d.SSHConfig, h.s.d.Config.Seconds()))
		return &terminu.Ask{Title: "Connect", Subject: x.Name, Details: lines, Warn: warn,
			Question: "Connect to " + x.Name + "?", Verb: "connect"}
	}
	return []terminu.Action{
		{Key: "Enter", Label: "Connect", Desc: "what it is, then in", Scope: terminu.ItemScope, Hint: "connect",
			AskFor: ask, Clear: true, Do: func() tea.Cmd { return h.s.sh.connect(target) }},
		{Key: "E", Label: "Edit", Desc: "change this host", Scope: terminu.ItemScope, Hint: "edit", Do: h.edit},
		{Key: "D", Label: "Duplicate", Desc: "a new host starting from this one", Scope: terminu.ItemScope, Do: h.duplicate},
		gated(terminu.Action{Key: "X", Label: "Delete", Desc: "remove from hosts.yaml", Scope: terminu.ItemScope}, h.deleteAsk),
		{Key: "A", Label: "Add", Desc: "a new host", Scope: terminu.PanelScope, Hint: "add", Do: h.add},
	}
}

// reload draws the rows from the data, the cursor on sel when given.
func (h *hostsSection) reload(sel string) {
	rows := make([]terminu.Row, len(h.s.d.Hosts))
	for i, x := range h.s.d.Hosts {
		port := ""
		if x.Port > 0 {
			port = strconv.Itoa(x.Port)
		}
		user := x.User
		if x.Auth == store.AuthCredential {
			if r, err := store.Resolve(x, h.s.d.Creds); err == nil {
				user = r.User
			}
		}
		rows[i] = terminu.Row{ID: x.Name, Cells: []string{
			x.Name, user, x.Host, port, authGlyph(string(x.Auth)) + " " + string(x.Auth), store.JoinTags(x.Tags),
		}}
	}
	h.list.SetRows(rows)
	if sel != "" {
		h.list.Select(sel)
	}
}

// current is the host under the cursor.
func (h *hostsSection) current() (store.Host, int, bool) {
	r, ok := h.list.Current()
	if !ok {
		return store.Host{}, -1, false
	}
	i := slices.IndexFunc(h.s.d.Hosts, func(x store.Host) bool { return x.Name == r.ID })
	if i < 0 {
		return store.Host{}, -1, false
	}
	return h.s.d.Hosts[i], i, true
}

func (h *hostsSection) deleteAsk() (terminu.Ask, func() tea.Cmd, bool) {
	x, _, ok := h.current()
	if !ok {
		return terminu.Ask{}, nil, false
	}
	return terminu.Ask{Title: "Delete", Subject: x.Name,
			Details:  []string{"This rewrites hosts.yaml."},
			Question: "Delete host " + x.Name + "?", Verb: "delete"},
		func() tea.Cmd {
			hosts := slices.DeleteFunc(slices.Clone(h.s.d.Hosts), func(y store.Host) bool { return y.Name == x.Name })
			if err := h.s.d.SaveHosts(hosts); err != nil {
				return h.s.a.Error("Delete failed", "Can't write hosts.yaml: "+err.Error())
			}
			h.s.d.Hosts = hosts
			h.reload("")
			h.s.mg.refresh()
			h.s.logChange("deleted host " + x.Name)
			return h.s.a.Toast(terminu.Success, "Deleted "+x.Name)
		}, true
}

func (h *hostsSection) add() tea.Cmd {
	return h.form("New host", "", store.Host{Port: store.DefaultPort, Auth: store.AuthPassword}, -1)
}

func (h *hostsSection) edit() tea.Cmd {
	x, i, ok := h.current()
	if !ok {
		return nil
	}
	return h.form("Edit", x.Name, x, i)
}

// duplicate opens a new host filled in from this one: a new name saves it.
func (h *hostsSection) duplicate() tea.Cmd {
	x, _, ok := h.current()
	if !ok {
		return nil
	}
	return h.form("Duplicate", x.Name, x, -1)
}

// The auth methods in the order the form offers them.
var authMethods = []store.AuthMethod{store.AuthPassword, store.AuthPrivateKey, store.AuthCredential, store.AuthSSHConfig}

// form is the host form (components/dialog/form): add, edit or duplicate.
// replace is the index the edited host has, -1 for a new one.
func (h *hostsSection) form(title, subject string, start store.Host, replace int) tea.Cmd {
	name, host, user := start.Name, start.Host, start.User
	// port is what was typed. The default 22 is not typed: shown while the
	// auth is not sshconfig and saved as 22 then, it is gone once the auth
	// turns to sshconfig, where an empty port lets ~/.ssh/config decide and
	// a 22 would override it (design §6.3, §11.41).
	port := ""
	if start.Port > 0 && (start.Port != store.DefaultPort || start.Auth == store.AuthSSHConfig) {
		port = strconv.Itoa(start.Port)
	}
	cred, identity, password := start.Credential, start.IdentityFile, start.Password
	tags := store.JoinTags(start.Tags)
	auth := &terminu.FormField{Label: "Auth", Options: []string{"password", "privatekey", "credential", "sshconfig"},
		Selected: max(slices.Index(authMethods, start.Auth), 0)}
	method := func() store.AuthMethod { return authMethods[auth.Selected] }
	text := func(label string, v *string, prompt string, check func(string) error) *terminu.FormField {
		return &terminu.FormField{Label: label, Value: func() string { return *v }, Empty: "not set",
			Edit: func() terminu.Input {
				return terminu.TextInput{Title: label, Glyph: glyphPencil, Prompt: prompt, Verb: "set", Old: *v,
					Check: check, Done: func(s string) tea.Cmd { *v = strings.TrimSpace(s); return nil }}
			}}
	}
	nameRule := func(v string) string {
		v = strings.TrimSpace(v)
		switch {
		case v == "":
			return "Name can't be empty"
		case slices.ContainsFunc(h.s.d.Hosts, func(y store.Host) bool { return y.Name == v }) &&
			(replace < 0 || h.s.d.Hosts[replace].Name != v):
			return "Name " + v + " is taken"
		}
		return ""
	}
	nameF := text("Name", &name, "What the host is called in sshu", inputRule(nameRule))
	nameF.Check = func() string { return nameRule(name) }
	hostRule := required("Host")
	hostF := text("Host", &host, "The address, or a Host of ~/.ssh/config", inputRule(hostRule))
	hostF.Check = func() string { return hostRule(host) }
	shownPort := func() string {
		if port == "" && method() != store.AuthSSHConfig {
			return strconv.Itoa(store.DefaultPort)
		}
		return port
	}
	portF := &terminu.FormField{Label: "Port", Value: shownPort, Empty: "ssh decides",
		Edit: func() terminu.Input {
			if method() == store.AuthSSHConfig {
				return optionalPort("1–65535, or empty to let ~/.ssh/config decide", port, func(s string) { port = s })
			}
			return terminu.NumberInput{Title: "Port", Glyph: glyphPencil, Prompt: "1–65535",
				Verb: "set", Old: shownPort(), Min: 1, Max: 65535, Done: func(s string) tea.Cmd {
					port = s
					if s == strconv.Itoa(store.DefaultPort) {
						port = "" // the default, not a port of its own
					}
					return nil
				}}
		}}
	// The input refuses an empty user where the form would (tdp K3).
	userRule := func(v string) string {
		if strings.TrimSpace(v) == "" && method() != store.AuthCredential && method() != store.AuthSSHConfig {
			return "User can't be empty"
		}
		return ""
	}
	userF := text("User", &user, "Who to log in as", inputRule(userRule))
	userF.Disabled = func() bool { return method() == store.AuthCredential }
	userF.Check = func() string { return userRule(user) }
	credF := &terminu.FormField{Label: "Credential", Value: func() string { return cred }, Empty: "not set",
		Disabled: func() bool { return method() != store.AuthCredential },
		Edit: func() terminu.Input {
			names := make([]string, len(h.s.d.Creds))
			for i, c := range h.s.d.Creds {
				names[i] = c.Name
			}
			return terminu.Select{Title: "Credential", Glyph: glyphCred, Options: names, Selected: slices.Index(names, cred),
				Done: func(i int) tea.Cmd { cred = names[i]; return nil }}
		},
		Check: func() string {
			switch {
			case method() != store.AuthCredential:
			case cred == "":
				return "Credential can't be empty"
			case !slices.ContainsFunc(h.s.d.Creds, func(c store.Credential) bool { return c.Name == cred }):
				return "Credential " + cred + " is not in credentials.yaml"
			}
			return ""
		}}
	// The auth needs what it authenticates with: a privatekey host its key
	// file, a password host its password (CHANGELOG 1.4.0).
	identityF := &terminu.FormField{Label: "Identity file", Value: func() string { return identity }, Empty: "not set", Path: true,
		Disabled: func() bool { return method() != store.AuthPrivateKey },
		Edit: func() terminu.Input {
			return identityPicker("Identity file", identity, func(p string) { identity = p })
		},
		Check: func() string {
			if method() == store.AuthPrivateKey && identity == "" {
				return "Identity file can't be empty"
			}
			return ""
		}}
	passwordF := &terminu.FormField{Label: "Password", Empty: "not set",
		Value:    func() string { return maskIf(password) },
		Disabled: func() bool { return method() != store.AuthPassword },
		Edit: func() terminu.Input {
			return terminu.PasswordInput{Title: "Password", Glyph: glyphLock, Prompt: "The password to log in with; it is stored encrypted",
				Verb: "set", Set: password != "", Done: func(v string, clear bool) tea.Cmd { password = v; return nil }}
		},
		Check: func() string {
			if method() == store.AuthPassword && password == "" {
				return "Password can't be empty"
			}
			return ""
		}}
	tagsF := text("Tags", &tags, "Words to find it by, separated by spaces", nil)
	f := &terminu.Form{Title: title, Subject: subject, Glyph: glyphPlus, Name: cmpOr(subject, title), Button: "Save",
		Fields: []*terminu.FormField{nameF, hostF, portF, userF, auth, credF, identityF, passwordF, tagsF}}
	if replace >= 0 {
		f.Glyph = glyphPencil
	}
	f.Submit = func() (tea.Cmd, error) {
		x := store.Host{Name: name, Host: host, User: user, Auth: method(), Tags: store.ParseTags(tags)}
		if p := shownPort(); p != "" {
			x.Port, _ = strconv.Atoi(p)
		}
		switch x.Auth {
		case store.AuthPassword:
			x.Password = password
		case store.AuthPrivateKey:
			x.IdentityFile = identity
		case store.AuthCredential:
			x.Credential, x.User = cred, ""
		}
		if err := x.Validate(); err != nil {
			return nil, err
		}
		hosts := slices.Clone(h.s.d.Hosts)
		verb := "added"
		if replace >= 0 {
			hosts[replace] = x
			verb = "edited"
		} else {
			hosts = append(hosts, x)
		}
		if err := h.s.d.SaveHosts(hosts); err != nil {
			return nil, errors.New("Can't write hosts.yaml: " + err.Error())
		}
		h.s.d.Hosts = hosts
		h.reload(x.Name)
		h.s.mg.refresh()
		h.s.logChange(verb + " host " + x.Name)
		return h.s.a.Toast(terminu.Success, "Saved "+x.Name), nil
	}
	return h.s.a.Form(f)
}

// identityPicker picks a private key, starting in ~/.ssh.
func identityPicker(title, cur string, set func(string)) terminu.Input {
	root := store.ExpandTilde("~/.ssh")
	if cur != "" {
		root = filepath.Dir(store.ExpandTilde(cur))
	}
	if st, err := os.Stat(root); err != nil || !st.IsDir() {
		root = store.ExpandTilde("~")
	}
	return terminu.FilePicker{Title: title, Glyph: glyphKey, Root: root, Value: store.ExpandTilde(cur),
		Done: func(p string) tea.Cmd { set(store.FoldHome(p)); return nil }}
}

// required is the rule of a field that can't be left empty.
func required(name string) func(string) string {
	return func(v string) string {
		if strings.TrimSpace(v) == "" {
			return name + " can't be empty"
		}
		return ""
	}
}

// inputRule is a field's rule as its input popup checks it.
func inputRule(rule func(string) string) func(string) error {
	return func(v string) error {
		if msg := rule(v); msg != "" {
			return errors.New(msg)
		}
		return nil
	}
}

// optionalPort is a port that may be left empty, which leaves the line out
// for ~/.ssh/config to decide (components/input/number: an optional number).
func optionalPort(prompt, old string, set func(string)) terminu.Input {
	return terminu.NumberInput{Title: "Port", Glyph: glyphPencil, Prompt: prompt, Verb: "set", Old: old,
		Min: 1, Max: 65535, Optional: true,
		Done: func(v string) tea.Cmd { set(strings.TrimSpace(v)); return nil }}
}

// maskIf shows a stored password as the fixed mask, nothing as nothing.
func maskIf(pw string) string {
	if pw == "" {
		return ""
	}
	return maskedSecret
}

func cmpOr(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

var _ = fmt.Sprint
