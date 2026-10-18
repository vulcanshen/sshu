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
// it connects to (Rules F1: a confirm may carry the details to read first).

type hostsSection struct {
	s    *sshu
	sec  *section
	list *terminu.List
}

func newHostsSection(s *sshu) *hostsSection {
	h := &hostsSection{s: s}
	h.list = &terminu.List{
		Columns: []terminu.Column{
			// Tags give up room first: name and address are what the table is
			// scanned by.
			{Title: "Name"}, {Title: "User"}, {Title: "Host"},
			{Title: "Port", Number: true}, {Title: "Auth"}, {Title: "Tags", Flex: true},
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
	return []terminu.Action{
		{Key: "Enter", Label: "Connect", Desc: "what it is, then in", Scope: terminu.ItemScope, Hint: "connect", Do: h.connect},
		{Key: "E", Label: "Edit", Desc: "change this host", Scope: terminu.ItemScope, Hint: "edit", Do: h.edit},
		{Key: "D", Label: "Duplicate", Desc: "a new host starting from this one", Scope: terminu.ItemScope, Do: h.duplicate},
		{Key: "X", Label: "Delete", Desc: "remove from hosts.yaml", Scope: terminu.ItemScope, Do: h.delete},
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

func (h *hostsSection) connect() tea.Cmd {
	x, _, ok := h.current()
	if !ok {
		return nil
	}
	lines, warn := detailLines(hostDetail(x, h.s.d.Creds, h.s.d.SSHConfig, h.s.d.Config.Seconds()))
	return h.s.a.Confirm(terminu.Ask{Title: "Connect", Subject: x.Name, Details: lines, Warn: warn,
		Question: "Connect to " + x.Name + "?", Verb: "connect"},
		func() tea.Cmd { return h.s.sh.connect(x) })
}

func (h *hostsSection) delete() tea.Cmd {
	x, _, ok := h.current()
	if !ok {
		return nil
	}
	return h.s.a.Confirm(terminu.Ask{Title: "Delete", Subject: x.Name,
		Details:  []string{"This rewrites hosts.yaml."},
		Question: "Delete host " + x.Name + "?", Verb: "delete"},
		func() tea.Cmd {
			hosts := slices.DeleteFunc(slices.Clone(h.s.d.Hosts), func(y store.Host) bool { return y.Name == x.Name })
			if err := h.s.d.SaveHosts(hosts); err != nil {
				return h.s.a.Error("Delete", "Can't write hosts.yaml: "+err.Error())
			}
			h.s.d.Hosts = hosts
			h.reload("")
			h.s.mg.refresh()
			h.s.logChange("deleted host " + x.Name)
			return h.s.a.Toast(terminu.Info, "Deleted "+x.Name)
		})
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
	port := ""
	if start.Port > 0 {
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
	nameF := text("Name", &name, "What the host is called in sshu", nil)
	nameF.Check = func() string {
		switch {
		case strings.TrimSpace(name) == "":
			return "Name can't be empty"
		case slices.ContainsFunc(h.s.d.Hosts, func(y store.Host) bool { return y.Name == name }) &&
			(replace < 0 || h.s.d.Hosts[replace].Name != name):
			return "Name " + name + " is taken"
		}
		return ""
	}
	hostF := text("Host", &host, "The address, or a Host of ~/.ssh/config", nil)
	hostF.Check = func() string {
		if strings.TrimSpace(host) == "" {
			return "Host can't be empty"
		}
		return ""
	}
	portF := &terminu.FormField{Label: "Port", Value: func() string { return port }, Empty: "ssh decides",
		Edit: func() terminu.Input {
			if method() == store.AuthSSHConfig {
				return optionalPort("1–65535, or empty to let ~/.ssh/config decide", port, func(s string) { port = s })
			}
			return terminu.NumberInput{Title: "Port", Glyph: glyphPencil, Prompt: "1–65535",
				Verb: "set", Old: port, Min: 1, Max: 65535, Done: func(s string) tea.Cmd { port = s; return nil }}
		},
		Check: func() string {
			if port == "" && method() != store.AuthSSHConfig {
				return "Port can't be empty"
			}
			return ""
		}}
	userF := text("User", &user, "Who to log in as", nil)
	userF.Disabled = func() bool { return method() == store.AuthCredential }
	userF.Check = func() string {
		if strings.TrimSpace(user) == "" && method() != store.AuthCredential && method() != store.AuthSSHConfig {
			return "User can't be empty"
		}
		return ""
	}
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
	identityF := &terminu.FormField{Label: "Identity file", Value: func() string { return identity }, Empty: "not set", Path: true,
		Disabled: func() bool { return method() != store.AuthPrivateKey },
		Edit: func() terminu.Input {
			return identityPicker("Identity file", identity, func(p string) { identity = p })
		}}
	passwordF := &terminu.FormField{Label: "Password", Empty: "not set",
		Value:    func() string { return maskIf(password) },
		Disabled: func() bool { return method() != store.AuthPassword },
		Edit: func() terminu.Input {
			return terminu.PasswordInput{Title: "Password", Glyph: glyphLock, Prompt: "The password to log in with; it is stored encrypted",
				Verb: "set", Set: password != "", Done: func(v string, clear bool) tea.Cmd { password = v; return nil }}
		}}
	tagsF := text("Tags", &tags, "Words to find it by, separated by spaces", nil)
	f := &terminu.Form{Title: title, Subject: subject, Glyph: glyphPlus, Name: cmpOr(subject, title), Button: "Save",
		Fields: []*terminu.FormField{nameF, hostF, portF, userF, auth, credF, identityF, passwordF, tagsF}}
	if replace >= 0 {
		f.Glyph = glyphPencil
	}
	f.Submit = func() (tea.Cmd, error) {
		x := store.Host{Name: name, Host: host, User: user, Auth: method(), Tags: store.ParseTags(tags)}
		if port != "" {
			x.Port, _ = strconv.Atoi(port)
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
		return h.s.a.Toast(terminu.Info, "Saved "+x.Name), nil
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

// optionalPort is a port that may be left empty. The lib's number input
// refuses an empty value, so it is a text input with the number's rule.
// TODO(lib SS16): a number input that may be empty.
func optionalPort(prompt, old string, set func(string)) terminu.Input {
	return terminu.TextInput{Title: "Port", Glyph: glyphPencil, Prompt: prompt, Verb: "set", Old: old, MaxLen: 5,
		Check: func(v string) error {
			v = strings.TrimSpace(v)
			if v == "" {
				return nil
			}
			if n, err := strconv.Atoi(v); err != nil || n < 1 || n > 65535 {
				return errors.New("Port must be 1-65535")
			}
			return nil
		},
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
