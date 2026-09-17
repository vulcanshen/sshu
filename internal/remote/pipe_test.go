package remote

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/pkg/sftp"
)

// The test binary doubles as the far end: started with this variable set, it
// serves SFTP over its own stdin and stdout and exits when they close. That
// is what `ssh -s sftp` looks like from this side, minus the network — which
// is the part DialPipe does not touch.
const serveEnv = "SSHU_TEST_SFTP_SERVER"

func TestMain(m *testing.M) {
	if os.Getenv(serveEnv) == "1" {
		srv, err := sftp.NewServer(stdio{os.Stdin, os.Stdout})
		if err != nil {
			os.Exit(2)
		}
		_ = srv.Serve()
		os.Exit(0)
	}
	os.Exit(m.Run())
}

type stdio struct {
	io.Reader
	io.WriteCloser
}

func serverCmd(t *testing.T) *exec.Cmd {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^$")
	cmd.Env = append(os.Environ(), serveEnv+"=1")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	return cmd
}

// SFTP over two pipes is SFTP: list a directory, stat a file, close.
func TestDialPipeSpeaksSFTPOverASubprocess(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello"), 0o600)
	os.Mkdir(filepath.Join(dir, "sub"), 0o700)

	fsys, err := DialPipe("srv", serverCmd(t))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer fsys.Close()

	if fsys.Label() != "srv" {
		t.Errorf("label = %q, want the name given", fsys.Label())
	}
	entries, err := fsys.List(dir)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name)
	}
	if got := strings.Join(names, ","); got != "sub,a.txt" {
		t.Errorf("listing = %q, want directories first then names", got)
	}
	if st, err := fsys.Stat(filepath.Join(dir, "a.txt")); err != nil || st.Size != 5 {
		t.Errorf("stat a.txt = %+v, %v", st, err)
	}
}

// When the handshake fails, the error is what the subprocess said on stderr —
// that is where ssh puts "Permission denied" and the rest.
func TestDialPipeFailureCarriesWhatTheSubprocessSaid(t *testing.T) {
	cmd := exec.Command("sh", "-c", `echo "demo@h: Permission denied (publickey,password)." >&2; exit 255`)
	_, err := DialPipe("h", cmd)
	if err == nil {
		t.Fatal("a subprocess that exits before speaking SFTP is a failed dial")
	}
	if !strings.Contains(err.Error(), "Permission denied") {
		t.Errorf("error should carry stderr, got %q", err)
	}
	if !strings.HasPrefix(err.Error(), "h: ") {
		t.Errorf("error should be labelled with the host, got %q", err)
	}
}

// A subprocess that says nothing still produces an error that says what
// happened rather than an empty string.
func TestDialPipeFailureWithNothingOnStderrStillExplains(t *testing.T) {
	_, err := DialPipe("h", exec.Command("true"))
	if err == nil || strings.TrimSpace(strings.TrimPrefix(err.Error(), "h:")) == "" {
		t.Errorf("want a reason, got %q", err)
	}
}

// Close ends the subprocess — nothing keeps running once the side lets go.
func TestDialPipeCloseEndsTheSubprocess(t *testing.T) {
	cmd := serverCmd(t)
	fsys, err := DialPipe("srv", cmd)
	if err != nil {
		t.Fatal(err)
	}
	if err := fsys.Close(); err != nil {
		t.Errorf("close: %v", err)
	}
	if cmd.ProcessState == nil {
		t.Error("the subprocess should have been reaped by Close")
	}
}
