package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/vulcanshen/sshu/internal/store"
)

// cfgFixture is the shape a real ~/.ssh/config has: a labelled global block
// whose options are all outside the five the form gives a permanent row, a
// labelled host block carrying a keyword sshu has never heard of, and an
// Include pointing at blocks this panel does not parse.
//
// The Include is at the TOP, where a real one goes and where it means "for
// everything": ssh scopes an Include to the block it sits in, so one at the
// bottom would belong to the last Host — correct, and not what anybody writing
// this file at the bottom would have meant.
const cfgFixture = `Include conf.d/*

# personal
Host *
  AddKeysToAgent yes
  UseKeychain yes

# production
Host prod
  HostName prod.example.internal
  User deploy
  Port 2222
  SetEnv LANG=en_US.UTF-8
`

// cfgApp is the manage tab with the Config panel holding the keyboard, backed
// by a real file in a temp dir — so a test can read back exactly what sshu
// wrote, which is the only thing that matters about this panel.
func cfgApp(t *testing.T, raw string) (AppModel, string) {
	t.Helper()
	m, p := cfgBackedApp(t, nil, raw)
	// nav → hosts → credentials → config, then hand the keyboard to the content.
	return pressA(m, "1", "j", "j", "enter"), p
}

// cfgBackedApp is the app with a real ~/.ssh/config behind it, sitting where it
// opens (the hosts table) — for the tests that are about what a HOST detail
// says rather than about the Config panel.
func cfgBackedApp(t *testing.T, hosts []store.Host, raw string) (AppModel, string) {
	t.Helper()
	p := filepath.Join(t.TempDir(), "config")
	if err := os.WriteFile(p, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := store.LoadSSHConfigFrom(p)
	if err != nil {
		t.Fatal(err)
	}
	m := New(hosts, nil, store.DefaultConfig()).
		WithSSHConfig(f, func(g store.SSHConfigFile) (store.SSHConfigFile, error) {
			return store.SaveSSHConfig(g)
		})
	// Tall, so a float never has to scroll for a test to read all of it.
	next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	return settle(next.(AppModel)), p
}

// unionFixture: a specific block, a global one, and an Include — so a host
// named `gw` takes something from each and the answer is knowingly incomplete.
const unionFixture = `Include conf.d/*

Host gw
  HostName 198.51.100.10
  Port 2222

Host *
  AddKeysToAgent yes
`

func aliasHost() store.Host {
	return store.Host{Name: "gateway", Host: "gw", Port: 22, User: "vulcan",
		Auth: store.AuthPrivateKey, IdentityFile: "~/.ssh/id_ed25519"}
}

// The host detail shows what ssh will ACTUALLY use — the union of every
// matching block, first value winning — because that is what ssh does, and
// none of it was visible anywhere before.
func TestAHostDetailShowsTheUnionSshWillActuallyUse(t *testing.T) {
	m, path := cfgBackedApp(t, []store.Host{aliasHost()}, unionFixture)
	m = pressA(m, "enter")
	if !m.detail.isActive() {
		t.Fatal("Enter should open the host detail")
	}
	v := ansi.Strip(m.detail.view())

	// From the specific block: where this host actually lands. The heading is
	// two lines — the file, then the block — so neither can crowd the other off
	// the edge.
	if !strings.Contains(v, "Host gw") || !strings.Contains(v, "198.51.100.10") {
		t.Errorf("the resolved address must be there:\n%s", v)
	}
	// From the global block: it applies too, and a "which block does this match"
	// answer would have shown one or the other rather than both.
	if !strings.Contains(v, "Host *") || !strings.Contains(v, "AddKeysToAgent") {
		t.Errorf("the global block contributes as well:\n%s", v)
	}
	// And the one sshu overrides is not allowed to read as if it applies.
	if !strings.Contains(v, "2222 — sshu sends -p 22") {
		t.Errorf("Port is beaten by the command line and must say so:\n%s", v)
	}
	// The heading NAMES THE FILE, on its own line above the block. With Include
	// followed the blocks no longer all come from one, and a heading that only
	// said which block would leave the reader with nowhere to go and edit it.
	named := false
	for _, sec := range m.detail.sections {
		if strings.Contains(sec.title, path) && strings.Contains(sec.title, glyphFileCog) {
			named = true
		}
	}
	if !named {
		t.Errorf("no section heading names %q:\n%s", path, v)
	}

	// What could ALSO apply and is not in the list, named rather than counted —
	// "1 file" told nobody which file, and was wrong anyway, since one Include
	// directive can be a glob standing for ten.
	if !strings.Contains(v, "may also apply") ||
		!strings.Contains(v, "conf.d/* — matches nothing") {
		t.Errorf("the Include must be disclosed BY NAME:\n%s", v)
	}
}

// An override that changes nothing is not an override. `User vulcan` in the
// file, beaten by sshu sending `vulcan@`, is the same answer twice — and a red
// row on every host for that would be crying wolf.
func TestAnOverrideThatChangesNothingIsNotFlagged(t *testing.T) {
	const raw = "Host gw\n  HostName 198.51.100.10\n  User vulcan\n  Port 22\n"
	m, _ := cfgBackedApp(t, []store.Host{aliasHost()}, raw) // the host is vulcan on 22
	m = pressA(m, "enter")
	v := ansi.Strip(m.detail.view())
	if strings.Contains(v, "sshu sends") {
		t.Errorf("nothing here disagrees, so nothing should be marked:\n%s", v)
	}
	// The rows are still THERE — they do apply, they just do not conflict.
	if !strings.Contains(v, "User") || !strings.Contains(v, "Port") {
		t.Errorf("the values still apply and belong in the union:\n%s", v)
	}
}

// A `Host *` Port would be shadowed by the specific block's, so listing it
// would be listing a value ssh will never use.
func TestTheUnionDoesNotListAShadowedValue(t *testing.T) {
	const raw = "Host gw\n  Port 2222\n\nHost *\n  Port 22\n"
	m, _ := cfgBackedApp(t, []store.Host{aliasHost()}, raw)
	m = pressA(m, "enter")
	v := ansi.Strip(m.detail.view())
	if strings.Count(v, "Port") != 2 { // the Connection row, and the one config row
		t.Errorf("Port should appear once in the config section:\n%s", v)
	}
}

func TestAHostThatMeetsNothingGetsNoConfigSection(t *testing.T) {
	// No `Host *`, and this host's address matches nothing — but the file DOES
	// have an Include to disclose. That is the case that matters: the caveat
	// belongs to an answer, and with no answer there is nothing to caveat. A
	// fixture without the Include would pass however the guard was written.
	const raw = "Include conf.d/*\n\nHost gw\n  HostName 198.51.100.10\n"
	h := store.Host{Name: "direct", Host: "10.0.3.14", Port: 22, User: "root",
		Auth: store.AuthPrivateKey, IdentityFile: "~/.ssh/id"}
	m, _ := cfgBackedApp(t, []store.Host{h}, raw)
	m = pressA(m, "enter")
	if v := ansi.Strip(m.detail.view()); strings.Contains(v, "~/.ssh/config") {
		t.Errorf("nothing applies, so nothing should be said:\n%s", v)
	}
}

// Deleting a block that DEFINES a name breaks every sshu host that uses it,
// quietly and at connect time. That is the same disclosure deleting a
// credential makes, and for the same reason.
func TestDeletingABlockWarnsWhichSshuHostsResolveThroughIt(t *testing.T) {
	m, _ := cfgBackedApp(t, []store.Host{aliasHost()}, unionFixture)
	m = pressA(m, "1", "j", "j", "enter") // to the Config panel
	m = pressA(m, "X")                    // the cursor opens on `Host gw`
	if !m.confirm.isActive() {
		t.Fatal("X should ask first")
	}
	lines := strings.Join(m.confirm.lines, " ")
	if !strings.Contains(lines, "1 sshu host resolves through it") {
		t.Errorf("it must name the cost, got %q", lines)
	}
	// And the count above it agrees with itself.
	if !strings.Contains(lines, "2 options go with it") {
		t.Errorf("got %q", lines)
	}
}

// And deleting the global block is the milder fact: nobody stops resolving,
// they just stop inheriting.
func TestDeletingTheGlobalBlockSaysTheMilderThing(t *testing.T) {
	hosts := []store.Host{aliasHost(),
		{Name: "direct", Host: "10.0.3.14", Port: 22, User: "root", Auth: store.AuthPrivateKey}}
	m, _ := cfgBackedApp(t, hosts, unionFixture)
	m = pressA(m, "1", "j", "j", "enter")
	m = pressA(m, "j", "X") // down to the `Host *` block
	lines := strings.Join(m.confirm.lines, " ")
	if !strings.Contains(lines, "2 sshu hosts inherit options from it") {
		t.Errorf("got %q", lines)
	}
	// One option, so the verb is singular. The confirm box is a sentence
	// somebody reads before doing something irreversible.
	if !strings.Contains(lines, "1 option goes with it") {
		t.Errorf("got %q", lines)
	}
	if strings.Contains(lines, "will not connect") {
		t.Errorf("nothing stops resolving, so nothing should say so: %q", lines)
	}
}

func diskText(t *testing.T, p string) string {
	t.Helper()
	raw, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// Once Include is followed the list spans files, and both facts a reader needs
// are things a single-file config never had to say: how many files, and which
// one each row is in.
// A config section can be long — a `Host *` block with twenty options is an
// ordinary thing — so the float has to scroll, with the same j/k/u/d the rest
// of the app uses (§4.2). And the offer at the foot must NOT scroll with it:
// the hint promises Enter does something, and a promise that can leave the box
// is not one (§11.29).
func TestALongConfigSectionScrollsAndTheOfferStaysPut(t *testing.T) {
	var b strings.Builder
	b.WriteString("Host gw\n  HostName 198.51.100.10\n")
	for i := 1; i <= 30; i++ {
		fmt.Fprintf(&b, "  Opt%02d value-%02d\n", i, i)
	}
	m, _ := cfgBackedApp(t, []store.Host{aliasHost()}, b.String())
	// Short enough that thirty options cannot possibly fit.
	next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 20})
	m = settle(next.(AppModel))
	m = pressA(m, "enter")
	if !m.detail.isActive() {
		t.Fatal("Enter should open the detail float")
	}
	if len(m.detail.lines()) <= m.detail.visible() {
		t.Fatalf("setup: %d lines fit in %d, so scrolling proves nothing",
			len(m.detail.lines()), m.detail.visible())
	}

	// The hint has to SAY it scrolls, or the keys are a secret.
	if v := ansi.Strip(m.detail.view()); !strings.Contains(v, "j/k") {
		t.Errorf("the hint must disclose the scroll keys:\n%s", v)
	}

	top := m.detail.top
	m = pressA(m, "j", "j", "j")
	if m.detail.top != top+3 {
		t.Errorf("j should move one line each, %d → %d", top, m.detail.top)
	}
	afterJ := m.detail.top
	m = pressA(m, "d")
	if m.detail.top <= afterJ+1 {
		t.Errorf("d should take a half page, %d → %d", afterJ, m.detail.top)
	}
	m = pressA(m, "u", "k")
	if m.detail.top >= afterJ+1 {
		t.Errorf("u and k should come back up, got %d", m.detail.top)
	}
	m = pressA(m, "G")
	if m.detail.top == 0 {
		t.Error("G should reach the bottom")
	}

	// Scrolled to the very end, the offer is still there — it is a fixed foot,
	// not the last scrollable row.
	if v := ansi.Strip(m.detail.view()); !strings.Contains(v, `Connect to "gateway"?`) {
		t.Errorf("the offer must not scroll away:\n%s", v)
	}
}

