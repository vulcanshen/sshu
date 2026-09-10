package store

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// AuthMethod is how sshu authenticates to a host. The two values are the whole
// vocabulary — the UI picks the card's glyph off this, and the edit form shows
// the identity-file row or the password row depending on it.
type AuthMethod string

const (
	AuthPassword   AuthMethod = "password"
	AuthPrivateKey AuthMethod = "privatekey"
	// AuthCredential defers to a named entry in credentials.yaml, which supplies
	// user and the concrete auth together — see Resolve.
	AuthCredential AuthMethod = "credential"
)

// DefaultPort is what a new host gets when the form's Port field is left alone.
const DefaultPort = 22

// Host is one entry in hosts.yaml. Name is the key: it is what the card's first
// row shows and what CRUD locates a host by, so it must be unique.
//
// IdentityFile and Password are siblings selected by Auth, not a nested map —
// exactly one of them is meaningful for any given host.
//
// SECURITY: Password is stored in plaintext (a deliberate, recorded decision —
// see docs/sshu-ui-design.md §8.3). Save keeps hosts.yaml at 0600 and the UI
// never renders the value, but a copied or synced file leaks it. Password I/O
// is deliberately confined to this file so a keychain-backed store can replace
// it without touching the rest of sshu.
type Host struct {
	Name         string     `yaml:"name"`
	Host         string     `yaml:"host"`
	Port         int        `yaml:"port"`
	User         string     `yaml:"user"`
	Auth         AuthMethod `yaml:"auth"`
	IdentityFile string     `yaml:"identity_file,omitempty"`
	Password     string     `yaml:"password,omitempty"`
	// Credential names an entry in credentials.yaml when Auth is "credential".
	// The credential supplies User too, so User may be empty on such a host.
	Credential string `yaml:"credential,omitempty"`
	// Tags are the user's own words for grouping hosts — "prod", "tokyo",
	// "needs-vpn". They carry no meaning to sshu beyond being shown and
	// searched, which is the point: a field sshu interprets is a field the user
	// has to learn the rules of.
	//
	// Space is the only separator. Everything else is literal, so a tag can be
	// "web/db" or "k8s:prod" without an escaping rule to remember.
	Tags []string `yaml:"tags,omitempty"`
}

// Addr is the ssh-native "user@host:port" rendering, used by the connect
// confirmation. The card shows the parts on separate rows instead.
func (h Host) Addr() string {
	return fmt.Sprintf("%s@%s:%d", h.User, h.Host, h.Port)
}

// Validate reports what is wrong with a host, or nil. Uniqueness of Name is a
// property of the whole list, so it is checked in File.Validate, not here.
func (h Host) Validate() error {
	switch {
	case strings.TrimSpace(h.Name) == "":
		return fmt.Errorf("name is required")
	case strings.TrimSpace(h.Host) == "":
		return fmt.Errorf("host is required")
	// A credential host has no user of its own — the credential supplies it.
	case strings.TrimSpace(h.User) == "" && h.Auth != AuthCredential:
		return fmt.Errorf("user is required")
	case h.Port < 1 || h.Port > 65535:
		return fmt.Errorf("port must be 1-65535, got %d", h.Port)
	case h.Auth == AuthCredential && strings.TrimSpace(h.Credential) == "":
		return fmt.Errorf("auth is credential but no credential is named")
	case h.Auth != AuthPassword && h.Auth != AuthPrivateKey && h.Auth != AuthCredential:
		return fmt.Errorf("auth must be %q, %q or %q, got %q",
			AuthPassword, AuthPrivateKey, AuthCredential, h.Auth)
	}
	return nil
}

// Resolve returns the host as ssh will actually see it. A credential host
// takes user and auth wholesale from the named credential — the credential is
// one package, not a set of defaults the host can partially override. The two
// concrete methods pass through untouched.
func Resolve(h Host, creds []Credential) (Host, error) {
	if h.Auth != AuthCredential {
		return h, nil
	}
	for _, c := range creds {
		if c.Name == h.Credential {
			h.User, h.Auth = c.User, c.Auth
			h.IdentityFile, h.Password = c.IdentityFile, c.Password
			return h, nil
		}
	}
	return h, fmt.Errorf("host %q: credential %q is not in credentials.yaml",
		h.Name, h.Credential)
}

