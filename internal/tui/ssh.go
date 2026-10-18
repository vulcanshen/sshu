package tui

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	terminu "github.com/vulcanshen/terminu-go"

	"github.com/vulcanshen/sshu/internal/store"
)

// The SSH screen: [1] SESSIONS, the live sessions; [2] LAYOUT, how the grid
// is arranged; and the grid, one panel per session from [3] on, each a real
// ssh in a PTY (components/dialog/terminal: a PTY in a panel). Enter on a
// session puts the keyboard in its cell; Alt-Esc takes it back, asking first
// (Rules K10).

type sshScreen struct {
	s       *sshu
	screen  *terminu.Screen
	list    *terminu.List
	listP   *terminu.Panel
	layout  *terminu.Values
	layoutP *terminu.Panel

	sessions []*session
	nextID   int

	arrange int // 0 horizontal, 1 vertical, 2 custom
	columns int // the custom grid's columns
	zoomed  bool
}

// session is one ssh connection and its cell.
type session struct {
	id     int
	host   store.Host // as given
	addr   store.Host // resolved: what ssh is handed
	term   *terminu.Terminal
	panel  *terminu.Panel
	hidden bool
	ended  bool
	reason string
	log    string // ssh's own messages (-E), read when it ends
}

var arrangements = []string{"horizontal", "vertical", "custom"}

const sshLeftW = 30 // a row says <glyph> user@host:port #N whole for the common shapes

func newSSHScreen(s *sshu) *sshScreen {
	h := &sshScreen{s: s, columns: 2}
	h.list = &terminu.List{Columns: []terminu.Column{{Title: "Session", Flex: true}},
		EmptyFact: "No live sessions", EmptyHint: "Press [M] and connect to a host"}
	h.listP = &terminu.Panel{Label: "Sessions", Body: h.list, Actions: h.listActions()}
	arrange := &terminu.Field{Label: "Arrange", Options: arrangements, Selected: 0}
	arrange.OnChange = func() tea.Cmd { h.arrange = arrange.Selected; h.resize(); return nil }
	cols := &terminu.Field{Label: "Columns", Value: func() string { return strconv.Itoa(h.columns) },
		Disabled: func() bool { return h.arrange != 2 },
		Edit: func() tea.Cmd {
			return h.s.a.Number(terminu.NumberInput{Title: "Columns", Glyph: glyphGrid, Prompt: "How many cells across, 1–9",
				Verb: "set", Old: strconv.Itoa(h.columns), Min: 1, Max: 9,
				Done: func(v string) tea.Cmd { h.columns, _ = strconv.Atoi(v); h.resize(); return nil }})
		}}
	h.layout = &terminu.Values{Fields: []*terminu.Field{arrange, cols}}
	h.layoutP = &terminu.Panel{Label: "Layout", Body: h.layout}
	h.screen = &terminu.Screen{Name: "SSH", Key: "S", Desc: "the grid of live sessions",
		Panels: []*terminu.Panel{h.listP, h.layoutP}, NarrowBelow: sshLeftW + 28, Layout: h.place}
	h.listP.OtherKeys = []terminu.KeySection{{Title: "ssh grid", Rows: []terminu.KeyRow{
		{Key: "Alt-Esc", Desc: "the keyboard back out of a cell, asked first"},
		{Key: "Alt-←|→|↑|↓", Desc: "the cell that way"},
		{Key: "Alt-z", Desc: "the cell over the whole screen, and back"},
		{Key: "PgUp|PgDn", Desc: "the cell's history"},
	}}}
	return h
}

func (h *sshScreen) start() tea.Cmd { return nil }

func (h *sshScreen) liveCount() int {
	n := 0
	for _, x := range h.sessions {
		if !x.ended {
			n++
		}
	}
	return n
}