func TestAMultiFileTreeSaysHowManyFilesAndWhichRowIsWhere(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "conf.d"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "conf.d", "work"),
		[]byte("Host office\n  HostName 10.20.0.1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(dir, "config")
	if err := os.WriteFile(root,
		[]byte("Include conf.d/*\n\nHost gw\n  HostName 198.51.100.10\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := store.LoadSSHConfigFrom(root)
	if err != nil {
		t.Fatal(err)
	}
	m := New(nil, nil, store.DefaultConfig()).
		WithSSHConfig(f, func(g store.SSHConfigFile) (store.SSHConfigFile, error) {
			return store.SaveSSHConfig(g)
		})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 110, Height: 30})
	m = pressA(settle(next.(AppModel)), "1", "j", "j", "enter")

	if st := m.prefStatus(); !strings.Contains(st, "2 blocks in 2 files") {
		t.Errorf("the status should say the tree spans files, got %q", st)
	}
	v := ansi.Strip(m.View())
	if !strings.Contains(v, "File") || !strings.Contains(v, "work") {
		t.Errorf("the File column should say which row is where:\n%s", v)
	}
	// Both blocks are listed and both are the panel's to edit — the included
	// one is not a second-class row.
	if !strings.Contains(v, "office") || !strings.Contains(v, "gw") {
		t.Errorf("both files' blocks belong in the list:\n%s", v)
	}
}

