package ui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/vulcanshen/sshu/internal/store"
)

func sample() []store.Host {
	return []store.Host{
		{Name: "prod-web-01", Host: "10.0.3.14", Port: 22, User: "deploy",
			Auth: store.AuthPrivateKey, IdentityFile: "~/.ssh/id_ed25519"},
		{Name: "db-replica-tokyo-ap-northeast-1", Host: "db.internal.corp", Port: 2222,
			User: "postgres", Auth: store.AuthPassword, Password: "s3cr3t"},
		{Name: "bastion-eu-west-1", Host: "bastion.eu-west-1.compute.internal", Port: 22,
			User: "ec2-user", Auth: store.AuthPrivateKey},
		{Name: "staging-api", Host: "staging.example.com", Port: 22, User: "app",
			Auth: store.AuthPassword},
		{Name: "jump", Host: "jump.corp", Port: 22, User: "root", Auth: store.AuthPrivateKey},
	}
}

func sized(hosts []store.Host, w, h int) AppModel {
	m := New(hosts, nil, store.DefaultConfig())
	next, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	return next.(AppModel)
}

func press(m AppModel, keys ...string) AppModel {
	for _, k := range keys {
		next, _ := m.Update(keyMsg(k))
		m = next.(AppModel)
	}
	return m
}

// TestViewLineWidths is the frame invariant: every rendered line must be exactly
// the terminal width. A card is a fixed-width box, so one field that miscounts
// itself pushes a border out and shears the whole grid.
func TestViewLineWidths(t *testing.T) {
	sizes := [][2]int{
		{78, 24}, {80, 30}, {120, 40}, {160, 50}, // grid, 2-4 columns
		{40, 20}, {38, 12}, // grid at its lower bound
		{37, 20}, {34, 14}, {32, 10}, // narrow list fallback (32 = minAppW)
		{100, 7}, // very short: chrome + a clipped panel
	}
	for _, hosts := range [][]store.Host{sample(), nil} {
		for _, sz := range sizes {
			w, h := sz[0], sz[1]
			m := sized(hosts, w, h)
			for _, tab := range []string{"M", "F", "S"} {
				got := press(m, tab).View()
				lines := strings.Split(got, "\n")
				if len(lines) != h {
					t.Errorf("hosts=%d %dx%d tab=%s: %d lines, want %d",
						len(hosts), w, h, tab, len(lines), h)
				}
				for i, l := range lines {
					if lw := lipgloss.Width(l); lw != w {
						t.Errorf("hosts=%d %dx%d tab=%s line %d: width %d, want %d\n%q",
							len(hosts), w, h, tab, i, lw, w, l)
					}
				}
			}
		}
	}
}

// The table sheds columns as it narrows, from the least load-bearing end. The
// name is the last thing standing: a row you cannot name is not a row.
func TestTableShedsColumnsAsItNarrows(t *testing.T) {
	for _, tc := range []struct {
		w                      int
		wantPort, wantAuth     bool
		wantUser, wantHostCols bool
	}{
		// Derived from the minimums: Auth needs w>=51, Port w>=37, and the
		// user/host pair w>=30. Below that only the name is left.
		{100, true, true, true, true},
		{51, true, true, true, true},
		{50, true, false, true, true},
		{37, true, false, true, true},
		{36, false, false, true, true},
		{30, false, false, true, true},
		{29, false, false, false, false},
	} {
		c := computeCols(tc.w)
		if c.port != tc.wantPort || c.auth != tc.wantAuth {
			t.Errorf("w=%d: port=%v auth=%v, want %v/%v", tc.w, c.port, c.auth, tc.wantPort, tc.wantAuth)
		}
		if (c.user > 0) != tc.wantUser || (c.host > 0) != tc.wantHostCols {
			t.Errorf("w=%d: user=%d host=%d, want present %v/%v", tc.w, c.user, c.host, tc.wantUser, tc.wantHostCols)
		}
		if c.name <= 0 {
			t.Errorf("w=%d: the name column must never disappear", tc.w)
		}
	}
}

// Header and rows are laid out by the same function, so their columns cannot
// drift apart. This checks the invariant that keeps that true.
func TestTableHeaderAndRowsAlign(t *testing.T) {
	for _, w := range []int{100, 76, 60, 44, 30, 18} {
		c := computeCols(w)
		head := dispW(tableHeader(c, w))
		row := dispW(renderHostRow(sample()[1], sample()[1].User, c, false, w))
		selRow := dispW(renderHostRow(sample()[1], sample()[1].User, c, true, w))
		if head != w || row != w || selRow != w {
			t.Errorf("w=%d: header=%d row=%d selected=%d, all should be %d", w, head, row, selRow, w)
		}
	}
}

