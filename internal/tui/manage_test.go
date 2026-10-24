package tui

import (
	"bytes"
	"os"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	terminu "github.com/vulcanshen/terminu-go"

	"github.com/vulcanshen/sshu/internal/store"
)

// docs/icon.svg is the mark; the splash draws the copy go:embed can reach
// (tdp E6). The two must not drift apart.
func TestTheIconIsTheSplash(t *testing.T) {
	doc, err := os.ReadFile("../../docs/icon.svg")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(doc, icon) {
		t.Fatal("internal/tui/icon.svg differs from docs/icon.svg: copy it over again")
	}
}

// The sections panel lists what sshu keeps under three groups, and [2] shows
// the section under the cursor as the cursor moves (layout/panel, details).
func TestTheSectionUnderTheCursorIsShownBesideIt(t *testing.T) {
	hs := newHarness(t, 100, 30, testData(sampleHosts(), sampleCreds()))
	hs.contains("SSHU")
	hs.contains("LOGS")
	hs.contains("[2] HOSTS")
	hs.contains("prod-web-01")
	hs.keys("j")
	hs.contains("[2] CREDENTIALS")
	hs.lacks("prod-web-01")
	hs.keys("k", "Enter")
	// Enter sent the focus to [2]: its hint is the hosts' own.
	hs.contains("Enter:connect")
}

// Connecting first shows what the host is and how it authenticates, with the
// question last (tdp F1, a confirm with details).
func TestConnectShowsTheHostBeforeAsking(t *testing.T) {
	hs := newHarness(t, 100, 40, testData(sampleHosts(), sampleCreds()))
	hs.keys("2", "j", "Enter")
	hs.contains("CONNECT: db-replica")
	hs.contains("db.internal")
	hs.contains("••••••••")
	hs.lacks("s3cret")
	hs.contains("Connect to db-replica?")
	hs.keys("Esc")
	hs.lacks("Connect to db-replica?")
}

// A credential host shows who it connects as from the credential.
func TestACredentialHostShowsItsCredentialsUser(t *testing.T) {
	hs := newHarness(t, 120, 30, testData(sampleHosts(), sampleCreds()))
	for _, l := range hs.frame() {
		if strings.Contains(l, "staging") && !strings.Contains(l, "ops") {
			t.Fatalf("the staging row does not show the credential's user:\n%s", l)
		}
	}
}

// Adding a host goes through the form and is written back.
func TestAddingAHostSavesIt(t *testing.T) {
	d := testData(nil, nil)
	var saved []store.Host
	d.SaveHosts = func(h []store.Host) error { saved = h; return nil }
	hs := newHarness(t, 100, 40, d)
	hs.keys("2", "A")
	hs.contains("NEW HOST")
	// Name
	hs.keys("Enter")
	hs.keys("web")
	hs.keys("Enter")
	// Host
	hs.keys("j", "Enter")
	hs.keys("10.0.0.9")
	hs.keys("Enter")
	// User
	hs.keys("j", "j", "Enter")
	hs.keys("me")
	hs.keys("Enter")
	hs.keys("Tab", "Enter")
	if len(saved) != 1 || saved[0].Name != "web" || saved[0].Host != "10.0.0.9" || saved[0].User != "me" || saved[0].Port != 22 {
		t.Fatalf("saved %+v\n%s", saved, hs.text())
	}
	hs.lacks("NEW HOST")
	hs.contains("web")
	if len(hs.s.d.Changes) != 1 {
		t.Fatalf("changes: %+v", hs.s.d.Changes)
	}
}

// Every row of every Manage view is exactly as wide as the terminal
// (tdp L4), with icons one cell or two and powerline symbols one or two (tdp E5).
func TestEveryManageRowIsAsWideAsTheScreen(t *testing.T) {
	defer terminu.SetIconWidth(1, 1)
	for _, iw := range [][2]int{{1, 1}, {2, 1}, {2, 2}} {
		for _, size := range [][2]int{{80, 40}, {60, 24}, {120, 40}, {60, 20}, {200, 60}} {
			w, h := size[0], size[1]
			hs := newHarness(t, w, h, testData(sampleHosts(), sampleCreds()))
			terminu.SetIconWidth(iw[0], iw[1])
			for i := 0; i < 7; i++ {
				hs.fullWidth(w, "section")
				hs.keys("j")
			}
			// Popups take their opening animation each: once per width.
			if size != [2]int{80, 40} && size != [2]int{60, 20} {
				continue
			}
			hs.keys("gg", "Space")
			hs.fullWidth(w, "space menu")
			hs.keys("Esc", "?")
			hs.fullWidth(w, "key reference")
			hs.keys("Esc", "2", "E")
			hs.fullWidth(w, "host form")
			hs.keys("Esc", "Enter")
			hs.fullWidth(w, "connect confirm")
		}
	}
}

