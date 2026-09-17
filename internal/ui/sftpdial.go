package ui

import (
	"context"
	"fmt"
	"os/exec"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/vulcanshen/sshu/internal/remote"
	"github.com/vulcanshen/sshu/internal/store"
)

// A dial can take its full 15-second timeout, and for all of that time the panel
// used to show "Press [H] to pick a host" — the no-host prompt, because the
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
	s.endDial() // a dial still out there is one the user has moved on from
	s.dialGen++
	s.dialing, s.dialSince, s.host, s.err = h.Name, time.Now(), h.Name, ""
	s.dialCancelled = false
	budget := m.timeout
	if budget <= 0 {
		budget = store.DefaultConnectTimeout * time.Second
	}
	if h.Auth != store.AuthSSHConfig {
		return tea.Batch(dialCmd(sd, h, s.dialGen, budget), m.dialTick())
	}
	// An sshconfig host goes through the real ssh, and the real ssh may have
	// questions — so a socket for them is opened first, and the dial carries
	// its path. The whole thing is one generation: the socket, the ssh and
	// the answer all belong to THIS attempt (§11.52).
	srv, err := newAskpassServer(sd, s.dialGen)
	if err != nil {
		s.dialing, s.host, s.err = "", "", err.Error()
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	cmd := buildSFTPCmd(ctx, h, selfPath(), srv.path(), int(budget/time.Second))
	s.askpass, s.dialCancel = srv, cancel
	return tea.Batch(dialPipeCmd(sd, h, s.dialGen, cmd, cancel), m.dialTick(), srv.next())
}

// endDial ABANDONS an ssh-backed dial: the questions socket is closed and
// the ssh is killed. For a dial the user moved on from, or a side that is
// being torn down. Idempotent, and a no-op for a dial that never had either.
func (s *sftpSideModel) endDial() {
	s.askpass.close()
	if s.dialCancel != nil {
		s.dialCancel()
	}
	s.askpass, s.dialCancel = nil, nil
}

// dialDone is the other ending: the dial has ANSWERED, and whatever it
// answered with — a connection, or a reason — the questions socket has no
// more to do. The ssh is not touched here: on success it IS the connection
// now, and belongs to the FS; on failure it has already exited. The first
// version of this called endDial and killed the ssh it had just connected —
// "connection lost" one frame after the panel title changed.
func (s *sftpSideModel) dialDone() {
	s.askpass.close()
	s.askpass, s.dialCancel = nil, nil
}

// abortDial is the user's cancel, from the question popup: the ssh is
// killed, and the result that follows is marked as theirs. The socket stays
// until that result lands, so a question already in flight is refused in
// order rather than lost.
func (s *sftpSideModel) abortDial(gen int) {
	if gen != s.dialGen || s.dialCancel == nil {
		return
	}
	s.dialCancelled = true
	s.dialCancel()
}

// dialPipeCmd runs the ssh-backed dial off the update loop. The child is in
// the process registry from before it starts, so no exit path — a closed
// terminal window included — can leave it running with a question open.
func dialPipeCmd(sd side, h store.Host, gen int, cmd *exec.Cmd, cancel context.CancelFunc) tea.Cmd {
	return func() tea.Msg {
		registerProc(cmd)
		fsys, err := remote.DialPipe(h.Name, cmd)
		if err != nil {
			deregisterProc(cmd)
			cancel()
			return sftpConnectedMsg{sd: sd, gen: gen, err: err}
		}
		return sftpConnectedMsg{sd: sd, gen: gen, fs: &trackedFS{FS: fsys, cmd: cmd, cancel: cancel}}
	}
}

// trackedFS owns what the dial handed over: the ssh's registry entry and
// its context. Both are released when the side lets go of the connection.
type trackedFS struct {
	remote.FS
	cmd    *exec.Cmd
	cancel context.CancelFunc
}

func (f *trackedFS) Close() error {
	err := f.FS.Close()
	f.cancel()
	deregisterProc(f.cmd)
	return err
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
