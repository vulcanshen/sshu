// Package tui is sshu's terminal UI, built on terminu-go: the Manage, File
// transfer and SSH screens over the store and remote packages. terminu-go
// draws every screen and popup and owns the core keys; this package registers
// the panels, the actions and what they do (tdp E8).
package tui

import (
	_ "embed"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

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

	Conns      []store.ConnectionEntry
	AppendConn func(store.ConnectionEntry) error
	ClearConns func() error
	Changes    []store.ChangeEntry
	AppendChg  func(store.ChangeEntry) error
	ClearChgs  func() error

	// Notes are what startup found wrong with the files. Each goes to the
	// App Log, and a toast says there are some.
	Notes []Note

	// Unreadable are sshu's own files startup could not read: before anything
	// else, sshu asks to move each aside (tdp E3, when it cannot be read).
	Unreadable []Unreadable

	// SSHConfigErr and KnownErr are why ~/.ssh/config and ~/.ssh/known_hosts
	// could not be read: not sshu's own, so their sections say so instead
	// (tdp E3, panel.15).
	SSHConfigErr, KnownErr error

	// Moves are what startup moved to where tdp E3 puts it now, or could not.
	Moves []store.Move

	// NoLongerUsed are where files of an older sshu would be that this one
	// does not read: each that is there is named in the App Log at every
	// start, never deleted (tdp E3, files no longer used).
	NoLongerUsed []string

	// Now is the clock, for the journals' times; time.Now when nil.
	Now func() time.Time
}

// Note is one thing startup has to say: a file that would not load, a
// duplicate it dropped, a password it could not open. Each starts with a
// verb, never a name, as an App Log entry does.
type Note struct {
	Level  string // "warn" or "error"
	Cause  string
	Detail string // the whole of it, Cause included
}

