package ui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// With a popup open, everything below the topmost one is drawn dim (tdp F8):
// the base screen and every popup under the top. All popups are one width
// (tdp F7), so the upper ones cover the lower ones' side borders and the frames
// no longer tell the layers apart — brightness is what is left, and it is the
// z-axis tdp D2 already reads it as: only the layer being used is lit.
//
// That includes what is still moving underneath — a remote session's output,
// the transfer bar, a warning colour (tdp T2's exception): the attention is on
// the popup, and a lit stream below it competes for it.

// dimScreen redraws s in the dim colour, line for line. The text is kept —
// what is underneath is still there to be glanced at — and so is every line's
// width, because stripping SGR does not move a single cell.
func dimScreen(s string) string {
	dim := lipgloss.NewStyle().Foreground(dimColor)
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		if plain := ansi.Strip(l); plain != "" {
			lines[i] = dim.Render(plain)
		}
	}
	return strings.Join(lines, "\n")
}

// dimPopup redraws a rendered popup box as a layer below the top: its content
// in the dim colour, its frame in a dimmed version of its own layer colour —
// dark, but still reading as the layer it is (tdp F8, D2). The frame is the
// first and last line and the first and last cell of every line between, which
// is how drawPopupBox lays every box out.
func dimPopup(s string, layer int) string {
	dim := lipgloss.NewStyle().Foreground(dimColor)
	frame := lipgloss.NewStyle().Foreground(dimmedLayerColor(layer))
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		plain := ansi.Strip(l)
		if i == 0 || i == len(lines)-1 {
			lines[i] = frame.Render(plain)
			continue
		}
		r := []rune(plain)
		if len(r) < 2 {
			lines[i] = frame.Render(plain)
			continue
		}
		lines[i] = frame.Render(string(r[0])) + dim.Render(string(r[1:len(r)-1])) +
			frame.Render(string(r[len(r)-1]))
	}
	return strings.Join(lines, "\n")
}

// dimmedLayerColor is a layer's border colour pulled halfway to the canvas.
func dimmedLayerColor(layer int) lipgloss.Color {
	return lipgloss.Color(mixHex(string(popupLayerColor(layer)), baseHex, 0.5))
}

// mixHex blends a toward b by t (0 is a, 1 is b). Both are #rrggbb.
func mixHex(a, b string, t float64) string {
	ca, cb := hexRGB(a), hexRGB(b)
	var out [3]int
	for i := range out {
		out[i] = int(float64(ca[i])*(1-t) + float64(cb[i])*t + 0.5)
	}
	return fmt.Sprintf("#%02x%02x%02x", out[0], out[1], out[2])
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