func TestConfigListsTheBlocksAndSaysWhatItIsNotListing(t *testing.T) {
	m, _ := cfgApp(t, cfgFixture)
	v := ansi.Strip(m.View())
	for _, want := range []string{"[2] Config", "prod", "prod.example.internal", "deploy"} {
		if !strings.Contains(v, want) {
			t.Errorf("the table should show %q:\n%s", want, v)
		}
	}
	// The Include here resolves to nothing, and an Include that went nowhere is
	// still a source the list dropped — saying so is what stops the list being
	// read as the whole picture.
	if st := m.prefStatus(); !strings.Contains(st, "1 include unread") {
		t.Errorf("the status must disclose the unread Include, got %q", st)
	}
}

func TestEnterOnABlockShowsEveryOptionAndOffersEdit(t *testing.T) {
	m, p := cfgApp(t, cfgFixture)
	m = pressA(m, "j", "enter") // the prod block
	if !m.detail.isActive() {
		t.Fatal("Enter should open the detail float")
	}
	body := ansi.Strip(m.detail.view())
	// SetEnv has no column and no form row of its own; the float is where a
	// block's whole contents are legible.
	for _, want := range []string{"SetEnv", "LANG=en_US.UTF-8", "HostName"} {
		if !strings.Contains(body, want) {
			t.Errorf("the float should carry %q:\n%s", want, body)
		}
	}
	// The file this block lives in, headed the same way a host's detail heads
	// it — with Include followed, "which file" is the first thing anybody
	// about to hand-edit it needs, and it used to be buried in the Lines value.
	if !strings.Contains(m.detail.sections[0].title, glyphFileCog) ||
		!strings.Contains(m.detail.sections[0].title, p) {
		t.Errorf("the heading should name %q, got %q", p, m.detail.sections[0].title)
	}
	if m.detail.prompt != `Edit "prod"?` || m.detail.action != detailEditSSHCfg {
		t.Errorf("the foot should offer the edit, got %q / %d", m.detail.prompt, m.detail.action)
	}
	// And Enter again goes to the form, replacing the float rather than
	// stacking on it (§6.4).
	m = pressA(m, "enter")
	if !m.sshcfgFormUI.isActive() || m.detail.isActive() {
		t.Fatalf("the form should replace the float, form=%v detail=%v",
			m.sshcfgFormUI.isActive(), m.detail.isActive())
	}
}

