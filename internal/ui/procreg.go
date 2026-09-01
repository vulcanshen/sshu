//go:build darwin || linux

package ui

import (
	"os/exec"
	"sync"
)

// Every subprocess sshu starts runs on its own PTY, and a PTY start puts the
// child in its own session — which means a signal that kills sshu does NOT
// reach it. Without this registry, closing the terminal window (SIGHUP) or an
// outside SIGINT/SIGTERM would end the app and leave every ssh it opened
// running, holding connections nobody can see.
//
// The registry is the one place that knows every child, whatever it is — an
// ssh session, an sftp editor — because startPty is the one place children
// are born.

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

// liveChildren is how many subprocesses have been started and not yet reaped.
func liveChildren() int {
	procMu.Lock()
	defer procMu.Unlock()
	return len(procs)
}

// KillChildren force-terminates every live subprocess. Idempotent, safe from
// any goroutine, and callable after the program loop is gone — it is the last
// line of every exit path and the whole of the signal path.
func KillChildren() {
	procMu.Lock()
	defer procMu.Unlock()
	for cmd := range procs {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
	}
}
