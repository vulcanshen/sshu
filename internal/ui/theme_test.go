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

// A panel frame carries focus, and that is the whole job of the colour: the
// blue frame is where your keystrokes are going. Nothing tested it before —
// breaking panelChrome's focus so every panel drew dim left the suite green.
func TestAPanelFrameCarriesFocus(t *testing.T) {
	withColour(t)
	body := []string{strings.Repeat(" ", 20)}

	for _, tc := range []struct {
		name    string
		focused bool
		want    lipgloss.Color
	}{
		{"focused", true, focusColor},
		{"not focused", false, borderDim},
	} {
		frames := frameSGRs(t, panelChrome(20, body, "title", tc.focused))
		if len(frames) != 1 {
			t.Fatalf("%s: expected one frame, got %d", tc.name, len(frames))
		}
		if frames[0] != ansiOf(t, tc.want) {
			t.Errorf("%s: frame is not %s", tc.name, string(tc.want))
		}
	}
}

// The title capsule sits ON the frame, so it wears the frame's colour — in all
// three states. A chip that stayed dim under a lit frame would read as a label
// that belongs to something else.
func TestTheTitleChipFollowsTheFrame(t *testing.T) {
	withColour(t)
	body := []string{strings.Repeat(" ", 20)}

	for _, tc := range []struct {
		name string
		tone borderTone
		want lipgloss.Color
	}{
		{"idle", toneIdle, borderDim},
		{"echo", toneEcho, handColor},
		{"focus", toneFocus, focusColor},
	} {
		out := panelChromeTone(20, body, "title", tc.tone)
		frames := frameSGRs(t, out)
		if len(frames) != 1 || frames[0] != ansiOf(t, tc.want) {
			t.Fatalf("%s: the frame itself is wrong", tc.name)
		}
		// The chip body paints the same colour as a BACKGROUND.
		if !strings.Contains(out, ansiBgOf(t, tc.want)) {
			t.Errorf("%s: the title chip does not sit on the frame's colour", tc.name)
		}
	}
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

// Exactly the tab you are on is lit. There used to be a second permanently lit
// segment — the [Alt] chord lead — and it went out with the chords: a lit thing
// that names no surface only competes with the one that does.
func TestOnlyTheActiveTabIsLit(t *testing.T) {
	withColour(t)
	bg := ansiBgOf(t, focusColor)
	for i, key := range []string{"M", "F", "S"} {
		m := pressA(sized(sample(), 100, 26), key)
		row := strings.Split(m.View(), "\n")[0]
		if strings.Contains(row, "[Alt]") {
			t.Fatalf("%s: the chord lead should be gone from the strip", key)
		}
		for j, label := range tabLabels {
			at := strings.Index(row, label)
			if at < 0 {
				t.Fatalf("%s: %q missing from the strip", key, label)
			}
			// The style run covering the label opens at the last escape before it.
			open := strings.LastIndex(row[:at], "\x1b[")
			if open < 0 {
				t.Fatalf("%s: %q carries no style", key, label)
			}
			lit := strings.Contains(row[open:at], bg)
			if want := i == j; lit != want {
				t.Errorf("%s: %q lit = %v, want %v", key, label, lit, want)
			}
		}
	}
}
