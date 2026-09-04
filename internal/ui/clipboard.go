package ui

import (
	"errors"
	"os/exec"
	"runtime"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// The clipboard sshu writes to is the LOCAL one — the machine the user is
// sitting at, running sshu. That is the whole point of copying out of a remote
// panel: the text has to end up somewhere a local editor can paste it.
//
// It goes through the platform's own tool rather than OSC 52. OSC 52 would also
// survive sshu itself being run over ssh, but it depends on the outer terminal
// allowing it — Terminal.app never has, iTerm2 asks first — and it fails
// silently, which is the one thing a copy must not do.

var errNoClipboard = errors.New("no clipboard tool found")

// clipboardTools is what sshu will hand the text to, first one that exists.
// macOS has exactly one; elsewhere the three cover Wayland, X11 with xclip and
// X11 with xsel, which is every desktop anyone is likely to be on.
func clipboardTools() [][]string {
	if runtime.GOOS == "darwin" {
		return [][]string{{"pbcopy"}}
	}
	return [][]string{
		{"wl-copy"},
		{"xclip", "-selection", "clipboard"},
		{"xsel", "--clipboard", "--input"},
	}
}

// putClipboard is a variable so a test can watch what would have been copied
// without a clipboard — and without the test having to own the developer's real
// one, which it would then be pasting into for the rest of the day.
var putClipboard = execClipboard

func execClipboard(text string) error {
	for _, argv := range clipboardTools() {
		if _, err := exec.LookPath(argv[0]); err != nil {
			continue
		}
		cmd := exec.Command(argv[0], argv[1:]...)
		cmd.Stdin = strings.NewReader(text)
		return cmd.Run()
	}
	return errNoClipboard
}

// clipboardDoneMsg reports what happened, because a copy that silently did
// nothing looks exactly like a copy that worked until the paste comes up empty.
type clipboardDoneMsg struct {
	lines int
	err   error
}

func copyToClipboard(text string) tea.Cmd {
	n := strings.Count(text, "\n") + 1
	return func() tea.Msg {
		return clipboardDoneMsg{lines: n, err: putClipboard(text)}
	}
}

// clipboardFailure says what to do about it, not just that it happened. A
// missing tool is the one failure the user can fix, so it names the packages.
func clipboardFailure(err error) string {
	if errors.Is(err, errNoClipboard) {
		if runtime.GOOS == "darwin" {
			return "copy failed: pbcopy not found"
		}
		return "copy failed: install wl-clipboard, xclip or xsel"
	}
	return "copy failed: " + err.Error()
}
