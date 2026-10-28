package tui

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	terminu "github.com/vulcanshen/terminu-go"

	"github.com/vulcanshen/sshu/v3/internal/store"
)

// harness runs sshu without a terminal: keys in, the frame out as text.
type harness struct {
	t       *testing.T
	s       *sshu
	quit    bool
	pending []chan tea.Msg // commands still running when the harness moved on
}

// testData is a Data whose savers keep what they are given, in memory.
func testData(hosts []store.Host, creds []store.Credential) *Data {
	d := &Data{Hosts: hosts, Creds: creds, Config: store.DefaultConfig(),
		Now: func() time.Time { return time.Date(2026, 10, 18, 9, 30, 0, 0, time.UTC) }}
	d.SaveHosts = func([]store.Host) error { return nil }
	d.SaveCreds = func([]store.Credential) error { return nil }
	d.SaveSSHConfig = func(f store.SSHConfigFile) (store.SSHConfigFile, error) { return f, nil }
	d.SaveKnown = func(f store.KnownHostsFile) (store.KnownHostsFile, error) { return f, nil }
	return d
}

func newHarness(t *testing.T, w, h int, d *Data) *harness {
	t.Helper()
	terminu.SetIconWidth(1, 1)
	hs := &harness{t: t, s: New(d)}
	hs.run(hs.s.a.Init())
	hs.send(tea.WindowSizeMsg{Width: w, Height: h})
	return hs
}

func (hs *harness) send(msg tea.Msg) {
	_, cmd := hs.s.a.Update(msg)
	hs.run(cmd)
}

// longTimer is how long a command may take before the harness stops waiting
// for it: an animation frame is 32ms, a toast's time seconds. What is not
// waited for does not happen, so a toast stays to be read.
const longTimer = 200 * time.Millisecond

func (hs *harness) run(cmd tea.Cmd) {
	deadline := time.Now().Add(3 * time.Second)
	queue := []tea.Cmd{cmd}
	for len(queue) > 0 && time.Now().Before(deadline) && !hs.quit {
		c := queue[0]
		queue = queue[1:]
		if c == nil {
			continue
		}
		done := make(chan tea.Msg, 1)
		go func() { done <- c() }()
		select {
		case msg := <-done:
			queue = append(queue, hs.handle(msg)...)
		case <-time.After(longTimer):
			// Kept: a PTY's next output, a program ending. settle takes it.
			hs.pending = append(hs.pending, done)
		}
	}
}

// handle gives one message to the program and returns what it sets off.
func (hs *harness) handle(msg tea.Msg) []tea.Cmd {
	switch msg := msg.(type) {
	case nil:
	case tea.BatchMsg:
		return msg
	case tea.QuitMsg:
		hs.quit = true
	default:
		if seq, ok := sequenceCmds(msg); ok {
			for _, c := range seq {
				hs.run(c)
			}
			return nil
		}
		_, next := hs.s.a.Update(msg)
		return []tea.Cmd{next}
	}
	return nil
}

// settle delivers the messages that arrived late — a PTY's output, the end
// of a program — until cond holds or d has passed, and reports whether it
// held.
func (hs *harness) settle(cond func() bool, d time.Duration) bool {
	deadline := time.Now().Add(d)
	for {
		if cond() {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		kept := hs.pending[:0]
		var ready []tea.Msg
		for _, ch := range hs.pending {
			select {
			case msg := <-ch:
				ready = append(ready, msg)
			default:
				kept = append(kept, ch)
			}
		}
		hs.pending = kept
		for _, msg := range ready {
			for _, c := range hs.handle(msg) {
				hs.run(c)
			}
		}
		if len(ready) == 0 {
			time.Sleep(10 * time.Millisecond)
		}
	}
}

// keys presses each key: a name as terminu.KeyName writes it ("Enter",
// "Esc", "Space", "Tab", "←", "Alt-Esc"), or text to type.
func (hs *harness) keys(keys ...string) {
	for _, k := range keys {
		hs.send(keyMsg(k))
	}
}

func keyMsg(k string) tea.KeyMsg {
	named := map[string]tea.KeyType{
		"Enter": tea.KeyEnter, "Esc": tea.KeyEsc, "Tab": tea.KeyTab, "Shift-Tab": tea.KeyShiftTab,
		"Space": tea.KeySpace, "Backspace": tea.KeyBackspace, "Delete": tea.KeyDelete,
		"←": tea.KeyLeft, "→": tea.KeyRight, "↑": tea.KeyUp, "↓": tea.KeyDown,
		"Home": tea.KeyHome, "End": tea.KeyEnd, "Ctrl-C": tea.KeyCtrlC, "PgUp": tea.KeyPgUp, "PgDn": tea.KeyPgDown,
	}
	alt := false
	if rest, ok := strings.CutPrefix(k, "Alt-"); ok {
		alt, k = true, rest
	}
	if t, ok := named[k]; ok {
		if t == tea.KeySpace {
			return tea.KeyMsg{Type: t, Runes: []rune{' '}, Alt: alt}
		}
		return tea.KeyMsg{Type: t, Alt: alt}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k), Alt: alt}
}

