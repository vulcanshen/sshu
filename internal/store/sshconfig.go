package store

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ~/.ssh/config is the one file sshu manages that is NOT sshu's — except that
// it is usually more than one file, because of Include.
//
// It matters here because tab [3] shells out to the real ssh binary
// (internal/ui/session.go), so every option in it is already steering sshu's
// sessions — HostName, ProxyJump, ForwardAgent and the rest arrive without sshu
// passing or even seeing them. Tab [2]/[4] speak SFTP themselves and read none
// of it, which is a real asymmetry the panel makes visible.
//
// Everything below serves one rule: EDITING A BLOCK CHANGES ONLY THAT BLOCK'S
// LINES, IN ITS OWN FILE. A parse-to-struct-and-marshal round trip would be a
// third of the code and would silently eat comments, Match blocks, the global
// options above the first Host, and every keyword sshu does not know — in files
// that git, rsync, scp and half the editors on the machine also read. So the
// parse keeps each file's own lines, and a write rebuilds only the range it
// touched, in the file that range lives in.
//
// Three consequences worth stating up front:
//
//   - A repeated Host pattern is NOT dropped. ssh takes the first value it
//     finds for each keyword, so two `Host prod` blocks both apply and the
//     second is meaningful. That is the opposite of hosts.yaml and
//     credentials.yaml, where a repeat is unreachable and gets dropped at the
//     door (see dedupeByName). Blocks here are addressed by POSITION.
//   - Include is FOLLOWED, at the point it appears, which is where ssh splices
//     it. The blocks it brings in are listed and editable like any other; they
//     just belong to a different file, and every question about them says so.
//   - What still cannot be answered is disclosed rather than omitted: a `Match`
//     condition sshu does not evaluate, an Include that resolves to nothing.
//     See Unread and MatchConditions.

// maxIncludeDepth is OpenSSH's own limit. A config nested deeper than this is
// one ssh itself refuses, so stopping here agrees with it rather than inventing
// a rule of sshu's own.
const maxIncludeDepth = 16

// SSHOption is one `Keyword value` line inside a Host block.
//
// At is the 1-based line it came from, or 0 for an option that is not on disk
// yet. Zero-meaning-new is deliberate: a caller building an option by hand gets
// the right answer from the zero value, where a 0-based index would silently
// claim line one.
type SSHOption struct {
	Key   string
	Value string
	At    int
}

// SSHBlock is one `Host <patterns>` block: the patterns, and every keyword line
// under it up to the next Host or Match IN THE SAME FILE.
//
// Options keeps EVERY line, including a keyword that appears twice. ssh honours
// only the first, so the second is dead weight — but it is the user's dead
// weight, and dropping it on load would delete a line from their file the next
// time they saved anything.
type SSHBlock struct {
	Patterns string
	Options  []SSHOption
	file     int // which of SSHConfigFile.files this block lives in
	head     int // first line the block owns — the comments labelling it, if any
	start    int // index of the Host line
	end      int // one past the last line this block owns
}

// configFile is one file of the include tree.
type configFile struct {
	path     string
	lines    []string
	trailing bool // the file ended with a newline
	// onDisk is the exact content read at load time. It is what a save compares
	// against to notice that another editor got there first — the check
	// hosts.yaml does not need and these files do, because vim, VS Code and a
	// dotfiles repo all write here too.
	onDisk string
}

// SSHConfigFile is ~/.ssh/config and everything it includes.
type SSHConfigFile struct {
	root  string
	files []configFile

	Blocks []SSHBlock
	// MatchConditions is each `Match` line's condition, verbatim. They are
	// conditional and sshu does not evaluate them, so anything claiming to be
	// "what ssh will use" has to disclose them — by NAME rather than by count,
	// because a count tells nobody which part of their file to go and read.
	MatchConditions []string
	// Unread is each Include sshu could not follow, as the file said it: a
	// pattern matching nothing, a file it could not read, a nesting too deep.
	// Same rule as MatchConditions — named, not counted.
	Unread []string
}

// Files is how many files the tree turned out to be, root included.
func (f SSHConfigFile) Files() int { return len(f.files) }