// File is the whole hosts.yaml document.
type File struct {
	Version int    `yaml:"version"`
	Hosts   []Host `yaml:"hosts"`
	// UnreadableSecrets names the hosts whose stored password would not
	// decrypt — a key that was replaced, or a file edited by hand. Not part of
	// the document: it describes THIS read, and the caller turns it into
	// something the user can see.
	//
	// Those hosts keep their ciphertext. sshu runs with a password it cannot
	// use, which fails at connect time, and the secret is still on disk for
	// the right key to open later.
	UnreadableSecrets []string `yaml:"-"`
	// HadPlaintextSecret records that the FILE still held a password in the
	// clear, and it has to be recorded at read time because it stops being
	// answerable afterwards: decryption happens on the way in, so every
	// password in memory is plaintext by definition.
	//
	// A method asking the loaded struct was tried and was wrong in the way
	// that matters — it answered "yes" always, so startup would have rewritten
	// the file on every single run. The test that pins this is what caught it.
	HadPlaintextSecret bool `yaml:"-"`
}

// hostsVersion and credsVersion count SEPARATELY, and that is the whole point.
// They were one constant until tags arrived, which changed hosts.yaml and left
// credentials.yaml byte-for-byte the same — a shared number would have stamped
// the unchanged file as new too, and an older sshu would refuse a file it can
// read perfectly well. A version that lies is worse than no version.
//
// hosts.yaml v2 adds Host.Tags. An older sshu READS a v2 file fine (yaml drops
// unknown keys) and then SILENTLY DROPS the tags on its next save, which is why
// SaveTo now refuses to write over a file newer than itself. That check ships
// here, so it protects every version after this one — it cannot protect against
// v1.5.1 and earlier, which have no such check and are already released.
// ENCRYPTED PASSWORDS DID NOT MOVE THESE NUMBERS, and the reason is worth
// keeping because the opposite was tried first.
//
// A version exists to stop an older sshu doing damage it cannot know it is
// doing. Tags qualify: v1.5.1 reads a v2 file, does not see the key, and drops
// the tags on its next save. Encryption does not. Measured, not reasoned —
// v1.5.1 was built from the tag and handed a sealed file:
//
//	it loads it, hands "ENC:…" to ssh as the password, and the connection
//	fails — which a version number cannot prevent, because refuseIfNewer only
//	guards WRITES and nothing stopped it reading;
//	and it writes the file back with the ciphertext intact, because a string
//	it does not understand still goes out the way it came in.
//
// So the version would have bought nothing at all: no protection it does not
// already have, against damage that does not happen. The "ENC:" prefix is what
// tells sealed from plain, at both ends, and a number saying the same thing
// again is a second source of truth to keep in step.
//
// What the version WAS doing here, quietly, was triggering the startup
// rewrite that seals existing plaintext. That job moved to the thing it is
// actually about — HasPlaintextSecret, which asks the data.
const (
	hostsVersion = 2
	credsVersion = 1
)

// versionUnset is what a file written before sshu stamped versions parses as.
// Distinct from 1, because "no version key" and "version: 1" call for the same
// upgrade but only one of them is a file somebody's sshu actually wrote.
const versionUnset = 0

// header is prepended to every write. Encryption did not retire the warning,
// it narrowed it: .sshukey lives in this same directory, so sealing the values
// protects the file on its own and does nothing for a copy of the directory.
const header = `# sshu hosts — managed by preference → hosts. Hand-editing is fine.
#
# Passwords here are stored encrypted (ENC:...), with the key in .sshukey beside
# this file. That protects THIS FILE on its own — not a copy of the directory,
# which carries the key with it. Plaintext you hand-edit in is sealed on the next
# start. Mode 0600 is re-asserted on every write and does not survive a copy
# either. Keep this directory out of version control, out of auto-syncing
# folders, and out of backups.
`