// A delete leaves its Space menu nothing to act on, so Enter on its confirm
// closes the menu too; Esc on the confirm goes back to the menu (tdp T1: the
// confirm is only a gate).
func TestADeleteFromTheSpaceMenuClearsTheMenu(t *testing.T) {
	d := testData(sampleHosts(), sampleCreds())
	var saved []store.Host
	d.SaveHosts = func(h []store.Host) error { saved = h; return nil }
	hs := newHarness(t, 100, 40, d)
	menu := "a new host starting from this one" // a row only the Space menu has
	hs.keys("2", "Space")
	hs.contains(menu)
	hs.keys("X")
	hs.contains("Delete host prod-web-01?")
	hs.keys("Esc")
	hs.lacks("Delete host prod-web-01?")
	hs.contains(menu)
	hs.keys("X", "Enter")
	if len(saved) != len(sampleHosts())-1 {
		t.Fatalf("the host was not deleted: %+v", saved)
	}
	hs.lacks(menu)
	hs.lacks("Delete host")
}

// What startup moved is said in a toast, from where to where; what it could
// not move, and what the new place already had, open an error popup with both
// paths, to move or to merge (tdp E3). The popup's message is one paragraph:
// a \n in it is drawn as \n (components, control characters).
func TestStartupSaysWhatItMoved(t *testing.T) {
	d := testData(sampleHosts(), sampleCreds())
	d.Moves = []store.Move{{What: "journals", From: "/home/u/.config/sshu", To: "/home/u/.sshu", Moved: []string{"errors.yaml"}}}
	hs := newHarness(t, 120, 40, d)
	hs.contains("Moved the journals from /home/u/.config/sshu to /home/u/.sshu")

	d = testData(sampleHosts(), sampleCreds())
	d.Moves = []store.Move{{What: "settings", From: "/home/u/old", To: "/home/u/.config/sshu", Err: os.ErrPermission}}
	hs = newHarness(t, 120, 40, d)
	hs.contains("NOT MOVED")
	hs.contains("/home/u/old")
	hs.contains("/home/u/.config/sshu")
	hs.contains("move them to")
	hs.lacks(`\n`)

	d = testData(sampleHosts(), sampleCreds())
	d.Moves = []store.Move{{What: "settings", From: "/home/u/old", To: "/home/u/.config/sshu",
		Moved: []string{"state.yaml"}, Left: []string{"config.yaml"}},
		{What: "data", From: "/home/u/.config/sshu", To: "/home/u/.sshu", Left: []string{".sshukey"}}}
	hs = newHarness(t, 120, 40, d)
	hs.contains("NOT MOVED")
	hs.contains("already has config.yaml")
	hs.contains("already has .sshukey")
	hs.contains("Merge it into")
	hs.lacks(`\n`)

	// A file kept with the key is not one the new place has.
	d = testData(sampleHosts(), sampleCreds())
	d.Moves = []store.Move{{What: "data", From: "/home/u/.config/sshu", To: "/home/u/.sshu",
		Left: []string{".sshukey"}, Kept: []string{"hosts.yaml"}}}
	hs = newHarness(t, 120, 40, d)
	hs.contains("/home/u/.sshu already has .sshukey, so sshu left it in")
	hs.contains("hosts.yaml stays there too:")
	hs.contains("Merge them into")
	hs.lacks("already has .sshukey, hosts.yaml")

	d = testData(sampleHosts(), sampleCreds())
	d.Moves = []store.Move{{What: "data", From: "/home/u/.config/sshu", To: "/home/u/.sshu",
		Kept: []string{".sshukey", "credentials.yaml"}}}
	hs = newHarness(t, 120, 40, d)
	hs.contains("already has some of the key and the files it seals")
	hs.contains("left .sshukey, credentials.yaml in")
}

// Below sshu's least size, down to 1×1, nothing brings it down: every row is
// as wide as the terminal, the name is drawn and the hint where it fits; a
// popup that was open is there again when the terminal grows (tdp E5, tdp
// L1).
func TestTinyTerminalsDoNotCrash(t *testing.T) {
	for _, size := range [][2]int{{80, 5}, {59, 20}, {40, 3}, {2, 2}, {1, 1}} {
		w, h := size[0], size[1]
		hs := newHarness(t, 100, 40, testData(sampleHosts(), sampleCreds()))
		hs.keys("2", "Space")
		hs.send(tea.WindowSizeMsg{Width: w, Height: h})
		for _, k := range []string{"Esc", "j", "F", "Tab", "S"} {
			hs.keys(k) // keys do nothing down here
			hs.fullWidth(w, "too small")
		}
		if w >= 4 {
			hs.contains("sshu")
		}
		if w >= 40 && h >= 2 {
			hs.contains("needs a terminal of at least 60×20")
		}
		hs.send(tea.WindowSizeMsg{Width: 100, Height: 40})
		hs.contains("ITEM OPERATION") // the Space menu, as it was
	}
}
