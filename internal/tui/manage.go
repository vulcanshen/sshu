package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	terminu "github.com/vulcanshen/terminu-go"
)

// The Manage screen: [1] SECTIONS, a grouped list of what sshu keeps, and [2],
// the section under [1]'s cursor (components/layout/panel, details). Moving
// the cursor swaps [2] at once; Enter on a section sends the focus there
// (Rules K3).

// section is one row of [1] and what [2] shows for it.
type section struct {
	id, label, group string
	body             terminu.Body
	actions          func() []terminu.Action
	// shown runs when the section comes into [2].
	shown func()
}

type manage struct {
	s       *sshu
	screen  *terminu.Screen
	nav     *terminu.List
	navP    *terminu.Panel
	content *terminu.Panel

	sections []*section
	cur      *section

	hosts *hostsSection
	creds *credsSection
	cfg   *sshcfgSection
	known *knownSection
	errs  *errorsSection
	conns *connsSection
	chgs  *changesSection
}

const navWidth = 20

func newManage(s *sshu) *manage {
	m := &manage{s: s}
	m.hosts = newHostsSection(s)
	m.creds = newCredsSection(s)
	m.cfg = newSSHCfgSection(s)
	m.known = newKnownSection(s)
	m.errs = newErrorsSection(s)
	m.conns = newConnsSection(s)
	m.chgs = newChangesSection(s)
	m.sections = []*section{
		m.hosts.section(), m.creds.section(),
		m.cfg.section(), m.known.section(),
		m.errs.section(), m.conns.section(), m.chgs.section(),
	}
	m.nav = &terminu.List{Columns: []terminu.Column{{Title: "Section", Flex: true}}, OnMove: m.navMoved}
	rows := make([]terminu.Row, len(m.sections))
	for i, sec := range m.sections {
		rows[i] = terminu.Row{ID: sec.id, Cells: []string{sec.label}, Group: sec.group}
	}
	m.nav.SetRows(rows)
	m.content = &terminu.Panel{}
	m.navP = &terminu.Panel{Label: "Sections", Body: m.nav, Details: m.content, Actions: []terminu.Action{
		{Key: "Enter", Label: "Open", Desc: "go to this section", Scope: terminu.ItemScope, Hint: "open",
			ToDetails: func() bool { return true }},
	}}
	m.show(m.sections[0])
	m.screen = &terminu.Screen{Name: "Manage", Key: "M", Desc: "hosts, credentials, ~/.ssh, logs",
		Panels: []*terminu.Panel{m.navP, m.content}, NarrowBelow: 60,
		Layout: func(w, h int) []terminu.Rect {
			left := min(navWidth, w/3)
			return []terminu.Rect{{X: 0, Y: 0, W: left, H: h}, {X: left, Y: 0, W: w - left, H: h}}
		}}
	return m
}

// navMoved puts the section under the cursor into [2].
func (m *manage) navMoved(r terminu.Row) tea.Cmd {
	for _, sec := range m.sections {
		if sec.id == r.ID {
			m.show(sec)
		}
	}
	return nil
}

func (m *manage) show(sec *section) {
	m.cur = sec
	m.content.Label = sec.label
	m.content.Body = sec.body
	m.content.Actions = sec.actions() // the lib adds the list's own (the filter)
	if sec.shown != nil {
		sec.shown()
	}
}

// refresh registers the current section's actions again, after a change that
// adds or takes away rows.
func (m *manage) refresh() { m.show(m.cur) }

// journalsChanged redraws the three journal sections from the data.
func (m *manage) journalsChanged() {
	if m.errs == nil {
		return // still building
	}
	m.errs.reload()
	m.conns.reload()
	m.chgs.reload()
	if m.cur != nil {
		m.refresh()
	}
}

// unreadErrors is how many errors came in since Errors was last on screen.
func (m *manage) unreadErrors() int {
	if m.s.a.Screen() == m.screen && m.cur == m.errs.sec {
		m.errs.seen()
	}
	return m.errs.unread()
}
