package ui

import (
	"fmt"
	"strconv"
	"strings"
)

// With a popup open, everything below the topmost one is drawn dim (tdp F8):
// the base screen and every popup under the top. All popups are one width
// (tdp F7), so the upper ones cover the lower ones' side borders and the frames
// no longer tell the layers apart — brightness is what is left, and it is the
// z-axis tdp D2 already reads it as: only the layer being used is lit.
//
// Dimming FADES every colour toward the canvas, foreground and background, and
// leaves the shapes alone (tdp F8, D2). An earlier version stripped the colour
// and redrew the text in one grey (§11.62); that took apart everything drawn
// with a background — the tab row's powerline chip, a panel's [N] chip, every
// cursor bar, the selection, a remote vim's status line — and left grey
// letters where they had been (§11.64). Fading keeps each colour itself, only
// darker: a layer border stays its layer, a warning stays a warning.
//
// The arithmetic is filu's (internal/ui/dim.go, the family's reference), with
// the rule tdp v0.1.12 added: a fade never lightens.

// dimKeep is how much of a colour survives the dim; the rest is the canvas.
const dimKeep = 0.45

// dimBase is what a dimmed colour fades toward: the canvas sshu draws on.
var dimBase = hexRGB(baseHex)

// dimText is the dimmed form of text that sets no colour of its own (the
// terminal's default foreground, taken as the theme's text).
var dimText = dimRGB(hexRGB(string(textColor)))

// dimRGB fades c toward dimBase. A channel darker than the canvas would come
// out LIGHTER — pure black fades up toward the base — so each channel keeps
// the smaller of the two (tdp D2): dimming never lightens.
func dimRGB(c [3]int) [3]int {
	var out [3]int
	for i := range c {
		out[i] = min(c[i], int(float64(c[i])*dimKeep+float64(dimBase[i])*(1-dimKeep)+0.5))
	}
	return out
}

// sgrRGB writes one colour as 24-bit SGR. The family requires a truecolor
// terminal (tdp D6), so a dimmed screen is always written in 24-bit, whatever
// depth the colours it came in with.
func sgrRGB(fg bool, c [3]int) string {
	lead := "48"
	if fg {
		lead = "38"
	}
	return fmt.Sprintf("%s;2;%d;%d;%d", lead, c[0], c[1], c[2])
}

// ansi16 is the xterm palette for the 16 basic colours. A remote program's
// 16 colours are the user's terminal palette, which sshu cannot read; tdp D2
// converts them through xterm's, so a custom palette dims to xterm's hues.
var ansi16 = [16][3]int{
	{0, 0, 0}, {205, 0, 0}, {0, 205, 0}, {205, 205, 0}, {0, 0, 238}, {205, 0, 205}, {0, 205, 205}, {229, 229, 229},
	{127, 127, 127}, {255, 0, 0}, {0, 255, 0}, {255, 255, 0}, {92, 92, 255}, {255, 0, 255}, {0, 255, 255}, {255, 255, 255},
}

// xterm256 converts a 256-colour index to RGB.
func xterm256(n int) [3]int {
	switch {
	case n < 16:
		return ansi16[n]
	case n < 232:
		n -= 16
		step := func(v int) int {
			if v == 0 {
				return 0
			}
			return 55 + v*40
		}
		return [3]int{step(n / 36), step(n / 6 % 6), step(n % 6)}
	default:
		v := 8 + (n-232)*10
		return [3]int{v, v, v}
	}
}

// dimANSI redraws an already-styled screen dimmed (tdp F8): every foreground
// and background colour in its SGR sequences is faded toward the base, and
// text with no colour of its own gets the dimmed default. Everything else —
// bold, reverse, cursor movement, the text itself, every other escape (the
// nest announcement included) — is left as it is, so no cell moves.
func dimANSI(s string) string {
	var b strings.Builder
	b.Grow(len(s) + len(s)/4)
	lineStart := "\x1b[" + sgrRGB(true, dimText) + "m"
	b.WriteString(lineStart)
	for i := 0; i < len(s); {
		switch {
		case s[i] == '\n':
			b.WriteByte('\n')
			b.WriteString(lineStart)
			i++
		case strings.HasPrefix(s[i:], "\x1b["):
			end := i + 2
			for end < len(s) && (s[end] < 0x40 || s[end] > 0x7e) {
				end++
			}
			if end >= len(s) {
				b.WriteString(s[i:])
				return b.String()
			}
			if s[end] == 'm' {
				b.WriteString("\x1b[" + dimSGR(s[i+2:end]) + "m")
			} else {
				b.WriteString(s[i : end+1])
			}
			i = end + 1
		default:
			b.WriteByte(s[i])
			i++
		}
	}
	return b.String()
}

// dimSGR rewrites one SGR parameter list with its colours dimmed. A reset (or a
// "default foreground") is followed by the dimmed default so the text after it
// stays dim.
func dimSGR(params string) string {
	ps := strings.Split(params, ";")
	var out []string
	needText := false
	for i := 0; i < len(ps); i++ {
		p := ps[i]
		n, err := strconv.Atoi(p)
		if p == "" {
			n, err = 0, nil
		}
		if err != nil {
			out = append(out, p)
			continue
		}
		switch {
		case n == 0:
			out = append(out, "0")
			needText = true
		case n == 39:
			needText = true
		case (n == 38 || n == 48) && i+1 < len(ps):
			fg := n == 38
			var c [3]int
			switch ps[i+1] {
			case "2":
				if i+4 >= len(ps) {
					out = append(out, ps[i:]...)
					i = len(ps)
					continue
				}
				for k := 0; k < 3; k++ {
					c[k], _ = strconv.Atoi(ps[i+2+k])
				}
				i += 4
			case "5":
				if i+2 >= len(ps) {
					out = append(out, ps[i:]...)
					i = len(ps)
					continue
				}
				idx, _ := strconv.Atoi(ps[i+2])
				c = xterm256(idx)
				i += 2
			default:
				out = append(out, p)
				continue
			}
			out = append(out, sgrRGB(fg, dimRGB(c)))
			if fg {
				needText = false
			}
		case n >= 30 && n <= 37, n >= 90 && n <= 97:
			idx := n - 30
			if n >= 90 {
				idx = n - 90 + 8
			}
			out = append(out, sgrRGB(true, dimRGB(ansi16[idx])))
			needText = false
		case n >= 40 && n <= 47, n >= 100 && n <= 107:
			idx := n - 40
			if n >= 100 {
				idx = n - 100 + 8
			}
			out = append(out, sgrRGB(false, dimRGB(ansi16[idx])))
		default:
			out = append(out, p)
		}
	}
	if needText {
		out = append(out, sgrRGB(true, dimText))
	}
	return strings.Join(out, ";")
}

func hexRGB(h string) [3]int {
	h = strings.TrimPrefix(h, "#")
	var c [3]int
	for i := range c {
		if len(h) >= 2*i+2 {
			v, _ := strconv.ParseUint(h[2*i:2*i+2], 16, 8)
			c[i] = int(v)
		}
	}
	return c
}
