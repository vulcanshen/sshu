// Command sshu is a TUI for managing ssh connections and sftp transfers.
package main

import (
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/vulcanshen/sshu/internal/store"
	"github.com/vulcanshen/sshu/internal/ui"
	"github.com/vulcanshen/sshu/internal/version"
)

func main() {
	// Before anything renders: a sshu started by another one over ssh has no
	// COLORTERM, and would quantise its whole UI to 256 colours (§11.46).
	ui.AdoptForwardedColor()

	// `sshu version` prints the build version and exits — checked before
	// anything else, so it answers even with a broken config.
	if len(os.Args) > 1 && os.Args[1] == "version" {
		fmt.Println("sshu " + version.Display())
		return
	}

	// ssh re-executes this binary as its SSH_ASKPASS helper. That mode prints one
	// password and exits — it must never start the TUI, and must never write
	// anything else to stdout, because ssh reads the first line as the password.
	if name := ui.AskpassHost(); name != "" {
		os.Exit(runAskpass(name))
	}

	hosts, dupHosts, err := store.Load()
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

	// A journal's own file failing to load is itself news — but never fatal,
	// and never a reason to stop recording new events. Three files, three
	// independent failures: errors.yaml being unreadable says nothing about
	// whether history.yaml is.
	errTail, errErr := store.LoadErrors()
	histTail, histErr := store.LoadHistory()
	actTail, actErr := store.LoadActivity()

	// Credentials are data like hosts, but a broken credentials.yaml only
	// breaks the hosts that reference it — sshu still starts, and says so.
	credsFile, dupCreds, credsErr := store.LoadCreds()

	// ~/.ssh/config is not sshu's file, and it is already deciding what tab [3]
	// does — sshu launches the real ssh, which reads it. Missing is the ordinary
	// empty state; unreadable is worth saying, and never a reason not to start.
	sshCfg, sshCfgErr := store.LoadSSHConfig()

	// known_hosts is the other file sshu did not write and already reads:
	// remote/sftp.go verifies against it, and refuses outright when a key has
	// changed. Missing is the ordinary empty state.
	knownHosts, knownErr := store.LoadKnownHosts()

	// sshu's own files are brought up to today's format here, once, at a moment
	// the user is present for — rather than on whatever their next edit happens
	// to be, which would put a format change inside an unrelated action.
	//
	// This is the ONLY place it happens. The askpass helper returned above,
	// before store.Load: it runs inside ssh's authentication, and a program
	// invoked to print one password has no business rewriting configuration.
	// The early exit is what makes that structural instead of a convention.
	versionNotes := reconcileVersions(hosts, credsFile, credsErr)

	save := func(list []store.Host) error {
		return store.Save(store.File{Hosts: list})
	}
	saveCreds := func(list []store.Credential) error {
		return store.SaveCreds(store.CredsFile{Credentials: list})
	}
	app := ui.New(hosts.Hosts, save, cfg).
		WithJournals(
			errTail, store.AppendError, store.ClearErrors,
			histTail, store.AppendHistory, store.ClearHistory,
			actTail, store.AppendActivity, store.ClearActivity).
		WithCredentials(credsFile.Credentials, saveCreds).
		WithSSHConfig(sshCfg, store.SaveSSHConfig).
		WithKnownHosts(knownHosts, store.SaveKnownHosts)
	if cfgErr != nil {
		app = app.WithStartupError("config.yaml: " + cfgErr.Error())
	}
	for _, j := range []struct {
		file string
		err  error
	}{{"errors.yaml", errErr}, {"history.yaml", histErr}, {"activity.yaml", actErr}} {
		if j.err != nil {
			app = app.WithStartupError(j.file + ": " + j.err.Error())
		}
	}
	if credsErr != nil {
		app = app.WithStartupError("credentials.yaml: " + credsErr.Error())
	}
	if sshCfgErr != nil {
		app = app.WithStartupError("~/.ssh/config: " + sshCfgErr.Error())
	}
	if knownErr != nil {
		app = app.WithStartupError("~/.ssh/known_hosts: " + knownErr.Error())
	}
	// A dropped duplicate is not an error — sshu is running on a list that is
	// now internally consistent — but it is a difference between the file and
	// what is on screen, and the user is the only one who can close it.
	if len(dupHosts) > 0 {
		app = app.WithStartupWarning(dupeWarning("hosts.yaml", dupHosts))
	}
	if len(dupCreds) > 0 {
		app = app.WithStartupWarning(dupeWarning("credentials.yaml", dupCreds))
	}
	for _, n := range versionNotes {
		app = app.WithStartupWarning(n)
	}
	// stdin goes through sshu first: a parent sshu addresses a layer with an
	// escape sequence, and Bubble Tea would decode it into keystrokes of its
	// own (design §11.45). Everything that is not a command passes through.
	in, pump := ui.NestInput(os.Stdin)
	p := tea.NewProgram(app, tea.WithAltScreen(), tea.WithInput(in))
	go pump(func(m any) { p.Send(m) })

	// SIGHUP is the terminal window closing. Bubble Tea does not catch it, and
	// the default action would end sshu with every child ssh still running —
	// each leads its own session on its PTY, so no signal reaches them on its
	// own.
	hup := make(chan os.Signal, 1)
	signal.Notify(hup, syscall.SIGHUP)
	go func() {
		<-hup
		ui.KillChildren()
		p.Quit()
	}()

	_, runErr := p.Run()
	// Whatever door the program left through — q, Ctrl+C inside the app, an
	// outside SIGINT or SIGTERM (both end the loop WITHOUT the model's own
	// quit path), or the loop failing — the children go too.
	ui.KillChildren()
	switch {
	case errors.Is(runErr, tea.ErrInterrupted):
		os.Exit(130) // the conventional 128+SIGINT
	case runErr != nil:
		fmt.Fprintln(os.Stderr, "sshu:", runErr)
		os.Exit(1)
	}
}

