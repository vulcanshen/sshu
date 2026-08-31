package ui

import (
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/formatters"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
)

// Ported from filu, unchanged in substance — the point of the u-family is that a
// file looks the same whichever tool you are looking at it through.
//
// chroma pieces resolved once: terminal16m emits truecolor ANSI (clipANSI cuts
// it safely) and catppuccin-mocha is sshu's palette. The style's background is
// stripped so highlighting is foreground-only and blends with the popup, rather
// than laying a #1e1e2e band over it.
var (
	hlStyle     = noBackground(styles.Get("catppuccin-mocha"))
	hlFormatter = formatters.Get("terminal16m")
)

func noBackground(s *chroma.Style) *chroma.Style {
	out, err := s.Builder().Transform(func(e chroma.StyleEntry) chroma.StyleEntry {
		e.Background = 0 // Colour(0) == unset
		return e
	}).Build()
	if err != nil {
		return s
	}
	return out
}

const ansiReset = "\x1b[0m"

// highlight syntax-colours src for a file named name (used only to pick the
// lexer). ok is false when no lexer matches — the caller then shows plain text.
//
// src must already be sanitised (no raw tabs or ESC): the returned lines carry
// ANSI of their own, and sanitising afterwards would destroy it.
func highlight(name, src string) (lines []string, ok bool) {
	lexer := lexers.Match(name)
	if lexer == nil {
		return nil, false
	}
	lexer = chroma.Coalesce(lexer)
	iter, err := lexer.Tokenise(nil, src)
	if err != nil {
		return nil, false
	}
	var b strings.Builder
	if err := hlFormatter.Format(&b, hlStyle, iter); err != nil {
		return nil, false
	}
	out := strings.Split(strings.TrimSuffix(b.String(), "\n"), "\n")
	// A multi-line token opens its colour on the first line only; force a reset
	// per line so an unclosed span cannot bleed into the popup's padding.
	for i := range out {
		out[i] += ansiReset
	}
	return out, true
}
