package store

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ~/.ssh/known_hosts is the second file sshu manages that is not sshu's, and
// the one with teeth: it is what stands between a connection and a machine
// pretending to be the one you meant.
//
// sshu already reads it — internal/remote/sftp.go verifies against it and
// REFUSES outright when a known host's key has changed, without offering a
// yes/no. That refusal is exactly the moment somebody needs this panel: the
// only way out of it today is `ssh-keygen -R` in another terminal.
//
// The line-preserving discipline is the same as sshconfig.go's and for a
// sharper reason — a real known_hosts is mostly annotation. One entry is one
// LINE, so there are no blocks here: identity is still the position, an edit
// rewrites one field of one line, and everything else is byte-for-byte.
//
// The one place it deliberately differs from Config: DELETING AN ENTRY DOES NOT
// TAKE THE COMMENT ABOVE IT. A comment over a `Host` block titles that block; a
// comment in known_hosts sits over a RUN of entries far more often than over
// one, and deleting somebody's section heading because they removed a row under
// it is the worse mistake.

// KnownHostEntry is one host-key line.
//
// Hosts is kept as the raw first field rather than a split list: it may be a
// comma-separated set of patterns, a `[host]:port` for a non-default port, or
// `|1|salt|hash` for a hashed name that CANNOT be read back — only tested
// against a candidate. Splitting it would mean re-joining it on the way out and
// getting somebody's file subtly wrong.
type KnownHostEntry struct {
	Marker  string // "@cert-authority" / "@revoked", or ""
	Hosts   string
	Type    string // ssh-ed25519, ecdsa-sha2-nistp256, ssh-rsa, sk-…
	Key     string // base64, as it sits in the file
	Comment string
	At      int // 1-based line, or 0 for an entry not on disk yet

	// byte range of the Hosts field within its line, so an edit to the name
	// rewrites those bytes and nothing else.
	from, to int
}

// Hashed reports whether the hostname is a HMAC rather than a name. Such an
// entry can be deleted and its key read, but the name it stands for cannot be
// recovered from the file — that is the whole point of hashing it.
func (e KnownHostEntry) Hashed() bool { return strings.HasPrefix(e.Hosts, "|1|") }

// Fingerprint is the SHA256 form ssh prints and people actually compare.
//
// Computed here rather than through x/crypto/ssh because it is a property of
// the stored bytes: base64 of the SHA-256 of the wire-format key, unpadded.
// That is precisely what ssh.FingerprintSHA256 does, without dragging a crypto
// handshake library into the package that reads files.
func (e KnownHostEntry) Fingerprint() string {
	raw, err := base64.StdEncoding.DecodeString(e.Key)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(raw)
	return "SHA256:" + base64.RawStdEncoding.EncodeToString(sum[:])
}

// KnownHostsFile is ~/.ssh/known_hosts: its lines, verbatim, plus the entries
// found among them. onDisk is what a save compares against — same reason as
// SSHConfigFile's, and ssh itself writes here too.
type KnownHostsFile struct {
	lines    []string
	trailing bool
	onDisk   string

	Entries []KnownHostEntry
}

// KnownHostsPath is ~/.ssh/known_hosts.
func KnownHostsPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".ssh", "known_hosts"), nil
}

// LoadKnownHosts reads ~/.ssh/known_hosts. Missing is the empty state: a
// machine that has never connected anywhere has no file, and that is not news.
func LoadKnownHosts() (KnownHostsFile, error) {
	path, err := KnownHostsPath()
	if err != nil {
		return KnownHostsFile{}, err
	}
	return LoadKnownHostsFrom(path)
}

// LoadKnownHostsFrom is LoadKnownHosts against an explicit path (tests).
//
// A line sshu cannot parse is not an error — it is a line, and it stays one.
// The obsolete SSH1 format and whatever OpenSSH adds next both land there.
func LoadKnownHostsFrom(path string) (KnownHostsFile, error) {
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return KnownHostsFile{}, nil
	}
	if err != nil {
		return KnownHostsFile{}, err
	}
	return parseKnownHostsFile(string(raw)), nil
}

func parseKnownHostsFile(raw string) KnownHostsFile {
	lines, trailing := splitConfigLines(raw)
	f := KnownHostsFile{lines: lines, trailing: trailing, onDisk: raw}
	f.reparse()
	return f
}

// Text is the file as it would be written.
func (f KnownHostsFile) Text() string {
	if len(f.lines) == 0 {
		return ""
	}
	out := strings.Join(f.lines, "\n")
	if f.trailing {
		out += "\n"
	}
	return out
}

func (f *KnownHostsFile) reparse() {
	f.Entries = nil
	for i, raw := range f.lines {
		if e, ok := parseKnownHostLine(raw); ok {
			e.At = i + 1
			f.Entries = append(f.Entries, e)
		}
	}
}

// field is one whitespace-delimited token and where it sat.
type field struct {
	text     string
	from, to int
}

func splitFields(line string) []field {
	var out []field
	i := 0
	for i < len(line) {
		for i < len(line) && (line[i] == ' ' || line[i] == '\t' || line[i] == '\r') {
			i++
		}
		if i >= len(line) {
			break
		}
		j := i
		for j < len(line) && line[j] != ' ' && line[j] != '\t' && line[j] != '\r' {
			j++
		}
		out = append(out, field{line[i:j], i, j})
		i = j
	}
	return out
}

