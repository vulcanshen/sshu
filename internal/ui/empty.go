package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Every panel with nothing in it says the same two things in the same shape:
// what is not here, and — where there is something to do about it — what to do.
// Both centred, vertically and horizontally.
//
// The panels were each inventing their own: "(empty)" and "(none)" pinned to the
// top-left, one centred sentence with no title, a title and a sentence. A left-
// aligned "(empty)" also reads as a list whose first ENTRY says "(empty)", which
// is the one thing an empty state must not look like.
//
// And the hint WRAPS. It did not, and centerLine truncates what does not fit —
// so on a 26-column panel the sentence telling a stuck user what to press was
// cut off, which is precisely the width at which they needed it.

// hintWord is one word of a hint and whether it is a key.
//
// The styling travels with the word rather than being applied to a finished
// string, because the sentence has to survive being wrapped: a key that lands at
// the start of the second line is still a key.
type hintWord struct {
	text string
	key  bool
}

// emptyHint splits a sentence into words, marking the ones named as keys.
func emptyHint(text string, keys ...string) []hintWord {
	isKey := make(map[string]bool, len(keys))
	for _, k := range keys {
		isKey[k] = true
	}
	out := make([]hintWord, 0, 8)
	for _, w := range strings.Fields(text) {
		out = append(out, hintWord{text: w, key: isKey[w]})
	}
	return out
}

// wrapHint greedily breaks words to at most w cells per line. A single word
// longer than the line gets its own line and is left to the renderer to clip —
// there is nowhere better to put it.
func wrapHint(words []hintWord, w int) [][]hintWord {
	if w <= 0 || len(words) == 0 {
		return nil
	}
	var out [][]hintWord
	line, used := []hintWord{}, 0
	for _, word := range words {
		n := dispW(word.text)
		switch {
		case len(line) == 0:
			line, used = []hintWord{word}, n
		case used+1+n <= w:
			line, used = append(line, word), used+1+n
		default:
			out = append(out, line)
			line, used = []hintWord{word}, n
		}
	}
	return append(out, line)
}

// renderHint draws one wrapped line, centred, keys lit.
func renderHint(line []hintWord, w int) string {
	dim := lipgloss.NewStyle().Foreground(dimColor)
	key := lipgloss.NewStyle().Foreground(handColor)

	plain := make([]string, len(line))
	styled := make([]string, len(line))
	for i, word := range line {
		plain[i] = word.text
		if word.key {
			styled[i] = key.Render(word.text)
		} else {
			styled[i] = dim.Render(word.text)
		}
	}
	return centerLine(w, strings.Join(plain, " "), strings.Join(styled, " "))
}

// emptyBody is the one shape. hint may be nil, for a state with nothing to do
// about it — an empty directory is a fact, not a prompt.
//
// The whole block is centred vertically, and it gives up its parts in order when
// the panel is too short for all of them: the blank first, then hint lines from
// the bottom. The fact is the last thing standing, because a panel that says
// nothing at all is the state this exists to prevent.
func emptyBody(innerW, innerH int, fact string, hint []hintWord) []string {
	dim := lipgloss.NewStyle().Foreground(dimColor)
	factLine := centerLine(innerW, fact, dim.Render(fact))

	lines := wrapHint(hint, innerW-2)
	for len(lines) > 0 && 2+len(lines) > innerH {
		lines = lines[:len(lines)-1]
	}

	body := []string{factLine}
	if len(lines) > 0 && innerH >= 2+len(lines) {
		body = append(body, spaces(innerW))
	}
	for _, l := range lines {
		body = append(body, renderHint(l, innerW))
	}

	out := make([]string, 0, max(0, innerH))
	for i := 0; i < max(0, (innerH-len(body))/2); i++ {
		out = append(out, spaces(innerW))
	}
	return append(out, body...)
}