// Path is where file i lives, in ~ form.
func (f SSHConfigFile) Path(i int) string {
	if i < 0 || i >= len(f.files) {
		return ""
	}
	return FoldHome(f.files[i].path)
}

// File is which of them this block belongs to.
func (b SSHBlock) File() int { return b.file }

// LineRange is the block's extent in ITS file, 1-based and inclusive.
func (b SSHBlock) LineRange() (int, int) { return b.start + 1, b.end }

// Option returns the value of the first line with this keyword, or "". First
// rather than last because that is the one ssh will use.
func (b SSHBlock) Option(key string) string {
	if i := b.IndexOf(key); i >= 0 {
		return b.Options[i].Value
	}
	return ""
}

// IndexOf is Option's position, or -1. Keywords are case-insensitive in ssh, so
// the match is too.
func (b SSHBlock) IndexOf(key string) int {
	for i, o := range b.Options {
		if strings.EqualFold(o.Key, key) {
			return i
		}
	}
	return -1
}

// Matches reports whether ssh would apply this block to a destination.
//
// The rules are OpenSSH's, not filepath.Match's: patterns are separated by
// whitespace, `*` is any run and `?` is exactly one character, matching is
// case-insensitive, and a pattern prefixed with `!` is a NEGATION that vetoes
// the whole block however many positive patterns also matched. filepath.Match
// gets two of those wrong — it has no negation, and it refuses to let `*` cross
// a `/`, which a hostname is entitled to contain.
func (b SSHBlock) Matches(host string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "" {
		return false
	}
	hit := false
	for _, p := range strings.Fields(b.Patterns) {
		if neg, ok := strings.CutPrefix(p, "!"); ok {
			if matchPattern(host, strings.ToLower(neg)) {
				return false
			}
			continue
		}
		if matchPattern(host, strings.ToLower(p)) {
			hit = true
		}
	}
	return hit
}

// matchPattern is `*`/`?` globbing, iterative so that a pattern like `*a*a*a*`
// cannot turn into exponential backtracking. Byte-wise rather than rune-wise:
// a hostname is ASCII (an internationalised one arrives already punycoded), so
// the two agree, and bytes keep `?` meaning one character rather than one byte
// of a multi-byte one.
func matchPattern(s, pat string) bool {
	si, pi, star, mark := 0, 0, -1, 0
	for si < len(s) {
		switch {
		case pi < len(pat) && (pat[pi] == '?' || pat[pi] == s[si]):
			si, pi = si+1, pi+1
		case pi < len(pat) && pat[pi] == '*':
			star, mark = pi, si
			pi++
		case star >= 0:
			pi, mark = star+1, mark+1
			si = mark
		default:
			return false
		}
	}
	for pi < len(pat) && pat[pi] == '*' {
		pi++
	}
	return pi == len(pat)
}

// EffectiveOption is one keyword ssh will actually use for a destination, and
// where it came from. Block is the identity (two blocks may share a pattern);
// From is what to show.
type EffectiveOption struct {
	Key   string
	Value string
	From  string
	Block int
}

// Effective is what ssh resolves for this destination: every matching block
// merged, the FIRST value for each keyword winning.
//
// The merge across all matching blocks is the file's real semantic —
// ssh_config(5): "Since the first obtained value for each parameter is used,
// more host-specific declarations should be given near the beginning of the
// file, and general defaults at the end." So "which block does this host match"
// is the wrong question: a host normally matches several and takes something
// from each.
//
// It is still not the whole truth, and MatchConditions and Unread are how a
// caller knows what is missing — a list headed "what ssh will use" that quietly
// omits a source is worse than no list at all.
func (f SSHConfigFile) Effective(host string) []EffectiveOption {
	seen := make(map[string]bool)
	var out []EffectiveOption
	for i, b := range f.Blocks {
		if !b.Matches(host) {
			continue
		}
		for _, o := range b.Options {
			k := strings.ToLower(o.Key)
			if seen[k] {
				continue
			}
			seen[k] = true
			out = append(out, EffectiveOption{
				Key: o.Key, Value: o.Value, From: b.Patterns, Block: i,
			})
		}
	}
	return out
}