// Validate checks the list as a whole: every host valid, and no duplicate names.
func (f File) Validate() error {
	seen := make(map[string]bool, len(f.Hosts))
	for i, h := range f.Hosts {
		if err := h.Validate(); err != nil {
			return fmt.Errorf("hosts[%d]: %w", i, err)
		}
		if seen[h.Name] {
			return fmt.Errorf("hosts[%d]: duplicate name %q", i, h.Name)
		}
		seen[h.Name] = true
	}
	return nil
}

// Index returns the position of the host with this name, or -1.
func (f File) Index(name string) int {
	for i, h := range f.Hosts {
		if h.Name == name {
			return i
		}
	}
	return -1
}

// Load reads hosts.yaml. A missing file is not an error — it is the first-run
// empty state, and the UI has a panel for it. The second return is the names of
// any hosts dropped for repeating one — see LoadFrom.
func Load() (File, []string, error) {
	path, err := HostsPath()
	if err != nil {
		return File{Version: hostsVersion}, nil, err
	}
	return LoadFrom(path)
}

// LoadFrom is Load against an explicit path (tests).
//
// Duplicate names are DROPPED rather than refused. A hand-edited file is the
// only way to get one, and refusing to start would be the harshest possible
// answer to a paste that went in twice: the whole list stays unreachable until
// the user finds the repeat by hand, in a file they cannot read because sshu is
// what they read it with. The dropped names come back so the caller can say
// what happened — dropping them silently would be worse than either.
func LoadFrom(path string) (File, []string, error) {
	// A file that is not there is the first run, and a first run writes the
	// current format — so THAT one is stamped current. A file that IS there
	// starts at versionUnset and only gets a number if it carries one, because
	// defaulting a real file to "current" would hide exactly the case the
	// upgrade exists for.
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return File{Version: hostsVersion}, nil, nil
	}
	f := File{Version: versionUnset}
	if err != nil {
		return File{Version: hostsVersion}, nil, err
	}
	if err := yaml.Unmarshal(raw, &f); err != nil {
		return File{Version: hostsVersion}, nil, fmt.Errorf("%s: %w", path, err)
	}
	// A hand-edited file may omit port; fill the default rather than reject the
	// whole file over a field the user reasonably left out.
	for i := range f.Hosts {
		if f.Hosts[i].Port == 0 {
			f.Hosts[i].Port = DefaultPort
		}
		f.Hosts[i].Tags = NormalizeTags(f.Hosts[i].Tags)
	}
	var dropped []string
	f.Hosts, dropped = dedupeByName(f.Hosts, func(h Host) string { return h.Name })

	// Passwords come off the disk sealed (§11.50). Opening them HERE is what
	// keeps the rest of sshu — the UI, the askpass helper, Resolve — working
	// with the password it always had.
	names := make([]string, len(f.Hosts))
	secrets := make([]*string, len(f.Hosts))
	for i := range f.Hosts {
		names[i] = f.Hosts[i].Name
		secrets[i] = &f.Hosts[i].Password
		// Asked HERE, before decryptInto rewrites these in place.
		if p := f.Hosts[i].Password; p != "" && !IsEncrypted(p) {
			f.HadPlaintextSecret = true
		}
	}
	bad, decErr := decryptInto(names, secrets)
	if decErr != nil {
		return f, dropped, fmt.Errorf("%s: %w", path, decErr)
	}
	f.UnreadableSecrets = bad
	return f, dropped, nil
}

// Save writes hosts.yaml atomically at 0600.
//
// Atomic because a half-written file loses the entire host list, and this is
// the only copy. 0600 because of the plaintext passwords — reasserted on every
// write, so a file that was widened by hand narrows again on the next save.
func Save(f File) error {
	path, err := HostsPath()
	if err != nil {
		return err
	}
	return SaveTo(path, f)
}

