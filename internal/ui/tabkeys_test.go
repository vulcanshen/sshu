package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// The tabs are the app's top-level structure, so they are reached with one
// shifted letter and no modifier.
func TestBareLettersSwitchTabs(t *testing.T) {
	m := appWith(sample(), nil)
	m = pressA(m, "F")
	if m.tab != tabFT {
		t.Fatalf("F should reach the file transfer tab, tab=%d", m.tab)
	}
	m = pressA(m, "S")
	if m.tab != tabSSH {
		t.Fatalf("S should reach the ssh tab, tab=%d", m.tab)
	}
	m = pressA(m, "M")
	if m.tab != tabPref {
		t.Fatalf("M should reach the manage tab, tab=%d", m.tab)
	}
}

// Lower case belongs to the panels — the same exact-case rule hotkeyIndex keeps
// everywhere else, so a tab key cannot swallow a row action's letter.
func TestLowerCaseIsNotATabKey(t *testing.T) {
	m := appWith(sample(), nil)
	for _, k := range []string{"m", "f", "s"} {
		if got := pressA(m, k); got.tab != tabPref {
			t.Errorf("%q must not switch tabs, landed on tab=%d", k, got.tab)
		}
	}
}

// The chords are gone. Alt+F used to reach the file transfer tab from anywhere,
// including through a remote; keeping it as a second spelling would leave the
// app with two vocabularies for one move.
func TestTheAltChordsAreGone(t *testing.T) {
	m := appWith(sample(), nil)
	for _, k := range []string{"alt+F", "alt+f", "alt+S", "alt+s"} {
		if got := pressA(m, k); got.tab != tabPref {
			t.Errorf("%q must no longer switch tabs, landed on tab=%d", k, got.tab)
		}
	}
}

// Inside a pty every bare key is the far end's, tab letters included. This is
// what the Alt chords used to buy, and it is deliberately not bought any more:
// Alt+Esc comes out first, and it is the one key the footer keeps advertising.
func TestTabKeysBelongToTheRemote(t *testing.T) {
	m := openOne(t)
	if !m.inPty() {
		t.Fatal("setup: the remote should hold the keyboard")
	}
	s := m.ssh.currentSession()

	m = pressA(m, "F")
	if m.tab != tabSSH {
		t.Fatalf("F inside a pty belongs to the remote, tab=%d", m.tab)
	}
	waitFor(t, "the letter to be echoed back", func() bool {
		return strings.Contains(strings.Join(s.pty.render(80, 24), ""), "F")
	})

	// And Alt+Esc still gets the keyboard back, after which it switches tabs.
	m = pressA(m, "alt+esc", "F")
	if m.tab != tabFT {
		t.Fatalf("F should switch tabs once the keyboard is back, tab=%d", m.tab)
	}
}

// Under a popup they do nothing: a tab switching beneath a form would strand
// the form over a surface it knows nothing about.
func TestTabKeysAreInertUnderAPopup(t *testing.T) {
	m := pressA(appWith(sample(), nil), "enter") // the connect confirm is up
	if !m.confirm.isActive() {
		t.Fatal("setup: expected the connect confirmation")
	}
	m = pressA(m, "S")
	if m.tab != tabPref {
		t.Fatalf("S under a popup must be inert, tab=%d", m.tab)
	}
	if !m.confirm.isActive() {
		t.Fatal("the popup must survive the key")
	}
}

// The footer is the disclosure channel. Inside a pty it must NOT offer them —
// a key advertised where it does not work is worse than one nobody mentions.
func TestFooterDisclosesTheTabKeys(t *testing.T) {
	m := appWith(sample(), nil)
	if foot := m.footer(); !strings.Contains(foot, "M/F/S") {
		t.Errorf("the panel footer must disclose the tab keys, got %q", foot)
	}
	pty := openOne(t)
	foot := pty.footer()
	if strings.Contains(foot, "M/F/S") {
		t.Errorf("the pty footer must not offer keys the remote is taking, got %q", foot)
	}
	if !strings.Contains(foot, "alt+esc") {
		t.Errorf("the pty footer must keep the way out, got %q", foot)
	}
}

