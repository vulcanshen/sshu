package ui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/vulcanshen/sshu/internal/store"
)

// `/` narrows the table, matching across the identifying columns at once — so a
// query can span them.
func TestHostsSearchMatchesAcrossColumns(t *testing.T) {
	m := sized(sample(), 100, 24)
	all := m.hosts.rowCount()

	m = pressA(m, "/")
	if !m.hosts.filtering {
		t.Fatal("/ should start a search")
	}
	m = typeText(m, "postgres")
	if n := m.hosts.rowCount(); n == 0 || n >= all {
		t.Fatalf("the table should have narrowed, %d of %d", n, all)
	}
	// Ranked best-first: a permissive subsequence can match several rows, so what
	// matters is that the one you meant is row 0.
	h, ok := m.hosts.rowAt(0)
	if !ok || h.User != "postgres" {
		t.Errorf("row 0 is %+v, want the postgres host", h)
	}

	// A query spanning two columns still lands, because the haystack is the
	// row's fields joined.
	m = pressA(m, "esc")
	m = pressA(m, "/")
	m = typeText(m, "postgres 2222")
	if n := m.hosts.rowCount(); n != 1 {
		t.Errorf("a cross-column query matched %d rows, want 1", n)
	}
}

// Auth is deliberately NOT in the haystack: "password" and "privatekey" are two
// words most of the table shares, so matching them would drag in rows that have
// nothing to do with what was typed.
func TestHostsSearchIgnoresTheAuthColumn(t *testing.T) {
	hosts := []store.Host{
		{Name: "alpha", User: "root", Host: "a.example", Port: 22, Auth: store.AuthPassword},
		{Name: "beta", User: "root", Host: "b.example", Port: 22, Auth: store.AuthPrivateKey},
	}
	m := sized(hosts, 100, 24)

	m = pressA(m, "/")
	m = typeText(m, "privatekey")
	if n := m.hosts.rowCount(); n != 0 {
		t.Errorf("searching the auth column matched %d rows, want none", n)
	}

	// And the fields that ARE searched still work on the same data.
	m = pressA(m, "esc")
	m = pressA(m, "/")
	m = typeText(m, "beta")
	if n := m.hosts.rowCount(); n != 1 {
		t.Errorf("searching a name matched %d rows, want 1", n)
	}
}

// The query takes the header's row rather than pushing the table down, and its
// right end says how much of the table is left.
func TestHostsSearchRowReplacesTheHeader(t *testing.T) {
	m := sized(sample(), 100, 24)
	before := strings.Count(m.View(), "\n")

	m = pressA(m, "/")
	m = typeText(m, "prod")

	if got := strings.Count(m.View(), "\n"); got != before {
		t.Errorf("the search row changed the line count: %d -> %d", before, got)
	}
	view := ansi.Strip(m.View())
	if strings.Contains(view, "Name  ") {
		t.Error("the column header should have yielded its row to the query")
	}
	if !strings.Contains(view, glyphSearch) {
		t.Error("the query should be prompted by the search glyph, not a literal /")
	}
	want := fmt.Sprintf("%d of %d", m.hosts.rowCount(), len(sample()))
	if !strings.Contains(view, want) {
		t.Errorf("the row should say %q, got:\n%s", want, view)
	}
}

// The cursor addresses the FILTERED row, so every action lands on what is on
// screen rather than on whatever holds that index in the unfiltered list.
func TestHostsActionsFollowTheFilteredCursor(t *testing.T) {
	m := sized(sample(), 100, 24)
	m = pressA(m, "/")
	m = typeText(m, "postgres")

	want, ok := m.hosts.rowAt(0)
	if !ok {
		t.Fatal("expected a match")
	}
	if want.Name == sample()[0].Name {
		t.Fatal("setup: the match should not also be the first unfiltered row")
	}

	got, ok := m.cursorHost()
	if !ok || got.Name != want.Name {
		t.Errorf("cursorHost is %q, want the row on screen %q", got.Name, want.Name)
	}
	// While the query is open, letters are letters (§4.5) — so acting on a result
	// means leaving the search first, and leaving must not lose your place.
	m = pressA(m, "esc")
	if got, _ := m.cursorHost(); got.Name != want.Name {
		t.Fatalf("Esc landed on %q, want the row that was under the cursor, %q",
			got.Name, want.Name)
	}
	after := pressA(m, "E")
	if !after.form.isActive() {
		t.Fatal("E should open the form")
	}
	if after.form.editing != want.Name {
		t.Errorf("editing %q, want %q", after.form.editing, want.Name)
	}
}

// Esc leaves the search and restores the table; it is the innermost thing Esc
// can drop on this tab.
func TestEscLeavesTheHostsSearch(t *testing.T) {
	m := sized(sample(), 100, 24)
	all := m.hosts.rowCount()

	m = pressA(m, "/")
	m = typeText(m, "prod")
	m = pressA(m, "esc")

	if m.hosts.filtering {
		t.Error("Esc should have left the search")
	}
	if n := m.hosts.rowCount(); n != all {
		t.Errorf("the table should be whole again: %d of %d", n, all)
	}
	// Backspace past the start does the same.
	m = pressA(m, "/")
	m = typeText(m, "p")
	m = pressA(m, "backspace", "backspace")
	if m.hosts.filtering {
		t.Error("backspacing past an empty query should leave the search")
	}
}

// A search that matches nothing says so — and does not say "no hosts yet",
// which is a different and wrong thing to say when there are five.
func TestHostsSearchWithNoMatchIsNotTheEmptyState(t *testing.T) {
	m := sized(sample(), 100, 24)
	m = pressA(m, "/")
	m = typeText(m, "zzzzzz")

	view := ansi.Strip(m.View())
	if !strings.Contains(view, "no match") {
		t.Error("a search with no results should say so")
	}
	if strings.Contains(view, "No hosts yet") {
		t.Error("that is the first-run state, and there are five hosts")
	}
}

// With no hosts there is nothing to search, so the letter goes away with the
// menu row — they are one declaration.
func TestSearchNeedsHostsToSearch(t *testing.T) {
	m := sized(nil, 100, 24)
	for _, it := range m.menuItems() {
		if it.key == "/" {
			t.Error("an empty table should not offer Search")
		}
	}
	if after := pressA(m, "/"); after.hosts.filtering {
		t.Error("/ started a search over nothing")
	}
}

// The panel carries no border title: a title tells panels APART, and this tab
// has one panel under a tab capsule that already reads "[1] hosts".
func TestHostsPanelHasNoTitle(t *testing.T) {
	view := ansi.Strip(sized(sample(), 100, 24).View())
	lines := strings.Split(view, "\n")
	// The panel's top border is the first line starting with the box corner.
	for _, l := range lines {
		if strings.HasPrefix(l, "╭") {
			if strings.Contains(l, "hosts") {
				t.Errorf("the panel still wears a title: %q", l)
			}
			return
		}
	}
	t.Error("no panel border found")
}
