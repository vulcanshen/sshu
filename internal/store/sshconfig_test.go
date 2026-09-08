package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// sampleConfig is the shape that makes this parser worth having: a file header,
// a labelled global block, a labelled host block with a keyword sshu has never
// heard of, a Match block whose options belong to nobody, and an Include.
const sampleConfig = `# my ssh notes

# personal
Host *
  AddKeysToAgent yes
  UseKeychain yes

# production
Host prod prod-*
  HostName prod.example.internal
  User deploy
  Port 2222
  SetEnv LANG=en_US.UTF-8

Match host bastion
  ForwardAgent yes

Include conf.d/*
`

// loadRaw writes raw to a temp root and loads it the way the app does, so a
// test exercises the real path — including Include resolution, which is why
// every fixture's Include is RELATIVE: an absolute or ~ one would reach out of
// the temp dir and read the machine's actual ~/.ssh.
func loadRaw(t *testing.T, raw string) SSHConfigFile {
	t.Helper()
	return loadRawIn(t, t.TempDir(), raw)
}

func loadRawIn(t *testing.T, dir, raw string) SSHConfigFile {
	t.Helper()
	p := filepath.Join(dir, "config")
	if err := os.WriteFile(p, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := LoadSSHConfigFrom(p)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

// changed reports the lines that differ between two renderings, as
// "before | after". A surgical edit should produce exactly one of these.
func changed(before, after string) []string {
	b, a := strings.Split(before, "\n"), strings.Split(after, "\n")
	var out []string
	for i := 0; i < len(b) || i < len(a); i++ {
		var x, y string
		if i < len(b) {
			x = b[i]
		}
		if i < len(a) {
			y = a[i]
		}
		if x != y {
			out = append(out, x+" | "+y)
		}
	}
	return out
}

func TestAnUntouchedFileRendersBackByteForByte(t *testing.T) {
	if got := loadRaw(t, sampleConfig).Text(); got != sampleConfig {
		t.Fatalf("Text() must be the file it was given\n%q", got)
	}
}

func TestEditingOneValueTouchesOneLine(t *testing.T) {
	f := loadRaw(t, sampleConfig)
	b := f.Blocks[1]
	b.Options = append([]SSHOption(nil), b.Options...)
	b.Options[b.IndexOf("HostName")].Value = "prod.example.net"

	g, err := f.Set(1, b)
	if err != nil {
		t.Fatal(err)
	}
	diff := changed(sampleConfig, g.Text())
	if len(diff) != 1 {
		t.Fatalf("one value changed, so one line should differ; got %d:\n%s",
			len(diff), strings.Join(diff, "\n"))
	}
	if !strings.Contains(diff[0], "prod.example.net") {
		t.Fatalf("the differing line should be HostName's: %s", diff[0])
	}
	// The keyword sshu does not model has to come through untouched — it is the
	// whole reason this file is not marshalled from a struct.
	if !strings.Contains(g.Text(), "  SetEnv LANG=en_US.UTF-8\n") {
		t.Fatalf("SetEnv did not survive the edit:\n%s", g.Text())
	}
}

func TestClearingAValueTakesTheLineOut(t *testing.T) {
	f := loadRaw(t, sampleConfig)
	b := f.Blocks[1]
	b.Options = append([]SSHOption(nil), b.Options...)
	b.Options[b.IndexOf("Port")].Value = ""

	g, err := f.Set(1, b)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(g.Text(), "Port") {
		t.Fatalf("clearing Port should remove its line:\n%s", g.Text())
	}
	if n := len(strings.Split(sampleConfig, "\n")) - len(strings.Split(g.Text(), "\n")); n != 1 {
		t.Fatalf("exactly one line should be gone, %d are", n)
	}
}

func TestANewOptionTakesTheBlocksIndentation(t *testing.T) {
	f := loadRaw(t, sampleConfig)
	b := f.Blocks[1]
	b.Options = append(append([]SSHOption(nil), b.Options...),
		SSHOption{Key: "ProxyJump", Value: "bastion"})

	g, err := f.Set(1, b)
	if err != nil {
		t.Fatal(err)
	}
	// Two spaces, because that is what every other option in this block uses —
	// a new line at column 0 would read as a different block.
	if !strings.Contains(g.Text(), "\n  ProxyJump bastion\n") {
		t.Fatalf("new option should match the block's indentation:\n%s", g.Text())
	}
	// It goes after the last keyword line, not before the blank that ends the
	// block and not at the very bottom of the file.
	if !strings.Contains(g.Text(), "  SetEnv LANG=en_US.UTF-8\n  ProxyJump bastion\n\nMatch") {
		t.Fatalf("new option landed in the wrong place:\n%s", g.Text())
	}
}

func TestARepeatedHostPatternKeepsBothBlocks(t *testing.T) {
	// Legal, and meaningful: ssh takes the FIRST value it finds for each
	// keyword, so this asks for port 22 with user deploy. Dropping the second
	// block the way hosts.yaml drops a repeated name would change what ssh does.
	const raw = "Host prod\n  Port 22\n\nHost prod\n  User deploy\n"
	f := loadRaw(t, raw)
	if len(f.Blocks) != 2 {
		t.Fatalf("both blocks must survive, got %d", len(f.Blocks))
	}
	if f.Blocks[0].Option("Port") != "22" || f.Blocks[1].Option("User") != "deploy" {
		t.Fatalf("blocks parsed wrong: %+v", f.Blocks)
	}
}

func TestMatchOptionsBelongToNobody(t *testing.T) {
	f := loadRaw(t, sampleConfig)
	for i, b := range f.Blocks {
		if b.IndexOf("ForwardAgent") >= 0 {
			t.Fatalf("block %d swallowed a Match option: %+v", i, b.Options)
		}
	}
}

func TestIncludeIsCountedNotFollowed(t *testing.T) {
	f := loadRaw(t, sampleConfig)
	if len(f.Unread) != 1 || f.Unread[0] != "conf.d/* — matches nothing" {
		t.Fatalf("the Include should be reported for disclosure, got %v", f.Unread)
	}
	if len(f.Blocks) != 2 {
		t.Fatalf("only the blocks in THIS file are listed, got %d", len(f.Blocks))
	}
}

func TestDeletingABlockTakesItsOwnCommentAndLeavesTheNextOnes(t *testing.T) {
	f := loadRaw(t, sampleConfig)
	g, err := f.Delete(0)
	if err != nil {
		t.Fatal(err)
	}
	out := g.Text()
	if strings.Contains(out, "# personal") {
		t.Fatalf("the deleted block's own label should go with it:\n%s", out)
	}
	if !strings.Contains(out, "# production\nHost prod prod-*") {
		t.Fatalf("the NEXT block's label must survive:\n%s", out)
	}
	// The file's own header sits a blank line clear of the block, so it is not
	// the block's label and must not be taken either.
	if !strings.HasPrefix(out, "# my ssh notes\n") {
		t.Fatalf("the file header is nobody's label:\n%s", out)
	}
}

func TestAddAppendsAtTheEnd(t *testing.T) {
	f := loadRaw(t, sampleConfig)
	g, err := f.Add(SSHBlock{Patterns: "gw", Options: []SSHOption{
		{Key: "HostName", Value: "198.51.100.10"},
		{Key: "User", Value: ""}, // empty: no line for it
	}})
	if err != nil {
		t.Fatal(err)
	}
	// At the END because ssh reads top to bottom and takes the first match —
	// inserting above an existing block can change what another host resolves to.
	if !strings.HasSuffix(g.Text(), "Include conf.d/*\n\nHost gw\n  HostName 198.51.100.10\n") {
		t.Fatalf("new block should be appended:\n%s", g.Text())
	}
	// Only the APPENDED part, not the whole file: the block above already has a
	// User line, and a whole-file search would pass whatever Add did.
	tail := strings.TrimPrefix(g.Text(), sampleConfig)
	if tail == g.Text() {
		t.Fatalf("Add rewrote what was already there:\n%s", g.Text())
	}
	if strings.Contains(tail, "User") {
		t.Fatalf("an option with no value should not get a line:\n%s", tail)
	}
}

func TestAddRefusesABlockWithNoPattern(t *testing.T) {
	if _, err := loadRaw(t, "").Add(SSHBlock{Patterns: "  "}); err == nil {
		t.Fatal("a Host block with no pattern matches nothing and must be refused")
	}
}

func TestLoadAndSaveLeaveTheFileAlone(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config")
	if err := os.WriteFile(p, []byte(sampleConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := LoadSSHConfigFrom(p)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := SaveSSHConfig(f); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != sampleConfig {
		t.Fatalf("a save with nothing changed must not rewrite the file:\n%s", raw)
	}
}

func TestSaveKeepsTheModeTheFileAlreadyHad(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config")
	if err := os.WriteFile(p, []byte(sampleConfig), 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := LoadSSHConfigFrom(p)
	if err != nil {
		t.Fatal(err)
	}
	g, err := f.Delete(0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := SaveSSHConfig(g); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	// ssh only objects when OTHERS can write the file. A user who deliberately
	// left it readable is not making a mistake sshu should correct.
	if fi.Mode().Perm() != 0o644 {
		t.Fatalf("mode should be left alone, got %04o", fi.Mode().Perm())
	}
}

func TestANewConfigIsWrittenAt0600(t *testing.T) {
	p := filepath.Join(t.TempDir(), ".ssh", "config")
	f, err := LoadSSHConfigFrom(p) // missing file is the empty state
	if err != nil {
		t.Fatal(err)
	}
	g, err := f.Add(SSHBlock{Patterns: "gw"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := SaveSSHConfig(g); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Fatalf("a file sshu creates should be 0600, got %04o", fi.Mode().Perm())
	}
}

func TestSaveRefusesWhenSomebodyElseWroteFirstAndHandsBackTheirFile(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config")
	if err := os.WriteFile(p, []byte(sampleConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := LoadSSHConfigFrom(p)
	if err != nil {
		t.Fatal(err)
	}
	g, err := f.Delete(0)
	if err != nil {
		t.Fatal(err)
	}

	// Another editor gets there first. sshu writes the WHOLE file back, so
	// going ahead would erase this without a trace.
	const theirs = "Host somewhere-else\n  User root\n"
	if err := os.WriteFile(p, []byte(theirs), 0o600); err != nil {
		t.Fatal(err)
	}

	fresh, err := SaveSSHConfig(g)
	if err == nil {
		t.Fatal("saving over somebody else's write must be refused")
	}
	if raw, _ := os.ReadFile(p); string(raw) != theirs {
		t.Fatalf("their file must be left exactly as it was:\n%s", raw)
	}
	// The refusal hands back what is actually on disk, so the caller can show
	// that instead of a stale list next to a message about staleness.
	if fresh.Text() != theirs || len(fresh.Blocks) != 1 ||
		fresh.Blocks[0].Patterns != "somewhere-else" {
		t.Fatalf("the refusal should carry the reloaded file, got %q", fresh.Text())
	}
	// And it is immediately saveable: the reloaded copy knows what is on disk.
	if _, err := SaveSSHConfig(fresh); err != nil {
		t.Fatalf("the reloaded file should save cleanly: %v", err)
	}
}

func TestEqualsSeparatedAndIndentedFormsRoundTrip(t *testing.T) {
	// `Key=Value`, tabs, and trailing whitespace are all legal here, and none of
	// them are sshu's to tidy up.
	const raw = "Host odd\n\tHostName=10.0.0.1\n  User   deploy   \n"
	f := loadRaw(t, raw)
	if f.Blocks[0].Option("HostName") != "10.0.0.1" || f.Blocks[0].Option("User") != "deploy" {
		t.Fatalf("parsed wrong: %+v", f.Blocks[0].Options)
	}
	if f.Text() != raw {
		t.Fatalf("round trip changed the file:\n%q", f.Text())
	}

	b := f.Blocks[0]
	b.Options = append([]SSHOption(nil), b.Options...)
	b.Options[0].Value = "10.0.0.2"
	g, _ := f.Set(0, b)
	if !strings.Contains(g.Text(), "\tHostName=10.0.0.2\n") {
		t.Fatalf("the separator and indentation should survive an edit:\n%q", g.Text())
	}
	if !strings.Contains(g.Text(), "  User   deploy   \n") {
		t.Fatalf("an untouched line should not be reformatted:\n%q", g.Text())
	}
}

func TestMatchesFollowsSshsRulesNotFilepathMatchs(t *testing.T) {
	for _, tc := range []struct {
		patterns, host string
		want           bool
	}{
		{"*", "anything", true},
		{"prod prod-*", "prod", true},
		{"prod prod-*", "prod-web-01", true},
		{"prod prod-*", "staging", false},
		{"gw", "GW", true},         // ssh lowercases before comparing
		{"GW", "gw", true},         // both sides
		{"db?", "db1", true},       // ? is exactly one
		{"db?", "db12", false},     //
		{"*", "", false},           // no destination, no match
		{"prod", "  prod  ", true}, // the caller's whitespace is not a pattern
	} {
		if got := (SSHBlock{Patterns: tc.patterns}).Matches(tc.host); got != tc.want {
			t.Errorf("Host %q vs %q = %v, want %v", tc.patterns, tc.host, got, tc.want)
		}
	}
}

// The two rules filepath.Match cannot express, which is the whole reason this
// matcher exists rather than a one-line delegation.
func TestMatchesHandlesNegationAndSlashes(t *testing.T) {
	b := SSHBlock{Patterns: "* !gw !bastion"}
	if b.Matches("prod") != true {
		t.Error("a positive * should still match everything else")
	}
	// A negated pattern vetoes the block even though `*` matched first — and
	// filepath.Match has no way to say this at all.
	if b.Matches("gw") {
		t.Error("!gw must veto the block")
	}
	if b.Matches("bastion") {
		t.Error("a later negation must veto too")
	}

	// `*` crosses a slash. filepath.Match refuses, and a hostname is entitled to
	// contain one.
	if !(SSHBlock{Patterns: "a*"}).Matches("a/b") {
		t.Error("* must cross a slash")
	}
	if ok, _ := filepath.Match("a*", "a/b"); ok {
		t.Error("setup: filepath.Match was supposed to disagree — if it no longer " +
			"does, this matcher's justification has changed")
	}
}

func TestEffectiveIsTheUnionWithTheFirstValueWinning(t *testing.T) {
	// Specific first, defaults last — the order ssh_config(5) tells people to
	// write, and the only order in which first-wins does what they meant.
	const raw = `Host prod
  HostName prod.example.internal
  Port 2222

Host *
  Port 22
  AddKeysToAgent yes
`
	got := loadRaw(t, raw).Effective("prod")
	want := []EffectiveOption{
		{Key: "HostName", Value: "prod.example.internal", From: "prod", Block: 0},
		{Key: "Port", Value: "2222", From: "prod", Block: 0},
		{Key: "AddKeysToAgent", Value: "yes", From: "*", Block: 1},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d options, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("option %d = %+v, want %+v", i, got[i], want[i])
		}
	}
	// The `Host *` Port is NOT in there: ssh took prod's 2222 and never looked
	// at the second one. A union that listed both would be listing a value ssh
	// will not use.
	for _, o := range got {
		if o.Key == "Port" && o.Value == "22" {
			t.Error("the second Port must not appear — first value wins")
		}
	}
}

func TestEffectiveIsEmptyWhenNothingMatches(t *testing.T) {
	const raw = "Host prod\n  HostName p.example.com\n"
	if got := loadRaw(t, raw).Effective("staging"); len(got) != 0 {
		t.Errorf("nothing should apply, got %+v", got)
	}
}

func TestMatchConditionsAreKeptSoTheAnswerCanNameWhatItLeftOut(t *testing.T) {
	got := loadRaw(t, sampleConfig).MatchConditions
	// The CONDITION, not a count: a reader who is told "1 Match block" still has
	// to go and find which one.
	if len(got) != 1 || got[0] != "host bastion" {
		t.Errorf("the sample's Match condition should be kept verbatim, got %v", got)
	}
	if n := len(loadRaw(t, "Host a\n  User x\n").MatchConditions); n != 0 {
		t.Errorf("no Match blocks here, got %d", n)
	}
	// And its options stay out of the union, because sshu cannot tell whether
	// the condition holds.
	for _, o := range loadRaw(t, sampleConfig).Effective("bastion") {
		if strings.EqualFold(o.Key, "ForwardAgent") {
			t.Errorf("a Match option must not be claimed as effective: %+v", o)
		}
	}
}

func TestABlockThatIsJustAHostLineTakesItsFirstOption(t *testing.T) {
	f := loadRaw(t, "Host lonely\n")
	b := f.Blocks[0]
	b.Options = []SSHOption{{Key: "User", Value: "deploy"}}
	g, err := f.Set(0, b)
	if err != nil {
		t.Fatal(err)
	}
	if g.Text() != "Host lonely\n  User deploy\n" {
		t.Fatalf("got %q", g.Text())
	}
}

// ------------------------------------------------------------------ Include

// tree writes a root plus named files under conf.d/ and loads it.
func tree(t *testing.T, root string, parts map[string]string) (SSHConfigFile, string) {
	t.Helper()
	dir := t.TempDir()
	if len(parts) > 0 {
		if err := os.Mkdir(filepath.Join(dir, "conf.d"), 0o700); err != nil {
			t.Fatal(err)
		}
		for name, body := range parts {
			if err := os.WriteFile(filepath.Join(dir, "conf.d", name), []byte(body), 0o600); err != nil {
				t.Fatal(err)
			}
		}
	}
	p := filepath.Join(dir, "config")
	if err := os.WriteFile(p, []byte(root), 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := LoadSSHConfigFrom(p)
	if err != nil {
		t.Fatal(err)
	}
	return f, dir
}

func patterns(f SSHConfigFile) []string {
	var out []string
	for _, b := range f.Blocks {
		out = append(out, b.Patterns)
	}
	return out
}

// ssh splices an included file AT THE POINT the directive appears, and that
// position is what decides which value wins. A tree that appended the included
// blocks at the end would resolve differently from ssh on the same file.
func TestIncludeIsSplicedWhereTheDirectiveSits(t *testing.T) {
	f, _ := tree(t, "Host first\n  User a\n\nInclude conf.d/mid\n\nHost last\n  User c\n",
		map[string]string{"mid": "Host middle\n  User b\n"})
	if got := patterns(f); len(got) != 3 || got[0] != "first" || got[1] != "middle" || got[2] != "last" {
		t.Fatalf("blocks are in the wrong order: %v", got)
	}
	if f.Files() != 2 {
		t.Errorf("the tree should be two files, got %d", f.Files())
	}
}

// The recursion has to save and restore "which block are we in": an Include is
// spliced INTO the block it sits in, so the lines after it still belong there.
func TestOptionsAfterAnIncludeStillBelongToTheBlockItSatIn(t *testing.T) {
	f, _ := tree(t, "Host prod\n  HostName p.example.com\n  Include conf.d/extra\n  Port 2222\n",
		map[string]string{"extra": "Host other\n  User x\n"})
	prod := f.Blocks[0]
	if prod.Patterns != "prod" {
		t.Fatalf("setup: %v", patterns(f))
	}
	if prod.Option("Port") != "2222" {
		t.Errorf("Port came after the Include and still belongs to prod: %+v", prod.Options)
	}
}

// A glob is read in sorted order, and order decides which value ssh keeps.
func TestAGlobIsReadInSortedOrder(t *testing.T) {
	f, _ := tree(t, "Include conf.d/*\n", map[string]string{
		"20-late":  "Host both\n  Port 20\n",
		"10-early": "Host both\n  Port 10\n",
	})
	if got := f.Effective("both"); len(got) == 0 || got[0].Value != "10" {
		t.Errorf("the sorted-first file's value should win, got %+v", got)
	}
}

func TestAnIncludedBlockIsEditedInItsOwnFileOnly(t *testing.T) {
	root := "Host gw\n  User me\n\nInclude conf.d/work\n"
	f, dir := tree(t, root, map[string]string{"work": "# team\nHost office\n  HostName 10.20.0.1\n"})

	i := -1
	for j, b := range f.Blocks {
		if b.Patterns == "office" {
			i = j
		}
	}
	if i < 0 {
		t.Fatalf("setup: %v", patterns(f))
	}
	b := f.Blocks[i]
	b.Options = append([]SSHOption(nil), b.Options...)
	b.Options[0].Value = "10.20.0.2"

	g, err := f.Set(i, b)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := SaveSSHConfig(g); err != nil {
		t.Fatal(err)
	}

	// The included file changed…
	got, _ := os.ReadFile(filepath.Join(dir, "conf.d", "work"))
	if string(got) != "# team\nHost office\n  HostName 10.20.0.2\n" {
		t.Errorf("the included file is wrong:\n%q", got)
	}
	// …and the root did not. A save that rewrote every file in the tree would
	// churn files nobody touched.
	if got, _ := os.ReadFile(filepath.Join(dir, "config")); string(got) != root {
		t.Errorf("the root must be untouched:\n%q", got)
	}
}

func TestAnIncludedBlockIsDeletedFromItsOwnFile(t *testing.T) {
	f, dir := tree(t, "Include conf.d/work\n\nHost gw\n  User me\n",
		map[string]string{"work": "# team\nHost office\n  HostName 10.20.0.1\n\nHost desk\n  User d\n"})
	i := -1
	for j, b := range f.Blocks {
		if b.Patterns == "office" {
			i = j
		}
	}
	g, err := f.Delete(i)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := SaveSSHConfig(g); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(filepath.Join(dir, "conf.d", "work"))
	if strings.Contains(string(got), "office") || strings.Contains(string(got), "# team") {
		t.Errorf("the block and its own label should be gone:\n%s", got)
	}
	if !strings.Contains(string(got), "Host desk") {
		t.Errorf("its neighbour must survive:\n%s", got)
	}
}

// [A] writes to the ROOT. An include tree has no obvious home for something
// new, and picking one by guessing would put a host in a file the user shares
// with a team.
func TestAddAlwaysGoesToTheRootFile(t *testing.T) {
	f, dir := tree(t, "Include conf.d/work\n", map[string]string{"work": "Host office\n  User o\n"})
	g, err := f.Add(SSHBlock{Patterns: "new", Options: []SSHOption{{Key: "User", Value: "me"}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := SaveSSHConfig(g); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "config")); !strings.Contains(string(got), "Host new") {
		t.Errorf("the root should have it:\n%s", got)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "conf.d", "work")); strings.Contains(string(got), "Host new") {
		t.Errorf("the included file must not:\n%s", got)
	}
}

func TestAnIncludeThatResolvesToNothingIsDisclosedByName(t *testing.T) {
	f, _ := tree(t, "Include conf.d/nope\nInclude conf.d/*\n\nHost gw\n  User me\n", nil)
	if len(f.Unread) == 0 {
		t.Fatal("an Include that went nowhere must be reported")
	}
	// By name — a count leaves the reader hunting through their own file.
	if !strings.Contains(strings.Join(f.Unread, " "), "conf.d/nope") {
		t.Errorf("it should name the pattern, got %v", f.Unread)
	}
}

func TestAnIncludeCycleStopsInsteadOfLooping(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "conf.d"), 0o700); err != nil {
		t.Fatal(err)
	}
	// a includes b, b includes a.
	if err := os.WriteFile(filepath.Join(dir, "conf.d", "b"),
		[]byte("Include config\nHost fromb\n  User b\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, "config")
	if err := os.WriteFile(p, []byte("Include conf.d/b\nHost root\n  User r\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	done := make(chan SSHConfigFile, 1)
	go func() {
		f, err := LoadSSHConfigFrom(p)
		if err != nil {
			t.Error(err)
		}
		done <- f
	}()
	select {
	case f := <-done:
		if got := patterns(f); len(got) != 2 {
			t.Errorf("each block once, got %v", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the cycle was followed instead of cut")
	}
}

// A save must not rewrite a file nobody edited: churning somebody's dotfiles
// repo is a diff they have to read and did not ask for.
func TestSaveWritesOnlyTheFilesThatChanged(t *testing.T) {
	f, dir := tree(t, "Include conf.d/work\n\nHost gw\n  User me\n",
		map[string]string{"work": "Host office\n  User o\n"})
	work := filepath.Join(dir, "conf.d", "work")
	before, err := os.Stat(work)
	if err != nil {
		t.Fatal(err)
	}

	i := -1
	for j, b := range f.Blocks {
		if b.Patterns == "gw" {
			i = j
		}
	}
	b := f.Blocks[i]
	b.Options = append([]SSHOption(nil), b.Options...)
	b.Options[0].Value = "you"
	g, _ := f.Set(i, b)
	if _, err := SaveSSHConfig(g); err != nil {
		t.Fatal(err)
	}

	after, err := os.Stat(work)
	if err != nil {
		t.Fatal(err)
	}
	if !after.ModTime().Equal(before.ModTime()) {
		t.Error("the untouched file was rewritten")
	}
}
