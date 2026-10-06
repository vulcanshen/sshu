package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/vulcanshen/sshu/internal/store"
)

// The input survey round (2026-10-06), one rule for every single-line value in
// the terminu family: a pasted line break or tab stays in the value, drawn as a
// Red \n / \t two cells wide; any other control character is dropped; a value
// that gets used is refused while it holds one, before any trim. Each test here
// failed against the sshu that let a paste split a field's row in two.

// paste is a bracketed paste: one KeyRunes carrying everything that was pasted,
// line breaks, tabs and escapes included.
func paste(m AppModel, s string) AppModel {
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s), Paste: true})
	return settle(next.(AppModel))
}

func pasteMsg(s string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s), Paste: true}
}

// redSGR is how Red #f38ba8 opens, pinned rather than derived from warnColor.
const redSGR = "\x1b[38;2;243;139;168m"

// ------------------------------------------------------------ what is kept

func TestALineBreakOrTabStaysAndOtherControlsGo(t *testing.T) {
	got := singleLine("a\r\nb\rc\nd\te\x1b[1mf\x7fg\u0085h\x00i")
	if want := "a\nb\rc\nd\te[1mfghi"; got != want {
		t.Errorf("singleLine = %q, want %q", got, want)
	}
}

// Every place that takes typed runes filters them. Six entries; taking the
// filter out of any one of them leaves an ESC in its value.
func TestEveryTextEntryFiltersAPaste(t *testing.T) {
	const in, want = "x\r\ny\x1bz", "x\nyz"

	f := formField{label: "Name"}
	editField(&f, pasteMsg(in))
	if f.value != want {
		t.Errorf("form field: %q", f.value)
	}

	ip := inputPopup{anim: settledAnim("input")}
	ip.update(pasteMsg(in))
	if ip.value != want {
		t.Errorf("input box: %q", ip.value)
	}

	ap := askpassPopup{anim: settledAnim("askpass"), req: &askpassRequest{prompt: "Password: "}}
	ap.update(pasteMsg(in))
	if ap.value != want {
		t.Errorf("askpass: %q", ap.value)
	}

	pk := filePicker{anim: settledAnim("picker")}
	pk.update(pasteMsg(in))
	if pk.query != want {
		t.Errorf("file picker: %q", pk.query)
	}

	hs := hostsModel{filtering: true}
	hs.filterKey(pasteMsg(in))
	if hs.query != want {
		t.Errorf("hosts search: %q", hs.query)
	}

	ss := sftpSideModel{filtering: true}
	ss.filterKey(pasteMsg(in))
	if ss.query != want {
		t.Errorf("file transfer search: %q", ss.query)
	}
}

// settledAnim is a popup animator that has finished opening, so update takes
// keys.
func settledAnim(target string) popupAnimator {
	a := newPopupAnimator(target)
	a.open()
	for i := 0; i <= animFrames; i++ {
		a.tick(AnimTickMsg{Target: target})
	}
	return a
}

// A pasted \r\n is ONE line break: drawn as one \n, deleted by one Backspace.
// 12\r\n34 tells it apart from keeping both characters, where 12\n34 would not.
func TestAPastedCRLFIsOneBreakAndOneBackspace(t *testing.T) {
	m := pressA(appWith(sample(), nil), "A")
	m = paste(m, "12\r\n34")
	if got := m.form.fields[fName].value; got != "12\n34" {
		t.Fatalf("value %q, want the \\r\\n kept as one \\n", got)
	}
	m = pressA(m, "backspace", "backspace", "backspace")
	if got := m.form.fields[fName].value; got != "12" {
		t.Errorf("three Backspaces should take 4, 3 and the break: %q", got)
	}
}

// A digits-only field still takes digits only.
func TestADigitsFieldStillTakesOnlyDigits(t *testing.T) {
	m := pressA(appWith(sample(), nil), "A")
	m.form.focus, m.form.fields[fPort].value, m.form.fields[fPort].caret = fPort, "", 0
	m = paste(m, "8\n0\t8")
	if got := m.form.fields[fPort].value; got != "808" {
		t.Errorf("Port = %q", got)
	}
}