// Unreadable is a file of sshu's own that could not be read, and why.
type Unreadable struct {
	Path string
	Err  error
	// Fresh makes a new one where it was, once it has moved aside, as a first
	// start would (tdp E3-18); nil when a first start does not make one
	// either, and it appears at the first save.
	Fresh func() error
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

// App is sshu's terminu App, for a test to drive (tdptest).
func (s *sshu) App() *terminu.App { return s.a }

// start asks about the files sshu could not read, then starts.
func (s *sshu) start() tea.Cmd { return s.moveAside(0) }

// moveAside asks about the i-th file sshu could not read, each after the one
// before, and starts sshu after the last; Esc on any leaves it (tdp E3).
// Nothing else runs meanwhile: no grid restored, nothing saved over a file
// that is still to be asked about.
func (s *sshu) moveAside(i int) tea.Cmd {
	if i == len(s.d.Unreadable) {
		return s.started()
	}
	u := s.d.Unreadable[i]
	return s.a.MoveAside(u.Path, u.Err, func() tea.Cmd {
		var failed tea.Cmd
		if u.Fresh != nil {
			if err := u.Fresh(); err != nil {
				failed = s.a.ErrorAbout("Start a new one failed", filepath.Base(u.Path), err.Error())
			}
		}
		return tea.Batch(failed, s.moveAside(i+1))
	})
}

// started records what startup found and says so once. sshu opens with the
// keyboard on the hosts table, its content, so A adds the first host.
func (s *sshu) started() tea.Cmd {
	cmds := []tea.Cmd{s.a.Focus(s.mg.content), s.ft.start(), s.sh.start()}
	// What moved says from where to where in a toast; what could not, or
	// what the new place already had, in an error popup with both paths, for
	// the user to move or merge (tdp E3). Its message comes a sentence a row:
	// the app's own text carries no line break (tdp
	// components.5).
	var moved, stuck []string
	for _, m := range s.d.Moves {
		from, to := store.FoldHome(m.From), store.FoldHome(m.To)
		switch {
		case m.Err != nil:
			stuck = append(stuck, fmt.Sprintf("sshu could not move its %s from %s to %s: %v.", m.What, from, to, m.Err),
				fmt.Sprintf("It goes on using %s; move them to %s yourself.", from, to))
		case len(m.Left)+len(m.Kept) > 0:
			stuck = append(stuck, notMoved(m, from, to)...)
		}
		if len(m.Moved) > 0 {
			moved = append(moved, fmt.Sprintf("the %s from %s to %s", m.What, from, to))
			s.a.Log(terminu.Info, fmt.Sprintf("Moved %s from %s to %s", strings.Join(m.Moved, ", "), from, to))
		}
	}
	s.a.NoLongerUsed(s.d.NoLongerUsed...)
	if len(stuck) > 0 {
		cmds = append(cmds, s.a.Error("Not moved", stuck...))
	}
	var says []string
	kind := terminu.Info
	if len(moved) > 0 {
		says = append(says, "Moved "+strings.Join(moved, ", and "))
	}
	if len(s.d.Notes) > 0 {
		kind = terminu.Warning
		for _, n := range s.d.Notes {
			s.a.Log(logKind(n.Level), oneLine(n.Detail))
			if n.Level != store.LevelWarn {
				kind = terminu.Error
			}
		}
		says = append(says, plural(len(s.d.Notes), "problem")+" at startup: see App Log [!]")
	}
	// One toast: a second would replace the first.
	if len(says) > 0 {
		cmds = append(cmds, s.a.Toast(kind, says...))
	}
	return tea.Batch(cmds...)
}

// notMoved says what a move left in from, and why, a sentence a row: what to
// already had, and what stays there with the key, which moves only with the
// files it seals (tdp E3).
func notMoved(m store.Move, from, to string) []string {
	var rows []string
	if len(m.Left) > 0 {
		rows = append(rows, fmt.Sprintf("%s already has %s, so sshu left %s in %s.", to, strings.Join(m.Left, ", "), pronoun(len(m.Left)), from))
		if len(m.Kept) > 0 {
			verb := "stay"
			if len(m.Kept) == 1 {
				verb = "stays"
			}
			rows = append(rows, fmt.Sprintf("%s %s there too: the key and the files it seals move together.", strings.Join(m.Kept, ", "), verb))
		}
	} else {
		rows = append(rows, fmt.Sprintf("%s already has some of the key and the files it seals, so sshu left %s in %s: they move together.",
			to, strings.Join(m.Kept, ", "), from))
	}
	return append(rows, fmt.Sprintf("Merge %s into %s yourself.", pronoun(len(m.Left)+len(m.Kept)), to))
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

// status is the statusbar at the right of the screen chips (tdp screen.4):
// each part "label: value", a transfer in flight and the live sessions in
// Green. It stands against the right edge, so a part that grows pushes the
// ones on its left: every part but the leftmost keeps its width (tdp L2).
// Errors not seen yet are the footer's (!:N errors).
func (s *sshu) status(int) []terminu.Span {
	grey, green := terminu.Fg(terminu.Overlay0), terminu.Fg(terminu.Green)
	var spans []terminu.Span
	if t := s.ft.statusText(); t != "" {
		spans = append(spans, terminu.Span{Text: "transfer: ", Style: grey}, terminu.Span{Text: t, Style: green},
			terminu.Span{Text: "  "})
	}
	return append(spans, terminu.Span{Text: "live: ", Style: grey},
		terminu.Span{Text: fmt.Sprintf("%-2d", s.sh.liveCount()), Style: green})
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

// logError records a failure in the App Log (tdp F5): line says what failed
// and where, a verb or a noun first and never a name, so it starts with a
// capital as a toast does (tdp note.7); detail, when there is
// more, is what the far end said. The toast or the error popup next to it
// is in the App Log already, in its own words.
func (s *sshu) logError(level, line, detail string) {
	line = capital(line)
	if detail != "" {
		line += " · " + oneLine(detail)
	}
	s.a.Log(logKind(level), line)
}

// capital is s with its first letter a capital, as a toast and an App Log
// entry start (tdp note.7): for text that starts with words, such
// as a Go error's.
func capital(s string) string {
	r, n := utf8.DecodeRuneInString(s)
	if !unicode.IsLower(r) {
		return s
	}
	return string(unicode.ToUpper(r)) + s[n:]
}

// logKind is a journal level's kind in the App Log.
func logKind(level string) terminu.ToastKind {
	if level == store.LevelWarn {
		return terminu.Warning
	}
	return terminu.Error
}

// oneLine is text of several lines as one App Log entry.
func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

// logChange records something the user changed through sshu, in the Changes
// journal and the App Log. Each begins with words or a path (~/.ssh/config),
// never a name, so the App Log's capital is safe.
func (s *sshu) logChange(action string) {
	e := store.ChangeEntry{At: s.d.Now(), Action: action}
	s.d.Changes = append(s.d.Changes, e)
	if s.d.AppendChg != nil {
		_ = s.d.AppendChg(e)
	}
	s.mg.journalsChanged()
	s.a.Log(terminu.Info, capital(action))
}

// logOp records an operation the user ran whose result says nothing of
// its own: one that only looks (a viewer, a directory, a section) or
// changes only what is on the screen (a mark, a hidden cell). The others
// are in the App Log already, by their toast, their error popup or
// logChange (tdp note.7). what starts with a verb, a capital.
func (s *sshu) logOp(what string) { s.a.Log(terminu.Info, what) }

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
