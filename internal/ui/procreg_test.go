//go:build darwin || linux

package ui

import (
	"os/exec"
	"testing"
	"time"
)

// Every child is registered at birth and reaped from the registry when it
// exits — so KillChildren, the last line of every exit path, can always name
// what is still alive.
func TestKillChildrenLeavesNoOrphans(t *testing.T) {
	before := liveChildren()

	p, err := startPty(exec.Command("sleep", "60"), 20, 5)
	if err != nil {
		t.Fatal(err)
	}
	if liveChildren() != before+1 {
		t.Fatalf("the child should be registered at birth, have %d", liveChildren())
	}

	KillChildren()
	waitFor(t, "the child to die", func() bool { return p.exited() })
	// Earlier tests' children may still be deregistering on their own
	// readLoops, so the count can pass below the baseline — at-most is the
	// stable claim.
	waitFor(t, "the registry to reap it", func() bool { return liveChildren() <= before })
}

// A child that exits on its own leaves the registry too — the registry counts
// the LIVING, or KillChildren would grow a list of ghosts to re-kill.
func TestNaturalExitDeregisters(t *testing.T) {
	before := liveChildren()
	p, err := startPty(exec.Command("true"), 20, 5)
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the child to finish", func() bool { return p.exited() })
	deadline := time.Now().Add(2 * time.Second)
	for liveChildren() > before && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if liveChildren() > before {
		t.Fatalf("a finished child should leave the registry, have %d want %d",
			liveChildren(), before)
	}
}