// ---------------------------------------------------------- prefilled values

// A value the form opens with goes through the same filter: an ESC from a
// hand-edited file is dropped before it can reach the terminal, a tab stays to
// be seen and refused.
func TestTheHostFormFiltersWhatItOpensWith(t *testing.T) {
	h := store.Host{Name: "we\x1bb\t", Host: "h\x1bost\t", Port: 22, User: "ro\x1bot\t",
		Auth: store.AuthPassword, Credential: "c\x1br\t", IdentityFile: "~/i\x1bd\t",
		Password: "p\x1bw\t", Tags: []string{"pr\x1bod"}}
	var f hostForm
	f.openEdit(h, 1)
	for _, i := range []int{fName, fHost, fUser, fCredential, fIdentity, fPassword} {
		v := f.fields[i].value
		if strings.Contains(v, "\x1b") || !strings.HasSuffix(v, "\t") {
			t.Errorf("%s = %q: the ESC should go and the tab stay", f.fields[i].label, v)
		}
		if f.fields[i].caret != len([]rune(v)) {
			t.Errorf("%s: the caret should sit at the end", f.fields[i].label)
		}
	}
	if v := f.fields[fTags].value; strings.Contains(v, "\x1b") {
		t.Errorf("Tags = %q", v)
	}
}

func TestTheCredentialFormFiltersWhatItOpensWith(t *testing.T) {
	c := store.Credential{Name: "o\x1bps\t", User: "ro\x1bot\t", Auth: store.AuthPassword,
		IdentityFile: "~/i\x1bd\t", Password: "p\x1bw\t"}
	var f credForm
	f.openEdit(c, 1)
	for _, i := range []int{cName, cUser, cIdentity, cPassword} {
		v := f.fields[i].value
		if strings.Contains(v, "\x1b") || !strings.HasSuffix(v, "\t") {
			t.Errorf("%s = %q: the ESC should go and the tab stay", f.fields[i].label, v)
		}
		if f.fields[i].caret != len([]rune(v)) {
			t.Errorf("%s: the caret should sit at the end", f.fields[i].label)
		}
	}
}

func TestTheHostBlockFormFiltersWhatItOpensWith(t *testing.T) {
	b := store.SSHBlock{Patterns: "pr\x1bod", Options: []store.SSHOption{
		{Key: "HostName", Value: "a\x1bb"}, {Key: "SetEnv", Value: "X=\x1b1"}}}
	var f sshcfgForm
	f.openEdit(b, 0, 1)
	for i, want := range map[int]string{sfHost: "prod", sfHostName: "ab", sfFixedCount: "X=1"} {
		if got := f.fields[i].value; got != want {
			t.Errorf("%s = %q, want %q", f.fields[i].label, got, want)
		}
		if f.fields[i].caret != len([]rune(want)) {
			t.Errorf("%s: the caret should sit at the end", f.fields[i].label)
		}
	}
}