func (h *sshScreen) listActions() []terminu.Action {
	none := func() bool { return h.liveCount() == 0 }
	return []terminu.Action{
		{Key: "Enter", Label: "Open", Desc: "show it and take the keyboard", Scope: terminu.ItemScope, Hint: "open", Do: h.open},
		{Key: "H", Label: "Hide", Desc: "hide or show this session's cell", Scope: terminu.ItemScope, Hint: "hide", Do: h.toggleHidden},
		{Key: "C", Label: "Close", Desc: "end this session", Scope: terminu.ItemScope, Do: h.askClose},
		{Key: "D", Label: "Duplicate", Desc: "another session to this host", Scope: terminu.ItemScope, Do: h.askDuplicate},
		// No hotkey: a sweep of every session has no fast path.
		{Label: "Close all sessions", Desc: "end every one of them", Scope: terminu.PanelScope, Hidden: none, Do: h.askCloseAll},
	}
}

// --- the grid --------------------------------------------------------------------

// place lays the screen out: the sessions and the layout down the left, the
// cells of the shown sessions on the right in the chosen arrangement. With
// the keyboard in a cell the left column goes, and a zoomed cell takes the
// whole panel area. The panels after the shown cells get nothing.
func (h *sshScreen) place(w, hh int) []terminu.Rect {
	h.prune()
	ps := h.screen.Panels
	rects := make([]terminu.Rect, len(ps))
	focused := h.screen.Focused()
	inside := false
	for _, x := range h.sessions {
		if x.panel == focused && x.term.Inside() {
			inside = true
		}
	}
	left := sshLeftW
	switch {
	case inside:
		left = 0
	case !h.anyShown():
		left = w // no cell to give the room to
	}
	layoutH := min(7, hh/3)
	if left > 0 {
		rects[0] = terminu.Rect{X: 0, Y: 0, W: left, H: hh - layoutH}
		rects[1] = terminu.Rect{X: 0, Y: hh - layoutH, W: left, H: layoutH}
	}
	gx, gw := left, w-left
	var cells []int
	for i := 2; i < len(ps); i++ {
		x := h.byPanel(ps[i])
		if x != nil && (!x.hidden || ps[i] == focused) {
			cells = append(cells, i)
		}
	}
	if h.zoomed {
		for _, i := range cells {
			if ps[i] == focused {
				only := make([]terminu.Rect, len(ps))
				only[i] = terminu.Rect{X: 0, Y: 0, W: w, H: hh}
				return only
			}
		}
	}
	n := len(cells)
	if n == 0 {
		return rects
	}
	cols, rows := n, 1
	switch h.arrange {
	case 1:
		cols, rows = 1, n
	case 2:
		cols = min(max(h.columns, 1), n)
		rows = (n + cols - 1) / cols
	}
	ws, hs := splitEven(gw, cols), splitEven(hh, rows)
	for k, i := range cells {
		c, r := k%cols, k/cols
		x := gx
		for j := 0; j < c; j++ {
			x += ws[j]
		}
		y := 0
		for j := 0; j < r; j++ {
			y += hs[j]
		}
		cw := ws[c]
		if c == cols-1 || k == n-1 && r == rows-1 {
			cw = gx + gw - x // the last cell of a row takes what is left
		}
		rects[i] = terminu.Rect{X: x, Y: y, W: cw, H: hs[r]}
	}
	return rects
}

// anyShown reports a cell on the grid.
func (h *sshScreen) anyShown() bool {
	for _, x := range h.sessions {
		if !x.ended && !x.hidden {
			return true
		}
	}
	return false
}

func splitEven(total, parts int) []int {
	out := make([]int, max(parts, 0))
	for i := range out {
		out[i] = total / parts
		if i < total%parts {
			out[i]++
		}
	}
	return out
}

func (h *sshScreen) byPanel(p *terminu.Panel) *session {
	for _, x := range h.sessions {
		if x.panel == p {
			return x
		}
	}
	return nil
}

func (h *sshScreen) byID(id string) *session {
	for _, x := range h.sessions {
		if strconv.Itoa(x.id) == id {
			return x
		}
	}
	return nil
}

