package ui

import (
	"os"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/vulcanshen/sshu/internal/store"
)

// emptyApp is every tab with nothing in it.
func emptyApp(t *testing.T, w, h int) AppModel {
	t.Helper()
	m := New(nil, nil, store.DefaultConfig())
	next, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	return settle(next.(AppModel))
}

// The reported bug: the hint was truncated instead of wrapped, so on a narrow
// panel the sentence telling a stuck user what to press was cut off — at exactly
// the width where they most needed it.
func TestEmptyHintWrapsInsteadOfTruncating(t *testing.T) {
	for _, tab := range []tabID{tabHosts, tabSFTP, tabSSH} {
		m := emptyApp(t, 46, 16)
		m.tab = tab
		view := ansi.Strip(m.View())

		// Panel bodies only. The tab row's status slot is legitimately
		// truncated — it is the ambient channel, not the thing being read.
		for _, l := range strings.Split(view, "\n") {
			if strings.HasPrefix(l, "│") && strings.Contains(l, "…") {
				t.Errorf("tab %d: a panel body was truncated: %q", tab, l)
			}
		}
	}

	// Specifically: the tail of the longest hint survives a 46-column panel.
	m := emptyApp(t, 46, 16)
	if !strings.Contains(ansi.Strip(m.View()), "what you can do here") {
		t.Error("the end of the hint was cut off")
	}
}

// A key that lands at the start of the wrapped line is still a key — the style
// travels with the word, not with the finished sentence.
func TestKeysStayLitAcrossAWrap(t *testing.T) {
	withColour(t)
	words := emptyHint("aaaa bbbb [X] cccc", "[X]")
	lines := wrapHint(words, 10)
	if len(lines) < 2 {
		t.Fatalf("expected a wrap, got %d line(s)", len(lines))
	}

	var withKey string
	for _, l := range lines {
		rendered := renderHint(l, 20)
		if strings.Contains(ansi.Strip(rendered), "[X]") {
			withKey = rendered
		}
	}
	if withKey == "" {
		t.Fatal("the key did not survive wrapping")
	}
	if !strings.Contains(withKey, ansiOf(t, handColor)) {
		t.Error("the key lost its colour when it moved to another line")
	}
}

// Every empty panel is the same shape: the fact centred, the hint centred under
// it. They each used to invent their own — "(empty)" and "(none)" pinned to the
// top-left, a centred sentence with no fact, a fact and a sentence.
func TestEveryEmptyPanelIsTheSameShape(t *testing.T) {
	centred := func(t *testing.T, view, text string) {
		t.Helper()
		for _, l := range strings.Split(view, "\n") {
			if !strings.Contains(l, text) {
				continue
			}
			// Split on the border: two panels can share a line, and measuring
			// across both of them would call anything off-centre.
			for _, cell := range strings.Split(l, "│") {
				if !strings.Contains(cell, text) {
					continue
				}
				left := len(cell) - len(strings.TrimLeft(cell, " "))
				right := len(cell) - len(strings.TrimRight(cell, " "))
				if left-right > 1 || right-left > 1 {
					t.Errorf("%q is not centred in its panel (left=%d right=%d): %q",
						text, left, right, cell)
				}
				return
			}
		}
		t.Errorf("%q is not on screen", text)
	}

	m := emptyApp(t, 100, 20)
	centred(t, ansi.Strip(m.View()), "No hosts yet")

	m.tab = tabSFTP
	view := ansi.Strip(m.View())
	centred(t, view, "No host")
	centred(t, view, "Nothing marked")

	m.tab = tabSSH
	view = ansi.Strip(m.View())
	centred(t, view, "No sessions")
	centred(t, view, "No session on screen")

	// And an empty DIRECTORY, which is a fact about a connected side.
	s := sftpFixture(t, 100, 26)
	empty := s.sftp.sides[sideLeft].home + "/nothing"
	if err := os.Mkdir(empty, 0o755); err != nil {
		t.Fatal(err)
	}
	s.sftp.sides[sideLeft].open(empty)
	centred(t, ansi.Strip(s.View()), "Empty directory")
}

// A panel too short for the whole block gives up its parts in order — the blank,
// then hint lines — and keeps the fact. A panel that says nothing at all is the
// state this exists to prevent.
func TestAShortPanelKeepsTheFact(t *testing.T) {
	for _, innerH := range []int{1, 2, 3, 6} {
		got := emptyBody(40, innerH, "No hosts yet",
			emptyHint("Press [A] to add a host, or Space to see what you can do here",
				"[A]", "Space"))
		if len(got) == 0 {
			t.Fatalf("innerH=%d: nothing at all", innerH)
		}
		joined := ansi.Strip(strings.Join(got, "\n"))
		if !strings.Contains(joined, "No hosts yet") {
			t.Errorf("innerH=%d: the fact was dropped:\n%s", innerH, joined)
		}
		if len(got) > innerH {
			t.Errorf("innerH=%d: produced %d lines", innerH, len(got))
		}
	}
}

// The frame invariant, for every tab in its empty state.
func TestEmptyStatesPreserveFrame(t *testing.T) {
	for _, sz := range [][2]int{{120, 40}, {100, 26}, {80, 20}, {72, 16}, {50, 12}, {30, 9}} {
		w, h := sz[0], sz[1]
		for _, tab := range []tabID{tabHosts, tabSFTP, tabSSH} {
			m := emptyApp(t, w, h)
			m.tab = tab
			for i, l := range strings.Split(m.View(), "\n") {
				if lw := dispW(l); lw != w {
					t.Errorf("%dx%d tab %d line %d is %d cells, want %d: %q",
						w, h, tab, i, lw, w, l)
					break
				}
			}
		}
	}
}