// Rename opens on the remote name, and a remote name can hold anything. macOS's
// Icon\r keeps its line break — shown as \n, and refused as it stands rather
// than trimmed into "Icon" — and an ESC in a name is dropped.
func TestRenameShowsAndRefusesALineBreakInTheName(t *testing.T) {
	m := sftpFixture(t, 100, 26)
	m.sftp.focus = panelLeftFiles
	cwd := m.sftp.sides[sideLeft].cwd
	if err := os.WriteFile(filepath.Join(cwd, "Icon\r"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	m.sftp.cur().reload()
	m = cursorOnName(t, m, "Icon\r")

	m = pressA(m, "r")
	if m.input.value != "Icon\r" {
		t.Fatalf("the box should start from the name, line break kept: %q", m.input.value)
	}
	if !strings.Contains(ansi.Strip(m.input.view()), `Icon\n`) {
		t.Errorf("the line break should show as \\n:\n%s", ansi.Strip(m.input.view()))
	}
	m = pressA(m, "enter")
	if !m.input.isActive() || m.input.err != "A name cannot have line breaks or tabs" {
		t.Fatalf("Enter should be refused in the box, err=%q", m.input.err)
	}
	if _, err := os.Stat(filepath.Join(cwd, "Icon")); err == nil {
		t.Error("the trim must not have renamed it to Icon")
	}
}

func TestRenameDropsAnEscapeFromTheName(t *testing.T) {
	m := sftpFixture(t, 100, 26)
	m.sftp.focus = panelLeftFiles
	cwd := m.sftp.sides[sideLeft].cwd
	if err := os.WriteFile(filepath.Join(cwd, "e\x1b[2Jsc"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	m.sftp.cur().reload()
	m = pressA(cursorOnName(t, m, "e\x1b[2Jsc"), "r")
	if m.input.value != "e[2Jsc" {
		t.Errorf("the ESC should be dropped from the box: %q", m.input.value)
	}
}

func cursorOnName(t *testing.T, m AppModel, name string) AppModel {
	t.Helper()
	s := m.sftp.cur()
	for i := 0; i < s.rowCount(); i++ {
		if e, ok := s.rowAt(i); ok && e.Name == name {
			s.cursor = i
			return m
		}
	}
	t.Fatalf("no row named %q", name)
	return m
}

// The pickers hand in a value from somewhere else too: a path off the disk, a
// credential name out of credentials.yaml. All three forms that take a path.
func TestAPickedPathIsFiltered(t *testing.T) {
	dir := fixtureKeys(t)
	if err := os.WriteFile(filepath.Join(dir, "id_\x1bzz"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	pick := func(m AppModel, layer int) AppModel {
		m = settle(m)
		m.picker.open(identityRoot(), layer)
		m = typeText(settle(m), "zz")
		return pressA(m, "enter")
	}

	m := pressA(appWith(sample(), nil), "A")
	if m = pick(m, 2); strings.Contains(m.form.fields[fIdentity].value, "\x1b") ||
		!strings.HasSuffix(m.form.fields[fIdentity].value, "id_zz") {
		t.Errorf("host form: %q", m.form.fields[fIdentity].value)
	}

	m = appWith(sample(), nil)
	m.credFormUI.openCreate(1)
	if m = pick(m, 2); !strings.HasSuffix(m.credFormUI.fields[cIdentity].value, "id_zz") {
		t.Errorf("credential form: %q", m.credFormUI.fields[cIdentity].value)
	}

	m = appWith(sample(), nil)
	m.sshcfgFormUI.openCreate(1)
	if m = pick(m, 2); !strings.HasSuffix(m.sshcfgFormUI.fields[sfIdentity].value, "id_zz") {
		t.Errorf("Host block form: %q", m.sshcfgFormUI.fields[sfIdentity].value)
	}
}

func TestAPickedCredentialIsFiltered(t *testing.T) {
	m := pressA(credApp(sample(), []store.Credential{
		{Name: "o\x1bps", User: "root", Auth: store.AuthPassword, Password: "pw"}}), "A")
	m.form.focus = fAuth
	m = pressA(m, "right") // credential
	m.form.focus = fCredential
	m = pressA(m, "enter", "enter")
	if got := m.form.fields[fCredential].value; got != "ops" {
		t.Errorf("Credential = %q", got)
	}
}

// The Export page opens on the directory sshu was started in.
func TestTheExportDirectoryIsFiltered(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "a\x1bb")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	if v := newExportPage().fields[0].value; strings.Contains(v, "\x1b") || !strings.HasSuffix(v, "ab") {
		t.Errorf("Directory = %q", v)
	}
}

// ------------------------------------------------------------------- drawing

// \n and \t are drawn, Red, two cells; a \ and an n typed by hand are not.
func TestAFieldDrawsALineBreakAsARedEscape(t *testing.T) {
	withColour(t)
	for _, focused := range []bool{false, true} {
		f := formField{value: "a\nb\tc" + `\n`}
		f.caret = len([]rune(f.value))
		got := renderTextValue(f, focused, 20)
		if plain := ansi.Strip(got); !strings.HasPrefix(plain, `a\nb\tc\n`) || dispW(got) != 20 {
			t.Errorf("focused=%v: %q (%d cells)", focused, plain, dispW(got))
		}
		if n := strings.Count(got, redSGR); n != 2 {
			t.Errorf("focused=%v: the two escapes, and only they, are Red: %d in %q", focused, n, got)
		}
	}
}

// A masked value shows one bullet per rune, a line break among them — the
// error row says it is there.
func TestAMaskedValueDoesNotShowItsLineBreak(t *testing.T) {
	f := formField{value: "p\nw", mask: true}
	if got := ansi.Strip(renderTextValue(f, false, 10)); got != "•••       " {
		t.Errorf("%q", got)
	}
}

// \n is never cut in half: where the slot ends inside one, it is left out whole.
func TestAnEscapeIsNeverCut(t *testing.T) {
	cases := []struct {
		value   string
		caret   int
		focused bool
		w       int
		want    string
	}{
		{"abc\nd", 0, false, 5, "abc …"},     // cut at the end, \n would straddle the …
		{"abcd\nx", 0, true, 6, `abcd\n`},    // the tail ends exactly on it
		{"abcd\nx", 0, true, 5, "abcd "},     // the tail has room for half of it
		{"abcdef\n", 7, true, 6, `ef\n  `},   // scrolled: \n moves the window two
		{"a\nbcdefgh", 0, true, 6, `a\nbcd`}, // caret on a, tail by whole units
		{"ab\ncd", 2, true, 8, `ab\ncd  `},   // the caret sits on the \n itself
	}
	for _, c := range cases {
		f := formField{value: c.value, caret: c.caret}
		got := renderTextValue(f, c.focused, c.w)
		if plain := ansi.Strip(got); plain != c.want {
			t.Errorf("%q caret %d in %d: %q, want %q", c.value, c.caret, c.w, plain, c.want)
		}
	}
}

func TestTheInputBoxDrawsALineBreak(t *testing.T) {
	withColour(t)
	p := inputPopup{anim: settledAnim("input"), value: "a\nb", screenW: 100}
	if v := p.view(); !strings.Contains(ansi.Strip(v), `a\nb`) || !strings.Contains(v, redSGR+`\n`) {
		t.Errorf("the box should show a Red \\n:\n%s", ansi.Strip(v))
	}
}

func TestThePickerQueryDrawsALineBreak(t *testing.T) {
	withColour(t)
	fixtureKeys(t)
	var pk filePicker
	pk.open(identityRoot(), 1)
	pk.anim = settledAnim("picker")
	pk.screenW, pk.screenH = 100, 30
	pk.query = "a\tb"
	if v := pk.view(); !strings.Contains(ansi.Strip(v), `a\tb`) || !strings.Contains(v, redSGR+`\t`) {
		t.Errorf("the query should show a Red \\t:\n%s", ansi.Strip(v))
	}
}

// A search row being typed into draws its \n Red; one nobody is typing into is
// grey whole — one span, \n included — with no caret (tdp defaults F1).
func TestASearchRowIsGreyWholeWhenNotTypedInto(t *testing.T) {
	withColour(t)
	typing := queryRow(glyphSearch+" a\nb", "1 of 3", 30, true)
	if !strings.Contains(typing, redSGR+`\n`) || dispW(typing) != 30 {
		t.Errorf("typed into: %q", typing)
	}
	idle := queryRow(glyphSearch+" a\nb", "1 of 3", 30, false)
	if plain := ansi.Strip(idle); !strings.Contains(plain, `a\nb`) || !strings.Contains(plain, "1 of 3") {
		t.Errorf("the query and the count stay: %q", plain)
	}
	if n := strings.Count(idle, "\x1b["); n != 2 || !strings.HasPrefix(idle, sgr(ansiOf(t, dimColor))) {
		t.Errorf("the idle row should be one Overlay0 span, no caret: %q", idle)
	}
	if dispW(idle) != 30 {
		t.Errorf("the idle row is %d cells", dispW(idle))
	}
}

// ------------------------------------------------------------------ refusing

// Every text row of the host form refuses a line break or tab — at the end of
// the value too, where the save's trim would have dropped it unseen and saved.
func TestTheHostFormRefusesALineBreakInEveryRow(t *testing.T) {
	for _, c := range []struct {
		field, auth int
	}{
		{fName, 1}, {fHost, 1}, {fUser, 1}, {fIdentity, 1}, {fTags, 1}, {fPassword, 0},
	} {
		var saved []store.Host
		m := fillHostForm(pressA(appWith(sample(), &saved), "A"), "box")
		m.form.fields[fTags].value = "web"
		m.form.fields[fTags].caret = 3
		m.form.fields[fAuth].sel = c.auth
		m.form.focus = c.field
		m = paste(m, "\n")
		m.form.focus = fName
		m = pressA(m, "enter")
		label := m.form.fields[c.field].label
		if saved != nil {
			t.Errorf("%s: a value with a line break must not be saved", label)
		}
		if m.form.focus != c.field || m.form.errIdx != c.field ||
			m.form.err != label+" cannot have line breaks or tabs" {
			t.Errorf("%s: focus=%d errIdx=%d err=%q", label, m.form.focus, m.form.errIdx, m.form.err)
		}
		// Re-checked on every key after the first submit, as before. (On the
		// IdentityFile row Backspace clears the whole pick, by design.)
		m.form.focus = c.field
		m = pressA(m, "backspace")
		if m.form.err != "" && c.field != fIdentity {
			t.Errorf("%s: taking the break out should clear the error, got %q", label, m.form.err)
		}
	}
}

// A Name that is only a pasted line break is not a missing Name: the error
// says what is in it.
func TestALineBreakAloneIsNotReportedAsMissing(t *testing.T) {
	m := fillHostForm(pressA(appWith(sample(), nil), "A"), "")
	m.form.focus = fName
	m = paste(m, "\n")
	m = pressA(m, "enter")
	if m.form.err != "Name cannot have line breaks or tabs" {
		t.Errorf("err=%q", m.form.err)
	}
}

// A row Auth has switched off is not saved, so what is in it does not matter.
func TestADisabledRowsLineBreakDoesNotBlockTheSave(t *testing.T) {
	var saved []store.Host
	m := fillHostForm(pressA(appWith(sample(), &saved), "A"), "box")
	m.form.fields[fPassword].value = "pw\n" // privatekey: Password is dark
	m = pressA(m, "enter")
	if saved == nil {
		t.Errorf("the save should go through, err=%q", m.form.err)
	}
}

func TestTheCredentialFormRefusesALineBreak(t *testing.T) {
	m := credApp(nil, nil)
	m.credFormUI.openCreate(1)
	m = settle(m)
	m = fillCredForm(m, "ops")
	m.credFormUI.focus = cUser
	m = paste(m, "\t")
	next, _ := m.commitCredForm()
	m = next.(AppModel)
	if !m.credFormUI.isActive() || m.credFormUI.focus != cUser ||
		m.credFormUI.err != "User cannot have line breaks or tabs" {
		t.Errorf("focus=%d err=%q", m.credFormUI.focus, m.credFormUI.err)
	}
}

func TestACredentialsDisabledRowDoesNotBlockTheSave(t *testing.T) {
	m := credApp(nil, nil)
	m.credFormUI.openCreate(1)
	m = fillCredForm(settle(m), "ops")
	m.credFormUI.fields[cPassword].value = "pw\n" // privatekey: Password is dark
	next, _ := m.commitCredForm()
	if m = next.(AppModel); m.credFormUI.err != "" || len(m.creds.creds) != 1 {
		t.Errorf("the save should go through, err=%q", m.credFormUI.err)
	}
}

func TestTheHostBlockFormRefusesALineBreak(t *testing.T) {
	m, p := cfgApp(t, cfgFixture)
	before, _ := os.ReadFile(p)
	m = pressA(m, "A")
	m = typeText(m, "box")
	m.sshcfgFormUI.focus = sfHostName
	m = paste(m, "a\nb")
	m = pressA(m, "enter")
	if after, _ := os.ReadFile(p); string(after) != string(before) {
		t.Fatalf("the block must not be written:\n%s", after)
	}
	if m.sshcfgFormUI.focus != sfHostName || m.sshcfgFormUI.err != "HostName cannot have line breaks or tabs" {
		t.Errorf("focus=%d err=%q", m.sshcfgFormUI.focus, m.sshcfgFormUI.err)
	}
}

// The add row is its own submit: a line break there is refused when it is
// taken, and the save around it ignores it, as it ignores a half-typed option.
func TestTheAddOptionRowRefusesALineBreak(t *testing.T) {
	m, p := cfgApp(t, cfgFixture)
	m = typeText(pressA(m, "A"), "box")
	m.sshcfgFormUI.focus = m.sshcfgFormUI.addRow()
	m = paste(m, "SetEnv A=1\n")
	n := len(m.sshcfgFormUI.fields)
	m = pressA(m, "enter")
	if len(m.sshcfgFormUI.fields) != n || m.sshcfgFormUI.err != "An option cannot have line breaks or tabs" {
		t.Fatalf("the option must not be taken: fields %d→%d err=%q", n, len(m.sshcfgFormUI.fields), m.sshcfgFormUI.err)
	}

	m.sshcfgFormUI.focus = sfHost
	m = pressA(m, "enter")
	if after, _ := os.ReadFile(p); !strings.Contains(string(after), "Host box") {
		t.Errorf("the block should save without the half-typed option, err=%q", m.sshcfgFormUI.err)
	}
}

func TestAddKnownHostRefusesALineBreak(t *testing.T) {
	m, _ := knownApp(t, knownUIFixture())
	m = pressA(m, "A")
	m = paste(m, "gw\n")
	m = pressA(m, "enter")
	if m.knownAddUI.scanning || m.knownAddUI.err != "Host cannot have line breaks or tabs" {
		t.Errorf("scanning=%v err=%q", m.knownAddUI.scanning, m.knownAddUI.err)
	}
}

func TestTheOperationPagesRefuseALineBreak(t *testing.T) {
	m := openPage(appWith(sample(), nil), prefExport)
	m.exportPage.fields[1].value = "backup\n"
	m = pressA(m, "enter")
	if m.exportPage.err != "filename cannot have line breaks or tabs" || m.exportPage.focus != 1 {
		t.Errorf("filename: err=%q focus=%d", m.exportPage.err, m.exportPage.focus)
	}
	m.exportPage.fields[0].value = "\t" + m.exportPage.fields[0].value
	m = pressA(m, "enter")
	if m.exportPage.err != "directory cannot have line breaks or tabs" || m.exportPage.focus != 0 {
		t.Errorf("directory: err=%q focus=%d", m.exportPage.err, m.exportPage.focus)
	}

	m = openPage(m, prefImport)
	m.importPage.fields[0].value = "x.sshu\n"
	m = pressA(m, "enter")
	if m.importPage.err != "the bundle path cannot have line breaks or tabs" {
		t.Errorf("bundle: err=%q", m.importPage.err)
	}
}

// Every input box refuses one, each naming what it asked for.
func TestEveryInputBoxRefusesALineBreak(t *testing.T) {
	m := sftpFixture(t, 100, 26)
	m.sftp.focus = panelLeftFiles
	m = paste(pressA(m, "A"), "new\n")
	m = pressA(m, "enter")
	if !m.input.isActive() || m.input.err != "A name cannot have line breaks or tabs" {
		t.Errorf("Add: err=%q", m.input.err)
	}
	if _, err := os.Stat(filepath.Join(m.sftp.sides[sideLeft].cwd, "new")); err == nil {
		t.Error("Add trimmed the break off and created the file")
	}

	g := pressA(appWith(sample(), nil), "S", "2")
	g.ssh.layout = layoutCustom
	for i := 0; i < 8 && !g.input.isActive(); i++ {
		g = pressA(g, "j", "enter")
	}
	if !g.input.isActive() {
		t.Fatal("setup: the custom grid box did not open")
	}
	g = paste(pressA(g, "backspace"), "3\n")
	g = pressA(g, "enter")
	if g.input.err != "Columns cannot have line breaks or tabs" {
		t.Errorf("Custom grid: err=%q", g.input.err)
	}

	k, _ := knownApp(t, knownUIFixture())
	k = pressA(k, "E")
	if !k.input.isActive() {
		t.Fatal("setup: E should ask which names")
	}
	k = paste(k, "\n")
	k = pressA(k, "enter")
	if k.input.err != "Names cannot have line breaks or tabs" {
		t.Errorf("Trusted for: err=%q", k.input.err)
	}
}

// ssh reads the answer up to its first newline, so a pasted one would send
// half of it. The box keeps an error row from the moment it opens, so the
// refusal does not change its height (tdp F7).
func TestThePasswordBoxRefusesALineBreak(t *testing.T) {
	m, _, _ := sshconfigDialing(t)
	req, far := question(sideLeft, 7, "demo@gw's password: ")
	got := make(chan string, 1)
	go func() { got <- readReply(t, far) }()
	next, _ := m.askpassArrived(req)
	m = settle(next.(AppModel))
	opened := lineCount(m.askpassUI.view())

	m = paste(m, "hunter2\n")
	m = pressA(m, "enter")
	if !m.askpassUI.anim.owns() {
		t.Fatal("Enter must not send an answer with a line break in it")
	}
	v := m.askpassUI.view()
	if !strings.Contains(ansi.Strip(v), "The answer cannot have line breaks or tabs") {
		t.Errorf("the error row should say why:\n%s", ansi.Strip(v))
	}
	if strings.Contains(ansi.Strip(v), `\n`) || !strings.Contains(ansi.Strip(v), "••••••••") {
		t.Errorf("masked: one bullet per rune, the break among them, never \\n:\n%s", ansi.Strip(v))
	}
	if lineCount(v) != opened {
		t.Errorf("the box changed height: %d -> %d", opened, lineCount(v))
	}

	m = pressA(m, "backspace")
	if m.askpassUI.err != "" {
		t.Errorf("typing should clear the error, got %q", m.askpassUI.err)
	}
	m = pressA(m, "enter")
	if reply := <-got; reply != "hunter2\n" {
		t.Errorf("ssh would have received %q", reply)
	}
}

// ------------------------------------------------- an sshconfig host's rows

// User and Port are optional on an sshconfig host — the file has both — and
// the save says so too, not only the form (fix list item 2).
func TestAnSSHConfigHostSavesWithoutUserOrPort(t *testing.T) {
	var saved []store.Host
	m := pressA(appWith(sample(), &saved), "A")
	m.form.focus = fAuth
	m = pressA(m, "right", "right") // privatekey → credential → sshconfig
	if m.form.auth() != store.AuthSSHConfig || m.form.fields[fPort].value != "" {
		t.Fatalf("setup: auth=%s port=%q", m.form.auth(), m.form.fields[fPort].value)
	}
	m.form.focus = fName
	m = typeText(m, "cfg")
	m.form.focus = fHost
	m = typeText(m, "prod")

	m.form.focus = fPort
	m = typeText(m, "70000")
	m = pressA(m, "enter")
	if saved != nil || m.form.err != "Port must be 1-65535" {
		t.Errorf("a Port that is typed is still checked: err=%q", m.form.err)
	}

	m.form.focus = fPort
	m = pressA(m, "backspace", "backspace", "backspace", "backspace", "backspace", "enter")
	if m.form.err != "" || len(saved) == 0 {
		t.Fatalf("the host should save, err=%q", m.form.err)
	}
	h := saved[len(saved)-1]
	if h.Name != "cfg" || h.Port != 0 || h.User != "" {
		t.Errorf("saved %+v, want no port and no user", h)
	}
}

// --------------------------------------- the search belongs to its panel's focus

// Tab to the nav and the keys are the nav's: the query stays, grey, and the
// typing picks up again when the focus comes back (tdp K8, fix list item 3).
func TestTheHostsSearchStopsTypingWhenItsPanelLosesFocus(t *testing.T) {
	withColour(t)
	m := sized(sample(), 100, 24)
	m = typeText(pressA(m, "/"), "prod")
	if !caretOnQueryRow(t, m, "prod") {
		t.Fatal("setup: the query row being typed into has a caret")
	}
	m = pressA(m, "tab")
	if m.pref.focus != panelPrefNav {
		t.Fatal("setup: Tab should move to the nav")
	}
	if caretOnQueryRow(t, m, "prod") {
		t.Error("the query row should lose its caret with the focus")
	}
	if m.typing() {
		t.Error("nothing is being typed into with the nav focused")
	}
	m = typeText(m, "x")
	if m.hosts.query != "prod" {
		t.Errorf("a key on the nav went into the query: %q", m.hosts.query)
	}
	if m = pressA(m, "?"); !m.help.isActive() {
		t.Error("? is the nav's again: the key reference")
	}
	m = pressA(m, "esc", "esc")
	if !m.hosts.filtering || m.hosts.query != "prod" {
		t.Errorf("Esc on the nav is the nav's, the search stays: filtering=%v query=%q",
			m.hosts.filtering, m.hosts.query)
	}

	m = pressA(m, "tab")
	m = typeText(m, "x")
	if m.hosts.query != "prodx" {
		t.Errorf("back on the table the typing goes on: %q", m.hosts.query)
	}
}

// caretOnQueryRow reports whether the screen line holding the query has the
// block caret on it — the one cell drawn on a Subtext1 background.
func caretOnQueryRow(t *testing.T, m AppModel, query string) bool {
	t.Helper()
	bg := ansiBgOf(t, handColor) // in one SGR with the foreground
	for _, l := range strings.Split(m.View(), "\n") {
		if strings.Contains(ansi.Strip(l), glyphSearch+" "+query) {
			return strings.Contains(l, bg)
		}
	}
	t.Fatalf("no query row %q on screen", query)
	return false
}

func TestTheFileSearchStopsTypingOnTheMarksBesideIt(t *testing.T) {
	withColour(t)
	m := sftpFixture(t, 100, 26)
	m.sftp.focus = panelLeftFiles
	m = typeText(pressA(m, "/"), "dep")
	if !caretOnQueryRow(t, m, "dep") {
		t.Fatal("setup: the query row being typed into has a caret")
	}
	m = pressA(m, "tab")
	if m.sftp.focus != panelLeftMarks {
		t.Fatal("setup: Tab should move to the marks on the same side")
	}
	if caretOnQueryRow(t, m, "dep") {
		t.Error("the query row should lose its caret with the focus")
	}
	m = typeText(m, "w")
	if q := m.sftp.sides[sideLeft].query; q != "dep" {
		t.Errorf("a key on the marks went into the query: %q", q)
	}
	if m = pressA(m, "?"); !m.help.isActive() {
		t.Error("? is the marks panel's again: the key reference")
	}
	m = pressA(m, "esc", "esc")
	if !m.sftp.sides[sideLeft].filtering {
		t.Error("Esc on the marks must not drop the files panel's search")
	}

	m = pressA(m, "1")
	m = typeText(m, "w")
	if q := m.sftp.sides[sideLeft].query; q != "depw" {
		t.Errorf("back on the files the typing goes on: %q", q)
	}
}
