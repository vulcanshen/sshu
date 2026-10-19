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

// icon is the mark the splash draws (tdp S5, E6), a copy of docs/icon.svg,
// which go:embed cannot reach; TestTheIconIsTheSplash keeps them the same.
//
//go:embed icon.svg
var icon []byte

const intro = "An ssh and sftp front end for the terminal"

// New builds sshu over d.
func New(d *Data) *sshu {
	if d.Now == nil {
		d.Now = time.Now
	}
	s := &sshu{d: d}
	a := terminu.New("sshu")
	a.QuitDesc = "leave sshu"
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
		Scope: terminu.GlobalScope, Stay: true, Hidden: func() bool { return len(s.ft.jobs) == 0 }, Do: s.ft.jobsMenu}}
	a.Start = s.start
	s.refreshQuit()
	return s
}

// Run runs sshu until it quits.
func (s *sshu) Run() error { return s.a.Run() }

// start records what startup found and says so once.
func (s *sshu) start() tea.Cmd {
	cmds := []tea.Cmd{s.ft.start(), s.sh.start()}
	// What moved says from where to where in a toast; what could not, in an
	// error popup with both paths, for the user to move (tdp E3).
	var moved, stuck []string
	for _, m := range s.d.Moves {
		from, to := store.FoldHome(m.From), store.FoldHome(m.To)
		if m.Err != nil {
			stuck = append(stuck, fmt.Sprintf("sshu could not move its %s from %s to %s: %v.\nIt goes on using %s; move them to %s yourself.",
				m.What, from, to, m.Err, from, to))
			continue
		}
		moved = append(moved, fmt.Sprintf("the %s from %s to %s", m.What, from, to))
	}
	if len(stuck) > 0 {
		cmds = append(cmds, s.sized(func() tea.Cmd { return s.a.Error("Move failed", strings.Join(stuck, "\n\n")) }))
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
// live sessions in Green, a transfer in flight in Green, unread errors in
// Red. Each part keeps its width (tdp L2).
func (s *sshu) status(w int) string {
	grey := terminu.Fg(terminu.Overlay0)
	var parts []string
	if n := s.mg.unreadErrors(); n > 0 {
		noun := " errors"
		if n == 1 {
			noun = " error "
		}
		parts = append(parts, terminu.Fg(terminu.Red).Render(fmt.Sprintf("%3d", n))+grey.Render(noun))
	}
	if t := s.ft.statusText(); t != "" {
		parts = append(parts, terminu.Fg(terminu.Green).Render(t))
	}
	n := s.sh.liveCount()
	parts = append(parts, terminu.Fg(terminu.Green).Render(fmt.Sprintf("%2d", n))+grey.Render(" live"))
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

// whenSized is a popup to open once the terminal's size is known.
type whenSized struct{ open func() tea.Cmd }

// sized opens a popup once the size is known: one opened before, at
// startup, wraps its text to a width of nothing.
// TODO(lib SS37): an error popup wraps when it is drawn, not when it opens.
func (s *sshu) sized(open func() tea.Cmd) tea.Cmd {
	return func() tea.Msg { return whenSized{open} }
}

// onMsg routes the messages the lib leaves to the screen that sent them.
func (s *sshu) onMsg(msg tea.Msg) tea.Cmd {
	if m, ok := msg.(copied); ok {
		return s.onCopied(m)
	}
	if m, ok := msg.(whenSized); ok {
		if w, _ := s.a.Size(); w == 0 {
			return tea.Tick(250*time.Millisecond, func(time.Time) tea.Msg { return m })
		}
		return m.open()
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

// liveAsk is an action's confirm, filled in for the item under the cursor.
// The lib reads Action.Ask when the action runs and asks Hidden just
// before, so Hidden fills it; with Ask set, Esc on the confirm goes back to
// the Space menu it was run from, and Enter closes them all (tdp F4, T1).
// TODO(lib SS29): an Ask that is made when the action runs.
func liveAsk(fill func(*terminu.Ask) bool) (*terminu.Ask, func() bool) {
	ask := &terminu.Ask{}
	return ask, func() bool {
		*ask = terminu.Ask{}
		return !fill(ask)
	}
}

// gated is act with a confirm in front of something that leaves its source
// with nothing to act on — a delete, a close (tdp T1: the confirm is only a
// gate, and what comes after it clears the source). ask gives the confirm for
// the item under the cursor and what Enter on it does; it becomes the
// action's own Ask, so Enter closes the Space menu as well, and Esc goes back
// to it.
func gated(act terminu.Action, ask func() (terminu.Ask, func() tea.Cmd, bool)) terminu.Action {
	var run func() tea.Cmd
	q, none := liveAsk(func(a *terminu.Ask) bool {
		x, do, ok := ask()
		if ok {
			*a, run = x, do
		}
		return ok
	})
	hidden := act.Hidden
	act.Ask, act.Stay = q, false
	act.Hidden = func() bool { return hidden != nil && hidden() || none() }
	act.Do = func() tea.Cmd {
		if run == nil {
			return nil
		}
		return run()
	}
	return act
}

// plural is "1 host", "2 hosts".
func plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return strconv.Itoa(n) + " " + noun + "s"
}
