// Package tui is sshu's terminal UI, built on terminu-go: the Manage, File
// transfer and SSH screens over the store and remote packages. terminu-go
// draws every screen and popup and owns the core keys; this package registers
// the panels, the actions and what they do (tdp E8).
package tui

import (
	_ "embed"
	"fmt"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	terminu "github.com/vulcanshen/terminu-go"

	"github.com/vulcanshen/sshu/internal/store"
	"github.com/vulcanshen/sshu/internal/version"
)

// Data is what sshu keeps and how each part is written back. main loads it;
// the UI changes it and calls the savers.
type Data struct {
	Hosts     []store.Host
	SaveHosts func([]store.Host) error
	Creds     []store.Credential
	SaveCreds func([]store.Credential) error

	SSHConfig     store.SSHConfigFile
	SaveSSHConfig func(store.SSHConfigFile) (store.SSHConfigFile, error)
	Known         store.KnownHostsFile
	SaveKnown     func(store.KnownHostsFile) (store.KnownHostsFile, error)

	Config store.Config

	// State is what the UI keeps between runs (the SSH grid's arrangement).
	State     store.State
	SaveState func(store.State) error

	Errors      []store.ErrorEntry
	AppendError func(store.ErrorEntry) error
	ClearErrors func() error
	Conns       []store.ConnectionEntry
	AppendConn  func(store.ConnectionEntry) error
	ClearConns  func() error
	Changes     []store.ChangeEntry
	AppendChg   func(store.ChangeEntry) error
	ClearChgs   func() error

	// Notes are what startup found wrong with the files. Each goes to the
	// Errors journal, and a toast says there are some.
	Notes []Note

	// Moves are what startup moved to where tdp E3 puts it now, or could not.
	Moves []store.Move

	// Now is the clock, for the journals' times; time.Now when nil.
	Now func() time.Time
}

// Note is one thing startup has to say: a file that would not load, a
// duplicate it dropped, a password it could not open.
type Note struct {
	Level       string // "warn" or "error"
	Host, Cause string
	Detail      string // the whole of it, Cause included
}

// sshu is the app: the three screens over the data.
type sshu struct {
	a  *terminu.App
	d  *Data
	mg *manage
	ft *transfer
	sh *sshScreen
}

// icon is the mark the splash draws (tdp S5, tdp E6), a copy of docs/icon.svg,
// which go:embed cannot reach; TestTheIconIsTheSplash keeps them the same.
//
//go:embed icon.svg
var icon []byte

const intro = "An ssh and sftp front end for the terminal"

// The least size sshu draws itself at (tdp L1): a sshu nested in a zoomed
// cell of an 80×24 terminal has 78×20, and every screen and form still works
// at 60 columns.
const minWidth, minHeight = 60, 20

// New builds sshu over d.
func New(d *Data) *sshu {
	if d.Now == nil {
		d.Now = time.Now
	}
	s := &sshu{d: d}
	a := terminu.New("sshu")
	a.QuitDesc = "leave sshu"
	// sshu's least size (tdp L1): see dev-remarks, design decisions.
	a.MinWidth, a.MinHeight = minWidth, minHeight
	splash, err := terminu.NewSplash(icon, "sshu", version.Display(), intro)
	if err != nil {
		panic(err) // the embedded icon is broken: a mistake in the program
	}
	a.Splash = splash
	a.HideSecretsInPreview = d.Config.HideSecretsInPreview
	a.InputTabWidth = d.Config.TabWidth()
	a.Status = s.status
	a.Progress = s.progress
	a.OnMsg = s.onMsg
	s.a = a
	s.mg = newManage(s)
	s.ft = newTransfer(s)
	s.sh = newSSHScreen(s)
	a.AddScreen(s.mg.screen)
	a.AddScreen(s.ft.screen)
	a.AddScreen(s.sh.screen)
	// The transfers are the whole app's, whichever side started them.
	a.Global = []terminu.Action{{Key: "J", Label: "Jobs", Desc: "the transfers of this run, and cancelling one",
		Scope: terminu.GlobalScope, Hidden: func() bool { return len(s.ft.jobs) == 0 }, Do: s.ft.jobsMenu}}
	a.Start = s.start
	s.refreshQuit()
	return s
}

// Run runs sshu until it quits.
func (s *sshu) Run() error { return s.a.Run() }

// start records what startup found and says so once.
func (s *sshu) start() tea.Cmd {
	cmds := []tea.Cmd{s.ft.start(), s.sh.start()}
	// What moved says from where to where in a toast; what could not, or
	// what the new place already had, in an error popup with both paths, for
	// the user to move or merge (tdp E3).
	var moved, stuck []string
	for _, m := range s.d.Moves {
		from, to := store.FoldHome(m.From), store.FoldHome(m.To)
		switch {
		case m.Err != nil:
			stuck = append(stuck, fmt.Sprintf("sshu could not move its %s from %s to %s: %v.\nIt goes on using %s; move them to %s yourself.",
				m.What, from, to, m.Err, from, to))
		case len(m.Left) > 0:
			stuck = append(stuck, fmt.Sprintf("%s already has %s, so sshu left %s in %s.\nMerge %s into %s yourself.",
				to, strings.Join(m.Left, ", "), pronoun(len(m.Left)), from, pronoun(len(m.Left)), to))
		}
		if len(m.Moved) > 0 {
			moved = append(moved, fmt.Sprintf("the %s from %s to %s", m.What, from, to))
		}
	}
	if len(stuck) > 0 {
		cmds = append(cmds, s.a.Error("Not moved", strings.Join(stuck, "\n\n")))
	}
	var says []string
	kind := terminu.Info
	if len(moved) > 0 {
		says = append(says, "Moved "+strings.Join(moved, ", and "))
	}
	if len(s.d.Notes) > 0 {
		errors := 0
		for _, n := range s.d.Notes {
			s.logError(n.Level, n.Host, "", n.Cause, n.Detail)
			if n.Level != "warn" {
				errors++
			}
		}
		kind = terminu.Warning
		if errors > 0 {
			kind = terminu.Error
		}
		says = append(says, plural(len(s.d.Notes), "problem")+" at startup: see Manage → Errors")
	}
	// One toast: a second would replace the first.
	if len(says) > 0 {
		cmds = append(cmds, s.a.Toast(kind, strings.Join(says, ". ")))
	}
	return tea.Batch(cmds...)
}

