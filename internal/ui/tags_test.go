package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/vulcanshen/sshu/internal/store"
)

func tagged(name string, port int, tags ...string) store.Host {
	return store.Host{Name: name, Host: "h.internal", Port: port, User: "deploy",
		Auth: store.AuthPassword, Password: "pw", Tags: tags}
}

// ------------------------------------------------------------------ the line

// An entry is two lines and the second one is the tags. Both halves are
// checked, because the row above kept working the whole time the line below
// was being built.
func TestAHostIsDrawnAsTwoLinesWithItsTagsBelow(t *testing.T) {
	c := computeCols(90)
	lines := renderHostRow(tagged("web-01", 22, "prod", "tokyo"), "deploy", c, false, 90)

	if len(lines) != 2 {
		t.Fatalf("want 2 lines, got %d", len(lines))
	}
	if got := ansi.Strip(lines[0]); !strings.Contains(got, "web-01") {
		t.Errorf("the first line is the host: %q", got)
	}
	second := ansi.Strip(lines[1])
	if !strings.Contains(second, "prod") || !strings.Contains(second, "tokyo") {
		t.Errorf("the second line is the tags: %q", second)
	}
	if strings.Contains(second, "web-01") {
		t.Errorf("the tag line must not repeat the host: %q", second)
	}
}

// No tags is a state with its own mark, not a blank. A blank second line made
// a tagged host look like it had GROWN a line rather than filled one in.
func TestAnUntaggedHostShowsThePlaceholder(t *testing.T) {
	c := computeCols(90)
	lines := renderHostRow(tagged("web-01", 22), "deploy", c, false, 90)
	second := ansi.Strip(lines[1])
	if !strings.Contains(second, tagNone) {
		t.Errorf("want the placeholder %q, got %q", tagNone, second)
	}
	if !strings.Contains(second, glyphTag) {
		t.Errorf("the tag glyph marks the line even when empty: %q", second)
	}
}

// The line is capped at the entry width and says so with the app's ellipsis
// rather than running into the panel border.
func TestALongTagLineIsCutWithAnEllipsis(t *testing.T) {
	many := []string{"alpha", "bravo", "charlie", "delta", "echo", "foxtrot",
		"golf", "hotel", "india", "juliett", "kilo", "lima", "mike"}
	got := tagLineText(many, 40)
	if dispW(got) > 40 {
		t.Errorf("want at most 40 cells, got %d: %q", dispW(got), got)
	}
	if !strings.Contains(got, "…") {
		t.Errorf("a cut line must show it was cut: %q", got)
	}
	// ...and a line that fits is not touched.
	short := tagLineText([]string{"prod"}, 40)
	if strings.Contains(short, "…") {
		t.Errorf("a line that fits must not be marked as cut: %q", short)
	}
}

// ---------------------------------------------------------------- the colour

// Name, user and host are one thing — which machine is this — so they share
// one tone. An earlier draft ranked them (host brighter than user), which
// asserted an order that is not real.
func TestNameUserAndHostShareOneTone(t *testing.T) {
	withColour(t)
	c := computeCols(90)
	line := renderHostRow(tagged("web-01", 22), "deploy", c, false, 90)[0]

	if n := strings.Count(line, ansiOf(t, textColor)); n != 3 {
		t.Errorf("name, user and host should each carry the text tone (3), got %d in %q",
			n, line)
	}
}

// The one colour on an unselected row, and it marks the EXCEPTION. 22 is what
// everything is; a list where every row has a coloured port is a list where
// the colour says nothing.
func TestOnlyANonDefaultPortIsColoured(t *testing.T) {
	withColour(t)
	c := computeCols(90)
	peach := ansiOf(t, peachColor)

	std := renderHostRow(tagged("web-01", 22), "deploy", c, false, 90)[0]
	if strings.Contains(std, peach) {
		t.Errorf("port 22 is the rule and must not be marked: %q", std)
	}
	odd := renderHostRow(tagged("db-01", 5432), "deploy", c, false, 90)[0]
	if !strings.Contains(odd, peach) {
		t.Errorf("port 5432 is the exception and should stand out: %q", odd)
	}
}