// ------------------------------------------------------------------- loading

// SSHConfigPath is ~/.ssh/config.
func SSHConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".ssh", "config"), nil
}

// LoadSSHConfig reads ~/.ssh/config and everything it includes. A missing file
// is the empty state, not an error — plenty of machines have never had one, and
// [A] is how the first block gets written.
func LoadSSHConfig() (SSHConfigFile, error) {
	path, err := SSHConfigPath()
	if err != nil {
		return SSHConfigFile{}, err
	}
	return LoadSSHConfigFrom(path)
}

// LoadSSHConfigFrom is LoadSSHConfig against an explicit root (tests).
//
// There is no parse error to return. Every line is either a directive, a
// comment or blank, and a line sshu cannot make sense of is simply not a
// directive — the file keeps it and the panel does not show it. Refusing to
// start over a keyword OpenSSH added last month would be the wrong trade. An
// Include that cannot be followed lands in Unread for the same reason.
func LoadSSHConfigFrom(path string) (SSHConfigFile, error) {
	f := SSHConfigFile{root: path}
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return f, nil
	}
	if err != nil {
		return f, err
	}
	f.files = append(f.files, newConfigFile(path, string(raw)))
	f.reparse()
	return f, nil
}

func newConfigFile(path, raw string) configFile {
	lines, trailing := splitConfigLines(raw)
	return configFile{path: path, lines: lines, trailing: trailing, onDisk: raw}
}

// splitConfigLines splits without inventing or losing a trailing newline, so
// TextOf on an untouched file returns exactly what was read.
func splitConfigLines(raw string) ([]string, bool) {
	if raw == "" {
		return nil, false
	}
	trailing := strings.HasSuffix(raw, "\n")
	if trailing {
		raw = raw[:len(raw)-1]
	}
	return strings.Split(raw, "\n"), trailing
}

// Text is the root file as it would be written; TextOf reaches the others.
func (f SSHConfigFile) Text() string { return f.TextOf(0) }

func (f SSHConfigFile) TextOf(i int) string {
	if i < 0 || i >= len(f.files) {
		return ""
	}
	c := f.files[i]
	if len(c.lines) == 0 {
		return ""
	}
	out := strings.Join(c.lines, "\n")
	if c.trailing {
		out += "\n"
	}
	return out
}

// ---------------------------------------------------------------- directives

// directive is one keyword line taken apart into pieces that put back together
// byte-for-byte. Keeping the separator and the trailing whitespace is what lets
// an untouched line survive a save unchanged.
type directive struct {
	indent  string
	keyword string
	sep     string // whitespace, and at most one '=', between keyword and value
	value   string
	trail   string // trailing whitespace and CR
}

func parseDirective(raw string) (directive, bool) {
	var d directive
	line, cr := raw, ""
	if strings.HasSuffix(line, "\r") {
		line, cr = line[:len(line)-1], "\r"
	}
	i := 0
	for i < len(line) && (line[i] == ' ' || line[i] == '\t') {
		i++
	}
	d.indent = line[:i]
	if i >= len(line) || line[i] == '#' {
		return d, false
	}
	j := i
	for j < len(line) && line[j] != ' ' && line[j] != '\t' && line[j] != '=' {
		j++
	}
	d.keyword = line[i:j]

	k := j
	for k < len(line) && (line[k] == ' ' || line[k] == '\t') {
		k++
	}
	// OpenSSH allows exactly one '=' as the separator, with optional space
	// either side. Anything after it is the value, '=' included.
	if k < len(line) && line[k] == '=' {
		k++
		for k < len(line) && (line[k] == ' ' || line[k] == '\t') {
			k++
		}
	}
	d.sep = line[j:k]

	rest := line[k:]
	e := len(rest)
	for e > 0 && (rest[e-1] == ' ' || rest[e-1] == '\t') {
		e--
	}
	d.value, d.trail = rest[:e], rest[e:]+cr
	return d, true
}

