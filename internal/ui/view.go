package ui

import (
	"strings"

	overlay "github.com/rmhubbert/bubbletea-overlay"
)

// View composes the whole screen: one strip row, the active tab's panel, one
// footer — then composites whatever floats are up on top of that canvas.
//
// Both chrome rows are locked at one line each, so the panel absorbs every
// change in terminal height and nothing shifts vertically (tdp L3).
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
	// line each (tdp L3) is what stops the panel shifting under a resize; it was
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

	out = m.composeFloats(out)
	// The toast is feedback about what just happened, so it sits above the stack
	// and out of its way — low, where it does not cover the surface being used.
	// It holds no keyboard and is not a layer (tdp F8): nothing is dimmed for it.
	if m.toast.isActive() {
		out = compositeDisp(m.toast.view(), out, overlay.Center, overlay.Bottom, 0, -2)
	}
	// Every frame carries the announcement. It costs ~40 invisible bytes and
	// buys idempotence: a parent that missed one report gets the next one,
	// with no edge to lose and no channel to keep alive (§11.44).
	return out + m.nestAnnounce()
}

// float is one popup as the compositor sees it.
type float struct {
	active bool
	owns   bool
	layer  int
	view   func() string
}

// floats lists the popups bottom to top — the order they are drawn in. The
// Space menu goes down first so anything it launches lands above it and Esc
// unwinds in the order the user built the stack.
//
// The credential picker sits above both forms: it is OPENED FROM the host form,
// and a popup painted under the surface that launched it is a popup that never
// opened — which is exactly how it shipped the first time. isActive tests
// cannot catch a z-order bug; only a rendered frame can.
//
// The leaving question and the help can be raised from on top of anything
// (tdp K9, K6), so they are drawn above every float they may cover — the help
// highest but one, since it can be opened over the question too (tdp D3).
// ssh's question outranks every float it may have arrived on top of: it holds
// the keyboard (app.go), so it has to be the thing on top.
func (m AppModel) floats() []float {
	f := func(a popupAnimator, layer int, view func() string) float {
		return float{active: a.isActive(), owns: a.owns(), layer: layer, view: view}
	}
	return []float{
		f(m.spaceMenu.anim, m.spaceMenu.layer, m.spaceMenu.view),
		f(m.globalMenu.anim, m.globalMenu.layer, m.globalMenu.view),
		f(m.lockMenu.anim, m.lockMenu.layer, m.lockMenu.view),
		f(m.hostPicker.anim, m.hostPicker.layer, m.hostPicker.view),
		f(m.transfersUI.anim, m.transfersUI.layer,
			func() string { return m.transfersUI.view(m.transfers.jobs) }),
		f(m.viewer.anim, m.viewer.layer, m.viewer.view),
		f(m.detail.anim, m.detail.layer, m.detail.view),
		f(m.editorUI.anim, m.editorUI.layer, m.editorUI.view),
		f(m.confirm.anim, m.confirm.layer, m.confirm.view),
		f(m.input.anim, m.input.layer, m.input.view),
		f(m.form.anim, m.form.layer, m.form.view),
		f(m.credFormUI.anim, m.credFormUI.layer, m.credFormUI.view),
		f(m.sshcfgFormUI.anim, m.sshcfgFormUI.layer, m.sshcfgFormUI.view),
		f(m.knownAddUI.anim, m.knownAddUI.layer, m.knownAddUI.view),
		f(m.credPicker.anim, m.credPicker.layer, m.credPicker.view),
		f(m.picker.anim, m.picker.layer, m.picker.view),
		f(m.quitAsk.anim, m.quitAsk.layer, m.quitAsk.view),
		f(m.help.anim, m.help.layer, m.help.view),
		f(m.askpassUI.anim, m.askpassUI.layer, m.askpassUI.view),
	}
}

