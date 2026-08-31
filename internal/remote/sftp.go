package remote

import (
	"fmt"
	"io"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"

	"github.com/vulcanshen/sshu/internal/store"
)

// dialTimeout bounds a connection attempt so a dead host cannot hang the UI.
const dialTimeout = 15 * time.Second

// sftpFS is a host reached over SFTP.
//
// Unlike tab [3], which hands a PTY to the real ssh binary, this speaks the
// protocol itself — SFTP has no terminal to hand over. That moves two things
// onto us that ssh would otherwise have done: authentication, and host-key
// verification. Neither is skipped.
type sftpFS struct {
	label  string
	client *sftp.Client
	conn   *ssh.Client
}

// HostKeyPrompt is asked to approve a host that is not in known_hosts. Returning
// false aborts the connection.
//
// It is a parameter rather than a policy baked in here because the decision is
// the user's, and this package has no way to ask them.
type HostKeyPrompt func(host, fingerprint string) bool

// Dial opens an SFTP session to h.
//
// The host key is checked against ~/.ssh/known_hosts. An unknown host goes to
// prompt with its fingerprint; a host whose key has CHANGED is refused outright
// and never offered — that is the case known_hosts exists to catch, and turning
// it into a yes/no question trains people to answer yes.
func Dial(h store.Host, prompt HostKeyPrompt) (FS, error) {
	auth, err := authMethods(h)
	if err != nil {
		return nil, err
	}
	cb, err := hostKeyCallback(prompt)
	if err != nil {
		return nil, err
	}

	addr := net.JoinHostPort(h.Host, strconv.Itoa(h.Port))
	conn, err := ssh.Dial("tcp", addr, &ssh.ClientConfig{
		User:            h.User,
		Auth:            auth,
		HostKeyCallback: cb,
		Timeout:         dialTimeout,
	})
	if err != nil {
		return nil, fmt.Errorf("%s: %w", h.Name, err)
	}
	client, err := sftp.NewClient(conn)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("%s: %w", h.Name, err)
	}
	return &sftpFS{label: h.Name, client: client, conn: conn}, nil
}

func authMethods(h store.Host) ([]ssh.AuthMethod, error) {
	if h.Auth == store.AuthPassword {
		return []ssh.AuthMethod{ssh.Password(h.Password)}, nil
	}
	if h.IdentityFile == "" {
		return nil, fmt.Errorf("%s: no identity file set", h.Name)
	}
	raw, err := os.ReadFile(store.ExpandTilde(h.IdentityFile))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", h.Name, err)
	}
	signer, err := ssh.ParsePrivateKey(raw)
	if err != nil {
		// A passphrase-protected key needs an agent or a prompt, neither of
		// which exists here yet. Say so plainly rather than "invalid key".
		return nil, fmt.Errorf("%s: %w (encrypted keys are not supported yet)", h.Name, err)
	}
	return []ssh.AuthMethod{ssh.PublicKeys(signer)}, nil
}

// hostKeyCallback verifies against known_hosts, deferring unknown hosts to the
// prompt. A missing known_hosts file is treated as "nothing is known yet", so a
// fresh machine still gets asked rather than refused.
func hostKeyCallback(prompt HostKeyPrompt) (ssh.HostKeyCallback, error) {
	path := knownHostsPath()
	var known ssh.HostKeyCallback
	if _, err := os.Stat(path); err == nil {
		known, err = knownhosts.New(path)
		if err != nil {
			return nil, fmt.Errorf("known_hosts: %w", err)
		}
	}

	return func(hostname string, addr net.Addr, key ssh.PublicKey) error {
		if known != nil {
			err := known(hostname, addr, key)
			if err == nil {
				return nil
			}
			var ke *knownhosts.KeyError
			isKeyErr := asKeyError(err, &ke)
			// A KeyError carrying entries means the host IS known and the key
			// does not match. That is the case known_hosts exists to catch, so
			// it is refused outright — turning it into a yes/no question is how
			// people learn to answer yes.
			if isKeyErr && len(ke.Want) > 0 {
				return fmt.Errorf("host key for %s has changed — refusing to connect", hostname)
			}
			if !isKeyErr {
				return err
			}
		}
		if prompt == nil || !prompt(hostname, ssh.FingerprintSHA256(key)) {
			return fmt.Errorf("host key for %s was not accepted", hostname)
		}
		return nil
	}, nil
}

func knownHostsPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".ssh", "known_hosts")
}

func (f *sftpFS) Label() string { return f.label }

func (f *sftpFS) Home() (string, error) {
	// Getwd on a fresh SFTP session is the login directory, which is where a
	// person expects to land.
	return f.client.Getwd()
}

func (f *sftpFS) List(dir string) ([]Entry, error) {
	infos, err := f.client.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	out := make([]Entry, 0, len(infos))
	for _, fi := range infos {
		out = append(out, Entry{
			Name: fi.Name(), IsDir: fi.IsDir(),
			Size: fi.Size(), Mode: fi.Mode(), ModTime: fi.ModTime(),
		})
	}
	sortEntries(out)
	return out, nil
}

func (f *sftpFS) Stat(p string) (Entry, error) {
	fi, err := f.client.Stat(p)
	if err != nil {
		return Entry{}, err
	}
	return Entry{
		Name: fi.Name(), IsDir: fi.IsDir(),
		Size: fi.Size(), Mode: fi.Mode(), ModTime: fi.ModTime(),
	}, nil
}

func (f *sftpFS) Lstat(p string) (Entry, error) {
	fi, err := f.client.Lstat(p)
	if err != nil {
		return Entry{}, err
	}
	return Entry{
		Name: fi.Name(), IsDir: fi.IsDir(),
		Size: fi.Size(), Mode: fi.Mode(), ModTime: fi.ModTime(),
	}, nil
}

func (f *sftpFS) Rename(from, to string) error { return f.client.Rename(from, to) }

func (f *sftpFS) Close() error {
	if f.client != nil {
		f.client.Close()
	}
	if f.conn != nil {
		return f.conn.Close()
	}
	return nil
}

func (f *sftpFS) Open(p string) (io.ReadCloser, error) { return f.client.Open(p) }

func (f *sftpFS) Create(p string, mode fs.FileMode) (io.WriteCloser, error) {
	w, err := f.client.Create(p)
	if err != nil {
		return nil, err
	}
	if mode != 0 {
		// Best effort: some servers refuse chmod, and a file that arrived beats a
		// transfer aborted over its permission bits.
		_ = f.client.Chmod(p, mode.Perm())
	}
	return w, nil
}

func (f *sftpFS) MkdirAll(p string) error { return f.client.MkdirAll(p) }
func (f *sftpFS) Remove(p string) error   { return f.client.Remove(p) }