// prune takes the cells of ended sessions off the screen. Only the panels
// after the focused one can go: the lib keeps the focus by its place in the
// list, so taking one off at or before it would move the focus.
// TODO(lib SS3): terminu-go has no call to add or remove a panel, or to
// move the focus; an ended cell at or before the focus waits there.
func (h *sshScreen) prune() {
	ps := h.screen.Panels
	f := slices.Index(ps, h.screen.Focused())
	kept := ps[:0:0]
	for i, p := range ps {
		if x := h.byPanel(p); i > f && i >= 2 && (x == nil || x.ended) {
			continue
		}
		kept = append(kept, p)
	}
	if len(kept) != len(ps) {
		h.screen.Panels = kept
	}
	h.sessions = slices.DeleteFunc(h.sessions, func(x *session) bool {
		return x.ended && !slices.Contains(h.screen.Panels, x.panel)
	})
}

// resize is a change of layout: the next frame lays the cells out again,
// and the lib sizes each PTY to its cell when it draws it.
func (h *sshScreen) resize() {}

// --- sessions --------------------------------------------------------------------

func (h *sshScreen) label(x *session) string {
	a := x.addr
	s := destination(a)
	if a.Port > 0 && a.Port != store.DefaultPort {
		s += ":" + strconv.Itoa(a.Port)
	}
	return s + " #" + strconv.Itoa(x.id)
}

func (h *sshScreen) redrawList() {
	var rows []terminu.Row
	for _, x := range h.sessions {
		if x.ended {
			continue
		}
		glyph := terminu.Fg(terminu.Green).Render(authGlyph(string(x.host.Auth)))
		if x.hidden {
			glyph = terminu.Fg(terminu.Overlay0).Render(authGlyph(string(x.host.Auth)))
		}
		rows = append(rows, terminu.Row{ID: strconv.Itoa(x.id), Cells: []string{glyph + " " + h.label(x)}})
	}
	h.list.SetRows(rows)
}

func (h *sshScreen) current() *session {
	r, ok := h.list.Current()
	if !ok {
		return nil
	}
	return h.byID(r.ID)
}

// connect starts a session to host and puts the keyboard in it: from the
// hosts table it is one move, and the menu behind it is left behind (Rules
// T1: a long session clears its source).
func (h *sshScreen) connect(host store.Host) tea.Cmd {
	x, err := h.start1(host)
	if err != nil {
		h.s.logError(store.LevelError, host.Name, host.User, err.Error(), err.Error())
		return h.s.a.Error("Connect", err.Error())
	}
	cmds := []tea.Cmd{x.term.Start()}
	if h.s.a.Screen() != h.screen {
		cmds = append(cmds, pressKey("S"))
	}
	return tea.Sequence(append(cmds, h.focus(x))...)
}

