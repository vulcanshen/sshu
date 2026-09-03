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

// linesFor is the two lines one entry draws, with the styling stripped.
func linesFor(m sshModel, s *session) []string {
	innerW, _ := m.listInner()
	out := m.listItem(s, false, innerW)
	for i := range out {
		out[i] = ansi.Strip(out[i])
	}
	return out
}

func nameLine(m sshModel, s *session) string { return linesFor(m, s)[0] }
func addrLine(m sshModel, s *session) string { return linesFor(m, s)[1] }

func sessionOf(user, host string, port int) *session {
	return &session{id: 1, host: store.Host{Name: "the-name", User: user, Host: host, Port: port}}
}

// ------------------------------------------------------------------ the row

// An entry is TWO lines, whatever it holds — the name above, the address below.
// The height being a constant is what lets the scrolling divide instead of
// measure, so it is asserted for the shapes most likely to want a third line.
func TestASessionEntryIsAlwaysTwoLines(t *testing.T) {
	m := listOf(0, 100, 30)
	innerW, _ := m.listInner()
	for _, tc := range []struct {
		user, host string
		port       int
	}{
		{"demo", "localhost", 2222},
		{"root", "192.168.1.100", 22},
		{"deploy", "prod-web-01", 22},
		{"ec2-user", "bastion.eu-west-1.compute.internal", 22},
		{"a", "b", 1},
		{"averylongusernameindeed", "an.equally.long.hostname.example.com", 65535},
	} {
		lines := linesFor(m, sessionOf(tc.user, tc.host, tc.port))
		if len(lines) != sshItemH {
			t.Errorf("%s@%s:%d drew %d lines, want %d", tc.user, tc.host, tc.port, len(lines), sshItemH)
			continue
		}
		for i, ln := range lines {
			if strings.Contains(ln, "\n") {
				t.Errorf("%s@%s:%d line %d wrapped: %q", tc.user, tc.host, tc.port, i, ln)
			}
			if w := dispW(ln); w != innerW {
				t.Errorf("%s@%s:%d line %d is %d cells, want %d", tc.user, tc.host, tc.port, i, w, innerW)
			}
		}
	}
}

// The two lines say two different things, in the order they are asked in: what
// you called the machine, then what ssh will do about it. A single line could
// only ever hold one of them, and the one it held was the address — so a list
// of sessions never showed the names the hosts table is made of.
func TestTheNameIsAboveTheAddress(t *testing.T) {
	m := listOf(0, 100, 30)
	s := &session{id: 1, host: store.Host{
		Name: "prod-web-01", User: "deploy", Host: "10.0.3.14", Port: 2222}}

	if got := nameLine(m, s); !strings.Contains(got, "prod-web-01") {
		t.Errorf("the first line should be the name: %q", got)
	} else if strings.Contains(got, "10.0.3.14") || strings.Contains(got, "@") {
		t.Errorf("the address belongs on the second line: %q", got)
	}
	if got := addrLine(m, s); !strings.Contains(got, "deploy@10.0.3.14:2222") {
		t.Errorf("the second line should be the address: %q", got)
	} else if strings.Contains(got, "prod-web-01") {
		t.Errorf("the name belongs on the first line: %q", got)
	}
}

// The address starts at the BORDER, under the glyph rather than after the name.
// The alignment is not the point — the two columns are: indenting spends them
// on every row, and they come out of the half that is never allowed to be
// ambiguous. This asserts the gain, not just the offset, because an
// implementation could start at the border and still budget as if it had not.
func TestTheAddressStartsAtTheBorder(t *testing.T) {
	m := listOf(0, 100, 30)
	lines := linesFor(m, sessionOf("deploy", "10.0.3.14", 22))
	// In CELLS, not bytes: the display glyph is four bytes and one column.
	colOf := func(line, want string) int {
		i := strings.Index(line, want)
		if i < 0 {
			t.Fatalf("%q is not in %q", want, line)
		}
		return dispW(line[:i])
	}
	nameAt, addrAt := colOf(lines[0], "the-name"), colOf(lines[1], "deploy@")
	if addrAt >= nameAt {
		t.Errorf("the address should start left of the name, at %d vs %d\n%q\n%q",
			addrAt, nameAt, lines[0], lines[1])
	}
	if addrAt != sshListLeadW {
		t.Errorf("the address should start at the border (column %d), got %d",
			sshListLeadW, addrAt)
	}

	// And the columns it saved are actually SPENT on the address: an address
	// exactly as wide as the second line must survive whole, while one two
	// columns longer — the width the old indent would have left — must not.
	innerW, _ := m.listInner()
	fits := strings.Repeat("h", innerW-sshListLeadW-len("deploy@:22"))
	if got := addrLine(m, sessionOf("deploy", fits, 22)); strings.Contains(got, "…") {
		t.Errorf("an address that fills the line exactly was still cut: %q", got)
	}
}

// A name too long for the column is cut, not wrapped — the entry's height is
// the promise, and the address underneath is still there to identify it.
func TestALongNameIsCutRatherThanWrapped(t *testing.T) {
	m := listOf(0, 100, 30)
	s := &session{id: 1, host: store.Host{
		Name: "db-replica-tokyo-ap-northeast-1", User: "postgres", Host: "db.corp", Port: 22}}

	lines := linesFor(m, s)
	if len(lines) != sshItemH {
		t.Fatalf("a long name should not add a line, got %d", len(lines))
	}
	if !strings.Contains(lines[0], "…") {
		t.Errorf("the cut should be marked: %q", lines[0])
	}
	if !strings.Contains(lines[0], "db-replica") {
		t.Errorf("the beginning is what identifies it: %q", lines[0])
	}
}