// setValue rewrites one directive's value, keeping everything else the line
// had. An unchanged value returns the ORIGINAL string, so a block can be saved
// without a single byte moving on the lines nobody edited.
func setValue(raw, value string) string {
	d, ok := parseDirective(raw)
	if !ok || d.value == value {
		return raw
	}
	sep := d.sep
	if sep == "" {
		sep = " "
	}
	return d.indent + d.keyword + sep + value + d.trail
}

func isComment(line string) bool {
	return strings.HasPrefix(strings.TrimSpace(line), "#")
}

// ------------------------------------------------------------------- parsing

// reparse rebuilds Blocks from the loaded files, following Include as it goes.
// Every mutation edits lines and then calls this, so the model can never drift
// from the bytes it describes.
//
// It reads a file it has not seen before, and NEVER re-reads one it has: the
// lines in memory may be a mutation waiting to be saved, and going back to disk
// would throw it away.
func (f *SSHConfigFile) reparse() {
	f.Blocks, f.MatchConditions, f.Unread = nil, nil, nil
	if len(f.files) == 0 {
		return
	}
	f.scan(0, 1, map[string]bool{})
}

// scan walks one file, recursing into its Includes AT THE POINT THEY APPEAR —
// which is where ssh splices them, and therefore where their values sit in the
// first-wins order.
//
// `cur` is local, so the recursion saves and restores it for free: options
// after an Include still belong to the block the Include sat in, and an
// included file's own leading options (before its first Host) belong to nobody,
// exactly as the root file's do.
func (f *SSHConfigFile) scan(fi, depth int, seen map[string]bool) {
	path := f.files[fi].path
	if seen[path] {
		return // a cycle; ssh would loop, so stop rather than follow it
	}
	seen[path] = true
	defer delete(seen, path)

	var mine, barriers []int
	cur, inMatch := -1, false

	for li, raw := range f.files[fi].lines {
		d, ok := parseDirective(raw)
		if !ok {
			continue
		}
		switch strings.ToLower(d.keyword) {
		case "host":
			barriers = append(barriers, li)
			f.Blocks = append(f.Blocks, SSHBlock{
				Patterns: d.value, file: fi, start: li, end: li + 1,
			})
			mine = append(mine, len(f.Blocks)-1)
			cur, inMatch = len(f.Blocks)-1, false
			continue
		case "match":
			barriers = append(barriers, li)
			f.MatchConditions = append(f.MatchConditions, d.value)
			cur, inMatch = -1, true
			continue
		case "include":
			if depth >= maxIncludeDepth {
				f.Unread = append(f.Unread, d.value+" — nested too deep")
				continue
			}
			for _, j := range f.follow(d.value) {
				f.scan(j, depth+1, seen)
			}
			continue
		}
		if cur >= 0 && !inMatch {
			f.Blocks[cur].Options = append(f.Blocks[cur].Options,
				SSHOption{Key: d.keyword, Value: d.value, At: li + 1})
			f.Blocks[cur].end = li + 1
		}
	}
	f.settle(fi, mine, barriers)
}

// settle gives each of this file's blocks its full extent.
//
// A comment belongs to what FOLLOWS it. `# production` sitting above
// `Host prod` is that block's label, so deleting the block above must not take
// it along, and deleting prod itself must. So each block gives up its trailing
// comments and takes the ones directly overhead — directly meaning with no
// blank line between, which is what separates a label from the file's own
// header two paragraphs up.
func (f *SSHConfigFile) settle(fi int, mine, barriers []int) {
	lines := f.files[fi].lines
	for n, bi := range mine {
		b := &f.Blocks[bi]
		limit := len(lines)
		for _, x := range barriers {
			if x > b.start {
				limit = x
				break
			}
		}
		lastDir := b.end - 1
		e := limit
		for e-1 > lastDir && isComment(lines[e-1]) {
			e--
		}
		b.end = e

		floor := 0
		if n > 0 {
			floor = f.Blocks[mine[n-1]].end
		}
		h := b.start
		for h > floor && isComment(lines[h-1]) {
			h--
		}
		b.head = h
	}
}

