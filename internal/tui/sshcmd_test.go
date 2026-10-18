package tui

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	terminu "github.com/vulcanshen/terminu-go"

	"github.com/vulcanshen/sshu/internal/store"
)

// The two ends of the relay agree: a prompt goes in one side, the answer
// comes out the other, newline-terminated the way ssh reads it.
func TestAskpassRelayCarriesThePromptOutAndTheAnswerBack(t *testing.T) {
	srv, err := newAskpassServer(0, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer srv.close()
	var out bytes.Buffer
	rc := make(chan int, 1)
	go func() { rc <- runAskpassRelay(srv.path(), "demo@host's password: ", &out) }()
	req, ok := srv.next()().(askpassRequestMsg)
	if !ok {
		t.Fatal("no request arrived")
	}
	if req.req.prompt != "demo@host's password: " || req.req.isConfirm() {
		t.Errorf("prompt arrived as %q", req.req.prompt)
	}
	req.req.answer("hunter2")
	if code := <-rc; code != 0 {
		t.Errorf("relay exit %d, want 0", code)
	}
	if got := out.String(); got != "hunter2\n" {
		t.Errorf("helper printed %q", got)
	}
}

// A refusal is zero bytes and a non-zero exit: the one reply ssh cannot
// mistake for an empty password.
func TestAskpassRefusalIsExitOneWithNothingPrinted(t *testing.T) {
	srv, err := newAskpassServer(0, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer srv.close()
	var out bytes.Buffer
	rc := make(chan int, 1)
	go func() { rc <- runAskpassRelay(srv.path(), "Password: ", &out) }()
	srv.next()().(askpassRequestMsg).req.refuse()
	if code := <-rc; code != 1 || out.Len() != 0 {
		t.Errorf("relay exit %d, printed %q", code, out.String())
	}
}

// The host key question is told apart by its own words (OpenSSH sends no
// hint for it).
func TestAskpassTellsAHostKeyQuestionFromASecret(t *testing.T) {
	hostKey := "The authenticity of host '[localhost]:2222 ([::1]:2222)' can't be established.\n" +
		"ED25519 key fingerprint is: SHA256:Z/RqdV6TKRgKtwi6JAoVEGkM6DHM3Rnlusv+OaCeDGY\n" +
		"Are you sure you want to continue connecting (yes/no/[fingerprint])? "
	for prompt, confirm := range map[string]bool{
		hostKey: true, "demo@localhost's password: ": false, "Enter passphrase for key '/k/id': ": false,
	} {
		if got := (&askpassRequest{prompt: prompt}).isConfirm(); got != confirm {
			t.Errorf("%q: confirm = %v", prompt, got)
		}
	}
}

// Closing the server refuses what was queued: no helper hangs on a question
// nobody will answer.
func TestClosingTheAskpassServerRefusesWhatIsQueued(t *testing.T) {
	srv, err := newAskpassServer(0, 1)
	if err != nil {
		t.Fatal(err)
	}
	rc := make(chan int, 1)
	go func() { rc <- runAskpassRelay(srv.path(), "Password: ", io.Discard) }()
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
		t.Fatal("the helper was left hanging")
	}
}

// ssh carries TERM and nothing else, so a sshu on the far side would see 256
// colours: the depth is handed over deliberately, and ssh told to send it.
func TestTheColourDepthIsHandedToTheFarSide(t *testing.T) {
	t.Setenv("COLORTERM", "truecolor")
	h := store.Host{Name: "h", Host: "example", Port: 22, User: "u"}
	if !strings.Contains(strings.Join(sshEnv(h, ""), " "), colorEnv+"=truecolor") {
		t.Error("the environment does not carry the depth")
	}
	if !strings.Contains(strings.Join(sshArgs(h, 5, ""), " "), "SendEnv="+colorEnv) {
		t.Error("ssh was not asked to send it")
	}
	t.Setenv("COLORTERM", "")
	if strings.Contains(strings.Join(sshEnv(h, ""), " "), colorEnv) ||
		strings.Contains(strings.Join(sshArgs(h, 5, ""), " "), "SendEnv="+colorEnv) {
		t.Error("with no depth of its own, nothing should be carried")
	}
}

// The far side takes the depth only into a gap: a real COLORTERM wins.
func TestTheFarSideAdoptsTheDepthOnlyIntoAGap(t *testing.T) {
	t.Setenv("COLORTERM", "")
	t.Setenv(colorEnv, "truecolor")
	AdoptForwarded()
	if got := os.Getenv("COLORTERM"); got != "truecolor" {
		t.Errorf("COLORTERM = %q, want the forwarded depth", got)
	}
	t.Setenv("COLORTERM", "24bit")
	AdoptForwarded()
	if got := os.Getenv("COLORTERM"); got != "24bit" {
		t.Errorf("COLORTERM = %q, want the local terminal's own", got)
	}
}

// An sshconfig host hands ssh only what it has: no port, no user, no key
// when the record leaves them to ~/.ssh/config.
func TestAnSSHConfigHostLeavesTheRestToTheFile(t *testing.T) {
	args := sshArgs(store.Host{Name: "jump", Host: "jump", Auth: store.AuthSSHConfig}, 15, "")
	for _, bad := range []string{"-p", "-i", "@"} {
		for _, a := range args {
			if strings.Contains(a, bad) && a != "SendEnv="+colorEnv {
				t.Errorf("%q in %v", bad, args)
			}
		}
	}
	if args[len(args)-1] != "jump" {
		t.Errorf("destination %q", args[len(args)-1])
	}
}

// The widths cross ssh too (tdp E5): the outer sshu sends its own, and the
// far side takes them unless the user set sshu's own variable there.
func TestTheIconWidthsAreHandedToTheFarSide(t *testing.T) {
	terminu.SetIconWidth(2, 1)
	defer terminu.SetIconWidth(1, 1)
	h := store.Host{Name: "h", Host: "example", Port: 22, User: "u"}
	env := strings.Join(sshEnv(h, ""), " ")
	if !strings.Contains(env, iconEnv+"=2") || !strings.Contains(env, powerlineEnv+"=1") {
		t.Errorf("the environment does not carry the widths: %s", env)
	}
	args := strings.Join(sshArgs(h, 5, ""), " ")
	if !strings.Contains(args, "SendEnv="+iconEnv) || !strings.Contains(args, "SendEnv="+powerlineEnv) {
		t.Errorf("ssh was not asked to send them: %s", args)
	}
	t.Setenv(iconEnv, "2")
	t.Setenv("SSHU__ICON_WIDTH", "")
	t.Setenv(powerlineEnv, "2")
	t.Setenv("SSHU__POWERLINE_WIDTH", "1")
	AdoptForwarded()
	if os.Getenv("SSHU__ICON_WIDTH") != "2" {
		t.Error("the forwarded icon width was not taken")
	}
	if os.Getenv("SSHU__POWERLINE_WIDTH") != "1" {
		t.Error("the user's own SSHU__POWERLINE_WIDTH must win")
	}
}
