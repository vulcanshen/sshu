package tui

import (
	"os/exec"
	"sync"
)

// The subprocesses sshu starts outside a PTY of the lib's — the ssh under an
// sshconfig host's sftp dial — run in their own session, so a signal that ends
// sshu does not reach them. The registry knows each one, for KillChildren.

var (
	procMu sync.Mutex
	procs  = map[*exec.Cmd]struct{}{}
)

func registerProc(cmd *exec.Cmd) {
	procMu.Lock()
	defer procMu.Unlock()
	procs[cmd] = struct{}{}
}

func deregisterProc(cmd *exec.Cmd) {
	procMu.Lock()
	defer procMu.Unlock()
	delete(procs, cmd)
}

// stopper is something running that has to end when sshu does: a session's
// terminal.
type stopper interface{ Stop() }

var (
	stopMu   sync.Mutex
	stoppers = map[stopper]struct{}{}
)

func registerStop(s stopper) {
	stopMu.Lock()
	defer stopMu.Unlock()
	stoppers[s] = struct{}{}
}

func deregisterStop(s stopper) {
	stopMu.Lock()
	defer stopMu.Unlock()
	delete(stoppers, s)
}

// KillChildren ends every subprocess sshu started. It is the last line of
// every way out, and the whole of the SIGHUP one.
func KillChildren() {
	procMu.Lock()
	for cmd := range procs {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
	}
	procMu.Unlock()
	stopMu.Lock()
	for s := range stoppers {
		s.Stop()
	}
	stopMu.Unlock()
}
