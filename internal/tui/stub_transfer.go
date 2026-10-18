package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	terminu "github.com/vulcanshen/terminu-go"
)

// Placeholder until the File transfer screen moves over.
type transfer struct {
	s      *sshu
	screen *terminu.Screen
}

func newTransfer(s *sshu) *transfer {
	return &transfer{s: s, screen: &terminu.Screen{Name: "File transfer", Key: "F", Desc: "copy between any two machines",
		Panels: []*terminu.Panel{{Label: "Local", Body: &terminu.Pager{}}}}}
}

func (t *transfer) start() tea.Cmd                    { return nil }
func (t *transfer) running() int                      { return 0 }
func (t *transfer) statusText() string                { return "" }
func (t *transfer) progress() (float64, bool)         { return 0, false }
func (t *transfer) onMsg(msg tea.Msg) (tea.Cmd, bool) { return nil, false }