// composeFloats lays the popups over the screen, and dims everything that is
// not the top one (tdp F8). The top is the last popup that still holds the
// keyboard — the same test routing uses (owns, tdp D3) — so the moment one
// starts to close, the one under it is lit again; while the last one closes,
// it is still the top.
func (m AppModel) composeFloats(out string) string {
	fl := m.floats()
	top := -1
	for i, f := range fl {
		if f.active && (f.owns || top < 0 || !fl[top].owns) {
			top = i
		}
	}
	if top < 0 {
		return out
	}
	out = dimANSI(out)
	for i, f := range fl {
		if !f.active {
			continue
		}
		v := f.view()
		if i != top {
			v = dimANSI(v)
		}
		out = compositeDisp(v, out, overlay.Center, overlay.Center, 0, 0)
	}
	return out
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
// ways in two places is a mode documented wrong in one of them (§11.33, tdp K11).
func copyLegendPairs() [][2]string {
	return [][2]string{
		// ? leads, as on every other footer (tdp K11, M1): the mode's key
		// reference behind it carries whatever the row has to drop. Space is
		// not listed — it does nothing in a mode.
		{"?", "help"},
		{"y", "copy"},
		{"v/V", "select"},
		// The way out sits third, ahead of the motions: keyLegend drops from
		// the end, and seven pairs are one more than an 80-column row holds.
		// What goes on that row is u/d — the only pair with another spelling
		// on the same page (j and k, held down).
		{"Alt-v", "leave"},
		{"h/j/k/l", "move"},
		{"w/e/b", "word"},
		{"0/$", "line start/end"},
		{"u/d", "half page"},
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

// footer is the mandatory disclosure channel for both entry keys (tdp M1). A
// user who never opened a README learns from this row that Space and ?
// exist — without it the entry keys are unreachable and nothing else says they
// are there.
//
// That last clause used to read "and X collapses", after VTP's
// disclosure score. The score was removed from VTP entirely, so the
// sentence pointed at something that no longer exists — the CLAIM survives it,
// because it never depended on a number.
func (m AppModel) footer() string {
	// While the remote holds the keyboard every other entry in this row is a lie
	// — space, ?, the digits, q all travel to the far end. So the row says the one
	// thing that is still true, which is also the only way back out. This is the
	// mandatory disclosure for Alt+Esc: it is advertised exactly where it means
	// something, and nowhere else.
	//
	// That holds from the moment the cell has the keyboard, not from the moment
	// the far end first speaks: while it is connecting every other key is
	// swallowed, and Alt+Esc is the only one that does anything (tdp K10, M1).
	if m.ptyFocused() {
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
			out = "unzoom"
		}
		// A LOCKED cell keeps only one key, so the row says only that: every
		// other entry would be a lie — the same honesty rule this row already
		// follows about the remote's keys (§11.43).
		if s := m.ssh.currentSession(); s != nil && s.locked {
			return keyLegend([][2]string{{"Alt-Enter", "release"}}, m.w)
		}
		pairs := [][2]string{{"Alt-Esc", out}}
		// And the scrollback keys, but only where they would do something: they
		// are the remote's while a full-screen program is up, and there is
		// nothing to page through until more has been said than fits (§11.19).
		if m.inPty() && m.ssh.canScroll() {
			pairs = append(pairs, [2]string{"PgUp/PgDn", "history"})
		}
		// The way to get text OUT of the cell. Unconditional: inPty already
		// means a live session that has spoken, which is everything the mode
		// needs. It sits ahead of the cell chord because keyLegend drops from
		// the END on a cramped footer, and there is no other way to reach this
		// at all — a bare `?` in here belongs to the remote, so the footer is
		// the only live disclosure the pty has (§11.33, §11.19).
		if m.inPty() {
			pairs = append(pairs, [2]string{"Alt-v", "select"})
		}
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
		pairs = append(pairs, [2]string{"Alt-z", m.ssh.nextZoomLabel()})
		// Last of the three: the lock chord is a nested-session tool, the
		// rarest need of the group.
		pairs = append(pairs, [2]string{"Alt-Enter", "lock"})
		pairs = append(pairs, [2]string{"Alt-" + arrowGlyphs + "/" + arrowUpDown, "cell"})
		return keyLegend(pairs, m.w)
	}
	// An Operation page eats every printable key — the digits, space, ?, q and
	// the tab letters — so the row says only what is still true, the same
	// honesty the pty row keeps. Esc first, then the tab keys answer again.
	if m.textPage() {
		return keyLegend([][2]string{{"Tab", "field"}, {"Enter", "run"},
			{"Esc", "back"}}, m.w)
	}
	// The digits offered are the ones the current tab actually shows (tdp M5): a
	// number the screen does not display is a number the keyboard ignores.
	//
	// Tab moves between panels too, except on the ssh tab (a deviation from
	// tdp K2, see dev-remarks), so it is offered where it works.
	panels := [2]string{"Tab/1–2", "panels"}
	switch m.tab {
	case tabFT:
		panels = [2]string{"Tab/1–4", "panels"}
	case tabSSH:
		panels = [2]string{"1–2", "panels"}
	}
	// Unread errors are disclosed against the key that reaches them: the log
	// lives at manage → logs, and a record nobody is told about is a record
	// nobody opens.
	pairs := [][2]string{{"Space", "menu"}, {"?", "help"}, panels, {"M/F/S", "tabs"}}
	if n := m.errors.unreadErrors(); n > 0 {
		pairs = append(pairs, [2]string{"M", plural(n, "unread error")})
	}
	pairs = append(pairs, [2]string{"q", "quit"})
	return keyLegend(pairs, m.w)
}
