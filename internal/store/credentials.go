package store

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// credentials.yaml holds reusable identities: a user plus how that user
// authenticates. A host whose auth is "credential" names one of these and takes
// the whole package — user AND auth together, not a set of overridable
// defaults — so "who does this connection run as" has exactly one answer, and
// it is written in exactly one place.
//
// SECURITY: the same trade as hosts.yaml, with the same mitigations. The file
// may hold plaintext passwords, is re-asserted to 0600 on every write, and
// carries a warning header saying so.

// Credential is one entry in credentials.yaml. Name is the key a host refers
// to it by, so it must be unique.
type Credential struct {
	Name         string     `yaml:"name"`
	User         string     `yaml:"user"`
	Auth         AuthMethod `yaml:"auth"`
	IdentityFile string     `yaml:"identity_file,omitempty"`
	Password     string     `yaml:"password,omitempty"`
}

// Validate reports what is wrong with a credential, or nil. Auth here is the
// concrete pair only — a credential naming another credential would be
// indirection with no floor under it.
func (c Credential) Validate() error {
	switch {
	case strings.TrimSpace(c.Name) == "":
		return fmt.Errorf("credential name is required")
	case strings.TrimSpace(c.User) == "":
		return fmt.Errorf("credential %q: user is required", c.Name)
	case c.Auth != AuthPassword && c.Auth != AuthPrivateKey:
		return fmt.Errorf("credential %q: auth must be %q or %q, got %q",
			c.Name, AuthPassword, AuthPrivateKey, c.Auth)
	}
	return nil
}

// CredsFile is the whole credentials.yaml document.
type CredsFile struct {
	Version     int          `yaml:"version"`
	Credentials []Credential `yaml:"credentials"`
}

// credsHeader is prepended to every write, for the same reason hosts.yaml has
// one: 0600 does not survive being copied somewhere it should not go.
const credsHeader = `# sshu credentials — managed by preference → credentials. Hand-editing is fine.
#
# WARNING: this file may contain plaintext passwords (credentials with
# auth: password). It is kept at mode 0600, but that does not protect a copy:
# keep it out of version control, out of auto-syncing folders, and out of
# backups.
`

// Validate checks the list as a whole: every credential valid, no duplicates.
func (f CredsFile) Validate() error {
	seen := make(map[string]bool, len(f.Credentials))
	for i, c := range f.Credentials {
		if err := c.Validate(); err != nil {
			return fmt.Errorf("credentials[%d]: %w", i, err)
		}
		if seen[c.Name] {
			return fmt.Errorf("credentials[%d]: duplicate name %q", i, c.Name)
		}
		seen[c.Name] = true
	}
	return nil
}

// Index returns the position of the credential with this name, or -1.
func (f CredsFile) Index(name string) int {
	for i, c := range f.Credentials {
		if c.Name == name {
			return i
		}
	}
	return -1
}

// CredsPath is the full path to credentials.yaml.
func CredsPath() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return dir + string(os.PathSeparator) + "credentials.yaml", nil
}

// LoadCreds reads credentials.yaml. Missing is the first-run empty state.
func LoadCreds() (CredsFile, error) {
	path, err := CredsPath()
	if err != nil {
		return CredsFile{Version: currentVersion}, err
	}
	return LoadCredsFrom(path)
}

// LoadCredsFrom is LoadCreds against an explicit path (tests).
func LoadCredsFrom(path string) (CredsFile, error) {
	f := CredsFile{Version: currentVersion}
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return f, nil
	}
	if err != nil {
		return f, err
	}
	if err := yaml.Unmarshal(raw, &f); err != nil {
		return CredsFile{Version: currentVersion}, fmt.Errorf("%s: %w", path, err)
	}
	return f, nil
}

// SaveCreds writes credentials.yaml atomically at 0600.
func SaveCreds(f CredsFile) error {
	path, err := CredsPath()
	if err != nil {
		return err
	}
	return SaveCredsTo(path, f)
}

// SaveCredsTo is SaveCreds against an explicit path (tests).
func SaveCredsTo(path string, f CredsFile) error {
	if err := f.Validate(); err != nil {
		return err
	}
	f.Version = currentVersion
	body, err := yaml.Marshal(f)
	if err != nil {
		return err
	}
	return writeFile0600(path, append([]byte(credsHeader), body...))
}
