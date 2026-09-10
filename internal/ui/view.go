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

	// A full-screen cell replaces the whole frame — the same shape as the
	// splash above, and for a harder reason. Locking the chrome rows at one
	// line each (§1.3) is what stops the panel shifting under a resize; it was
	// never a promise that they are always drawn. A nested sshu pays for them
	// at EVERY layer, which is what put a ceiling on how deep the nesting
	// could go, and this state is how that ceiling comes off (§11.47).
	var out string
	if m.tab == tabSSH && m.ssh.maxed() {
		out = m.panel()
	} else {
		// The rule carries the transfer bar on every tab; the green status only
		// where the summary itself lives (the file-transfer tab).
		pct, moving := m.transfers.progress()
		out = strings.Join([]string{
			tabRow(m.w, tabLabels, int(m.tab), m.status(), moving && m.tab == tabFT),
			tabRule(m.w, pct, moving),
			m.panel(),
			m.footer(),
		}, "\n")
	}

	// Bottom to top. The Space menu goes down first so anything it launches
	// lands above it and Esc unwinds in the order the user built the stack.
	if m.spaceMenu.isActive() {
		out = overlay.Composite(m.spaceMenu.view(), out, overlay.Center, overlay.Center, 0, 0)
	}
	if m.lockMenu.isActive() {
		out = overlay.Composite(m.lockMenu.view(), out, overlay.Center, overlay.Center, 0, 0)
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
	if m.detail.isActive() {
		out = overlay.Composite(m.detail.view(), out, overlay.Center, overlay.Center, 0, 0)
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
	if m.sshcfgFormUI.isActive() {
		out = overlay.Composite(m.sshcfgFormUI.view(), out, overlay.Center, overlay.Center, 0, 0)
	}
	if m.knownAddUI.isActive() {
		out = overlay.Composite(m.knownAddUI.view(), out, overlay.Center, overlay.Center, 0, 0)
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
	// Every frame carries the announcement. It costs ~40 invisible bytes and
	// buys idempotence: a parent that missed one report gets the next one,
	// with no edge to lose and no channel to keep alive (§11.44).
	return out + m.nestAnnounce()
}

// nestAnnounce is what this sshu tells whatever is drawing it — which is a
// parent sshu, or a terminal that will consume the sequence and print
// nothing. Sent unconditionally BECAUSE it cannot ask: being seen is the
// whole point, and a report nobody reads costs the bytes and nothing else.
//
// One entry per sshu, this one first: where its focused cell leads, and
// whether that cell is passing every key through. A parent prepends its own
// entry and passes the result up, so the outermost assembles the whole
// chain without anybody knowing how deep they are.
func (m AppModel) nestAnnounce() string {
	return nestEncode(m.nestChain())
}

// nestChain is this sshu's own entry followed by everything below it.
func (m AppModel) nestChain() []nestLayer {
	s := m.ssh.currentSession()
	if s == nil || s.state != sessLive {
		return nil
	}
	chain := []nestLayer{{Host: nameOr(s.host.Name, s.host.Host), Locked: s.locked}}
	if inner, ok := s.pty.nestChain(); ok {
		chain = append(chain, inner...)
	}
	return chain
}

// copyLegendPairs is selection mode's own keys. It has two homes — the footer,
// and an overlay on the bottom row when the cell is full screen and there is no
// footer to put it in — and one list, because a mode disclosed two different
// ways in two places is a mode documented wrong in one of them (§11.33, §A.1).
func copyLegendPairs() [][2]string {
	return [][2]string{
		{"y", "copy"},
		{"v/V", "select"},
		{"hjkl", "move"},
		{"u/d", "half page"},
		{"alt+v", "leave"},
	}
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
		// Selection mode replaces the row outright: every key under the user's
		// fingers means something different in there, and a row still offering
		// the pty's keys would be describing a panel that is not on screen.
		if m.ssh.copy.on {
			return keyLegend(copyLegendPairs(), m.w)
		}
		// The tab keys are NOT offered: they are bare letters now, and in here a
		// bare letter is the remote's (§11.21). What is left is sshu's own Alt
		// keys, which no remote can claim.
		// Alt+Esc peels one layer at a time, so the row says which layer it
		// would take off if it were pressed right now.
		out := "leave pty"
		if m.ssh.zoomAt() != zoomOff {
			out = "leave zoom"
		}
		// A LOCKED cell keeps only one key, so the row says only that: every
		// other entry would be a lie — the same honesty rule this row already
		// follows about the remote's keys (§11.43).
		if s := m.ssh.currentSession(); s != nil && s.locked {
			return keyLegend([][2]string{{"alt+enter", "release"}}, m.w)
		}
		pairs := [][2]string{{"alt+esc", out}}
		// And the scrollback keys, but only where they would do something: they
		// are the remote's while a full-screen program is up, and there is
		// nothing to page through until more has been said than fits (§11.19).
		if m.ssh.canScroll() {
			pairs = append(pairs, [2]string{"pgup/pgdn", "history"})
		}
		// The way to get text OUT of the cell. Unconditional: inPty already
		// means a live session that has spoken, which is everything the mode
		// needs. It sits ahead of the cell chord because keyLegend drops from
		// the END on a cramped footer, and there is no other way to reach this
		// at all — a bare `?` in here belongs to the remote, so the footer is
		// the only live disclosure the pty has (§11.33, §11.19).
		pairs = append(pairs, [2]string{"alt+v", "select"})
		// Both of the next two sit AFTER alt+v, because §11.33 ruled that
		// select must survive a cramped footer — and it nearly stopped doing
		// so when the zoom label grew. "full screen" is longer than "zoom",
		// and at 40 columns, where the row fits exactly two pairs, that
		// difference was enough to drop select off the end (§11.47).
		//
		// The zoom is named by what the NEXT press does: the cycle has three
		// stops, and one label for all of them would be describing the key
		// rather than the state. It is offered unconditionally now — there is
		// always chrome left to take off, so there is no longer a grid on
		// which the chord does nothing.
		pairs = append(pairs, [2]string{"alt+z", m.ssh.nextZoomLabel()})
		// Last of the three: the lock chord is a nested-session tool, the
		// rarest need of the group.
		pairs = append(pairs, [2]string{"alt+enter", "lock"})
		pairs = append(pairs, [2]string{"alt+" + arrowGlyphs + arrowUpDown, "cell"})
		return keyLegend(pairs, m.w)
	}
	// An Operation page eats every printable key — the digits, space, ?, q and
	// the tab letters — so the row says only what is still true, the same
	// honesty the pty row keeps. Esc first, then the tab keys answer again.
	if m.textPage() {
		return keyLegend([][2]string{{"tab", "field"}, {"enter", "run"},
			{"esc", "back"}}, m.w)
	}
	// The digits offered are the ones the current tab actually shows (§4.4): a
	// number the screen does not display is a number the keyboard ignores.
	nav := [2]string{"1-2 M/F/S", "panel tab"}
	if m.tab == tabFT {
		nav = [2]string{"1-4 M/F/S", "panel tab"}
	}
	// Unread errors are disclosed against the key that reaches them: the log
	// lives at manage → logs, and a record nobody is told about is a record
	// nobody opens.
	pairs := [][2]string{{"space", "menu"}, {"?", "help"}, nav}
	if n := m.errors.unreadErrors(); n > 0 {
		pairs = append(pairs, [2]string{"M", plural(n, "unread error")})
	}
	pairs = append(pairs, [2]string{"q", "quit"})
	return keyLegend(pairs, m.w)
}
