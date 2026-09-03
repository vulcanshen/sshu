package ui

import (
	"strconv"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/vulcanshen/sshu/internal/store"
)

// listOf is a sessions list with no processes behind it — [1] draws from the
// host and the shown set, and nothing about scrolling touches a PTY.
func listOf(n, w, h int) sshModel {
	m := newSSHModel()
	m.w, m.h = w, h
	for i := 1; i <= n; i++ {
		m.sessions = append(m.sessions, &session{
			id: i,
			host: store.Host{
				Name: "host-" + strconv.Itoa(i), User: "deploy",
				Host: "node-" + strconv.Itoa(i) + ".internal", Port: 22,
			},
		})
	}
	m.focus = panelSessions
	m.clampCursors()
	return m
}

func rowFor(m sshModel, s *session) string {
	innerW, _ := m.listInner()
	return ansi.Strip(m.listItem(s, false, innerW))
}

func sessionOf(user, host string, port, ord int) *session {
	return &session{id: 1, ordinal: ord, host: store.Host{User: user, Host: host, Port: port}}
}

// ------------------------------------------------------------------ the row

// The row says the whole of what a session IS, on ONE line: the display glyph,
// the address, the port, and the ordinal. It used to wrap instead, so the
// commonest entry of all — "demo@localhost:2222 #2" — cost two lines.
func TestASessionRowIsAlwaysOneLine(t *testing.T) {
	m := listOf(0, 100, 30)
	for _, tc := range []struct {
		user, host string
		port, ord  int
	}{
		{"demo", "localhost", 2222, 12},
		{"root", "192.168.1.100", 22, 12},
		{"deploy", "prod-web-01", 22, 12},
		{"ec2-user", "bastion.eu-west-1.compute.internal", 22, 12},
		{"a", "b", 1, 0},
	} {
		if row := rowFor(m, sessionOf(tc.user, tc.host, tc.port, tc.ord)); strings.Contains(row, "\n") {
			t.Errorf("%s@%s:%d wrapped:\n%q", tc.user, tc.host, tc.port, row)
		}
	}
}

// The port and the ordinal are never shortened. A truncated port is a different
// number rather than a shorter one, and the ordinal is the only thing that
// tells two sessions to the same host apart.
func TestThePortAndOrdinalSurviveAnyAddress(t *testing.T) {
	m := listOf(0, 100, 30)
	for _, tc := range []struct {
		user, host string
		port, ord  int
		want       string
	}{
		{"demo", "localhost", 2222, 12, ":2222 #12"},
		{"ec2-user", "bastion.eu-west-1.compute.internal", 22, 12, ":22 #12"},
		{"averyverylongusernameindeed", "an.equally.long.hostname.example.com", 65535, 9, ":65535 #9"},
	} {
		row := strings.TrimRight(rowFor(m, sessionOf(tc.user, tc.host, tc.port, tc.ord)), " ")
		if !strings.HasSuffix(row, tc.want) {
			t.Errorf("%s@%s:%d #%d lost its tail: %q, want it to end %q",
				tc.user, tc.host, tc.port, tc.ord, row, tc.want)
		}
	}
}

// An address that does not fit is SHORTENED, not wrapped — and the @ survives,
// because it is what makes the string read as an address at all.
func TestALongAddressIsShortenedOnBothSidesOfTheAt(t *testing.T) {
	m := listOf(0, 100, 30)
	row := rowFor(m, sessionOf("ec2-user", "bastion.eu-west-1.compute.internal", 22, 12))
	if strings.Count(row, "@") != 1 {
		t.Fatalf("the @ must survive: %q", row)
	}
	if !strings.Contains(row, "…") {
		t.Errorf("something should be marked as cut: %q", row)
	}
	// The beginning of each side is what identifies it, so both are kept.
	if !strings.Contains(row, "ec2") || !strings.Contains(row, "bast") {
		t.Errorf("both sides should still be recognisable: %q", row)
	}
}

