package ui

import (
	"strings"

	overlay "github.com/rmhubbert/bubbletea-overlay"
)

// View composes the whole screen: one strip row, the active tab's panel, one
// footer — then composites whatever floats are up on top of that canvas.
//
// Both chrome rows are locked at one line each, so the panel absorbs every
// change in terminal height and nothing shifts vertically (§1.3).
func (m AppModel) View() string {
	if m.w == 0 || m.h == 0 {
		return "" // wait for the first WindowSizeMsg
	}
	// minAppW is where the strip (at its letter tier) still fits and the
	// panels are still worth drawing.
	if m.w < minAppW || m.h < chromeRows+3 {
		return "terminal too small"
	}
	// The easter-egg splash replaces the whole frame while it plays.
	if m.splash.isActive() {
		return m.splash.render(m.w, m.h)
	}

	// The rule carries the transfer bar on every tab; the green status only
	// where the summary itself lives (the file-transfer tab).
	pct, moving := m.transfers.progress()
	out := strings.Join([]string{
		tabRow(m.w, tabLabels, int(m.tab), m.status(), moving && m.tab == tabFT),
		tabRule(m.w, pct, moving),
		m.panel(),
		m.footer(),
	}, "\n")

	// Bottom to top. The Space menu goes down first so anything it launches
	// lands above it and Esc unwinds in the order the user built the stack.
	if m.spaceMenu.isActive() {
		out = overlay.Composite(m.spaceMenu.view(), out, overlay.Center, overlay.Center, 0, 0)
	}
	if m.hostPicker.isActive() {
		out = overlay.Composite(m.hostPicker.view(), out, overlay.Center, overlay.Center, 0, 0)
	}
	if m.transfersUI.isActive() {
		out = overlay.Composite(m.transfersUI.view(m.transfers.jobs), out,
			overlay.Center, overlay.Center, 0, 0)
	}
	if m.viewer.isActive() {
		out = overlay.Composite(m.viewer.view(), out, overlay.Center, overlay.Center, 0, 0)
	}
	if m.editorUI.isActive() {
		out = overlay.Composite(m.editorUI.view(), out, overlay.Center, overlay.Center, 0, 0)
	}
	if m.help.isActive() {
		out = overlay.Composite(m.help.view(), out, overlay.Center, overlay.Center, 0, 0)
	}
	if m.confirm.isActive() {
		out = overlay.Composite(m.confirm.view(), out, overlay.Center, overlay.Center, 0, 0)
	}
	if m.input.isActive() {
		out = overlay.Composite(m.input.view(), out, overlay.Center, overlay.Center, 0, 0)
	}
	if m.form.isActive() {
		out = overlay.Composite(m.form.view(), out, overlay.Center, overlay.Center, 0, 0)
	}
	if m.credFormUI.isActive() {
		out = overlay.Composite(m.credFormUI.view(), out, overlay.Center, overlay.Center, 0, 0)
	}
	// Above both forms: this menu is OPENED FROM the host form, and a popup
	// painted under the surface that launched it is a popup that never opened
	// — which is exactly how it shipped the first time. isActive tests cannot
	// catch a z-order bug; only a rendered frame can.
	if m.credPicker.isActive() {
		out = overlay.Composite(m.credPicker.view(), out, overlay.Center, overlay.Center, 0, 0)
	}
	if m.picker.isActive() {
		out = overlay.Composite(m.picker.view(), out, overlay.Center, overlay.Center, 0, 0)
	}
	// The toast is feedback about what just happened, so it sits above the stack
	// and out of its way — low, where it does not cover the surface being used.
	if m.toast.isActive() {
		out = overlay.Composite(m.toast.view(), out, overlay.Center, overlay.Bottom, 0, -2)
	}
	return out
}

// status is the right-hand slot of the capsule row: whatever the active tab
// wants to say about itself, in one line.
func (m AppModel) status() string {
	switch m.tab {
	case tabFT:
		return m.sftp.status(m.transfers.summary())
	case tabSSH:
		return m.ssh.status()
	}
	return m.prefStatus()
}

func (m AppModel) panel() string {
	switch m.tab {
	case tabFT:
		return m.sftp.view(m.transfers.arrivals())
	case tabSSH:
		return m.ssh.view()
	}
	return m.prefView()
}

// footer is the mandatory disclosure channel for both VTP entry keys (§A.1 /
// §A.2). A user who never opened a README learns from this row that Space and ?
// exist — without it the entry keys are unreachable and X collapses.
func (m AppModel) footer() string {
	// While the remote holds the keyboard every other entry in this row is a lie
	// — space, ?, the digits, q all travel to the far end. So the row says the one
	// thing that is still true, which is also the only way back out. This is the
	// mandatory disclosure for Alt+Esc: it is advertised exactly where it means
	// something, and nowhere else.
	if m.inPty() {
		// The chords still work in here — that is the point of them being
		// chords — so they are the only other thing the row may honestly say.
		return keyLegend([][2]string{{"alt+esc", "leave pty"}, {"alt+" + arrowGlyphs + arrowUpDown, "cell"}, {"alt+P/F/S", "tab"}}, m.w)
	}
	// An Operation page eats the digits, space, ? and q — while one is
	// focused the footer says only the keys that are still true, the same
	// honesty the pty row keeps.
	if m.textPage() {
		return keyLegend([][2]string{{"tab", "field"}, {"enter", "run"},
			{"esc", "back"}, {"alt+p/f/s", "tab"}}, m.w)
	}
	// The digits offered are the ones the current tab actually shows (§4.4): a
	// number the screen does not display is a number the keyboard ignores.
	nav := [2]string{"1-2 alt+p/f/s", "panel tab"}
	if m.tab == tabFT {
		nav = [2]string{"1-4 alt+p/f/s", "panel tab"}
	}
	// Unread errors are disclosed against the chord that reaches them: the log
	// lives at preference → logs now, and a record nobody is told about is a
	// record nobody opens.
	pairs := [][2]string{{"space", "menu"}, {"?", "help"}, nav}
	if n := m.log.unreadErrors(); n > 0 {
		pairs = append(pairs, [2]string{"alt+P", plural(n, "unread error")})
	}
	pairs = append(pairs, [2]string{"q", "quit"})
	return keyLegend(pairs, m.w)
}