func TestEditingOneValueRewritesOnlyThatLine(t *testing.T) {
	m, p := cfgApp(t, cfgFixture)
	m = pressA(m, "j", "E")
	if !m.sshcfgFormUI.isActive() {
		t.Fatal("E should open the form")
	}
	m.sshcfgFormUI.fields[sfHostName].value = "prod.example.net"
	m = pressA(m, "enter")

	got := diskText(t, p)
	if !strings.Contains(got, "  HostName prod.example.net\n") {
		t.Fatalf("the edit did not land:\n%s", got)
	}
	// Everything else is byte-identical: the comments, the unknown keyword, the
	// global block, and the Include.
	want := strings.Replace(cfgFixture, "prod.example.internal", "prod.example.net", 1)
	if got != want {
		t.Errorf("only the HostName line should have moved:\nwant %q\ngot  %q", want, got)
	}
}

func TestClearingAnOptionDeletesItsLine(t *testing.T) {
	m, p := cfgApp(t, cfgFixture)
	m = pressA(m, "j", "E")
	m.sshcfgFormUI.fields[sfPort].value = ""
	m = pressA(m, "enter")

	got := diskText(t, p)
	if strings.Contains(got, "Port") {
		t.Errorf("clearing the field should take the line out:\n%s", got)
	}
	if !strings.Contains(got, "  User deploy\n  SetEnv") {
		t.Errorf("the lines either side should close up cleanly:\n%s", got)
	}
}

