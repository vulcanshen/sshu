// Command sshu is an ssh and sftp front end for the terminal.
package main

import (
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"github.com/vulcanshen/sshu/internal/store"
	"github.com/vulcanshen/sshu/internal/tui"
	"github.com/vulcanshen/sshu/internal/version"
)

const usage = `sshu — an ssh and sftp front end for the terminal

Usage:
  sshu            open it: Space shows what you can do, ? the keys
  sshu version    print the version (also --version, -v)
  sshu help       print this (also --help, -h)

Environment:
  SSHU__CONFIG               the settings directory (default ~/.config/sshu)
  SSHU__STATE                where the three journals go (default: the settings directory)
  SSHU__KEY_FILE             the key that seals stored passwords (default: .sshukey in the settings directory)
  SSHU__ICON_WIDTH           1 or 2: how many cells a Nerd Font icon takes, instead of asking the terminal
  SSHU__POWERLINE_WIDTH      1 or 2: the same for a powerline symbol
  TERMINU__ICON_WIDTH        the same as SSHU__ICON_WIDTH, for every terminu app
  TERMINU__POWERLINE_WIDTH   the same as SSHU__POWERLINE_WIDTH, for every terminu app
`

func main() {
	// Before anything is drawn: a sshu started by another one over ssh has
	// no COLORTERM, and the depth and the widths wait in the variables that
	// crossed (tdp E5).
	tui.AdoptForwarded()

	if len(os.Args) > 1 && !helperRun() {
		switch os.Args[1] {
		case "version", "--version", "-v":
			fmt.Println("sshu " + version.Display())
			return
		case "help", "--help", "-h":
			fmt.Print(usage)
			return
		default:
			fmt.Fprintf(os.Stderr, "sshu: unknown command %q\n\n%s", os.Args[1], usage)
			os.Exit(2)
		}
	}

	// ssh runs this binary as its SSH_ASKPASS helper. That run prints one
	// password and exits: it never starts the UI, and never prints anything
	// else, because ssh reads the first line as the password.
	if name := tui.AskpassHost(); name != "" {
		os.Exit(runAskpass(name))
	}
	// The same helper for a question sshu does not know the answer to: an
	// sshconfig host's file transfer runs ssh with no terminal, and what ssh
	// asks is relayed to the sshu that is drawing. argv[1] is ssh's prompt.
	if sock := tui.AskpassSock(); sock != "" {
		prompt := ""
		if len(os.Args) > 1 {
			prompt = os.Args[1]
		}
		os.Exit(tui.RunAskpassRelay(sock, prompt))
	}

	d, err := load()
	if err != nil {
		// A malformed hosts.yaml is worth refusing to start over: an empty list
		// would look like data loss, and the next save would overwrite it.
		fmt.Fprintln(os.Stderr, "sshu:", err)
		os.Exit(1)
	}

	// SIGHUP is the terminal window closing: the sessions go with sshu.
	hup := make(chan os.Signal, 1)
	signal.Notify(hup, syscall.SIGHUP)
	go func() {
		<-hup
		tui.KillChildren()
		os.Exit(129)
	}()

	runErr := tui.New(d).Run()
	tui.KillChildren()
	if runErr != nil {
		fmt.Fprintln(os.Stderr, "sshu:", runErr)
		os.Exit(1)
	}
}

// helperRun reports a run as ssh's askpass helper, whose argv[1] is ssh's
// prompt, not a command.
func helperRun() bool { return tui.AskpassHost() != "" || tui.AskpassSock() != "" }