func TestFitUserHost(t *testing.T) {
	for _, tc := range []struct {
		name, user, host string
		w                int
	}{
		{"fits whole", "demo", "localhost", 20},
		{"exactly", "demo", "localhost", 14},
		{"long host", "demo", "a-very-long-hostname.example.com", 14},
		{"long user", "a-very-long-username-indeed", "h", 14},
		{"both long", "averylongusername", "averylonghostname", 15},
		{"one column", "demo", "localhost", 1},
		{"none at all", "demo", "localhost", 0},
	} {
		got := fitUserHost(tc.user, tc.host, tc.w)
		if w := dispW(got); w > tc.w {
			t.Errorf("%s: %q is %d cells, over the %d asked for", tc.name, got, w, tc.w)
		}
		if tc.w >= 1 && !strings.Contains(got, "@") {
			t.Errorf("%s: the @ must survive: %q", tc.name, got)
		}
	}

	// The short side is kept whole AND the room it saves goes to the other one:
	// trimming "dep" to its half would buy nothing and cost the hostname three
	// characters. (Checking the prefix alone is not enough — truncate is a
	// no-op on a string already short enough, so an even split looks identical
	// from the left.)
	const long = "a-very-long-hostname.example.com"
	if got := fitUserHost("dep", long, 14); !strings.HasPrefix(got, "dep@") {
		t.Errorf("the short user should survive whole, got %q", got)
	} else if _, host, _ := strings.Cut(got, "@"); dispW(host) != 10 {
		t.Errorf("the host should take every column the user did not: %q gives it %d of 10",
			got, dispW(host))
	}
	if got := fitUserHost("a-very-long-username-indeed", "h", 14); !strings.HasSuffix(got, "@h") {
		t.Errorf("the short host should survive whole, got %q", got)
	}
	// Both over half: they split it, so neither disappears.
	got := fitUserHost("averylongusername", "averylonghostname", 15)
	u, h, _ := strings.Cut(got, "@")
	if dispW(u) == 0 || dispW(h) == 0 {
		t.Errorf("neither side may vanish: %q", got)
	}
}

// The width exists for this: the addresses people actually type arrive whole,
// with their port and ordinal, and nothing is marked as cut. 26 columns left 21
// for the row, one short of "demo@localhost:2222 #2" — the commonest shape
// there is.
func TestCommonAddressesAreNotShortenedAtAll(t *testing.T) {
	m := listOf(0, 100, 30)
	for _, tc := range []struct {
		user, host string
		port, ord  int
	}{
		{"demo", "localhost", 2222, 12},
		{"root", "192.168.1.100", 22, 12},
		{"deploy", "prod-web-01", 22, 12},
		{"deploy", "10.0.3.14", 22, 12},
	} {
		row := rowFor(m, sessionOf(tc.user, tc.host, tc.port, tc.ord))
		if strings.Contains(row, "…") {
			t.Errorf("%s@%s:%d #%d should fit whole at the default width: %q",
				tc.user, tc.host, tc.port, tc.ord, strings.TrimRight(row, " "))
		}
	}
}

// listRows is the number of rows the panel actually draws, which is what u/d
// take half of and what the viewport is sized against. Returning something
// self-consistent but wrong (a constant, say) leaves the scrolling agreeing
// with itself and disagreeing with the screen.
func TestListRowsMatchesWhatThePanelDraws(t *testing.T) {
	m := listOf(40, 100, 30)
	innerW, innerH := m.listInner()
	if got := m.listRows(); got != innerH {
		t.Errorf("listRows = %d, but the box is %d lines", got, innerH)
	}
	drawn := m.listBody(m.sessions, 0, 0, innerW, innerH)
	if len(drawn) != m.listRows() {
		t.Errorf("the panel drew %d rows, listRows says %d", len(drawn), m.listRows())
	}
}

// ------------------------------------------------------------- scrolling

// Walking off the bottom scrolls. This never happened: topSess was only ever
// clamped into range, so the cursor simply left the screen and stayed gone.
func TestTheSessionsListFollowsItsCursorDown(t *testing.T) {
	m := listOf(40, 100, 30)
	if vis := m.listRows(); vis >= 40 {
		t.Fatalf("setup: the list should not fit — %d of 40 visible", vis)
	}

	for range 39 {
		m.handleListKey("j")
	}
	if m.curSess != 39 {
		t.Fatalf("the cursor should be on the last session, got %d", m.curSess)
	}
	if m.topSess == 0 {
		t.Error("the viewport never moved — the cursor is off the bottom")
	}
	if m.curSess < m.topSess || m.curSess >= m.topSess+m.listRows() {
		t.Errorf("the cursor is outside the window: cur=%d top=%d vis=%d",
			m.curSess, m.topSess, m.listRows())
	}
}