func parseKnownHostLine(line string) (KnownHostEntry, bool) {
	if strings.TrimSpace(line) == "" || isComment(line) {
		return KnownHostEntry{}, false
	}
	fs := splitFields(line)
	var e KnownHostEntry
	if len(fs) > 0 && strings.HasPrefix(fs[0].text, "@") {
		e.Marker = fs[0].text
		fs = fs[1:]
	}
	// hosts, keytype, key — anything shorter is not an entry sshu understands,
	// and a line it does not understand is one it leaves alone.
	if len(fs) < 3 {
		return KnownHostEntry{}, false
	}
	e.Hosts, e.from, e.to = fs[0].text, fs[0].from, fs[0].to
	e.Type, e.Key = fs[1].text, fs[2].text
	if len(fs) > 3 {
		e.Comment = strings.TrimRight(line[fs[3].from:], " \t\r")
	}
	return e, true
}

// ----------------------------------------------------------------- mutations

// SetHosts rewrites which names entry i is trusted for, and nothing else. The
// key, the comment and every byte of spacing on that line stay exactly as they
// were — the key is not sshu's to retype, and an edit that reformatted the line
// around it would look like one that changed it.
func (f KnownHostsFile) SetHosts(i int, hosts string) (KnownHostsFile, error) {
	if i < 0 || i >= len(f.Entries) {
		return f, fmt.Errorf("no entry at index %d", i)
	}
	hosts = strings.TrimSpace(hosts)
	if hosts == "" {
		return f, fmt.Errorf("a host key with no host is trusted for nothing")
	}
	if strings.ContainsAny(hosts, " \t") {
		return f, fmt.Errorf("separate several names with commas, not spaces")
	}
	e := f.Entries[i]
	ln := e.At - 1
	line := f.lines[ln][:e.from] + hosts + f.lines[ln][e.to:]
	return f.spliceLines(ln, ln+1, []string{line}), nil
}

// Add appends one entry. At the end because this file has no order that means
// anything — ssh scans it for a match, and a new line at the bottom is exactly
// what ssh's own first-connect prompt writes.
func (f KnownHostsFile) Add(e KnownHostEntry) (KnownHostsFile, error) {
	if strings.TrimSpace(e.Hosts) == "" {
		return f, fmt.Errorf("a host key with no host is trusted for nothing")
	}
	if strings.TrimSpace(e.Type) == "" || strings.TrimSpace(e.Key) == "" {
		return f, fmt.Errorf("a known host needs a key")
	}
	var parts []string
	if e.Marker != "" {
		parts = append(parts, e.Marker)
	}
	parts = append(parts, strings.TrimSpace(e.Hosts), strings.TrimSpace(e.Type),
		strings.TrimSpace(e.Key))
	if c := strings.TrimSpace(e.Comment); c != "" {
		parts = append(parts, c)
	}
	g := f.spliceLines(len(f.lines), len(f.lines), []string{strings.Join(parts, " ")})
	g.trailing = true
	return g, nil
}

// Delete removes entry i's line — and ONLY its line. See the package comment: a
// comment here sits over a run of entries more often than over one.
func (f KnownHostsFile) Delete(i int) (KnownHostsFile, error) {
	if i < 0 || i >= len(f.Entries) {
		return f, fmt.Errorf("no entry at index %d", i)
	}
	ln := f.Entries[i].At - 1
	return f.spliceLines(ln, ln+1, nil), nil
}

func (f KnownHostsFile) spliceLines(from, to int, body []string) KnownHostsFile {
	lines := make([]string, 0, len(f.lines)-(to-from)+len(body))
	lines = append(lines, f.lines[:from]...)
	lines = append(lines, body...)
	lines = append(lines, f.lines[to:]...)

	g := KnownHostsFile{lines: lines, trailing: f.trailing, onDisk: f.onDisk}
	if len(lines) == 0 {
		g.trailing = false
	}
	g.reparse()
	return g
}

// -------------------------------------------------------------------- saving

// SaveKnownHosts writes ~/.ssh/known_hosts.
func SaveKnownHosts(f KnownHostsFile) (KnownHostsFile, error) {
	path, err := KnownHostsPath()
	if err != nil {
		return f, err
	}
	return SaveKnownHostsTo(path, f)
}

// SaveKnownHostsTo is SaveKnownHosts against an explicit path (tests). Same
// contract as SaveSSHConfigTo, and it matters more here: ssh appends to this
// file every time it meets a host for the first time, so "somebody else wrote
// while sshu had it open" is not a rare case, it is Tuesday.
//
// The returned file is always what is now ON DISK — after a successful write,
// after a refusal, and after a failed write alike — so the caller can adopt it
// without having to ask which happened.
func SaveKnownHostsTo(path string, f KnownHostsFile) (KnownHostsFile, error) {
	mode := os.FileMode(0o600)
	raw, err := os.ReadFile(path)
	switch {
	case os.IsNotExist(err):
		if f.onDisk != "" {
			return KnownHostsFile{}, fmt.Errorf("%s was deleted while sshu had it open", FoldHome(path))
		}
	case err != nil:
		return f, err
	default:
		if string(raw) != f.onDisk {
			return parseKnownHostsFile(string(raw)),
				fmt.Errorf("%s changed on disk — reloaded, make the change again", FoldHome(path))
		}
		if fi, ferr := os.Stat(path); ferr == nil {
			mode = fi.Mode().Perm()
		}
	}

	out := f.Text()
	if err := writeFileMode(path, []byte(out), mode); err != nil {
		return parseKnownHostsFile(f.onDisk), err
	}
	f.onDisk = out
	return f, nil
}
