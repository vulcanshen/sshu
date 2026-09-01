package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// withColour forces a true-colour profile so a test can see what the styles
// actually emit. Without it lipgloss renders plain text under `go test` and every
// colour assertion would pass for the wrong reason.
func withColour(t *testing.T) {
	t.Helper()
	old := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(old) })
}

// §B in executable form: the row you are EDITING is lavender, and it is not the
// same colour as the cursor on a row you would ACT on. Colour assignment is the
// one design rule here that nothing else can catch drifting.
func TestFocusedFormRowIsLavender(t *testing.T) {
	withColour(t)
	fixtureKeys(t)

	// Pin the value, not just the variable. Deriving the expected sequence from
	// editColor alone would make this test agree with whatever the code says —
	// it would survive changing lavender to anything at all.
	const lavenderHex = "#b4befe"
	if string(editColor) != lavenderHex {
		t.Fatalf("the edited form row must be lavender %s, got %s", lavenderHex, editColor)
	}

	lavender := ansiOf(t, editColor)
	hand := ansiOf(t, handColor)
	if lavender == hand {
		t.Fatal("the fixture is broken: the two colours must differ")
	}

	m := pressA(appWith(sample(), nil), "A")
	m.form.focus = fUser
	rows := strings.Split(m.form.view(), "\n")

	var userRow, hostRow string
	for _, r := range rows {
		if strings.Contains(r, "User") {
			userRow = r
		}
		if strings.Contains(r, "Host") {
			hostRow = r
		}
	}
	if userRow == "" || hostRow == "" {
		t.Fatal("could not find the User / Host rows")
	}
	if !strings.Contains(userRow, lavender) {
		t.Error("the focused row should be lavender")
	}
	if strings.Contains(hostRow, lavender) {
		t.Error("an unfocused row must not borrow the focus colour")
	}
	if strings.Contains(userRow, hand) {
		t.Error("the focused form row must not also carry the list-cursor colour (§B)")
	}
}

// The list cursor keeps handColor — the two roles stay apart.
func TestListCursorIsNotLavender(t *testing.T) {
	withColour(t)
	m := pressA(appWith(sample(), nil), " ") // Space menu, cursor on the first row
	got := m.spaceMenu.view()
	if !strings.Contains(got, ansiOf(t, handColor)) {
		t.Error("the menu cursor should still be handColor")
	}
	if strings.Contains(got, ansiOf(t, editColor)) {
		t.Error("lavender belongs to the edited form row, not to a list cursor (§B)")
	}
}

// ansiOf returns just the SGR PARAMETERS lipgloss emits for c as a foreground —
// not the whole escape prefix. A real row often sets foreground and background
// in one sequence, and a prefix match would miss the colour that is plainly
// there.
func ansiOf(t *testing.T, c lipgloss.Color) string {
	t.Helper()
	return sgrParams(t, lipgloss.NewStyle().Foreground(c).Render("x"), c)
}

// ansiBgOf is ansiOf for a background, which is how a cursor bar paints.
func ansiBgOf(t *testing.T, c lipgloss.Color) string {
	t.Helper()
	return sgrParams(t, lipgloss.NewStyle().Background(c).Render("x"), c)
}

func sgrParams(t *testing.T, rendered string, c lipgloss.Color) string {
	t.Helper()
	start := strings.Index(rendered, "\x1b[")
	end := strings.Index(rendered, "m")
	if start < 0 || end <= start+2 {
		t.Fatalf("no colour in output for %q — is the profile forced?", string(c))
	}
	return rendered[start+2 : end]
}

// Selection has to be unmistakable. The first version of this list changed only
// a border, and in a dense list that reads as nothing happening.
func TestSelectedHostRowIsBlue(t *testing.T) {
	withColour(t)
	h := sample()[0]
	c := computeCols(80)

	const blueHex = "#89b4fa"
	if string(rowSelColor) != blueHex {
		t.Fatalf("the selected row should be blue %s, got %s", blueHex, rowSelColor)
	}

	sel := renderHostRow(h, h.User, c, true, 80)
	un := renderHostRow(h, h.User, c, false, 80)
	if sel == un {
		t.Fatal("a selected row renders identically to an unselected one")
	}
	// The bar paints blue as a BACKGROUND, so a foreground probe would miss it.
	if !strings.Contains(sel, ansiBgOf(t, rowSelColor)) {
		t.Error("the selected row should carry the selection colour")
	}
	if strings.Contains(un, ansiBgOf(t, rowSelColor)) {
		t.Error("an unselected row must not carry the selection colour")
	}
	// Unselected: the name stays readable, the detail columns recede.
	if !strings.Contains(un, ansiOf(t, textColor)) || !strings.Contains(un, ansiOf(t, dimColor)) {
		t.Error("an unselected row should be a bright name over dim detail")
	}
}

// The [Alt] lead never goes out: it is half of every chord, so the strip
// keeps it on the lit fill no matter which tab is active.
func TestTheAltLeadIsAlwaysLit(t *testing.T) {
	withColour(t)
	bg := ansiBgOf(t, focusColor)
	for _, key := range []string{"alt+P", "alt+F", "alt+S"} {
		m := pressA(sized(sample(), 100, 26), key)
		row := strings.Split(m.View(), "\n")[0]
		at := strings.Index(row, "[Alt]")
		if at < 0 {
			t.Fatalf("%s: no [Alt] lead in the strip", key)
		}
		// The style run covering the lead opens at the last escape before it.
		open := strings.LastIndex(row[:at], "\x1b[")
		if open < 0 || !strings.Contains(row[open:at], bg) {
			t.Errorf("%s: the [Alt] lead must sit on the lit fill", key)
		}
	}
}
