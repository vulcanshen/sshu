package tui

import (
	"errors"
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	terminu "github.com/vulcanshen/terminu-go"
)

// newViewer is every viewer sshu opens: titled by what it shows, with the
// selection mode that copies out of it (components/dialog/note).
func newViewer(title, subject, glyph string, lines []string) *terminu.Viewer {
	return &terminu.Viewer{Title: title, Subject: subject, Glyph: glyph, Lines: lines,
		Select: &terminu.Selection{Key: "v", Name: "Visual", Copy: copyText}}
}

// copyText puts text on the system clipboard, through whichever of the usual
// tools this machine has.
func copyText(text string) tea.Cmd {
	return func() tea.Msg {
		for _, c := range [][]string{{"pbcopy"}, {"wl-copy"}, {"xclip", "-selection", "clipboard"}, {"xsel", "--clipboard", "--input"}} {
			if _, err := exec.LookPath(c[0]); err != nil {
				continue
			}
			cmd := exec.Command(c[0], c[1:]...)
			cmd.Stdin = strings.NewReader(text)
			if err := cmd.Run(); err != nil {
				return copied{err: err}
			}
			return copied{n: len([]rune(text))}
		}
		return copied{err: errNoClipboard}
	}
}

var errNoClipboard = errors.New("no clipboard tool: install pbcopy, wl-copy, xclip or xsel")

type copied struct {
	n   int
	err error
}

func (s *sshu) onCopied(m copied) tea.Cmd {
	if m.err != nil {
		return s.a.Toast(terminu.Error, "Not copied: "+m.err.Error())
	}
	return s.a.Toast(terminu.Success, "Copied "+plural(m.n, "character"))
}
