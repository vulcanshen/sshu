// Package tui is sshu's terminal UI, built on terminu-go: the Manage, File
// transfer and SSH screens over the store and remote packages. terminu-go
// draws every screen and popup and owns the core keys; this package registers
// the panels, the actions and what they do (tdp E8).
package tui

import (
	_ "embed"
	"fmt"
	"strconv"
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
	a.Start = s.start
	s.refreshQuit()
	return s
}

// Run runs sshu until it quits.
func (s *sshu) Run() error { return s.a.Run() }

// start records what startup found and says so once.
func (s *sshu) start() tea.Cmd {
	cmds := []tea.Cmd{s.ft.start(), s.sh.start()}
	if len(s.d.Notes) == 0 {
		return tea.Batch(cmds...)
	}
	errors := 0
	for _, n := range s.d.Notes {
		s.logError(n.Level, n.Host, "", n.Cause, n.Detail)
		if n.Level != "warn" {
			errors++
		}
	}
	kind := terminu.Warning
	if errors > 0 {
		kind = terminu.Error
	}
	text := plural(len(s.d.Notes), "problem") + " at startup: see Manage → Errors"
	return tea.Batch(append(cmds, s.a.Toast(kind, text))...)
}

// refreshQuit decides whether quitting asks first (Rules K9: the quit flow
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
// Red. Each part keeps its width (Rules L2).
func (s *sshu) status(w int) string {
	grey := terminu.Fg(terminu.Overlay0)
	var parts []string
	if n := s.mg.unreadErrors(); n > 0 {
		parts = append(parts, terminu.Fg(terminu.Red).Render(fmt.Sprintf("%3d", n))+grey.Render(" errors"))
	}
	if t := s.ft.statusText(); t != "" {
		parts = append(parts, terminu.Fg(terminu.Green).Render(t))
	}
	n := s.sh.liveCount()
	parts = append(parts, terminu.Fg(terminu.Green).Render(fmt.Sprintf("%2d", n))+grey.Render(" live"))
	text := ""
	for i, p := range parts {
		if i > 0 {
			text += grey.Render("  ·  ")
		}
		text += p
	}
	return terminu.PadLeft(terminu.Truncate(text, w), w)
}

func (s *sshu) progress() (float64, bool) { return s.ft.progress() }

// onMsg routes the messages the lib leaves to the screen that sent them.
func (s *sshu) onMsg(msg tea.Msg) tea.Cmd {
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

// plural is "1 host", "2 hosts".
func plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return strconv.Itoa(n) + " " + noun + "s"
}
