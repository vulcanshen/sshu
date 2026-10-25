package tui

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/vulcanshen/terminu-go/tdptest"

	"github.com/vulcanshen/sshu/internal/store"
)

func screenOf(d *tdptest.Driver) string { return strings.Join(d.Screen(), "\n") }

// What startup found wrong goes to the App Log, each in full on one line,
// and one toast says where (tdp F5).
func TestStartupProblemsGoToTheAppLog(t *testing.T) {
	d := testData(nil, nil)
	d.Notes = []Note{{Level: store.LevelWarn, Cause: `Can't decrypt the password of "web" in hosts.yaml`,
		Detail: "Can't decrypt the password of \"web\" in hosts.yaml\nIt has been left as it is."}}
	dr := tdptest.Start(t, New(d).a, 120, 30)
	if !dr.Has("1 problem at startup: see App Log [!]") {
		t.Errorf("no toast pointing at the App Log:\n%s", screenOf(dr))
	}
	dr.Press("!")
	if !dr.Has(`WARN Can't decrypt the password of "web" in hosts.yaml It has been left as it is.`) {
		t.Errorf("the problem is not in the App Log:\n%s", screenOf(dr))
	}
}

// What startup moved is in the App Log, item by item, capitalised like a
// toast (tdp E3, F5; dialog/note, App Log).
func TestWhatMovedIsInTheAppLog(t *testing.T) {
	d := testData(nil, nil)
	d.Moves = []store.Move{{What: "data", From: "/old", To: "/new", Moved: []string{"hosts.yaml", ".sshukey"}}}
	dr := tdptest.Start(t, New(d).a, 120, 30)
	dr.Press("!")
	if !dr.Has("INFO Moved hosts.yaml, .sshukey from /old to /new") {
		t.Errorf("the move is not in the App Log:\n%s", screenOf(dr))
	}
}

// A failure goes to the App Log with what the far end said, and nowhere else:
// there is no Errors journal. The footer counts it, the statusbar leaves it
// to the footer. A change goes to the Changes journal and the App Log. Each
// entry starts with a capital (dialog/note, App Log).
func TestAFailureIsInTheAppLog(t *testing.T) {
	d := testData(nil, nil)
	s := New(d)
	dr := tdptest.Start(t, s.a, 160, 30)
	dr.Do(func() tea.Cmd {
		s.logError(store.LevelError, "Session to root@web failed: connection refused", "ssh: connect to host web port 22:\nConnection refused")
		s.logError(store.LevelError, "can't read /srv on local", "open /srv: permission denied")
		s.logChange(`host "web" added`)
		return nil
	})
	if len(d.Changes) != 1 {
		t.Fatalf("changes: %d", len(d.Changes))
	}
	if dr.Has("errors:") || !dr.Has("!:2 errors") {
		t.Errorf("want the footer to count the errors, not the statusbar:\n%s", screenOf(dr))
	}
	if dr.Has("Errors") {
		t.Errorf("an Errors section is still there:\n%s", screenOf(dr))
	}
	dr.Press("!")
	for _, want := range []string{
		"ERR  Session to root@web failed: connection refused · ssh: connect to host web port 22",
		"ERR  Can't read /srv on local · open /srv: permission denied",
		`INFO Host "web" added`,
	} {
		if !dr.Has(want) {
			t.Errorf("no %q in the App Log:\n%s", want, screenOf(dr))
		}
	}
}

// ~/.ssh is not sshu's: a file of it that would not read is said in its
// section, why in Red, not the empty list's words (tdp E3, layout/panel).
func TestAnUnreadableSSHConfigIsSaidInItsSection(t *testing.T) {
	d := testData(nil, nil)
	d.SSHConfigErr = errors.New("permission denied")
	d.KnownErr = errors.New("is a directory")
	dr := tdptest.Start(t, New(d).a, 120, 30)
	dr.Press("1", "j", "j")
	if !dr.Has("Can't read ~/.ssh/config: permission denied") || dr.Has("No Host blocks") {
		t.Errorf("the Config section does not say why:\n%s", screenOf(dr))
	}
	dr.Press("j")
	if !dr.Has("Can't read ~/.ssh/known_hosts: is a directory") || dr.Has("No host keys") {
		t.Errorf("the KnownHosts section does not say why:\n%s", screenOf(dr))
	}
	dr.Press("j")
	if dr.Has("Can't read") {
		t.Errorf("the failure stayed on the next section:\n%s", screenOf(dr))
	}
}

// Every value sshu draws from outside — a host's name, a cause, a result —
// may hold control characters: the lib draws them by name, nothing in a cell
// reaches the terminal as a sequence (components, control characters). And
// sshu's own colours are styles, never codes in a cell: a plain value shows
// no escape anywhere.
func TestTrickyValuesAreDrawnByName(t *testing.T) {
	for _, v := range append([]string{"web"}, tdptest.Tricky...) {
		d := testData([]store.Host{{Name: v, Host: v, User: v, Port: 22, Auth: store.AuthPassword}}, nil)
		d.Conns = []store.ConnectionEntry{{Host: v, User: v, Result: store.ResultFail}, {Host: v, Result: store.ResultSuccess}}
		d.Changes = []store.ChangeEntry{{Action: v}}
		dr := tdptest.Start(t, New(d).a, 100, 30)
		dr.AllowLineBreaks = v != "web" // a tricky name may hold one, drawn by its name
		dr.Note("value " + strings.ReplaceAll(v, "\x1b", `\x1b`))
		dr.Press("1") // the sections, to walk them
		for range 6 {
			dr.Press("j")
			if v == "web" && dr.Has(`\x1b`) {
				t.Fatalf("an escape on the screen:\n%s", screenOf(dr))
			}
		}
	}
}