func TestAKeywordSshuDoesNotKnowGetsARowOfItsOwn(t *testing.T) {
	m, _ := cfgApp(t, cfgFixture)

	// The global block is the case the five fixed rows cannot serve at all:
	// everything it does is outside them.
	m = pressA(m, "E")
	labels := map[string]string{}
	for _, f := range m.sshcfgFormUI.fields {
		labels[f.label] = f.value
	}
	if labels["AddKeysToAgent"] != "yes" || labels["UseKeychain"] != "yes" {
		t.Fatalf("a Host * block must be editable, fields were %v", labels)
	}
	if labels["Host"] != "*" {
		t.Errorf("the pattern row should hold the pattern, got %q", labels["Host"])
	}
}

func TestTheAddRowTurnsTypingIntoAnOptionRow(t *testing.T) {
	m, p := cfgApp(t, cfgFixture)
	m = pressA(m, "j", "E")

	at := m.sshcfgFormUI.addRow()
	m.sshcfgFormUI.focus = at
	m.sshcfgFormUI.fields[at].value = "ProxyJump bastion"
	m = pressA(m, "tab")

	if m.sshcfgFormUI.fields[at].label != "ProxyJump" {
		t.Fatalf("the add row should have become a ProxyJump row, got %q",
			m.sshcfgFormUI.fields[at].label)
	}
	if m.sshcfgFormUI.focus != at {
		t.Errorf("the cursor should land on the new row so its value can be typed, focus=%d",
			m.sshcfgFormUI.focus)
	}
	if last := m.sshcfgFormUI.fields[len(m.sshcfgFormUI.fields)-1].label; last != sshcfgAddLabel {
		t.Errorf("a fresh add row should be waiting underneath, got %q", last)
	}

	m = pressA(m, "enter")
	if got := diskText(t, p); !strings.Contains(got, "\n  ProxyJump bastion\n") {
		t.Errorf("the new option did not reach the file:\n%s", got)
	}
}

func TestTheAddRowRefusesAKeywordThatWouldSplitTheBlock(t *testing.T) {
	m, _ := cfgApp(t, cfgFixture)
	m = pressA(m, "j", "E")
	before := len(m.sshcfgFormUI.fields)

	at := m.sshcfgFormUI.addRow()
	m.sshcfgFormUI.focus = at
	m.sshcfgFormUI.fields[at].value = "Host other"
	m = pressA(m, "tab")

	// `Host` as an option would END this block at that line, and everything
	// under it would silently belong to a block nobody meant to create.
	if !strings.Contains(m.sshcfgFormUI.err, "starts a new block") {
		t.Errorf("it must say why, got %q", m.sshcfgFormUI.err)
	}
	if len(m.sshcfgFormUI.fields) != before {
		t.Errorf("nothing should have been added, %d → %d fields", before, len(m.sshcfgFormUI.fields))
	}
}

func TestAddingAKeywordThatAlreadyHasARowGoesToThatRow(t *testing.T) {
	m, _ := cfgApp(t, cfgFixture)
	m = pressA(m, "j", "E")
	before := len(m.sshcfgFormUI.fields)

	at := m.sshcfgFormUI.addRow()
	m.sshcfgFormUI.focus = at
	m.sshcfgFormUI.fields[at].value = "hostname"
	m = pressA(m, "tab")

	// ssh reads only the first of a repeated keyword, so a second row would be
	// a line that looks like it works and does nothing.
	if m.sshcfgFormUI.focus != sfHostName {
		t.Errorf("the cursor should go to the existing HostName row, focus=%d", m.sshcfgFormUI.focus)
	}
	if len(m.sshcfgFormUI.fields) != before {
		t.Errorf("no second row should appear, %d → %d fields", before, len(m.sshcfgFormUI.fields))
	}
}