// Auth is deliberately NOT colour-encoded (§B). Every row has one, so a colour
// there would mark every row — and it would compete with the port for the one
// signal an unselected row is allowed to raise.
func TestAuthIsNotColourEncoded(t *testing.T) {
	withColour(t)
	c := computeCols(90)

	pw := tagged("a", 22)
	key := store.Host{Name: "b", Host: "h", Port: 22, User: "u",
		Auth: store.AuthPrivateKey, IdentityFile: "~/.ssh/id_ed25519"}

	// The auth cell is the last one; whatever tone it wears must be the same
	// for both methods, so the difference between them is carried by the glyph.
	tones := func(h store.Host) string {
		line := renderHostRow(h, "u", c, false, 90)[0]
		// Everything after the host column is where auth lives.
		return line[strings.LastIndex(line, "\x1b["):]
	}
	if tones(pw) != tones(key) {
		t.Errorf("password and privatekey rows end in different tones:\n pw %q\nkey %q",
			tones(pw), tones(key))
	}
	if !strings.Contains(ansi.Strip(renderHostRow(pw, "u", c, false, 90)[0]), glyphLock) {
		t.Error("password should be marked by its glyph")
	}
	if !strings.Contains(ansi.Strip(renderHostRow(key, "u", c, false, 90)[0]), glyphKey) {
		t.Error("privatekey should be marked by its glyph")
	}
}

// The selected entry drops every per-column tone. The bar answers "you are
// here" and the tones answer "what kind of value is this"; both on one row
// means the bar wins anyway and the tones only muddy it.
func TestTheSelectedEntryCarriesNoColumnColours(t *testing.T) {
	withColour(t)
	c := computeCols(90)
	lines := renderHostRow(tagged("db-01", 5432, "prod"), "deploy", c, true, 90)

	whole := strings.Join(lines, "\n")
	if strings.Contains(whole, ansiOf(t, peachColor)) {
		t.Errorf("a selected row must not keep the port colour: %q", whole)
	}
	// Both lines wear the bar, or the entry looks torn in half.
	for i, l := range lines {
		if !strings.Contains(l, ansiBgOf(t, rowSelColor)) {
			t.Errorf("line %d of a selected entry is missing the bar: %q", i, l)
		}
	}
}

// ------------------------------------------------------------------ the list

// An entry costs two lines, so a panel shows half as many hosts as it has
// lines for them. Everything downstream counts entries, and this is the only
// place the height is divided out.
func TestVisibleRowsCountsEntriesNotLines(t *testing.T) {
	m := hostsModel{hosts: make([]store.Host, 40), w: 80, h: 2 + headerRows + 10}
	if got := m.visibleRows(); got != 5 {
		t.Errorf("10 lines of room is 5 entries, got %d", got)
	}
}

// A host whose tag line would fall off the bottom is not drawn as a headless
// row. The odd line is left blank instead.
func TestAHostEntryIsDrawnWholeOrNotAtAll(t *testing.T) {
	hosts := []store.Host{tagged("aaa", 22), tagged("bbb", 22), tagged("ccc", 22)}
	m := hostsModel{hosts: hosts, w: 80, h: 40}

	// 6 inner lines: 1 header + room for 2 whole entries, with one line spare
	// that the third entry cannot fit into.
	body := m.tableBody(78, 6)
	if len(body) != 1+2*hostRowLines {
		t.Fatalf("want header + 2 whole entries = %d lines, got %d:\n%s",
			1+2*hostRowLines, len(body), strings.Join(body, "\n"))
	}
	flat := ansi.Strip(strings.Join(body, "\n"))
	if strings.Contains(flat, "ccc") {
		t.Errorf("the third host had no room and must be left out entirely:\n%s", flat)
	}
}

// ---------------------------------------------------------------- the search

// A tag is a word the user chose to group hosts by, so pulling the group up
// with one query is what it was written for. A tag you cannot search is
// decoration.
func TestSearchFindsHostsByTag(t *testing.T) {
	m := hostsModel{w: 80, h: 40, hosts: []store.Host{
		tagged("web-01", 22, "prod", "frontend"),
		tagged("web-02", 22, "staging"),
		tagged("db-01", 5432, "prod", "db"),
	}}
	m.startFilter()
	m.query = "prod"
	m.refilter()

	if len(m.matches) != 2 {
		t.Fatalf("want the two prod hosts, got %d: %v", len(m.matches), m.matches)
	}
	for _, i := range m.matches {
		if !strings.Contains(strings.Join(m.hosts[i].Tags, " "), "prod") {
			t.Errorf("%q matched without the tag", m.hosts[i].Name)
		}
	}
}