// follow resolves one Include directive to the files it names, loading any it
// has not seen. Anything it cannot resolve goes to Unread rather than nowhere.
//
// The rules are OpenSSH's: several whitespace-separated pathnames per
// directive, `~` expanded, and a RELATIVE path taken against ~/.ssh rather than
// the working directory — which is why the ROOT's directory is the base, not
// the including file's.
func (f *SSHConfigFile) follow(value string) []int {
	base := filepath.Dir(f.files[0].path)
	var out []int
	for _, raw := range strings.Fields(value) {
		p := ExpandTilde(raw)
		if !filepath.IsAbs(p) {
			p = filepath.Join(base, p)
		}
		matches, err := filepath.Glob(p)
		if err != nil || len(matches) == 0 {
			f.Unread = append(f.Unread, raw+" — matches nothing")
			continue
		}
		// ssh reads a glob in sorted order, and order decides which value wins.
		sort.Strings(matches)
		for _, m := range matches {
			if j := f.indexOfFile(m); j >= 0 {
				out = append(out, j)
				continue
			}
			body, err := os.ReadFile(m)
			if err != nil {
				f.Unread = append(f.Unread, FoldHome(m)+" — "+err.Error())
				continue
			}
			f.files = append(f.files, newConfigFile(m, string(body)))
			out = append(out, len(f.files)-1)
		}
	}
	return out
}

func (f SSHConfigFile) indexOfFile(path string) int {
	for i, c := range f.files {
		if c.path == path {
			return i
		}
	}
	return -1
}

// ----------------------------------------------------------------- mutations

// Set rewrites block i in place, in its own file.
//
// Only the lines this block owns are rebuilt, and inside that range only what
// changed: an option whose value is untouched keeps its exact line, comments
// and blanks between the options survive where they sat, an option dropped from
// b.Options loses its line, and one that is new to the block is added after the
// last keyword line at the indentation the block already uses.
//
// An option with an empty value is a removal. There is no such thing as a
// keyword with no argument in the file, so "clear the field" is the only
// gesture that could mean "take this line out", and it is the one people reach
// for.
func (f SSHConfigFile) Set(i int, b SSHBlock) (SSHConfigFile, error) {
	if i < 0 || i >= len(f.Blocks) {
		return f, fmt.Errorf("no block at index %d", i)
	}
	if strings.TrimSpace(b.Patterns) == "" {
		return f, fmt.Errorf("Host pattern is required")
	}
	blk := f.Blocks[i]
	lines := f.files[blk.file].lines

	keep := make(map[int]SSHOption, len(b.Options))
	var fresh []SSHOption
	for _, o := range b.Options {
		if strings.TrimSpace(o.Key) == "" || strings.TrimSpace(o.Value) == "" {
			continue
		}
		if at := o.At - 1; at > blk.start && at < blk.end {
			keep[at] = o
			continue
		}
		fresh = append(fresh, o)
	}

	body := []string{setValue(lines[blk.start], strings.TrimSpace(b.Patterns))}
	last, indent := 0, ""
	for ln := blk.start + 1; ln < blk.end; ln++ {
		d, isDir := parseDirective(lines[ln])
		if !isDir {
			body = append(body, lines[ln])
			continue
		}
		if indent == "" {
			indent = d.indent
		}
		o, ok := keep[ln]
		if !ok {
			continue
		}
		body = append(body, setValue(lines[ln], strings.TrimSpace(o.Value)))
		last = len(body) - 1
	}
	if indent == "" {
		indent = "  "
	}

	out := make([]string, 0, len(body)+len(fresh))
	out = append(out, body[:last+1]...)
	for _, o := range fresh {
		out = append(out, indent+strings.TrimSpace(o.Key)+" "+strings.TrimSpace(o.Value))
	}
	out = append(out, body[last+1:]...)

	return f.splice(blk.file, blk.start, blk.end, out), nil
}

