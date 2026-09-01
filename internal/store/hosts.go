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
}

const currentVersion = 1

// header is prepended to every write. The warning is not decoration: the file
// can hold plaintext passwords, and 0600 does not survive being copied into a
// synced folder or a git repo.
const header = `# sshu hosts — managed by the [1] hosts tab. Hand-editing is fine.
#
# WARNING: this file may contain plaintext passwords (hosts with auth: password).
# It is kept at mode 0600, but that does not protect a copy: keep it out of
# version control, out of auto-syncing folders, and out of backups.
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
// empty state, and the UI has a panel for it.
func Load() (File, error) {
	path, err := HostsPath()
	if err != nil {
		return File{Version: currentVersion}, err
	}
	return LoadFrom(path)
}

// LoadFrom is Load against an explicit path (tests).
func LoadFrom(path string) (File, error) {
	f := File{Version: currentVersion}
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return f, nil
	}
	if err != nil {
		return f, err
	}
	if err := yaml.Unmarshal(raw, &f); err != nil {
		return File{Version: currentVersion}, fmt.Errorf("%s: %w", path, err)
	}
	// A hand-edited file may omit port; fill the default rather than reject the
	// whole file over a field the user reasonably left out.
	for i := range f.Hosts {
		if f.Hosts[i].Port == 0 {
			f.Hosts[i].Port = DefaultPort
		}
	}
	return f, nil
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
func SaveTo(path string, f File) error {
	if err := f.Validate(); err != nil {
		return err
	}
	f.Version = currentVersion

	body, err := yaml.Marshal(f)
	if err != nil {
		return err
	}
	return writeFile0600(path, append([]byte(header), body...))
}

// writeFile0600 lands out at path atomically, at mode 0600 whatever the file
// was before. Shared by every store file that can hold a secret.
//
// Atomic because a half-written file loses the only copy; 0600 re-asserted so a
// file that was widened by hand narrows again on the next save.
func writeFile0600(path string, out []byte) error {
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
	if err := tmp.Chmod(0o600); err != nil {
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
