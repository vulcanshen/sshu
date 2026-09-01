package ui

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"time"

	"github.com/vulcanshen/sshu/internal/store"
)

// asExitError is errors.As narrowed to *exec.ExitError, kept here so pty_unix.go
// can stay focused on terminal mechanics.
func asExitError(err error, target **exec.ExitError) bool { return errors.As(err, target) }

type sessionState int

const (
	sessLive sessionState = iota
	sessEnded
)

// session is one ssh connection: a host, a PTY running ssh, and how it ended.
//
// An ended session releases its ptyTerm immediately — a whole terminal grid kept
// for something nothing draws — but not before its last line is read out, which
// is where ssh puts the reason it gave up. That line goes to the app log.
//
// Nothing is written to disk: the final screen can hold anything the remote
// printed, and a connection log on disk is not worth the leak.
type session struct {
	id      int
	host    store.Host
	started time.Time
	ended   time.Time
	state   sessionState
	reason  string // "exited 0" / "disconnected" / a start-up failure
	ok      bool   // ended cleanly — [6] colours the row on this
	// ordinal distinguishes two live sessions to the same host. 0 means this is
	// the only one and nothing is shown.
	ordinal int
	// detail is the whole final screen, kept for the app log while reason keeps
	// the one line a toast has room for.
	detail []string
	// timedOut marks a session sshu stopped itself, because ssh got past the
	// part its own ConnectTimeout covers and then said nothing at all.
	timedOut bool
	// appliedCols/Rows is the grid-cell geometry last pushed to the PTY, so a
	// reflow only SIGWINCHes the sessions whose numbers actually changed.
	appliedCols, appliedRows int
	pty                      *ptyTerm
}

// ordinalTag is the #N shown only when a host has more than one live session.
func (s *session) ordinalTag() string {
	if s.ordinal == 0 {
		return ""
	}
	return "#" + strconv.Itoa(s.ordinal)
}

// ---------------------------------------------------------------- ssh command

// sshBinary is the command sshu launches. A variable so tests can point it at a
// harmless stand-in instead of opening real connections.
var sshBinary = "ssh"

// buildSSHCmd assembles the ssh invocation for a host.
//
// sshu shells out to the real ssh rather than speaking the protocol itself, so
// ssh_config, ProxyJump, agent forwarding, host-key handling and everything else
// the user already relies on keeps working. TERM is pinned to xterm-256color
// because that is what the embedded emulator implements — claiming more would
// make remote programs emit sequences vt10x drops on the floor.
func buildSSHCmd(h store.Host, self string, timeoutSecs int) *exec.Cmd {
	// ssh does the timing out, not sshu, because ssh knows how to SAY it: it
	// prints "Connection timed out" and exits, which flows through the same
	// path every other failure does. Killing the process ourselves would have
	// produced a corpse with no explanation attached to it.
	args := []string{"-p", strconv.Itoa(h.Port),
		"-o", "ConnectTimeout=" + strconv.Itoa(timeoutSecs)}
	if h.Auth == store.AuthPrivateKey && h.IdentityFile != "" {
		args = append(args, "-i", store.ExpandTilde(h.IdentityFile))
		// With an explicit key, stop ssh from silently trying every agent
		// identity first — otherwise a wrong key here fails as "too many
		// authentication failures" and the real cause is invisible.
		args = append(args, "-o", "IdentitiesOnly=yes")
	}
	args = append(args, fmt.Sprintf("%s@%s", h.User, h.Host))

	cmd := exec.Command(sshBinary, args...)
	cmd.Env = sshEnv(h, self)
	return cmd
}

// sshEnv builds the child environment.
//
// For a password host it wires SSH_ASKPASS back at sshu itself: ssh runs
// `sshu` with SSHU_ASKPASS_HOST set, and that mode prints the stored password.
// SSH_ASKPASS_REQUIRE=force is what makes ssh use the helper even though a TTY
// is present (OpenSSH 8.4+); without it ssh would prompt on the PTY instead —
// which still works, the user just types it.
//
// The password itself is never put in the environment. The helper re-reads
// hosts.yaml, so the secret stays in one 0600 file rather than being copied into
// a child process's environment where it is readable for the process's lifetime.
func sshEnv(h store.Host, self string) []string {
	env := append(os.Environ(),
		"TERM=xterm-256color",
		"LC_ALL="+envOr("LC_ALL", "en_US.UTF-8"),
	)
	if h.Auth == store.AuthPassword && h.Password != "" && self != "" {
		env = append(env,
			"SSH_ASKPASS="+self,
			"SSH_ASKPASS_REQUIRE=force",
			askpassHostEnv+"="+h.Name,
		)
	}
	return env
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// askpassHostEnv names the host whose password sshu should print when it is
// re-executed as ssh's askpass helper.
const askpassHostEnv = "SSHU_ASKPASS_HOST"

// AskpassHost reports the host name when this process was started as ssh's
// askpass helper, or "" for a normal run.
func AskpassHost() string { return os.Getenv(askpassHostEnv) }

// selfPath is the sshu binary, used as its own askpass helper. An error here is
// not fatal: without it a password host simply prompts inside the PTY.
func selfPath() string {
	p, err := os.Executable()
	if err != nil {
		return ""
	}
	return p
}
