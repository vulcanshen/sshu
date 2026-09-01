package ui

import (
	"strings"
	"testing"
)

// The whole reason the tabs moved onto Alt chords: they must work while a
// remote holds the keyboard, where every bare key belongs to the far end.
func TestAltChordsSwitchTabsFromInsideThePty(t *testing.T) {
	m := openOne(t)
	if !m.inPty() {
		t.Fatal("setup: the remote should hold the keyboard")
	}

	m = pressA(m, "alt+F")
	if m.tab != tabFT {
		t.Fatalf("alt+F from inside the pty should reach the file transfer tab, tab=%d", m.tab)
	}
	m = pressA(m, "alt+P")
	if m.tab != tabPref {
		t.Fatalf("alt+P should reach preference, tab=%d", m.tab)
	}
	m = pressA(m, "alt+S")
	if m.tab != tabSSH {
		t.Fatalf("alt+S should come back, tab=%d", m.tab)
	}
	// And the session neither died nor lost the screen on the way.
	if m.ssh.currentSession() == nil {
		t.Fatal("switching tabs must not touch the session")
	}

	// The UNSHIFTED chord is not sshu's inside a pty: M-f is forward-word on
	// the far end. It must reach the remote, not switch the tab.
	m = pressA(m, "alt+f")
	if m.tab != tabSSH {
		t.Fatalf("alt+f belongs to the remote inside a pty, tab=%d", m.tab)
	}
}

// Outside a pty the unshifted chord answers too — alt+f is a dead key there,
// and a dead key one shift away from a live one is a trap with no payoff.
func TestUnshiftedChordsWorkOutsideThePty(t *testing.T) {
	m := pressA(appWith(sample(), nil), "alt+f")
	if m.tab != tabFT {
		t.Fatalf("alt+f on a panel should switch tabs, tab=%d", m.tab)
	}
	m = pressA(m, "alt+p")
	if m.tab != tabPref {
		t.Fatalf("alt+p on a panel should switch tabs, tab=%d", m.tab)
	}
}

// Under a popup the chords do nothing: a tab switching beneath a form would
// strand the form over a surface it knows nothing about.
func TestAltChordsAreInertUnderAPopup(t *testing.T) {
	m := pressA(appWith(sample(), nil), "enter") // the connect confirm is up
	if !m.confirm.isActive() {
		t.Fatal("setup: expected the connect confirmation")
	}
	m = pressA(m, "alt+S")
	if m.tab != tabPref {
		t.Fatalf("alt+S under a popup must be inert, tab=%d", m.tab)
	}
	if !m.confirm.isActive() {
		t.Fatal("the popup must survive the chord")
	}
}

// The footer is the disclosure channel: the chords have to be readable there,
// on the panel and inside the pty alike.
func TestFooterDisclosesTheTabChords(t *testing.T) {
	m := appWith(sample(), nil)
	if foot := m.footer(); !strings.Contains(foot, "alt+p/f/s") {
		t.Errorf("the panel footer must disclose the tab chords, got %q", foot)
	}
	// Inside a pty only the SHIFTED chords are intercepted, so that is what
	// the footer says there.
	pty := openOne(t)
	if foot := pty.footer(); !strings.Contains(foot, "alt+P/F/S") {
		t.Errorf("the pty footer must disclose the chords that still work, got %q", foot)
	}
}

// The short tier must still tell the tabs apart: each keeps its whole
// bracket, and no two collapse to the same thing (cutting at the wrong spot
// once left three identical prefixes).
func TestShortLabelsKeepTheChord(t *testing.T) {
	short := shortLabels(append([]string{tabLead}, tabLabels...))
	want := []string{"[Alt]", "[p]", "[f]", "[s]"}
	for i := range want {
		if short[i] != want[i] {
			t.Fatalf("short label %d is %q, want %q", i, short[i], want[i])
		}
	}
}