// The query folds case; the stored tag does not. Folding at the query end is
// what lets the user keep the capitals they typed.
func TestSearchByTagIgnoresCase(t *testing.T) {
	m := hostsModel{w: 80, h: 40, hosts: []store.Host{tagged("web-01", 22, "Prod")}}
	m.startFilter()
	m.query = "prod"
	m.refilter()
	if len(m.matches) != 1 {
		t.Errorf("a lowercase query should find an uppercase tag, got %d matches", len(m.matches))
	}
}

// ------------------------------------------------------------------ the form

// Tags is the first row that may be left blank. Every other enabled text row
// is required, which is what lets complete() derive the needed set from
// enabled() — so this is the case that would break if the two were folded back
// together.
func TestAFormWithNoTagsIsStillComplete(t *testing.T) {
	f := newHostForm()
	f.openCreate(0)
	f.fields[fName].value = "web-01"
	f.fields[fHost].value = "10.0.0.1"
	f.fields[fUser].value = "deploy"
	f.fields[fIdentity].value = "~/.ssh/id_ed25519"

	if !f.complete() {
		t.Error("a host with no tags is a finished form, not an unfinished one")
	}
	// ...and a required row still holds it back, or the flag has swallowed too
	// much.
	f.fields[fName].value = ""
	if f.complete() {
		t.Error("an empty Name must still make the form incomplete")
	}
}

func TestTagsRoundTripThroughTheForm(t *testing.T) {
	h := tagged("web-01", 22, "prod", "k8s:prod")
	f := newHostForm()
	f.openEdit(h, 0)

	if got := f.fields[fTags].value; got != "prod k8s:prod" {
		t.Errorf("the form should show the tags space separated, got %q", got)
	}
	// Typing more, including the repeats and padding a human produces.
	f.fields[fTags].value = "  prod   tokyo prod "
	got := f.host().Tags
	if len(got) != 2 || got[0] != "prod" || got[1] != "tokyo" {
		t.Errorf("want [prod tokyo] cleaned out of what was typed, got %q", got)
	}
}

// ---------------------------------------------------------------- the picker

func TestTheCredentialPickerSaysWhichKey(t *testing.T) {
	c := store.Credential{Name: "deploy", User: "deploy", Auth: store.AuthPrivateKey,
		IdentityFile: "~/.ssh/id_ed25519"}
	if got := credValueHint(c, 40); got != "~/.ssh/id_ed25519" {
		t.Errorf("a key credential should name its file, got %q", got)
	}
}

// Cut from the FRONT: the end of a path is the half that identifies the key,
// and every key in ~/.ssh shares the beginning.
func TestALongKeyPathKeepsItsFilename(t *testing.T) {
	c := store.Credential{Name: "ci", Auth: store.AuthPrivateKey,
		IdentityFile: "~/.ssh/keys/very/deep/tree/production-deploy-2026"}
	got := credValueHint(c, 24)
	if dispW(got) > 24 {
		t.Errorf("want at most 24 cells, got %d: %q", dispW(got), got)
	}
	if !strings.HasSuffix(got, "production-deploy-2026") {
		t.Errorf("the filename must survive the cut, got %q", got)
	}
}

// The password is never shown, and neither is its length — a fixed mask, the
// same constant preference → credentials uses.
func TestThePickerNeverShowsAPasswordOrItsLength(t *testing.T) {
	short := store.Credential{Name: "a", Auth: store.AuthPassword, Password: "x"}
	long := store.Credential{Name: "b", Auth: store.AuthPassword,
		Password: "correct-horse-battery-staple"}

	for _, c := range []store.Credential{short, long} {
		got := credValueHint(c, 40)
		if strings.Contains(got, c.Password) {
			t.Fatalf("the password leaked into the picker: %q", got)
		}
	}
	if credValueHint(short, 40) != credValueHint(long, 40) {
		t.Error("the mask must not vary with the password's length")
	}
}

// A password credential with no password is broken. The list is a better place
// to find that out than the next failed connection.
func TestThePickerCallsOutAMissingSecret(t *testing.T) {
	noPw := store.Credential{Name: "a", Auth: store.AuthPassword}
	if got := credValueHint(noPw, 40); got != "(not set)" {
		t.Errorf("want (not set), got %q", got)
	}
	noKey := store.Credential{Name: "b", Auth: store.AuthPrivateKey}
	if got := credValueHint(noKey, 40); got != "(no key file)" {
		t.Errorf("want (no key file), got %q", got)
	}
}