// The short tier must still tell the tabs apart: each keeps its whole bracket,
// and no two collapse to the same thing.
func TestShortLabelsKeepTheKey(t *testing.T) {
	short := shortLabels(tabLabels)
	want := []string{"[M]", "[F]", "[S]"}
	for i := range want {
		if short[i] != want[i] {
			t.Fatalf("short label %d is %q, want %q", i, short[i], want[i])
		}
	}
}

// -------------------------------------------------------------- typing wins

// While something is being typed into, a printable key is a CHARACTER. This
// used to be decided key by key, and two of them got it wrong: V opened the
// splash and ? opened the help over a half-typed filename.
func TestATypedFilterKeepsEveryPrintableKey(t *testing.T) {
	m := sftpFixture(t, 100, 26)
	m.sftp.focus = panelLeftFiles
	m = pressA(m, "/")
	if !m.sftp.cur().filtering {
		t.Fatal("setup: expected a filter")
	}

	m = pressA(m, "M", "F", "S", "V", "?", "P", "J", "H")
	if got := m.sftp.cur().query; got != "MFSV?PJH" {
		t.Errorf("query = %q, want every key typed as a character", got)
	}
	if m.tab != tabFT {
		t.Errorf("no tab may switch while a filter is being typed, tab=%d", m.tab)
	}
	if m.splash.isActive() {
		t.Error("V must be a character here, not the easter egg")
	}
	if m.help.anim.isActive() {
		t.Error("? must be a character here, not the help")
	}
}

// Same rule on the other filter — one question asked in one place, so the two
// searches cannot drift apart.
func TestTheHostsFilterKeepsEveryPrintableKey(t *testing.T) {
	m := appWith(sample(), nil)
	m = pressA(m, "2", "/")
	if !m.hosts.filtering {
		t.Fatal("setup: expected a filter")
	}
	m = pressA(m, "S", "V")
	if got := m.hosts.query; got != "SV" {
		t.Errorf("query = %q, want the letters typed", got)
	}
	if m.tab != tabPref {
		t.Errorf("no tab may switch while a filter is being typed, tab=%d", m.tab)
	}
	if m.splash.isActive() {
		t.Error("V must be a character here")
	}
}

// Esc is not printable, so it stays the panel's: it drops the filter rather
// than being typed into it.
func TestEscStillLeavesAFilter(t *testing.T) {
	m := sftpFixture(t, 100, 26)
	m.sftp.focus = panelLeftFiles
	m = pressA(m, "/", "M")
	m = pressA(m, "esc")
	if m.sftp.cur().filtering {
		t.Error("esc must still drop the filter")
	}
}

// And once the filter is gone the same letters are keys again.
func TestTabKeysReturnAfterTheFilter(t *testing.T) {
	m := sftpFixture(t, 100, 26)
	m.sftp.focus = panelLeftFiles
	m = pressA(m, "/", "esc", "S")
	if m.tab != tabSSH {
		t.Errorf("S should switch tabs once nothing is being typed, tab=%d", m.tab)
	}
}

// A tab key must not reach a session either — quitting is the only thing that
// still works from under everything, and it is Ctrl+C.
func TestTabKeyDoesNotDisturbTheSessionItLeaves(t *testing.T) {
	m := openOne(t)
	m = pressA(m, "alt+esc", "F")
	if m.tab != tabFT {
		t.Fatalf("setup: expected the file transfer tab, got %d", m.tab)
	}
	if len(m.ssh.sessions) != 1 {
		t.Errorf("sessions = %d, want the one that was open", len(m.ssh.sessions))
	}
	if m.ssh.sessions[0].pty.exited() {
		t.Error("switching tabs must not touch the session")
	}
}

// The digits still address panels of the current tab — that is what the tabs
// gave the digits up for, and it survives them coming off the chords.
func TestDigitsStillAddressPanels(t *testing.T) {
	m := pressA(appWith(sample(), nil), "F")
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("3")})
	if got := next.(AppModel).sftp.focus; got != panelRightFiles {
		t.Errorf("3 should reach the right files panel, got %v", got)
	}
}