func TestDeleteAsksFirstThenTakesTheBlockAndItsOwnComment(t *testing.T) {
	m, p := cfgApp(t, cfgFixture)
	m = pressA(m, "j", "X")
	if !m.confirm.isActive() {
		t.Fatal("X should ask first — this is somebody's ssh config")
	}
	lines := strings.Join(m.confirm.lines, " ")
	// The FILE by name — with Include followed, [X] can rewrite one the user
	// never opened, and a confirmation naming the wrong file lies exactly when
	// it matters. In a temp dir that name is the temp path; what matters is
	// that it is the file this block is really in.
	for _, want := range []string{`Delete Host "prod"?`, p, "4 options"} {
		if !strings.Contains(lines, want) {
			t.Errorf("the question should say %q, got %q", want, lines)
		}
	}

	m = pressA(m, "enter")
	got := diskText(t, p)
	if strings.Contains(got, "Host prod") || strings.Contains(got, "# production") {
		t.Errorf("the block and its own label should both be gone:\n%s", got)
	}
	if !strings.Contains(got, "# personal\nHost *") || !strings.Contains(got, "Include") {
		t.Errorf("everything else must survive:\n%s", got)
	}
}

func TestAddAppendsANewBlockToTheEndOfTheFile(t *testing.T) {
	m, p := cfgApp(t, cfgFixture)
	m = pressA(m, "A")
	if !m.sshcfgFormUI.isActive() || m.sshcfgFormUI.editing != -1 {
		t.Fatal("A should open a form that creates")
	}
	m.sshcfgFormUI.fields[sfHost].value = "gw"
	m.sshcfgFormUI.fields[sfHostName].value = "198.51.100.10"
	m = pressA(m, "enter")

	got := diskText(t, p)
	if !strings.HasSuffix(got, "Host gw\n  HostName 198.51.100.10\n") {
		t.Fatalf("a new block goes at the end — ssh takes the first match:\n%s", got)
	}
	if !strings.HasPrefix(got, cfgFixture) {
		t.Errorf("nothing above it should have moved:\n%s", got)
	}
	// And the cursor follows what was just written, as it does on both siblings.
	if b, _, ok := m.cursorSSHCfg(); !ok || b.Patterns != "gw" {
		t.Errorf("the cursor should land on the new block, got %+v", b)
	}
}

func TestASaveRefusedByAnotherEditorPutsTheirFileOnScreen(t *testing.T) {
	m, p := cfgApp(t, cfgFixture)

	// vim, VS Code, a dotfiles pull — this file has other writers, and sshu
	// rewrites the whole of it.
	const theirs = "Host somewhere-else\n  User root\n"
	if err := os.WriteFile(p, []byte(theirs), 0o600); err != nil {
		t.Fatal(err)
	}

	m = pressA(m, "j", "X", "enter")
	if got := diskText(t, p); got != theirs {
		t.Fatalf("their file must be left exactly as it was:\n%s", got)
	}
	// The log is the durable half of saying it — the toast is gone in seconds,
	// and this is the one refusal a user may need to reason about later.
	if n := len(m.log.entries); n == 0 ||
		!strings.Contains(m.log.entries[n-1].msg, "changed on disk") {
		t.Errorf("the refusal must be recorded, log is %+v", m.log.entries)
	}
	// The panel adopts what is actually on disk, so the list and the message
	// are not contradicting each other.
	if len(m.sshcfg.file.Blocks) != 1 || m.sshcfg.file.Blocks[0].Patterns != "somewhere-else" {
		t.Errorf("the panel should be showing their file, got %+v", m.sshcfg.file.Blocks)
	}
}

func TestAnEmptyConfigOffersTheKeyThatFillsIt(t *testing.T) {
	m, _ := cfgApp(t, "")
	v := ansi.Strip(m.View())
	// The FILE is named, because "nothing here" on a panel called Config would
	// be ambiguous with sshu's own config.yaml.
	if !strings.Contains(v, "Nothing in ~/.ssh/config") || !strings.Contains(v, "[A]") {
		t.Errorf("the empty state must name the file and the way out:\n%s", v)
	}
}