// The port is never shortened, however little room is left: a truncated port is
// a different number rather than a shorter one.
func TestThePortSurvivesAnyAddress(t *testing.T) {
	m := listOf(0, 100, 30)
	for _, tc := range []struct {
		user, host string
		port       int
		want       string
	}{
		{"demo", "localhost", 2222, ":2222"},
		{"ec2-user", "bastion.eu-west-1.compute.internal", 22, ":22"},
		{"averyverylongusernameindeed", "an.equally.long.hostname.example.com", 65535, ":65535"},
	} {
		row := strings.TrimRight(addrLine(m, sessionOf(tc.user, tc.host, tc.port)), " ")
		if !strings.HasSuffix(row, tc.want) {
			t.Errorf("%s@%s:%d lost its port: %q, want it to end %q",
				tc.user, tc.host, tc.port, row, tc.want)
		}
	}
}

// An address that does not fit is SHORTENED, not wrapped — and the @ survives,
// because it is what makes the string read as an address at all.
func TestALongAddressIsShortenedOnBothSidesOfTheAt(t *testing.T) {
	m := listOf(0, 100, 30)
	row := addrLine(m, sessionOf("ec2-user", "bastion.eu-west-1.compute.internal", 22))
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
// port and all, and nothing is marked as cut. This is what pins sshLeftW, and
// the cases that pin it are the ones on a NON-DEFAULT port — ":2222" costs two
// columns more than ":22", and a column that only fits the default port is a
// column that shortens exactly the addresses whose port was worth showing.
// (sshu's own demo host is one of them.)
func TestCommonAddressesAreNotShortenedAtAll(t *testing.T) {
	m := listOf(0, 100, 30)
	for _, tc := range []struct {
		user, host string
		port       int
	}{
		{"demo", "localhost", 2222},
		{"root", "192.168.1.100", 22},
		{"deploy", "prod-web-01", 22},
		{"deploy", "10.0.3.14", 22},
		{"ubuntu", "ip-10-0-1-23", 22},  // the EC2 default shape, 22 columns
		{"deploy", "prod-web-01", 2222}, // the same host on a moved port, 23
		{"ec2-user", "10.0.3.14", 2222}, // 23 — the widest that has to survive
	} {
		row := addrLine(m, sessionOf(tc.user, tc.host, tc.port))
		if strings.Contains(row, "…") {
			t.Errorf("%s@%s:%d should fit whole at the default width: %q",
				tc.user, tc.host, tc.port, strings.TrimRight(row, " "))
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
	if got, want := m.listRows(), innerH/sshItemH; got != want {
		t.Errorf("listRows = %d entries, but %d lines hold %d", got, innerH, want)
	}
	drawn := m.listBody(m.sessions, 0, 0, innerW, innerH)
	if len(drawn) != m.listRows()*sshItemH {
		t.Errorf("the panel drew %d lines for the %d entries listRows claims",
			len(drawn), m.listRows())
	}
}

// An odd number of lines cannot hold half an entry: a session drawn as a name
// with no address under it reads as a rendering fault, not as a list that ran
// out of room. The last line is left blank instead.
func TestAnEntryIsDrawnWholeOrNotAtAll(t *testing.T) {
	for _, h := range []int{9, 10, 11, 12} {
		m := listOf(40, 100, h)
		innerW, innerH := m.listInner()
		drawn := m.listBody(m.sessions, 0, 0, innerW, innerH)
		if len(drawn)%sshItemH != 0 {
			t.Errorf("panel height %d drew %d lines — half an entry", h, len(drawn))
		}
		if len(drawn) > innerH {
			t.Errorf("panel height %d drew %d lines into a %d-line box", h, len(drawn), innerH)
		}
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

	// The window came with it. Three duplicates of one host draw three identical
	// entries (§11.32), so "which one is on screen" is not a question the frame
	// can answer — what it can answer is that the panel is no longer showing the
	// top of the list, and the invariant listBody draws on is that the cursor is
	// inside the window.
	if m.ssh.topSess == 0 {
		t.Errorf("the viewport never moved, top=%d cur=%d vis=%d",
			m.ssh.topSess, m.ssh.curSess, m.ssh.listRows())
	}
	if m.ssh.curSess < m.ssh.topSess || m.ssh.curSess >= m.ssh.topSess+m.ssh.listRows() {
		t.Errorf("the new session is outside the window: cur=%d top=%d vis=%d",
			m.ssh.curSess, m.ssh.topSess, m.ssh.listRows())
	}
	// And the panel drew whole entries into the space it has.
	panel := m.ssh.sessionsPanel(sshLeftW, m.ssh.sessionsH())
	if n := strings.Count(ansi.Strip(panel), "@"); n != m.ssh.listRows() {
		t.Errorf("the panel shows %d addresses, the window holds %d:\n%s",
			n, m.ssh.listRows(), ansi.Strip(panel))
	}
}
