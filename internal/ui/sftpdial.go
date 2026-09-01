package ui

import (
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/vulcanshen/sshu/internal/store"
)

// A dial can take its full 15-second timeout, and for all of that time the panel
// used to show "Press [S] to select a host" — the no-host prompt, because the
// side genuinely has no filesystem yet. So the app looked like it had ignored
// the keypress, or hung.
//
// It now says what it is doing, and it MOVES while it does. A static line is not
// enough here: the complaint is "is this thing stuck", and a frame that never
// changes is exactly what stuck looks like. The spinner is the answer to that
// question and the elapsed count is the answer to the next one.

// dialTickEvery drives the spinner. Fast enough to read as motion, slow enough
// that a slow terminal is not being asked to repaint for nothing.
const dialTickEvery = 120 * time.Millisecond

// spinnerFrames are Braille dots — every one of them is a single cell in every
// terminal (east-asian width "neutral"), which a rotating ASCII bar or a glyph
// from the Nerd Font PUA could not promise.
var spinnerFrames = []string{
	string(rune(0x280B)), string(rune(0x2819)), string(rune(0x2839)), string(rune(0x2838)),
	string(rune(0x283C)), string(rune(0x2834)), string(rune(0x2826)), string(rune(0x2827)),
	string(rune(0x2807)), string(rune(0x280F)),
}

type dialTickMsg struct{}

// startDial marks the side as connecting and returns the work plus the tick.
//
// gen retires an earlier dial's answer: pick a host, change your mind, pick
// another, and the first one can still land afterwards and put you on the host
// you rejected.
func (m *sftpModel) startDial(sd side, h store.Host) tea.Cmd {
	s := &m.sides[sd]
	s.dialGen++
	s.dialing, s.dialSince, s.host, s.err = h.Name, time.Now(), h.Name, ""
	budget := m.timeout
	if budget <= 0 {
		budget = store.DefaultConnectTimeout * time.Second
	}
	return tea.Batch(dialCmd(sd, h, s.dialGen, budget), m.dialTick())
}

// dialTick keeps the spinner turning while any side is connecting, and stops
// itself when none is — an idle sshu repaints for nothing.
func (m *sftpModel) dialTick() tea.Cmd {
	if m.sides[0].dialing == "" && m.sides[1].dialing == "" {
		return nil
	}
	return tea.Tick(dialTickEvery, func(time.Time) tea.Msg { return dialTickMsg{} })
}

// onDialTick advances the spinner and re-arms.
func (m *sftpModel) onDialTick() tea.Cmd {
	m.spinAt++
	return m.dialTick()
}

// dialingBody is what the panel shows while it waits.
func (m sftpModel) dialingBody(s sftpSideModel, innerW, innerH int) []string {
	dim := lipgloss.NewStyle().Foreground(dimColor)
	hand := lipgloss.NewStyle().Foreground(handColor)

	spin := spinnerFrames[m.spinAt%len(spinnerFrames)]
	waited := int(time.Since(s.dialSince).Seconds())
	elapsed := ""
	if waited >= 2 {
		// Only once it is worth mentioning. A counter that starts at 0 on every
		// connection makes a fast one look slow.
		elapsed = fmt.Sprintf("  %ds", waited)
	}

	plain := spin + " connecting to " + s.dialing + elapsed
	line := centerLine(innerW, plain,
		hand.Render(spin)+dim.Render(" connecting to ")+hand.Render(s.dialing)+
			dim.Render(elapsed))

	blank := spaces(innerW)
	out := make([]string, 0, max(0, innerH))
	for i := 0; i < max(0, (innerH-1)/2); i++ {
		out = append(out, blank)
	}
	return append(out, line)
}