// dupeWarning is the one line the app log gets about entries a load had to drop.
//
// It NAMES them. "2 duplicates were ignored" would leave the user hunting
// through a file for which ones, and the whole reason to say anything is that
// they go and fix it. A name appearing twice in the list is not a bug in the
// message: it means that name was in the file three times.
// reconcileVersions upgrades sshu's own files in place, and reports anything
// it could not settle. It never stops startup: a version problem is about the
// FILE, and refusing to run would take away the tool the user needs to look at
// it.
//
// credsErr matters. A credentials.yaml that failed to parse comes back as an
// empty document, and writing that back would turn "sshu could not read your
// file" into "sshu deleted your credentials". Nothing is written for a file
// that did not load.
func reconcileVersions(hosts store.File, creds store.CredsFile, credsErr error) []string {
	var out []string
	if hosts.FromNewerSshu() {
		out = append(out, "hosts.yaml was written by a newer sshu — it is shown as read, "+
			"but saving will be refused. Upgrade sshu.")
	} else if hosts.NeedsUpgrade() {
		if err := store.Save(hosts); err != nil {
			out = append(out, "hosts.yaml could not be upgraded: "+err.Error())
		}
	}
	if credsErr != nil {
		return out
	}
	if creds.FromNewerSshu() {
		out = append(out, "credentials.yaml was written by a newer sshu — it is shown as read, "+
			"but saving will be refused. Upgrade sshu.")
	} else if creds.NeedsUpgrade() {
		if err := store.SaveCreds(creds); err != nil {
			out = append(out, "credentials.yaml could not be upgraded: "+err.Error())
		}
	}
	return out
}

func dupeWarning(file string, names []string) string {
	quoted := make([]string, len(names))
	for i, n := range names {
		quoted[i] = strconv.Quote(n)
	}
	noun := "entries"
	if len(names) == 1 {
		noun = "entry"
	}
	return fmt.Sprintf("%s: ignored %d duplicate %s (%s); the first of each name is the one in use",
		file, len(names), noun, strings.Join(quoted, ", "))
}

// runAskpass prints the stored password for name. A non-zero exit tells ssh the
// helper had nothing, and ssh falls back to prompting inside the PTY — which is
// the right outcome for a key host, a host that has since been renamed, or a
// hosts.yaml that cannot be read.
func runAskpass(name string) int {
	f, _, err := store.Load()
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
		cf, _, err := store.LoadCreds()
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
