package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// capsuleGap is the space between two tab capsules. They are deliberately NOT
// joined into a powerline chain (filu's tab-bar vocabulary): a chain says "these
// are pages of one thing", and sshu's three tabs are three co-existing surfaces.
const capsuleGap = 2

// minAppW is the narrowest terminal sshu draws in: the width of the short-label
// capsule strip. Below it the tab row cannot be honest about which tab is lit.
const minAppW = 20

// statusMinRoom is how much slack the tab row needs before it shows the status
// slot at all. Less than this and the text is all ellipsis, which reads as
// breakage rather than information.
const statusMinRoom = 6

// capsule is one rounded pill: round-left cap + flush label + round-right cap.
// Active is a bright blue fill with dark text; inactive is recessed on crust.
// The caps carry the fill colour as their FOREGROUND, so the pill reads as one
// solid rounded shape sitting on the canvas.
func capsule(label string, active bool) string {
	fg, bg := borderDim, lipgloss.Color(crustHex)
	if active {
		fg, bg = lipgloss.Color(baseHex), focusColor
	}
	cap := lipgloss.NewStyle().Foreground(bg)
	body := lipgloss.NewStyle().Foreground(fg).Background(bg)
	if active {
		body = body.Bold(true)
	}
	return cap.Render(capLeft) + body.Render(label) + cap.Render(capRight)
}

// capsulesW is the width of the whole capsule strip, including the leading
// indent and the gaps between pills.
func capsulesW(labels []string) int {
	w := 1
	for i, l := range labels {
		if i > 0 {
			w += capsuleGap
		}
		w += dispW(l) + 2 // two caps
	}
	return w
}

// shortLabels drops each capsule to its "[N]" prefix. This is the narrow-width
// degradation for the tab row: the label is the content signal and losing it
// hurts, but a capsule strip wider than the terminal breaks the frame outright
// (§1.1 — narrow must stay usable).
func shortLabels(labels []string) []string {
	out := make([]string, len(labels))
	for i, l := range labels {
		if k := strings.Index(l, "]"); k >= 0 {
			out[i] = l[:k+1]
		} else {
			out[i] = l
		}
	}
	return out
}

// tabRow is the top content row: capsules on the left, a per-tab status slot
// right-aligned. This row replaces the panel border title entirely — the lit
// capsule is what says which surface you are on (§1.1).
//
// Always exactly one row of exactly w cells (§1.3): the status is truncated,
// the labels shorten, nothing wraps.
func tabRow(w int, labels []string, active int, status string) string {
	if capsulesW(labels) > w {
		labels = shortLabels(labels)
	}

	var b strings.Builder
	b.WriteString(" ")
	for i, lab := range labels {
		if i > 0 {
			b.WriteString(strings.Repeat(" ", capsuleGap))
		}
		b.WriteString(capsule(lab, i == active))
	}
	used := capsulesW(labels)
	if used >= w {
		return b.String() // pathological width; the guard in View catches this
	}

	// Right-align the status with one cell of breathing room at the edge; drop
	// it entirely rather than let it collide with the capsules.
	room := w - used - 1
	if s := truncate(status, room-1); status != "" && room >= statusMinRoom && s != "" {
		b.WriteString(strings.Repeat(" ", room-dispW(s)))
		b.WriteString(lipgloss.NewStyle().Foreground(dimColor).Render(s))
		b.WriteString(" ")
	} else {
		b.WriteString(strings.Repeat(" ", room+1))
	}
	return b.String()
}

// keyLegend renders the footer's "key desc" pairs. This is the mandatory
// disclosure channel for the two VTP entry keys (§A.1 / §A.2): a user who never
// read a README learns Space and ? exist by reading this row.
//
// When the terminal is too narrow, pairs are dropped from the RIGHT — the entry
// keys are listed first precisely so they are the last thing to go.
func keyLegend(pairs [][2]string, w int) string {
	const sep = "   "
	plainW := func(n int) int {
		total := 1
		for i := 0; i < n; i++ {
			if i > 0 {
				total += dispW(sep)
			}
			total += dispW(pairs[i][0]) + 1 + dispW(pairs[i][1])
		}
		return total
	}

	n := len(pairs)
	for n > 0 && plainW(n) > w {
		n--
	}

	k := lipgloss.NewStyle().Foreground(handColor)
	d := lipgloss.NewStyle().Foreground(dimColor)
	parts := make([]string, 0, n)
	for i := 0; i < n; i++ {
		parts = append(parts, k.Render(pairs[i][0])+" "+d.Render(pairs[i][1]))
	}
	return " " + strings.Join(parts, sep) + strings.Repeat(" ", max(0, w-plainW(n)))
}

// tabRule separates the tab row from the panels below it.
//
// It earns its row: without it the tab capsules and the panel capsules sit
// directly against each other and read as one strip of buttons. The line splits
// the screen into "which surface am I on" above and "the surface" below, which
// is what lets the panels wear capsules of their own without competing.
func tabRule(w int) string {
	return lipgloss.NewStyle().Foreground(borderDim).Render(strings.Repeat("─", max(0, w)))
}

func borderColor(focused bool) lipgloss.Color {
	if focused {
		return focusColor
	}
	return borderDim
}

// panelChip is a panel border title as one rounded capsule: round-left cap,
// dark-on-border-colour body, round-right cap.
//
// The capsules came off these titles once, on the grounds that a capsule reads
// as a button and a title is not one. The rule above the panels settles that:
// with the two zones visibly separated, a panel capsule is read as belonging to
// its panel rather than as another tab to press.
func panelChip(title string, focused bool) string {
	bc := borderColor(focused)
	cap := lipgloss.NewStyle().Foreground(bc)
	body := lipgloss.NewStyle().Foreground(lipgloss.Color(baseHex)).Background(bc).Bold(true)
	return cap.Render(capLeft) + body.Render(title) + cap.Render(capRight)
}

// panelChrome frames body in a titled box whose border colour carries focus.
//
// Every line of body must already be innerW cells; short lines are padded, so a
// PTY frame arriving mid-resize cannot shear the border.
func panelChrome(innerW int, body []string, title string, focused bool) string {
	bc := borderColor(focused)
	bs := lipgloss.NewStyle().Foreground(bc)

	chip, chipW := panelChip(title, focused), dispW(title)+2
	if chipW > innerW {
		chip, chipW = "", 0
	}

	out := make([]string, 0, len(body)+2)
	out = append(out, bs.Render("╭")+chip+
		bs.Render(strings.Repeat("─", max(0, innerW-chipW))+"╮"))
	side := bs.Render("│")
	for _, l := range body {
		out = append(out, side+l+strings.Repeat(" ", max(0, innerW-dispW(l)))+side)
	}
	out = append(out, bs.Render("╰"+strings.Repeat("─", innerW)+"╯"))
	return strings.Join(out, "\n")
}

// joinVertical / joinHorizontal are display-width aware block joins.
func joinVertical(blocks ...string) string {
	return lipgloss.JoinVertical(lipgloss.Left, blocks...)
}

func joinHorizontal(blocks ...string) string {
	return lipgloss.JoinHorizontal(lipgloss.Top, blocks...)
}
