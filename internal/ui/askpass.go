package ui

import (
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// An sshconfig host's sftp dial runs the real ssh with no terminal (§11.52),
// so every question ssh has — a password, a passphrase, an unknown host key
// — goes to SSH_ASKPASS, and SSH_ASKPASS is sshu itself. This file is both
// ends of that: the helper that ssh runs, which relays the prompt over a
// socket, and the popup in the sshu that is drawing, which asks the user and
// relays the answer back.
//
// The prompt is ssh's own words, verbatim: "demo@host's password: ", or the
// four lines ending in "(yes/no/[fingerprint])?". sshu does not know in
// advance which question is coming, or whether one is coming at all — a key
// the agent holds asks nothing — and it does not try to guess. It shows what
// ssh asked, when ssh asked it. Measured against OpenSSH 10.3: no
// SSH_ASKPASS_PROMPT hint arrives for either kind, so the two are told apart
// by the "(yes/no" every confirmation carries.
//
// The answer is never written down. It goes to the helper's stdout, which is
// ssh's stdin for that one read, and nowhere else.

// askpassSockEnv carries the socket path to the helper. Its presence is what
// puts the re-executed sshu into relay mode rather than the TUI.
const askpassSockEnv = "SSHU_ASKPASS_SOCK"

// AskpassSock reports the socket when this process was started as ssh's
// askpass helper for an sshconfig dial, or "" for a normal run.
func AskpassSock() string { return os.Getenv(askpassSockEnv) }

// RunAskpassRelay is the helper: send the prompt, print the reply. A non-zero
// exit is how ssh learns there is no answer — the user cancelled, or the
// sshu that asked is gone.
func RunAskpassRelay(sock, prompt string) int {
	return runAskpassRelay(sock, prompt, os.Stdout)
}

func runAskpassRelay(sock, prompt string, out io.Writer) int {
	c, err := net.Dial("unix", sock)
	if err != nil {
		return 1
	}
	defer c.Close()
	if _, err := io.WriteString(c, prompt); err != nil {
		return 1
	}
	// Half-close: the prompt is complete, and the far end reads to EOF
	// because a prompt can be several lines.
	if u, ok := c.(*net.UnixConn); ok {
		_ = u.CloseWrite()
	}
	reply, err := io.ReadAll(c)
	if err != nil || len(reply) == 0 {
		return 1
	}
	_, _ = out.Write(reply)
	return 0
}

// ------------------------------------------------------------- the TUI side

// askpassRequest is one question, from one dial, waiting for one answer.
type askpassRequest struct {
	sd     side
	gen    int
	prompt string
	conn   net.Conn
}

// isConfirm tells a yes/no question from a secret. The host key prompt is
// the one that matters, and every OpenSSH phrasing of it carries "(yes/no".
func (r *askpassRequest) isConfirm() bool {
	return strings.Contains(strings.ToLower(r.prompt), "(yes/no")
}

// answer sends the reply and hangs up. The newline is what ssh reads up to;
// an empty password is one newline, which is why refuse below sends nothing
// at all — zero bytes is the only reply that cannot be mistaken for one.
func (r *askpassRequest) answer(text string) {
	_, _ = io.WriteString(r.conn, text+"\n")
	_ = r.conn.Close()
}

// refuse hangs up without a reply: the helper exits 1 and ssh gives up.
func (r *askpassRequest) refuse() { _ = r.conn.Close() }

// askpassRequestMsg carries a question into the update loop.
type askpassRequestMsg struct{ req *askpassRequest }

// askpassServer is the socket one dial's helpers connect to. One per dial,
// not one per app: the path is what ties a prompt to the side and the
// generation it belongs to, so a question from a dial the user has already
// abandoned is recognised as such rather than answered into the void.
type askpassServer struct {
	sd   side
	gen  int
	dir  string
	ln   net.Listener
	reqs chan *askpassRequest
	done chan struct{}
	once sync.Once
}

// newAskpassServer listens on a fresh socket in a directory only this user
// can enter. The temp dir rather than the config dir: a socket is not
// config, and a path under $TMPDIR stays under the unix-socket length limit
// on both platforms.
func newAskpassServer(sd side, gen int) (*askpassServer, error) {
	dir, err := os.MkdirTemp("", "sshu-askpass-")
	if err != nil {
		return nil, err
	}
	ln, err := net.Listen("unix", filepath.Join(dir, "s"))
	if err != nil {
		_ = os.RemoveAll(dir)
		return nil, err
	}
	s := &askpassServer{sd: sd, gen: gen, dir: dir, ln: ln,
		reqs: make(chan *askpassRequest, 8), done: make(chan struct{})}
	go s.accept()
	return s, nil
}

func (s *askpassServer) path() string { return s.ln.Addr().String() }

func (s *askpassServer) accept() {
	for {
		c, err := s.ln.Accept()
		if err != nil {
			return // closed
		}
		go func(c net.Conn) {
			b, err := io.ReadAll(c) // to the helper's half-close
			if err != nil {
				_ = c.Close()
				return
			}
			select {
			case s.reqs <- &askpassRequest{sd: s.sd, gen: s.gen, prompt: string(b), conn: c}:
			case <-s.done:
				_ = c.Close()
			}
		}(c)
	}
}

// next waits for the next question and delivers it as a message. Re-armed
// after each one by the handler, which is what makes the questions arrive
// one at a time in the order ssh asked them.
func (s *askpassServer) next() tea.Cmd {
	if s == nil {
		return nil
	}
	return func() tea.Msg {
		select {
		case r := <-s.reqs:
			return askpassRequestMsg{req: r}
		case <-s.done:
			return nil
		}
	}
}

// close stops listening, refuses anything still queued, and removes the
// socket. Safe to call more than once and on nil.
func (s *askpassServer) close() {
	if s == nil {
		return
	}
	s.once.Do(func() {
		close(s.done)
		_ = s.ln.Close()
		_ = os.RemoveAll(s.dir)
		for {
			select {
			case r := <-s.reqs:
				r.refuse()
			default:
				return
			}
		}
	})
}

// ------------------------------------------------------------------ popup

// askpassPopup is ssh's question, put to the user. Two shapes, decided by
// the question: a masked line for a secret, a yes/no for a host key. It is
// its own float rather than a use of inputPopup or confirmPopup because it
// arrives on its own schedule — the rename box may be open when ssh asks —
// and it has to take the keyboard from whatever is up, since ssh is waiting.
type askpassPopup struct {
	anim    popupAnimator
	req     *askpassRequest
	title   string // the host being dialled
	value   string
	layer   int
	screenW int
	screenH int
}

func newAskpassPopup() askpassPopup { return askpassPopup{anim: newPopupAnimator("askpass")} }

func (m askpassPopup) isActive() bool      { return m.anim.isActive() }
func (m askpassPopup) isInteractive() bool { return m.anim.isInteractive() }
func (m *askpassPopup) close() tea.Cmd     { return m.anim.close() }
func (m *askpassPopup) setSize(w, h int)   { m.screenW, m.screenH = w, h }

func (m *askpassPopup) ask(req *askpassRequest, title string, layer int) tea.Cmd {
	m.req, m.title, m.value, m.layer = req, title, "", layer
	return m.anim.open()
}

// update edits the line and reports Enter. Esc is the caller's, as on every
// float (§4.3). A yes/no question takes no text: the only answers are the
// two keys.
func (m *askpassPopup) update(msg tea.KeyMsg) (done bool) {
	if !m.anim.isInteractive() {
		return false
	}
	switch msg.Type {
	case tea.KeyEnter:
		return true
	case tea.KeyBackspace:
		if r := []rune(m.value); len(r) > 0 {
			m.value = string(r[:len(r)-1])
		}
	case tea.KeySpace:
		if !m.req.isConfirm() {
			m.value += " "
		}
	case tea.KeyRunes:
		if !m.req.isConfirm() {
			m.value += string(msg.Runes)
		}
	}
	return false
}

// reply is what Enter sends: "yes" to a question that takes yes, the line
// otherwise.
func (m askpassPopup) reply() string {
	if m.req.isConfirm() {
		return "yes"
	}
	return m.value
}

func (m askpassPopup) view() string {
	if m.req == nil {
		return ""
	}
	dim := lipgloss.NewStyle().Foreground(dimColor)
	txt := lipgloss.NewStyle().Foreground(textColor)
	edit := lipgloss.NewStyle().Foreground(editColor)
	cur := lipgloss.NewStyle().Foreground(lipgloss.Color(baseHex)).Background(editColor)

	lines := strings.Split(strings.TrimRight(m.req.prompt, " \n"), "\n")
	w := 44
	for _, l := range lines {
		w = max(w, dispW(l)+4)
	}
	innerW := popupInnerW(m.screenW, w)

	var rows []string
	glyph, accept := glyphLock, "send"
	if m.req.isConfirm() {
		// The host key question, as ssh wrote it: the first line is the
		// claim, the fingerprint and the rest sit dim under it, and the
		// question is the last line. Warn-coloured because that is what it
		// is — the one prompt where "yes" writes something to disk.
		glyph, accept = glyphWarn, "yes"
		for i, l := range lines {
			style := dim
			if i == 0 {
				style = txt
			}
			rows = append(rows, style.Render(padRight("  "+l, innerW)))
		}
	} else {
		// A secret: the prompt, a gap, and a line of bullets — one per rune,
		// as the form draws a password being typed (§6.3).
		for _, l := range lines {
			rows = append(rows, dim.Render(padRight("  "+l, innerW)))
		}
		masked := strings.Repeat("•", len([]rune(m.value)))
		masked = truncate(masked, innerW-3)
		rows = append(rows, spaces(innerW),
			" "+edit.Render(masked)+cur.Render(" ")+spaces(max(0, innerW-2-dispW(masked))))
	}

	hint := hintLegend([][2]string{{"Enter", accept}, {"Esc", "cancel connection"}})
	return drawPopupBox(popupLayerColor(m.layer), " "+glyph+" "+m.title+" ",
		hint, animRows(m.anim, capRows(rows, m.screenH)), innerW)
}

// -------------------------------------------------------------- app side

// askpassArrived is a question from a dial. It goes straight up if nothing
// is being asked, and queues behind the current question otherwise — one at
// a time, because two password boxes on screen is a way to type one into the
// other. A question from a dial that is no longer the side's current one is
// refused: its ssh is being killed anyway, and answering it would be
// answering a host the user already left.
func (m AppModel) askpassArrived(r *askpassRequest) (tea.Model, tea.Cmd) {
	s := &m.sftp.sides[r.sd]
	if r.gen != s.dialGen || s.askpass == nil {
		r.refuse()
		return m, nil
	}
	rearm := s.askpass.next()
	if m.askpassUI.isActive() {
		m.askpassQueue = append(m.askpassQueue, r)
		return m, rearm
	}
	return m, tea.Batch(rearm, m.askpassUI.ask(r, s.dialing, m.layer()))
}

func (m AppModel) askpassKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if !m.askpassUI.update(msg) {
		return m, nil
	}
	m.askpassUI.req.answer(m.askpassUI.reply())
	return m.askpassNext()
}