// SaveTo is Save against an explicit path (tests).
//
// It refuses to write over a file stamped NEWER than this build understands.
// The check reads the file back rather than trusting a flag carried in f,
// because the UI rebuilds File from its own slice on every save and a flag
// would not survive the trip — and because the file can also be changed by
// another sshu while this one is running.
func SaveTo(path string, f File) error {
	if err := f.Validate(); err != nil {
		return err
	}
	if err := refuseIfNewer(path, hostsVersion, "hosts.yaml"); err != nil {
		return err
	}
	f.Version = hostsVersion

	// COPY before sealing. f is a value but its Hosts slice is not, and the
	// caller builds one straight from the list the UI is displaying — sealing
	// in place would swap the passwords on screen for ciphertext, and the next
	// edit would save that as though it were what the user typed.
	hosts := make([]Host, len(f.Hosts))
	copy(hosts, f.Hosts)
	f.Hosts = hosts
	secrets := make([]*string, len(f.Hosts))
	for i := range f.Hosts {
		secrets[i] = &f.Hosts[i].Password
	}
	if err := encryptInto(secrets); err != nil {
		return err
	}

	body, err := yaml.Marshal(f)
	if err != nil {
		return err
	}
	return writeFile0600(path, append([]byte(header), body...))
}

// NormalizeTags cleans a tag list: blanks dropped, repeats dropped, order kept.
//
// Case is NOT folded. The tags are the user's own words and "Prod" is what they
// typed; searching folds case at the query end, which is where folding belongs.
// Repeats go because the same tag twice on one host says nothing the once did
// not, and a hand-edited file is the only way to get one.
func NormalizeTags(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, t := range in {
		t = strings.TrimSpace(t)
		if t == "" || seen[t] {
			continue
		}
		seen[t] = true
		out = append(out, t)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// ParseTags splits what the user typed into one field. Space is the only
// separator — every other character is part of a tag, so "k8s:prod" and
// "web/db" need no escaping rule.
func ParseTags(s string) []string { return NormalizeTags(strings.Fields(s)) }

// JoinTags is ParseTags' inverse, for putting a saved host back in the form.
func JoinTags(tags []string) string { return strings.Join(tags, " ") }

// NeedsUpgrade reports whether this file was written by an older sshu and
// should be rewritten in the current format.
func (f File) NeedsUpgrade() bool { return f.Version < hostsVersion }

// FromNewerSshu reports the opposite: this file knows things this build does
// not. Saving it is refused (refuseIfNewer), and this is how the app can say so
// at startup instead of letting the user edit for ten minutes and hit the
// refusal on the way out.
func (f File) FromNewerSshu() bool { return f.Version > hostsVersion }

// NeedsUpgrade reports the same for credentials.yaml.
func (f CredsFile) NeedsUpgrade() bool { return f.Version < credsVersion }

// FromNewerSshu reports the same for credentials.yaml.
func (f CredsFile) FromNewerSshu() bool { return f.Version > credsVersion }

// refuseIfNewer stops a save that would downgrade a file written by a newer
// sshu. An unreadable or unparseable file is NOT refused: the version cannot be
// established, and refusing then would lock the user out of the one program
// that can fix the file.
func refuseIfNewer(path string, current int, what string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var probe struct {
		Version int `yaml:"version"`
	}
	if yaml.Unmarshal(raw, &probe) != nil {
		return nil
	}
	if probe.Version > current {
		return fmt.Errorf("%s was written by a newer sshu (version %d, this build understands %d) — "+
			"refusing to overwrite it; upgrade sshu or move the file aside",
			what, probe.Version, current)
	}
	return nil
}

// writeFile0600 lands out at path atomically, at mode 0600 whatever the file
// was before. Shared by every store file that can hold a secret.
//
// Atomic because a half-written file loses the only copy; 0600 re-asserted so a
// file that was widened by hand narrows again on the next save.
func writeFile0600(path string, out []byte) error {
	return writeFileMode(path, out, 0o600)
}

// writeFileMode is writeFile0600 with the permission as an argument, for
// ~/.ssh/config — a file sshu did not create and whose mode is therefore not
// sshu's to decide (see SaveSSHConfigTo). The atomicity is the part both
// callers need.
func writeFileMode(path string, out []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op once the rename succeeds

	// Chmod before the rename: CreateTemp makes 0600 already, but be explicit —
	// this is the permission the finished file inherits.
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(out); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}
