package tui

import (
	"context"
	"os"
	"os/exec"
	"strconv"
	"syscall"
	"time"

	terminu "github.com/vulcanshen/terminu-go"

	"github.com/vulcanshen/sshu/internal/store"
)

// sshu runs the real ssh, so ~/.ssh/config, ProxyJump, the agent and host
// keys work as they do for the user's own ssh.

// sshBinary is what sshu runs; tests point it at a stand-in.
var sshBinary = "ssh"

// sshArgs is the command line of a session on h. ssh does the timing out:
// it says why it gave up, and a killed ssh says nothing. logFile, when set,
// takes ssh's own messages (-E), so a session that fails can say what ssh
// said.
func sshArgs(h store.Host, timeoutSecs int, logFile string) []string {
	args := append(portArgs(h), "-o", "ConnectTimeout="+strconv.Itoa(timeoutSecs))
	// The colour depth rides an LC_ variable, which sshd accepts by default.
	if os.Getenv("COLORTERM") != "" {
		args = append(args, "-o", "SendEnv="+colorEnv)
	}
	args = append(args, "-o", "SendEnv="+iconEnv, "-o", "SendEnv="+powerlineEnv)
	if logFile != "" {
		args = append(args, "-E", logFile)
	}
	args = append(args, identityArgs(h)...)
	return append([]string{sshBinary}, append(args, destination(h))...)
}

// portArgs, identityArgs and destination are what the SSH screen and the
// file transfer share. An sshconfig host sends only what it has: an empty
// field is the file's to fill.
func portArgs(h store.Host) []string {
	if h.Port <= 0 {
		return nil
	}
	return []string{"-p", strconv.Itoa(h.Port)}
}

func identityArgs(h store.Host) []string {
	if h.Auth != store.AuthPrivateKey || h.IdentityFile == "" {
		return nil
	}
	// With an explicit key, ssh must not try every agent identity first: a
	// wrong key then fails as "too many authentication failures".
	return []string{"-i", store.ExpandTilde(h.IdentityFile), "-o", "IdentitiesOnly=yes"}
}

func destination(h store.Host) string {
	if h.User == "" {
		return h.Host
	}
	return h.User + "@" + h.Host
}

// sshEnv is what a session adds to the environment. A password host has ssh
// ask sshu itself for the password (SSH_ASKPASS, forced even with a terminal):
// the helper reads hosts.yaml, so the password never sits in an environment.
func sshEnv(h store.Host, self string) []string {
	env := []string{"LC_ALL=" + envOr("LC_ALL", "en_US.UTF-8")}
	if v := os.Getenv("COLORTERM"); v != "" {
		env = append(env, colorEnv+"="+v)
	}
	env = append(env, iconEnv+"="+strconv.Itoa(terminu.IconWidth()), powerlineEnv+"="+strconv.Itoa(terminu.PowerlineWidth()))
	if h.Auth == store.AuthPassword && h.Password != "" && self != "" {
		env = append(env, "SSH_ASKPASS="+self, "SSH_ASKPASS_REQUIRE=force", askpassHostEnv+"="+h.Name)
	}
	return env
}

// sftpCmd is an sshconfig host's file transfer: `ssh -s <host> sftp`, as
// OpenSSH's sftp runs it. With no terminal and the askpass helper forced,
// whatever ssh asks comes to sshu over sock. Cancelling ctx ends the whole
// process group, a ProxyJump's child with it.
func sftpCmd(ctx context.Context, h store.Host, self, sock string, timeoutSecs int) *exec.Cmd {
	args := append(portArgs(h),
		"-o", "ConnectTimeout="+strconv.Itoa(timeoutSecs),
		"-o", "ClearAllForwardings=yes",
		"-o", "PermitLocalCommand=no",
		"-o", "ForwardX11=no")
	args = append(args, identityArgs(h)...)
	args = append(args, "-s", destination(h), "sftp")
	cmd := exec.CommandContext(ctx, sshBinary, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM) }
	cmd.WaitDelay = 2 * time.Second
	cmd.Env = append(os.Environ(), "SSH_ASKPASS="+self, "SSH_ASKPASS_REQUIRE=force", askpassSockEnv+"="+sock)
	return cmd
}

// colorEnv carries the colour depth across ssh: sshd takes LC_* by default,
// and ssh forwards nothing else of the kind. iconEnv and powerlineEnv carry
// how many cells an icon and a powerline symbol take here (tdp E5): a sshu
// on the far side runs in this sshu's terminal emulator, whose answer to its
// probe counts both as one, so it has to be told — and TERMINU__ variables do
// not cross ssh.
const (
	colorEnv     = "LC_SSHU_COLORTERM"
	iconEnv      = "LC_SSHU_ICON_WIDTH"
	powerlineEnv = "LC_SSHU_POWERLINE_WIDTH"
)

// AdoptForwarded is called before anything is drawn: a sshu started by
// another one over ssh finds its colour depth and the widths there. A real
// COLORTERM wins, and so does a width the user set for sshu itself.
func AdoptForwarded() {
	if v := os.Getenv(colorEnv); v != "" && os.Getenv("COLORTERM") == "" {
		_ = os.Setenv("COLORTERM", v)
	}
	for _, w := range [][2]string{{iconEnv, "SSHU__ICON_WIDTH"}, {powerlineEnv, "SSHU__POWERLINE_WIDTH"}} {
		if v := os.Getenv(w[0]); (v == "1" || v == "2") && os.Getenv(w[1]) == "" {
			_ = os.Setenv(w[1], v)
		}
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// askpassHostEnv names the host whose password sshu prints when ssh runs it
// as its askpass helper.
const askpassHostEnv = "SSHU__ASKPASS_HOST"

// AskpassHost is the host of an askpass run, "" for a normal one.
func AskpassHost() string { return os.Getenv(askpassHostEnv) }

// selfPath is the sshu binary, its own askpass helper. Without it a password
// host asks in the terminal instead.
func selfPath() string {
	p, err := os.Executable()
	if err != nil {
		return ""
	}
	return p
}
