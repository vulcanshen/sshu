package ui

import (
	"bytes"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/vulcanshen/sshu/internal/store"
)

// The two ends of the relay agree: a prompt goes in one side, the answer
// comes out the other, newline-terminated the way ssh reads it.
func TestAskpassRelayCarriesThePromptOutAndTheAnswerBack(t *testing.T) {
	srv, err := newAskpassServer(sideLeft, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer srv.close()

	var out bytes.Buffer
	rc := make(chan int, 1)
	go func() { rc <- runAskpassRelay(srv.path(), "demo@host's password: ", &out) }()

	msg := srv.next()()
	req, ok := msg.(askpassRequestMsg)
	if !ok {
		t.Fatalf("want a request, got %T", msg)
	}
	if req.req.prompt != "demo@host's password: " {
		t.Errorf("prompt arrived as %q", req.req.prompt)
	}
	if req.req.isConfirm() {
		t.Error("a password prompt is not a yes/no question")
	}
	req.req.answer("hunter2")

	if code := <-rc; code != 0 {
		t.Errorf("relay exit %d, want 0", code)
	}
	if got := out.String(); got != "hunter2\n" {
		t.Errorf("helper printed %q, want the answer and a newline", got)
	}
}

// A refusal is zero bytes and a non-zero exit — the only reply ssh cannot
// mistake for an empty password.
func TestAskpassRefusalIsExitOneWithNothingPrinted(t *testing.T) {
	srv, err := newAskpassServer(sideLeft, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer srv.close()

	var out bytes.Buffer
	rc := make(chan int, 1)
	go func() { rc <- runAskpassRelay(srv.path(), "Password: ", &out) }()
	req := srv.next()().(askpassRequestMsg).req
	req.refuse()

	if code := <-rc; code != 1 {
		t.Errorf("relay exit %d, want 1", code)
	}
	if out.Len() != 0 {
		t.Errorf("a refusal must print nothing, got %q", out.String())
	}
}

// The host key question is told apart by its own words — measured against
// OpenSSH 10.3, which sends no SSH_ASKPASS_PROMPT hint for it.
func TestAskpassTellsAHostKeyQuestionFromASecret(t *testing.T) {
	hostKey := "The authenticity of host '[localhost]:2222 ([::1]:2222)' can't be established.\n" +
		"ED25519 key fingerprint is: SHA256:Z/RqdV6TKRgKtwi6JAoVEGkM6DHM3Rnlusv+OaCeDGY\n" +
		"This key is not known by any other names.\n" +
		"Are you sure you want to continue connecting (yes/no/[fingerprint])? "
	for prompt, confirm := range map[string]bool{
		hostKey:                              true,
		"demo@localhost's password: ":        false,
		"Enter passphrase for key '/k/id': ": false,
	} {
		if got := (&askpassRequest{prompt: prompt}).isConfirm(); got != confirm {
			t.Errorf("%q: confirm = %v, want %v", prompt, got, confirm)
		}
	}
}

// A socket that has been closed refuses what was still queued, so no helper
// is left hanging on a question nobody will answer.
func TestClosingTheAskpassServerRefusesWhatIsQueued(t *testing.T) {
	srv, err := newAskpassServer(sideLeft, 1)
	if err != nil {
		t.Fatal(err)
	}
	rc := make(chan int, 1)
	go func() { rc <- runAskpassRelay(srv.path(), "Password: ", io.Discard) }()
	// Wait for the question to be queued, then close without reading it.
	deadline := time.Now().Add(2 * time.Second)
	for len(srv.reqs) == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	srv.close()
	select {
	case code := <-rc:
		if code != 1 {
			t.Errorf("relay exit %d, want 1", code)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the helper is still waiting after the server closed")
	}
}

// ---------------------------------------------------------------- the popup

// sshconfigDialing puts the left side into an ssh-backed dial without an
// ssh: the socket is real, the ssh is a cancel func that counts.
func sshconfigDialing(t *testing.T) (AppModel, *askpassServer, *int) {
	t.Helper()
	m := sftpFixture(t, 100, 26)
	m.sftp.focus = panelLeftFiles
	s := &m.sftp.sides[sideLeft]
	s.fs, s.entries = nil, nil
	srv, err := newAskpassServer(sideLeft, 7)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(srv.close)
	cancels := 0
	s.dialGen, s.dialing, s.host = 7, "gw", "gw"
	s.dialSince = time.Now()
	s.askpass, s.dialCancel = srv, func() { cancels++ }
	return m, srv, &cancels
}

// question builds a request whose far end is a pipe the test can read.
func question(sd side, gen int, prompt string) (*askpassRequest, net.Conn) {
	ours, theirs := net.Pipe()
	return &askpassRequest{sd: sd, gen: gen, prompt: prompt, conn: ours}, theirs
}

func readReply(t *testing.T, c net.Conn) string {
	t.Helper()
	c.SetReadDeadline(time.Now().Add(2 * time.Second))
	b, _ := io.ReadAll(c)
	return string(b)
}

// A password question puts up a masked box titled with the host; Enter sends
// the line and closes it.
func TestAPasswordQuestionIsAskedMaskedAndAnswered(t *testing.T) {
	m, _, _ := sshconfigDialing(t)
	req, far := question(sideLeft, 7, "demo@gw's password: ")
	got := make(chan string, 1)
	go func() { got <- readReply(t, far) }()

	next, _ := m.askpassArrived(req)
	m = settle(next.(AppModel))
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "demo@gw's password:") || !strings.Contains(view, " gw ") {
		t.Fatalf("the question and the host should be on screen:\n%s", view)
	}
	m = pressA(m, "h", "u", "n", "t", "e", "r", "2")
	view = ansi.Strip(m.View())
	if strings.Contains(view, "hunter2") {
		t.Errorf("the password must not be echoed:\n%s", view)
	}
	if !strings.Contains(view, "•••••••") {
		t.Errorf("the box should show one bullet per rune:\n%s", view)
	}
	m = pressA(m, "enter")
	if reply := <-got; reply != "hunter2\n" {
		t.Errorf("ssh would have received %q", reply)
	}
	if m.askpassUI.anim.owns() {
		t.Error("answering should close the box")
	}
}

// Space is a character in the box — a password can contain one — so the
// entry key must not close the float it is being typed into (§4.5).
func TestSpaceInsideThePasswordBoxIsACharacter(t *testing.T) {
	m, _, _ := sshconfigDialing(t)
	req, far := question(sideLeft, 7, "Password: ")
	got := make(chan string, 1)
	go func() { got <- readReply(t, far) }()
	next, _ := m.askpassArrived(req)
	m = pressA(settle(next.(AppModel)), "a", " ", "b", "enter")
	if reply := <-got; reply != "a b\n" {
		t.Errorf("reply %q, want the space kept", reply)
	}
}

// Esc is not "no answer", it is "no connection": the ssh is killed and the
// box goes away. Otherwise ssh asks twice more and a cancel becomes three.
func TestEscOnTheQuestionCancelsTheDial(t *testing.T) {
	m, _, cancels := sshconfigDialing(t)
	req, far := question(sideLeft, 7, "Password: ")
	got := make(chan string, 1)
	go func() { got <- readReply(t, far) }()
	next, _ := m.askpassArrived(req)
	m = pressA(settle(next.(AppModel)), "esc")

	if reply := <-got; reply != "" {
		t.Errorf("a cancel must send nothing, sent %q", reply)
	}
	if *cancels != 1 {
		t.Errorf("the ssh should have been cancelled once, got %d", *cancels)
	}
	if !m.sftp.sides[sideLeft].dialCancelled {
		t.Error("the side should remember that the user cancelled")
	}
	if m.askpassUI.anim.owns() {
		t.Error("the box should be gone")
	}
}

// The host key question is a yes/no: typing does nothing, Enter says yes.
func TestAHostKeyQuestionTakesYesAndNothingElse(t *testing.T) {
	m, _, _ := sshconfigDialing(t)
	req, far := question(sideLeft, 7, "The authenticity of host 'gw' can't be established.\n"+
		"ED25519 key fingerprint is: SHA256:abc.\n"+
		"Are you sure you want to continue connecting (yes/no/[fingerprint])? ")
	got := make(chan string, 1)
	go func() { got <- readReply(t, far) }()
	next, _ := m.askpassArrived(req)
	m = settle(next.(AppModel))
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "SHA256:abc") || !strings.Contains(view, "Enter yes") {
		t.Fatalf("the fingerprint and the verb should be on screen:\n%s", view)
	}
	m = pressA(m, "n", "o", "enter")
	if reply := <-got; reply != "yes\n" {
		t.Errorf("reply %q, want yes — the keys typed are not an answer", reply)
	}
}

// Two questions at once queue: the second waits for the first to be
// answered, then comes up on its own.
func TestQuestionsAreAskedOneAtATime(t *testing.T) {
	m, _, _ := sshconfigDialing(t)
	first, farA := question(sideLeft, 7, "first: ")
	second, farB := question(sideLeft, 7, "second: ")
	gotA, gotB := make(chan string, 1), make(chan string, 1)
	go func() { gotA <- readReply(t, farA) }()
	go func() { gotB <- readReply(t, farB) }()

	next, _ := m.askpassArrived(first)
	m = settle(next.(AppModel))
	next, _ = m.askpassArrived(second)
	m = settle(next.(AppModel))
	if !strings.Contains(ansi.Strip(m.View()), "first:") {
		t.Fatal("the first question should be the one on screen")
	}
	if len(m.askpassQueue) != 1 {
		t.Fatalf("the second should be queued, queue = %d", len(m.askpassQueue))
	}
	m = pressA(m, "a", "enter")
	if r := <-gotA; r != "a\n" {
		t.Errorf("first reply %q", r)
	}
	m = settle(m)
	if !strings.Contains(ansi.Strip(m.View()), "second:") {
		t.Fatalf("the second question should come up next:\n%s", ansi.Strip(m.View()))
	}
	m = pressA(m, "b", "enter")
	if r := <-gotB; r != "b\n" {
		t.Errorf("second reply %q", r)
	}
}

// A question from a dial the user already left is refused, not asked.
func TestAQuestionFromAnAbandonedDialIsRefused(t *testing.T) {
	m, _, _ := sshconfigDialing(t)
	req, far := question(sideLeft, 6, "Password: ") // gen 6: superseded by 7
	got := make(chan string, 1)
	go func() { got <- readReply(t, far) }()
	next, _ := m.askpassArrived(req)
	m = next.(AppModel)
	if r := <-got; r != "" {
		t.Errorf("an abandoned dial's question got an answer: %q", r)
	}
	if m.askpassUI.isActive() {
		t.Error("nothing should have been asked")
	}
}

// When the dial ends — however it ends — the question it was asking is
// moot, and the box goes away without an answer.
func TestTheDialEndingTakesItsQuestionDown(t *testing.T) {
	m, _, _ := sshconfigDialing(t)
	req, far := question(sideLeft, 7, "Password: ")
	got := make(chan string, 1)
	go func() { got <- readReply(t, far) }()
	next, _ := m.askpassArrived(req)
	m = settle(next.(AppModel))

	next, _ = m.Update(sftpConnectedMsg{sd: sideLeft, gen: 7, err: errTestDial})
	m = next.(AppModel)
	if r := <-got; r != "" {
		t.Errorf("the box was answered by the dial ending: %q", r)
	}
	if m.askpassUI.anim.owns() {
		t.Error("the box should be gone with the dial")
	}
	if m.sftp.sides[sideLeft].askpass != nil {
		t.Error("the socket should be closed with the dial")
	}
}

// A cancelled dial is recorded as the user's decision, not as an error.
func TestACancelledDialIsNotAnError(t *testing.T) {
	m, _, _ := sshconfigDialing(t)
	m.sftp.sides[sideLeft].dialCancelled = true
	before := len(m.errors.entries)
	next, _ := m.Update(sftpConnectedMsg{sd: sideLeft, gen: 7, err: errTestDial})
	m = next.(AppModel)
	if len(m.errors.entries) != before {
		t.Error("a connection the user cancelled is not something that went wrong")
	}
	if got := m.sftp.sides[sideLeft].err; got != "cancelled" {
		t.Errorf("the side should say cancelled, says %q", got)
	}
}

// The ssh-backed dial is only for the host that asked for it.
func TestOnlyAnSSHConfigHostDialsThroughSSH(t *testing.T) {
	m := sftpFixture(t, 100, 26)
	s := &m.sftp.sides[sideLeft]
	m.sftp.startDial(sideLeft, store.Host{Name: "p", Host: "h", Port: 22, User: "u", Auth: store.AuthPassword})
	if s.askpass != nil || s.dialCancel != nil {
		t.Error("a password host keeps the in-process dial and needs no socket")
	}
	s.endDial()
}

var _ tea.Model = AppModel{}