func TestConfigTableHeaderAndRowsAlign(t *testing.T) {
	b := store.SSHBlock{Patterns: "prod prod-*", Options: []store.SSHOption{
		{Key: "HostName", Value: "prod.example.internal"},
		{Key: "User", Value: "deploy"},
	}}
	// Swept rather than sampled: the widths that break a responsive layout are
	// the ones where a column is dropped, and those move with the constants.
	// Both file counts, because the File column only exists above one and its
	// arrival is exactly the kind of thing that shears a header off its rows.
	for _, files := range []int{1, 3} {
		m := sshcfgModel{file: fileTree(t, files)}
		for w := 12; w <= 140; w++ {
			host, target, user, file := sshcfgCols(w, files)
			m.w, m.h = w+2, 10
			head := dispW(m.tableBody(w, 3)[0])
			row := dispW(m.row(b, false, host, target, user, file, w))
			sel := dispW(m.row(b, true, host, target, user, file, w))
			if head != w || row != w || sel != w {
				t.Errorf("files=%d w=%d: header=%d row=%d selected=%d, all should be %d",
					files, w, head, row, sel, w)
			}
		}
	}
}

// fileTree is a config whose Include really resolves, so Files() reports n.
func fileTree(t *testing.T, n int) store.SSHConfigFile {
	t.Helper()
	dir := t.TempDir()
	root := "Include conf.d/*\n\nHost gw\n  HostName 198.51.100.10\n"
	if n > 1 {
		if err := os.Mkdir(filepath.Join(dir, "conf.d"), 0o700); err != nil {
			t.Fatal(err)
		}
		for i := 1; i < n; i++ {
			p := filepath.Join(dir, "conf.d", "part"+itoa(i))
			if err := os.WriteFile(p, []byte("Host in"+itoa(i)+"\n  User x\n"), 0o600); err != nil {
				t.Fatal(err)
			}
		}
	}
	p := filepath.Join(dir, "config")
	if err := os.WriteFile(p, []byte(root), 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := store.LoadSSHConfigFrom(p)
	if err != nil {
		t.Fatal(err)
	}
	if f.Files() != n {
		t.Fatalf("setup: wanted %d files, got %d", n, f.Files())
	}
	return f
}

// The File column carries the WHOLE path, cut from the FRONT. Two included
// files with the same base name are the case it exists for, and a base name or
// a tail-truncated path would render them identically.
func TestTheFileColumnKeepsWhatTellsTwoPathsApart(t *testing.T) {
	dir := t.TempDir()
	for _, d := range []string{"work", "home"} {
		if err := os.MkdirAll(filepath.Join(dir, d), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, d, "hosts"),
			[]byte("Host "+d+"-box\n  User x\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	root := filepath.Join(dir, "config")
	if err := os.WriteFile(root, []byte("Include work/hosts home/hosts\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := store.LoadSSHConfigFrom(root)
	if err != nil {
		t.Fatal(err)
	}
	if f.Files() != 3 {
		t.Fatalf("setup: wanted three files, got %d", f.Files())
	}

	m := sshcfgModel{file: f, w: 120, h: 12}
	rows := m.tableBody(118, 6)
	a, b := ansi.Strip(rows[1]), ansi.Strip(rows[2])
	if a == b {
		t.Fatalf("setup: the two rows should differ\n%q", a)
	}
	// Both base names are `hosts`, so the directory is the only thing that
	// separates them and it has to be on screen.
	if !strings.Contains(a+b, "work/hosts") || !strings.Contains(a+b, "home/hosts") {
		t.Errorf("the path must keep what distinguishes it:\n%s\n%s", a, b)
	}
	// And no Opts column: it counted something the detail float states properly.
	if strings.Contains(ansi.Strip(rows[0]), "Opts") {
		t.Errorf("the Opts column should be gone:\n%s", ansi.Strip(rows[0]))
	}
}

func TestTheFormScrollsRatherThanHidingFieldsItCanStillReach(t *testing.T) {
	var big strings.Builder
	big.WriteString("Host busy\n")
	for i := 0; i < 20; i++ {
		big.WriteString("  Opt" + itoa(i) + " on\n")
	}
	m, _ := cfgApp(t, big.String())
	m = pressA(m, "E")
	// The form's own height, not the helper's: this test is about what happens
	// when the fields outgrow the box, and it must not quietly stop being about
	// that because a shared fixture got taller.
	m.sshcfgFormUI.setSize(100, 16)

	fields := len(m.sshcfgFormUI.fields)
	lo, hi := m.sshcfgFormUI.window()
	if hi-lo >= fields {
		t.Fatalf("setup: %d fields fit in the window, so scrolling proves nothing", fields)
	}
	// The last field must still be reachable AND visible: a row you can Tab to
	// and cannot see is worse than one that is simply not there.
	m.sshcfgFormUI.focus = fields - 1
	lo, hi = m.sshcfgFormUI.window()
	if fields-1 < lo || fields-1 >= hi {
		t.Errorf("the focused field %d is outside the window [%d,%d)", fields-1, lo, hi)
	}
}

// The config sections are grouped BY FILE, with the blocks indented under the
// file's name and their options indented under those — three levels, because
// there are three things.
func TestTheConfigSectionsAreGroupedByFileAndIndented(t *testing.T) {
	const raw = "Host *\n  AddKeysToAgent yes\n\nHost gw\n  HostName 198.51.100.10\n"
	m, _ := cfgBackedApp(t, []store.Host{aliasHost()}, raw)
	m = pressA(m, "enter")

	// Both blocks are in one file, so ONE section carries both — the file name
	// is not repeated over each of them.
	cfgSecs := 0
	for _, s := range m.detail.sections {
		if strings.Contains(s.title, glyphFileCog) {
			cfgSecs++
		}
	}
	if cfgSecs != 1 {
		t.Fatalf("two blocks in one file should be one section, got %d", cfgSecs)
	}

	v := ansi.Strip(m.detail.view())
	// The file is outermost, the blocks are in from it, the options in again.
	file := indentOf(t, v, glyphFileCog)
	host := indentOf(t, v, "Host *")
	opt := indentOf(t, v, "AddKeysToAgent")
	// Strictly in, each level past the last. NOT evenly: this float already
	// puts a heading one cell left of its own labels (Connection/Name), so
	// nesting that pair under a file gives a 2-then-1 step — the existing idiom
	// one level down, rather than a new one.
	if !(file < host && host < opt) {
		t.Errorf("the three levels should step in: file=%d host=%d option=%d\n%s",
			file, host, opt, v)
	}
	// The label column has to be sized WITH the indent, or the deepest label
	// ends exactly where its value begins and the two run together.
	if !strings.Contains(v, "AddKeysToAgent  yes") {
		t.Errorf("an indented label needs its gap before the value:\n%s", v)
	}
}

// An Include between two Host blocks really does make ssh read root → included
// → root, and the section order IS the precedence order. Collecting a file's
// blocks together would tidy that into a lie about which value won.
func TestInterleavedFilesStayInTheOrderSshReadsThem(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "conf.d"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "conf.d", "mid"),
		[]byte("Host g*\n  User mid\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(dir, "config")
	if err := os.WriteFile(root,
		[]byte("Host *\n  Port 2200\n\nInclude conf.d/mid\n\nHost gw\n  HostName 198.51.100.10\n"),
		0o600); err != nil {
		t.Fatal(err)
	}
	f, err := store.LoadSSHConfigFrom(root)
	if err != nil {
		t.Fatal(err)
	}
	m := New([]store.Host{aliasHost()}, nil, store.DefaultConfig()).
		WithSSHConfig(f, func(g store.SSHConfigFile) (store.SSHConfigFile, error) {
			return store.SaveSSHConfig(g)
		})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 110, Height: 40})
	m = pressA(settle(next.(AppModel)), "enter")

	var titles []string
	for _, s := range m.detail.sections {
		if strings.Contains(s.title, glyphFileCog) {
			titles = append(titles, s.title)
		}
	}
	// THREE, not two: the root speaks, the included file speaks, the root
	// speaks again. Two would mean the root's blocks had been collected.
	if len(titles) != 3 {
		t.Fatalf("want three sections in reading order, got %d: %v", len(titles), titles)
	}
	if titles[0] == titles[1] || titles[1] == titles[2] || titles[0] != titles[2] {
		t.Errorf("the run should be root, included, root: %v", titles)
	}
}

// indentOf is how far in the line containing needle starts.
func indentOf(t *testing.T, view, needle string) int {
	t.Helper()
	for _, line := range strings.Split(view, "\n") {
		i := strings.Index(line, needle)
		if i < 0 {
			continue
		}
		// The DISPLAY width of what precedes it, not the byte offset: the box
		// border is three bytes and the file glyph is four, so a byte index
		// would report a column nothing is actually at.
		return dispW(line[:i])
	}
	t.Fatalf("no line contains %q:\n%s", needle, view)
	return -1
}
