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

	save := func(list []store.Host) error {
		return store.Save(store.File{Hosts: list})
	}
	p := tea.NewProgram(ui.New(hosts.Hosts, save), tea.WithAltScreen())
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
	if h.Auth != store.AuthPassword || h.Password == "" {
		return 1
	}
	fmt.Println(h.Password)
	return 0
}
