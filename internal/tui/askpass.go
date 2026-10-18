package tui

import (
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"

	tea "github.com/charmbracelet/bubbletea"
)

// An sshconfig host's file transfer runs ssh with no terminal. Whatever ssh
// asks on the way in — a password, a passphrase, an unknown host key — goes
// to the askpass helper, which is sshu itself with SSHU__ASKPASS_SOCK set: it
// relays the question over a socket to the sshu that is drawing, which asks
// the user.

const askpassSockEnv = "SSHU__ASKPASS_SOCK"

// AskpassSock is the socket of an askpass relay run, "" for a normal one.
func AskpassSock() string { return os.Getenv(askpassSockEnv) }

// RunAskpassRelay is the helper's side: send ssh's prompt, print the answer.
// Exiting non-zero tells ssh there is no answer.
func RunAskpassRelay(sock, prompt string) int { return runAskpassRelay(sock, prompt, os.Stdout) }

func runAskpassRelay(sock, prompt string, out io.Writer) int {
	c, err := net.Dial("unix", sock)
	if err != nil {
		return 1
	}
	defer c.Close()
	if _, err := io.WriteString(c, prompt); err != nil {
		return 1
	}
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

// askpassRequest is one question ssh asked.
type askpassRequest struct {
	side   int
	gen    int
	prompt string
	conn   net.Conn
}

// isConfirm is the host key question, answered yes or not at all.
func (r *askpassRequest) isConfirm() bool {
	return strings.Contains(strings.ToLower(r.prompt), "(yes/no")
}

func (r *askpassRequest) answer(text string) {
	_, _ = io.WriteString(r.conn, text+"\n")
	_ = r.conn.Close()
}

func (r *askpassRequest) refuse() { _ = r.conn.Close() }

type askpassRequestMsg struct{ req *askpassRequest }

// askpassServer listens for one dial's questions.
type askpassServer struct {
	side int
	gen  int
	dir  string
	ln   net.Listener
	reqs chan *askpassRequest
	done chan struct{}
	once sync.Once
}

func newAskpassServer(side, gen int) (*askpassServer, error) {
	dir, err := os.MkdirTemp("", "sshu-askpass-")
	if err != nil {
		return nil, err
	}
	ln, err := net.Listen("unix", filepath.Join(dir, "s"))
	if err != nil {
		_ = os.RemoveAll(dir)
		return nil, err
	}
	s := &askpassServer{side: side, gen: gen, dir: dir, ln: ln,
		reqs: make(chan *askpassRequest, 8), done: make(chan struct{})}
	go s.accept()
	return s, nil
}

func (s *askpassServer) path() string { return s.ln.Addr().String() }

func (s *askpassServer) accept() {
	for {
		c, err := s.ln.Accept()
		if err != nil {
			return
		}
		go func(c net.Conn) {
			b, err := io.ReadAll(c)
			if err != nil {
				_ = c.Close()
				return
			}
			select {
			case s.reqs <- &askpassRequest{side: s.side, gen: s.gen, prompt: string(b), conn: c}:
			case <-s.done:
				_ = c.Close()
			}
		}(c)
	}
}

// next waits for the next question.
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

// close stops listening and refuses what is still waiting.
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
