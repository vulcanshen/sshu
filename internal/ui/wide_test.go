package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/vulcanshen/sshu/internal/store"
)

// A remote that prints emoji renders wider than the emulator grid says, which is
// what pushed the panel border off the right edge.
func TestWideRemoteOutputCannotBreakTheFrame(t *testing.T) {
	fakeSSH(t, `printf "vulcan in \360\237\214\220 Mac in ~   \360\237\225\220 11:24:10\n"; exec cat`)
	for _, sz := range [][2]int{{92, 24}, {70, 20}, {54, 16}} {
		w, h := sz[0], sz[1]
		m := New(sample(), nil, store.DefaultConfig())
		next, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: h})
		m = pressA(settle(next.(AppModel)), "enter", "enter")
		s := m.ssh.currentSession()
		waitFor(t, "the emoji line", func() bool {
			return strings.Contains(strings.Join(s.pty.render(w-2, h-2), ""), "Mac")
		})
		for _, focus := range []sshPanel{panelPty, panelSessions} {
			m.ssh.setFocus(focus)
			for i, l := range strings.Split(m.View(), "\n") {
				if lw := dispW(l); lw != w {
					t.Fatalf("%dx%d focus=%d line %d: width %d, want %d\n%q", w, h, focus, i, lw, w, l)
				}
			}
		}
		m.ssh.stopAll()
	}
}