// start1 makes the session and its cell; it starts when the cell is drawn.
func (h *sshScreen) start1(host store.Host) (*session, error) {
	addr, err := store.Resolve(host, h.s.d.Creds)
	if err != nil {
		return nil, err
	}
	if _, err := exec.LookPath(sshBinary); err != nil {
		return nil, errors.New("no ssh on PATH: the SSH screen runs OpenSSH's ssh")
	}
	h.nextID++
	x := &session{id: h.nextID, host: host, addr: addr}
	if f, err := os.CreateTemp("", "sshu-ssh-*.log"); err == nil {
		x.log = f.Name()
		f.Close()
	}
	x.term = &terminu.Terminal{Title: "SSH", Subject: host.Name, History: true,
		Command: sshArgs(addr, h.s.d.Config.Seconds(), x.log), Env: sshEnv(addr, selfPath())}
	// What sshu's own PTY did and terminu-go's Terminal cannot yet:
	// TODO(lib SS4): Alt-v, the selection mode that copies out of a cell,
	// needs the screen's text.
	// TODO(lib SS5): the nest channel — telling a sshu inside a cell how wide
	// an icon is (tdp E5), seeing the chain, driving its lock and zoom —
	// needs to write into the PTY and read what the program sends.
	// TODO(lib SS6): Alt-Enter, the lock that hands every key, Alt-Esc too, to
	// a nested sshu.
	// TODO(lib SS7): Alt-z's last stage, a cell over the whole terminal with
	// no frame, so a nested sshu loses no row.
	// TODO(lib SS11): NotReady and the "connecting" look need to know when
	// the program first prints.
	// TODO(lib SS12): one key row for Alt-←|→|↑|↓; four for now.
	x.term.Keys = []terminu.TerminalKey{
		{Key: "Alt-z", Desc: "zoom", Do: func() tea.Cmd { h.zoomed = !h.zoomed; return nil }},
		{Key: "Alt-←", Desc: "cell", Do: func() tea.Cmd { return h.move(x, -1, 0) }},
		{Key: "Alt-→", Desc: "cell", Do: func() tea.Cmd { return h.move(x, 1, 0) }},
		{Key: "Alt-↑", Desc: "cell", Do: func() tea.Cmd { return h.move(x, 0, -1) }},
		{Key: "Alt-↓", Desc: "cell", Do: func() tea.Cmd { return h.move(x, 0, 1) }},
	}
	x.term.OnExit = func(err error) tea.Cmd { return h.ended(x, err) }
	x.panel = &terminu.Panel{Label: h.label(x), Body: x.term, Stream: true, Actions: []terminu.Action{
		{Key: "Enter", Label: "Type", Desc: "the keyboard into this session; Alt-Esc takes it back", Scope: terminu.PanelScope,
			Hint: "type", Do: func() tea.Cmd { x.term.Enter(); return nil }},
		{Key: "C", Label: "Close", Desc: "end this session", Scope: terminu.PanelScope, Do: func() tea.Cmd { return h.askCloseOne(x) }},
	}}
	h.sessions = append(h.sessions, x)
	h.screen.Panels = append(h.screen.Panels, x.panel)
	registerStop(x.term)
	h.redrawList()
	h.s.refreshQuit()
	return x, nil
}

// focus puts the keyboard in x's cell: the cell gets the focus and its PTY
// the keys.
func (h *sshScreen) focus(x *session) tea.Cmd {
	x.hidden = false
	h.redrawList()
	x.term.Enter()
	i := slices.Index(h.screen.Panels, x.panel)
	if i < 0 {
		return nil
	}
	return focusIndex(h.screen, i)
}

// focusIndex moves the focus to panel i of screen as the keys would: its
// digit, or Tab enough times past [9]. TODO(lib SS3).
func focusIndex(screen *terminu.Screen, i int) tea.Cmd {
	if i < 9 {
		return focusPanel(i + 1)
	}
	f := slices.Index(screen.Panels, screen.Focused())
	n := (i - f + len(screen.Panels)) % len(screen.Panels)
	cmds := make([]tea.Cmd, n)
	for k := range cmds {
		cmds[k] = pressKey("Tab")
	}
	return tea.Sequence(cmds...)
}

// pressKey is a key pressed by sshu itself: what terminu-go has no call for.
func pressKey(k string) tea.Cmd {
	return func() tea.Msg {
		if k == "Tab" {
			return tea.KeyMsg{Type: tea.KeyTab}
		}
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
	}
}

func (h *sshScreen) open() tea.Cmd {
	x := h.current()
	if x == nil {
		return nil
	}
	return h.focus(x)
}

func (h *sshScreen) toggleHidden() tea.Cmd {
	x := h.current()
	if x == nil {
		return nil
	}
	x.hidden = !x.hidden
	h.redrawList()
	return nil
}