// askpassCancel is Esc on the question: the user is not going to answer it,
// so the connection is off. Off by killing the ssh rather than by sending
// "no" or an empty password — ssh asks for a password three times before it
// gives up, and a cancel that came back as a second box would not have been
// a cancel.
func (m AppModel) askpassCancel() (tea.Model, tea.Cmd) {
	r := m.askpassUI.req
	r.refuse()
	m.sftp.sides[r.sd].abortDial(r.gen)
	return m.askpassNext()
}

// askpassNext closes the current question and puts up the next one that is
// still worth asking.
func (m AppModel) askpassNext() (tea.Model, tea.Cmd) {
	for len(m.askpassQueue) > 0 {
		r := m.askpassQueue[0]
		m.askpassQueue = m.askpassQueue[1:]
		s := &m.sftp.sides[r.sd]
		if r.gen != s.dialGen || s.askpass == nil {
			r.refuse()
			continue
		}
		return m, m.askpassUI.ask(r, s.dialing, m.layer())
	}
	return m, m.askpassUI.close()
}

// askpassDrop retires every question from one dial: the one on screen, if it
// is that dial's, and any queued behind it. Called when the dial ends —
// whatever ssh was asking, it has stopped waiting for the answer.
func (m *AppModel) askpassDrop(sd side, gen int) tea.Cmd {
	kept := m.askpassQueue[:0]
	for _, r := range m.askpassQueue {
		if r.sd == sd && r.gen == gen {
			r.refuse()
			continue
		}
		kept = append(kept, r)
	}
	m.askpassQueue = kept
	if r := m.askpassUI.req; m.askpassUI.isActive() && r != nil && r.sd == sd && r.gen == gen {
		r.refuse()
		mm, cmd := m.askpassNext()
		*m = mm.(AppModel)
		return cmd
	}
	return nil
}