// load reads everything sshu keeps. Only hosts.yaml failing stops it; the
// rest becomes a note, and sshu runs on what it could read.
func load() (*tui.Data, error) {
	hosts, dupHosts, err := store.Load()
	if err != nil {
		return nil, err
	}
	d := &tui.Data{
		SaveHosts:     func(list []store.Host) error { return store.Save(store.File{Hosts: list}) },
		SaveCreds:     func(list []store.Credential) error { return store.SaveCreds(store.CredsFile{Credentials: list}) },
		SaveSSHConfig: store.SaveSSHConfig,
		SaveKnown:     store.SaveKnownHosts,
		AppendError:   store.AppendError, ClearErrors: store.ClearErrors,
		AppendConn: store.AppendConnection, ClearConns: store.ClearConnections,
		AppendChg: store.AppendChange, ClearChgs: store.ClearChanges,
	}
	d.Hosts = hosts.Hosts
	note := func(level, cause string) {
		d.Notes = append(d.Notes, tui.Note{Level: level, Cause: cause, Detail: cause})
	}
	var cfgErr error
	if d.Config, cfgErr = store.LoadConfig(); cfgErr != nil {
		note(store.LevelError, "config.yaml: "+cfgErr.Error())
	}
	// A journal that does not load is news, never a reason to stop recording.
	var jerr error
	if d.Errors, jerr = store.LoadErrors(); jerr != nil {
		note(store.LevelError, "errors.yaml: "+jerr.Error())
	}
	if d.Conns, jerr = store.LoadConnections(); jerr != nil {
		note(store.LevelError, "connections.yaml: "+jerr.Error())
	}
	if d.Changes, jerr = store.LoadChanges(); jerr != nil {
		note(store.LevelError, "changes.yaml: "+jerr.Error())
	}
	creds, dupCreds, credsErr := store.LoadCreds()
	d.Creds = creds.Credentials
	if credsErr != nil {
		note(store.LevelError, "credentials.yaml: "+credsErr.Error())
	}
	var e error
	if d.SSHConfig, e = store.LoadSSHConfig(); e != nil {
		note(store.LevelError, "~/.ssh/config: "+e.Error())
	}
	if d.Known, e = store.LoadKnownHosts(); e != nil {
		note(store.LevelError, "~/.ssh/known_hosts: "+e.Error())
	}
	// The key that seals passwords exists from the first run, not the first
	// save: a key that appears with the first password is one the user has
	// no reason to know about until it is lost.
	if _, err := store.EnsureKey(); err != nil {
		note(store.LevelError, "key file: "+err.Error())
	}
	for _, n := range reconcileFiles(hosts, creds, credsErr) {
		note(store.LevelWarn, n)
	}
	if len(dupHosts) > 0 {
		note(store.LevelWarn, dupeWarning("hosts.yaml", dupHosts))
	}
	if len(dupCreds) > 0 {
		note(store.LevelWarn, dupeWarning("credentials.yaml", dupCreds))
	}
	for _, j := range []struct {
		file  string
		names []string
	}{{"hosts.yaml", hosts.UnreadableSecrets}, {"credentials.yaml", creds.UnreadableSecrets}} {
		for _, name := range j.names {
			cause := j.file + ": this password could not be decrypted"
			d.Notes = append(d.Notes, tui.Note{Level: store.LevelWarn, Host: name, Cause: cause,
				Detail: cause + "\nIt has been left exactly as it is, so nothing is lost. Restoring the key file it " +
					"was sealed with will open it again.\nThe key file is " + keyPathForMessage() + "."})
		}
	}
	return d, nil
}

// keyPathForMessage is the key's path for a message, vaguer when it cannot
// be worked out.
func keyPathForMessage() string {
	p, err := store.KeyPath()
	if err != nil {
		return "in sshu's settings directory"
	}
	return store.FoldHome(p)
}

// reconcileFiles brings sshu's own files up to today's format, once, at
// startup, and reports what it could not settle. Nothing is written for a
// file that did not load: writing back an empty document would delete it.
func reconcileFiles(hosts store.File, creds store.CredsFile, credsErr error) []string {
	var out []string
	if hosts.FromNewerSshu() {
		out = append(out, "hosts.yaml was written by a newer sshu — it is shown as read, "+
			"but saving will be refused. Upgrade sshu.")
	} else if hosts.NeedsUpgrade() || hosts.HadPlaintextSecret {
		if err := store.Save(hosts); err != nil {
			out = append(out, "hosts.yaml could not be rewritten: "+err.Error())
		}
	}
	if credsErr != nil {
		return out
	}
	if creds.FromNewerSshu() {
		out = append(out, "credentials.yaml was written by a newer sshu — it is shown as read, "+
			"but saving will be refused. Upgrade sshu.")
	} else if creds.NeedsUpgrade() || creds.HadPlaintextSecret {
		if err := store.SaveCreds(creds); err != nil {
			out = append(out, "credentials.yaml could not be rewritten: "+err.Error())
		}
	}
	return out
}

// dupeWarning names the entries a load dropped, so the user can go and fix
// them.
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

// runAskpass prints the stored password for name. Exiting non-zero tells ssh
// there is none, and it asks in the terminal instead.
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
