//go:build darwin || linux

package ui

import (
	"os/exec"
	"strings"
	"testing"
)

// A terminal is not only a screen — it also ANSWERS. The child asks where the
// cursor is and then blocks on the reply, so an emulator that parses the
// question and drops its own answer leaves the child waiting out a timeout it
// chose for a terminal that does not exist. termenv's is five seconds, and
// every Bubble Tea program asks before main() runs — which is why sshu inside
// sshu took five seconds to draw anything.
//
// dd blocking on a single byte of stdin IS the assertion: that byte exists only
// if the emulator's answer travelled back up the master. What the answer SAYS
// is vt10x's business; that it arrives at all is sshu's.
//
// The stty is not decoration. A reply carries no newline, so a line-buffered
// terminal would hold it back and the test would fail against a working
// emulator — which is why anything that asks turns canonical mode off first,
// termenv included, and why the test has to ask the way an asker asks.
func TestTheEmulatorAnswersTheChildsQuery(t *testing.T) {
	p, err := startPty(exec.Command("sh", "-c",
		`stty -icanon -echo min 1 time 0; printf '\033[6n'; `+
			`dd bs=1 count=1 >/dev/null 2>&1 && printf 'ANSWERED\n'`), 40, 6)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(p.stop)

	waitFor(t, "the child to receive the answer to its query", func() bool {
		return strings.Contains(strings.Join(p.screenLines(), "\n"), "ANSWERED")
	})
}
