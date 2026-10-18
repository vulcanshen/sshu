package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	terminu "github.com/vulcanshen/terminu-go"

	"github.com/vulcanshen/sshu/internal/store"
)

// Placeholder until the SSH screen moves over.
type sshScreen struct {
	s      *sshu
	screen *terminu.Screen
}

func newSSHScreen(s *sshu) *sshScreen {
	return &sshScreen{s: s, screen: &terminu.Screen{Name: "SSH", Key: "S", Desc: "the grid of live sessions",
		Panels: []*terminu.Panel{{Label: "Sessions", Body: &terminu.Pager{}}}}}
}

func (h *sshScreen) start() tea.Cmd                    { return nil }
func (h *sshScreen) liveCount() int                    { return 0 }
func (h *sshScreen) connect(store.Host) tea.Cmd        { return nil }
func (h *sshScreen) onMsg(msg tea.Msg) (tea.Cmd, bool) { return nil, false }