// Add appends a new block to the ROOT file, at the end.
//
// The root because it is the file the user opened and the only one sshu can
// pick without guessing — an include tree has no obvious home for something
// new. The end because ssh reads top to bottom and takes the first match, so
// inserting above an existing block can change what an unrelated host resolves
// to.
func (f SSHConfigFile) Add(b SSHBlock) (SSHConfigFile, error) {
	if strings.TrimSpace(b.Patterns) == "" {
		return f, fmt.Errorf("Host pattern is required")
	}
	if len(f.files) == 0 {
		if f.root == "" {
			return f, fmt.Errorf("no config file to write to")
		}
		g := f
		g.files = []configFile{newConfigFile(f.root, "")}
		f = g
	}
	lines := f.files[0].lines

	var out []string
	if len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) != "" {
		out = append(out, "")
	}
	out = append(out, "Host "+strings.TrimSpace(b.Patterns))
	for _, o := range b.Options {
		if strings.TrimSpace(o.Key) == "" || strings.TrimSpace(o.Value) == "" {
			continue
		}
		out = append(out, "  "+strings.TrimSpace(o.Key)+" "+strings.TrimSpace(o.Value))
	}
	g := f.splice(0, len(lines), len(lines), out)
	// A config that does not end in a newline is one ssh has to guess at, and
	// the next block appended after it would land on the same line.
	g.files[0].trailing = true
	g.reparse()
	return g, nil
}

// Delete removes block i: its own comment heading, its Host line, its options,
// and the blank lines under it — from the file it lives in.
func (f SSHConfigFile) Delete(i int) (SSHConfigFile, error) {
	if i < 0 || i >= len(f.Blocks) {
		return f, fmt.Errorf("no block at index %d", i)
	}
	blk := f.Blocks[i]
	return f.splice(blk.file, blk.head, blk.end, nil), nil
}

// splice replaces lines[from:to] of one file and re-parses. It copies rather
// than editing in place: f is a value, and a mutation that reached back into
// the caller's slice would change a file nobody asked to change.
func (f SSHConfigFile) splice(fi, from, to int, body []string) SSHConfigFile {
	old := f.files[fi]
	lines := make([]string, 0, len(old.lines)-(to-from)+len(body))
	lines = append(lines, old.lines[:from]...)
	lines = append(lines, body...)
	lines = append(lines, old.lines[to:]...)

	g := f
	g.files = append([]configFile(nil), f.files...)
	g.files[fi].lines = lines
	if len(lines) == 0 {
		g.files[fi].trailing = false
	}
	g.reparse()
	return g
}

// -------------------------------------------------------------------- saving

// SaveSSHConfig writes back every file whose content changed, and no others.
//
// Each file is checked on its own, because sshu rewrites a WHOLE file: one that
// another editor touched has to be refused, and refusing it must not depend on
// which other files happened to change. On any refusal the tree comes back
// reloaded from disk, so the caller can put what is actually there on screen
// rather than leaving a stale list next to a message about staleness.
//
// The mode is whatever each file already had. 0600 is the right answer for a
// new one, but a config somebody deliberately left at 0644 is not sshu's to
// narrow — ssh itself only objects when others can WRITE it.
func SaveSSHConfig(f SSHConfigFile) (SSHConfigFile, error) {
	var failed error
	for i := range f.files {
		out := f.TextOf(i)
		if out == f.files[i].onDisk {
			continue
		}
		if err := writeConfigFile(f.files[i], out); err != nil {
			failed = err
			break
		}
		f.files[i].onDisk = out
	}
	if failed == nil {
		return f, nil
	}
	fresh, err := LoadSSHConfigFrom(f.rootPath())
	if err != nil {
		return f, failed
	}
	return fresh, failed
}

func (f SSHConfigFile) rootPath() string {
	if len(f.files) > 0 {
		return f.files[0].path
	}
	return f.root
}

func writeConfigFile(c configFile, out string) error {
	mode := os.FileMode(0o600)
	raw, err := os.ReadFile(c.path)
	switch {
	case os.IsNotExist(err):
		if c.onDisk != "" {
			return fmt.Errorf("%s was deleted while sshu had it open", FoldHome(c.path))
		}
	case err != nil:
		return err
	default:
		if string(raw) != c.onDisk {
			return fmt.Errorf("%s changed on disk — reloaded, make the change again",
				FoldHome(c.path))
		}
		if fi, ferr := os.Stat(c.path); ferr == nil {
			mode = fi.Mode().Perm()
		}
	}
	return writeFileMode(c.path, []byte(out), mode)
}