// A table has rows, not a grid: j/k walk them, and the list is a ring — off one
// end is the other, so the last host is one keystroke from the first.
func TestNavigation(t *testing.T) {
	m := hostsModel{hosts: sample(), w: 78, h: 24}
	n := len(sample())

	m.handleKey("j")
	if m.cursor != 1 {
		t.Fatalf("j: cursor=%d want 1", m.cursor)
	}
	m.handleKey("k")
	m.handleKey("k") // off the top: round to the bottom
	if m.cursor != n-1 {
		t.Fatalf("k at the top should wrap to %d, cursor=%d", n-1, m.cursor)
	}
	m.handleKey("j") // and back again
	if m.cursor != 0 {
		t.Fatalf("j at the bottom should wrap to 0, cursor=%d", m.cursor)
	}
	m.handleKey("G")
	if m.cursor != n-1 {
		t.Fatalf("G: cursor=%d want %d", m.cursor, n-1)
	}
	m.handleKey("j") // off the bottom: round to the top
	if m.cursor != 0 {
		t.Fatalf("j at the bottom should wrap to 0, cursor=%d", m.cursor)
	}
	m.handleKey("G")
	m.handleKey("u") // a half-page is aimed, not wrapped
	if m.cursor >= n-1 {
		t.Fatalf("u should move up from the bottom, cursor=%d", m.cursor)
	}
	m.handleKey("u")
	m.handleKey("u")
	m.handleKey("u")
	if m.cursor != 0 {
		t.Fatalf("u must stop at the top, not wrap, cursor=%d", m.cursor)
	}
	m.handleKey("gg")
	if m.cursor != 0 {
		t.Fatalf("gg: cursor=%d want 0", m.cursor)
	}
}

func TestGGChord(t *testing.T) {
	m := sized(sample(), 78, 24)
	m = press(m, "G")
	if m.hosts.cursor != 4 {
		t.Fatalf("G: cursor=%d", m.hosts.cursor)
	}
	m = press(m, "g", "g")
	if m.hosts.cursor != 0 {
		t.Fatalf("gg chord: cursor=%d want 0", m.hosts.cursor)
	}
	// A lone g must not linger: g then j is a plain j, not a chord.
	m = press(m, "g", "j")
	if m.hosts.cursor != 1 {
		t.Fatalf("g then j should move down, cursor=%d want 1", m.hosts.cursor)
	}
}

func TestScrollFollowsCursor(t *testing.T) {
	many := make([]store.Host, 20)
	for i := range many {
		many[i] = store.Host{Name: fmt.Sprintf("h%02d", i), Host: "x", Port: 22,
			User: "u", Auth: store.AuthPassword}
	}
	m := hostsModel{hosts: many, w: 78, h: 10} // 10 - 2 border - 1 header = 7 rows
	if got := m.visibleRows(); got != 7 {
		t.Fatalf("fixture expects 7 visible rows, got %d", got)
	}
	m.cursor = 19
	m.ensureVisible()
	if m.top != 13 {
		t.Fatalf("top=%d, want 13 (last row visible)", m.top)
	}
	m.cursor = 0
	m.ensureVisible()
	if m.top != 0 {
		t.Fatalf("top=%d, want 0", m.top)
	}
}

func TestTabSwitching(t *testing.T) {
	m := sized(sample(), 78, 24)
	if m.tab != tabPref {
		t.Fatal("must start on hosts")
	}
	m = press(m, "S")
	if m.tab != tabSSH {
		t.Fatalf("alt+S -> tab=%d want %d", m.tab, tabSSH)
	}
	// Tab stays in this tab. The ssh tab has one list panel, so nowhere to go.
	m = press(m, "tab")
	if m.tab != tabSSH || m.ssh.focus != panelSessions {
		t.Fatalf("tab should stay on [1], got tab=%d focus=%d", m.tab, m.ssh.focus)
	}
	// Only an Alt chord changes tab — a bare digit addresses a panel.
	m = press(m, "M")
	if m.tab != tabPref {
		t.Fatalf("alt+P -> tab=%d want %d", m.tab, tabPref)
	}
	m = press(m, "3")
	if m.tab != tabPref {
		t.Fatalf("a bare digit must not switch tabs any more, tab=%d", m.tab)
	}
}

// The empty state must name the panel action own bracket and Space, or a
// first-run user facing an empty panel has no visible way forward (§1.5).
//
// The bracket is read OUT OF THE ACTION TABLE rather than spelled here: renaming
// the action should not need this test edited, it should just carry on checking
// the truth.
func TestEmptyStateDisclosesEntryPoints(t *testing.T) {
	// The action that gets you OUT of the empty state — found by what it is,
	// not by its letter, so renaming the key does not need this test edited.
	add := ""
	for _, a := range hostActions {
		if a.label == "Add" {
			add = "[" + a.key + "]"
		}
	}
	if add == "" {
		t.Fatal("panel [1] has no panel-level action to disclose")
	}
	got := sized(nil, 78, 24).View()
	for _, want := range []string{add, "Space", "No hosts yet"} {
		if !strings.Contains(got, want) {
			t.Errorf("empty state must mention %q", want)
		}
	}
}

// The footer is the only disclosure channel for the two VTP entry keys.
func TestFooterDisclosesEntryKeys(t *testing.T) {
	got := sized(sample(), 78, 24).View()
	for _, want := range []string{"space", "menu", "?", "help"} {
		if !strings.Contains(got, want) {
			t.Errorf("footer must mention %q", want)
		}
	}
}

// TestDump is not an assertion — run `go test ./internal/ui -run Dump -v` to eyeball
// the layout at a given size.
func TestDump(t *testing.T) {
	if !testing.Verbose() {
		t.Skip("run with -v to print the rendered screen")
	}
	for _, sz := range [][2]int{{78, 22}, {118, 22}, {34, 14}} {
		t.Logf("\n=== %dx%d ===\n%s", sz[0], sz[1], sized(sample(), sz[0], sz[1]).View())
	}
}
