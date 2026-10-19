package tui

import (
	"bytes"
	"os"
	"strings"
	"testing"
	"time"

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

// Every row of every Manage view is exactly as wide as the terminal (Rules
// L4), with icons one cell or two and powerline symbols one or two (tdp E5).
func TestEveryManageRowIsAsWideAsTheScreen(t *testing.T) {
	defer terminu.SetIconWidth(1, 1)
	for _, iw := range [][2]int{{1, 1}, {2, 1}, {2, 2}} {
		for _, size := range [][2]int{{80, 40}, {60, 24}, {120, 40}, {40, 12}, {200, 60}} {
			w, h := size[0], size[1]
			hs := newHarness(t, w, h, testData(sampleHosts(), sampleCreds()))
			terminu.SetIconWidth(iw[0], iw[1])
			for i := 0; i < 7; i++ {
				hs.fullWidth(w, "section")
				hs.keys("j")
			}
			// Popups take their opening animation each: once per width.
			if w != 80 && w != 40 {
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
// not move opens an error popup with both paths (tdp E3).
func TestStartupSaysWhatItMoved(t *testing.T) {
	d := testData(sampleHosts(), sampleCreds())
	d.Moves = []store.Move{{What: "journals", From: "/home/u/.config/sshu", To: "/home/u/.sshu"}}
	hs := newHarness(t, 120, 40, d)
	hs.contains("Moved the journals from /home/u/.config/sshu to /home/u/.sshu")

	d = testData(sampleHosts(), sampleCreds())
	d.Moves = []store.Move{{What: "settings", From: "/home/u/old", To: "/home/u/.config/sshu", Err: os.ErrPermission}}
	hs = newHarness(t, 120, 40, d)
	hs.settle(func() bool { return hs.shows("MOVE FAILED") }, 2*time.Second)
	hs.contains("MOVE FAILED")
	hs.contains("/home/u/old")
	hs.contains("/home/u/.config/sshu")
	hs.contains("move them to")
}

// Below the smallest size, down to 1×1, no screen brings sshu down (tdp E5,
// L1). Drawing the name and the hint there is the lib's, from terminu-go's
// next version; then this test looks for them too, and opens the popups:
// v0.1.13 panics drawing a popup taller than the terminal (fixed after it,
// f245cbc).
func TestTinyTerminalsDoNotCrash(t *testing.T) {
	for _, size := range [][2]int{{80, 5}, {40, 3}, {2, 2}, {1, 1}} {
		hs := newHarness(t, size[0], size[1], testData(sampleHosts(), sampleCreds()))
		for _, k := range []string{"2", "j", "F", "Tab", "S", "Tab", "M"} {
			hs.keys(k)
			_ = hs.frame()
		}
	}
}