// move goes from x's cell to the shown cell dx, dy away in the grid.
func (h *sshScreen) move(x *session, dx, dy int) tea.Cmd {
	var shown []*session
	for _, y := range h.sessions {
		if !y.ended && (!y.hidden || y == x) && slices.Contains(h.screen.Panels, y.panel) {
			shown = append(shown, y)
		}
	}
	at := slices.Index(shown, x)
	n := len(shown)
	if at < 0 || n < 2 {
		return nil
	}
	cols := n
	switch h.arrange {
	case 1:
		cols = 1
	case 2:
		cols = min(max(h.columns, 1), n)
	}
	to := at + dx + dy*cols
	if to < 0 || to >= n || dx != 0 && to/cols != at/cols {
		return nil
	}
	y := shown[to]
	x.term.Hide(h.s.a)
	return h.focus(y)
}

func (h *sshScreen) askClose() tea.Cmd {
	if x := h.current(); x != nil {
		return h.askCloseOne(x)
	}
	return nil
}

func (h *sshScreen) askCloseOne(x *session) tea.Cmd {
	return h.s.a.Confirm(terminu.Ask{Title: "Close", Subject: h.label(x), Question: "End the session to " + x.host.Name + "?",
		Verb: "close"}, func() tea.Cmd { x.term.Stop(); return nil })
}

func (h *sshScreen) askCloseAll() tea.Cmd {
	n := h.liveCount()
	return h.s.a.Confirm(terminu.Ask{Title: "Close all", Question: "End " + plural(n, "session") + "?", Verb: "close all"},
		func() tea.Cmd {
			for _, x := range h.sessions {
				x.term.Stop()
			}
			return nil
		})
}

func (h *sshScreen) askDuplicate() tea.Cmd {
	x := h.current()
	if x == nil {
		return nil
	}
	return h.s.a.Confirm(terminu.Ask{Title: "Duplicate", Subject: x.host.Name,
		Question: "Open another session to " + x.host.Name + "?", Verb: "open"},
		func() tea.Cmd { return h.connect(x.host) })
}

// ended settles a session that is over: why it ended goes to the journals
// and a toast; its cell leaves the grid.
func (h *sshScreen) ended(x *session, err error) tea.Cmd {
	x.ended = true
	deregisterStop(x.term)
	words := lastWords(x.log)
	if x.log != "" {
		_ = os.Remove(x.log)
	}
	ok, reason := true, "exited 0"
	var exit *exec.ExitError
	switch {
	case errors.As(err, &exit) && exit.ExitCode() == 255:
		ok, reason = false, cmpOr(words, "ssh exited 255")
	case errors.As(err, &exit) && exit.ExitCode() < 0:
		reason = "closed"
	case errors.As(err, &exit):
		reason = "exited " + strconv.Itoa(exit.ExitCode())
	case err != nil:
		ok, reason = false, err.Error()
	}
	x.reason = reason
	result := store.ResultSuccess
	if !ok {
		result = store.ResultFail
	}
	h.s.logConn(store.ConnectionEntry{Host: x.host.Name, User: x.addr.User, Result: result})
	h.redrawList()
	h.s.refreshQuit()
	if !ok {
		h.s.logError(store.LevelError, x.host.Name, x.addr.User, reason, words)
		return h.s.a.Toast(terminu.Error, x.host.Name+": "+reason)
	}
	return h.s.a.Toast(terminu.Info, "Session to "+x.host.Name+" ended: "+reason)
}

// lastWords is the last thing ssh said, from its own log.
func lastWords(log string) string {
	if log == "" {
		return ""
	}
	b, err := os.ReadFile(log)
	if err != nil {
		return ""
	}
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if l := strings.TrimSpace(lines[i]); l != "" {
			return l
		}
	}
	return ""
}

func (h *sshScreen) onMsg(msg tea.Msg) (tea.Cmd, bool) { return nil, false }

// closeAll ends every session, on the way out.
func (h *sshScreen) closeAll() {
	for _, x := range h.sessions {
		x.term.Stop()
	}
}

var _ = fmt.Sprint