// refreshQuit decides whether quitting asks first (tdp K9: the quit flow
// is the app's): live sessions and transfers in flight would be lost, so
// with either it asks and says which.
func (s *sshu) refreshQuit() {
	var details []string
	if n := s.sh.liveCount(); n > 0 {
		details = append(details, plural(n, "live session")+" will be closed.")
	}
	if n := s.ft.running(); n > 0 {
		details = append(details, plural(n, "transfer")+" in flight will be cancelled.")
	}
	if len(details) == 0 {
		s.a.Quit = nil
		return
	}
	s.a.Quit = &terminu.Ask{Title: "Quit", Question: "Quit sshu?", Verb: "quit",
		Details: details, Warn: []int{0, 1}[:len(details)]}
}

// status is the statusbar at the right of the screen chips (layout/screen):
// each part "label: value", unread errors in Red, a transfer in flight and
// the live sessions in Green. It stands against the right edge, so a part
// that grows pushes the ones on its left: every part but the leftmost keeps
// its width (tdp L2).
func (s *sshu) status(w int) string {
	grey := terminu.Fg(terminu.Overlay0)
	var parts []string
	if n := s.mg.unreadErrors(); n > 0 {
		parts = append(parts, grey.Render("errors: ")+terminu.Fg(terminu.Red).Render(strconv.Itoa(n)))
	}
	if t := s.ft.statusText(); t != "" {
		parts = append(parts, grey.Render("transfer: ")+terminu.Fg(terminu.Green).Render(t))
	}
	n := s.sh.liveCount()
	parts = append(parts, grey.Render("live: ")+terminu.Fg(terminu.Green).Render(fmt.Sprintf("%-2d", n)))
	text := ""
	for i, p := range parts {
		if i > 0 {
			text += "  "
		}
		text += p
	}
	return terminu.PadLeft(terminu.Truncate(text, w), w)
}

func (s *sshu) progress() (float64, bool) { return s.ft.progress() }

// onMsg routes the messages the lib leaves to the screen that sent them.
func (s *sshu) onMsg(msg tea.Msg) tea.Cmd {
	if m, ok := msg.(copied); ok {
		return s.onCopied(m)
	}
	if cmd, ok := s.ft.onMsg(msg); ok {
		return cmd
	}
	if cmd, ok := s.sh.onMsg(msg); ok {
		return cmd
	}
	return nil
}

// logError records a failure in the Errors journal, on screen and on disk.
// A journal that cannot be written is not itself journalled: it would fail
// the same way.
func (s *sshu) logError(level, host, user, cause, detail string) {
	e := store.ErrorEntry{At: s.d.Now(), Host: host, User: user, Level: level, Cause: cause, Error: detail}
	if e.Error == "" {
		e.Error = cause
	}
	s.d.Errors = append(s.d.Errors, e)
	if s.d.AppendError != nil {
		_ = s.d.AppendError(e)
	}
	s.mg.journalsChanged()
}

// logChange records something the user changed through sshu.
func (s *sshu) logChange(action string) {
	e := store.ChangeEntry{At: s.d.Now(), Action: action}
	s.d.Changes = append(s.d.Changes, e)
	if s.d.AppendChg != nil {
		_ = s.d.AppendChg(e)
	}
	s.mg.journalsChanged()
}

// logConn records one connection attempt and how it ended.
func (s *sshu) logConn(e store.ConnectionEntry) {
	e.At = s.d.Now()
	s.d.Conns = append(s.d.Conns, e)
	if s.d.AppendConn != nil {
		_ = s.d.AppendConn(e)
	}
	s.mg.journalsChanged()
}

// gated is act with a confirm in front of something that leaves its source
// with nothing to act on — a delete, a close. ask gives, when the action
// runs, the confirm for the item under the cursor and what Enter on it does
// (Action.AskFor). The confirm is only a door (tdp T1): Esc on it goes back
// to the Space menu, and what follows opens nothing, so the menu closes.
func gated(act terminu.Action, ask func() (terminu.Ask, func() tea.Cmd, bool)) terminu.Action {
	return asked(act, func() (*terminu.Ask, func() tea.Cmd) {
		x, do, ok := ask()
		if !ok {
			return nil, nil
		}
		return &x, do
	})
}

// asked is act whose question, if it has one, is worked out when it runs:
// ask gives the question, nil to ask nothing, and what the action then does
// (Action.AskFor). Asking or not, the action ends the same way (tdp T1: a
// confirm is only a door).
func asked(act terminu.Action, ask func() (*terminu.Ask, func() tea.Cmd)) terminu.Action {
	var run func() tea.Cmd
	act.AskFor = func() *terminu.Ask {
		q, do := ask()
		run = do
		return q
	}
	act.Do = func() tea.Cmd {
		if run == nil {
			return nil
		}
		return run()
	}
	return act
}

// pronoun is "it" for one thing, "them" for more.
func pronoun(n int) string {
	if n == 1 {
		return "it"
	}
	return "them"
}

// plural is "1 host", "2 hosts".
func plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return strconv.Itoa(n) + " " + noun + "s"
}