// And back up again — a viewport that only ever scrolls one way strands the
// top of the list.
func TestTheSessionsListFollowsItsCursorUp(t *testing.T) {
	m := listOf(40, 100, 30)
	m.curSess = 39
	m.clampCursors()
	if m.topSess == 0 {
		t.Fatal("setup: expected to be scrolled down")
	}

	for range 39 {
		m.handleListKey("k")
	}
	if m.curSess != 0 {
		t.Fatalf("the cursor should be back at the top, got %d", m.curSess)
	}
	if m.topSess != 0 {
		t.Errorf("the viewport should have come back with it, top=%d", m.topSess)
	}
}

// G and gg are the ends, and the window has to be at the matching end.
func TestTheSessionsListFollowsToEitherEnd(t *testing.T) {
	m := listOf(40, 100, 30)

	m.handleListKey("G")
	if m.curSess != 39 {
		t.Fatalf("G should reach the last session, got %d", m.curSess)
	}
	if m.curSess >= m.topSess+m.listRows() {
		t.Error("G left the cursor below the window")
	}

	m.handleListKey("gg")
	if m.curSess != 0 || m.topSess != 0 {
		t.Errorf("gg should reach the top with the window, cur=%d top=%d", m.curSess, m.topSess)
	}
}

// Shrinking the window under a cursor that was on screen has to bring it back.
func TestAResizeBringsTheCursorBackOnScreen(t *testing.T) {
	m := listOf(40, 100, 40)
	m.curSess = 20
	m.clampCursors()
	before := m.topSess

	m.setSize(100, 14) // a much shorter panel
	if m.curSess < m.topSess || m.curSess >= m.topSess+m.listRows() {
		t.Errorf("the cursor left the window on resize: cur=%d top=%d vis=%d (was top=%d)",
			m.curSess, m.topSess, m.listRows(), before)
	}
}

// Nothing is drawn when the side column is folded away, so nothing is scrolled
// either.
func TestNoScrollingWhileTheListIsFoldedAway(t *testing.T) {
	m := listOf(40, 100, 30)
	m.curSess, m.topSess = 39, 5
	m.setFocus(panelPty) // the side column folds
	m.clampCursors()
	if m.topSess != 5 {
		t.Errorf("a hidden list should not be scrolled, top=%d", m.topSess)
	}
}

// ------------------------------------------------------------- end to end

// The cursor lands on a session that is genuinely off the bottom, through the
// real key path, and the RENDERED panel shows it.
func TestANewSessionScrollsIntoView(t *testing.T) {
	aliveSSH(t)
	// Short enough that [1] holds two rows at a time — one row per session now,
	// so the window has to be shorter than it used to be to overflow.
	m := New(sample(), nil, store.DefaultConfig())
	next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 7})
	m = settle(next.(AppModel))
	m = pressA(m, "enter", "enter") // connect, landing in the pty
	t.Cleanup(func() { m.ssh.stopAll() })
	m = pressA(m, "alt+esc")

	if got := m.ssh.listRows(); got > 2 {
		t.Fatalf("setup: [1] should be too short for the list, it holds %d", got)
	}

	m = pressA(m, "D", "enter") // duplicate: the cursor goes to the new session
	m = pressA(m, "D", "enter") // and again
	if len(m.ssh.sessions) != 3 {
		t.Fatalf("expected three sessions, got %d", len(m.ssh.sessions))
	}
	if m.ssh.curSess != 2 {
		t.Fatalf("the cursor should be on the newest, got %d", m.ssh.curSess)
	}

	// The newest session's row is what the panel is actually showing.
	panel := ansi.Strip(m.ssh.sessionsPanel(sshLeftW, m.ssh.sessionsH()))
	if !strings.Contains(panel, "#3") {
		t.Errorf("the new session's row is not on screen:\n%s", panel)
	}
}
