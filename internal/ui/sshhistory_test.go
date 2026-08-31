package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/vulcanshen/sshu/internal/store"
)

func endedSession(name, reason string, ok bool) *session {
	return &session{
		host: store.Host{Name: name}, state: sessEnded,
		reason: reason, ok: ok, ended: time.Unix(1756600000, 0),
	}
}

// The left column is one panel now. [6] could not be acted on and was usually
// empty, and it spent a third of that column permanently.
func TestTabThreeHasTwoPanels(t *testing.T) {
	m := sshApp(t, sample())
	m.ssh.setSize(100, 24)
	m.ssh.setFocus(panelSessions)

	leftW, leftH, _, _ := m.ssh.panes()
	if leftH != m.ssh.h {
		t.Errorf("[4] should take the whole left column: %d of %d", leftH, m.ssh.h)
	}
	if leftW != sshLeftW {
		t.Errorf("left column is %d wide, want %d", leftW, sshLeftW)
	}

	// The digit is gone with the panel: a number the screen does not show is a
	// number the keyboard ignores (§4.4).
	before := m.ssh.focus
	m = pressA(m, "3", "6")
	if m.ssh.focus != before {
		t.Errorf("6 should do nothing in tab [3] now, focus=%d", m.ssh.focus)
	}
	if !strings.Contains(ansi.Strip(m.View()), "1-5 surfaces") {
		t.Error("the footer should offer 1-5, not 1-6")
	}
}

// The information survived the panel: a popup on demand, reached from the menu
// and by its letter.
func TestHistoryPopupListsEndedSessions(t *testing.T) {
	m := sshApp(t, sample())
	m.ssh.setSize(100, 24)
	m.tab = tabSSH
	m.ssh.history = []*session{
		endedSession("prod-web-01", "exited 0", true),
		endedSession("db-replica", "exited 255", false),
	}

	m = pressA(m, "H")
	if !m.historyUI.isActive() {
		t.Fatal("H should open the history")
	}
	got := ansi.Strip(m.View())
	for _, want := range []string{"prod-web-01", "exited 0", "db-replica", "exited 255"} {
		if !strings.Contains(got, want) {
			t.Errorf("the popup does not show %q", want)
		}
	}
	// Name, reason and time on ONE line — the room the panel never had.
	for _, line := range strings.Split(got, "\n") {
		if strings.Contains(line, "prod-web-01") {
			if !strings.Contains(line, "exited 0") {
				t.Errorf("the reason should sit on the name's line: %q", line)
			}
			return
		}
	}
	t.Error("no row for prod-web-01")
}

// The empty case says so rather than opening a blank box (§1.5).
func TestHistoryPopupSaysWhenEmpty(t *testing.T) {
	m := sshApp(t, sample())
	m.ssh.setSize(100, 24)
	m.tab = tabSSH

	m = pressA(m, "H")
	if !strings.Contains(ansi.Strip(m.View()), "nothing has ended yet") {
		t.Error("an empty history should say so")
	}
}

// A session dying used to be completely silent: it left [4], [5] switched away,
// and the only trace was a panel that no longer exists.
func TestABadExitIsAnnounced(t *testing.T) {
	for _, tc := range []struct {
		name  string
		ended []*session
		want  string
	}{
		{"clean exits say nothing",
			[]*session{endedSession("a", "exited 0", true)}, ""},
		{"one failure names it",
			[]*session{endedSession("prod-web-01", "exited 255", false)},
			"prod-web-01 · exited 255"},
		{"several point at the history",
			[]*session{
				endedSession("a", "exited 1", false),
				endedSession("b", "signal: killed", false),
			}, "2 sessions ended badly · press H for history"},
		{"a clean one among failures does not count",
			[]*session{
				endedSession("a", "exited 0", true),
				endedSession("b", "exited 1", false),
			}, "b · exited 1"},
	} {
		if got := endedBadly(tc.ended); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, got, tc.want)
		}
	}
}

// End to end: a real subprocess exits non-zero and the app says so.
func TestAFailedSessionRaisesAToast(t *testing.T) {
	fakeSSH(t, "exit 7")
	m := pressA(sshApp(t, sample()), "enter", "enter")
	s := m.ssh.sessions[0]
	waitFor(t, "the subprocess to exit", func() bool { return s.pty.exited() })

	next, _ := m.Update(sshTickMsg{})
	m = settle(next.(AppModel))

	if !m.toast.isActive() {
		t.Fatal("a session that exited 7 should have said so")
	}
	if m.toast.kind != toastError {
		t.Errorf("the toast should be an error, kind=%d", m.toast.kind)
	}
	if !strings.Contains(m.toast.msg, "exit") {
		t.Errorf("the toast should carry the reason, got %q", m.toast.msg)
	}
}

// History is reachable from the Space menu too, in the panel region — it is
// about the tab, not about the session under the cursor.
func TestHistoryIsAPanelActionInTheMenu(t *testing.T) {
	m := openOne(t)
	next, _ := m.Update(keyMsg("alt+esc"))
	m = settle(next.(AppModel))
	m.ssh.setFocus(panelSessions)

	items := m.sshMenuItems()
	region := ""
	for _, it := range items {
		if it.header {
			region = it.label
		}
		if it.key == "H" {
			if region != "panel" {
				t.Errorf("History is in the %q region, want panel", region)
			}
			return
		}
	}
	t.Error("the menu does not offer History")
}