func (hs *harness) frame() []string {
	var out []string
	for _, l := range hs.s.a.Frame() {
		out = append(out, terminu.StripStyles(l))
	}
	return out
}

func (hs *harness) text() string { return strings.Join(hs.frame(), "\n") }

func (hs *harness) contains(s string) {
	hs.t.Helper()
	if !strings.Contains(hs.text(), s) {
		hs.t.Fatalf("the screen does not show %q:\n%s", s, hs.text())
	}
}

func (hs *harness) lacks(s string) {
	hs.t.Helper()
	if strings.Contains(hs.text(), s) {
		hs.t.Fatalf("the screen shows %q:\n%s", s, hs.text())
	}
}

// fullWidth fails unless every row of the frame is exactly w cells (tdp L4).
func (hs *harness) fullWidth(w int, what string) {
	hs.t.Helper()
	for i, l := range hs.s.a.Frame() {
		if got := terminu.Width(l); got != w {
			hs.t.Fatalf("%s: row %d is %d cells, not %d:\n%s", what, i, got, w, hs.text())
		}
	}
}

func sampleHosts() []store.Host {
	return []store.Host{
		{Name: "prod-web-01", Host: "10.0.3.14", Port: 22, User: "deploy", Auth: store.AuthPrivateKey,
			IdentityFile: "~/.ssh/id_ed25519", Tags: []string{"prod", "web"}},
		{Name: "db-replica", Host: "db.internal", Port: 2222, User: "postgres", Auth: store.AuthPassword, Password: "s3cret"},
		{Name: "jump", Host: "jump", Auth: store.AuthSSHConfig},
		{Name: "staging", Host: "staging.example", Port: 22, Auth: store.AuthCredential, Credential: "ops"},
	}
}

func sampleCreds() []store.Credential {
	return []store.Credential{{Name: "ops", User: "ops", Auth: store.AuthPrivateKey, IdentityFile: "~/.ssh/ops"}}
}

// sequenceCmds opens a tea.Sequence, whose message type Bubble Tea keeps to
// itself: its commands run one after another, as the program runs them.
func sequenceCmds(msg tea.Msg) ([]tea.Cmd, bool) {
	v := reflect.ValueOf(msg)
	if v.Kind() != reflect.Slice || v.Type().String() != "tea.sequenceMsg" {
		return nil, false
	}
	out := make([]tea.Cmd, v.Len())
	for i := range out {
		out[i], _ = v.Index(i).Interface().(tea.Cmd)
	}
	return out, true
}

// fakeSSH points sshu at a stand-in for ssh: script runs with ssh's
// arguments, $LOG set to the -E file.
func fakeSSH(t *testing.T, script string) {
	t.Helper()
	p := filepath.Join(t.TempDir(), "ssh")
	body := "#!/bin/sh\nLOG=\nwhile [ $# -gt 0 ]; do\n  if [ \"$1\" = -E ]; then LOG=$2; fi\n  shift\ndone\n" + script + "\n"
	if err := os.WriteFile(p, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	old := sshBinary
	sshBinary = p
	t.Cleanup(func() { sshBinary = old })
}

func (hs *harness) shows(s string) bool { return strings.Contains(hs.text(), s) }
