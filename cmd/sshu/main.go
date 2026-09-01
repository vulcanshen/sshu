// Command sshu is a TUI for managing ssh connections and sftp transfers.
package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/vulcanshen/sshu/internal/store"
	"github.com/vulcanshen/sshu/internal/ui"
)

func main() {
	// ssh re-executes this binary as its SSH_ASKPASS helper. That mode prints one
	// password and exits — it must never start the TUI, and must never write
	// anything else to stdout, because ssh reads the first line as the password.
	if name := ui.AskpassHost(); name != "" {
		os.Exit(runAskpass(name))
	}

	hosts, err := store.Load()
	if err != nil {
		// A malformed hosts.yaml is worth refusing to start over: silently
		// showing an empty list would look like data loss and the next save
		// would overwrite whatever the user was mid-way through hand-editing.
		fmt.Fprintln(os.Stderr, "sshu:", err)
		os.Exit(1)
	}

	// A settings file that cannot be parsed is not fatal — sshu runs on the
	// defaults — but it must not be silent either: somebody wrote something and
	// it is not being honoured. stderr is invisible behind the alt screen, so
	// the complaint goes where complaints go now, which is the app log.
	cfg, cfgErr := store.LoadConfig()

	// The log's own file failing to load is itself news — but never fatal, and
	// never a reason to stop recording new events.
	logTail, logErr := store.LoadLog()

	save := func(list []store.Host) error {
		return store.Save(store.File{Hosts: list})
	}
	app := ui.New(hosts.Hosts, save, cfg).WithLog(logTail, store.AppendLog)
	if cfgErr != nil {
		app = app.WithStartupError("config.yaml: " + cfgErr.Error())
	}
	if logErr != nil {
		app = app.WithStartupError("applogs.yaml: " + logErr.Error())
	}
	p := tea.NewProgram(app, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "sshu:", err)
		os.Exit(1)
	}
}

// runAskpass prints the stored password for name. A non-zero exit tells ssh the
// helper had nothing, and ssh falls back to prompting inside the PTY — which is
// the right outcome for a key host, a host that has since been renamed, or a
// hosts.yaml that cannot be read.
func runAskpass(name string) int {
	f, err := store.Load()
	if err != nil {
		return 1
	}
	i := f.Index(name)
	if i < 0 {
		return 1
	}
	h := f.Hosts[i]
	// A credential host stores its password in credentials.yaml, one hop away.
	if h.Auth == store.AuthCredential {
		cf, err := store.LoadCreds()
		if err != nil {
			return 1
		}
		if h, err = store.Resolve(h, cf.Credentials); err != nil {
			return 1
		}
	}
	if h.Auth != store.AuthPassword || h.Password == "" {
		return 1
	}
	fmt.Println(h.Password)
	return 0
}
