package remote

import (
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"syscall"

	"github.com/pkg/sftp"
)

// DialPipe speaks SFTP over a subprocess's stdin and stdout — the subprocess
// being `ssh -s <host> sftp`, which is how OpenSSH's own sftp client works.
//
// It exists for the host whose auth is "sshconfig": everything about reaching
// that host — HostName, ProxyJump, the key, the agent, a passphrase — is in a
// file ssh reads and this package does not, so the only honest way to connect
// is to let ssh do it. Dial keeps speaking the protocol itself for the hosts
// whose secrets sshu holds; that path reads none of ~/.ssh/config, which is
// the asymmetry the Config panel already discloses, and it is not widened
// here — a host that told sshu its password did not ask ssh to get involved.
//
// cmd is started here and owned by the returned FS: Close ends the
// subprocess. Whatever ssh printed to stderr is kept, and becomes the error
// text when the handshake fails — that is where ssh explains itself, in the
// words the Errors journal wants: "Permission denied", "Host key verification
// failed", "Connection timed out".
//
// The caller decides what the subprocess IS, and that includes putting it in
// its own session (Setsid) so it cannot reach for the terminal sshu is
// drawing on: with no tty and SSH_ASKPASS set, every question ssh has goes to
// the helper instead. This function only knows that a session leader is
// killed as a group, so a ProxyJump's child goes with it.
func DialPipe(label string, cmd *exec.Cmd) (FS, error) {
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", label, err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", label, err)
	}
	stderr := &tailBuffer{}
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("%s: %w", label, err)
	}

	client, err := sftp.NewClientPipe(stdout, stdin)
	if err != nil {
		endProcess(cmd)
		if said := stderr.text(); said != "" {
			return nil, fmt.Errorf("%s: %s", label, said)
		}
		return nil, fmt.Errorf("%s: %w", label, err)
	}
	return &pipeFS{sftpFS: &sftpFS{label: label, client: client}, cmd: cmd}, nil
}

// pipeFS is an sftpFS whose transport is a subprocess rather than a
// connection this package opened.
type pipeFS struct {
	*sftpFS
	cmd *exec.Cmd
}

// Close ends the SFTP session and then the subprocess under it. The order
// matters: closing the client sends the channel's EOF, so ssh usually exits
// on its own and the signal that follows finds nothing to do.
func (f *pipeFS) Close() error {
	_ = f.sftpFS.Close()
	endProcess(f.cmd)
	return nil
}

// endProcess stops the subprocess and reaps it. A session leader is signalled
// as a group — the process ssh started for a ProxyJump or ProxyCommand is in
// that group and would otherwise outlive the ssh that needed it.
func endProcess(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	if cmd.SysProcAttr != nil && cmd.SysProcAttr.Setsid {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
	} else {
		_ = cmd.Process.Kill()
	}
	_ = cmd.Wait()
}

// tailBuffer keeps the LAST few kilobytes written to it. ssh's stderr is
// short when something fails and empty when nothing does, but a host that
// prints a banner on every login could say anything at any length, and the
// end is where the reason lives.
type tailBuffer struct {
	mu  sync.Mutex
	buf []byte
}

const tailBufferCap = 8 << 10

func (b *tailBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.buf = append(b.buf, p...)
	if len(b.buf) > tailBufferCap {
		b.buf = b.buf[len(b.buf)-tailBufferCap:]
	}
	return len(p), nil
}

func (b *tailBuffer) text() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return strings.TrimSpace(string(b.buf))
}
