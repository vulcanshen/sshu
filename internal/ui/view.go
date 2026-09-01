package ui

import (
	"strings"

	overlay "github.com/rmhubbert/bubbletea-overlay"
)

// View composes the whole screen: one capsule row, the active tab's panel, one
// footer — then composites whatever floats are up on top of that canvas.
//
// Both chrome rows are locked at one line each, so the panel absorbs every
// change in terminal height and nothing shifts vertically (§1.3).
func (m AppModel) View() string {
	if m.w == 0 || m.h == 0 {
		return "" // wait for the first WindowSizeMsg
	}
	// 20 is where the short-label capsule strip ([1] [2] [3]) still fits.
	if m.w < minAppW || m.h < chromeRows+3 {
		return "terminal too small"
	}

	out := strings.Join([]string{
		tabRow(m.w, tabLabels, int(m.tab), m.status()),
		tabRule(m.w),
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
	if m.log.isActive() {
		out = overlay.Composite(m.log.view(), out, overlay.Center, overlay.Center, 0, 0)
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
	case tabHosts:
		return m.hosts.status()
	case tabSFTP:
		return m.sftp.status(m.transfers.summary())
	case tabSSH:
		return m.ssh.status()
	}
	return "planned"
}

func (m AppModel) panel() string {
	switch m.tab {
	case tabSFTP:
		return m.sftp.view()
	case tabSSH:
		return m.ssh.view()
	}
	return m.hosts.view()
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
		return keyLegend([][2]string{{"alt+esc", "leave pty"}}, m.w)
	}
	// The digits offered are the ones the current tab actually shows (§4.4): a
	// number the screen does not display is a number the keyboard ignores.
	nav := [2]string{"1-3", "tabs"}
	switch m.tab {
	case tabSFTP:
		nav = [2]string{"1-7", "surfaces"}
	case tabSSH:
		nav = [2]string{"1-5", "surfaces"}
	}
	// `!` is disclosed like every other entry key, and when the log holds errors
	// nobody has read it says HOW MANY instead of just "log". A record nobody is
	// told about is a record nobody opens.
	log := [2]string{"!", "log"}
	if n := m.log.unreadErrors(); n > 0 {
		log[1] = plural(n, "error")
	}
	return keyLegend([][2]string{
		{"space", "menu"}, {"?", "help"}, nav, log, {"q", "quit"},
	}, m.w)
}
